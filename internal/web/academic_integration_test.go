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

func TestAcademicHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	admin, adminPassword := testutil.User(t, db, "admin")
	curriculum, curriculumPassword := testutil.User(t, db, "kurikulum")
	teacher, teacherPassword := testutil.User(t, db, "guru_mapel")
	wali, waliPassword := testutil.User(t, db, "guru_mapel", "wali_kelas")
	multi, multiPassword := testutil.User(t, db, "admin", "kurikulum")
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
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	bodyOf := func(resp *http.Response, status int) string {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status {
			t.Fatalf("HTTP %d expected %d; body=%s", resp.StatusCode, status, body)
		}
		return string(body)
	}
	get := func(c *http.Client, path string, status int) string {
		resp, err := c.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return bodyOf(resp, status)
	}
	csrfField := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	csrf := func(c *http.Client, path string) string {
		match := csrfField.FindStringSubmatch(get(c, path, http.StatusOK))
		if len(match) != 2 {
			t.Fatal("CSRF field not rendered")
		}
		return match[1]
	}
	post := func(c *http.Client, path string, values url.Values, status int) string {
		resp, err := c.PostForm(server.URL+path, values)
		if err != nil {
			t.Fatal(err)
		}
		return bodyOf(resp, status)
	}
	login := func(user auth.User, password string) *http.Client {
		c := newClient()
		token := csrf(c, "/login")
		post(c, "/login", url.Values{"gorilla.csrf.Token": {token}, "username": {user.Username}, "password": {password}}, http.StatusSeeOther)
		return c
	}
	ac, kc := login(admin, adminPassword), login(curriculum, curriculumPassword)
	gc, mc := login(teacher, teacherPassword), login(multi, multiPassword)
	wc := login(wali, waliPassword)
	get(newClient(), "/akademik", http.StatusSeeOther)
	get(ac, "/akademik", http.StatusForbidden)
	get(kc, "/akademik", http.StatusOK)
	get(kc, "/akademik/tahun-ajaran", http.StatusOK)
	token := csrf(kc, "/akademik/tahun-ajaran")
	name := "Web " + testutil.Random(t)[:10]
	post(kc, "/akademik/tahun-ajaran", url.Values{"gorilla.csrf.Token": {token}, "name": {name}}, http.StatusSeeOther)
	var yearID int64
	if err = db.QueryRow(`SELECT id FROM app.school_years WHERE name=$1`, name).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	base := "/akademik/tahun-ajaran/" + strconv.FormatInt(yearID, 10)
	get(kc, "/akademik/tahun-ajaran", http.StatusOK)
	post(ac, base+"/aktifkan", url.Values{"gorilla.csrf.Token": {csrf(ac, "/account/password")}}, http.StatusForbidden)
	post(kc, base+"/aktifkan", url.Values{"gorilla.csrf.Token": {csrf(kc, "/akademik/tahun-ajaran")}}, http.StatusSeeOther)
	var semesterID int64
	if err = db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	semesterToken := csrf(kc, "/akademik/semester")
	post(kc, "/akademik/semester/"+strconv.FormatInt(semesterID, 10)+"/aktifkan", url.Values{"gorilla.csrf.Token": {semesterToken}}, http.StatusSeeOther)
	get(kc, "/akademik/semester", http.StatusOK)
	adminClasses := get(ac, "/akademik/kelas", http.StatusOK)
	if !strings.Contains(adminClasses, "Tambah kelas") {
		t.Fatal("Admin cannot create classes")
	}
	get(gc, "/akademik/kelas", http.StatusForbidden)
	get(newClient(), "/akademik/kelas", http.StatusSeeOther)
	get(kc, "/akademik/kelas/new", http.StatusForbidden)
	classForm := get(ac, "/akademik/kelas/new", http.StatusOK)
	classToken := csrfField.FindStringSubmatch(classForm)[1]
	className := "Rombongan"
	values := url.Values{"gorilla.csrf.Token": {classToken}, "school_year_id": {strconv.FormatInt(yearID, 10)}, "grade": {"7"}, "name": {className}, "version": {"0"}}
	withoutCSRF := url.Values{}
	for key, value := range values {
		withoutCSRF[key] = value
	}
	withoutCSRF.Del("gorilla.csrf.Token")
	post(ac, "/akademik/kelas/new", withoutCSRF, http.StatusForbidden)
	post(ac, "/akademik/kelas/new", values, http.StatusSeeOther)
	var classID int64
	if err = db.QueryRow(`SELECT id FROM app.classes WHERE school_year_id=$1 AND grade=7 AND name=$2`, yearID, className).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	var teacherID, studentID int64
	if err = db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Wali Uji',$1) RETURNING id`, wali.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender,nis) VALUES('Siswa Wali Uji','2013-01-01','L',$1) RETURNING id`, "WALI"+testutil.Random(t)[:8]).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if _, err = (&academic.Store{DB: db}).CreatePlacement(context.Background(), admin.ID, studentID, classID); err != nil {
		t.Fatal(err)
	}
	assignPath := "/akademik/kelas/" + strconv.FormatInt(classID, 10) + "/wali"
	assignPage := get(kc, assignPath, http.StatusOK)
	if !strings.Contains(assignPage, "Wali Kelas 7Rombongan") || !strings.Contains(assignPage, "Guru Wali Uji") {
		t.Fatal("form penugasan wali kelas tidak tersedia")
	}
	assignToken := csrfField.FindStringSubmatch(assignPage)[1]
	post(kc, assignPath, url.Values{"gorilla.csrf.Token": {assignToken}, "teacher_id": {strconv.FormatInt(teacherID, 10)}}, http.StatusSeeOther)
	if !strings.Contains(get(kc, "/akademik/kelas", http.StatusOK), "Guru Wali Uji") {
		t.Fatal("penugasan tidak tampil pada kelas")
	}
	homeroomHistory := get(kc, assignPath+"/histori", http.StatusOK)
	if !strings.Contains(homeroomHistory, "Guru Wali Uji") || !strings.Contains(homeroomHistory, "Histori Wali Kelas") {
		t.Fatal("histori wali kelas tidak tampil")
	}
	get(ac, assignPath+"/histori", http.StatusForbidden)
	if !strings.Contains(get(wc, "/wali-kelas", http.StatusOK), "Rombongan") {
		t.Fatal("wali tidak melihat kelas yang ditugaskan")
	}
	rosterPage := get(wc, "/wali-kelas/"+strconv.FormatInt(classID, 10), http.StatusOK)
	if !strings.Contains(rosterPage, "Siswa Wali Uji") {
		t.Fatal("wali tidak melihat siswa kelasnya")
	}
	get(gc, "/wali-kelas/"+strconv.FormatInt(classID, 10), http.StatusForbidden)
	get(wc, "/wali-kelas/999999", http.StatusForbidden)
	editPath := "/akademik/kelas/" + strconv.FormatInt(classID, 10) + "/edit"
	historyPath := "/akademik/histori/class/" + strconv.FormatInt(classID, 10)
	edit := get(ac, editPath, http.StatusOK)
	editToken := csrfField.FindStringSubmatch(edit)[1]
	values.Set("gorilla.csrf.Token", editToken)
	values.Set("name", "Dikoreksi")
	values.Set("reason", "Koreksi nama rombongan oleh Admin")
	values.Set("version", "1")
	post(ac, editPath, values, http.StatusSeeOther)
	adminClasses = get(ac, "/akademik/kelas", http.StatusOK)
	if !strings.Contains(adminClasses, editPath) || !strings.Contains(adminClasses, "Kurikulum menetapkan wali kelas") {
		t.Fatal("Admin edit link or readiness boundary missing")
	}
	readOnly := get(kc, "/akademik/kelas", http.StatusOK)
	if strings.Contains(readOnly, "Tambah kelas") || strings.Contains(readOnly, editPath) || !strings.Contains(readOnly, "Baca-saja. Hubungi Admin untuk membuat atau memperbaiki kelas.") {
		t.Fatal("Kurikulum class view does not enforce read-only UI")
	}
	get(kc, editPath, http.StatusForbidden)
	get(gc, editPath, http.StatusForbidden)
	for _, client := range []*http.Client{kc, gc} {
		values.Set("gorilla.csrf.Token", csrf(client, "/account/password"))
		post(client, "/akademik/kelas/new", values, http.StatusForbidden)
		post(client, editPath, values, http.StatusForbidden)
	}
	for _, client := range []*http.Client{ac, kc, mc} {
		history := get(client, historyPath, http.StatusOK)
		if !strings.Contains(history, "Koreksi nama rombongan oleh Admin") || !strings.Contains(history, `href="/akademik/kelas"`) || !strings.Contains(history, "Kembali ke Kelas") {
			t.Fatal("class history or accessible back link missing")
		}
	}
	get(gc, historyPath, http.StatusForbidden)
	get(newClient(), historyPath, http.StatusSeeOther)
	get(ac, "/akademik/tahun-ajaran", http.StatusForbidden)
	get(ac, "/akademik/semester", http.StatusForbidden)
	get(ac, "/akademik/histori/school_year/"+strconv.FormatInt(yearID, 10), http.StatusForbidden)
	get(ac, "/akademik/histori/semester/"+strconv.FormatInt(semesterID, 10), http.StatusForbidden)
	get(kc, "/akademik/histori/school_year/"+strconv.FormatInt(yearID, 10), http.StatusOK)
	get(kc, "/akademik/histori/semester/"+strconv.FormatInt(semesterID, 10), http.StatusOK)
	adminToken := csrf(ac, "/account/password")
	post(ac, "/akademik/tahun-ajaran", url.Values{"gorilla.csrf.Token": {adminToken}, "name": {"Denied " + name}}, http.StatusForbidden)
	post(ac, "/akademik/semester/"+strconv.FormatInt(semesterID, 10)+"/aktifkan", url.Values{"gorilla.csrf.Token": {adminToken}}, http.StatusForbidden)
	multiClasses := get(mc, "/akademik/kelas", http.StatusOK)
	if !strings.Contains(multiClasses, "Tambah kelas") || strings.Count(multiClasses, `href="/akademik/kelas"`) != 1 {
		t.Fatal("multi-role class management or sidebar incorrect")
	}
	get(mc, "/akademik/tahun-ajaran", http.StatusOK)
	get(mc, "/akademik/kelas/new", http.StatusOK)
	if _, err = db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1 AND role_code='admin'`, multi.ID); err != nil {
		t.Fatal(err)
	}
	get(mc, editPath, http.StatusForbidden)
	values.Set("gorilla.csrf.Token", csrf(mc, "/account/password"))
	post(mc, editPath, values, http.StatusForbidden)
	if strings.Contains(get(mc, "/akademik/kelas", http.StatusOK), "Tambah kelas") {
		t.Fatal("revoked Admin retained write controls in active session")
	}
	var version, count int
	var actualName string
	if err = db.QueryRow(`SELECT name,version FROM app.classes WHERE id=$1`, classID).Scan(&actualName, &version); err != nil || actualName != "Dikoreksi" || version != 2 {
		t.Fatal("forbidden requests altered class")
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.academic_history WHERE entity_type='class' AND entity_id=$1`, classID).Scan(&count); err != nil || count != 2 {
		t.Fatal("forbidden requests altered class history")
	}
}
