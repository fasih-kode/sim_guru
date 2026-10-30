package master

import (
	"context"
	"database/sql"
)

// AdminDashboardStats berisi ringkasan baca-saja untuk Beranda Admin.
type AdminDashboardStats struct {
	ActiveStudents int64
	ActiveTeachers int64
	ActiveStaff    int64
	ActiveYear     string
	HasActiveYear  bool
	ActiveClasses  int64
}

// AdminDashboard membaca jumlah identitas aktif dan kelas pada tahun aktif.
func (s *Store) AdminDashboard(ctx context.Context, actor int64) (AdminDashboardStats, error) {
	var stats AdminDashboardStats
	if err := authorize(ctx, s.DB, actor, Staff, true); err != nil {
		return stats, err
	}
	var year sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM app.students s
			 WHERE s.active AND NOT EXISTS (
				 SELECT 1 FROM app.outstanding_student_school_exits e
				 WHERE e.student_id=s.id
				   AND e.effective_date <= (now() AT TIME ZONE 'Asia/Jakarta')::date
			 )),
			(SELECT count(*) FROM app.teachers WHERE active),
			(SELECT count(*) FROM app.staff_members WHERE active),
			(SELECT name FROM app.school_years WHERE active),
			(SELECT count(*) FROM app.classes c
			 JOIN app.school_years y ON y.id=c.school_year_id
			 WHERE y.active)`).Scan(
		&stats.ActiveStudents,
		&stats.ActiveTeachers,
		&stats.ActiveStaff,
		&year,
		&stats.ActiveClasses,
	)
	if err != nil {
		return AdminDashboardStats{}, err
	}
	if year.Valid {
		stats.ActiveYear = year.String
		stats.HasActiveYear = true
	}
	return stats, nil
}
