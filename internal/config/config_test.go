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
		{"custom port", map[string]string{"APP_PORT": "9095"}, true},
		{"trusted proxy cidrs", map[string]string{"APP_TRUSTED_PROXY_CIDRS": "127.0.0.1/32, ::1/128"}, true},
		{"invalid trusted proxy cidr", map[string]string{"APP_TRUSTED_PROXY_CIDRS": "not-a-cidr"}, false},
		{"invalid port", map[string]string{"APP_PORT": "0"}, false},
		{"port overflow", map[string]string{"APP_PORT": "65536"}, false},
		{"environment", map[string]string{"APP_ENV": "staging"}, false},
		{"invalid host", map[string]string{"APP_HOST": "bad host"}, false},
		{"missing csrf", map[string]string{"CSRF_AUTH_KEY": ""}, false},
		{"invalid csrf", map[string]string{"CSRF_AUTH_KEY": "secret-invalid"}, false},
		{"missing database", map[string]string{"DATABASE_URL": ""}, false},
		{"invalid secret url", map[string]string{"DATABASE_URL": "postgres://user:secret%xx@localhost/db"}, false},
		{"missing db name", map[string]string{"DATABASE_URL": "postgres://localhost/"}, false},
		{"production tls", map[string]string{"APP_ENV": "production"}, false},
		{"production verified", map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://localhost/db?sslmode=verify-full"}, true},
		{"photo storage disabled", map[string]string{"TEACHER_PHOTO_DIR": ""}, true},
		{"absolute photo storage", map[string]string{"TEACHER_PHOTO_DIR": "/private/selfies", "APP_REPOSITORY_ROOT": "/source/sim_guru"}, true},
		{"relative photo directory", map[string]string{"TEACHER_PHOTO_DIR": "private/selfies", "APP_REPOSITORY_ROOT": "/source/sim_guru"}, false},
		{"missing repository root", map[string]string{"TEACHER_PHOTO_DIR": "/private/selfies"}, false},
		{"relative repository root", map[string]string{"TEACHER_PHOTO_DIR": "/private/selfies", "APP_REPOSITORY_ROOT": "sim_guru"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://user:secret@localhost/db", "CSRF_AUTH_KEY": strings.Repeat("a", 64)}
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
			if tc.name == "custom port" && cfg.Address != "127.0.0.1:9095" {
				t.Fatalf("custom port ignored: %s", cfg.Address)
			}
			if tc.name == "defaults" && cfg.Address != "127.0.0.1:18743" {
				t.Fatalf("unexpected address %s", cfg.Address)
			}
			if tc.name == "trusted proxy cidrs" && (len(cfg.TrustedProxyCIDRs) != 2 || cfg.TrustedProxyCIDRs[0].String() != "127.0.0.1/32" || cfg.TrustedProxyCIDRs[1].String() != "::1/128") {
				t.Fatal("CIDR proxy tepercaya tidak diparsing dengan benar")
			}
			if tc.name == "absolute photo storage" && (cfg.TeacherPhotoDirectory != tc.values["TEACHER_PHOTO_DIR"] || cfg.RepositoryRoot != tc.values["APP_REPOSITORY_ROOT"]) {
				t.Fatal("konfigurasi storage foto tidak diteruskan")
			}
		})
	}
}
