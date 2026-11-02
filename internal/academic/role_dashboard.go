package academic

import (
	"context"
	"database/sql"
	"errors"
)

// TeacherDashboardStats hanya berisi ringkasan jadwal resmi dan presensi milik guru.
type TeacherDashboardStats struct {
	Date                                     string
	HasActiveSemester, HasPublishedTimetable bool
	TodayMeetings, WeeklyMeetings            int64
	ActiveClasses                            int64
	HasCheckIn, HasCheckOut                  bool
}

func (s *Store) dashboardReader(ctx context.Context, actor int64, role string) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	if err = s.authorize(ctx, tx, actor, role); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// TeacherDashboard membaca jadwal serta presensi pribadi dalam cakupan akun aktif.
func (s *Store) TeacherDashboard(ctx context.Context, actor int64) (TeacherDashboardStats, error) {
	var result TeacherDashboardStats
	tx, err := s.dashboardReader(ctx, actor, "guru_mapel")
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
	var semesterID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text,
		       (SELECT sm.id FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id
		        WHERE sm.active AND y.active LIMIT 1)`,
	).Scan(&result.Date, &semesterID)
	if err != nil {
		return result, err
	}
	if semesterID.Valid {
		result.HasActiveSemester = true
		var revisionID sql.NullInt64
		err = tx.QueryRowContext(ctx, `SELECT id FROM app.weekly_timetable_revisions
			WHERE semester_id=$1 AND status='published' AND effective_from<=$2::date
			AND EXISTS(SELECT 1 FROM app.semester_calendars cal WHERE cal.semester_id=$1 AND $2::date BETWEEN cal.starts_on AND cal.ends_on)
			ORDER BY effective_from DESC,revision_no DESC LIMIT 1`, semesterID.Int64, result.Date).Scan(&revisionID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		if revisionID.Valid {
			result.HasPublishedTimetable = true
			err = tx.QueryRowContext(ctx, `SELECT
				count(*) FILTER (WHERE e.weekday=extract(isodow FROM $2::date)::smallint),
				count(*)
				FROM app.weekly_timetable_entries e
				JOIN app.teaching_assignments a ON a.id=e.assignment_id AND a.semester_id=$3
				 AND a.class_id=e.class_id AND a.teacher_id=e.teacher_id
			 AND a.effective_from<=$2::date AND (a.effective_until IS NULL OR a.effective_until>$2::date)
			WHERE e.revision_id=$1 AND e.teacher_id=$4`, revisionID.Int64, result.Date, semesterID.Int64, teacherID).Scan(&result.TodayMeetings, &result.WeeklyMeetings)
			if err != nil {
				return result, err
			}
		}
		err = tx.QueryRowContext(ctx, `SELECT count(DISTINCT a.class_id) FROM app.teaching_assignments a
			JOIN app.semesters sm ON sm.id=a.semester_id
			JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id
			WHERE a.semester_id=$1 AND a.teacher_id=$2 AND a.effective_from<=$3::date
			AND (a.effective_until IS NULL OR a.effective_until>$3::date)`,
			semesterID.Int64, teacherID, result.Date).Scan(&result.ActiveClasses)
		if err != nil {
			return result, err
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM app.teacher_attendance_events WHERE teacher_id=$1 AND on_date=$2::date AND action='check_in'),
		EXISTS(SELECT 1 FROM app.teacher_attendance_events WHERE teacher_id=$1 AND on_date=$2::date AND action='check_out')`,
		teacherID, result.Date).Scan(&result.HasCheckIn, &result.HasCheckOut)
	if err != nil {
		return result, err
	}
	return result, nil
}

// HomeroomDashboardStats hanya merangkum kelas binaan aktif, roster saat ini,
// bukti absensi hari ini, dan jumlah kasus terbuka tanpa membaca isi catatan.
type HomeroomDashboardStats struct {
	Date                           string
	HasHomeroom, HasAttendance     bool
	Classes, Students              int64
	AttendanceRecorded             int64
	Present, Sick, Excused, Absent int64
	OpenCases                      int64
	ClassNames                     []string
}

func (s *Store) HomeroomDashboard(ctx context.Context, actor int64) (HomeroomDashboardStats, error) {
	var result HomeroomDashboardStats
	tx, err := s.dashboardReader(ctx, actor, "wali_kelas")
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&result.Date); err != nil {
		return result, err
	}
	err = tx.QueryRowContext(ctx, `
		WITH wali_classes AS (
			SELECT c.id,c.grade::text||c.name AS label
			FROM app.homeroom_assignments h
			JOIN app.teachers t ON t.id=h.teacher_id AND t.user_id=$1 AND t.active
			JOIN app.classes c ON c.id=h.class_id
			JOIN app.school_years y ON y.id=c.school_year_id AND y.active
		), roster AS (
			SELECT p.class_id,p.student_id FROM app.current_student_placements p
			JOIN app.students s ON s.id=p.student_id JOIN wali_classes wc ON wc.id=p.class_id
		)
		SELECT count(DISTINCT wc.id),count(DISTINCT r.student_id)
		FROM wali_classes wc LEFT JOIN roster r ON r.class_id=wc.id`, actor).Scan(&result.Classes, &result.Students)
	if err != nil {
		return result, err
	}
	result.HasHomeroom = result.Classes > 0
	if result.HasHomeroom {
		rows, queryErr := tx.QueryContext(ctx, `SELECT DISTINCT c.grade::text||c.name FROM app.homeroom_assignments h
			JOIN app.teachers t ON t.id=h.teacher_id AND t.user_id=$1 AND t.active
			JOIN app.classes c ON c.id=h.class_id JOIN app.school_years y ON y.id=c.school_year_id AND y.active
			ORDER BY 1`, actor)
		if queryErr != nil {
			return result, queryErr
		}
		for rows.Next() {
			var name string
			if queryErr = rows.Scan(&name); queryErr != nil {
				rows.Close()
				return result, queryErr
			}
			result.ClassNames = append(result.ClassNames, name)
		}
		if queryErr = rows.Err(); queryErr != nil {
			rows.Close()
			return result, queryErr
		}
		rows.Close()
		queryErr = tx.QueryRowContext(ctx, `SELECT count(*),
			count(*) FILTER (WHERE a.status='hadir'), count(*) FILTER (WHERE a.status='sakit'),
			count(*) FILTER (WHERE a.status='izin'), count(*) FILTER (WHERE a.status='alpa')
			FROM app.student_daily_attendance a
			JOIN app.current_student_placements p ON p.student_id=a.student_id AND p.class_id=a.class_id
			JOIN app.homeroom_assignments h ON h.class_id=p.class_id
			JOIN app.teachers t ON t.id=h.teacher_id AND t.user_id=$1 AND t.active
			JOIN app.classes c ON c.id=h.class_id JOIN app.school_years y ON y.id=c.school_year_id AND y.active
			WHERE a.on_date=$2::date`, actor, result.Date).Scan(&result.AttendanceRecorded, &result.Present, &result.Sick, &result.Excused, &result.Absent)
		if queryErr != nil {
			return result, queryErr
		}
		result.HasAttendance = result.AttendanceRecorded > 0
		queryErr = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.student_cases c WHERE c.status='perlu_tindak_lanjut' AND `+currentCaseWaliScope, actor).Scan(&result.OpenCases)
		if queryErr != nil {
			return result, queryErr
		}
	}
	return result, nil
}
