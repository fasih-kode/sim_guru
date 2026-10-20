package config

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]string
		valid  bool
	}{
		{"defaults", map[string]string{}, true},
		{"invalid port", map[string]string{"APP_PORT": "0"}, false},
		{"port overflow", map[string]string{"APP_PORT": "65536"}, false},
		{"environment", map[string]string{"APP_ENV": "staging"}, false},
		{"invalid host", map[string]string{"APP_HOST": "bad host"}, false},
		{"missing database", map[string]string{"DATABASE_URL": ""}, false},
		{"invalid secret url", map[string]string{"DATABASE_URL": "postgres://user:secret%xx@localhost/db"}, false},
		{"missing db name", map[string]string{"DATABASE_URL": "postgres://localhost/"}, false},
		{"production tls", map[string]string{"APP_ENV": "production"}, false},
		{"production verified", map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://localhost/db?sslmode=verify-full"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://user:secret@localhost/db"}
			for k, v := range tc.values {
				values[k] = v
			}
			cfg, err := Parse(func(k string) string { return values[k] })
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("error exposed credential")
			}
			if tc.name == "defaults" && cfg.Address != "127.0.0.1:8080" {
				t.Fatalf("unexpected address %s", cfg.Address)
			}
		})
	}
}
