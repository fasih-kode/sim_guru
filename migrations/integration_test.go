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
		t.Fatal("migration up failed")
	}
	results, err := p.Up(ctx)
	if err != nil || len(results) != 0 {
		t.Fatal("migration up is not idempotent")
	}
	current, target, err := p.GetVersions(ctx)
	if err != nil || current != 1 || target != 1 {
		t.Fatal("unexpected migration version")
	}
	var exists bool
	if err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='app')").Scan(&exists); err != nil || !exists {
		t.Fatal("missing application schema")
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
