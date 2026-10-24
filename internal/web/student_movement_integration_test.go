//go:build integration

package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentMovementHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := academic.Store{DB: db}
	admin, ap := testutil.User(t, db, "admin")
	cur, cp := testutil.User(t, db, "kurikulum")
	teacher, tp := testutil.User(t, db, "guru_mapel")
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &s, CSRFKey: []byte("12345678901234567890123456789012")})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	body := func(resp *http.Response, status int) string {
		t.Helper()
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		if resp.StatusCode != status {
			t.Fatalf("status %d want %d body %s", resp.StatusCode, status, b)
		}
		return string(b)
	}
	get := func(c *http.Client, path string, status int) string {
		t.Helper()
		r, e := c.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		return body(r, status)
	}
	post := func(c *http.Client, path string, v url.Values, status int) string {
		t.Helper()
		r, e := c.PostForm(server.URL+path, v)
		if e != nil {
			t.Fatal(e)
		}
		return body(r, status)
	}
	re := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	token := func(c *http.Client, path string) string {
		t.Helper()
		m := re.FindStringSubmatch(get(c, path, 200))
		if len(m) != 2 {
			t.Fatal("missing csrf")
		}
		return m[1]
	}
	login := func(u auth.User, password string) *http.Client {
		t.Helper()
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		post(c, "/login", url.Values{"gorilla.csrf.Token": {token(c, "/login")}, "username": {u.Username}, "password": {password}}, 303)
		return c
	}
	ac, kc, gc := login(admin, ap), login(cur, cp), login(teacher, tp)
	y, err := s.CreateYear(ctx, cur.ID, "HTTP Movement "+testutil.Random(t)[:10])
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateYear(ctx, cur.ID, y.ID); err != nil {
		t.Fatal(err)
	}
	a, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "B", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var student int64
	if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Perpindahan Uji','2013-01-01','L') RETURNING id`).Scan(&student); err != nil {
		t.Fatal(err)
	}
	placement, err := s.CreatePlacement(ctx, admin.ID, student, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	move := "/akademik/penempatan/" + strconv.FormatInt(placement, 10) + "/pindah-kelas"
	exit := "/master/students/" + strconv.FormatInt(student, 10) + "/pindah-sekolah"
	hist := "/students/" + strconv.FormatInt(student, 10) + "/perpindahan"
	get(ac, move, 403)
	get(kc, exit, 403)
	get(gc, hist, 403)
	post(kc, move, url.Values{}, 403)
	moveBody := get(kc, move, 200)
	if !strings.Contains(moveBody, "data-modal-open-on-load") {
		t.Fatal("modal missing")
	}
	today := time.Now().In(time.FixedZone("WIB", 7*3600))
	values := url.Values{"gorilla.csrf.Token": {token(kc, move)}, "expected_class_id": {strconv.FormatInt(a.ID, 10)}, "target_class_id": {strconv.FormatInt(b.ID, 10)}, "effective_date": {today.AddDate(0, 0, -1).Format("2006-01-02")}, "reason": {"Penyesuaian kelas"}, "action": {"preview"}}
	preview := post(kc, move, values, 200)
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.student_class_moves WHERE placement_id=$1`, placement).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview wrote data", err)
	}
	values.Set("action", "save")
	post(kc, move, values, 200)
	values.Set("confirm", "yes")
	post(kc, move, values, 303)
	current, err := s.MovementStudent(ctx, admin.ID, student, 0)
	if err != nil {
		t.Fatal(err)
	}
	exitBody := get(ac, exit, 200)
	ev := url.Values{"gorilla.csrf.Token": {token(ac, exit)}, "version": {strconv.FormatInt(current.Version, 10)}, "effective_date": {today.Format("2006-01-02")}, "reason": {"Pindah domisili"}, "school": {"SMP Tujuan"}, "action": {"preview"}}
	exitPreview := post(ac, exit, ev, 200)
	ev.Set("action", "save")
	ev.Set("confirm", "yes")
	post(ac, exit, ev, 303)
	if !strings.Contains(get(ac, hist, 200), "SMP Tujuan") {
		t.Fatal("exit history missing")
	}
	detail := "/master/students/" + strconv.FormatInt(student, 10)
	if !strings.Contains(get(ac, detail, 200), "Pindah sekolah") {
		t.Fatal("status missing")
	}
	readmit := "/master/students/" + strconv.FormatInt(student, 10) + "/terima-kembali"
	get(kc, readmit, 403)
	post(ac, readmit, url.Values{}, 403)
	readmitBody := get(ac, readmit, 200)
	if !strings.Contains(readmitBody, "Sekolah asal terakhir") || !strings.Contains(readmitBody, `name="exit_id"`) {
		t.Fatal("readmission form missing fields")
	}
	current, err = s.MovementStudent(ctx, admin.ID, student, 0)
	if err != nil {
		t.Fatal(err)
	}
	rv := url.Values{"gorilla.csrf.Token": {token(ac, readmit)}, "version": {strconv.FormatInt(current.Version, 10)}, "exit_id": {strconv.FormatInt(current.ExitID, 10)}, "target_class_id": {strconv.FormatInt(a.ID, 10)}, "effective_date": {today.AddDate(0, 0, 1).Format("2006-01-02")}, "reason": {"Kembali belajar"}, "school": {"Sekolah asal terakhir"}, "action": {"preview"}}
	readmitPreview := post(ac, readmit, rv, 200)
	if err = db.QueryRow(`SELECT count(*) FROM app.student_school_returns WHERE exit_id=$1`, current.ExitID).Scan(&count); err != nil || count != 0 {
		t.Fatal("readmission preview wrote data", err)
	}
	rv.Set("action", "save")
	post(ac, readmit, rv, 200)
	rv.Set("confirm", "yes")
	post(ac, readmit, rv, 303)
	if !strings.Contains(get(ac, detail, 200), "Kembali terjadwal") {
		t.Fatal("pending readmission status missing")
	}
	post(ac, readmit, rv, 200)
	if err = db.QueryRow(`SELECT count(*) FROM app.student_school_returns WHERE exit_id=$1`, current.ExitID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate readmission", err)
	}
	if dir := os.Getenv("SIM_GURU_VISUAL_DIR"); dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, html := range map[string]string{"move.html": moveBody, "move-preview.html": preview, "exit.html": exitBody, "exit-preview.html": exitPreview, "readmit.html": readmitBody, "readmit-preview.html": readmitPreview} {
			html = re.ReplaceAllString(html, `name="gorilla.csrf.Token" value="visual-fixture"`)
			if err = os.WriteFile(filepath.Join(dir, name), []byte(html), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
