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

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestSemesterFinalExceptionHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "exhttp"+testutil.Random(t)[:7])
	s := &academic.Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, e := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAssessmentWeight(ctx, cur.ID, academic.AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []academic.ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "0"}}); e != nil {
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
		h.ServeHTTP(w, req)
		return w
	}
	cv := fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)
	list := request("GET", "/guru/rekomendasi-semester?context="+url.QueryEscape(cv), nil, nil, owner)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Tinjau Final dengan Pengecualian") {
		t.Fatalf("aksi pengecualian tidak tampil: %d %s", list.Code, list.Body.String())
	}
	path := fmt.Sprintf("/guru/rekomendasi-semester/final?context=%s&student_id=%d", url.QueryEscape(cv), f.StudentA)
	page := request("GET", path, nil, nil, owner)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Bukti Sumatif sah berbobot positif") || !strings.Contains(page.Body.String(), "Rekomendasi otomatis tidak tersedia") || !strings.Contains(page.Body.String(), "ASTS: 0,00") {
		t.Fatalf("preview %d %s", page.Code, page.Body.String())
	}
	get := func(name string) string {
		m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
		if len(m) != 2 {
			t.Fatalf("field %s tidak ditemukan", name)
		}
		return m[1]
	}
	form := url.Values{"context": {cv}, "student_id": {fmt.Sprint(f.StudentA)}, "snapshot_token": {get("snapshot_token")}, "submission_key": {get("submission_key")}, "gorilla.csrf.Token": {get("gorilla.csrf.Token")}, "mode": {"exception"}, "final_score": {"0"}, "reason": {"Alasan uji"}}
	if w := request("POST", path, form, nil, owner); w.Code != http.StatusForbidden {
		t.Fatalf("csrf %d", w.Code)
	}
	if w := request("GET", path, nil, nil, other); w.Code != http.StatusForbidden {
		t.Fatalf("guru lain %d", w.Code)
	}
	form.Set("reason", " ")
	if w := request("POST", path, form, page.Result().Cookies(), owner); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("alasan kosong %d", w.Code)
	}
	form.Set("reason", "Alasan uji")
	form.Set("final_score", "0.5")
	if w := request("POST", path, form, page.Result().Cookies(), owner); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("desimal %d", w.Code)
	}
	var source int64
	if e = db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE semester_id=$1 AND student_id=$2 AND component='asts'`, f.Semester, f.StudentA).Scan(&source); e != nil {
		t.Fatal(e)
	}
	if e = s.CorrectExamInitialScore(ctx, f.Teacher, source, 0, "10", "Koreksi setelah pratinjau"); e != nil {
		t.Fatal(e)
	}
	form.Set("final_score", "0")
	if w := request("POST", path, form, page.Result().Cookies(), owner); w.Code != http.StatusConflict {
		t.Fatalf("snapshot basi: %d %s", w.Code, w.Body.String())
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); e != nil || count != 0 {
		t.Fatalf("Final parsial: %d %v", count, e)
	}
	page = request("GET", path, nil, nil, owner)
	if page.Code != http.StatusOK {
		t.Fatalf("pratinjau baru: %d", page.Code)
	}
	form.Set("gorilla.csrf.Token", get("gorilla.csrf.Token"))
	form.Set("submission_key", get("submission_key"))
	form.Set("snapshot_token", get("snapshot_token"))
	if w := request("POST", path, form, page.Result().Cookies(), owner); w.Code != http.StatusSeeOther {
		t.Fatalf("simpan %d %s", w.Code, w.Body.String())
	}
	page = request("GET", path, nil, nil, owner)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Dengan Pengecualian") || !strings.Contains(page.Body.String(), "Riwayat dan snapshot") || !strings.Contains(page.Body.String(), "Alasan uji") {
		t.Fatalf("historis %d %s", page.Code, page.Body.String())
	}
	if e = s.CorrectExamInitialScore(ctx, f.Teacher, source, 1, "20", "Koreksi setelah Final"); e != nil {
		t.Fatal(e)
	}
	page = request("GET", path, nil, nil, owner)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Dengan Pengecualian · Perlu Ditinjau") || !strings.Contains(page.Body.String(), "ASTS: 10,00") {
		t.Fatalf("histori dan indikator: %d %s", page.Code, page.Body.String())
	}
}
