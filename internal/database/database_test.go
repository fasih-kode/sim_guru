package database

import (
	"context"
	"strings"
	"testing"
)

func TestOpenRedactsInvalidCredentials(t *testing.T) {
	db, err := Open(context.Background(), "postgres://user:supersecret%xx@localhost/db")
	if db != nil {
		db.Close()
		t.Fatal("unexpected connection")
	}
	if err == nil || strings.Contains(err.Error(), "supersecret") {
		t.Fatalf("unsafe error: %v", err)
	}
}
