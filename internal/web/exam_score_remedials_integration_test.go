//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"fmt"
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

func TestExamRemedialHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "remweb"+testutil.Random(t)[:8])
	ctx := context.Background()
	s := &academic.Store{DB: db}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []academic.ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "65"}}); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE class_id=$1 AND student_id=$2 AND component='asts'`, f.Class, f.StudentA).Scan(&id); err != nil {
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
	path := fmt.Sprintf("/guru/nilai-ujian/%d/remedial", id)
	get := func(user auth.User, p string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", p, nil)
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	correction := get(owner, fmt.Sprintf("/guru/nilai-ujian/%d/koreksi", id))
	if correction.Code != 200 || !strings.Contains(correction.Body.String(), path) {
		t.Fatalf("link remedial: %d", correction.Code)
	}
	page := get(owner, path)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "65.00") || !strings.Contains(page.Body.String(), "Riwayat Remedial") {
		t.Fatalf("detail %d: %s", page.Code, page.Body.String())
	}
	token := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	submission := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	if len(token) != 2 || len(submission) != 2 {
		t.Fatal("token form hilang")
	}
	cookies := page.Result().Cookies()
	form := url.Values{"gorilla.csrf.Token": {token[1]}, "submission_key": {submission[1]}, "value": {"75"}, "remedial_date": {"2020-01-10"}}
	post := func(user auth.User, withCookie bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if withCookie {
			for _, c := range cookies {
				req.AddCookie(c)
			}
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := post(owner, false); w.Code != 403 {
		t.Fatalf("CSRF %d", w.Code)
	}
	if w := post(other, true); w.Code != 403 {
		t.Fatalf("guru lain %d", w.Code)
	}
	if w := get(other, path); w.Code != 404 {
		t.Fatalf("baca guru lain %d", w.Code)
	}
	form.Set("value", "100,01")
	if w := post(owner, true); w.Code != 422 || !strings.Contains(w.Body.String(), "100,01") {
		t.Fatalf("validasi %d", w.Code)
	}
	form.Set("value", "75")
	if w := post(owner, true); w.Code != 303 {
		t.Fatalf("POST %d %s", w.Code, w.Body.String())
	}
	if w := post(owner, true); w.Code != 303 {
		t.Fatalf("replay %d", w.Code)
	}
	page = get(owner, path)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "75.00") {
		t.Fatalf("nilai efektif %d", page.Code)
	}
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET effective_until=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date WHERE semester_id=$1 AND class_id=$2 AND teacher_id=$3`, f.Semester, f.Class, f.TeacherID); err != nil {
		t.Fatal(err)
	}
	page = get(owner, path)
	if page.Code != 200 || strings.Contains(page.Body.String(), `name="submission_key"`) || !strings.Contains(page.Body.String(), "Penugasan aktif diperlukan") {
		t.Fatalf("histori tanpa hak mutasi %d", page.Code)
	}
	correction = get(owner, fmt.Sprintf("/guru/nilai-ujian/%d/koreksi", id))
	if correction.Code != 200 || !strings.Contains(correction.Body.String(), "Riwayat Remedial") {
		t.Fatal("akses histori tidak tersedia")
	}
	if w := post(owner, true); w.Code != 403 {
		t.Fatalf("penugasan berakhir POST %d", w.Code)
	}

}
