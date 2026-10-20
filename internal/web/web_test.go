package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, path, method, hx, origin string
		failed                         bool
		status                         int
		contains, excludes             string
	}{
		{name: "full page", path: "/", method: "GET", status: 200, contains: "<!doctype html>"},
		{name: "htmx fragment", path: "/", method: "GET", hx: "true", status: 200, contains: "id=\"overview\"", excludes: "<!doctype html>"},
		{name: "liveness independent", path: "/health/live", method: "GET", failed: true, status: 200, contains: "ok"},
		{name: "ready", path: "/health/ready", method: "GET", status: 200, contains: "ready"},
		{name: "unready redacts errors", path: "/health/ready", method: "GET", failed: true, status: 503, contains: "belum siap", excludes: "secret"},
		{name: "unknown", path: "/students", method: "GET", status: 404},
		{name: "no mutation", path: "/", method: "POST", status: 405},
		{name: "cross origin blocked", path: "/", method: "POST", origin: "https://evil.example", status: 403},
		{name: "css", path: "/static/app.css", method: "GET", status: 200, contains: "tailwindcss"},
		{name: "htmx", path: "/static/htmx.min.js", method: "GET", status: 200, contains: "htmx"},
		{name: "traversal", path: "/static/../../.env", method: "GET", status: 307, excludes: "DATABASE_URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := New(func(context.Context) error {
				if tc.failed {
					return errors.New("secret")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("HX-Request", tc.hx)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.status, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.contains) {
				t.Fatalf("missing %q", tc.contains)
			}
			if tc.excludes != "" && strings.Contains(w.Body.String(), tc.excludes) {
				t.Fatalf("unexpected %q", tc.excludes)
			}
			if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing security headers")
			}
			if tc.path == "/" && tc.method == "GET" && w.Header().Get("Vary") != "HX-Request" {
				t.Fatal("missing Vary")
			}
		})
	}
}
