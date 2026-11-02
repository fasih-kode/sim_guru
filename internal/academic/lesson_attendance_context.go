package academic

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

// TeacherLessonDate berisi pertemuan terbit yang boleh dibuka guru pada tanggal tertentu.
type TeacherLessonDate struct {
	Date        string
	Lessons     []TimetableEntry
	Eligibility map[int64]TeacherLessonEligibility
}

// TeacherLessonAttendanceContext adalah konteks baca-saja untuk satu pertemuan.
type TeacherLessonAttendanceContext struct {
	Date                  string
	Fingerprint           string
	Lesson                TimetableEntry
	AttendanceEligibility TeacherLessonEligibility
	Students              []AttendanceStudent
	Records               []LessonAttendanceRecord
}

// TeacherLessonAttendanceRevisionContext hanya membuka konteks pengganti setelah
// penugasan guru pada kelas dan mapel yang sama diverifikasi ulang.
type TeacherLessonAttendanceRevisionContext struct {
	Lessons     []TimetableEntry
	Eligibility map[int64]TeacherLessonEligibility
	Replacement *TeacherLessonAttendanceContext
}

// LessonAttendanceRecord adalah bukti status yang sudah tersimpan untuk pertemuan.
type LessonAttendanceRecord struct {
	StudentID   int64
	StudentName string
	NIS         string
	Status      string
	RecordedAt  time.Time
}

// TeacherLessonsOnDate membatasi daftar pada penugasan Guru Mapel yang efektif.
func (s *Store) TeacherLessonsOnDate(ctx context.Context, actor int64, date string) (TeacherLessonDate, error) {
	result := TeacherLessonDate{Date: date}
	if _, err := civilDate(date); err != nil {
		return result, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return result, err
	}
	var teacherID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active`, actor).Scan(&teacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrForbidden
	}
	if err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT a.id,a.class_id FROM app.teaching_assignments a JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id JOIN app.semester_calendars cal ON cal.semester_id=sm.id WHERE a.teacher_id=$1 AND a.effective_from<=$2::date AND (a.effective_until IS NULL OR a.effective_until>$2::date) AND $2::date BETWEEN cal.starts_on AND cal.ends_on ORDER BY a.class_id,a.id`, teacherID, date)
	if err != nil {
		return result, err
	}
	type assignmentScope struct{ id, classID int64 }
	var scopes []assignmentScope
	for rows.Next() {
		var scope assignmentScope
		if err = rows.Scan(&scope.id, &scope.classID); err != nil {
			rows.Close()
			return result, err
		}
		scopes = append(scopes, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	assignmentsByClass := make(map[int64]map[int64]bool)
	for _, scope := range scopes {
		if assignmentsByClass[scope.classID] == nil {
			assignmentsByClass[scope.classID] = make(map[int64]bool)
		}
		assignmentsByClass[scope.classID][scope.id] = true
	}
	for classID, assignments := range assignmentsByClass {
		// Daftar pertemuan perlu kalender dan jadwal, tidak membutuhkan roster siswa.
		attendance, contextErr := s.attendanceContextForTeacherClass(ctx, actor, teacherID, classID, date, false)
		if contextErr != nil {
			return result, contextErr
		}
		for _, lesson := range attendance.Lessons {
			if lesson.TeacherID == teacherID && assignments[lesson.AssignmentID] {
				result.Lessons = append(result.Lessons, lesson)
			}
		}
	}
	evidence, err := teacherLessonAttendanceEvidenceTx(ctx, tx, teacherID, date)
	if err != nil {
		return result, err
	}
	result.Eligibility = make(map[int64]TeacherLessonEligibility, len(result.Lessons))
	for _, lesson := range result.Lessons {
		result.Eligibility[lesson.ID] = evaluateTeacherLessonEligibility(evidence, date, lesson.EndsAt)
	}
	sort.Slice(result.Lessons, func(i, j int) bool {
		a, b := result.Lessons[i], result.Lessons[j]
		if a.Weekday != b.Weekday {
			return a.Weekday < b.Weekday
		}
		if a.PeriodStart != b.PeriodStart {
			return a.PeriodStart < b.PeriodStart
		}
		if a.ClassName != b.ClassName {
			return a.ClassName < b.ClassName
		}
		return a.ID < b.ID
	})
	return result, nil
}

// TeacherLessonAttendanceOnDate mengembalikan roster bertanggal hanya untuk pertemuan milik guru.
func (s *Store) TeacherLessonAttendanceOnDate(ctx context.Context, actor int64, date string, entryID int64) (TeacherLessonAttendanceContext, error) {
	result := TeacherLessonAttendanceContext{Date: date}
	if entryID < 1 {
		return result, ErrNotFound
	}
	meetings, err := s.TeacherLessonsOnDate(ctx, actor, date)
	if err != nil {
		return result, err
	}
	var selected *TimetableEntry
	for i := range meetings.Lessons {
		if meetings.Lessons[i].ID == entryID {
			selected = &meetings.Lessons[i]
			break
		}
	}
	if selected == nil {
		return result, ErrNotFound
	}
	result.AttendanceEligibility = meetings.Eligibility[selected.ID]
	// Baca roster lewat jalur ketat agar role tambahan tidak memperluas akses.
	attendance, err := s.attendanceContextForTeacherClass(ctx, actor, selected.TeacherID, selected.ClassID, date, true)
	if err != nil {
		return result, err
	}
	if !attendance.SchoolDay || attendance.RevisionID == 0 {
		return result, ErrNotFound
	}
	for _, lesson := range attendance.Lessons {
		if lesson.ID == selected.ID && lesson.TeacherID == selected.TeacherID && lesson.AssignmentID == selected.AssignmentID && attendance.RevisionID != 0 {
			result.Lesson = lesson
			result.Students = attendance.Students
			result.Fingerprint = attendance.Fingerprint
			rows, queryErr := s.DB.QueryContext(ctx, `SELECT student_id,student_name,nis,status,recorded_at FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND teacher_id=$3 ORDER BY student_name,student_id`, lesson.ID, date, selected.TeacherID)
			if queryErr != nil {
				return TeacherLessonAttendanceContext{}, queryErr
			}
			for rows.Next() {
				var record LessonAttendanceRecord
				if queryErr = rows.Scan(&record.StudentID, &record.StudentName, &record.NIS, &record.Status, &record.RecordedAt); queryErr != nil {
					rows.Close()
					return TeacherLessonAttendanceContext{}, queryErr
				}
				result.Records = append(result.Records, record)
			}
			queryErr = rows.Err()
			rows.Close()
			if queryErr != nil {
				return TeacherLessonAttendanceContext{}, queryErr
			}
			return result, nil
		}
	}
	// Jadwal atau penugasan berubah di antara pembacaan; jangan tampilkan konteks yang basi.
	return result, ErrNotFound
}

// TeacherLessonAttendanceAfterRevision menangani formulir lama tanpa membuka
// roster jika guru atau penugasan pada konteks tersebut sudah tidak berwenang.
func (s *Store) TeacherLessonAttendanceAfterRevision(ctx context.Context, actor int64, date string, oldEntryID int64) (TeacherLessonAttendanceRevisionContext, error) {
	result := TeacherLessonAttendanceRevisionContext{}
	if oldEntryID < 1 {
		return result, ErrNotFound
	}
	d, err := civilDate(date)
	if err != nil {
		return result, err
	}
	weekday := int(d.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	tx, err := s.reader(ctx, actor, "guru_mapel")
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
	var classID, subjectID, oldTeacherID, semesterID, oldRevisionID, currentRevisionID int64
	err = tx.QueryRowContext(ctx, `SELECT e.class_id,e.subject_id,e.teacher_id,r.semester_id,r.id,
		(SELECT current_revision.id FROM app.weekly_timetable_revisions current_revision WHERE current_revision.semester_id=r.semester_id AND current_revision.status='published' AND current_revision.effective_from<=$2::date ORDER BY current_revision.effective_from DESC,current_revision.revision_no DESC LIMIT 1)
		FROM app.weekly_timetable_entries e
		JOIN app.weekly_timetable_revisions r ON r.id=e.revision_id AND r.status='published' AND r.effective_from<=$2::date
		JOIN app.semester_calendars cal ON cal.semester_id=r.semester_id AND $2::date BETWEEN cal.starts_on AND cal.ends_on
		WHERE e.id=$1 AND e.weekday=$3`, oldEntryID, date, weekday).Scan(&classID, &subjectID, &oldTeacherID, &semesterID, &oldRevisionID, &currentRevisionID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if oldTeacherID != teacherID || currentRevisionID == 0 || currentRevisionID == oldRevisionID {
		return result, ErrNotFound
	}
	var assigned bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_assignments a JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id WHERE a.semester_id=$1 AND a.class_id=$2 AND a.subject_id=$3 AND a.teacher_id=$4 AND a.effective_from<=$5::date AND (a.effective_until IS NULL OR a.effective_until>$5::date))`, semesterID, classID, subjectID, teacherID, date).Scan(&assigned)
	if err != nil {
		return result, err
	}
	if !assigned {
		return result, ErrNotFound
	}
	tx.Rollback()

	meetings, err := s.TeacherLessonsOnDate(ctx, actor, date)
	if err != nil {
		return result, err
	}
	result.Lessons = meetings.Lessons
	result.Eligibility = meetings.Eligibility
	replacementID, unique := UniqueReplacementLesson(meetings.Lessons, classID, subjectID, teacherID)
	if !unique {
		return result, nil
	}
	replacement, err := s.TeacherLessonAttendanceOnDate(ctx, actor, date, replacementID)
	if err != nil {
		var validation ValidationError
		if errors.As(err, &validation) {
			// Daftar terbaru tetap aman ditampilkan, tetapi roster yang tidak lengkap
			// tidak boleh dipakai untuk memetakan pilihan formulir lama.
			return result, nil
		}
		return result, err
	}
	result.Replacement = &replacement
	return result, nil
}

// UniqueReplacementLesson memilih pertemuan hanya bila tepat satu entri cocok
// berdasarkan kelas, mata pelajaran, dan guru.
func UniqueReplacementLesson(lessons []TimetableEntry, classID, subjectID, teacherID int64) (int64, bool) {
	var replacementID int64
	for _, lesson := range lessons {
		if lesson.ClassID != classID || lesson.SubjectID != subjectID || lesson.TeacherID != teacherID {
			continue
		}
		if replacementID != 0 {
			return 0, false
		}
		replacementID = lesson.ID
	}
	return replacementID, replacementID != 0
}

// attendanceContextForTeacherClass memakai fondasi jadwal dengan cakupan Guru Mapel yang ketat.
func (s *Store) attendanceContextForTeacherClass(ctx context.Context, actor, teacherID, classID int64, date string, includeRoster bool) (AttendanceContext, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return AttendanceContext{}, err
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return AttendanceContext{}, err
	}
	var mappedTeacher int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active`, actor).Scan(&mappedTeacher)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && mappedTeacher != teacherID) {
		return AttendanceContext{}, ErrForbidden
	}
	if err != nil {
		return AttendanceContext{}, err
	}
	return s.attendanceContextOnDateTx(ctx, tx, actor, classID, date, teacherID, includeRoster)
}
