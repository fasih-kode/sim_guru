package web

import (
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestCurriculumDashboardOverviewAndQuickLinks(t *testing.T) {
	user := &auth.User{ID: 3, Active: true, DisplayName: "Kurikulum Uji", Username: "kurikulum-uji", Roles: []string{"kurikulum", "guru_mapel"}}
	stats := academic.CurriculumDashboardStats{
		ActiveYear: "2026/2027", HasActiveYear: true, ActiveSemester: 1, SemesterYear: "2026/2027",
		HasActiveSemester: true, ActiveClasses: 6, HasPublishedTimetable: true,
	}
	body := dashboardPageForTest(t, user, curriculumDashboardPresentation(stats))
	for _, want := range []string{
		"Ringkasan akademik", "Tahun ajaran aktif", "2026/2027", "Semester aktif", "Semester 1",
		"Kelas pada tahun aktif", ">6<", "Jadwal semester aktif", "Tersedia", "Jadwal resmi terbit dan sudah berlaku.",
		`href="/akademik/kalender-sekolah"`, `href="/akademik/struktur-kurikulum"`,
		`href="/akademik/pengampu"`, `href="/akademik/jadwal"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard Kurikulum tidak memuat %q", want)
		}
	}
	if strings.Contains(body, "Ringkasan sekolah") || strings.Contains(body, "Data Siswa") || strings.Contains(body, "/admin/users") {
		t.Fatal("dashboard Kurikulum membocorkan konten atau tautan Admin")
	}
	quick := body[strings.Index(body, `aria-labelledby="quick-access-title"`):]
	icons := map[string]string{
		`href="/akademik/kalender-sekolah"`:   `rect x="3" y="5" width="18" height="16"`,
		`href="/akademik/struktur-kurikulum"`: `M4 5h16v14H4z`,
		`href="/akademik/pengampu"`:           `rect x="4" y="5" width="16" height="14"`,
		`href="/akademik/jadwal"`:             `M3 10h18m-13 4h3`,
	}
	for href, signature := range icons {
		pos := strings.Index(quick, href)
		if pos < 0 {
			t.Errorf("akses cepat tidak ditemukan: %s", href)
			continue
		}
		end := strings.Index(quick[pos:], `</a>`)
		if end < 0 || !strings.Contains(quick[pos:pos+end], signature) {
			t.Errorf("ikon akses cepat tidak sesuai navigasi %s", href)
		}
	}
}

func TestCurriculumDashboardExplainsUnavailableData(t *testing.T) {
	user := &auth.User{ID: 3, Active: true, Roles: []string{"kurikulum"}}
	body := dashboardPageForTest(t, user, curriculumDashboardPresentation(academic.CurriculumDashboardStats{}))
	for _, want := range []string{
		"Belum aktif", "Aktifkan tahun ajaran melalui Kalender Akademik.",
		"Semester aktif", "Pilih semester pada tahun ajaran aktif.",
		"Jumlah kelas tersedia setelah tahun ajaran aktif.", "Semester aktif belum tersedia.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("penjelasan kondisi kosong tidak ditemukan: %q", want)
		}
	}
	if strings.Contains(body, ">Terbit<") || strings.Contains(body, ">0<") {
		t.Fatal("status/angka jadwal atau kelas tidak boleh dibuat ketika periode aktif tidak tersedia")
	}
}

func TestHomeDashboardRolePriorityAndIsolation(t *testing.T) {
	if got := homeDashboardKind(nil); got != "" {
		t.Fatalf("pengguna anonim mendapat dashboard %q", got)
	}
	if got := homeDashboardKind(&auth.User{Roles: []string{"guru_mapel"}}); got != "guru-mapel" {
		t.Fatalf("Guru Mapel seharusnya mendapat Beranda Guru Mapel, got %q", got)
	}
	if got := homeDashboardKind(&auth.User{Roles: []string{"wali_kelas"}}); got != "wali-kelas" {
		t.Fatalf("Wali Kelas seharusnya mendapat Beranda Wali Kelas, got %q", got)
	}
	if got := homeDashboardKind(&auth.User{Roles: []string{"guru_mapel", "wali_kelas"}}); got != "guru-wali" {
		t.Fatalf("Guru Mapel + Wali Kelas harus melihat kedua ringkasan, got %q", got)
	}
	if got := homeDashboardKind(&auth.User{Roles: []string{"kurikulum", "guru_mapel"}}); got != "kurikulum" {
		t.Fatalf("Kurikulum + Guru Mapel seharusnya mendapat dashboard Kurikulum, got %q", got)
	}
	if got := homeDashboardKind(&auth.User{Roles: []string{"admin", "kurikulum"}}); got != "admin" {
		t.Fatalf("Admin + Kurikulum harus mempertahankan prioritas Beranda Admin, got %q", got)
	}
	for _, tc := range []struct {
		roles []string
		want  string
	}{
		{[]string{"bk"}, "bk"},
		{[]string{"kesiswaan"}, "kesiswaan"},
		{[]string{"kepala_sekolah"}, "kepala-sekolah"},
		{[]string{"bk", "kesiswaan"}, "support-combined"},
		{[]string{"kesiswaan", "kepala_sekolah"}, "support-combined"},
		{[]string{"guru_mapel", "bk"}, "guru-mapel"},
		{[]string{"wali_kelas", "kepala_sekolah"}, "wali-kelas"},
		{[]string{"admin", "kepala_sekolah"}, "admin"},
	} {
		if got := homeDashboardKind(&auth.User{Roles: tc.roles}); got != tc.want {
			t.Errorf("prioritas role %v: got %q want %q", tc.roles, got, tc.want)
		}
	}
}
