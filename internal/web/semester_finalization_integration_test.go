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

func TestSemesterFinalizationHTTPPreviewCSRFAndOwnerAuthorizationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "finalhttp"+testutil.Random(t)[:7])
	s := &academic.Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	var err error
	if _, err = db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentWeight(ctx, cur.ID, academic.AssessmentWeights{SemesterID: f.Semester, NH: 60, ASTS: 20, ASAS: 20}, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP Final HTTP"); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP kedua HTTP"); err != nil {
		t.Fatal(err)
	}
	var tp, tp2, activity int64
	if err = db.QueryRow(`SELECT min(id),max(id) FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&tp, &tp2); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif Final HTTP", "sumatif", "2020-01-10", []int64{tp, tp2}); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, []academic.AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: tp, Value: "80"}, {StudentID: f.StudentA, ObjectiveID: tp2, Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []academic.ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"}}); err != nil {
		t.Fatal(err)
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	csrfKey := make([]byte, 32)
	if _, err = rand.Read(csrfKey); err != nil {
		t.Fatal(err)
	}
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: csrfKey})
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	ctxValue := fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)
	path := fmt.Sprintf("/guru/rekomendasi-semester/final?context=%s&student_id=%d", url.QueryEscape(ctxValue), f.StudentA)
	request := func(method, p string, form url.Values, cookies []*http.Cookie, user auth.User) *httptest.ResponseRecorder {
		var body *strings.Reader
		if form == nil {
			body = strings.NewReader("")
		} else {
			body = strings.NewReader(form.Encode())
		}
		req := httptest.NewRequest(method, p, body)
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	page := request(http.MethodGet, path, nil, nil, owner)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Nilai Final") || !strings.Contains(page.Body.String(), "Bobot resmi Kurikulum") || !strings.Contains(page.Body.String(), "Rekomendasi: 80") || !strings.Contains(page.Body.String(), "TP kedua HTTP") {
		t.Fatalf("pratinjau Final HTTP: %d %s", page.Code, page.Body.String())
	}
	token := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	key := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	snapshot := regexp.MustCompile(`name="snapshot_token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	if len(token) != 2 || len(key) != 2 || len(snapshot) != 2 {
		t.Fatal("CSRF, kunci pengiriman, atau token snapshot tidak ditemukan")
	}
	cookies := page.Result().Cookies()
	form := url.Values{"gorilla.csrf.Token": {token[1]}, "context": {ctxValue}, "student_id": {fmt.Sprint(f.StudentA)}, "snapshot_token": {snapshot[1]}, "submission_key": {key[1]}, "final_score": {"80"}, "reason": {}}
	if got := request(http.MethodPost, path, form, nil, owner); got.Code != http.StatusForbidden {
		t.Fatalf("POST tanpa cookie CSRF: %d", got.Code)
	}
	if got := request(http.MethodGet, path, nil, nil, other); got.Code != http.StatusForbidden {
		t.Fatalf("Guru tanpa penugasan harus ditolak: %d", got.Code)
	}
	invalid := url.Values{}
	for k, values := range form {
		invalid[k] = append([]string(nil), values...)
	}
	invalid.Set("final_score", "80.5")
	invalid.Set("reason", "nilai uji")
	if got := request(http.MethodPost, path, invalid, cookies, owner); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), `value="80.5"`) || !strings.Contains(got.Body.String(), "nilai uji") {
		t.Fatalf("nilai Final harus integer dan form dipertahankan: %d %s", got.Code, got.Body.String())
	}
	invalid.Set("final_score", "81")
	invalid.Set("reason", "   ")
	if got := request(http.MethodPost, path, invalid, cookies, owner); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Final berbeda tanpa alasan harus ditolak: %d %s", got.Code, got.Body.String())
	}
	// Perubahan salah satu sumber multi-TP setelah pratinjau harus menghasilkan 409.
	var scoreID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3`, activity, tp2, f.StudentA).Scan(&scoreID); err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectAssessmentInitialScore(ctx, f.Teacher, scoreID, 0, "84", "Koreksi uji"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentWeight(ctx, cur.ID, academic.AssessmentWeights{SemesterID: f.Semester, NH: 50, ASTS: 20, ASAS: 30}, 1); err != nil {
		t.Fatal(err)
	}
	invalid.Set("final_score", "81")
	invalid.Set("reason", "nilai uji")
	if got := request(http.MethodPost, path, invalid, cookies, owner); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "Muat ulang pratinjau") {
		t.Fatalf("snapshot basi harus 409 dan meminta pratinjau baru: %d %s", got.Code, got.Body.String())
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 0 {
		t.Fatalf("validasi/konflik tidak boleh membuat Final: %d %v", count, err)
	}
	page = request(http.MethodGet, path, nil, nil, owner)
	if page.Code != http.StatusOK {
		t.Fatalf("pratinjau terbaru: %d %s", page.Code, page.Body.String())
	}
	token = regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	key = regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	snapshot = regexp.MustCompile(`name="snapshot_token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	if len(token) != 2 || len(key) != 2 || len(snapshot) != 2 {
		t.Fatal("token form terbaru tidak ditemukan")
	}
	cookies = page.Result().Cookies()
	form = url.Values{"gorilla.csrf.Token": {token[1]}, "context": {ctxValue}, "student_id": {fmt.Sprint(f.StudentA)}, "snapshot_token": {snapshot[1]}, "submission_key": {key[1]}, "final_score": {"82"}, "reason": {""}}
	if got := request(http.MethodPost, path, form, cookies, owner); got.Code != http.StatusSeeOther {
		t.Fatalf("simpan Final: %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, path, form, cookies, owner); got.Code != http.StatusSeeOther {
		t.Fatalf("retry idempoten: %d %s", got.Code, got.Body.String())
	}
	finalPage := request(http.MethodGet, path, nil, nil, owner)
	if finalPage.Code != http.StatusOK || !strings.Contains(finalPage.Body.String(), "Final · snapshot tersimpan") && !strings.Contains(finalPage.Body.String(), "Nilai Final resmi versi 1") {
		t.Fatalf("detail Final setelah redirect: %d %s", finalPage.Code, finalPage.Body.String())
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4`, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&count); err != nil || count != 1 {
		t.Fatalf("retry tidak boleh menggandakan Final: %d %v", count, err)
	}
}
