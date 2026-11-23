package web

import (
	"html/template"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

type navigationTab struct {
	ID, Label, Href string
	Icon            template.HTML
}

// sectionNavigationState mengembalikan tab hanya untuk kelompok yang benar-benar memiliki submenu.
func sectionNavigationState(path string, user *auth.User) (section, active string) {
	states := []func(string, *auth.User) (string, string){
		adminNavigationState,
		curriculumNavigationState,
		teacherAttendanceNavigationState,
		subjectTeacherNavigationState,
		teacherManagementNavigationState,
		studentAttendanceNavigationState,
	}
	for _, resolve := range states {
		section, active := resolve(path, user)
		if section != "" && len(sectionNavigationTabs(section, active, user)) > 1 {
			return section, active
		}
	}
	return "", ""
}

func teacherAttendanceNavigationState(path string, user *auth.User) (section, active string) {
	if user == nil {
		return "", ""
	}
	if (user.HasRole("guru_mapel") || user.HasRole("wali_kelas")) && strings.HasPrefix(path, "/kehadiran-saya") {
		if strings.HasPrefix(path, "/kehadiran-saya/rekap") {
			return "Kehadiran Guru", "teacher-recap"
		}
		return "Kehadiran Guru", "teacher-attendance"
	}
	return "", ""
}

func subjectTeacherNavigationState(path string, user *auth.User) (section, active string) {
	if user == nil || !user.HasRole("guru_mapel") {
		return "", ""
	}
	if path == "/jurnal-saya" || strings.HasPrefix(path, "/jurnal-saya/") {
		return "Guru Mapel", "teaching-journal"
	}
	if path == "/guru/tujuan-pembelajaran" || strings.HasPrefix(path, "/guru/tujuan-pembelajaran/") || path == "/guru/kriteria-tp" {
		return "Guru Mapel", "learning-objectives"
	}
	if path == "/guru/nilai-asts-asas" || path == "/guru/nilai-ujian-saya" || strings.HasPrefix(path, "/guru/nilai-ujian/") {
		return "Guru Mapel", "exam-initial-scores"
	}
	if path == "/guru/rekomendasi-semester" || strings.HasPrefix(path, "/guru/rekomendasi-semester/") {
		return "Guru Mapel", "semester-recommendations"
	}
	if path == "/guru/rekap-nilai-tp" {
		return "Guru Mapel", "assessment-tp-recap"
	}
	if path == "/guru/buku-nilai" || strings.HasPrefix(path, "/guru/buku-nilai/") {
		return "Guru Mapel", "direct-tp-gradebook"
	}
	if path == "/guru/kegiatan-penilaian" || strings.HasPrefix(path, "/guru/kegiatan-penilaian/") {
		return "Guru Mapel", "assessment-activities"
	}
	if strings.HasPrefix(path, "/guru/nilai-awal/") && strings.HasSuffix(path, "/remedial") {
		return "Guru Mapel", "assessment-activities"
	}
	if path == "/guru/nilai-awal-saya" || strings.HasPrefix(path, "/guru/nilai-awal/") {
		return "Guru Mapel", "assessment-score-history"
	}
	if path != "/jadwal-saya" && !strings.HasPrefix(path, "/jadwal-saya/") {
		return "", ""
	}
	if strings.HasPrefix(path, "/jadwal-saya/absensi") {
		return "Guru Mapel", "lesson-attendance"
	}
	return "Guru Mapel", "my-schedule"
}

func teacherManagementNavigationState(path string, user *auth.User) (section, active string) {
	if user == nil || !user.HasRole("pengelola_kehadiran_guru") || !strings.HasPrefix(path, "/tu/kehadiran-guru") {
		return "", ""
	}
	if strings.HasPrefix(path, "/tu/kehadiran-guru/rekap") || strings.HasPrefix(path, "/tu/kehadiran-guru/laporan") {
		return "Penanganan Kehadiran Guru", "managed-teacher-recap"
	}
	return "Penanganan Kehadiran Guru", "teacher-attendance-management"
}

func studentAttendanceNavigationState(path string, user *auth.User) (section, active string) {
	if user == nil {
		return "", ""
	}
	switch {
	case strings.HasPrefix(path, "/absensi/harian"):
		if user.HasRole("petugas_absensi") || user.HasRole("wali_kelas") || user.HasRole("kesiswaan") || user.HasRole("kepala_sekolah") {
			return "Kehadiran Siswa", "daily-attendance"
		}
	case strings.HasPrefix(path, "/absensi/pemindai") && user.HasRole("petugas_absensi"):
		return "Kehadiran Siswa", "daily-scanner"
	case strings.HasPrefix(path, "/absensi/kartu") && user.HasRole("petugas_absensi"):
		return "Kehadiran Siswa", "student-cards"
	case strings.HasPrefix(path, "/kesiswaan/batas-kedatangan") && user.HasRole("kesiswaan"):
		return "Kehadiran Siswa", "arrival-rules"
	}
	return "", ""
}

func curriculumNavigationState(path string, user *auth.User) (section, active string) {
	if user == nil || !user.HasRole("kurikulum") {
		return "", ""
	}
	switch {
	case path == "/akademik/kalender-sekolah":
		return "Kalender Akademik", "curriculum-school-calendar"
	case path == "/akademik":
		return "Kalender Akademik", "curriculum-academic"
	case strings.HasPrefix(path, "/akademik/tahun-ajaran"):
		return "Kalender Akademik", "curriculum-years"
	case strings.HasPrefix(path, "/akademik/semester"):
		return "Kalender Akademik", "curriculum-semesters"
	case path == "/akademik/struktur-kurikulum":
		return "Struktur Kurikulum", "curriculum-structure"
	case path == "/master/subjects":
		return "Struktur Kurikulum", "curriculum-subjects"
	case path == "/akademik/pedoman-tp":
		return "Struktur Kurikulum", "curriculum-tp-guidance"
	case path == "/akademik/bobot-penilaian":
		return "Struktur Kurikulum", "curriculum-assessment-weights"
	case path == "/akademik/waktu-sekolah":
		return "Pengaturan Jadwal", "curriculum-school-time"
	case path == "/akademik/batasan-jadwal":
		return "Pengaturan Jadwal", "curriculum-schedule-constraints"
	case path == "/akademik/pengampu":
		return "Pengaturan Jadwal", "curriculum-teaching-assignments"
	case path == "/akademik/jam-pelajaran":
		return "Pengaturan Jadwal", "curriculum-lesson-periods"
	case path == "/akademik/jadwal" || strings.HasPrefix(path, "/akademik/jadwal/"):
		return "Pengaturan Jadwal", "curriculum-weekly-timetable"
	case path == "/akademik/jurnal-lineage":
		return "Pengaturan Jadwal", "curriculum-journal-lineage"
	case strings.HasPrefix(path, "/akademik/transisi"):
		return "Siswa & Penempatan", "curriculum-transitions"
	case strings.HasPrefix(path, "/master/students"), strings.HasPrefix(path, "/students/"):
		return "Siswa & Penempatan", "curriculum-students"
	case strings.HasPrefix(path, "/master/teachers"):
		return "Siswa & Penempatan", "curriculum-teachers"
	case path == "/akademik/kelas" || strings.HasPrefix(path, "/akademik/kelas/") || strings.HasPrefix(path, "/akademik/histori/class/"):
		return "Siswa & Penempatan", "curriculum-classes"
	case strings.HasPrefix(path, "/admin/placements"), strings.HasPrefix(path, "/akademik/penempatan/"), strings.HasPrefix(path, "/akademik/histori/penempatan/"):
		return "Siswa & Penempatan", "curriculum-placements"
	default:
		return "", ""
	}
}

func sectionNavigationTabs(section, active string, user *auth.User) []navigationTab {
	if user != nil && user.HasRole("admin") {
		if tabs := adminNavigationTabs(section, active); len(tabs) > 0 {
			return tabs
		}
	}
	if user == nil {
		return nil
	}
	if (section == "Kalender Akademik" || section == "Struktur Kurikulum" || section == "Pengaturan Jadwal" || section == "Siswa & Penempatan") && !user.HasRole("kurikulum") {
		return nil
	}
	var tabs []navigationTab
	switch section {
	case "Kalender Akademik":
		tabs = []navigationTab{
			{ID: "curriculum-school-calendar", Label: "Kalender Sekolah", Href: "/akademik/kalender-sekolah", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 10h18m-13 4h.01M12 14h.01" stroke-linecap="round"/>`},
			{ID: "curriculum-academic", Label: "Akademik", Href: "/akademik", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 19V5.5A2.5 2.5 0 0 1 6.5 3H20v17H6.5A2.5 2.5 0 0 1 4 17.5M8 8h8m-8 4h8m-8 4h5" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "curriculum-years", Label: "Tahun Ajaran", Href: "/akademik/tahun-ajaran", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 10h18m-13 4h.01M12 14h.01M16 14h.01M8 18h.01M12 18h.01" stroke-linecap="round"/>`},
			{ID: "curriculum-semesters", Label: "Semester", Href: "/akademik/semester", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 6h16M4 12h10M4 18h7" stroke-linecap="round"/><circle cx="18" cy="16" r="3"/><path d="M18 14.5v1.7l1.1.7" stroke-linecap="round"/>`},
		}
	case "Struktur Kurikulum":
		tabs = []navigationTab{
			{ID: "curriculum-tp-guidance", Label: "Pedoman TP", Href: "/akademik/pedoman-tp", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4h14v16H5zM8 8h8m-8 4h8m-8 4h5" stroke-linecap="round"/></svg>`},
			{ID: "curriculum-structure", Label: "Struktur Kurikulum", Href: "/akademik/struktur-kurikulum", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 5h16v14H4zM8 9h8m-8 4h8" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "curriculum-subjects", Label: "Mata Pelajaran", Href: "/master/subjects", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4.5A2.5 2.5 0 0 1 7.5 2H20v18H7.5A2.5 2.5 0 0 0 5 22V4.5ZM5 5h12m-8 4h7m-7 4h7" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "curriculum-assessment-weights", Label: "Bobot Penilaian", Href: "/akademik/bobot-penilaian", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 19h16M6 16V9m6 7V5m6 11v-4" stroke-linecap="round"/><circle cx="6" cy="7" r="1.5"/><circle cx="12" cy="3" r="1.5"/><circle cx="18" cy="10" r="1.5"/></svg>`},
		}
	case "Pengaturan Jadwal":
		tabs = []navigationTab{
			{ID: "curriculum-school-time", Label: "Istirahat dan Kegiatan", Href: "/akademik/waktu-sekolah", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l4 2" stroke-linecap="round"/>`},
			{ID: "curriculum-schedule-constraints", Label: "Batasan Jadwal", Href: "/akademik/batasan-jadwal", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 5h16v14H4zM8 9h8M8 13h5" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "curriculum-teaching-assignments", Label: "Guru Pengampu", Href: "/akademik/pengampu", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="4" y="5" width="16" height="14" rx="1"/><path d="M8 9h8m-8 4h5" stroke-linecap="round"/>`},
			{ID: "curriculum-lesson-periods", Label: "Jam Pelajaran", Href: "/akademik/jam-pelajaran", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2" stroke-linecap="round"/>`},
			{ID: "curriculum-weekly-timetable", Label: "Jadwal Mingguan", Href: "/akademik/jadwal", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 10h18m-13 4h3m3 0h3m-9 3h3" stroke-linecap="round"/>`},
			{ID: "curriculum-journal-lineage", Label: "Jurnal Mengajar", Href: "/akademik/jurnal-lineage", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4h14v16H5zM8 8h8M8 12h8M8 16h5" stroke-linecap="round" stroke-linejoin="round"/>`},
		}
	case "Siswa & Penempatan":
		tabs = []navigationTab{
			{ID: "curriculum-transitions", Label: "Transisi Siswa", Href: "/akademik/transisi", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 7h12m0 0-3-3m3 3-3 3M20 17H8m0 0 3-3m-3 3 3 3" stroke-linecap="round" stroke-linejoin="round"/>`},
		}
		if !user.HasRole("admin") {
			tabs = append(tabs,
				navigationTab{ID: "curriculum-students", Label: "Data Siswa", Href: "/master/students", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M16 20v-1.5a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4V20m7-9a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm6-7.5a4 4 0 0 1 0 7.7M21 20v-1.5a4 4 0 0 0-3-3.87" stroke-linecap="round" stroke-linejoin="round"/>`},
				navigationTab{ID: "curriculum-teachers", Label: "Data Guru", Href: "/master/teachers", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4h14v12H5zM8 20h8m-4-4v4m-4-9 3-3 2 2 2-2" stroke-linecap="round" stroke-linejoin="round"/>`},
				navigationTab{ID: "curriculum-classes", Label: "Kelas", Href: "/akademik/kelas", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3.5" y="4" width="17" height="16" rx="2"/><path d="M3.5 9h17M9 9v11m6-11v11"/>`},
				navigationTab{ID: "curriculum-placements", Label: "Penempatan", Href: "/admin/placements", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M16 20v-1a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v1m7-9a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm11-3v6m-3-3h6" stroke-linecap="round" stroke-linejoin="round"/>`},
			)
		}
	case "Kehadiran Guru":
		if user == nil || (!user.HasRole("guru_mapel") && !user.HasRole("wali_kelas")) {
			return nil
		}
		tabs = []navigationTab{
			{ID: "teacher-attendance", Label: "Kehadiran Saya", Href: "/kehadiran-saya", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 12.5 9.5 17 19 7.5" stroke-linecap="round" stroke-linejoin="round"/><circle cx="12" cy="12" r="9"/>`},
			{ID: "teacher-recap", Label: "Rekap Kehadiran Saya", Href: "/kehadiran-saya/rekap", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="4" y="4" width="16" height="16" rx="2"/><path d="M8 9h8M8 13h5M8 17h3" stroke-linecap="round"/>`},
		}
	case "Guru Mapel":
		if user == nil || !user.HasRole("guru_mapel") {
			return nil
		}
		tabs = []navigationTab{
			{ID: "my-schedule", Label: "Jadwal Saya", Href: "/jadwal-saya", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 10h18m-13 4h3m3 0h3" stroke-linecap="round"/>`},
			{ID: "lesson-attendance", Label: "Absensi Pelajaran", Href: "/jadwal-saya/absensi", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 5h16v14H4zM8 9h8m-8 4h5m3 2 1.5 1.5L20 14" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "teaching-journal", Label: "Jurnal Mengajar", Href: "/jurnal-saya", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4.5A2.5 2.5 0 0 1 7.5 2H20v18H7.5A2.5 2.5 0 0 0 5 22V4.5ZM5 5h12m-9 4h8m-8 4h8m-8 4h5" stroke-linecap="round" stroke-linejoin="round"/>`},
			navigationTab{ID: "learning-objectives", Label: "Tujuan Pembelajaran", Href: "/guru/tujuan-pembelajaran", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4h14v16H5zM8 8h8M8 12h8M8 16h5" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "direct-tp-gradebook", Label: "Buku Nilai", Href: "/guru/buku-nilai", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 4h16v16H4zM4 9h16M10 9v11" stroke-linecap="round"/></svg>`},
			navigationTab{ID: "assessment-activities", Label: "Kegiatan Penilaian", Href: "/guru/kegiatan-penilaian", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M6 3h12a2 2 0 0 1 2 2v16H4V5a2 2 0 0 1 2-2Zm2 5h8m-8 4h8m-8 4h5" stroke-linecap="round" stroke-linejoin="round"/>`},
			navigationTab{ID: "assessment-score-history", Label: "Nilai Milik Saya", Href: "/guru/nilai-awal-saya", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4h14v16H5zM8 8h8m-8 4h4m-4 4h8" stroke-linecap="round" stroke-linejoin="round"/><path d="m14 11 2 2 4-5" stroke-linecap="round" stroke-linejoin="round"/>`},

			navigationTab{ID: "exam-initial-scores", Label: "Nilai ASTS/ASAS", Href: "/guru/nilai-asts-asas", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4h14v16H5zM8 8h8m-8 4h8m-8 4h5" stroke-linecap="round"/></svg>`},
			navigationTab{ID: "semester-recommendations", Label: "Rekomendasi Semester", Href: "/guru/rekomendasi-semester", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4h14v16H5zM8 8h8m-8 4h8m-8 4h5" stroke-linecap="round"/></svg>`},
			navigationTab{ID: "assessment-tp-recap", Label: "Rekap Nilai TP/NH", Href: "/guru/rekap-nilai-tp", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 19.5V4.5M4 19.5h17M8 16v-4m4 4V7m4 9v-6m4 6V4" stroke-linecap="round" stroke-linejoin="round"/>`},
		}
	case "Penanganan Kehadiran Guru":
		if user == nil || !user.HasRole("pengelola_kehadiran_guru") {
			return nil
		}
		tabs = []navigationTab{
			{ID: "teacher-attendance-management", Label: "Kelola Kehadiran Guru", Href: "/tu/kehadiran-guru", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 5h16v14H4zM8 9h8m-8 4h5" stroke-linecap="round"/>`},
		}
		if user.HasRole("pegawai_tu") {
			tabs = append(tabs, navigationTab{ID: "managed-teacher-recap", Label: "Rekap Kehadiran Guru", Href: "/tu/kehadiran-guru/rekap", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="4" y="4" width="16" height="16" rx="2"/><path d="M8 9h8M8 13h5M8 17h3" stroke-linecap="round"/>`})
		}
	case "Kehadiran Siswa":
		if user == nil {
			return nil
		}
		if user.HasRole("petugas_absensi") || user.HasRole("wali_kelas") || user.HasRole("kesiswaan") || user.HasRole("kepala_sekolah") {
			tabs = append(tabs, navigationTab{ID: "daily-attendance", Label: "Absensi Harian", Href: "/absensi/harian", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 10h18m-13 4h3m3 0h3" stroke-linecap="round"/>`})
		}
		if user.HasRole("petugas_absensi") {
			tabs = append(tabs,
				navigationTab{ID: "daily-scanner", Label: "Pemindai QR", Href: "/absensi/pemindai", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h2v2h-2zM18 14h2v6h-6v-2" stroke-linecap="round" stroke-linejoin="round"/>`},
				navigationTab{ID: "student-cards", Label: "Kartu QR Siswa", Href: "/absensi/kartu", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 9h4v4H7zM14 9h3m-3 3h3m-10 4h10" stroke-linecap="round"/>`},
			)
		}
		if user.HasRole("kesiswaan") {
			tabs = append(tabs, navigationTab{ID: "arrival-rules", Label: "Batas Kedatangan", Href: "/kesiswaan/batas-kedatangan", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2m-3-8v1" stroke-linecap="round" stroke-linejoin="round"/>`})
		}
	default:
		return nil
	}
	for i := range tabs {
		tabs[i].Icon += `</svg>`
	}
	return tabs
}

// adminNavigationState mempertahankan konteks kelompok saat halaman dibuka langsung.
func adminNavigationState(path string, user *auth.User) (section, active string) {
	if user == nil || !user.HasRole("admin") {
		return "", ""
	}
	switch {
	case strings.HasPrefix(path, "/master/students"), strings.HasPrefix(path, "/students/"):
		return "Master Data", "admin-students"
	case strings.HasPrefix(path, "/master/teachers"):
		return "Master Data", "admin-teachers"
	case strings.HasPrefix(path, "/master/staff"):
		return "Master Data", "admin-staff"
	case path == "/akademik/kelas" || strings.HasPrefix(path, "/akademik/kelas/") || strings.HasPrefix(path, "/akademik/histori/class/"):
		return "Master Data", "admin-classes"
	case strings.HasPrefix(path, "/admin/placements"), strings.HasPrefix(path, "/akademik/penempatan/"), strings.HasPrefix(path, "/akademik/histori/penempatan/"):
		return "Master Data", "admin-placements"
	case strings.HasPrefix(path, "/admin/users"):
		return "Pengguna & Akses", "admin-users"
	case strings.HasPrefix(path, "/admin/audit"):
		return "Pengguna & Akses", "admin-audit"
	case strings.HasPrefix(path, "/admin/kehadiran-tu"):
		return "Kehadiran", "admin-staff-attendance"
	case strings.HasPrefix(path, "/admin/attendance/location"):
		return "Kehadiran", "admin-teacher-attendance-location"
	default:
		return "", ""
	}
}

// adminNavigationTabs menyimpan satu kontrak link agar daftar tab tidak berulang di halaman.
func adminNavigationTabs(section, active string) []navigationTab {
	var tabs []navigationTab
	switch section {
	case "Master Data":
		tabs = []navigationTab{
			{ID: "admin-students", Label: "Siswa", Href: "/master/students", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M16 20v-1.5a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4V20m7-9a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm6-7.5a4 4 0 0 1 0 7.7M21 20v-1.5a4 4 0 0 0-3-3.87" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "admin-teachers", Label: "Guru", Href: "/master/teachers", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 5.5 12 3l8 2.5v6c0 4.5-3.3 7.5-8 9.5-4.7-2-8-5-8-9.5v-6Z" stroke-linejoin="round"/><path d="M9 10a3 3 0 1 0 6 0 3 3 0 0 0-6 0Zm-1 7c.8-1.4 2.2-2.2 4-2.2s3.2.8 4 2.2" stroke-linecap="round"/>`},
			{ID: "admin-staff", Label: "Pegawai", Href: "/master/staff", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="4" y="7" width="16" height="13" rx="2"/><path d="M9 7V5a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2M4 12h16m-10 0v2h4v-2" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "admin-classes", Label: "Kelas", Href: "/akademik/kelas", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3.5" y="4" width="17" height="16" rx="2"/><path d="M3.5 9h17M9 9v11m6-11v11" stroke-linecap="round"/>`},
			{ID: "admin-placements", Label: "Penempatan Siswa", Href: "/admin/placements", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 7h12m0 0-3-3m3 3-3 3M20 17H8m0 0 3-3m-3 3 3 3" stroke-linecap="round" stroke-linejoin="round"/>`},
		}
	case "Pengguna & Akses":
		tabs = []navigationTab{
			{ID: "admin-users", Label: "Pengguna & Role", Href: "/admin/users", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="9" cy="8" r="3.5"/><path d="M3 20v-1a6 6 0 0 1 12 0v1m3-13v6m-3-3h6" stroke-linecap="round"/>`},
			{ID: "admin-audit", Label: "Riwayat Akses", Href: "/admin/audit", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M12 3 20 6v5c0 5-3.5 8-8 10-4.5-2-8-5-8-10V6l8-3Z" stroke-linejoin="round"/><path d="m8.5 12 2.2 2.2 4.8-5" stroke-linecap="round" stroke-linejoin="round"/>`},
		}
	case "Kehadiran":
		tabs = []navigationTab{
			{ID: "admin-staff-attendance", Label: "Penanganan Kehadiran TU", Href: "/admin/kehadiran-tu", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="4" y="5" width="16" height="15" rx="2"/><path d="M8 3v4m8-4v4M4 10h16m-11 4 2 2 4-4" stroke-linecap="round" stroke-linejoin="round"/>`},
			{ID: "admin-teacher-attendance-location", Label: "Lokasi Kehadiran Guru", Href: "/admin/attendance/location", Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M20 10c0 5-8 11-8 11S4 15 4 10a8 8 0 1 1 16 0Z" stroke-linejoin="round"/><circle cx="12" cy="10" r="2.5"/>`},
		}
	default:
		return nil
	}
	for i := range tabs {
		tabs[i].Icon += `</svg>`
	}
	return tabs
}
