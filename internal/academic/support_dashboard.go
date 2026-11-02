package academic

import (
	"context"
)

// CaseDashboardStats hanya memuat hitungan kasus dalam tahap dan cakupan aktor.
type CaseDashboardStats struct {
	Cases, OpenCases, CompletedCases int64
}

// CounselorDashboard hanya menghitung kasus yang sedang ditangani aktor pada tahap BK.
func (s *Store) CounselorDashboard(ctx context.Context, actor int64) (CaseDashboardStats, error) {
	return s.caseDashboard(ctx, actor, "bk", "guru_bk")
}

// StudentAffairsDashboard merangkum roster, absensi hari ini, dan kasus tahap Kesiswaan milik aktor.
func (s *Store) StudentAffairsDashboard(ctx context.Context, actor int64) (SchoolMonitoringDashboardStats, error) {
	return s.schoolMonitoringDashboard(ctx, actor, "kesiswaan", true)
}

// HeadmasterDashboard memberi ringkasan baca-saja seluruh sekolah tanpa membaca catatan konseling.
func (s *Store) HeadmasterDashboard(ctx context.Context, actor int64) (SchoolMonitoringDashboardStats, error) {
	return s.schoolMonitoringDashboard(ctx, actor, "kepala_sekolah", false)
}

// SchoolMonitoringDashboardStats memisahkan ketersediaan roster dari nilai hitungannya.
type SchoolMonitoringDashboardStats struct {
	Date                           string
	HasActiveYear, HasAttendance   bool
	ActiveStudents, Recorded       int64
	Present, Sick, Excused, Absent int64
	OpenCases                      int64
}

func (s *Store) caseDashboard(ctx context.Context, actor int64, role, stage string) (CaseDashboardStats, error) {
	tx, err := s.dashboardReader(ctx, actor, role)
	if err != nil {
		return CaseDashboardStats{}, err
	}
	defer tx.Rollback()
	var stats CaseDashboardStats
	err = tx.QueryRowContext(ctx, `SELECT count(*),
		count(*) FILTER (WHERE status='perlu_tindak_lanjut'),
		count(*) FILTER (WHERE status='selesai')
		FROM app.student_cases WHERE stage=$1 AND responsible_user_id=$2`, stage, actor).
		Scan(&stats.Cases, &stats.OpenCases, &stats.CompletedCases)
	return stats, err
}

func (s *Store) schoolMonitoringDashboard(ctx context.Context, actor int64, role string, casesOwned bool) (SchoolMonitoringDashboardStats, error) {
	tx, err := s.dashboardReader(ctx, actor, role)
	if err != nil {
		return SchoolMonitoringDashboardStats{}, err
	}
	defer tx.Rollback()
	var stats SchoolMonitoringDashboardStats
	err = tx.QueryRowContext(ctx, `SELECT
		(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text,
		EXISTS(SELECT 1 FROM app.school_years WHERE active)`).Scan(&stats.Date, &stats.HasActiveYear)
	if err != nil {
		return stats, err
	}
	if casesOwned {
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.student_cases WHERE stage='kesiswaan' AND responsible_user_id=$1 AND status='perlu_tindak_lanjut'`, actor).Scan(&stats.OpenCases)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.student_cases WHERE status='perlu_tindak_lanjut'`).Scan(&stats.OpenCases)
	}
	if err != nil || !stats.HasActiveYear {
		return stats, err
	}
	err = tx.QueryRowContext(ctx, `WITH candidates AS (
			SELECT p.id,p.student_id,p.class_id
			FROM app.current_student_placements p
			JOIN app.school_years y ON y.id=p.school_year_id AND y.active
			JOIN app.student_placements base ON base.id=p.id AND base.starts_on IS NOT NULL
				AND base.starts_on<=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date
			JOIN app.students s ON s.id=p.student_id AND s.active
		), roster AS (
			SELECT c.* FROM candidates c
			WHERE (SELECT count(*) FROM candidates c2 WHERE c2.student_id=c.student_id)=1
		)
		SELECT count(DISTINCT r.student_id), count(DISTINCT a.id),
			count(DISTINCT a.id) FILTER (WHERE a.status='hadir'),
			count(DISTINCT a.id) FILTER (WHERE a.status='sakit'),
			count(DISTINCT a.id) FILTER (WHERE a.status='izin'),
			count(DISTINCT a.id) FILTER (WHERE a.status='alpa')
		FROM roster r
		LEFT JOIN app.student_daily_attendance a ON a.student_id=r.student_id
			AND a.placement_id=r.id AND a.class_id=r.class_id
			AND a.on_date=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date`,
	).Scan(&stats.ActiveStudents, &stats.Recorded, &stats.Present, &stats.Sick, &stats.Excused, &stats.Absent)
	if err != nil {
		return SchoolMonitoringDashboardStats{}, err
	}
	stats.HasAttendance = stats.Recorded > 0
	return stats, nil
}
