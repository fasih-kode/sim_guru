package web

import (
	"context"
	"errors"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/studentcases"
)

func TestStudentCaseTemplateDoesNotRenderPrivateNotesWithoutServiceGrant(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return index + 1 },
		"weekdayName": func(int) string { return "Senin" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates}
	user := &auth.User{ID: 10, Active: true, DisplayName: "Wali Uji", Roles: []string{"wali_kelas"}}
	privateMarker := "ISI-KONSELING-PRIBADI-UJI"
	data := studentCasesView{Detail: studentcases.Detail{
		Case:              studentcases.Case{ID: 1, StudentName: "Siswa Uji", ClassName: "7A", Stage: studentcases.StageCounselor, Status: studentcases.StatusOpen, ResponsibleName: "BK Uji"},
		CounselingNotes:   []studentcases.CounselingNote{{Content: privateMarker}},
		CanReadCounseling: false,
	}}
	r := httptest.NewRequest("GET", "/kesiswaan/kasus/1", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, user))
	w := httptest.NewRecorder()
	app.render(w, r, "page", 200, view{Page: "student-case-detail", Title: "Detail Kasus", StudentCases: &data})
	if w.Code != 200 || strings.Contains(w.Body.String(), privateMarker) {
		t.Fatal("template rendered private counseling notes without service authorization")
	}
}

func TestStudentCaseNavigationExcludesAdminOnlyRole(t *testing.T) {
	active, group := sidebarState("/kesiswaan/kasus", &auth.User{Roles: []string{"admin"}})
	if active != "student-cases" || group != "student-cases" {
		t.Fatal("case path state should remain stable")
	}
	// Hak server tetap otoritatif; role Admin saja tidak muncul pada navigasi kasus.
	if strings.Contains(caseNavigationForTest(t, &auth.User{Roles: []string{"admin"}}), "Kasus Siswa") {
		t.Fatal("Admin-only navigation exposed case module")
	}
	waliMenu := caseNavigationForTest(t, &auth.User{Roles: []string{"wali_kelas"}})
	if !strings.Contains(waliMenu, "Kasus Siswa") || strings.Contains(waliMenu, "Catatan Pembinaan") {
		t.Fatal("navigation should expose one case menu without a separate guidance menu")
	}
}

func TestStudentCaseFailureDoesNotExposeDatabaseOrCounselingTextAndUsesAppShell(t *testing.T) {
	privateMarker := "ISI-KONSELING-RAHASIA-DB-ERROR"
	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(int, int) int { return 1 }, "weekdayName": func(int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/kesiswaan/kasus/1", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &auth.User{ID: 10, Active: true, Roles: []string{"bk"}}))
	app.studentCaseFailure(w, r, errors.New(privateMarker))
	if w.Code != 500 || strings.Contains(w.Body.String(), privateMarker) || !strings.Contains(w.Body.String(), "app-sidebar") {
		t.Fatal("case failure exposed internal/private text or omitted the app shell")
	}
}

func TestStudentCaseListUsesSharedLiveListContract(t *testing.T) {
	if !liveListPage("student-cases") || !liveListPage("student-guidance-archive") {
		t.Fatal("case and archive lists should use the shared HTMX Instant Search fragment")
	}
}

func caseNavigationForTest(t *testing.T, user *auth.User) string {
	t.Helper()
	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(int, int) int { return 1 }, "weekdayName": func(int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates}
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, user))
	w := httptest.NewRecorder()
	app.render(w, r, "page", 200, view{Page: "home", Title: "Beranda"})
	return w.Body.String()
}
