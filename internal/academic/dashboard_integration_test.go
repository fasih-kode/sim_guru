//go:build integration

package academic_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestCurriculumDashboardStatsEmptyStatesAndAuthorization(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	store := academic.Store{DB: db}
	curriculum, _ := testutil.User(t, db, "kurikulum", "guru_mapel")
	admin, _ := testutil.User(t, db, "admin")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	ctx := context.Background()

	got, err := store.CurriculumDashboard(ctx, curriculum.ID)
	if err != nil {
		t.Fatal(err)
	}
	var year sql.NullString
	var semesterYear sql.NullString
	var semester sql.NullInt64
	var classes int64
	var published bool
	err = db.QueryRowContext(ctx, `
		SELECT
			(SELECT name FROM app.school_years WHERE active),
			(SELECT y.name FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id WHERE sm.active AND y.active),
			(SELECT sm.number FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id WHERE sm.active AND y.active),
			(SELECT count(*) FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id WHERE y.active),
		EXISTS (SELECT 1 FROM app.weekly_timetable_revisions r JOIN app.semesters sm ON sm.id=r.semester_id AND sm.active JOIN app.school_years y ON y.id=sm.school_year_id AND y.active WHERE r.status='published' AND r.effective_from <= (now() AT TIME ZONE 'Asia/Jakarta')::date)`,
	).Scan(&year, &semesterYear, &semester, &classes, &published)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasActiveYear != year.Valid || (year.Valid && got.ActiveYear != year.String) || got.HasActiveSemester != (semester.Valid && semesterYear.Valid) || (got.HasActiveSemester && (got.ActiveSemester != int(semester.Int64) || got.SemesterYear != semesterYear.String)) || got.ActiveClasses != classes || got.HasPublishedTimetable != published {
		t.Fatalf("dashboard tidak sesuai sumber akademik resmi: got=%+v", got)
	}
	if _, err = store.CurriculumDashboard(ctx, admin.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("Admin tanpa role Kurikulum dapat membaca statistik Kurikulum: %v", err)
	}
	if _, err = store.CurriculumDashboard(ctx, teacher.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("Guru Mapel tanpa role Kurikulum dapat membaca statistik Kurikulum: %v", err)
	}

	var oldYear, oldSemester sql.NullInt64
	if err = db.QueryRowContext(ctx, `SELECT id FROM app.school_years WHERE active`).Scan(&oldYear); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT id FROM app.semesters WHERE active`).Scan(&oldSemester); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	var fixtureYearID, fixtureSemesterID int64
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `UPDATE app.semesters SET active=false WHERE active`)
		_, _ = db.ExecContext(context.Background(), `UPDATE app.school_years SET active=false WHERE active`)
		if fixtureSemesterID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.semesters WHERE id=$1`, fixtureSemesterID)
		}
		if fixtureYearID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.school_years WHERE id=$1`, fixtureYearID)
		}
		if oldYear.Valid {
			_, _ = db.ExecContext(context.Background(), `UPDATE app.school_years SET active=true WHERE id=$1`, oldYear.Int64)
		}
		if oldSemester.Valid {
			_, _ = db.ExecContext(context.Background(), `UPDATE app.semesters SET active=true WHERE id=$1`, oldSemester.Int64)
		}
	})

	if _, err = db.ExecContext(ctx, `UPDATE app.semesters SET active=false WHERE active`); err != nil {
		t.Fatal(err)
	}
	got, err = store.CurriculumDashboard(ctx, curriculum.ID)
	if err != nil || !got.HasActiveYear && got.HasActiveSemester || got.HasActiveSemester || got.HasPublishedTimetable {
		t.Fatalf("kondisi tanpa semester aktif tidak aman: stats=%+v err=%v", got, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE app.school_years SET active=false WHERE active`); err != nil {
		t.Fatal(err)
	}
	got, err = store.CurriculumDashboard(ctx, curriculum.ID)
	if err != nil || got.HasActiveYear || got.HasActiveSemester || got.ActiveClasses != 0 || got.HasPublishedTimetable {
		t.Fatalf("kondisi tanpa tahun ajaran aktif tidak aman: stats=%+v err=%v", got, err)
	}

	fixtureName := "Dashboard " + testutil.Random(t)[:12]
	if err = db.QueryRowContext(ctx, `INSERT INTO app.school_years(name,active) VALUES($1,true) RETURNING id`, fixtureName).Scan(&fixtureYearID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `INSERT INTO app.semesters(school_year_id,number,active) VALUES($1,1,true) RETURNING id`, fixtureYearID).Scan(&fixtureSemesterID); err != nil {
		t.Fatal(err)
	}
	got, err = store.CurriculumDashboard(ctx, curriculum.ID)
	if err != nil || !got.HasActiveYear || got.ActiveYear != fixtureName || !got.HasActiveSemester || got.SemesterYear != fixtureName || got.ActiveSemester != 1 || got.ActiveClasses != 0 || got.HasPublishedTimetable {
		t.Fatalf("periode aktif tanpa kelas/jadwal terbit tidak ditampilkan sebagai kondisi kosong: stats=%+v err=%v", got, err)
	}
}
