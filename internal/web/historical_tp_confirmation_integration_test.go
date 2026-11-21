//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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

func TestHistoricalTPConfirmationHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "cp20http"+testutil.Random(t)[:8])
	s := &academic.Store{DB: db}
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester)
	exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,actor_id) VALUES($1,0,40,60,$2)`, f.Semester, f.Teacher)
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP awal"); err != nil {
		t.Fatal(err)
	}
	var year int64
	if err := db.QueryRow(`SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&year); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("fixture"))
	exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',$8,'00000000-0000-4000-8000-000000002056',$8)`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher, hash[:])
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	user := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	request := func(method, path string, values url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
		body := ""
		if values != nil {
			body = values.Encode()
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
	path := "/guru/tujuan-pembelajaran/historis"
	page := request("GET", path, nil, nil)
	if page.Code != 200 {
		t.Fatalf("GET %d", page.Code)
	}
	field := func(body, name string) string {
		t.Helper()
		m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`).FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatalf("field %s tidak ditemukan", name)
		}
		return m[1]
	}
	vals := url.Values{"context": {fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)}, "definition": {"TP historis"}, "reason": {"terlupa"}, "assessment_date": {"2020-01-10"}, "date_source": {"arsip"}, "learning_kind": {"manual"}, "learning_document": {"jurnal"}, "learning_archive": {"lemari"}, "learning_teacher": {"guru"}, "learning_date": {"2020-01-09"}, "assessment_kind": {"manual"}, "assessment_document": {"sumatif"}, "assessment_archive": {"lemari"}, "assessment_teacher": {"guru"}, "result_source": {"lembar nilai"}, "gorilla.csrf.Token": {field(page.Body.String(), "gorilla.csrf.Token")}}
	preview := request("POST", path, vals, page.Result().Cookies())
	if preview.Code != 200 {
		t.Fatalf("preview %d", preview.Code)
	}
	vals.Set("action", "confirm")
	vals.Set("snapshot_token", field(preview.Body.String(), "snapshot_token"))
	vals.Set("submission_key", field(preview.Body.String(), "submission_key"))
	vals.Set("learning_section", "halaman 1")
	vals.Set("assessment_section", "halaman 2")
	vals.Set("verification_method", "diperiksa")
	vals.Set("verified_result_source", "lembar hasil")
	vals.Set("verified", "yes")
	saved := request("POST", path, vals, page.Result().Cookies())
	if saved.Code != 303 {
		t.Fatalf("simpan HTTP %d: %s", saved.Code, saved.Body.String())
	}
	loc := saved.Header().Get("Location")
	if strings.Contains(loc, "terlupa") || strings.Contains(loc, "submission") {
		t.Fatal("input sensitif di URL")
	}
	result := request("GET", loc, nil, page.Result().Cookies())
	if result.Code != 200 || !strings.Contains(result.Body.String(), "TP historis tersimpan") {
		t.Fatalf("hasil %d", result.Code)
	}
	owner := user
	user = auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	if denied := request("GET", loc, nil, page.Result().Cookies()); denied.Code != http.StatusForbidden {
		t.Fatalf("hasil guru lain: %d", denied.Code)
	}
	user = owner
	replay := request("POST", path, vals, page.Result().Cookies())
	if replay.Code != 303 || replay.Header().Get("Location") != loc {
		t.Fatalf("replay %d", replay.Code)
	}
	vals.Set("verification_method", "berbeda")
	conflict := request("POST", path, vals, page.Result().Cookies())
	if conflict.Code != 409 {
		t.Fatalf("replay berbeda %d", conflict.Code)
	}
}
