//go:build integration

// Package testutil supplies isolated fixtures only for explicitly selected test databases.
package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/database"
	"github.com/fasih-kode/sim_guru/migrations"
	"github.com/joho/godotenv"
)

func Database(t *testing.T, path string) *sql.DB {
	return databaseForTarget(t, path, "", 0)
}

// DatabaseForTarget menambahkan verifikasi nama dan OID sebelum pemeriksaan
// migration atau akses fixture khusus.
func DatabaseForTarget(t *testing.T, path, expectedName string, expectedOID int64) *sql.DB {
	if expectedName == "" || expectedOID <= 0 {
		t.Fatal("target database test tidak valid")
	}
	return databaseForTarget(t, path, expectedName, expectedOID)
}

func validateDatabaseTarget(actual string, actualOID int64, expectedName string, expectedOID int64) error {
	if actual != expectedName || actualOID != expectedOID {
		return fmt.Errorf("database test target tidak cocok: nama=%q OID=%d; butuh nama=%q OID=%d", actual, actualOID, expectedName, expectedOID)
	}
	return nil
}

func databaseForTarget(t *testing.T, path, expectedName string, expectedOID int64) *sql.DB {
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
	var actualOID int64
	var privileged bool
	if err = db.QueryRow(`SELECT current_database(),(SELECT oid FROM pg_database WHERE datname=current_database()),rolsuper OR rolcreatedb OR rolcreaterole FROM pg_roles WHERE rolname=current_user`).Scan(&actual, &actualOID, &privileged); err != nil || privileged || !strings.HasSuffix(actual, "_test") {
		t.Fatal("unsafe test database")
	}
	if expectedName != "" {
		if err = validateDatabaseTarget(actual, actualOID, expectedName, expectedOID); err != nil {
			t.Fatal(err)
		}
	}
	metadataDB := db
	if os.Getenv("TEST_FIXTURE_DATABASE_URL") != "" {
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

// FixtureDatabase hanya untuk seed/metadata pada database test terisolasi.
// Koneksi ini tidak boleh diberikan kepada service yang sedang diuji.
func FixtureDatabase(t *testing.T) *sql.DB {
	t.Helper()
	fixtureDSN := os.Getenv("TEST_FIXTURE_DATABASE_URL")
	runtimeDSN := os.Getenv("TEST_DATABASE_URL")
	role := os.Getenv("TEST_FIXTURE_ROLE")
	expectedOID, err := strconv.ParseInt(os.Getenv("TEST_FIXTURE_DATABASE_OID"), 10, 64)
	fixtureURL, e1 := url.Parse(fixtureDSN)
	runtimeURL, e2 := url.Parse(runtimeDSN)
	if err != nil || expectedOID <= 0 || role == "" || fixtureDSN == "" || e1 != nil || e2 != nil || fixtureURL.Path != runtimeURL.Path || !strings.HasSuffix(fixtureURL.Path, "_test") || fixtureDSN == runtimeDSN {
		t.Fatal("konfigurasi fixture terisolasi tidak valid")
	}
	db, err := database.Open(context.Background(), fixtureDSN)
	if err != nil {
		t.Fatal("koneksi fixture tidak tersedia")
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`SELECT set_config('role',$1,false)`, role); err != nil {
		t.Fatal("role fixture tidak tersedia")
	}
	var name, fixturePort string
	var oid int64
	var actualRole string
	var owner string
	var rolePrivileged bool
	var fixtureStart time.Time
	if err = db.QueryRow(`SELECT current_database(),current_user,(SELECT oid FROM pg_database WHERE datname=current_database()),current_setting('port'),pg_postmaster_start_time(),(SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()),(SELECT rolsuper OR rolcreatedb OR rolcreaterole FROM pg_roles WHERE rolname=current_user)`).Scan(&name, &actualRole, &oid, &fixturePort, &fixtureStart, &owner, &rolePrivileged); err != nil || name != strings.TrimPrefix(fixtureURL.Path, "/") || actualRole != role || actualRole != owner || rolePrivileged || oid != expectedOID {
		t.Fatal("identitas fixture tidak aman")
	}
	runtimeDB, err := database.Open(context.Background(), runtimeDSN)
	if err != nil {
		t.Fatal("koneksi runtime tidak tersedia")
	}
	defer runtimeDB.Close()
	var runtimeName, runtimeRole, runtimePort string
	var runtimeOID int64
	var runtimeStart time.Time
	if err = runtimeDB.QueryRow(`SELECT current_database(),current_user,(SELECT oid FROM pg_database WHERE datname=current_database()),current_setting('port'),pg_postmaster_start_time()`).Scan(&runtimeName, &runtimeRole, &runtimeOID, &runtimePort, &runtimeStart); err != nil || runtimeName != name || runtimeOID != oid || runtimeRole != "sim_guru_app" || runtimeRole == actualRole || runtimePort != fixturePort || !runtimeStart.Equal(fixtureStart) {
		t.Fatal("identitas runtime dan fixture tidak aman")
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
