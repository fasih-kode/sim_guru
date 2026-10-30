package academic

import (
	"context"
	"database/sql"
)

// CurriculumDashboardStats merangkum periode akademik dan status jadwal resmi.
type CurriculumDashboardStats struct {
	ActiveYear            string
	HasActiveYear         bool
	ActiveSemester        int
	SemesterYear          string
	HasActiveSemester     bool
	ActiveClasses         int64
	HasPublishedTimetable bool
}

// CurriculumDashboard hanya membaca sumber kalender, kelas, dan revisi jadwal resmi.
func (s *Store) CurriculumDashboard(ctx context.Context, actor int64) (CurriculumDashboardStats, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return CurriculumDashboardStats{}, err
	}
	defer tx.Rollback()

	var stats CurriculumDashboardStats
	var yearName, semesterYear sql.NullString
	var semesterNumber sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT
			(SELECT name FROM app.school_years WHERE active),
			(SELECT y.name FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id WHERE sm.active AND y.active),
			(SELECT sm.number FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id WHERE sm.active AND y.active),
			(SELECT count(*) FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id WHERE y.active),
			EXISTS (
				SELECT 1
				FROM app.weekly_timetable_revisions r
				JOIN app.semesters sm ON sm.id=r.semester_id AND sm.active
				JOIN app.school_years y ON y.id=sm.school_year_id AND y.active
				WHERE r.status='published'
				  AND r.effective_from <= (now() AT TIME ZONE 'Asia/Jakarta')::date
			)`,
	).Scan(&yearName, &semesterYear, &semesterNumber, &stats.ActiveClasses, &stats.HasPublishedTimetable)
	if err != nil {
		return CurriculumDashboardStats{}, err
	}
	if yearName.Valid {
		stats.ActiveYear = yearName.String
		stats.HasActiveYear = true
	}
	if semesterYear.Valid && semesterNumber.Valid {
		stats.SemesterYear = semesterYear.String
		stats.ActiveSemester = int(semesterNumber.Int64)
		stats.HasActiveSemester = true
	}
	return stats, nil
}
