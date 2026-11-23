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

func TestDirectTPApplicabilityHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	fixture := testutil.FixtureDatabase(t)
	f := testutil.ExamFixtureFor(t, fixture, "s06http"+testutil.Random(t)[:8])
	ctx := context.Background()
	if _, err := fixture.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := (&academic.Store{DB: fixture}).CreateLearningObjective(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, "TP pindahan"); err != nil {
		t.Fatal(err)
	}
	var objective int64
	if err := fixture.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2`, f.Semester, f.OtherClass).Scan(&objective); err != nil {
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
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db}, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	request := func(method, path string, values url.Values, cookies []*http.Cookie, user auth.User) *httptest.ResponseRecorder {
		body := ""
		if values != nil {
			body = values.Encode()
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	contextValue := fmt.Sprintf("%d:%d:%d", f.Semester, f.OtherClass, f.Subject)
	path := "/guru/buku-nilai/keberlakuan?" + url.Values{"context": {contextValue}, "student_id": {fmt.Sprint(f.MovedStudent)}, "objective_id": {fmt.Sprint(objective)}}.Encode()
	page := request("GET", path, nil, nil, owner)
	if page.Code != 200 {
		t.Fatalf("GET %d %s", page.Code, page.Body.String())
	}
	if w := request("GET", path, nil, nil, other); w.Code != 403 {
		t.Fatalf("guru lain %d", w.Code)
	}
	field := func(name string) string {
		match := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
		if len(match) != 2 {
			t.Fatalf("field %s", name)
		}
		return match[1]
	}
	values := url.Values{"context": {contextValue}, "student_id": {fmt.Sprint(f.MovedStudent)}, "objective_id": {fmt.Sprint(objective)}, "version": {field("version")}, "submission_key": {field("submission_key")}, "gorilla.csrf.Token": {field("gorilla.csrf.Token")}, "effective_date": {"2020-02-10"}, "applicable": {"true"}, "reason": {"Bukti sintetis"}, "evidence_verified": {"yes"}}
	if w := request("POST", "/guru/buku-nilai/keberlakuan", values, nil, owner); w.Code != 403 {
		t.Fatalf("CSRF %d", w.Code)
	}
	w := request("POST", "/guru/buku-nilai/keberlakuan", values, page.Result().Cookies(), owner)
	if w.Code != 303 {
		t.Fatalf("POST %d %s", w.Code, w.Body.String())
	}
	if w = request("POST", "/guru/buku-nilai/keberlakuan", values, page.Result().Cookies(), owner); w.Code != 303 {
		t.Fatalf("replay %d %s", w.Code, w.Body.String())
	}
	var count int
	if err = fixture.QueryRow(`SELECT count(*) FROM app.direct_tp_applicability_history WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 1 {
		t.Fatalf("histori %d %v", count, err)
	}
}
