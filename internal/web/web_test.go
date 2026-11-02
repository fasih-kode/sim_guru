package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
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
		{name: "bobot penilaian perlu sesi", path: "/akademik/bobot-penilaian?semester_id=1", method: "GET", status: http.StatusSeeOther, contains: ""},
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
			}, Options{Accounts: &auth.Store{}, Masters: &master.Store{}, Academics: &academic.Store{}, CSRFKey: []byte(strings.Repeat("a", 32))})
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

func TestTimetableFilterOptionsUseOnlyAssignedLessonsAndSortByName(t *testing.T) {
	classes, subjects := timetableFilterOptions([]academic.TimetableEntry{
		{ClassID: 2, ClassName: "8B", SubjectID: 4, SubjectName: "Matematika"},
		{ClassID: 1, ClassName: "7A", SubjectID: 3, SubjectName: "Bahasa Indonesia"},
		{Kind: "activity", ClassName: "Semua rombel", SubjectName: "Upacara"},
	})
	if len(classes) != 2 || classes[0].Name != "7A" || classes[1].Name != "8B" {
		t.Fatalf("unexpected class filter options: %#v", classes)
	}
	if len(subjects) != 2 || subjects[0].Name != "Bahasa Indonesia" || subjects[1].Name != "Matematika" {
		t.Fatalf("unexpected subject filter options: %#v", subjects)
	}
}

func TestSidebarKeepsActiveRoleMenuExpanded(t *testing.T) {
	tests := []struct {
		name, path, active, group string
		roles                     []string
	}{
		{name: "admin siswa", path: "/master/students", active: "students", group: "admin-master", roles: []string{"admin"}},
		{name: "kurikulum siswa", path: "/master/students", active: "students", group: "curriculum-students", roles: []string{"kurikulum"}},
		{name: "kurikulum guru", path: "/master/teachers", active: "teachers", group: "curriculum-students", roles: []string{"kurikulum"}},
		{name: "admin guru", path: "/master/teachers", active: "teachers", group: "admin-master", roles: []string{"admin"}},
		{name: "kurikulum kelas", path: "/akademik/kelas/4/wali", active: "classes", group: "curriculum-students", roles: []string{"kurikulum"}},
		{name: "admin penempatan", path: "/admin/placements", active: "placements", group: "admin-master", roles: []string{"admin"}},
		{name: "admin pengguna dan role", path: "/admin/users", active: "users", group: "admin-access", roles: []string{"admin"}},
		{name: "admin riwayat akses", path: "/admin/audit", active: "audit", group: "admin-access", roles: []string{"admin"}},
		{name: "admin penanganan kehadiran TU", path: "/admin/kehadiran-tu", active: "teacher-attendance-management", group: "admin-attendance", roles: []string{"admin"}},
		{name: "admin lokasi kehadiran guru", path: "/admin/attendance/location", active: "teacher-attendance-location", group: "admin-attendance", roles: []string{"admin"}},
		{name: "guru mapel kehadiran pribadi", path: "/kehadiran-saya", active: "teacher-attendance", group: "teacher-attendance", roles: []string{"guru_mapel"}},
		{name: "wali kelas rekap pribadi", path: "/kehadiran-saya/rekap", active: "teacher-recap", group: "teacher-attendance", roles: []string{"wali_kelas"}},
		{name: "guru mapel jadwal", path: "/jadwal-saya", active: "my-schedule", group: "guru-mapel", roles: []string{"guru_mapel"}},
		{name: "guru mapel detail absensi", path: "/jadwal-saya/absensi/catatan/3", active: "lesson-attendance", group: "guru-mapel", roles: []string{"guru_mapel"}},
		{name: "kurikulum mengoreksi absensi", path: "/jadwal-saya/absensi", active: "lesson-attendance", group: "curriculum-schedule", roles: []string{"kurikulum"}},
		{name: "kurikulum guru mapel route bersama", path: "/jadwal-saya/absensi", active: "lesson-attendance", group: "guru-mapel", roles: []string{"kurikulum", "guru_mapel"}},
		{name: "pengelola recap terpisah dari recap pribadi", path: "/tu/kehadiran-guru/rekap", active: "managed-teacher-recap", group: "teacher-attendance-management", roles: []string{"guru_mapel", "pegawai_tu", "pengelola_kehadiran_guru"}},
		{name: "kurikulum penempatan", path: "/admin/placements", active: "placements", group: "curriculum-students", roles: []string{"kurikulum"}},
		{name: "kalender akademik", path: "/akademik/tahun-ajaran", active: "years", group: "curriculum-calendar", roles: []string{"kurikulum"}},
		{name: "struktur kurikulum", path: "/akademik/struktur-kurikulum", active: "curriculum-structure", group: "curriculum-subjects", roles: []string{"kurikulum"}},
		{name: "jadwal mingguan", path: "/akademik/jadwal", active: "weekly-timetable", group: "curriculum-schedule", roles: []string{"kurikulum"}},
		{name: "impor jadwal", path: "/akademik/jadwal/impor", active: "weekly-timetable", group: "curriculum-schedule", roles: []string{"kurikulum"}},
		{name: "transisi siswa", path: "/akademik/transisi", active: "transitions", group: "curriculum-students", roles: []string{"kurikulum"}},
		{name: "gabungan memilih menu admin yang terlihat", path: "/master/students", active: "students", group: "admin-master", roles: []string{"admin", "kurikulum"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user := &auth.User{Roles: test.roles}
			active, group := sidebarState(test.path, user)
			if active != test.active || group != test.group {
				t.Fatalf("sidebarState(%q) = (%q, %q), want (%q, %q)", test.path, active, group, test.active, test.group)
			}
		})
	}
}

func TestPasswordMenuOpensFormModal(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber": func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(day int) string {
			return []string{"", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}[day]
		},
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templates}
	user := &auth.User{ID: 1, Username: "personel", DisplayName: "Pengguna Uji", Active: true, Roles: []string{"admin"}}
	r := httptest.NewRequest(http.MethodGet, "/account/password?modal=1", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, user))
	w := httptest.NewRecorder()
	a.password(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `<dialog id="password-modal" class="form-modal" data-form-modal data-modal-open-on-load`) {
		t.Fatal("password menu did not render the form modal open")
	}
	if !strings.Contains(body, `href="/account/password?modal=1"`) {
		t.Fatal("password action is missing from the user menu")
	}
	if strings.Contains(body, `<span>Kata Sandi</span></a>`) {
		t.Fatal("password action remains in the sidebar")
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
		r := httptest.NewRequest(http.MethodGet, "http://localhost/login", nil)
		w := httptest.NewRecorder()
		app.setSession(w, r, strings.Repeat("a", 64))
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("missing session cookie")
		}
		c := cookies[0]
		if !c.HttpOnly || c.Secure != secure || c.Domain != "" || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge != int(auth.SessionLifetime.Seconds()) {
			t.Fatal("unsafe cookie attributes")
		}
		w = httptest.NewRecorder()
		app.clearSession(w, r)
		c = w.Result().Cookies()[0]
		if c.MaxAge != -1 || c.Value != "" || c.Secure != secure {
			t.Fatal("cookie was not expired correctly")
		}
	}
}

func TestTrustedProxyRequestNormalization(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	for _, tc := range []struct {
		name, remote, proto, forwardedHost, wantHost string
		wantSecure                                   bool
	}{
		{name: "https dari proxy tepercaya", remote: "127.0.0.1:4567", proto: "https", forwardedHost: "school.ngrok-free.dev", wantHost: "school.ngrok-free.dev", wantSecure: true},
		{name: "header dari sumber tidak tepercaya", remote: "203.0.113.8:4567", proto: "https", forwardedHost: "evil.example", wantHost: "internal.local", wantSecure: false},
		{name: "daftar host ambigu", remote: "127.0.0.1:4567", proto: "https", forwardedHost: "school.ngrok-free.dev,evil.example", wantHost: "internal.local", wantSecure: false},
		{name: "protokol bukan https", remote: "127.0.0.1:4567", proto: "http", forwardedHost: "school.ngrok-free.dev", wantHost: "internal.local", wantSecure: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://internal.local/login", nil)
			r.RemoteAddr, r.Host = tc.remote, "internal.local"
			r.Header.Set("X-Forwarded-Proto", tc.proto)
			r.Header.Set("X-Forwarded-Host", tc.forwardedHost)
			got, secure := normalizeTrustedProxyRequest(r, trusted)
			if got.Host != tc.wantHost || secure != tc.wantSecure {
				t.Fatalf("host=%q secure=%v", got.Host, secure)
			}
			if got.Header.Get("X-Forwarded-Proto") != "" || got.Header.Get("X-Forwarded-Host") != "" {
				t.Fatal("header proxy diteruskan ke handler")
			}
		})
	}
}

func TestImportBodyLimit(t *testing.T) {
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: &auth.Store{}, Masters: &master.Store{}, Academics: &academic.Store{}, CSRFKey: []byte(strings.Repeat("a", 32))})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/master/students/import", nil)
	r.ContentLength = master.MaxImportBytes + (256 << 10) + 1
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal("upload limit not enforced before parsing")
	}
}

func TestQuickAttendanceMiddlewareBodyLimitByContentLength(t *testing.T) {
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: &auth.Store{}, Masters: &master.Store{}, Academics: &academic.Store{}, CSRFKey: []byte(strings.Repeat("a", 32))})
	if err != nil {
		t.Fatal(err)
	}

	const limit = 32 << 10
	for _, tc := range []struct {
		name       string
		length     int
		wantStatus int
	}{
		{name: "above limit rejected by middleware", length: limit + 1, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "at limit passes content length check", length: limit, wantStatus: http.StatusForbidden},
		{name: "below limit passes content length check", length: limit - 1, wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReader(strings.Repeat("x", tc.length))
			r := httptest.NewRequest(http.MethodPost, "/absensi/harian/cepat", body)
			r.ContentLength = int64(tc.length)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}
}

type countedReader struct {
	reader io.Reader
	read   int
}

func (r *countedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += n
	return n, err
}

func TestQuickAttendanceMiddlewareBodyLimitUnknownLength(t *testing.T) {
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: &auth.Store{}, Masters: &master.Store{}, Academics: &academic.Store{}, CSRFKey: []byte(strings.Repeat("a", 32))})
	if err != nil {
		t.Fatal(err)
	}

	const limit = 32 << 10
	body := &countedReader{reader: strings.NewReader(strings.Repeat("x", limit*2))}
	r := httptest.NewRequest(http.MethodPost, "/absensi/harian/cepat", nil)
	r.ContentLength = -1
	r.Body = io.NopCloser(body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d (CSRF rejects before authentication/handler)", w.Code, http.StatusForbidden)
	}
	if body.read != limit+1 {
		t.Fatalf("body bytes read=%d want exactly %d", body.read, limit+1)
	}
}

func TestLessonDayTemplatePreservesLegacySlots(t *testing.T) {
	periods := []academic.LessonPeriod{{Weekday: 1, Number: 1, StartsAt: "07:00:00", EndsAt: "07:40:00"}, {Weekday: 1, Number: 15, StartsAt: "09:00:00", EndsAt: "09:40:00"}, {Weekday: 2, Number: 2, StartsAt: "08:00:00", EndsAt: "08:40:00"}}
	rows := lessonDayRows(periods, 1)
	if len(rows) != 13 || rows[0].StartsAt != "07:00" || rows[1].StartsAt != "" || rows[12].Number != 15 {
		t.Fatal("template lost legacy/day scope", rows)
	}
	if len(lessonDayRows(nil, 6)) != 12 {
		t.Fatal("empty day missing template")
	}
}

// CSP intentionally blocks inline handlers; navigation must use external listeners.
func TestTemplatesAvoidInlineEventHandlers(t *testing.T) {
	files, err := fs.Glob(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	inline := regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	for _, name := range files {
		body, err := content.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if inline.Match(body) {
			t.Errorf("%s contains an inline event handler blocked by CSP", name)
		}
	}
}
