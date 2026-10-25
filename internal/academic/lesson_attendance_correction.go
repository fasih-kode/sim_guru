package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

type LessonAttendanceCorrectionInput struct {
	AttendanceID, ExpectedVersion int64
	NewStatus, Reason             string
}

type LessonAttendanceCorrectionResult struct {
	Version        int64
	Noop           bool
	Administrative bool
}

type LessonAttendanceHistoricalRecord struct {
	ID, StudentID, PlacementID, ClassID, SubjectID, TeacherID   int64
	RevisionID, EntryID, AssignmentID, SemesterID               int64
	Version                                                     int64
	Date, StudentName, NIS, ClassName, SubjectName, TeacherName string
	Status, StartsAt, EndsAt, TimeSegments                      string
	PeriodStart, PeriodEnd                                      int
	RecordedAt, UpdatedAt                                       time.Time
	CanCorrect, Administrative                                  bool
	History                                                     []LessonAttendanceHistoryEntry
}

type LessonAttendanceHistoryEntry struct {
	Action, Actor, Kind, Before, After, Reason string
	BeforeVersion, AfterVersion                int64
	At                                         time.Time
}

const lessonAttendanceAccess = `(
	(a.teacher_id IS NOT NULL AND ht.user_id=$1 AND hu.active AND ht.active
	 AND EXISTS(SELECT 1 FROM app.user_roles tr WHERE tr.user_id=$1 AND tr.role_code='guru_mapel'))
	OR
	(EXISTS(SELECT 1 FROM app.user_roles cr WHERE cr.user_id=$1 AND cr.role_code='kurikulum')
	 AND NOT (coalesce(hu.active,false) AND EXISTS(SELECT 1 FROM app.user_roles hr WHERE hr.user_id=hu.id AND hr.role_code='guru_mapel')))
)`

func validateLessonAttendanceCorrection(input LessonAttendanceCorrectionInput) (string, string, error) {
	status := strings.TrimSpace(input.NewStatus)
	reason := strings.TrimSpace(input.Reason)
	if input.AttendanceID < 1 || input.ExpectedVersion < 1 {
		return "", "", ErrNotFound
	}
	if !validLessonAttendanceStatus(status) {
		return "", "", ValidationError("Pilih Hadir, Sakit, Izin, atau Alpa.")
	}
	if reason == "" || !utf8.ValidString(reason) || strings.ContainsRune(reason, 0) || len([]rune(reason)) > 500 {
		return "", "", ValidationError("Alasan koreksi wajib diisi, maksimal 500 karakter.")
	}
	return status, reason, nil
}

// CorrectLessonAttendance mempertahankan identitas kejadian dan hanya mengubah status.
func (s *Store) CorrectLessonAttendance(ctx context.Context, actor int64, input LessonAttendanceCorrectionInput) (LessonAttendanceCorrectionResult, error) {
	result := LessonAttendanceCorrectionResult{}
	status, reason, err := validateLessonAttendanceCorrection(input)
	if err != nil {
		return result, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return result, err
	}
	if err = s.authorize(ctx, tx, actor, "guru_mapel", "kurikulum"); err != nil {
		return result, err
	}

	var current LessonAttendanceHistoricalRecord
	var historicUserID sql.NullInt64
	var historicUserActive, historicHasRole bool
	var snapshot []byte
	err = tx.QueryRowContext(ctx, `SELECT a.id,a.student_id,a.placement_id,a.class_id,a.subject_id,a.teacher_id,a.revision_id,a.timetable_entry_id,a.assignment_id,a.semester_id,a.version,a.meeting_date::text,a.student_name,a.nis,a.class_name,a.subject_name,a.teacher_name,a.status,a.starts_at::text,a.ends_at::text,a.time_segments,a.period_start,a.period_end,a.recorded_at,a.updated_at,
	 a.snapshot,ht.user_id,coalesce(hu.active,false),coalesce(EXISTS(SELECT 1 FROM app.user_roles hr WHERE hr.user_id=hu.id AND hr.role_code='guru_mapel'),false)
	 FROM app.student_lesson_attendance a
	 LEFT JOIN app.teachers ht ON ht.id=a.teacher_id
	 LEFT JOIN app.users hu ON hu.id=ht.user_id
	 WHERE a.id=$1 FOR UPDATE OF a`, input.AttendanceID).Scan(
		&current.ID, &current.StudentID, &current.PlacementID, &current.ClassID, &current.SubjectID, &current.TeacherID, &current.RevisionID, &current.EntryID, &current.AssignmentID, &current.SemesterID, &current.Version, &current.Date, &current.StudentName, &current.NIS, &current.ClassName, &current.SubjectName, &current.TeacherName, &current.Status, &current.StartsAt, &current.EndsAt, &current.TimeSegments, &current.PeriodStart, &current.PeriodEnd, &current.RecordedAt, &current.UpdatedAt, &snapshot, &historicUserID, &historicUserActive, &historicHasRole)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	var ownTeacherID int64
	teacherErr := tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active`, actor).Scan(&ownTeacherID)
	teacherAuthorized := teacherErr == nil && ownTeacherID == current.TeacherID && historicUserID.Valid && historicUserID.Int64 == actor
	if teacherErr != nil && !errors.Is(teacherErr, sql.ErrNoRows) {
		return result, teacherErr
	}
	curriculumAuthorized := !historicUserActive || !historicHasRole || !historicUserID.Valid
	var hasCurriculum bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.user_roles WHERE user_id=$1 AND role_code='kurikulum')`, actor).Scan(&hasCurriculum); err != nil {
		return result, err
	}
	if !teacherAuthorized {
		if !hasCurriculum || !curriculumAuthorized {
			return result, ErrNotFound
		}
		result.Administrative = true
	}
	if current.Version != input.ExpectedVersion {
		return result, ErrConflict
	}
	result.Version = current.Version
	if current.Status == status {
		result.Noop = true
		return result, nil
	}

	before, err := json.Marshal(map[string]any{
		"attendance_id": current.ID, "status": current.Status, "version": current.Version,
		"snapshot": json.RawMessage(snapshot),
	})
	if err != nil {
		return result, err
	}
	after, err := json.Marshal(map[string]any{
		"attendance_id": current.ID, "status": status, "version": current.Version + 1,
		"snapshot":        json.RawMessage(snapshot),
		"correction_type": map[bool]string{true: "kurikulum", false: "guru_mapel"}[result.Administrative],
	})
	if err != nil {
		return result, err
	}
	var updatedAt time.Time
	err = tx.QueryRowContext(ctx, `UPDATE app.student_lesson_attendance SET status=$1,version=version+1,updated_at=clock_timestamp() WHERE id=$2 AND version=$3 RETURNING updated_at`, status, current.ID, input.ExpectedVersion).Scan(&updatedAt)
	if err != nil {
		return result, dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_lesson_attendance_history(attendance_id,actor_id,action,before_state,after_state,reason) VALUES($1,$2,'corrected',$3::jsonb,$4::jsonb,$5)`, current.ID, actor, before, after, reason); err != nil {
		return result, dbError(err)
	}
	if err = tx.Commit(); err != nil {
		return result, dbError(err)
	}
	result.Version = current.Version + 1
	return result, nil
}

// LessonAttendanceRecordsOnDate hanya mengembalikan catatan dalam cakupan historis actor.
func (s *Store) LessonAttendanceRecordsOnDate(ctx context.Context, actor int64, date string) ([]LessonAttendanceHistoricalRecord, error) {
	if _, err := civilDate(date); err != nil {
		return nil, err
	}
	tx, err := s.reader(ctx, actor, "guru_mapel", "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.student_id,a.placement_id,a.class_id,a.subject_id,a.teacher_id,a.revision_id,a.timetable_entry_id,a.assignment_id,a.semester_id,a.version,a.meeting_date::text,a.student_name,a.nis,a.class_name,a.subject_name,a.teacher_name,a.status,a.starts_at::text,a.ends_at::text,a.time_segments,a.period_start,a.period_end,a.recorded_at,a.updated_at,
		(EXISTS(SELECT 1 FROM app.teachers own WHERE own.user_id=$1 AND own.id=a.teacher_id AND own.active AND EXISTS(SELECT 1 FROM app.user_roles rr WHERE rr.user_id=$1 AND rr.role_code='guru_mapel'))),
		(EXISTS(SELECT 1 FROM app.user_roles cr WHERE cr.user_id=$1 AND cr.role_code='kurikulum') AND NOT EXISTS(SELECT 1 FROM app.teachers ht JOIN app.users hu ON hu.id=ht.user_id JOIN app.user_roles hr ON hr.user_id=hu.id AND hr.role_code='guru_mapel' WHERE ht.id=a.teacher_id AND hu.active))
	 FROM app.student_lesson_attendance a LEFT JOIN app.teachers ht ON ht.id=a.teacher_id LEFT JOIN app.users hu ON hu.id=ht.user_id
	 WHERE a.meeting_date=$2 AND `+lessonAttendanceAccess+` ORDER BY a.class_name,a.subject_name,a.student_name,a.id`, actor, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []LessonAttendanceHistoricalRecord
	for rows.Next() {
		var item LessonAttendanceHistoricalRecord
		if err = rows.Scan(&item.ID, &item.StudentID, &item.PlacementID, &item.ClassID, &item.SubjectID, &item.TeacherID, &item.RevisionID, &item.EntryID, &item.AssignmentID, &item.SemesterID, &item.Version, &item.Date, &item.StudentName, &item.NIS, &item.ClassName, &item.SubjectName, &item.TeacherName, &item.Status, &item.StartsAt, &item.EndsAt, &item.TimeSegments, &item.PeriodStart, &item.PeriodEnd, &item.RecordedAt, &item.UpdatedAt, &item.CanCorrect, &item.Administrative); err != nil {
			return nil, err
		}
		records = append(records, item)
	}
	return records, rows.Err()
}

// LessonAttendanceRecord membaca snapshot dan histori berdasarkan ID permanen.
func (s *Store) LessonAttendanceRecord(ctx context.Context, actor, attendanceID int64) (LessonAttendanceHistoricalRecord, error) {
	var item LessonAttendanceHistoricalRecord
	if attendanceID < 1 {
		return item, ErrNotFound
	}
	tx, err := s.reader(ctx, actor, "guru_mapel", "kurikulum")
	if err != nil {
		return item, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT a.id,a.student_id,a.placement_id,a.class_id,a.subject_id,a.teacher_id,a.revision_id,a.timetable_entry_id,a.assignment_id,a.semester_id,a.version,a.meeting_date::text,a.student_name,a.nis,a.class_name,a.subject_name,a.teacher_name,a.status,a.starts_at::text,a.ends_at::text,a.time_segments,a.period_start,a.period_end,a.recorded_at,a.updated_at,a.snapshot,
		(EXISTS(SELECT 1 FROM app.teachers own WHERE own.user_id=$1 AND own.id=a.teacher_id AND own.active AND EXISTS(SELECT 1 FROM app.user_roles rr WHERE rr.user_id=$1 AND rr.role_code='guru_mapel'))),
		(EXISTS(SELECT 1 FROM app.user_roles cr WHERE cr.user_id=$1 AND cr.role_code='kurikulum') AND NOT EXISTS(SELECT 1 FROM app.teachers ht JOIN app.users hu ON hu.id=ht.user_id JOIN app.user_roles hr ON hr.user_id=hu.id AND hr.role_code='guru_mapel' WHERE ht.id=a.teacher_id AND hu.active))
	 FROM app.student_lesson_attendance a LEFT JOIN app.teachers ht ON ht.id=a.teacher_id LEFT JOIN app.users hu ON hu.id=ht.user_id
	 WHERE a.id=$2 AND `+lessonAttendanceAccess, actor, attendanceID).Scan(&item.ID, &item.StudentID, &item.PlacementID, &item.ClassID, &item.SubjectID, &item.TeacherID, &item.RevisionID, &item.EntryID, &item.AssignmentID, &item.SemesterID, &item.Version, &item.Date, &item.StudentName, &item.NIS, &item.ClassName, &item.SubjectName, &item.TeacherName, &item.Status, &item.StartsAt, &item.EndsAt, &item.TimeSegments, &item.PeriodStart, &item.PeriodEnd, &item.RecordedAt, &item.UpdatedAt, new(json.RawMessage), &item.CanCorrect, &item.Administrative)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, err
	}
	item.CanCorrect = item.CanCorrect || item.Administrative
	rows, err := tx.QueryContext(ctx, `SELECT h.action,u.display_name,coalesce(h.after_state->>'correction_type','guru_mapel'),coalesce(h.before_state->>'status',''),h.after_state->>'status',coalesce((h.before_state->>'version')::bigint,0),coalesce((h.after_state->>'version')::bigint,1),coalesce(h.reason,''),h.created_at FROM app.student_lesson_attendance_history h JOIN app.users u ON u.id=h.actor_id WHERE h.attendance_id=$1 ORDER BY h.id`, attendanceID)
	if err != nil {
		return item, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry LessonAttendanceHistoryEntry
		if err = rows.Scan(&entry.Action, &entry.Actor, &entry.Kind, &entry.Before, &entry.After, &entry.BeforeVersion, &entry.AfterVersion, &entry.Reason, &entry.At); err != nil {
			return item, err
		}
		item.History = append(item.History, entry)
	}
	return item, rows.Err()
}
