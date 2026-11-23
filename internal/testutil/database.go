//go:build integration

// Package testutil supplies isolated fixtures only for explicitly selected test databases.
package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/database"
	"github.com/fasih-kode/sim_guru/migrations"
	"github.com/joho/godotenv"
)

func Database(t *testing.T, path string) *sql.DB {
	t.Helper()
	if err := godotenv.Load(path); err != nil && !os.IsNotExist(err) {
		t.Fatal("cannot load test configuration")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || dsn == "" || !strings.HasSuffix(u.Path, "_test") || dsn == os.Getenv("DATABASE_URL") {
		t.Fatal("separate TEST_DATABASE_URL ending _test required")
	}
	db, err := database.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var actual string
	var privileged bool
	if err = db.QueryRow(`SELECT current_database(),rolsuper OR rolcreatedb OR rolcreaterole FROM pg_roles WHERE rolname=current_user`).Scan(&actual, &privileged); err != nil || privileged || !strings.HasSuffix(actual, "_test") {
		t.Fatal("unsafe test database")
	}
	metadataDB := db
	if actual == "sim_guru_rd02_s06_20261123_test" {
		metadataDB = FixtureDatabase(t)
	}
	provider, err := migrations.Provider(metadataDB)
	if err != nil {
		t.Fatal(err)
	}
	current, target, err := provider.GetVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current != target {
		t.Fatal("test database migrations must be applied separately before tests")
	}
	return db
}

// FixtureDatabase hanya untuk seed/metadata pada database RD-02 terisolasi.
// Koneksi ini tidak boleh diberikan kepada service yang sedang diuji.
func FixtureDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(context.Background(), "postgres:///sim_guru_rd02_s06_20261123_test?host=/home/fasih/Projects/sim_guru/.local/postgres/socket&port=55436")
	if err != nil {
		t.Fatal("koneksi fixture tidak tersedia")
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`SET ROLE sim_guru_ddl_owner`); err != nil {
		t.Fatal("role fixture tidak tersedia")
	}
	var name, role string
	var oid int64
	if err = db.QueryRow(`SELECT current_database(),current_user,(SELECT oid FROM pg_database WHERE datname=current_database())`).Scan(&name, &role, &oid); err != nil || name != "sim_guru_rd02_s06_20261123_test" || role != "sim_guru_ddl_owner" || oid != 84604 {
		t.Fatal("identitas fixture tidak aman")
	}
	return db
}
func Random(t *testing.T) string {
	t.Helper()
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// ActiveCalendar menyiapkan kalender privat untuk satu tes dan mengembalikan
// tahun aktif sebelumnya saat cleanup, tanpa mengandalkan fixture paket lain.
func ActiveCalendar(t *testing.T, db *sql.DB, suffix string) (int64, string) {
	t.Helper()
	var previous sql.NullInt64
	err := db.QueryRow(`SELECT id FROM app.school_years WHERE active`).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		t.Fatal("cannot inspect active test school year")
	}
	var yearID, semesterID int64
	t.Cleanup(func() {
		if semesterID > 0 {
			_, _ = db.Exec(`DELETE FROM app.semester_calendars WHERE semester_id=$1`, semesterID)
			_, _ = db.Exec(`DELETE FROM app.semesters WHERE id=$1`, semesterID)
		}
		if yearID > 0 {
			_, _ = db.Exec(`UPDATE app.school_years SET active=false WHERE id=$1`, yearID)
			_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		}
		if previous.Valid {
			_, _ = db.Exec(`UPDATE app.school_years SET active=true WHERE id=$1`, previous.Int64)
		}
	})
	if previous.Valid {
		if _, err = db.Exec(`UPDATE app.school_years SET active=false WHERE id=$1`, previous.Int64); err != nil {
			t.Fatal("cannot isolate active test year")
		}
	}
	if err = db.QueryRow(`INSERT INTO app.school_years(name,active) VALUES($1,true) RETURNING id`, "Panduan test "+suffix).Scan(&yearID); err != nil {
		t.Fatal("cannot create isolated active test year")
	}
	if err = db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal("cannot create isolated test semester")
	}
	eventDate := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -10).Format("2006-01-02")
	starts := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -30).Format("2006-01-02")
	ends := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, 30).Format("2006-01-02")
	if _, err = db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,$2,$3,'[1,2,3,4,5,6]'::jsonb)`, semesterID, starts, ends); err != nil {
		t.Fatal("cannot create isolated test calendar")
	}
	return yearID, eventDate
}

func User(t *testing.T, db *sql.DB, roles ...string) (auth.User, string) {
	t.Helper()
	password := Random(t)
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	u := auth.User{Username: "test_" + Random(t), DisplayName: "Fixture Pengujian", Active: true, Version: 1, Roles: roles}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = tx.QueryRow(`INSERT INTO app.users(username,display_name,password_hash) VALUES($1,$2,$3) RETURNING id`, u.Username, u.DisplayName, hash).Scan(&u.ID); err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		if _, err = tx.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,$2)`, u.ID, role); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return u, password
}
