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

func TestExamScoreCorrectionHTTPHistoryCSRFAndOwnershipIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "examcorrectionweb")
	ctx := context.Background()
	store := &academic.Store{DB: db}
	if err := store.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []academic.ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "72,50"}}); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4 AND component='asts'`, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&id); err != nil {
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
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	path := fmt.Sprintf("/guru/nilai-ujian/%d/koreksi", id)
	get := func(user auth.User, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	page := get(owner, path)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "72.50") || !strings.Contains(page.Body.String(), "Riwayat Perubahan") {
		t.Fatalf("detail: %d", page.Code)
	}
	tokenRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	match := tokenRE.FindStringSubmatch(page.Body.String())
	if len(match) != 2 {
		t.Fatal("CSRF tidak ada")
	}
	cookies := page.Result().Cookies()
	post := func(user auth.User, form url.Values, sendCookie bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if sendCookie {
			for _, c := range cookies {
				req.AddCookie(c)
			}
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	detail, err := store.ExamScoreCorrectionDetail(ctx, f.Teacher, id)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"gorilla.csrf.Token": {match[1]}, "expected_version": {fmt.Sprint(detail.CurrentVersion)}, "value": {"82,50"}, "reason": {"Kesalahan pencatatan"}}
	if w := post(owner, form, false); w.Code != 403 {
		t.Fatalf("CSRF kosong: %d", w.Code)
	}
	wrong := url.Values{}
	for k, v := range form {
		wrong[k] = append([]string(nil), v...)
	}
	wrong.Set("gorilla.csrf.Token", "invalid")
	if w := post(owner, wrong, true); w.Code != 403 {
		t.Fatalf("CSRF salah: %d", w.Code)
	}
	withoutRole := auth.User{ID: f.Teacher, Active: true, Roles: []string{"kurikulum"}}
	if w := get(withoutRole, path); w.Code != 403 {
		t.Fatalf("GET tanpa role Guru Mapel: %d", w.Code)
	}
	if w := post(withoutRole, form, true); w.Code != 403 {
		t.Fatalf("POST tanpa role Guru Mapel: %d", w.Code)
	}
	if w := get(other, path); w.Code != 404 {
		t.Fatalf("GET guru lain: %d", w.Code)
	}
	if w := post(other, form, true); w.Code != 403 {
		t.Fatalf("POST guru lain: %d", w.Code)
	}
	invalid := url.Values{}
	for k, v := range form {
		invalid[k] = append([]string(nil), v...)
	}
	invalid.Set("value", "100,01")
	invalid.Set("reason", "Alasan tetap ada")
	w := post(owner, invalid, true)
	if w.Code != 422 || !strings.Contains(w.Body.String(), `value="100,01"`) || !strings.Contains(w.Body.String(), "Alasan tetap ada") {
		t.Fatalf("validasi/input: %d %s", w.Code, w.Body.String())
	}
	if w = post(owner, form, true); w.Code != 303 {
		t.Fatalf("POST sah: %d %s", w.Code, w.Body.String())
	}
	if w = post(owner, form, true); w.Code != 409 || !strings.Contains(w.Body.String(), "Kesalahan pencatatan") || !strings.Contains(w.Body.String(), `value="82,50"`) {
		t.Fatalf("versi basi: %d", w.Code)
	}
	list := get(owner, "/guru/nilai-ujian-saya?q=fixture")
	if list.Code != 200 || !strings.Contains(list.Body.String(), "82.50") || !strings.Contains(list.Body.String(), "72.50") {
		t.Fatalf("daftar historis: %d", list.Code)
	}
	hx := httptest.NewRequest("GET", "/guru/nilai-ujian-saya?q=fixture", nil)
	hx.Header.Set("HX-Request", "true")
	hx.Header.Set("HX-List-Request", "true")
	hx = hx.WithContext(context.WithValue(hx.Context(), principalKey{}, &owner))
	fragment := httptest.NewRecorder()
	h.ServeHTTP(fragment, hx)
	if fragment.Code != 200 || !strings.Contains(fragment.Body.String(), `data-list-region`) || !strings.Contains(fragment.Body.String(), "72.50") || strings.Contains(fragment.Body.String(), "<html") {
		t.Fatalf("instant search fragment: %d", fragment.Code)
	}
	initial := get(owner, fmt.Sprintf("/guru/nilai-asts-asas?semester_id=%d&class_id=%d&subject_id=%d&date=2020-01-10", f.Semester, f.Class, f.Subject))
	if initial.Code != 200 || !strings.Contains(initial.Body.String(), "Nilai awal asli") || !strings.Contains(initial.Body.String(), "72,50") || !strings.Contains(initial.Body.String(), "82,50") {
		t.Fatalf("halaman awal tidak membedakan nilai: %d", initial.Code)
	}
}
