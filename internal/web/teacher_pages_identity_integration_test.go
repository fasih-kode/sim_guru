//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherScheduleAndAttendanceRequireIdentityButAllowNoAssignmentIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	teacher, teacherPassword := testutil.User(t, db, "guru_mapel")
	roleOnly, roleOnlyPassword := testutil.User(t, db, "guru_mapel")
	identity, err := (&master.Store{DB: db}).Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{
		Name: "Guru Uji Tanpa Penugasan " + testutil.Random(t)[:10], UserID: teacher.ID, Active: true,
	}})
	if err != nil {
		t.Fatal("gagal menautkan identitas fixture Guru Mapel", err)
	}
	var assignments int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_assignments WHERE teacher_id=$1`, identity.ID).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if assignments != 0 {
		t.Fatalf("fixture harus tanpa penugasan agar jalur data kosong diuji, ditemukan %d", assignments)
	}
	t.Cleanup(func() {
		if _, e := db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, identity.ID); e != nil {
			t.Errorf("cleanup histori identitas fixture: %v", e)
		}
		if _, e := db.Exec(`DELETE FROM app.teachers WHERE id=$1`, identity.ID); e != nil {
			t.Errorf("cleanup identitas fixture: %v", e)
		}
		for _, userID := range []int64{admin.ID, teacher.ID, roleOnly.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, e := db.Exec(q, userID); e != nil {
					t.Errorf("cleanup akun fixture: %v", e)
					return
				}
			}
		}
	})

	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db}, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	csrfPattern := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	login := func(username, password string) (*http.Client, string) {
		t.Helper()
		jar, e := cookiejar.New(nil)
		if e != nil {
			t.Fatal(e)
		}
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		page, e := client.Get(server.URL + "/login")
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(page.Body)
		_ = page.Body.Close()
		if e != nil || page.StatusCode != http.StatusOK {
			t.Fatalf("halaman login gagal: status=%d err=%v", page.StatusCode, e)
		}
		match := csrfPattern.FindSubmatch(body)
		if len(match) != 2 {
			t.Fatal("token CSRF login tidak ditemukan")
		}
		response, e := client.PostForm(server.URL+"/login", url.Values{"username": {username}, "password": {password}, "gorilla.csrf.Token": {string(match[1])}})
		if e != nil {
			t.Fatal(e)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusSeeOther {
			t.Fatalf("login fixture ditolak: status=%d", response.StatusCode)
		}
		return client, string(match[1])
	}
	readStatus := func(client *http.Client, path string, want int) string {
		t.Helper()
		response, e := client.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if e != nil || response.StatusCode != want {
			t.Fatalf("GET %s status=%d want=%d err=%v", path, response.StatusCode, want, e)
		}
		return string(body)
	}
	linkedClient, csrfToken := login(teacher.Username, teacherPassword)
	for _, path := range []string{"/jadwal-saya", "/kehadiran-saya"} {
		body := readStatus(linkedClient, path, http.StatusOK)
		if strings.Contains(body, academic.ErrForbidden.Error()) {
			t.Fatalf("identitas aktif tanpa assignment harus membuka halaman dengan kondisi data kosong, bukan 403: %s", path)
		}
	}
	if body := readStatus(linkedClient, "/jadwal-saya", http.StatusOK); !strings.Contains(body, "Jadwal Saya") {
		t.Fatal("halaman jadwal kosong tidak menampilkan judul halaman")
	}
	roleOnlyClient, _ := login(roleOnly.Username, roleOnlyPassword)
	for _, path := range []string{"/jadwal-saya", "/kehadiran-saya"} {
		readStatus(roleOnlyClient, path, http.StatusForbidden)
	}
	readStatus(linkedClient, "/akademik/bobot-penilaian", http.StatusForbidden)
	post, err := linkedClient.PostForm(server.URL+"/akademik/bobot-penilaian?semester_id=1", url.Values{
		"gorilla.csrf.Token": {csrfToken}, "nh": {"60"}, "asts": {"20"}, "asas": {"20"}, "version": {"0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = post.Body.Close()
	if post.StatusCode != http.StatusForbidden {
		t.Fatalf("POST bobot oleh Guru Mapel harus ditolak, status=%d", post.StatusCode)
	}
}
