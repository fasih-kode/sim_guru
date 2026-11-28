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

func TestHistoricalAssessmentLinkHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "cp21http"+testutil.Random(t)[:8])
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
	var year, objective, activity int64
	if err := db.QueryRow(`SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&year); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1`, f.Semester).Scan(&objective); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif", "sumatif", "2020-01-10", []int64{objective}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1`, f.Semester).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("fixture"))
	exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',$8,'00000000-0000-4000-8000-000000002121',$8)`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher, hash[:])
	in := academic.HistoricalTPPreviewInput{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, Definition: "TP historis", Reason: "terlupa", AssessmentDate: "2020-01-10", DateSource: "arsip", Learning: academic.HistoricalTPReference{Kind: "manual", Document: "jurnal", Archive: "lemari", Teacher: "guru", Date: "2020-01-09"}, Assessment: academic.HistoricalTPReference{Kind: "manual", Document: "sumatif", Archive: "lemari", Teacher: "guru", ResultSource: "hasil"}}
	p, err := s.PreviewHistoricalTP(ctx, f.Teacher, in, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	tp, err := s.ConfirmHistoricalLearningObjective(ctx, f.Teacher, academic.HistoricalTPConfirmation{Preview: in, Verification: academic.HistoricalTPVerification{LearningSection: "satu", AssessmentSection: "dua", Method: "periksa", ResultSource: "hasil", Acknowledged: true}, SnapshotToken: p.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002122"})
	if err != nil {
		t.Fatal(err)
	}
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
	field := func(body, name string) string {
		t.Helper()
		m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`).FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatalf("field %s tidak ditemukan", name)
		}
		return m[1]
	}
	path := "/guru/tujuan-pembelajaran/relasi-historis"
	page := request("GET", path, nil, nil)
	if page.Code != 200 {
		t.Fatalf("GET: %d", page.Code)
	}
	vals := url.Values{"context": {fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)}, "objective": {fmt.Sprint(tp)}, "activity": {fmt.Sprint(activity)}, "reason": {"koreksi"}, "document": {"dokumen"}, "archive": {"lemari"}, "section": {"butir satu"}, "method": {"diperiksa"}, "result_source": {"lembar"}, "verified": {"yes"}, "gorilla.csrf.Token": {field(page.Body.String(), "gorilla.csrf.Token")}}
	noCSRF := request("POST", path, vals, nil)
	if noCSRF.Code != 403 {
		t.Fatalf("CSRF: %d", noCSRF.Code)
	}
	owner := user
	user = auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	if denied := request("POST", path, vals, page.Result().Cookies()); denied.Code != 403 {
		t.Fatalf("POST guru lain: %d", denied.Code)
	}
	user = owner
	preview := request("POST", path, vals, page.Result().Cookies())
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), "Pratinjau dampak") {
		t.Fatalf("preview %d", preview.Code)
	}
	vals.Set("action", "confirm")
	vals.Set("snapshot_token", field(preview.Body.String(), "snapshot_token"))
	vals.Set("submission_key", field(preview.Body.String(), "submission_key"))
	invalid := url.Values{}
	for k, v := range vals {
		invalid[k] = append([]string(nil), v...)
	}
	invalid.Set("reason", "")
	if w := request("POST", path, invalid, page.Result().Cookies()); w.Code != 422 {
		t.Fatalf("validasi: %d", w.Code)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=60,asts=20,asas=20 WHERE semester_id=$1`, f.Semester)
	if w := request("POST", path, vals, page.Result().Cookies()); w.Code != 409 {
		t.Fatalf("basi: %d", w.Code)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=0,asts=40,asas=60 WHERE semester_id=$1`, f.Semester)
	saved := request("POST", path, vals, page.Result().Cookies())
	if saved.Code != 303 {
		t.Fatalf("simpan: %d %s", saved.Code, saved.Body.String())
	}
	loc := saved.Header().Get("Location")
	if strings.Contains(loc, "koreksi") || strings.Contains(loc, "submission") {
		t.Fatal("input sensitif pada URL")
	}
	if w := request("GET", loc, nil, page.Result().Cookies()); w.Code != 200 || !strings.Contains(w.Body.String(), "Koreksi relasi tersimpan") {
		t.Fatalf("hasil: %d", w.Code)
	}
	if w := request("POST", path, vals, page.Result().Cookies()); w.Code != 303 || w.Header().Get("Location") != loc {
		t.Fatalf("replay: %d", w.Code)
	}
	changed := url.Values{}
	for k, v := range vals {
		changed[k] = append([]string(nil), v...)
	}
	changed.Set("method", "berbeda")
	if w := request("POST", path, changed, page.Result().Cookies()); w.Code != 409 {
		t.Fatalf("replay berbeda: %d", w.Code)
	}
	user = auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	if w := request("GET", loc, nil, page.Result().Cookies()); w.Code != 403 {
		t.Fatalf("guru lain: %d", w.Code)
	}
	user = owner
}
