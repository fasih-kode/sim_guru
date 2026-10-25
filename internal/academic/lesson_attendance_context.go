package academic

import (
	"context"
	"database/sql"
	"errors"
	"sort"
)

// TeacherLessonDate berisi pertemuan terbit yang boleh dibuka guru pada tanggal tertentu.
type TeacherLessonDate struct {
	Date    string
	Lessons []TimetableEntry
}

// TeacherLessonAttendanceContext adalah konteks baca-saja untuk satu pertemuan.
type TeacherLessonAttendanceContext struct {
	Date     string
	Lesson   TimetableEntry
	Students []AttendanceStudent
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
		// Fondasi menghitung kalender, revisi terbit, dan roster untuk kelas pada tanggal ini.
		attendance, contextErr := s.attendanceContextForTeacherClass(ctx, actor, teacherID, classID, date)
		if contextErr != nil {
			return result, contextErr
		}
		for _, lesson := range attendance.Lessons {
			if lesson.TeacherID == teacherID && assignments[lesson.AssignmentID] {
				result.Lessons = append(result.Lessons, lesson)
			}
		}
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
	// Baca roster lewat jalur ketat agar role tambahan tidak memperluas akses.
	attendance, err := s.attendanceContextForTeacherClass(ctx, actor, selected.TeacherID, selected.ClassID, date)
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
			return result, nil
		}
	}
	// Jadwal atau penugasan berubah di antara pembacaan; jangan tampilkan konteks yang basi.
	return result, ErrNotFound
}

// attendanceContextForTeacherClass memakai fondasi kalender/roster tanpa memberi cakupan role rangkap.
func (s *Store) attendanceContextForTeacherClass(ctx context.Context, actor, teacherID, classID int64, date string) (AttendanceContext, error) {
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
	return s.attendanceContextOnDateTx(ctx, tx, actor, classID, date, teacherID)
}
