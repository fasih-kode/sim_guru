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
	"strconv"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestMasterHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	admin, adminPassword := testutil.User(t, db, "admin")
	curriculum, curriculumPassword := testutil.User(t, db, "kurikulum")
	teacher, teacherPassword := testutil.User(t, db, "guru_mapel")
	tu, tuPassword := testutil.User(t, db, "pegawai_tu")
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	rand.Read(key)
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db}, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	bodyOf := func(resp *http.Response) string {
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	get := func(c *http.Client, path string, status int) string {
		resp, err := c.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := bodyOf(resp)
		if resp.StatusCode != status {
			t.Fatalf("GET %s status=%d expected=%d", path, resp.StatusCode, status)
		}
		return body
	}
	pattern := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	csrfToken := func(body string) string {
		m := pattern.FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatal("csrf field absent")
		}
		return m[1]
	}
	post := func(c *http.Client, path string, data url.Values, status int) string {
		resp, err := c.PostForm(server.URL+path, data)
		if err != nil {
			t.Fatal(err)
		}
		body := bodyOf(resp)
		if resp.StatusCode != status {
			t.Fatalf("POST %s status=%d expected=%d body=%s", path, resp.StatusCode, status, body)
		}
		return body
	}
	login := func(u auth.User, password string) *http.Client {
		c := newClient()
		token := csrfToken(get(c, "/login", 200))
		post(c, "/login", url.Values{"gorilla.csrf.Token": {token}, "username": {u.Username}, "password": {password}}, 303)
		return c
	}
	ac := login(admin, adminPassword)
	kc := login(curriculum, curriculumPassword)
	tc := login(teacher, teacherPassword)
	tuc := login(tu, tuPassword)
	get(newClient(), "/master/students", 303)
	token := csrfToken(get(ac, "/master/students/new", 200))
	post(ac, "/master/students/new", url.Values{"name": {"Missing CSRF"}}, 403)
	invalid := post(ac, "/master/students/new", url.Values{"gorilla.csrf.Token": {token}, "name": {"Nama tetap terisi"}, "birth_date": {"2999-01-01"}, "gender": {"L"}, "active": {"true"}}, 422)
	if !strings.Contains(invalid, "Nama tetap terisi") || !strings.Contains(invalid, "Tanggal lahir") {
		t.Fatal("validation lost submitted data")
	}
	suffix := testutil.Random(t)[:12]
	name := "Siswa <script>bad</script> " + suffix
	phone := "PRIVATE_" + suffix
	post(ac, "/master/students/new", url.Values{"gorilla.csrf.Token": {token}, "name": {name}, "birth_date": {"2013-01-02"}, "gender": {"P"}, "phone": {phone}, "active": {"true"}}, 303)
	var studentID int64
	if err = db.QueryRow(`SELECT id FROM app.students WHERE name=$1`, name).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(studentID, 10)
	base := "/master/students/" + id
	detail := get(ac, base, 200)
	if strings.Contains(detail, "<script>bad</script>") || !strings.Contains(detail, phone) {
		t.Fatal("Admin detail escaping or contact view failed")
	}
	readOnly := get(kc, "/master/students?q="+suffix, 200)
	if !strings.Contains(readOnly, `nav-link nav-curriculum-parent is-active`) || !strings.Contains(readOnly, `data-navigation-tab="curriculum-students"`) || !strings.Contains(readOnly, `href="/master/students" aria-current="page"`) {
		t.Fatal("Kurikulum parent and active Data Siswa tab are not marked according to D-047")
	}
	if !strings.Contains(readOnly, "baca-saja") || strings.Contains(readOnly, "/edit") || strings.Contains(readOnly, "/new") {
		t.Fatal("read-only list exposed editing")
	}
	if strings.Contains(get(kc, base, 200), phone) {
		t.Fatal("curriculum contact leak")
	}
	get(kc, "/master/students/new", 403)
	get(kc, base+"/edit", 403)
	get(kc, base+"/history", 403)
	get(tc, "/master/students", 403)
	ktoken := csrfToken(get(kc, "/account/password", 200))
	ttoken := csrfToken(get(tc, "/account/password", 200))
	values := url.Values{"gorilla.csrf.Token": {ktoken}, "name": {"Unauthorized"}, "birth_date": {"2013-01-02"}, "gender": {"P"}, "reason": {"Unauthorized"}, "version": {"1"}}
	post(kc, base+"/edit", values, 403)
	values.Set("gorilla.csrf.Token", ttoken)
	post(tc, base+"/edit", values, 403)
	editToken := csrfToken(get(ac, base+"/edit", 200))
	correction := url.Values{"gorilla.csrf.Token": {editToken}, "name": {"Nama baru " + suffix}, "birth_date": {"2013-01-02"}, "gender": {"P"}, "nis": {"NIS_" + suffix}, "nisn": {"NISN_" + suffix}, "phone": {phone}, "version": {"1"}}
	post(ac, base+"/edit", correction, 422)
	correction.Set("reason", "Pembetulan identitas")
	post(ac, base+"/edit", correction, 303)
	if !strings.Contains(get(kc, base, 200), "Nama baru "+suffix) {
		t.Fatal("corrected identity not visible through same ID")
	}
	history := get(ac, base+"/history", 200)
	if !strings.Contains(history, "Pembetulan identitas") || !strings.Contains(history, "Siswa") {
		t.Fatal("correction history missing")
	}
	duplicate := url.Values{"gorilla.csrf.Token": {token}, "name": {"Duplikat"}, "birth_date": {"2013-01-01"}, "gender": {"L"}, "nis": {"NIS_" + suffix}, "active": {"true"}}
	post(ac, "/master/students/new", duplicate, 409)
	// Teacher master linkage does not grant roles or disable the linked account.
	teacherToken := csrfToken(get(ac, "/master/teachers/new", 200))
	teacherName := "Guru " + suffix
	post(ac, "/master/teachers/new", url.Values{"gorilla.csrf.Token": {teacherToken}, "name": {teacherName}, "user_id": {strconv.FormatInt(teacher.ID, 10)}, "email": {"guru@example.test"}, "phone": {phone}, "active": {"true"}}, 303)
	var teacherID int64
	if err = db.QueryRow(`SELECT id FROM app.teachers WHERE name=$1`, teacherName).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	teacherBase := "/master/teachers/" + strconv.FormatInt(teacherID, 10)
	get(kc, teacherBase, 200)
	get(kc, teacherBase+"/edit", 403)
	post(kc, teacherBase+"/edit", url.Values{"gorilla.csrf.Token": {ktoken}}, 403)
	// Admin without Curriculum cannot read/write academic subject master.
	get(ac, "/master/subjects", 403)
	get(ac, "/master/subjects/new", 403)
	post(ac, "/master/subjects/new", url.Values{"gorilla.csrf.Token": {token}, "name": {"Denied"}, "code": {"DENIED"}}, 403)
	subjectToken := csrfToken(get(kc, "/master/subjects/new", 200))
	post(kc, "/master/subjects/new", url.Values{"gorilla.csrf.Token": {subjectToken}, "name": {"Mapel " + suffix}, "code": {"PAI_" + suffix}, "active": {"true"}}, 303)
	if !strings.Contains(get(kc, "/master/subjects?q="+suffix, 200), strings.ToUpper("PAI_"+suffix)) {
		t.Fatal("subject creation/list failed")
	}
	get(tc, "/master/subjects", 403)
	get(tuc, "/master/staff", 403)
	get(tuc, "/master/staff/new", 403)
	staffToken := csrfToken(get(ac, "/master/staff/new", 200))
	staffName := "Pegawai " + suffix
	post(ac, "/master/staff/new", url.Values{"gorilla.csrf.Token": {staffToken}, "name": {staffName}, "employee_number": {"PGW-" + suffix}, "user_id": {strconv.FormatInt(tu.ID, 10)}, "active": {"true"}}, 303)
	var staffID int64
	if err = db.QueryRow(`SELECT id FROM app.staff_members WHERE name=$1`, staffName).Scan(&staffID); err != nil {
		t.Fatal(err)
	}
	var roleCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.user_roles WHERE user_id=$1`, tu.ID).Scan(&roleCount); err != nil || roleCount != 1 {
		t.Fatal("staff account linkage changed roles")
	}
	get(ac, "/master/staff/"+strconv.FormatInt(staffID, 10)+"/history", 200)
	get(ac, "/master/invalid", 404)
	// Unsupported PATCH cannot bypass the form-only write surface.
	req, _ := http.NewRequest("PATCH", server.URL+base, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", ktoken)
	resp, err := kc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	bodyOf(resp)
	if resp.StatusCode < 400 {
		t.Fatal("unsupported PATCH accepted")
	}
}
