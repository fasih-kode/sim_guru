//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

// serializeGradebook meniru successful controls pada formulir HTML yang benar-benar dirender.
func serializeGradebook(t *testing.T, body string) url.Values {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{}
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inForm bool) {
		if n.Type == html.ElementNode && n.Data == "form" {
			inForm = false
			for _, a := range n.Attr {
				if a.Key == "data-gradebook-form" {
					inForm = true
				}
			}
		}
		if inForm && n.Type == html.ElementNode && n.Data == "input" {
			name, value, disabled := "", "", false
			for _, a := range n.Attr {
				switch a.Key {
				case "name":
					name = a.Val
				case "value":
					value = a.Val
				case "disabled":
					disabled = true
				}
			}
			if name != "" && !disabled {
				values.Add(name, value)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inForm)
		}
	}
	walk(doc, false)
	return values
}

func TestDirectTPGradebookHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "book"+testutil.Random(t)[:8])
	s := &academic.Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP uji tabel"); err != nil {
		t.Fatal(err)
	}
	var objective, year int64
	if err := db.QueryRow(`SELECT id,school_year_id FROM app.learning_objectives WHERE semester_id=$1`, f.Semester).Scan(&objective, &year); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 36; i++ {
		var student int64
		if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, fmt.Sprintf("Tabel siswa %02d-%d", i, f.Semester)).Scan(&student); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01')`, year, student, f.Class, f.Teacher); err != nil {
			t.Fatal(err)
		}
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	csrfKey := make([]byte, 32)
	if _, err = rand.Read(csrfKey); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: csrfKey})
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	request := func(method, path string, form url.Values, cookies []*http.Cookie, user auth.User) *httptest.ResponseRecorder {
		body := ""
		if form != nil {
			body = form.Encode()
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	contextValue := fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)
	startPage := request("GET", "/guru/buku-nilai", nil, nil, owner)
	if startPage.Code != http.StatusOK || !strings.Contains(startPage.Body.String(), "Pilih konteks penugasan") {
		t.Fatalf("halaman awal: %d", startPage.Code)
	}
	if !strings.Contains(startPage.Body.String(), `hx-get="/guru/buku-nilai"`) {
		t.Fatal("pemilihan konteks awal harus mendukung HTMX")
	}
	path := "/guru/buku-nilai?" + url.Values{"context": {contextValue}}.Encode()
	page := request("GET", path, nil, nil, owner)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "TP1") || strings.Count(page.Body.String(), `scope="row"`) != 39 {
		t.Fatalf("tabel 36 siswa: %d count=%d", page.Code, strings.Count(page.Body.String(), `scope="row"`))
	}
	if !strings.Contains(page.Body.String(), "Penempatan perlu diverifikasi") {
		t.Fatal("siswa pindahan harus ditandai")
	}
	hxReq := httptest.NewRequest(http.MethodGet, path, nil)
	hxReq.Header.Set("HX-Request", "true")
	hxReq = hxReq.WithContext(context.WithValue(hxReq.Context(), principalKey{}, &owner))
	hxResult := httptest.NewRecorder()
	handler.ServeHTTP(hxResult, hxReq)
	if hxResult.Code != http.StatusOK || !strings.Contains(hxResult.Body.String(), `id="content"`) || !strings.Contains(hxResult.Body.String(), "TP1") {
		t.Fatalf("konten HTMX: %d", hxResult.Code)
	}
	if w := request("GET", path, nil, nil, other); w.Code != http.StatusForbidden {
		t.Fatalf("guru lain: %d", w.Code)
	}
	field := func(name string) string {
		match := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
		if len(match) != 2 {
			t.Fatalf("field %s tidak tersedia", name)
		}
		return match[1]
	}
	csrf, submission := field("gorilla.csrf.Token"), field("submission_key")
	a := fmt.Sprintf("%d_%d", f.StudentA, objective)
	b := fmt.Sprintf("%d_%d", f.StudentB, objective)
	form := url.Values{"context": {contextValue}, "submission_key": {submission}, "gorilla.csrf.Token": {csrf}, "score_" + a: {"0"}, "version_" + a: {"-1"}, "original_" + a: {""}, "score_" + b: {"100,01"}, "version_" + b: {"-1"}, "original_" + b: {""}}
	if w := request("POST", "/guru/buku-nilai", form, nil, owner); w.Code != http.StatusForbidden {
		t.Fatalf("CSRF: %d", w.Code)
	}
	w := request("POST", "/guru/buku-nilai", form, page.Result().Cookies(), owner)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "100,01") || !strings.Contains(w.Body.String(), "aria-invalid") {
		t.Fatalf("validasi & draft %d", w.Code)
	}
	form.Set("score_"+b, "82,50")
	w = request("POST", "/guru/buku-nilai", form, page.Result().Cookies(), owner)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("simpan %d %s", w.Code, w.Body.String())
	}
	confirmation := request("GET", w.Header().Get("Location"), nil, nil, owner)
	if confirmation.Code != http.StatusOK || !strings.Contains(confirmation.Body.String(), "Nilai berhasil disimpan.") {
		t.Fatalf("konfirmasi simpan: %d", confirmation.Code)
	}
	fake := request("GET", path+"&saved=00000000-0000-4000-8000-000000000001", nil, nil, owner)
	if strings.Contains(fake.Body.String(), "Nilai berhasil disimpan.") {
		t.Fatal("konfirmasi palsu")
	}
	if w = request("POST", "/guru/buku-nilai", form, page.Result().Cookies(), owner); w.Code != http.StatusSeeOther {
		t.Fatalf("replay %d", w.Code)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.direct_tp_initial_scores WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 2 {
		t.Fatalf("replay count=%d %v", count, err)
	}
	form.Set("score_"+b, "100")
	w = request("POST", "/guru/buku-nilai", form, page.Result().Cookies(), owner)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `value="100"`) {
		t.Fatalf("konflik & draft %d", w.Code)
	}
	page = request("GET", path, nil, nil, owner)
	form.Set("gorilla.csrf.Token", field("gorilla.csrf.Token"))
	form.Set("submission_key", field("submission_key"))
	form.Del("score_" + a)
	form.Del("version_" + a)
	form.Set("version_"+b, "0")
	form.Set("original_"+b, "82.50")
	w = request("POST", "/guru/buku-nilai", form, page.Result().Cookies(), owner)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("koreksi pra-Final: %d %s", w.Code, w.Body.String())
	}
	var original, corrected string
	if err = db.QueryRow(`SELECT s.score::text,c.after_score::text FROM app.direct_tp_initial_scores s JOIN app.direct_tp_score_corrections c ON c.initial_score_id=s.id WHERE s.semester_id=$1 AND s.student_id=$2`, f.Semester, f.StudentB).Scan(&original, &corrected); err != nil || original != "82.50" || corrected != "100.00" {
		t.Fatalf("histori nilai: %s %s %v", original, corrected, err)
	}
	// Dua tab: simpan tab lain lebih dahulu, lalu draft dua sel harus tetap utuh.
	page = request("GET", path, nil, nil, owner)
	staleKey, staleCSRF := field("submission_key"), field("gorilla.csrf.Token")
	page = request("GET", path, nil, nil, owner)
	otherForm := url.Values{"context": {contextValue}, "submission_key": {field("submission_key")}, "gorilla.csrf.Token": {field("gorilla.csrf.Token")}, "score_" + b: {"95"}, "version_" + b: {"1"}, "original_" + b: {"100.00"}}
	if w = request("POST", "/guru/buku-nilai", otherForm, page.Result().Cookies(), owner); w.Code != http.StatusSeeOther {
		t.Fatalf("simpan tab lain: %d %s", w.Code, w.Body.String())
	}
	page = request("GET", path, nil, nil, owner)
	staleForm := url.Values{"context": {contextValue}, "submission_key": {staleKey}, "gorilla.csrf.Token": {staleCSRF}, "score_" + a: {"15"}, "version_" + a: {"0"}, "original_" + a: {"0.00"}, "score_" + b: {"90"}, "version_" + b: {"1"}, "original_" + b: {"100.00"}}
	// Token dan cookie harus berasal dari halaman yang sama.
	staleForm.Set("gorilla.csrf.Token", field("gorilla.csrf.Token"))
	w = request("POST", "/guru/buku-nilai", staleForm, page.Result().Cookies(), owner)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "tersimpan 95") || !strings.Contains(w.Body.String(), "draft Anda 90") || !strings.Contains(w.Body.String(), `value="15"`) {
		t.Fatalf("konflik dua tab dan draft: %d %s", w.Code, w.Body.String())
	}
	var corrections int
	if err = db.QueryRow(`SELECT count(*) FROM app.direct_tp_score_corrections c JOIN app.direct_tp_initial_scores s ON s.id=c.initial_score_id WHERE s.semester_id=$1`, f.Semester).Scan(&corrections); err != nil || corrections != 2 {
		t.Fatalf("konflik tidak boleh menulis sebagian: %d %v", corrections, err)
	}
	staleForm.Set("action", "review_latest")
	staleCookies := page.Result().Cookies()
	w = request("POST", "/guru/buku-nilai", staleForm, staleCookies, owner)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Draft belum disimpan") || !strings.Contains(w.Body.String(), `value="15"`) || !strings.Contains(w.Body.String(), `value="90"`) {
		t.Fatalf("tinjau versi terbaru: %d %s", w.Code, w.Body.String())
	}
	page = w
	newKey := field("submission_key")
	if newKey == staleKey {
		t.Fatal("tinjau versi terbaru harus memakai UUID baru")
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.direct_tp_score_corrections c JOIN app.direct_tp_initial_scores s ON s.id=c.initial_score_id WHERE s.semester_id=$1`, f.Semester).Scan(&corrections); err != nil || corrections != 2 {
		t.Fatalf("tinjau tidak boleh menulis: %d %v", corrections, err)
	}
	staleForm.Del("action")
	staleForm.Set("submission_key", newKey)
	staleForm.Set("version_"+b, "2")
	staleForm.Set("original_"+b, "95.00")
	staleForm.Set("gorilla.csrf.Token", field("gorilla.csrf.Token"))
	if w = request("POST", "/guru/buku-nilai", staleForm, staleCookies, owner); w.Code != http.StatusSeeOther {
		t.Fatalf("simpan setelah keputusan eksplisit: %d %s", w.Code, w.Body.String())
	}
	var beforeB, afterB string
	if err = db.QueryRow(`SELECT c.before_score::text,c.after_score::text FROM app.direct_tp_score_corrections c JOIN app.direct_tp_initial_scores s ON s.id=c.initial_score_id WHERE s.semester_id=$1 AND s.student_id=$2 ORDER BY c.version DESC LIMIT 1`, f.Semester, f.StudentB).Scan(&beforeB, &afterB); err != nil || beforeB != "95.00" || afterB != "90.00" {
		t.Fatalf("histori sel konflik: %s -> %s %v", beforeB, afterB, err)
	}
	// Serialisasi HTML lengkap seperti submit tanpa JavaScript: hanya A diubah,
	// sementara tab lain mengubah B. Nilai B lama tidak boleh menjadi koreksi.
	first := request("GET", path, nil, nil, owner)
	second := request("GET", path, nil, nil, owner)
	firstForm := serializeGradebook(t, first.Body.String())
	secondForm := serializeGradebook(t, second.Body.String())
	noChange := serializeGradebook(t, first.Body.String())
	if w = request("POST", "/guru/buku-nilai", noChange, first.Result().Cookies(), owner); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Tidak ada perubahan") {
		t.Fatalf("form tanpa perubahan tidak boleh menulis: %d", w.Code)
	}
	firstForm.Set("score_"+a, "90")
	secondForm.Set("score_"+b, "85")
	if w = request("POST", "/guru/buku-nilai", secondForm, second.Result().Cookies(), owner); w.Code != http.StatusSeeOther {
		t.Fatalf("tab B: %d %s", w.Code, w.Body.String())
	}
	var countBeforeB, countAfterB int
	if err = db.QueryRow(`SELECT count(*) FROM app.direct_tp_score_corrections c JOIN app.direct_tp_initial_scores s ON s.id=c.initial_score_id WHERE s.semester_id=$1 AND s.student_id=$2`, f.Semester, f.StudentB).Scan(&countBeforeB); err != nil {
		t.Fatal(err)
	}
	w = request("POST", "/guru/buku-nilai", firstForm, first.Result().Cookies(), owner)
	if w.Code != http.StatusConflict {
		t.Fatalf("tab A harus konflik: %d %s", w.Code, w.Body.String())
	}
	conflictForm := serializeGradebook(t, w.Body.String())
	if conflictForm.Get("score_"+a) != "90" || conflictForm.Get("score_"+b) != "85.00" {
		t.Fatalf("draft A / B terbaru: %#v", conflictForm)
	}
	conflictForm.Set("action", "review_latest")
	w = request("POST", "/guru/buku-nilai", conflictForm, first.Result().Cookies(), owner)
	if w.Code != http.StatusOK {
		t.Fatalf("tinjau A: %d %s", w.Code, w.Body.String())
	}
	reviewed := serializeGradebook(t, w.Body.String())
	if reviewed.Get("submission_key") == firstForm.Get("submission_key") || reviewed.Get("score_"+b) != "85.00" {
		t.Fatal("UUID baru dan B terbaru wajib dipertahankan")
	}
	if w = request("POST", "/guru/buku-nilai", reviewed, first.Result().Cookies(), owner); w.Code != http.StatusSeeOther {
		t.Fatalf("simpan A setelah tinjau: %d %s", w.Code, w.Body.String())
	}
	var latestB string
	if err = db.QueryRow(`SELECT c.after_score::text FROM app.direct_tp_score_corrections c JOIN app.direct_tp_initial_scores s ON s.id=c.initial_score_id WHERE s.semester_id=$1 AND s.student_id=$2 ORDER BY c.version DESC LIMIT 1`, f.Semester, f.StudentB).Scan(&latestB); err != nil || latestB != "85.00" {
		t.Fatalf("B tidak boleh ditimpa: %q %v", latestB, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.direct_tp_score_corrections c JOIN app.direct_tp_initial_scores s ON s.id=c.initial_score_id WHERE s.semester_id=$1 AND s.student_id=$2`, f.Semester, f.StudentB).Scan(&countAfterB); err != nil || countAfterB != countBeforeB {
		t.Fatalf("A tidak boleh membuat koreksi B: %d -> %d %v", countBeforeB, countAfterB, err)
	}
	page = request("GET", path, nil, nil, owner)
	moved := fmt.Sprintf("%d_%d", f.MovedStudent, objective)
	form = url.Values{"context": {contextValue}, "submission_key": {field("submission_key")}, "gorilla.csrf.Token": {field("gorilla.csrf.Token")}, "score_" + moved: {"70"}, "version_" + moved: {"-1"}, "original_" + moved: {""}}
	if w = request("POST", "/guru/buku-nilai", form, page.Result().Cookies(), owner); w.Code != http.StatusForbidden {
		t.Fatalf("penempatan pindah: %d", w.Code)
	}
	_, err = db.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',decode(repeat('ab',32),'hex'),gen_random_uuid(),decode(repeat('cd',32),'hex'))`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher)
	if err != nil {
		t.Fatal(err)
	}
	page = request("GET", path, nil, nil, owner)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Siswa sudah Final") {
		t.Fatalf("final readonly: %d", page.Code)
	}
	form.Set("gorilla.csrf.Token", field("gorilla.csrf.Token"))
	form.Set("submission_key", field("submission_key"))
	form.Set("score_"+a, "100")
	form.Set("version_"+a, "0")
	form.Set("original_"+a, "15.00")
	form.Del("score_" + moved)
	form.Del("version_" + moved)
	form.Del("score_" + b)
	form.Del("version_" + b)
	w = request("POST", "/guru/buku-nilai", form, page.Result().Cookies(), owner)
	if w.Code != http.StatusConflict {
		t.Fatalf("POST siswa Final: %d", w.Code)
	}
	var snapshot string
	if err = db.QueryRow(`SELECT snapshot::text FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&snapshot); err != nil || snapshot != "{}" {
		t.Fatalf("snapshot berubah: %q %v", snapshot, err)
	}
}
