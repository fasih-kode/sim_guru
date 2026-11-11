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
		t.Fatalf("Guru tidak boleh membuka halaman administrasi pedoman: %d", w.Code)
	}
	guidanceUser := auth.User{ID: cur.ID, Active: true, Roles: []string{"kurikulum"}}
	if w := get(guidanceUser, "/akademik/pedoman-tp"); w.Code != 200 || !strings.Contains(w.Body.String(), "Pedoman umum sekolah") {
		t.Fatalf("pedoman %d", w.Code)
	}
	current, err := store.TPGuidance(ctx, cur.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveTPGuidance(ctx, cur.ID, current.Version, "Pedoman terkini guru", "Uji HTTP"); err != nil {
		t.Fatal(err)
	}
	w := get(owner, urlPath)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "TP uji HTTP") || !strings.Contains(w.Body.String(), "Belum diatur") || !strings.Contains(w.Body.String(), "Pedoman terkini guru") {
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
	form.Set("threshold", "100,01")
	form.Set("reason", "Alasan dipertahankan")
	if invalid := post(owner, true); invalid.Code != 422 || !strings.Contains(invalid.Body.String(), `value="100,01"`) || !strings.Contains(invalid.Body.String(), `value="Alasan dipertahankan"`) || !strings.Contains(invalid.Body.String(), `name="version" value="0"`) {
		t.Fatalf("input 422 tidak dipertahankan: %d", invalid.Code)
	}
	form.Set("threshold", "75")
	if w := post(owner, true); w.Code != 303 {
		t.Fatalf("POST %d: %s", w.Code, w.Body.String())
	}
	if w := post(owner, true); w.Code != 409 || !strings.Contains(w.Body.String(), `value="75"`) || !strings.Contains(w.Body.String(), `value="Alasan dipertahankan"`) || !strings.Contains(w.Body.String(), `name="version" value="0"`) {
		t.Fatalf("versi basi atau input tidak dipertahankan %d", w.Code)
	}
	req := httptest.NewRequest("POST", "/akademik/pedoman-tp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &owner))
	denied := httptest.NewRecorder()
	h.ServeHTTP(denied, req)
	if denied.Code != 403 {
		t.Fatalf("guru POST pedoman: %d", denied.Code)
	}
}
