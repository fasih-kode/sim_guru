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

func TestAdminSidebarUsesFlatLinksAndNavigationTabsTrackDeepLinks(t *testing.T) {
	admin := &auth.User{ID: 1, Active: true, DisplayName: "Admin Uji", Roles: []string{"admin"}}
	adminHome := caseNavigationForTest(t, admin)
	start := strings.Index(adminHome, `<nav aria-label="Menu utama"`)
	if start < 0 {
		t.Fatal("navigasi utama tidak dirender")
	}
	end := strings.Index(adminHome[start:], "</nav>")
	if end < 0 {
		t.Fatal("penutup navigasi utama tidak ditemukan")
	}
	sidebar := adminHome[start : start+end]
	for _, want := range []string{`href="/"`, `href="/master/students"`, `href="/admin/users"`, `href="/admin/kehadiran-tu"`} {
		if !strings.Contains(sidebar, want) {
			t.Errorf("sidebar Admin tidak memuat link %q", want)
		}
	}
	if strings.Count(sidebar, `<a class="nav-link nav-admin-parent`) != 3 || strings.Contains(sidebar, `class="nav-group"`) || strings.Contains(sidebar, "nav-submenu") {
		t.Fatal("sidebar Admin belum datar atau masih menampilkan submenu")
	}

	cases := []struct {
		path, section, active, parent, tabHref string
	}{
		{"/master/students/12/edit", "Master Data", "admin-students", "admin-master", "/master/students"},
		{"/master/teachers/8/history", "Master Data", "admin-teachers", "admin-master", "/master/teachers"},
		{"/master/staff/4/edit", "Master Data", "admin-staff", "admin-master", "/master/staff"},
		{"/akademik/kelas/2/edit", "Master Data", "admin-classes", "admin-master", "/akademik/kelas"},
		{"/admin/placements/dates/bulk", "Master Data", "admin-placements", "admin-master", "/admin/placements"},
		{"/admin/users/5/edit", "Pengguna & Akses", "admin-users", "admin-access", "/admin/users"},
		{"/admin/audit", "Pengguna & Akses", "admin-audit", "admin-access", "/admin/audit"},
		{"/admin/kehadiran-tu/catatan/7", "Kehadiran", "admin-staff-attendance", "admin-attendance", "/admin/kehadiran-tu"},
		{"/admin/attendance/location", "Kehadiran", "admin-teacher-attendance-location", "admin-attendance", "/admin/attendance/location"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			body := navigationPageForTest(t, admin, tc.path)
			sectionHTML := strings.ReplaceAll(tc.section, "&", "&amp;")
			if !strings.Contains(body, `<h1 class="section-navigation-title" id="section-navigation-title">`+sectionHTML+`</h1>`) {
				t.Fatalf("judul kelompok %q tidak dirender", tc.section)
			}
			activeLink := `class="navigation-tab is-active" data-navigation-tab="` + tc.active + `" href="` + tc.tabHref + `" aria-current="page"`
			if !strings.Contains(body, activeLink) {
				t.Fatalf("tab aktif %q tidak ditandai pada %s", tc.tabHref, tc.path)
			}
			if strings.Count(body, `aria-current="page"`) != 1 {
				t.Fatalf("halaman harus menampilkan tepat satu tab aktif pada %s", tc.path)
			}
			parentLink := `nav-link nav-admin-parent is-active` + `" href="`
			if !strings.Contains(body, parentLink) || !strings.Contains(body, `aria-label="Navigasi `+sectionHTML+`"`) {
				t.Fatalf("parent sidebar atau label navigasi tab tidak aktif pada %s", tc.path)
			}
		})
	}

	for _, tc := range []struct {
		section string
		want    int
	}{{"Master Data", 5}, {"Pengguna & Akses", 2}, {"Kehadiran", 2}} {
		tabs := adminNavigationTabs(tc.section, "")
		if len(tabs) != tc.want {
			t.Errorf("jumlah tab %q = %d, ingin %d", tc.section, len(tabs), tc.want)
		}
		for _, tab := range tabs {
			if !strings.HasPrefix(tab.Href, "/") || tab.Label == "" || tab.Icon == "" {
				t.Errorf("kontrak tab tidak lengkap: %#v", tab)
			}
		}
	}

	curriculum := &auth.User{ID: 2, Active: true, DisplayName: "Kurikulum Uji", Roles: []string{"kurikulum"}}
	body := navigationPageForTest(t, curriculum, "/master/students")
	if strings.Contains(body, "section-navigation-tabs") || strings.Contains(body, `href="/admin/users"`) || strings.Contains(body, `href="/admin/kehadiran-tu"`) {
		t.Fatal("role Kurikulum menerima navigasi Admin tanpa role Admin")
	}
	dual := &auth.User{ID: 3, Active: true, DisplayName: "Admin Rangkap", Roles: []string{"admin", "kurikulum"}}
	dualBody := navigationPageForTest(t, dual, "/admin/users")
	if strings.Count(dualBody, `href="/admin/users"`) != 2 {
		t.Fatal("role gabungan harus memiliki satu link sidebar dan satu tab Pengguna & Role")
	}
}

func TestCurriculumSidebarUsesSharedTabsWithoutRoleDuplicates(t *testing.T) {
	curriculum := &auth.User{ID: 4, Active: true, DisplayName: "Kurikulum Uji", Roles: []string{"kurikulum"}}
	nav := navigationPageForTest(t, curriculum, "/akademik/kalender-sekolah")
	start := strings.Index(nav, `<nav aria-label="Menu utama"`)
	if start < 0 {
		t.Fatal("navigasi utama tidak dirender")
	}
	end := strings.Index(nav[start:], "</nav>")
	sidebar := nav[start : start+end]
	for _, want := range []string{`href="/akademik/kalender-sekolah"`, `href="/akademik/struktur-kurikulum"`, `href="/akademik/waktu-sekolah"`, `href="/akademik/transisi"`} {
		if !strings.Contains(sidebar, want) {
			t.Errorf("sidebar Kurikulum tidak memuat %q", want)
		}
	}
	if strings.Count(sidebar, `class="nav-link nav-curriculum-parent`) != 4 || strings.Contains(sidebar, `class="nav-group"`) || strings.Contains(sidebar, "nav-submenu") {
		t.Fatal("sidebar Kurikulum masih menggunakan dropdown atau jumlah kelompok keliru")
	}

	cases := []struct {
		path, section, active, tabHref string
	}{
		{"/akademik/kalender-sekolah", "Kalender Akademik", "curriculum-school-calendar", "/akademik/kalender-sekolah"},
		{"/akademik", "Kalender Akademik", "curriculum-academic", "/akademik"},
		{"/akademik/tahun-ajaran/4/aktifkan", "Kalender Akademik", "curriculum-years", "/akademik/tahun-ajaran"},
		{"/akademik/semester/2/aktifkan", "Kalender Akademik", "curriculum-semesters", "/akademik/semester"},
		{"/akademik/struktur-kurikulum", "Struktur Kurikulum", "curriculum-structure", "/akademik/struktur-kurikulum"},
		{"/master/subjects", "Struktur Kurikulum", "curriculum-subjects", "/master/subjects"},
		{"/akademik/waktu-sekolah", "Pengaturan Jadwal", "curriculum-school-time", "/akademik/waktu-sekolah"},
		{"/akademik/batasan-jadwal", "Pengaturan Jadwal", "curriculum-schedule-constraints", "/akademik/batasan-jadwal"},
		{"/akademik/pengampu", "Pengaturan Jadwal", "curriculum-teaching-assignments", "/akademik/pengampu"},
		{"/akademik/jam-pelajaran", "Pengaturan Jadwal", "curriculum-lesson-periods", "/akademik/jam-pelajaran"},
		{"/akademik/jadwal/impor", "Pengaturan Jadwal", "curriculum-weekly-timetable", "/akademik/jadwal"},
		{"/akademik/jurnal-lineage", "Pengaturan Jadwal", "curriculum-journal-lineage", "/akademik/jurnal-lineage"},
		{"/akademik/transisi", "Siswa & Penempatan", "curriculum-transitions", "/akademik/transisi"},
		{"/master/students/4/edit", "Siswa & Penempatan", "curriculum-students", "/master/students"},
		{"/master/teachers/3/history", "Siswa & Penempatan", "curriculum-teachers", "/master/teachers"},
		{"/akademik/kelas/4/wali", "Siswa & Penempatan", "curriculum-classes", "/akademik/kelas"},
		{"/admin/placements", "Siswa & Penempatan", "curriculum-placements", "/admin/placements"},
		{"/akademik/penempatan/9/pindah-kelas", "Siswa & Penempatan", "curriculum-placements", "/admin/placements"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			body := navigationPageForTest(t, curriculum, tc.path)
			sectionHTML := strings.ReplaceAll(tc.section, "&", "&amp;")
			active := `class="navigation-tab is-active" data-navigation-tab="` + tc.active + `" href="` + tc.tabHref + `" aria-current="page"`
			if !strings.Contains(body, `<h1 class="section-navigation-title" id="section-navigation-title">`+sectionHTML+`</h1>`) || !strings.Contains(body, active) {
				t.Fatalf("kelompok atau tab aktif salah pada %s", tc.path)
			}
			if strings.Count(body, `aria-current="page"`) != 1 {
				t.Fatalf("harus ada satu tab aktif pada %s", tc.path)
			}
			parent := `nav-link nav-curriculum-parent is-active` + `" href="`
			if !strings.Contains(body, parent) {
				t.Fatalf("parent Kurikulum tidak aktif pada %s", tc.path)
			}
		})
	}

	for _, tc := range []struct {
		section string
		want    int
	}{{"Kalender Akademik", 4}, {"Struktur Kurikulum", 3}, {"Pengaturan Jadwal", 6}, {"Siswa & Penempatan", 5}} {
		if got := len(sectionNavigationTabs(tc.section, "", curriculum)); got != tc.want {
			t.Errorf("jumlah tab Kurikulum %q = %d, ingin %d", tc.section, got, tc.want)
		}
	}

	adminOnly := &auth.User{ID: 5, Active: true, Roles: []string{"admin"}}
	if section, _ := sectionNavigationState("/akademik/waktu-sekolah", adminOnly); section != "" {
		t.Fatal("role Admin saja menerima tab Kurikulum")
	}
	if got := len(sectionNavigationTabs("Master Data", "admin-students", curriculum)); got != 0 {
		t.Fatal("role Kurikulum saja tidak boleh membangun tab Admin")
	}

	dual := &auth.User{ID: 6, Active: true, Roles: []string{"admin", "kurikulum", "guru_mapel"}}
	adminPage := navigationPageForTest(t, dual, "/master/students")
	if strings.Contains(adminPage, `data-navigation-tab="curriculum-students"`) {
		t.Fatal("route bersama master siswa ditampilkan ganda pada navigasi Kurikulum")
	}
	transitionPage := navigationPageForTest(t, dual, "/akademik/transisi")
	if strings.Contains(transitionPage, `data-navigation-tabs`) || !strings.Contains(transitionPage, `href="/akademik/transisi"`) {
		t.Fatal("kelompok satu halaman harus tetap dapat diakses tanpa memunculkan tab tunggal")
	}
	if !strings.Contains(transitionPage, `nav-link nav-curriculum-parent is-active" href="/akademik/transisi" aria-current="page"`) {
		t.Fatal("link kelompok Siswa & Penempatan harus ditandai sebagai halaman saat ini")
	}
	if !strings.Contains(transitionPage, `href="/kehadiran-saya"`) || !strings.Contains(transitionPage, `href="/jadwal-saya"`) {
		t.Fatal("navigasi role gabungan kehilangan Kehadiran Guru atau Guru Mapel")
	}
}

func TestTeacherAttendanceAndSubjectTeacherUseSharedNavigationTabs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles []string
		want  []string
		avoid []string
	}{
		{"guru mapel saja", []string{"guru_mapel"}, []string{"Kehadiran Guru", "Guru Mapel"}, []string{"Kehadiran Saya", "Rekap Kehadiran Saya", "Jadwal Saya", "Absensi Pelajaran", "nav-submenu"}},
		{"wali kelas saja", []string{"wali_kelas"}, []string{"Kehadiran Guru"}, []string{"Guru Mapel", "Kehadiran Saya", "Rekap Kehadiran Saya", "nav-submenu"}},
		{"kurikulum dan guru mapel", []string{"kurikulum", "guru_mapel"}, []string{"Kalender Akademik", "Kehadiran Guru", "Guru Mapel"}, []string{"Koreksi Absensi Pelajaran", "nav-submenu"}},
		{"admin dan guru mapel", []string{"admin", "guru_mapel"}, []string{"Master Data", "Kehadiran Guru", "Guru Mapel"}, []string{"nav-submenu"}},
	} {
		t.Run("sidebar "+tc.name, func(t *testing.T) {
			body := navigationPageForTest(t, &auth.User{Roles: tc.roles}, "/")
			start := strings.Index(body, `<nav aria-label="Menu utama"`)
			end := strings.Index(body[start:], "</nav>")
			sidebar := body[start : start+end]
			for _, text := range tc.want {
				if !strings.Contains(sidebar, text) {
					t.Errorf("sidebar tidak memuat %q", text)
				}
			}
			for _, text := range tc.avoid {
				if strings.Contains(sidebar, text) {
					t.Errorf("sidebar masih memuat submenu lama %q", text)
				}
			}
		})
	}

	tests := []struct {
		name, path, section, active, parent string
		roles                               []string
	}{
		{"guru mapel kehadiran", "/kehadiran-saya", "Kehadiran Guru", "teacher-attendance", "teacher-attendance", []string{"guru_mapel"}},
		{"wali kelas rekap", "/kehadiran-saya/rekap", "Kehadiran Guru", "teacher-recap", "teacher-attendance", []string{"wali_kelas"}},
		{"guru mapel jadwal", "/jadwal-saya", "Guru Mapel", "my-schedule", "guru-mapel", []string{"guru_mapel"}},
		{"guru mapel absensi detail", "/jadwal-saya/absensi/catatan/9", "Guru Mapel", "lesson-attendance", "guru-mapel", []string{"guru_mapel"}},
		{"kurikulum guru mapel", "/jadwal-saya/absensi", "Guru Mapel", "lesson-attendance", "guru-mapel", []string{"kurikulum", "guru_mapel"}},
		{"admin guru mapel", "/jadwal-saya/absensi", "Guru Mapel", "lesson-attendance", "guru-mapel", []string{"admin", "guru_mapel"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user := &auth.User{ID: 20, Active: true, Roles: tc.roles}
			body := navigationPageForTest(t, user, tc.path)
			sectionHTML := strings.ReplaceAll(tc.section, "&", "&amp;")
			if !strings.Contains(body, `<h1 class="section-navigation-title" id="section-navigation-title">`+sectionHTML+`</h1>`) {
				t.Fatalf("judul kelompok %q tidak muncul pada %s", tc.section, tc.path)
			}
			if !strings.Contains(body, `class="navigation-tab is-active" data-navigation-tab="`+tc.active+`"`) {
				t.Fatalf("tab aktif %q tidak muncul pada %s", tc.active, tc.path)
			}
			if strings.Count(body, `aria-current="page"`) != 1 {
				t.Fatalf("halaman %s harus menandai tepat satu link sebagai aktif", tc.path)
			}
			if !strings.Contains(body, `nav-link nav-teacher-parent is-active"`) && tc.parent == "teacher-attendance" {
				t.Fatalf("parent Kehadiran Guru tidak aktif pada %s", tc.path)
			}
			if !strings.Contains(body, `nav-link nav-subject-teacher-parent is-active"`) && tc.parent == "guru-mapel" {
				t.Fatalf("parent Guru Mapel tidak aktif pada %s", tc.path)
			}
		})
	}

	for _, tc := range []struct {
		section string
		roles   []string
		want    int
	}{
		{"Kehadiran Guru", []string{"guru_mapel"}, 2},
		{"Kehadiran Guru", []string{"wali_kelas"}, 2},
		{"Guru Mapel", []string{"guru_mapel"}, 3},
		{"Guru Mapel", []string{"kurikulum"}, 0},
	} {
		user := &auth.User{Roles: tc.roles}
		if got := len(sectionNavigationTabs(tc.section, "", user)); got != tc.want {
			t.Errorf("role %v mendapat %d tab pada %q, ingin %d", tc.roles, got, tc.section, tc.want)
		}
	}

	adminOnly := &auth.User{Roles: []string{"admin"}}
	for _, path := range []string{"/kehadiran-saya", "/jadwal-saya", "/jadwal-saya/absensi"} {
		if section, _ := sectionNavigationState(path, adminOnly); section != "" {
			t.Errorf("Admin saja menerima tab %q pada route %s", section, path)
		}
	}

	dual := &auth.User{ID: 21, Active: true, Roles: []string{"guru_mapel", "pegawai_tu", "pengelola_kehadiran_guru"}}
	managementRecap := navigationPageForTest(t, dual, "/tu/kehadiran-guru/rekap")
	if strings.Count(managementRecap, `aria-current="page"`) != 1 || !strings.Contains(managementRecap, `data-navigation-tab="managed-teacher-recap"`) || !strings.Contains(managementRecap, `nav-link nav-management-parent is-active`) {
		t.Fatal("rekap pengelola dan kehadiran pribadi tidak boleh sama-sama aktif")
	}

	curriculumOnly := &auth.User{Roles: []string{"kurikulum"}}
	curriculumPage := navigationPageForTest(t, curriculumOnly, "/jadwal-saya/absensi")
	if strings.Contains(curriculumPage, `data-navigation-tab="lesson-attendance"`) || !strings.Contains(curriculumPage, "Koreksi Absensi Pelajaran") {
		t.Fatal("akses koreksi Kurikulum harus dipertahankan tanpa menampilkan tab Guru Mapel")
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
	return navigationPageForTest(t, user, "/")
}

func navigationPageForTest(t *testing.T, user *auth.User, path string) string {
	t.Helper()
	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(int, int) int { return 1 }, "weekdayName": func(int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates}
	r := httptest.NewRequest("GET", path, nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, user))
	w := httptest.NewRecorder()
	app.render(w, r, "page", 200, view{Page: "home", Title: "Beranda"})
	return w.Body.String()
}
