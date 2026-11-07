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

func TestExamInitialHTTPValidationAuthorizationAndCSRFIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "web")
	var before int
	if err := db.QueryRow(`SELECT count(*) FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2`, f.Semester, f.Class).Scan(&before); err != nil {
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
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db}, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	contextValue := fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)
	path := "/guru/nilai-asts-asas?context=" + url.QueryEscape(contextValue) + "&date=2020-01-10"
	user := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	get := httptest.NewRequest("GET", path, nil)
	get = get.WithContext(context.WithValue(get.Context(), principalKey{}, &user))
	page := httptest.NewRecorder()
	h.ServeHTTP(page, get)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "ASTS") || !strings.Contains(page.Body.String(), "ASAS") {
		t.Fatalf("form: %d %s", page.Code, page.Body.String())
	}
	matches := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("token CSRF tidak ada")
	}
	cookies := page.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("cookie CSRF tidak ada")
	}
	post := func(actor auth.User, values url.Values, cookie bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/guru/nilai-asts-asas", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie {
			for _, c := range cookies {
				req.AddCookie(c)
			}
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &actor))
		out := httptest.NewRecorder()
		h.ServeHTTP(out, req)
		return out
	}
	values := url.Values{"semester_id": {fmt.Sprint(f.Semester)}, "class_id": {fmt.Sprint(f.Class)}, "subject_id": {fmt.Sprint(f.Subject)}, "roster_date": {"2020-01-10"}, "gorilla.csrf.Token": {matches[1]}, fmt.Sprintf("score_asts_%d", f.StudentA): {"0"}, fmt.Sprintf("date_asts_%d", f.StudentA): {"2020-01-10"}, fmt.Sprintf("score_asas_%d", f.StudentB): {"100,01"}, fmt.Sprintf("date_asas_%d", f.StudentB): {"2020-01-10"}}
	without := post(user, values, false)
	if without.Code != 403 {
		t.Fatalf("CSRF kosong: %d", without.Code)
	}
	wrong := url.Values{}
	for k, v := range values {
		wrong[k] = v
	}
	wrong.Set("gorilla.csrf.Token", "salah")
	if got := post(user, wrong, true); got.Code != 403 {
		t.Fatalf("CSRF salah: %d", got.Code)
	}
	invalid := post(user, values, true)
	body := invalid.Body.String()
	if invalid.Code != 422 || !strings.Contains(body, `value="0"`) || !strings.Contains(body, `value="100,01"`) || !strings.Contains(body, `value="2020-01-10"`) || !strings.Contains(body, "Pilih konteks penugasan") {
		t.Fatalf("validasi/pemulihan: %d %s", invalid.Code, body)
	}
	blankDate := url.Values{}
	for k, v := range values {
		blankDate[k] = append([]string(nil), v...)
	}
	blankDate.Set(fmt.Sprintf("date_asts_%d", f.StudentA), "")
	blankDate.Set(fmt.Sprintf("score_asas_%d", f.StudentB), "50")
	blankResult := post(user, blankDate, true)
	if blankResult.Code != 422 || !strings.Contains(blankResult.Body.String(), fmt.Sprintf(`name="date_asts_%d" type="date" value=""`, f.StudentA)) || !strings.Contains(blankResult.Body.String(), `value="0"`) || !strings.Contains(blankResult.Body.String(), `value="50"`) {
		t.Fatalf("tanggal kosong dan input lain harus tetap ada: %d", blankResult.Code)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2`, f.Semester, f.Class).Scan(&n); err != nil || n != before {
		t.Fatalf("batch invalid menulis: %d %v", n, err)
	}
	invalidID := url.Values{}
	for k, v := range values {
		invalidID[k] = append([]string(nil), v...)
	}
	invalidID.Del(fmt.Sprintf("score_asas_%d", f.StudentB))
	invalidID.Set(fmt.Sprintf("score_asts_%d", 999999999), "42")
	invalidID.Set(fmt.Sprintf("date_asts_%d", 999999999), "2020-01-10")
	if got := post(user, invalidID, true); got.Code != 422 {
		t.Fatalf("ID siswa asing: %d", got.Code)
	}
	invalidComponent := url.Values{}
	for k, v := range values {
		invalidComponent[k] = append([]string(nil), v...)
	}
	invalidComponent.Del(fmt.Sprintf("score_asas_%d", f.StudentB))
	invalidComponent.Set(fmt.Sprintf("score_uts_%d", f.StudentB), "10")
	invalidComponent.Set(fmt.Sprintf("date_uts_%d", f.StudentB), "2020-01-10")
	if got := post(user, invalidComponent, true); got.Code != 422 {
		t.Fatalf("komponen asing: %d", got.Code)
	}
	invalidClass := url.Values{}
	for k, v := range values {
		invalidClass[k] = append([]string(nil), v...)
	}
	invalidClass.Set("class_id", "999999999")
	if got := post(user, invalidClass, true); got.Code != 403 {
		t.Fatalf("ID kelas asing: %d", got.Code)
	}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	denied := post(other, values, true)
	if denied.Code != 403 {
		t.Fatalf("guru lain: %d", denied.Code)
	}
	otherGet := httptest.NewRequest("GET", path, nil)
	otherGet = otherGet.WithContext(context.WithValue(otherGet.Context(), principalKey{}, &other))
	blocked := httptest.NewRecorder()
	h.ServeHTTP(blocked, otherGet)
	if blocked.Code != 403 {
		t.Fatalf("GET guru lain: %d", blocked.Code)
	}
	valid := url.Values{"semester_id": {fmt.Sprint(f.Semester)}, "class_id": {fmt.Sprint(f.Class)}, "subject_id": {fmt.Sprint(f.Subject)}, "roster_date": {"2020-01-10"}, "gorilla.csrf.Token": {matches[1]}, fmt.Sprintf("score_asas_%d", f.StudentA): {"90"}, fmt.Sprintf("date_asas_%d", f.StudentA): {"2020-01-10"}}
	if result := post(user, valid, true); result.Code != 303 {
		t.Fatalf("POST nilai awal sah: %d %s", result.Code, result.Body.String())
	}
	var actual string
	if err = db.QueryRow(`SELECT score::text FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2 AND student_id=$3 AND component='asas'`, f.Semester, f.Class, f.StudentA).Scan(&actual); err != nil || actual != "90.00" {
		t.Fatalf("nilai POST tidak tersimpan: %s %v", actual, err)
	}
	store := &academic.Store{DB: db}
	if err = store.SaveExamInitialScores(context.Background(), f.Teacher, f.Semester, f.OtherClass, f.Subject, []academic.ExamInitialInput{{StudentID: f.MovedStudent, Component: "asts", Date: "2020-02-10", Value: "82,50"}}); err != nil {
		t.Fatal(err)
	}
	sourcePath := fmt.Sprintf("/guru/nilai-asts-asas?context=%d%%3A%d%%3A%d&date=2020-02-10", f.Semester, f.OtherClass, f.Subject)
	sourceReq := httptest.NewRequest("GET", sourcePath, nil)
	sourceReq = sourceReq.WithContext(context.WithValue(sourceReq.Context(), principalKey{}, &user))
	source := httptest.NewRecorder()
	h.ServeHTTP(source, sourceReq)
	replay := url.Values{"semester_id": {fmt.Sprint(f.Semester)}, "class_id": {fmt.Sprint(f.OtherClass)}, "subject_id": {fmt.Sprint(f.Subject)}, "roster_date": {"2020-02-10"}, "gorilla.csrf.Token": {matches[1]}, fmt.Sprintf("score_asts_%d", f.MovedStudent): {"82,50"}, fmt.Sprintf("date_asts_%d", f.MovedStudent): {"2020-02-10"}}
	if got := post(user, replay, true); got.Code != 303 {
		t.Fatalf("POST replay sah: %d", got.Code)
	}
	replay.Set(fmt.Sprintf("score_asts_%d", f.MovedStudent), "92,25")
	conflict := post(user, replay, true)
	if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), "Input belum tersimpan: 92,25") || !strings.Contains(conflict.Body.String(), "82,50") {
		t.Fatalf("konflik dan pemulihan: %d %s", conflict.Code, conflict.Body.String())
	}
	for _, text := range []string{"82,50", "Sudah dicatat", "Guru fixture 1", "Pencatat:", "WIB"} {
		if source.Code != 200 || !strings.Contains(source.Body.String(), text) {
			t.Fatalf("asal nilai %q: %d", text, source.Code)
		}
	}
}
