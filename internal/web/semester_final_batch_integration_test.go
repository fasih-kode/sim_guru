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

func TestSemesterFinalBatchHTTPPreviewCSRFIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "batchhttp"+testutil.Random(t)[:7])
	s := &academic.Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentWeight(ctx, cur.ID, academic.AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []academic.ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "0"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "100"}, {StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "0"}, {StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "100"}}); err != nil {
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
	path := "/guru/rekomendasi-semester/massal?" + url.Values{"context": {cv}, "student_id": {fmt.Sprint(f.StudentA), fmt.Sprint(f.StudentB)}}.Encode()
	page := request("GET", path, nil, nil, owner)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Tetapkan Final untuk 2 Siswa") {
		t.Fatalf("preview %d %s", page.Code, page.Body.String())
	}
	token := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	submission := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	snap := regexp.MustCompile(`name="snapshot_token" value="([^"]+)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(token) != 2 || len(submission) != 2 || len(snap) != 2 {
		t.Fatal("token pratinjau tidak tersedia")
	}
	form := url.Values{"context": {cv}, "student_id": {fmt.Sprint(f.StudentA), fmt.Sprint(f.StudentB)}, "snapshot_token": {snap[0][1], snap[1][1]}, "submission_key": {submission[1]}, "gorilla.csrf.Token": {token[1]}}
	if w := request("POST", path, form, nil, owner); w.Code != http.StatusForbidden {
		t.Fatalf("CSRF: %d", w.Code)
	}
	if w := request("GET", path, nil, nil, other); w.Code != http.StatusForbidden {
		t.Fatalf("akses lain: %d", w.Code)
	}
	if w := request("POST", path, form, page.Result().Cookies(), other); w.Code != http.StatusForbidden {
		t.Fatalf("POST guru lain: %d %s", w.Code, w.Body.String())
	}
	// Koreksi resmi setelah pratinjau membatalkan seluruh batch, bukan hanya
	// siswa yang nilainya berubah.
	var scoreID int64
	if err = db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE semester_id=$1 AND student_id=$2 AND component='asas'`, f.Semester, f.StudentB).Scan(&scoreID); err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, scoreID, 0, "90", "Koreksi uji"); err != nil {
		t.Fatal(err)
	}
	if w := request("POST", path, form, page.Result().Cookies(), owner); w.Code != http.StatusConflict {
		t.Fatalf("pratinjau lama: %d %s", w.Code, w.Body.String())
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Final parsial: %d %v", count, err)
	}
	page = request("GET", path, nil, nil, owner)
	if page.Code != http.StatusOK {
		t.Fatalf("pratinjau terbaru: %d %s", page.Code, page.Body.String())
	}
	token = regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	submission = regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	snap = regexp.MustCompile(`name="snapshot_token" value="([^"]+)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(token) != 2 || len(submission) != 2 || len(snap) != 2 {
		t.Fatal("token pratinjau terbaru tidak tersedia")
	}
	form.Set("gorilla.csrf.Token", token[1])
	form.Set("submission_key", submission[1])
	form["snapshot_token"] = []string{snap[0][1], snap[1][1]}
	if w := request("POST", path, form, page.Result().Cookies(), owner); w.Code != http.StatusSeeOther {
		t.Fatalf("simpan: %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", path, form, page.Result().Cookies(), owner); w.Code != http.StatusSeeOther {
		t.Fatalf("ulangi: %d %s", w.Code, w.Body.String())
	}
}
