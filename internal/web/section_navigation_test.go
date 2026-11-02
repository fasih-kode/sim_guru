package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestSharedNavigationFiltersRolesAndKeepsSingleActiveTab(t *testing.T) {
	tests := []struct {
		name, path, section, active, parent string
		roles                               []string
		wantTabs                            int
	}{
		{"pengelola dengan pegawai TU", "/tu/kehadiran-guru/rekap", "Penanganan Kehadiran Guru", "managed-teacher-recap", "teacher-attendance-management", []string{"pengelola_kehadiran_guru", "pegawai_tu"}, 2},
		{"pengelola tanpa pegawai TU", "/tu/kehadiran-guru", "", "", "teacher-attendance-management", []string{"pengelola_kehadiran_guru"}, 0},
		{"petugas absensi", "/absensi/pemindai", "Kehadiran Siswa", "daily-scanner", "attendance", []string{"petugas_absensi"}, 3},
		{"kesiswaan", "/kesiswaan/batas-kedatangan", "Kehadiran Siswa", "arrival-rules", "attendance", []string{"kesiswaan"}, 2},
		{"wali kelas satu halaman", "/absensi/harian", "", "", "attendance", []string{"wali_kelas"}, 0},
		{"kepala sekolah satu halaman", "/absensi/harian", "", "", "attendance", []string{"kepala_sekolah"}, 0},
		{"role gabungan tanpa duplikasi", "/absensi/harian", "Kehadiran Siswa", "daily-attendance", "attendance", []string{"petugas_absensi", "kesiswaan", "wali_kelas"}, 4},
		{"jurnal Guru Mapel", "/jurnal-saya/123/versi/1", "Guru Mapel", "teaching-journal", "guru-mapel", []string{"guru_mapel"}, 4},
		{"TP Guru Mapel", "/guru/tujuan-pembelajaran", "Guru Mapel", "learning-objectives", "guru-mapel", []string{"guru_mapel"}, 4},
		{"bobot Kurikulum", "/akademik/bobot-penilaian", "Struktur Kurikulum", "curriculum-assessment-weights", "curriculum-subjects", []string{"kurikulum"}, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user := &auth.User{ID: 42, Active: true, Roles: tc.roles}
			section, active := sectionNavigationState(tc.path, user)
			if section != tc.section || active != tc.active {
				t.Fatalf("state = (%q, %q), ingin (%q, %q)", section, active, tc.section, tc.active)
			}
			if got := len(sectionNavigationTabs(section, active, user)); got != tc.wantTabs {
				t.Fatalf("jumlah tab = %d, ingin %d", got, tc.wantTabs)
			}
			body := navigationPageForTest(t, user, tc.path)
			if tc.section != "" {
				if !strings.Contains(body, `data-navigation-tab="`+tc.active+`"`) {
					t.Fatal("halaman kelompok tidak menampilkan tab aktif")
				}
				if strings.Count(body, `aria-current="page"`) != 1 {
					t.Fatal("navigasi harus memiliki tepat satu link halaman aktif")
				}
			} else if strings.Contains(body, `data-navigation-tabs`) {
				t.Fatal("kelompok satu halaman tidak perlu menampilkan strip tab")
			} else if strings.Count(body, `aria-current="page"`) != 1 {
				t.Fatal("link parent halaman tunggal harus menjadi satu-satunya halaman aktif")
			}
			switch tc.parent {
			case "teacher-attendance-management":
				if !strings.Contains(body, `nav-link nav-management-parent is-active`) {
					t.Fatal("parent pengelola kehadiran tidak ditandai aktif")
				}
			case "guru-mapel":
				if !strings.Contains(body, `nav-link nav-subject-teacher-parent is-active`) {
					t.Fatal("parent Guru Mapel tidak ditandai aktif")
				}
			case "attendance":
				if !strings.Contains(body, `nav-link nav-student-attendance-parent is-active`) {
					t.Fatal("parent Kehadiran Siswa tidak ditandai aktif")
				}
			case "curriculum-subjects":
				if !strings.Contains(body, `nav-link nav-curriculum-parent is-active`) {
					t.Fatal("parent Struktur Kurikulum tidak ditandai aktif")
				}
			}
		})
	}
}

func TestSharedNavigationDoesNotExposeUnauthorizedStudentAttendanceLinks(t *testing.T) {
	user := &auth.User{ID: 43, Active: true, Roles: []string{"guru_mapel"}}
	if section, _ := sectionNavigationState("/absensi/pemindai", user); section != "" {
		t.Fatalf("route petugas tidak boleh membangun tab untuk Guru Mapel: %q", section)
	}
	for _, role := range []*auth.User{{Roles: []string{"wali_kelas"}}, {Roles: []string{"kepala_sekolah"}}} {
		for _, tab := range sectionNavigationTabs("Kehadiran Siswa", "daily-scanner", role) {
			if tab.ID == "daily-scanner" || tab.ID == "student-cards" {
				t.Fatalf("role %v menerima link pemindai/kartu Petugas", role.Roles)
			}
		}
	}
}

func TestSharedNavigationUsesNativeLinksAndMobileKeyboardStyles(t *testing.T) {
	templateSource, err := content.ReadFile("templates/page.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(templateSource), `href="{{.Href}}"`) {
		t.Fatal("tab harus berupa link navigasi native")
	}
	if strings.Contains(string(templateSource), `role="tablist"`) || strings.Contains(string(templateSource), `role="tab"`) {
		t.Fatal("navigasi antarhalaman tidak boleh memakai semantik widget tab")
	}
	css, err := content.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".navigation-tabs-viewport", "overflow-x:auto", ".navigation-tab:focus-visible"} {
		if !strings.Contains(string(css), want) {
			t.Errorf("aset CSS terbangun tidak memuat dukungan %q", want)
		}
	}
}

func TestSidebarRoleInventoryHasNoStandaloneHeadingsOrDuplicateLinks(t *testing.T) {
	roles := []struct {
		name  string
		roles []string
		want  []string
		avoid []string
	}{
		{"Admin", []string{"admin"}, []string{"Beranda", "Master Data", "Pengguna &amp; Akses", "Kehadiran"}, []string{"KOORDINASI KASUS", "nav-submenu"}},
		{"Kurikulum", []string{"kurikulum"}, []string{"Beranda", "Kalender Akademik", "Struktur Kurikulum", "Pengaturan Jadwal", "Siswa &amp; Penempatan", "Koreksi Absensi Pelajaran"}, []string{"nav-submenu", "KOORDINASI KASUS"}},
		{"Guru Mapel", []string{"guru_mapel"}, []string{"Beranda", "Kehadiran Guru", "Guru Mapel"}, []string{"Kehadiran Pegawai", "Koreksi Absensi Pelajaran"}},
		{"Wali Kelas", []string{"wali_kelas"}, []string{"Beranda", "Kehadiran Guru", "Kasus Siswa", "Kelas Saya", "Kehadiran Siswa"}, []string{"KOORDINASI KASUS", "WALI KELAS"}},
		{"Guru BK", []string{"bk"}, []string{"Beranda", "Kasus Siswa"}, []string{"KOORDINASI KASUS", "Kehadiran Siswa"}},
		{"Kesiswaan", []string{"kesiswaan"}, []string{"Beranda", "Kasus Siswa", "Kehadiran Siswa"}, []string{"KOORDINASI KASUS"}},
		{"Kepala Sekolah", []string{"kepala_sekolah"}, []string{"Beranda", "Kasus Siswa", "Kehadiran Siswa"}, []string{"KOORDINASI KASUS"}},
		{"Pegawai TU", []string{"pegawai_tu"}, []string{"Beranda", "Kehadiran Pegawai"}, []string{"KEHADIRAN PEGAWAI"}},
		{"Pengelola Kehadiran Guru", []string{"pengelola_kehadiran_guru"}, []string{"Beranda", "Penanganan Kehadiran Guru"}, []string{"nav-submenu"}},
		{"Petugas Absensi", []string{"petugas_absensi"}, []string{"Beranda", "Kehadiran Siswa"}, []string{"MENU", "nav-submenu"}},
		{"Guru Mapel + Kesiswaan", []string{"guru_mapel", "kesiswaan"}, []string{"Beranda", "Guru Mapel", "Kehadiran Guru", "Kasus Siswa", "Kehadiran Siswa"}, []string{"KOORDINASI KASUS", "WALI KELAS"}},
		{"Admin + Kurikulum", []string{"admin", "kurikulum"}, []string{"Master Data", "Pengguna &amp; Akses", "Kalender Akademik", "Struktur Kurikulum"}, []string{"nav-submenu"}},
		{"Kurikulum + Guru Mapel", []string{"kurikulum", "guru_mapel"}, []string{"Kehadiran Guru", "Guru Mapel", "Kalender Akademik"}, []string{"Koreksi Absensi Pelajaran"}},
		{"Wali Kelas + Guru Mapel", []string{"wali_kelas", "guru_mapel"}, []string{"Kehadiran Guru", "Guru Mapel", "Kelas Saya", "Kasus Siswa"}, []string{"nav-submenu"}},
		{"TU + Pengelola Kehadiran Guru", []string{"pegawai_tu", "pengelola_kehadiran_guru"}, []string{"Kehadiran Pegawai", "Penanganan Kehadiran Guru"}, []string{"KEHADIRAN PEGAWAI"}},
		{"Kesiswaan + Petugas Absensi", []string{"kesiswaan", "petugas_absensi"}, []string{"Kasus Siswa", "Kehadiran Siswa"}, []string{"KOORDINASI KASUS"}},
	}
	hrefPattern := regexp.MustCompile(`href="([^"]+)"`)
	for _, tc := range roles {
		t.Run(tc.name, func(t *testing.T) {
			body := navigationPageForTest(t, &auth.User{ID: 77, Active: true, Roles: tc.roles}, "/")
			start := strings.Index(body, `<nav aria-label="Menu utama"`)
			if start < 0 {
				t.Fatal("sidebar tidak dirender")
			}
			relativeEnd := strings.Index(body[start:], "</nav>")
			if relativeEnd < 0 {
				t.Fatal("penutup sidebar tidak ditemukan")
			}
			sidebar := body[start : start+relativeEnd]
			if strings.Contains(sidebar, "nav-section-label") {
				t.Fatal("menu mandiri atau parent sidebar memiliki judul kelompok tambahan")
			}
			for _, label := range tc.want {
				if !strings.Contains(sidebar, label) {
					t.Errorf("menu untuk role %v tidak memuat %q", tc.roles, label)
				}
			}
			for _, label := range tc.avoid {
				if strings.Contains(sidebar, label) {
					t.Errorf("menu untuk role %v tidak semestinya memuat %q", tc.roles, label)
				}
			}
			seen := map[string]bool{}
			for _, match := range hrefPattern.FindAllStringSubmatch(sidebar, -1) {
				if seen[match[1]] {
					t.Errorf("route sidebar %q muncul dua kali untuk role %v", match[1], tc.roles)
				}
				seen[match[1]] = true
			}
			if got := strings.Count(sidebar, `aria-current="page"`); got != 1 {
				t.Errorf("Beranda harus menjadi satu-satunya menu aktif pada root; got %d", got)
			}
		})
	}
}

func TestSidebarRoleChangesDoNotExpandServerAuthorization(t *testing.T) {
	called := false
	app := &application{}
	handler := app.require("admin", func(http.ResponseWriter, *http.Request) { called = true })
	r := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &auth.User{ID: 78, Active: true, Roles: []string{"guru_mapel", "kesiswaan"}}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || called {
		t.Fatalf("role gabungan non-Admin mendapat status %d atau handler dijalankan", w.Code)
	}
}
