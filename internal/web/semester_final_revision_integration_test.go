//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestSemesterFinalRevisionHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "revhttp"+testutil.Random(t)[:6])
	s := &academic.Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, e := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAssessmentWeight(ctx, cur.ID, academic.AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []academic.ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "80"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "80"}}); e != nil {
		t.Fatal(e)
	}
	p, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000931"); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE app.teaching_assignments SET effective_until='2020-02-01' WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND teacher_id=$4`, f.Semester, f.Class, f.Subject, f.TeacherID); e != nil {
		t.Fatal(e)
	}
	accounts, e := auth.New(db)
	if e != nil {
		t.Fatal(e)
	}
	key := make([]byte, 32)
	if _, e = rand.Read(key); e != nil {
		t.Fatal(e)
	}
	h, e := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	if e != nil {
		t.Fatal(e)
	}
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	req := func(method, path string, form url.Values, cookies []*http.Cookie, user auth.User) *httptest.ResponseRecorder {
		body := ""
		if form != nil {
			body = form.Encode()
		}
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, c := range cookies {
			r.AddCookie(c)
		}
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	cv := fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)
	path := fmt.Sprintf("/guru/rekomendasi-semester/final?context=%s&student_id=%d&history=1", url.QueryEscape(cv), f.StudentA)
	list := req("GET", "/guru/rekomendasi-semester", nil, nil, owner)
	if list.Code != 200 || !strings.Contains(list.Body.String(), "Final Milik Saya") {
		t.Fatalf("daftar historis %d", list.Code)
	}
	page := req("GET", path, nil, nil, owner)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Koreksi / Konfirmasi Ulang Final") {
		t.Fatalf("pratinjau %d %s", page.Code, page.Body.String())
	}
	if w := req("GET", path, nil, nil, other); w.Code != 403 {
		t.Fatalf("akses guru lain %d", w.Code)
	}
	for _, query := range []string{"&saved=1", "&result=revision&submission=00000000-0000-4000-8000-000000000931", "&result=retain&submission=00000000-0000-4000-8000-000000000932"} {
		w := req("GET", path+query, nil, nil, owner)
		if w.Code != 200 || strings.Contains(w.Body.String(), "berhasil disimpan") || strings.Contains(w.Body.String(), "berhasil dicatat") {
			t.Fatalf("notifikasi palsu: %d %s", w.Code, w.Body.String())
		}
	}
	field := func(n string) string {
		m := regexp.MustCompile(`name="` + regexp.QuoteMeta(n) + `" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
		if len(m) != 2 {
			t.Fatalf("field %s", n)
		}
		return m[1]
	}
	form := url.Values{"history": {"1"}, "context": {cv}, "student_id": {fmt.Sprint(f.StudentA)}, "expected_version": {"1"}, "snapshot_token": {field("snapshot_token")}, "submission_key": {field("submission_key")}, "gorilla.csrf.Token": {field("gorilla.csrf.Token")}, "action": {"revision"}, "final_score": {"85"}, "reason": {"Perbaikan"}}
	if w := req("POST", path, form, nil, owner); w.Code != 403 {
		t.Fatalf("csrf %d", w.Code)
	}
	if w := req("POST", path, form, page.Result().Cookies(), other); w.Code != 403 {
		t.Fatalf("owner %d", w.Code)
	}
	form.Set("reason", " ")
	if w := req("POST", path, form, page.Result().Cookies(), owner); w.Code != 422 || !strings.Contains(w.Body.String(), `app-notification--error" data-notification role="alert"`) {
		t.Fatalf("alasan kosong %d %s", w.Code, w.Body.String())
	}
	form.Set("reason", "Perbaikan")
	revision := req("POST", path, form, page.Result().Cookies(), owner)
	if revision.Code != 303 {
		t.Fatalf("simpan %d %s", revision.Code, revision.Body.String())
	}
	if w := req("GET", revision.Header().Get("Location"), nil, nil, owner); w.Code != 200 || !strings.Contains(w.Body.String(), "Nilai Final versi baru berhasil disimpan.") || !strings.Contains(w.Body.String(), `app-notification--success`) || strings.Contains(w.Body.String(), "Keputusan mempertahankan Nilai Final lama berhasil dicatat.") {
		t.Fatalf("notifikasi revisi %d %s", w.Code, w.Body.String())
	}
	if w := req("GET", revision.Header().Get("Location"), nil, nil, other); w.Code != 403 {
		t.Fatalf("hasil guru lain %d", w.Code)
	}
	if w := req("POST", path, form, page.Result().Cookies(), owner); w.Code != 303 {
		t.Fatalf("replay revisi %d", w.Code)
	}
	if w := req("GET", path+"&result=retain&submission="+url.QueryEscape(form.Get("submission_key")), nil, nil, owner); strings.Contains(w.Body.String(), "berhasil dicatat") {
		t.Fatal("kunci revisi dianggap audit mempertahankan")
	}
	form.Set("submission_key", "00000000-0000-4000-8000-000000000932")
	if w := req("POST", path, form, page.Result().Cookies(), owner); w.Code != 409 {
		t.Fatalf("versi basi %d %s", w.Code, w.Body.String())
	}
	page = req("GET", path, nil, nil, owner)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Versi 2") || !strings.Contains(page.Body.String(), "Versi 1") {
		t.Fatalf("histori %d %s", page.Code, page.Body.String())
	}
	form.Set("expected_version", "2")
	form.Set("snapshot_token", field("snapshot_token"))
	form.Set("submission_key", field("submission_key"))
	form.Set("gorilla.csrf.Token", field("gorilla.csrf.Token"))
	if e = s.SaveAssessmentWeight(ctx, cur.ID, academic.AssessmentWeights{SemesterID: f.Semester, NH: 50, ASTS: 20, ASAS: 30}, 1); e != nil {
		t.Fatal(e)
	}
	if w := req("POST", path, form, page.Result().Cookies(), owner); w.Code != 409 {
		t.Fatalf("snapshot basi %d %s", w.Code, w.Body.String())
	}
	page = req("GET", path, nil, nil, owner)
	form.Set("snapshot_token", field("snapshot_token"))
	form.Set("submission_key", field("submission_key"))
	form.Set("gorilla.csrf.Token", field("gorilla.csrf.Token"))
	form.Set("action", "retain")
	form.Set("reason", "Pertahankan setelah tinjau")
	retained := req("POST", path, form, page.Result().Cookies(), owner)
	if retained.Code != 303 {
		t.Fatalf("pertahankan %d %s", retained.Code, retained.Body.String())
	}
	if w := req("GET", retained.Header().Get("Location"), nil, nil, owner); w.Code != 200 || !strings.Contains(w.Body.String(), "Keputusan mempertahankan Nilai Final lama berhasil dicatat.") || !strings.Contains(w.Body.String(), `app-notification--success`) || strings.Contains(w.Body.String(), `app-notification--error" data-notification role="alert"`) || !strings.Contains(w.Body.String(), "Perlu Ditinjau") || strings.Contains(w.Body.String(), "Nilai Final versi baru berhasil disimpan.") {
		t.Fatalf("notifikasi pertahankan %d %s", w.Code, w.Body.String())
	}
	if w := req("POST", path, form, page.Result().Cookies(), owner); w.Code != 303 {
		t.Fatalf("replay pertahankan %d", w.Code)
	}
	var versions, audits, score int
	if e = db.QueryRow(`SELECT (SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2),(SELECT count(*) FROM app.semester_final_retentions r JOIN app.semester_final_versions v ON v.id=r.final_version_id WHERE v.semester_id=$1 AND v.student_id=$2),(SELECT final_score FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 ORDER BY version DESC LIMIT 1)`, f.Semester, f.StudentA).Scan(&versions, &audits, &score); e != nil || versions != 2 || audits != 1 || score != 85 {
		t.Fatalf("retensi mengubah Final: versions=%d audits=%d score=%d err=%v", versions, audits, score, e)
	}
}
