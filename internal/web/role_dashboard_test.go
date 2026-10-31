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
