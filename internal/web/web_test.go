package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, path, method, hx, origin string
		failed                         bool
		status                         int
		contains, excludes             string
	}{
		{name: "full page", path: "/login", method: "GET", status: 200, contains: "<!doctype html>"},
		{name: "htmx session expired", path: "/", method: "GET", hx: "true", status: 401, excludes: "<!doctype html>"},
		{name: "liveness independent", path: "/health/live", method: "GET", failed: true, status: 200, contains: "ok"},
		{name: "ready", path: "/health/ready", method: "GET", status: 200, contains: "ready"},
		{name: "unready redacts errors", path: "/health/ready", method: "GET", failed: true, status: 503, contains: "belum siap", excludes: "secret"},
		{name: "unknown", path: "/students", method: "GET", status: 404},
		{name: "missing csrf", path: "/login", method: "POST", status: 403},
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
			}, Options{Accounts: &auth.Store{}, CSRFKey: []byte(strings.Repeat("a", 32))})
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
			if tc.hx == "true" && w.Header().Get("HX-Redirect") != "/login" {
				t.Fatal("missing login redirect for expired htmx request")
			}
		})
	}
}

func TestLoginLimiter(t *testing.T) {
	var limiter loginLimiter
	now := time.Now()
	for i := 0; i < 10; i++ {
		if !limiter.allow("127.0.0.1", "same-user", now) {
			t.Fatal("limited too soon")
		}
	}
	if limiter.allow("another-ip", "same-user", now) {
		t.Fatal("username limit bypassed")
	}
	if !limiter.allow("127.0.0.1", "same-user", now.Add(16*time.Minute)) {
		t.Fatal("limit did not expire")
	}
	for i := 0; i < 29; i++ {
		if !limiter.allow("127.0.0.2", fmt.Sprintf("user-%d", i), now) {
			t.Fatal("IP limited too soon")
		}
	}
	if !limiter.allow("127.0.0.2", "last-user", now) {
		t.Fatal("IP limit early")
	}
	if limiter.allow("127.0.0.2", "other-user", now) {
		t.Fatal("IP limit bypassed")
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	for _, secure := range []bool{false, true} {
		app := application{options: Options{SecureCookies: secure}}
		w := httptest.NewRecorder()
		app.setSession(w, strings.Repeat("a", 64))
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("missing session cookie")
		}
		c := cookies[0]
		if !c.HttpOnly || c.Secure != secure || c.Domain != "" || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge != int(auth.SessionLifetime.Seconds()) {
			t.Fatal("unsafe cookie attributes")
		}
		w = httptest.NewRecorder()
		app.clearSession(w)
		c = w.Result().Cookies()[0]
		if c.MaxAge != -1 || c.Value != "" || c.Secure != secure {
			t.Fatal("cookie was not expired correctly")
		}
	}
}
