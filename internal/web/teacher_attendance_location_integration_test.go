//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherAttendanceLocationHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	store := &academic.Store{DB: db}
	admin, adminPassword := testutil.User(t, db, "admin")
	adminDual, dualPassword := testutil.User(t, db, "admin", "kurikulum")
	nonAdmin, nonAdminPassword := testutil.User(t, db, "kesiswaan")
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: store, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	csrfRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	read := func(resp *http.Response, status int) string {
		t.Helper()
		defer resp.Body.Close()
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if resp.StatusCode != status {
			t.Fatalf("HTTP %d want %d: %s", resp.StatusCode, status, body)
		}
		return string(body)
	}
	get := func(client *http.Client, path string, status int) string {
		t.Helper()
		resp, getErr := client.Get(server.URL + path)
		if getErr != nil {
			t.Fatal(getErr)
		}
		return read(resp, status)
	}
	post := func(client *http.Client, values url.Values, status int) string {
		t.Helper()
		resp, postErr := client.PostForm(server.URL+"/admin/attendance/location", values)
		if postErr != nil {
			t.Fatal(postErr)
		}
		return read(resp, status)
	}
	login := func(user auth.User, password string) *http.Client {
		t.Helper()
		jar, jarErr := cookiejar.New(nil)
		if jarErr != nil {
			t.Fatal(jarErr)
		}
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		form := url.Values{"username": {user.Username}, "password": {password}}
		m := csrfRE.FindStringSubmatch(get(client, "/login", http.StatusOK))
		if len(m) != 2 {
			t.Fatal("login CSRF missing")
		}
		form.Set("gorilla.csrf.Token", m[1])
		resp, postErr := client.PostForm(server.URL+"/login", form)
		if postErr != nil {
			t.Fatal(postErr)
		}
		read(resp, http.StatusSeeOther)
		return client
	}
	adminClient := login(admin, adminPassword)
	dualClient := login(adminDual, dualPassword)
	otherClient := login(nonAdmin, nonAdminPassword)
	initial, _, err := store.TeacherAttendanceLocation(context.Background(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if initial.Configured {
			latest, _, getErr := store.TeacherAttendanceLocation(context.Background(), admin.ID)
			if getErr == nil && (latest.Latitude != initial.Latitude || latest.Longitude != initial.Longitude || latest.RadiusM != initial.RadiusM) {
				_ = store.SaveTeacherAttendanceLocation(context.Background(), admin.ID, academic.TeacherAttendanceLocationInput{Latitude: initial.Latitude, Longitude: initial.Longitude, RadiusM: initial.RadiusM, ExpectedVersion: latest.Version, Reason: "Pulihkan konfigurasi awal setelah tes HTTP"})
			}
		}
	}()
	body := get(adminClient, "/admin/attendance/location", http.StatusOK)
	if !strings.Contains(body, "Lokasi Kehadiran Guru") || strings.Contains(body, "Masuk") || strings.Contains(body, "Pulang") {
		t.Fatal("admin page missing or unexpectedly exposes operational attendance actions")
	}
	get(otherClient, "/admin/attendance/location", http.StatusForbidden)
	if _, err = otherClient.PostForm(server.URL+"/admin/attendance/location", url.Values{}); err != nil {
		t.Fatal(err)
	}
	resp, err := otherClient.PostForm(server.URL+"/admin/attendance/location", url.Values{"latitude": {"-6.2"}})
	if err != nil {
		t.Fatal(err)
	}
	read(resp, http.StatusForbidden)
	if _, err = adminClient.PostForm(server.URL+"/admin/attendance/location", url.Values{"latitude": {"-6.2"}}); err != nil {
		t.Fatal(err)
	}
	resp, err = adminClient.PostForm(server.URL+"/admin/attendance/location", url.Values{"latitude": {"-6.2"}, "longitude": {"106.8"}, "radius_m": {"100"}, "version": {"0"}, "reason": {"Titik sekolah"}})
	if err != nil {
		t.Fatal(err)
	}
	read(resp, http.StatusForbidden)
	m := csrfRE.FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatal("page CSRF missing")
	}
	values := url.Values{"latitude": {"-6.2"}, "longitude": {"106.8"}, "radius_m": {"100"}, "version": {"0"}, "reason": {"Titik sekolah"}, "gorilla.csrf.Token": {m[1]}}
	if initial.Configured {
		values.Set("version", strconvFormat(initial.Version))
		values.Set("latitude", strconvFormatFloat(initial.Latitude))
		values.Set("longitude", strconvFormatFloat(initial.Longitude))
		values.Set("radius_m", fmt.Sprintf("%d", initial.RadiusM))
		values.Set("reason", "Tidak ada perubahan")
	}
	badValues := cloneTeacherLocationValues(values)
	badValues.Set("latitude", "91")
	badValues.Set("reason", "input dipertahankan")
	bad := post(adminClient, badValues, http.StatusUnprocessableEntity)
	if !strings.Contains(bad, `value="91"`) || !strings.Contains(bad, "Koordinat sekolah tidak valid") || !strings.Contains(bad, "data-modal-open-on-load") {
		t.Fatalf("invalid location retention/error/modal missing: value=%v error=%v modal=%v", strings.Contains(bad, `value="91"`), strings.Contains(bad, "Koordinat sekolah tidak valid"), strings.Contains(bad, "data-modal-open-on-load"))
	}
	if dir := os.Getenv("SIM_GURU_VISUAL_DIR"); dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		css, readErr := content.ReadFile("static/app.css")
		if readErr != nil {
			t.Fatal(readErr)
		}
		appJS, readErr := content.ReadFile("static/app.js")
		if readErr != nil {
			t.Fatal(readErr)
		}
		appScript := strings.Replace(string(appJS), "htmx.config.allowEval = false;\nhtmx.config.includeIndicatorStyles = false;\nhtmx.config.selfRequestsOnly = true;\nhtmx.config.historyCacheSize = 0;\n\n", "", 1)
		fixture := csrfRE.ReplaceAllString(bad, `name="gorilla.csrf.Token" value="fixture-token"`)
		fixture = strings.Replace(fixture, `<link rel="stylesheet" href="/static/app.css">`, `<style>`+string(css)+`</style>`, 1)
		fixture = strings.Replace(fixture, `<link rel="icon" href="/static/favicon.svg" type="image/svg+xml">`, "", 1)
		fixture = strings.Replace(fixture, "<script src=\"/static/htmx.min.js\" defer>\n</script>", "", 1)
		fixture = strings.Replace(fixture, "<script src=\"/static/app.js\" defer>\n</script>", "", 1)
		fixture = strings.Replace(fixture, "</body>", `<script>`+appScript+`</script></body>`, 1)
		if err = os.WriteFile(filepath.Join(dir, "teacher-location-error.html"), []byte(fixture), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if initial.Configured {
		// Role rangkap sah karena akun ini memang memiliki role Admin aktif.
		values.Set("version", strconvFormat(initial.Version))
		if initial.Latitude > -89 {
			values.Set("latitude", strconvFormatFloat(initial.Latitude-0.0001))
		} else {
			values.Set("latitude", strconvFormatFloat(initial.Latitude+0.0001))
		}
		values.Set("reason", "Uji pengaturan multi-role")
	}
	resp, err = adminClient.PostForm(server.URL+"/admin/attendance/location", values)
	if err != nil {
		t.Fatal(err)
	}
	read(resp, http.StatusSeeOther)
	current, _, err := store.TeacherAttendanceLocation(context.Background(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	refreshed := get(dualClient, "/admin/attendance/location", http.StatusOK)
	newToken := csrfRE.FindStringSubmatch(refreshed)
	if len(newToken) != 2 {
		t.Fatal("refreshed CSRF token missing")
	}
	stale := cloneTeacherLocationValues(values)
	stale.Set("version", strconvFormat(current.Version-1))
	stale.Set("gorilla.csrf.Token", newToken[1])
	stale.Set("reason", "Versi basi")
	conflict := post(dualClient, stale, http.StatusConflict)
	if !strings.Contains(conflict, "Muat ulang") {
		t.Fatal("stale-version explanation missing")
	}
}

func cloneTeacherLocationValues(values url.Values) url.Values {
	out := url.Values{}
	for key, list := range values {
		out[key] = append([]string(nil), list...)
	}
	return out
}

func strconvFormat(value int64) string        { return fmt.Sprintf("%d", value) }
func strconvFormatFloat(value float64) string { return fmt.Sprintf("%.7f", value) }
