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
	"strconv"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherAttendanceRecapHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	accounts, e := auth.New(db)
	if e != nil {
		t.Fatal(e)
	}
	key := make([]byte, 32)
	if _, e = rand.Read(key); e != nil {
		t.Fatal(e)
	}
	handler, e := New(func(context.Context) error { return nil }, Options{Masters: &master.Store{DB: db}, Accounts: accounts, Academics: &academic.Store{DB: db}, CSRFKey: key})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var users, teachers []int64
	t.Cleanup(func() {
		tx, e := db.Begin()
		if e != nil {
			t.Error(e)
			return
		}
		defer tx.Rollback()
		for _, id := range teachers {
			if _, e = tx.Exec(`DELETE FROM app.teachers WHERE id=$1`, id); e != nil {
				t.Error(e)
				return
			}
		}
		for _, id := range users {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, e = tx.Exec(q, id); e != nil {
					t.Error(e)
					return
				}
			}
		}
		if e = tx.Commit(); e != nil {
			t.Error(e)
		}
	})
	actor := func(roles ...string) (*http.Client, int64) {
		t.Helper()
		u, pw := testutil.User(t, db, roles...)
		users = append(users, u.ID)
		var id int64
		if e = db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES($1,$2) RETURNING id`, "G3 HTTP <uji> "+testutil.Random(t)[:8], u.ID).Scan(&id); e != nil {
			t.Fatal(e)
		}
		teachers = append(teachers, id)
		token, e := accounts.Login(context.Background(), u.Username, pw, "")
		if e != nil {
			t.Fatal(e)
		}
		jar, _ := cookiejar.New(nil)
		base, _ := url.Parse(server.URL)
		jar.SetCookies(base, []*http.Cookie{{Name: sessionCookie, Value: token, Path: "/"}})
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, id
	}
	teacher, teacherID := actor("guru_mapel", "admin", "kurikulum", "petugas_absensi")
	other, otherID := actor("wali_kelas")
	manager, _ := actor("pegawai_tu", "pengelola_kehadiran_guru")
	plain, _ := actor("pegawai_tu")
	permission, _ := actor("pengelola_kehadiran_guru")
	officer, _ := actor("petugas_absensi")
	get := func(c *http.Client, path string, want int) string {
		t.Helper()
		resp, e := c.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s: status=%d want=%d", path, resp.StatusCode, want)
		}
		return string(b)
	}
	partial := func(client *http.Client, path string, want int) string {
		t.Helper()
		req, err := http.NewRequest("GET", server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-List-Request", "true")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("partial %s: got %d want %d", path, resp.StatusCode, want)
		}
		if want == 200 && (strings.Contains(string(data), "<!doctype html>") || !strings.Contains(string(data), `id="list-region"`)) {
			t.Fatal("partial bukan komponen daftar")
		}
		return string(data)
	}
	for _, q := range []string{"G3", "g3", ""} {
		b := partial(manager, "/tu/kehadiran-guru/rekap?from=1906-01-01&to=1906-01-02&q="+q, 200)
		if !strings.Contains(b, "G3 HTTP") {
			t.Fatal("pencarian cocok/kosong/kapitalisasi gagal")
		}
	}
	if b := partial(manager, "/tu/kehadiran-guru/rekap?from=1906-01-01&to=1906-01-02&q=tidak-ada-"+testutil.Random(t), 200); !strings.Contains(b, "Tidak ada data yang cocok dengan pencarian.") {
		t.Fatal("pesan kosong salah")
	}
	partial(officer, "/tu/kehadiran-guru/rekap", 403)
	partial(teacher, "/kehadiran-saya/rekap?from=1906-01-01&to=1906-01-02&teacher_id="+strconv.FormatInt(otherID, 10), 404)
	path := "/kehadiran-saya/rekap?from=1906-01-01&to=1906-01-31"
	body := get(teacher, path, 200)
	for _, text := range []string{"Rekap Kehadiran Saya", "Belum Dapat Dinilai", "Hari wajib hadir terjadwal", "Berikutnya", "G3 HTTP &lt;uji&gt;"} {
		if !strings.Contains(body, text) {
			t.Fatal("missing", text)
		}
	}
	if strings.Contains(body, "G3 HTTP <uji>") {
		t.Fatal("XSS")
	}
	get(teacher, path+"&day_page=2", 200)
	get(teacher, path+"&day_page=9223372036854775807", 422)
	get(teacher, path+"&date=1906-01-01", 200)
	get(teacher, path+"&date=1906-02-01", 404)
	get(teacher, path+"&teacher_id="+strconv.FormatInt(otherID, 10), 404)
	get(other, path+"&teacher_id="+strconv.FormatInt(teacherID, 10), 404)
	get(teacher, "/kehadiran-saya/rekap?from=1906-02-01&to=1906-01-01", 422)
	managed := "/tu/kehadiran-guru/rekap?from=1906-01-01&to=1906-01-31&teacher_id=" + strconv.FormatInt(teacherID, 10)
	get(manager, managed, 200)
	body = get(manager, managed+"&q=G3&page=2", 200)
	if !strings.Contains(body, `name="q" value="G3"`) || !strings.Contains(body, `name="page" value="2"`) || !strings.Contains(body, `page=2&amp;q=G3`) {
		t.Fatal("filter pencarian/halaman hilang dari detail dan tautan kembali")
	}

	get(manager, "/tu/kehadiran-guru/rekap?from=1906-01-01&to=1906-01-31&q=G3+HTTP", 200)
	for _, c := range []*http.Client{teacher, other, plain, permission, officer} {
		get(c, managed, 403)
	}
	get(officer, path, 403)
	get(permission, path, 403)
	// Endpoint hanya baca dan tidak membuka jalur POST baru.
	resp, e := teacher.PostForm(server.URL+"/kehadiran-saya/rekap", url.Values{})
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("POST tanpa CSRF", resp.StatusCode)
	}
	body = get(teacher, "/kehadiran-saya", 200)
	if !strings.Contains(body, "Ringkasan status hari ini") || !strings.Contains(body, "data-attendance-action=\"check_in\"") {
		t.Fatal("ringkasan merusak capture")
	}
}
