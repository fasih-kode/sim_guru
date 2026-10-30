//go:build integration

package migrations

import (
	"context"
	"github.com/fasih-kode/sim_guru/internal/database"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestMigrationsIntegration(t *testing.T) {
	if err := godotenv.Load("../.env"); err != nil && !os.IsNotExist(err) {
		t.Fatal("cannot read local test configuration")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || dsn == "" || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("TEST_DATABASE_URL must explicitly target a database ending in _test")
	}
	if dsn == os.Getenv("DATABASE_URL") {
		t.Fatal("test and application database must differ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var actual, user string
	var superuser, createDB, createRole bool
	err = db.QueryRowContext(ctx, "SELECT current_database(), current_user, rolsuper, rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname=current_user").Scan(&actual, &user, &superuser, &createDB, &createRole)
	if err != nil {
		t.Fatal("cannot inspect test role")
	}
	if !strings.HasSuffix(actual, "_test") || superuser || createDB || createRole {
		t.Fatal("unsafe test database or privileges")
	}
	p, err := Provider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatalf("migration up failed: %v", err)
	}
	results, err := p.Up(ctx)
	if err != nil || len(results) != 0 {
		t.Fatal("migration up is not idempotent")
	}
	current, target, err := p.GetVersions(ctx)
	if err != nil || current != 27 || target != 27 {
		t.Fatal("unexpected migration version")
	}
	var exists bool
	if err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='app')").Scan(&exists); err != nil || !exists {
		t.Fatal("missing application schema")
	}
	var roleCount int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM app.roles WHERE code IN('pegawai_tu','pengelola_kehadiran_guru')`).Scan(&roleCount); err != nil || roleCount != 2 {
		t.Fatal("missing TU attendance roles")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.teacher_attendance_correction_requests') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing teacher attendance correction request table")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.teacher_attendance_reports') IS NOT NULL AND to_regclass('app.teacher_attendance_report_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing teacher attendance report tables")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.staff_members') IS NOT NULL AND to_regclass('app.staff_member_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing staff master tables")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.staff_attendance_events') IS NOT NULL AND to_regclass('app.staff_attendance_photos') IS NOT NULL AND to_regclass('app.staff_attendance_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing staff attendance tables")
	}
	var uniqueActions, photoPair, staffForeignKey bool
	if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='app' AND t.relname='staff_attendance_events' AND c.contype='u' AND pg_get_constraintdef(c.oid) LIKE 'UNIQUE (staff_member_id, on_date, action)%'), EXISTS(SELECT 1 FROM pg_constraint WHERE conname='staff_attendance_event_photo_fk' AND contype='f'), EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid WHERE t.relname='staff_attendance_events' AND c.contype='f' AND pg_get_constraintdef(c.oid) LIKE '%staff_member_id%staff_members%')`).Scan(&uniqueActions, &photoPair, &staffForeignKey); err != nil || !uniqueActions || !photoPair || !staffForeignKey {
		t.Fatalf("missing staff event uniqueness or relationships (unique=%t photo_fk=%t staff_fk=%t)", uniqueActions, photoPair, staffForeignKey)
	}
	var overlap bool
	if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='teacher_attendance_reports_no_overlap' AND contype='x')`).Scan(&overlap); err != nil || !overlap {
		t.Fatal("missing report overlap exclusion constraint")
	}
	// Verify transaction rollback using a deliberately failing migration. No existing data is dropped.
	broken := fstest.MapFS{"00002_probe.sql": {Data: []byte("-- +goose Up\nCREATE TABLE app.foundation_rollback_probe(id integer);\nSELECT 1/0;\n-- +goose Down\nSELECT 1;\n")}}
	probe, err := goose.NewProvider(goose.DialectPostgres, db, broken, goose.WithDisableVersioning(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = probe.Up(ctx); err == nil {
		t.Fatal("expected failing migration")
	}
	if err = db.QueryRowContext(ctx, "SELECT to_regclass('app.foundation_rollback_probe') IS NULL").Scan(&exists); err != nil || !exists {
		t.Fatal("failed migration left partial state")
	}
}
