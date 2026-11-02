package academic

import (
	"context"
	"database/sql"
	"errors"
)

// PersonalAttendanceEligibility mengikuti identitas dan role yang diperiksa oleh service presensi.
type PersonalAttendanceEligibility struct {
	Teacher, Staff bool
}

// PersonalAttendanceEligibility memeriksa kelayakan presensi pribadi tanpa mengubah data.
func (s *Store) PersonalAttendanceEligibility(ctx context.Context, actor int64) (PersonalAttendanceEligibility, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return PersonalAttendanceEligibility{}, err
	}
	defer tx.Rollback()
	var result PersonalAttendanceEligibility
	err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM app.users u JOIN app.teachers t ON t.user_id=u.id AND t.active
			WHERE u.id=$1 AND u.active AND EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=u.id AND r.role_code IN ('guru_mapel','wali_kelas','kepala_sekolah'))),
		EXISTS(SELECT 1 FROM app.users u JOIN app.staff_members s ON s.user_id=u.id AND s.active
			WHERE u.id=$1 AND u.active AND EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=u.id AND r.role_code IN ('pegawai_tu','kepala_sekolah')))`, actor).
		Scan(&result.Teacher, &result.Staff)
	return result, err
}

// StaffSelfDashboardStats hanya memuat status event milik pegawai yang login.
type StaffSelfDashboardStats struct {
	Date                    string
	HasCheckIn, HasCheckOut bool
}

// StaffSelfDashboard membaca status kehadiran pribadi tanpa membuka data pegawai lain.
func (s *Store) StaffSelfDashboard(ctx context.Context, actor int64) (StaffSelfDashboardStats, error) {
	tx, err := s.dashboardReader(ctx, actor, "pegawai_tu")
	if err != nil {
		return StaffSelfDashboardStats{}, err
	}
	defer tx.Rollback()
	var result StaffSelfDashboardStats
	var staffID int64
	err = tx.QueryRowContext(ctx, `SELECT s.id FROM app.staff_members s
		JOIN app.users u ON u.id=s.user_id AND u.active
		JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='pegawai_tu'
		WHERE u.id=$1`, actor).Scan(&staffID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrForbidden
	}
	if err != nil {
		return result, err
	}
	err = tx.QueryRowContext(ctx, `WITH day AS (SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date AS on_date)
		SELECT day.on_date::text,
		EXISTS(SELECT 1 FROM app.staff_attendance_events WHERE staff_member_id=$1 AND staff_user_id=$2 AND on_date=day.on_date AND action='check_in'),
		EXISTS(SELECT 1 FROM app.staff_attendance_events WHERE staff_member_id=$1 AND staff_user_id=$2 AND on_date=day.on_date AND action='check_out') FROM day`, staffID, actor).
		Scan(&result.Date, &result.HasCheckIn, &result.HasCheckOut)
	return result, err
}

// TeacherAttendanceManagerDashboardStats hanya menghitung catatan dan laporan yang sudah ada.
type TeacherAttendanceManagerDashboardStats struct {
	Date                                  string
	TeachersRecorded, CheckIns, CheckOuts int64
	Excused, Sick, OutOfOffice            int64
}

// TeacherAttendanceManagerDashboard mensyaratkan pasangan role D-039 dan memakai snapshot baca-saja.
func (s *Store) TeacherAttendanceManagerDashboard(ctx context.Context, actor int64) (TeacherAttendanceManagerDashboardStats, error) {
	tx, err := s.dashboardReader(ctx, actor, "pegawai_tu")
	if err != nil {
		return TeacherAttendanceManagerDashboardStats{}, err
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, actor, "pengelola_kehadiran_guru"); err != nil {
		return TeacherAttendanceManagerDashboardStats{}, err
	}
	var result TeacherAttendanceManagerDashboardStats
	err = tx.QueryRowContext(ctx, `WITH day AS (SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date AS on_date)
		SELECT day.on_date::text,
		(SELECT count(DISTINCT teacher_id) FROM app.teacher_attendance_events WHERE on_date=day.on_date),
		(SELECT count(DISTINCT teacher_id) FROM app.teacher_attendance_events WHERE on_date=day.on_date AND action='check_in'),
		(SELECT count(DISTINCT teacher_id) FROM app.teacher_attendance_events WHERE on_date=day.on_date AND action='check_out'),
		(SELECT count(DISTINCT teacher_id) FROM app.teacher_attendance_reports WHERE active AND starts_on<=day.on_date AND ends_on>=day.on_date AND kind='izin'),
		(SELECT count(DISTINCT teacher_id) FROM app.teacher_attendance_reports WHERE active AND starts_on<=day.on_date AND ends_on>=day.on_date AND kind='sakit'),
		(SELECT count(DISTINCT teacher_id) FROM app.teacher_attendance_reports WHERE active AND starts_on<=day.on_date AND ends_on>=day.on_date AND kind='dinas_luar') FROM day`).
		Scan(&result.Date, &result.TeachersRecorded, &result.CheckIns, &result.CheckOuts, &result.Excused, &result.Sick, &result.OutOfOffice)
	return result, err
}

// StudentAttendanceOperatorDashboard memakai roster dan sumber catatan harian resmi tanpa statistik kasus.
func (s *Store) StudentAttendanceOperatorDashboard(ctx context.Context, actor int64) (SchoolMonitoringDashboardStats, error) {
	tx, err := s.dashboardReader(ctx, actor, "petugas_absensi")
	if err != nil {
		return SchoolMonitoringDashboardStats{}, err
	}
	defer tx.Rollback()
	var stats SchoolMonitoringDashboardStats
	err = tx.QueryRowContext(ctx, `SELECT
		(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text,
		EXISTS(SELECT 1 FROM app.school_years WHERE active)`).Scan(&stats.Date, &stats.HasActiveYear)
	if err != nil || !stats.HasActiveYear {
		return stats, err
	}
	err = tx.QueryRowContext(ctx, `WITH candidates AS (
			SELECT p.id,p.student_id,p.class_id FROM app.current_student_placements p
			JOIN app.school_years y ON y.id=p.school_year_id AND y.active
			JOIN app.student_placements base ON base.id=p.id AND base.starts_on IS NOT NULL
				AND base.starts_on<=$1::date
			JOIN app.students s ON s.id=p.student_id AND s.active
		), roster AS (
			SELECT c.* FROM candidates c
			WHERE (SELECT count(*) FROM candidates c2 WHERE c2.student_id=c.student_id)=1
		)
		SELECT count(DISTINCT r.student_id),count(DISTINCT a.id),
			count(DISTINCT a.id) FILTER (WHERE a.status='hadir'),
			count(DISTINCT a.id) FILTER (WHERE a.status='sakit'),
			count(DISTINCT a.id) FILTER (WHERE a.status='izin'),
			count(DISTINCT a.id) FILTER (WHERE a.status='alpa')
		FROM roster r LEFT JOIN app.student_daily_attendance a ON a.student_id=r.student_id
			AND a.placement_id=r.id AND a.class_id=r.class_id
			AND a.on_date=$1::date`, stats.Date).
		Scan(&stats.ActiveStudents, &stats.Recorded, &stats.Present, &stats.Sick, &stats.Excused, &stats.Absent)
	if err == nil {
		stats.HasAttendance = stats.Recorded > 0
	}
	return stats, err
}
