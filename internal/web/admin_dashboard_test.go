package web

import (
	"context"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
)

func TestAdminDashboardOverviewAndQuickLinks(t *testing.T) {
	admin := &auth.User{ID: 1, Active: true, DisplayName: "Admin Uji", Username: "admin-uji", Roles: []string{"admin"}}
	stats := &master.AdminDashboardStats{
		ActiveStudents: 1234,
		ActiveTeachers: 45,
		ActiveStaff:    12,
		ActiveYear:     "2026/2027",
		HasActiveYear:  true,
		ActiveClasses:  18,
	}
	body := dashboardPageForTest(t, admin, adminDashboardPresentation(*stats))
	for _, want := range []string{
		"Siswa aktif", "1234", "Guru aktif", "45", "Pegawai aktif", "12",
		"Kelas pada tahun aktif", "18", "Tahun ajaran aktif: 2026/2027", "Akses cepat",
		`href="/master/students"`, `href="/master/teachers"`, `href="/admin/users"`, `href="/admin/placements"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard Admin tidak memuat %q", want)
		}
	}
	if strings.Contains(body, "Selamat datang,") || strings.Contains(body, "Muat ulang") || strings.Count(body, `id="overview"`) != 1 {
		t.Fatal("dashboard Admin tidak boleh menampilkan sambutan atau tombol muat ulang")
	}
	start := strings.Index(body, `aria-labelledby="quick-access-title"`)
	if start < 0 {
		t.Fatal("bagian akses cepat tidak dirender")
	}
	quickAccess := body[start:]
	if strings.Contains(quickAccess, `icon-view`) || strings.Count(quickAccess, `class="size-4"`) != 4 || strings.Count(quickAccess, `class="size-4 shrink-0`) != 4 || !strings.Contains(quickAccess, `bg-gray-50`) {
		t.Fatal("kartu akses cepat harus memakai ikon aksen kecil, bukan ikon mata, dan tampilan terpisah dari statistik")
	}
	icons := map[string]string{
		`href="/master/students"`:  `d="M16 20v-1.5`,
		`href="/master/teachers"`:  `d="M4 5.5 12 3`,
		`href="/admin/users"`:      `circle cx="9" cy="8" r="3.5"`,
		`href="/admin/placements"`: `d="M4 7h12`,
	}
	for href, signature := range icons {
		pos := strings.Index(quickAccess, href)
		if pos < 0 {
			t.Errorf("link akses cepat tidak ditemukan: %s", href)
			continue
		}
		end := strings.Index(quickAccess[pos:], `</a>`)
		if end < 0 || !strings.Contains(quickAccess[pos:pos+end], signature) {
			t.Errorf("ikon akses cepat tidak cocok dengan tujuan %s", href)
		}
	}
}

func TestAdminDashboardExplainsMissingActiveYear(t *testing.T) {
	admin := &auth.User{ID: 1, Active: true, Roles: []string{"admin"}}
	stats := &master.AdminDashboardStats{ActiveStudents: 4, ActiveTeachers: 2, ActiveStaff: 1}
	body := dashboardPageForTest(t, admin, adminDashboardPresentation(*stats))
	if !strings.Contains(body, "Belum ada tahun ajaran aktif, jadi total kelas belum dapat dihitung.") || !strings.Contains(body, "Kelas pada tahun aktif") || !strings.Contains(body, "—") {
		t.Fatal("ketiadaan tahun aktif harus dijelaskan dan jumlah kelas tidak boleh tampak sebagai angka pasti")
	}
}

func TestNonAdminHomeKeepsExistingOverview(t *testing.T) {
	user := &auth.User{ID: 2, Active: true, DisplayName: "Guru Uji", Username: "guru-uji", Roles: []string{"guru_mapel"}}
	body := dashboardPageForTest(t, user, nil)
	if !strings.Contains(body, "Selamat datang, Guru Uji") || !strings.Contains(body, "Muat ulang") || strings.Contains(body, "Ringkasan sekolah") || strings.Contains(body, "Akses cepat") {
		t.Fatal("Beranda role selain Admin harus mempertahankan tampilan existing")
	}
}

func dashboardPageForTest(t *testing.T, user *auth.User, dashboard *dashboardPresentation) string {
	t.Helper()
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(int, int) int { return 1 },
		"weekdayName": func(int) string { return "Senin" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates}
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, user))
	w := httptest.NewRecorder()
	app.render(w, r, "page", 200, view{Page: "home", Title: "Beranda", Dashboard: dashboard})
	if w.Code != 200 {
		t.Fatalf("render status=%d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}
