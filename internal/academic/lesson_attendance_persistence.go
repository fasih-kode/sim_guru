package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type LessonAttendanceChoice struct {
	StudentID int64
	Status    string
}

type LessonAttendanceInput struct {
	Date, Fingerprint string
	EntryID           int64
	Students          []LessonAttendanceChoice
}

type LessonAttendanceSaveResult struct {
	Inserted   int
	Idempotent bool
}

type lessonAttendanceSnapshot struct {
	ActorID, TeacherID     int64
	SemesterID, RevisionID int64
	Date                   string
	SchoolDay              bool
	Calendar               SemesterCalendar
	Lesson                 TimetableEntry
	Periods                []LessonPeriod
	PlanSnapshot           json.RawMessage
	Student                AttendanceStudent
	Assignment             lessonAssignmentSnapshot
	ClassVersion           int64
	TimeSegments           string
}

type lessonAssignmentSnapshot struct {
	ID, SemesterID, ClassID, SubjectID, TeacherID int64
	EffectiveFrom, EffectiveUntil                 string
}

type lessonFingerprintPayload struct {
	ActorID, TeacherID     int64
	SemesterID, RevisionID int64
	Date                   string
	SchoolDay              bool
	Calendar               SemesterCalendar
	Roster                 []AttendanceStudent
	Meetings               []lessonMeetingFingerprint
}

type lessonMeetingFingerprint struct {
	Lesson       TimetableEntry
	ClassVersion int64
	Assignment   lessonAssignmentSnapshot
	Periods      []LessonPeriod
	PlanSnapshot json.RawMessage
}

// SaveLessonAttendance mencatat pilihan eksplisit dalam satu transaksi atomik.
func (s *Store) SaveLessonAttendance(ctx context.Context, actor int64, in LessonAttendanceInput) (LessonAttendanceSaveResult, error) {
	result := LessonAttendanceSaveResult{}
	if in.EntryID < 1 {
		return result, ErrNotFound
	}
	if _, err := civilDate(in.Date); err != nil {
		return result, err
	}
	if strings.TrimSpace(in.Fingerprint) == "" || len(in.Students) == 0 {
		return result, ValidationError("Pilih status sedikitnya satu siswa dari formulir terbaru.")
	}
	seen := make(map[int64]bool, len(in.Students))
	selected := make([]LessonAttendanceChoice, 0, len(in.Students))
	for _, choice := range in.Students {
		if choice.StudentID < 1 || seen[choice.StudentID] {
			return result, ValidationError("Pilihan siswa tidak valid atau berulang.")
		}
		seen[choice.StudentID] = true
		if choice.Status == "" {
			continue
		}
		if !validLessonAttendanceStatus(choice.Status) {
			return result, ValidationError("Pilih Hadir, Sakit, Izin, atau Alpa.")
		}
		selected = append(selected, choice)
	}
	if len(selected) == 0 {
		return result, ValidationError("Pilih status sedikitnya satu siswa.")
	}

	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var teacherID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active`, actor).Scan(&teacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrForbidden
	}
	if err != nil {
		return result, err
	}
	var classID, entryTeacherID int64
	err = tx.QueryRowContext(ctx, `SELECT e.class_id,e.teacher_id FROM app.weekly_timetable_entries e JOIN app.weekly_timetable_revisions r ON r.id=e.revision_id JOIN app.semester_calendars cal ON cal.semester_id=r.semester_id WHERE e.id=$1 AND r.status='published' AND r.effective_from<=$2::date AND $2::date BETWEEN cal.starts_on AND cal.ends_on`, in.EntryID, in.Date).Scan(&classID, &entryTeacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if entryTeacherID != teacherID {
		return result, ErrNotFound
	}
	contextForDate, err := s.attendanceContextOnDateTx(ctx, tx, actor, classID, in.Date, teacherID, true)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			return result, ErrConflict
		}
		return result, err
	}
	if !contextForDate.SchoolDay || contextForDate.RevisionID == 0 || contextForDate.Fingerprint == "" {
		return result, ValidationError("Tidak ada pertemuan terbit pada hari sekolah yang dipilih.")
	}
	var lesson *TimetableEntry
	for i := range contextForDate.Lessons {
		if contextForDate.Lessons[i].ID == in.EntryID {
			lesson = &contextForDate.Lessons[i]
			break
		}
	}
	if lesson == nil {
		return result, ErrConflict
	}
	if contextForDate.Fingerprint != in.Fingerprint {
		return result, ErrConflict
	}
	roster := make(map[int64]AttendanceStudent, len(contextForDate.Students))
	for _, student := range contextForDate.Students {
		roster[student.ID] = student
	}
	calendar, err := querySemesterCalendar(ctx, tx, contextForDate.SemesterID)
	if err != nil {
		return result, err
	}
	for _, choice := range selected {
		if _, ok := roster[choice.StudentID]; !ok {
			return result, ValidationError("Ada siswa yang tidak termasuk roster pertemuan pada tanggal tersebut.")
		}
	}
	var dbNow time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&dbNow); err != nil {
		return result, err
	}
	if err = validateLessonAttendanceAt(dbNow, in.Date, lesson.StartsAt); err != nil {
		return result, err
	}

	snapshotByStudent := make(map[int64][]byte, len(selected))
	segmentsByStudent := make(map[int64]string, len(selected))
	for _, choice := range selected {
		student := roster[choice.StudentID]
		snapshot, err := lessonAttendanceSnapshotForStudent(ctx, tx, actor, teacherID, contextForDate, calendar, *lesson, student)
		if err != nil {
			return result, err
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return result, err
		}
		snapshotByStudent[choice.StudentID] = encoded
		segmentsByStudent[choice.StudentID] = snapshot.TimeSegments
	}

	// Periksa semua baris lebih dahulu agar retry identik tidak mengubah bukti lama.
	for _, choice := range selected {
		var oldStatus string
		err = tx.QueryRowContext(ctx, `SELECT status FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id=$3 FOR UPDATE`, lesson.ID, in.Date, choice.StudentID).Scan(&oldStatus)
		if err == nil {
			if oldStatus != choice.Status {
				return result, ErrConflict
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
	}
	if len(selected) > 0 {
		if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&dbNow); err != nil {
			return result, err
		}
		if err = validateLessonAttendanceAt(dbNow, in.Date, lesson.StartsAt); err != nil {
			return result, err
		}
	}

	for _, choice := range selected {
		student := roster[choice.StudentID]
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id=$3)`, lesson.ID, in.Date, choice.StudentID).Scan(&exists)
		if err != nil {
			return result, err
		}
		if exists {
			continue
		}
		var id int64
		err = tx.QueryRowContext(ctx, `INSERT INTO app.student_lesson_attendance(timetable_entry_id,revision_id,assignment_id,semester_id,meeting_date,student_id,placement_id,class_id,subject_id,teacher_id,student_name,nis,class_name,subject_name,teacher_name,period_start,period_end,starts_at,ends_at,time_segments,snapshot,status,recorded_by,recorded_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21::jsonb,$22,$23,$24,$24) RETURNING id`, lesson.ID, contextForDate.RevisionID, lesson.AssignmentID, contextForDate.SemesterID, in.Date, student.ID, student.PlacementID, lesson.ClassID, lesson.SubjectID, lesson.TeacherID, student.Name, student.NIS, lesson.ClassName, lesson.SubjectName, lesson.TeacherName, lesson.PeriodStart, lesson.PeriodEnd, lesson.StartsAt, lesson.EndsAt, segmentsByStudent[choice.StudentID], snapshotByStudent[choice.StudentID], choice.Status, actor, dbNow).Scan(&id)
		if err != nil {
			return result, dbError(err)
		}
		after, err := json.Marshal(map[string]any{"status": choice.Status, "meeting_date": in.Date, "snapshot": json.RawMessage(snapshotByStudent[choice.StudentID])})
		if err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_lesson_attendance_history(attendance_id,actor_id,action,before_state,after_state,reason) VALUES($1,$2,'recorded',NULL,$3::jsonb,NULL)`, id, actor, after); err != nil {
			return result, err
		}
		result.Inserted++
	}
	if result.Inserted == 0 {
		result.Idempotent = true
		return result, nil
	}
	if err = tx.Commit(); err != nil {
		return LessonAttendanceSaveResult{}, dbError(err)
	}
	return result, nil
}

func validLessonAttendanceStatus(status string) bool {
	return status == "hadir" || status == "sakit" || status == "izin" || status == "alpa"
}

func validateLessonAttendanceAt(now time.Time, date, startsAt string) error {
	meetingDate, err := civilDate(date)
	if err != nil {
		return err
	}
	wib, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return fmt.Errorf("memuat zona waktu Asia/Jakarta: %w", err)
	}
	if date > now.In(wib).Format("2006-01-02") {
		return ValidationError("Absensi tidak dapat dicatat untuk tanggal pelajaran mendatang.")
	}
	start, err := time.ParseInLocation("15:04:05", startsAt, wib)
	if err != nil {
		start, err = time.ParseInLocation("15:04", startsAt, wib)
	}
	if err != nil {
		return ValidationError("Waktu mulai pertemuan pada jadwal tidak valid.")
	}
	startAt := time.Date(meetingDate.Year(), meetingDate.Month(), meetingDate.Day(), start.Hour(), start.Minute(), start.Second(), 0, wib)
	if now.Before(startAt) {
		return ValidationError("Absensi belum dapat dicatat sebelum waktu mulai pelajaran.")
	}
	return nil
}

func lessonAttendanceSnapshotForStudent(ctx context.Context, tx *sql.Tx, actor, teacherID int64, attendance AttendanceContext, calendar SemesterCalendar, lesson TimetableEntry, student AttendanceStudent) (lessonAttendanceSnapshot, error) {
	snapshot := lessonAttendanceSnapshot{ActorID: actor, TeacherID: teacherID, SemesterID: attendance.SemesterID, RevisionID: attendance.RevisionID, Date: attendance.Date, SchoolDay: attendance.SchoolDay, Calendar: calendar, Lesson: lesson, Student: student}
	if err := tx.QueryRowContext(ctx, `SELECT version FROM app.classes WHERE id=$1`, lesson.ClassID).Scan(&snapshot.ClassVersion); err != nil {
		return snapshot, err
	}
	err := tx.QueryRowContext(ctx, `SELECT id,semester_id,class_id,subject_id,teacher_id,effective_from::text,coalesce(effective_until::text,'') FROM app.teaching_assignments WHERE id=$1 AND semester_id=$2 AND class_id=$3 AND subject_id=$4 AND teacher_id=$5 AND effective_from<=$6::date AND (effective_until IS NULL OR effective_until>$6::date)`, lesson.AssignmentID, attendance.SemesterID, lesson.ClassID, lesson.SubjectID, teacherID, attendance.Date).Scan(&snapshot.Assignment.ID, &snapshot.Assignment.SemesterID, &snapshot.Assignment.ClassID, &snapshot.Assignment.SubjectID, &snapshot.Assignment.TeacherID, &snapshot.Assignment.EffectiveFrom, &snapshot.Assignment.EffectiveUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, ErrConflict
	}
	if err != nil {
		return snapshot, err
	}
	snapshot.Periods, err = queryRevisionPeriods(ctx, tx, attendance.RevisionID)
	if err != nil {
		return snapshot, err
	}
	snapshot.TimeSegments = entrySegments(lesson, snapshot.Periods)
	var planSnapshot []byte
	if err = tx.QueryRowContext(ctx, `SELECT plan_snapshot FROM app.weekly_timetable_revisions WHERE id=$1 AND status='published'`, attendance.RevisionID).Scan(&planSnapshot); err != nil {
		return snapshot, err
	}
	snapshot.PlanSnapshot = json.RawMessage(planSnapshot)
	return snapshot, nil
}

// lessonAttendanceFingerprint mengikat form pada data yang benar-benar dibaca.
func lessonAttendanceFingerprint(ctx context.Context, tx *sql.Tx, actor, teacherID int64, attendance AttendanceContext, calendar SemesterCalendar) (string, error) {
	payload := lessonFingerprintPayload{ActorID: actor, TeacherID: teacherID, SemesterID: attendance.SemesterID, RevisionID: attendance.RevisionID, Date: attendance.Date, SchoolDay: attendance.SchoolDay, Calendar: calendar, Roster: append([]AttendanceStudent(nil), attendance.Students...)}
	sort.Slice(payload.Roster, func(i, j int) bool { return payload.Roster[i].ID < payload.Roster[j].ID })
	lessons := append([]TimetableEntry(nil), attendance.Lessons...)
	sort.Slice(lessons, func(i, j int) bool {
		if lessons[i].Weekday != lessons[j].Weekday {
			return lessons[i].Weekday < lessons[j].Weekday
		}
		if lessons[i].PeriodStart != lessons[j].PeriodStart {
			return lessons[i].PeriodStart < lessons[j].PeriodStart
		}
		return lessons[i].ID < lessons[j].ID
	})
	for _, lesson := range lessons {
		var meeting lessonMeetingFingerprint
		meeting.Lesson = lesson
		if err := tx.QueryRowContext(ctx, `SELECT version FROM app.classes WHERE id=$1`, lesson.ClassID).Scan(&meeting.ClassVersion); err != nil {
			return "", err
		}
		err := tx.QueryRowContext(ctx, `SELECT id,semester_id,class_id,subject_id,teacher_id,effective_from::text,coalesce(effective_until::text,'') FROM app.teaching_assignments WHERE id=$1`, lesson.AssignmentID).Scan(&meeting.Assignment.ID, &meeting.Assignment.SemesterID, &meeting.Assignment.ClassID, &meeting.Assignment.SubjectID, &meeting.Assignment.TeacherID, &meeting.Assignment.EffectiveFrom, &meeting.Assignment.EffectiveUntil)
		if err != nil {
			return "", err
		}
		allPeriods, err := queryRevisionPeriods(ctx, tx, attendance.RevisionID)
		if err != nil {
			return "", err
		}
		for _, period := range allPeriods {
			if period.Weekday == lesson.Weekday && period.Number >= lesson.PeriodStart && period.Number <= lesson.PeriodEnd {
				meeting.Periods = append(meeting.Periods, period)
			}
		}
		var planSnapshot []byte
		if err = tx.QueryRowContext(ctx, `SELECT plan_snapshot FROM app.weekly_timetable_revisions WHERE id=$1`, attendance.RevisionID).Scan(&planSnapshot); err != nil {
			return "", err
		}
		meeting.PlanSnapshot = json.RawMessage(planSnapshot)
		payload.Meetings = append(payload.Meetings, meeting)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
