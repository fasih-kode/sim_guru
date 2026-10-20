package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestValidation(t *testing.T) {
	b := make([]byte, 24)
	rand.Read(b)
	password := hex.EncodeToString(b)
	for _, tc := range []struct {
		name  string
		input Input
		valid bool
	}{
		{"normalizes", Input{Username: "  Guru.A ", DisplayName: " Guru A ", Password: password, Roles: []string{"kurikulum", "guru_mapel"}}, true},
		{"injection username", Input{Username: "' OR 1=1--", DisplayName: "A", Password: password, Roles: []string{"admin"}}, false},
		{"empty role", Input{Username: "guru", DisplayName: "A", Password: password}, false},
		{"invented role", Input{Username: "guru", DisplayName: "A", Password: password, Roles: []string{"pengganti"}}, false},
		{"duplicate role", Input{Username: "guru", DisplayName: "A", Password: password, Roles: []string{"admin", "admin"}}, false},
		{"short password", Input{Username: "guru", DisplayName: "A", Password: "short", Roles: []string{"admin"}}, false},
		{"byte limit", Input{Username: "guru", DisplayName: "A", Password: strings.Repeat("字", 25), Roles: []string{"admin"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.input.Validate(true)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.name == "normalizes" && tc.input.Username != "guru.a" {
				t.Fatal("username not normalized")
			}
			if err != nil {
				var validation ValidationError
				if !errors.As(err, &validation) {
					t.Fatal("expected validation error")
				}
			}
		})
	}
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if hash == password || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		t.Fatal("password hash invalid")
	}
	if cost, _ := bcrypt.Cost([]byte(hash)); cost != 12 {
		t.Fatal("incorrect bcrypt cost")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password+"x")) == nil {
		t.Fatal("wrong password matched")
	}
}
