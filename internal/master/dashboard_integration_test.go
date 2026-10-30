//go:build integration

package master_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAdminDashboardStatsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	store := master.Store{DB: db}
	admin, _ := testutil.User(t, db, "admin")
	studentAffairs, _ := testutil.User(t, db, "kesiswaan")
	ctx := context.Background()

	got, err := store.AdminDashboard(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	var students, teachers, staff, classes int64
	var year sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM app.students s WHERE s.active AND NOT EXISTS (
				SELECT 1 FROM app.outstanding_student_school_exits e
				WHERE e.student_id=s.id AND e.effective_date <= (now() AT TIME ZONE 'Asia/Jakarta')::date)),
			(SELECT count(*) FROM app.teachers WHERE active),
			(SELECT count(*) FROM app.staff_members WHERE active),
			(SELECT name FROM app.school_years WHERE active),
			(SELECT count(*) FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id WHERE y.active)`).Scan(&students, &teachers, &staff, &year, &classes)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveStudents != students || got.ActiveTeachers != teachers || got.ActiveStaff != staff || got.ActiveClasses != classes || got.HasActiveYear != year.Valid || (year.Valid && got.ActiveYear != year.String) {
		t.Fatalf("dashboard stats differ from database aggregate: got=%+v", got)
	}
	if _, err := store.AdminDashboard(ctx, studentAffairs.ID); !errors.Is(err, master.ErrForbidden) {
		t.Fatalf("non-admin dashboard read must be forbidden, got %v", err)
	}
}
