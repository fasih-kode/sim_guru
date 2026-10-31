package web

import (
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestTeacherDashboardShowsScheduledMeetingsAndOwnAttendance(t *testing.T) {
	user := &auth.User{ID: 7, Active: true, Roles: []string{"guru_mapel"}}
	stats := academic.TeacherDashboardStats{
		Date: "2026-10-31", HasActiveSemester: true, HasPublishedTimetable: true,
		TodayMeetings: 2, WeeklyMeetings: 11, ActiveClasses: 4, HasCheckIn: true,
	}
	body := dashboardPageForTest(t, user, teacherDashboardPresentation(stats))
	for _, want := range []string{
		"Pertemuan terjadwal hari ini", ">2<", "Pertemuan terjadwal per minggu", ">11<",
		"bukan jumlah yang telah terlaksana", "Kelas dengan penugasan aktif", ">4<",
		"Presensi pribadi hari ini", "Masuk tercatat", "2026-10-31 WIB",
		`href="/jadwal-saya"`, `href="/jadwal-saya/absensi"`, `href="/kehadiran-saya"`, `href="/kehadiran-saya/rekap"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Beranda Guru Mapel tidak memuat %q", want)
		}
	}
}

func TestTeacherDashboardUnavailableScheduleIsNotPresentedAsZero(t *testing.T) {
	body := dashboardPageForTest(t, &auth.User{ID: 7, Roles: []string{"guru_mapel"}}, teacherDashboardPresentation(academic.TeacherDashboardStats{Date: "2026-10-31"}))
	for _, want := range []string{"Belum tersedia", "Belum ada jadwal terbit yang berlaku", "Belum dicatat"} {
		if !strings.Contains(body, want) {
			t.Errorf("kondisi jadwal/presensi kosong tidak dijelaskan: %q", want)
		}
	}
	if strings.Contains(body, ">0<") {
		t.Fatal("ketiadaan semester atau jadwal tidak boleh ditampilkan sebagai angka nol")
	}
}

func TestHomeroomDashboardEmptyAndPrivateCaseBoundary(t *testing.T) {
	stats := academic.HomeroomDashboardStats{Date: "2026-10-31"}
	body := dashboardPageForTest(t, &auth.User{ID: 8, Roles: []string{"wali_kelas"}}, homeroomDashboardPresentation(stats))
	for _, want := range []string{
		"Belum ada penugasan Wali Kelas aktif", "Kelas binaan aktif", "Siswa pada kelas binaan",
		"Belum ada kelas binaan aktif", "Kasus mengikuti cakupan pemantauan aktif D-042",
		`href="/wali-kelas"`, `href="/absensi/harian"`, `href="/kesiswaan/kasus"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("kondisi kosong Wali Kelas tidak memuat %q", want)
		}
	}
	for _, privateText := range []string{"catatan konseling", "isi konseling", "layanan konseling"} {
		if strings.Contains(strings.ToLower(body), privateText) {
			t.Fatalf("Beranda Wali Kelas memuat informasi konseling pribadi: %q", privateText)
		}
	}
}

func TestCombinedTeacherHomeroomDashboardKeepsScopesSeparate(t *testing.T) {
	user := &auth.User{ID: 9, Roles: []string{"guru_mapel", "wali_kelas"}}
	teacher := teacherDashboardPresentation(academic.TeacherDashboardStats{Date: "2026-10-31", HasPublishedTimetable: true, TodayMeetings: 1, WeeklyMeetings: 5})
	wali := homeroomDashboardPresentation(academic.HomeroomDashboardStats{Date: "2026-10-31", HasHomeroom: true, Classes: 1, Students: 20, OpenCases: 2})
	body := dashboardPageWithSectionsForTest(t, user, teacher, []*dashboardPresentation{wali})
	for _, want := range []string{"Ringkasan Guru Mapel", "Ringkasan Wali Kelas", ">1<", ">20<", ">2<", "Jadwal Saya", "Kasus Siswa"} {
		if !strings.Contains(body, want) {
			t.Errorf("Beranda role gabungan tidak memuat %q", want)
		}
	}
	for _, id := range []string{`id="overview"`, `id="teacher-overview-title"`, `id="teacher-quick-access-title"`, `id="homeroom-overview-title"`, `id="homeroom-quick-access-title"`} {
		if strings.Count(body, id) != 1 {
			t.Errorf("ID dashboard gabungan harus unik satu kali: %s", id)
		}
	}
	if strings.Contains(body, "Ringkasan sekolah") || strings.Contains(body, "Ringkasan akademik") {
		t.Fatal("role gabungan tidak boleh mengambil statistik Admin/Kurikulum")
	}
}

func TestSupportDashboardsUseScopedLinksAndNoCounselingContent(t *testing.T) {
	const privateNote = "RAHASIA_KONSELING_FIXTURE"
	bk := dashboardPageForTest(t, &auth.User{ID: 9, Roles: []string{"bk"}}, counselorDashboardPresentation(academic.CaseDashboardStats{Cases: 3, OpenCases: 2, CompletedCases: 1}))
	for _, want := range []string{"Kasus tahap BK", "Kasus Siswa", "/kesiswaan/kasus", ">3<", ">2<", ">1<"} {
		if !strings.Contains(bk, want) {
			t.Errorf("Beranda BK tidak memuat %q", want)
		}
	}
	if strings.Contains(bk, privateNote) || strings.Contains(bk, "Batas Kedatangan") || strings.Contains(bk, "/absensi/harian") {
		t.Fatal("Beranda BK menampilkan konten privat atau tautan di luar cakupan")
	}

	kesiswaan := dashboardPageForTest(t, &auth.User{ID: 10, Roles: []string{"kesiswaan"}}, studentAffairsDashboardPresentation(academic.SchoolMonitoringDashboardStats{Date: "2026-10-31", HasActiveYear: true, ActiveStudents: 20, Recorded: 12, Present: 10, Sick: 1, Excused: 1, OpenCases: 2, HasAttendance: true}))
	for _, want := range []string{"Siswa pada roster aktif", "Absensi tercatat hari ini", "Belum tercatat hari ini", "Kasus perlu tindak lanjut", "/absensi/harian", "/kesiswaan/kasus", "/kesiswaan/batas-kedatangan", "Hadir 10 · Sakit 1 · Izin 1 · Alpa 0"} {
		if !strings.Contains(kesiswaan, want) {
			t.Errorf("Beranda Kesiswaan tidak memuat %q", want)
		}
	}
	if strings.Contains(kesiswaan, privateNote) {
		t.Fatal("Beranda Kesiswaan membocorkan isi konseling")
	}
	if count := dashboardLinkCount(t, kesiswaan, `/kesiswaan/kasus`); count != 1 {
		t.Fatalf("bagian Kesiswaan harus menampilkan satu tautan Kasus Siswa, got %d", count)
	}

	empty := dashboardPageForTest(t, &auth.User{ID: 11, Roles: []string{"kepala_sekolah"}}, headmasterDashboardPresentation(academic.SchoolMonitoringDashboardStats{Date: "2026-10-31", OpenCases: 0}))
	for _, want := range []string{"Tahun ajaran aktif belum tersedia", "Belum tersedia", "Kasus perlu tindak lanjut", "/absensi/harian", "/kesiswaan/kasus"} {
		if !strings.Contains(empty, want) {
			t.Errorf("Beranda Kepala Sekolah tidak menangani kondisi kosong %q", want)
		}
	}
	if strings.Contains(empty, "Batas Kedatangan") || strings.Contains(empty, privateNote) {
		t.Fatal("Beranda Kepala Sekolah memuat tautan operasional atau isi konseling")
	}
}

func TestKesiswaanCaseQuickLinkAppearsOnceForCombinedRoles(t *testing.T) {
	user := &auth.User{ID: 12, Roles: []string{"guru_mapel", "kesiswaan"}}
	dashboard := teacherDashboardPresentation(academic.TeacherDashboardStats{Date: "2026-10-31"})
	page := view{Dashboard: dashboard}
	ensureKesiswaanCaseQuickLink(&page, user)
	ensureKesiswaanCaseQuickLink(&page, user)
	body := dashboardPageForTest(t, user, page.Dashboard)
	if count := dashboardLinkCount(t, body, `/kesiswaan/kasus`); count != 1 {
		t.Fatalf("role gabungan Guru Mapel + Kesiswaan harus mendapat satu tautan Kasus Siswa, got %d", count)
	}

	homeroom := homeroomDashboardPresentation(academic.HomeroomDashboardStats{Date: "2026-10-31"})
	page = view{Dashboard: teacherDashboardPresentation(academic.TeacherDashboardStats{Date: "2026-10-31"}), AdditionalDashboards: []*dashboardPresentation{homeroom}}
	ensureKesiswaanCaseQuickLink(&page, &auth.User{ID: 13, Roles: []string{"guru_mapel", "wali_kelas", "kesiswaan"}})
	combined := dashboardPageWithSectionsForTest(t, &auth.User{ID: 13, Roles: []string{"guru_mapel", "wali_kelas", "kesiswaan"}}, page.Dashboard, page.AdditionalDashboards)
	if count := dashboardLinkCount(t, combined, `/kesiswaan/kasus`); count != 1 {
		t.Fatalf("tautan pada bagian Wali Kelas tidak boleh diduplikasi pada bagian Guru Mapel, got %d", count)
	}
}

func dashboardLinkCount(t *testing.T, body, href string) int {
	t.Helper()
	start := strings.Index(body, `<div id="overview"`)
	if start < 0 {
		t.Fatal("konten Beranda tidak ditemukan")
	}
	return strings.Count(body[start:], `href="`+href+`"`)
}
