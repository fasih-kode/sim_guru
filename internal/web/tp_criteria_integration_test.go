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

func TestTPCriteriaHTTPAuthorizationAndCSRFIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "tpweb"+testutil.Random(t)[:8])
	ctx := context.Background()
	store := &academic.Store{DB: db}
	if err := store.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP uji HTTP"); err != nil {
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
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: store, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := testutil.User(t, db, "kurikulum")
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	urlPath := fmt.Sprintf("/guru/kriteria-tp?semester_id=%d&class_id=%d&subject_id=%d", f.Semester, f.Class, f.Subject)
	get := func(user auth.User, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := get(owner, "/akademik/pedoman-tp"); w.Code != 403 {
		t.Fatalf("Guru membaca pedoman Kurikulum %d", w.Code)
	}
	guidanceUser := auth.User{ID: cur.ID, Active: true, Roles: []string{"kurikulum"}}
	if w := get(guidanceUser, "/akademik/pedoman-tp"); w.Code != 200 || !strings.Contains(w.Body.String(), "Pedoman umum sekolah") {
		t.Fatalf("pedoman %d", w.Code)
	}
	w := get(owner, urlPath)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "TP uji HTTP") || !strings.Contains(w.Body.String(), "Belum diatur") {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	if w := get(other, urlPath); w.Code != 403 {
		t.Fatalf("guru lain GET %d", w.Code)
	}
	token := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
	if len(token) != 2 {
		t.Fatal("CSRF hilang")
	}
	cookies := w.Result().Cookies()
	form := url.Values{"gorilla.csrf.Token": {token[1]}, "semester_id": {fmt.Sprint(f.Semester)}, "class_id": {fmt.Sprint(f.Class)}, "subject_id": {fmt.Sprint(f.Subject)}, "objective_id": {"0"}, "version": {"0"}, "threshold": {"75"}, "reason": {"Uji"}}
	post := func(user auth.User, cookie bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/guru/kriteria-tp", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie {
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
		t.Fatalf("guru lain POST %d", w.Code)
	}
	if w := post(owner, true); w.Code != 303 {
		t.Fatalf("POST %d: %s", w.Code, w.Body.String())
	}
	if w := post(owner, true); w.Code != 409 {
		t.Fatalf("versi basi %d", w.Code)
	}
}
