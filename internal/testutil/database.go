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
	provider, err := migrations.Provider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(context.Background()); err != nil {
		t.Fatal(err)
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
