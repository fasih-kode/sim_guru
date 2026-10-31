package web

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/master"
)

type dashboardStatCard struct {
	Label, Value, Detail string
	Icon                 template.HTML
	IconBackground       string
	IconColor            string
}

type dashboardQuickLink struct {
	Label string
	Href  string
	Icon  template.HTML
}

type dashboardPresentation struct {
	Eyebrow, Heading, Context, Notice string
	TitleID, QuickAccessID            string
	Cards                             []dashboardStatCard
	QuickLinks                        []dashboardQuickLink
}

func adminDashboardPresentation(stats master.AdminDashboardStats) *dashboardPresentation {
	yearContext := ""
	classValue := "—"
	notice := ""
	if stats.HasActiveYear {
		yearContext = "Tahun ajaran aktif: " + stats.ActiveYear
		classValue = fmt.Sprint(stats.ActiveClasses)
	} else {
		notice = "Belum ada tahun ajaran aktif, jadi total kelas belum dapat dihitung."
	}
	return &dashboardPresentation{
		Eyebrow: "Ringkasan sekolah", Heading: "Data utama", Context: yearContext, Notice: notice,
		TitleID: "overview-title", QuickAccessID: "quick-access-title",
		Cards: []dashboardStatCard{
			{Label: "Siswa aktif", Value: fmt.Sprint(stats.ActiveStudents), Icon: dashboardIconPeople, IconBackground: "bg-brand-50", IconColor: "text-brand-600"},
			{Label: "Guru aktif", Value: fmt.Sprint(stats.ActiveTeachers), Icon: dashboardIconTeacher, IconBackground: "bg-green-50", IconColor: "text-green-700"},
			{Label: "Pegawai aktif", Value: fmt.Sprint(stats.ActiveStaff), Icon: dashboardIconStaff, IconBackground: "bg-orange-50", IconColor: "text-orange-600"},
			{Label: "Kelas pada tahun aktif", Value: classValue, Icon: dashboardIconClasses, IconBackground: "bg-blue-50", IconColor: "text-blue-600"},
		},
		QuickLinks: []dashboardQuickLink{
			{Label: "Data Siswa", Href: "/master/students", Icon: smallDashboardIcon(dashboardIconPeople)},
			{Label: "Data Guru", Href: "/master/teachers", Icon: smallDashboardIcon(dashboardIconTeacherNav)},
			{Label: "Pengguna & Role", Href: "/admin/users", Icon: smallDashboardIcon(dashboardIconUsers)},
			{Label: "Penempatan Siswa", Href: "/admin/placements", Icon: smallDashboardIcon(dashboardIconPlacement)},
		},
	}
}

func curriculumDashboardPresentation(stats academic.CurriculumDashboardStats) *dashboardPresentation {
	yearValue, yearDetail := "Belum aktif", "Aktifkan tahun ajaran melalui Kalender Akademik."
	if stats.HasActiveYear {
		yearValue, yearDetail = stats.ActiveYear, "Tahun ajaran yang sedang digunakan."
	}
	semesterValue, semesterDetail := "Belum tersedia", "Pilih semester pada tahun ajaran aktif."
	if stats.HasActiveSemester {
		semesterValue = fmt.Sprintf("Semester %d", stats.ActiveSemester)
		semesterDetail = "Tahun ajaran " + stats.SemesterYear
	}
	classValue, classDetail := "—", "Jumlah kelas tersedia setelah tahun ajaran aktif."
	if stats.HasActiveYear {
		classValue = fmt.Sprint(stats.ActiveClasses)
		classDetail = "Pada tahun ajaran " + stats.ActiveYear
	}
	timetableValue, timetableDetail := "Belum tersedia", "Semester aktif belum tersedia."
	if stats.HasActiveSemester {
		timetableValue, timetableDetail = "Belum terbit", "Belum ada revisi jadwal terbit yang berlaku untuk semester aktif."
		if stats.HasPublishedTimetable {
			timetableValue, timetableDetail = "Tersedia", "Jadwal resmi terbit dan sudah berlaku."
		}
	}
	return &dashboardPresentation{
		Eyebrow: "Ringkasan akademik", Heading: "Periode berjalan",
		TitleID: "overview-title", QuickAccessID: "quick-access-title",
		Cards: []dashboardStatCard{
			{Label: "Tahun ajaran aktif", Value: yearValue, Detail: yearDetail, Icon: dashboardIconCalendar, IconBackground: "bg-brand-50", IconColor: "text-brand-600"},
			{Label: "Semester aktif", Value: semesterValue, Detail: semesterDetail, Icon: dashboardIconSemester, IconBackground: "bg-green-50", IconColor: "text-green-700"},
			{Label: "Kelas pada tahun aktif", Value: classValue, Detail: classDetail, Icon: dashboardIconClasses, IconBackground: "bg-blue-50", IconColor: "text-blue-600"},
			{Label: "Jadwal semester aktif", Value: timetableValue, Detail: timetableDetail, Icon: dashboardIconTimetable, IconBackground: "bg-orange-50", IconColor: "text-orange-600"},
		},
		QuickLinks: []dashboardQuickLink{
			{Label: "Kalender Akademik", Href: "/akademik/kalender-sekolah", Icon: smallDashboardIcon(dashboardIconCalendar)},
			{Label: "Struktur Kurikulum", Href: "/akademik/struktur-kurikulum", Icon: smallDashboardIcon(dashboardIconStructure)},
			{Label: "Guru Pengampu", Href: "/akademik/pengampu", Icon: smallDashboardIcon(dashboardIconAssignment)},
			{Label: "Jadwal Mingguan", Href: "/akademik/jadwal", Icon: smallDashboardIcon(dashboardIconTimetable)},
		},
	}
}

func teacherDashboardPresentation(stats academic.TeacherDashboardStats) *dashboardPresentation {
	meetingsToday, meetingsTodayDetail := "Belum tersedia", "Belum ada jadwal terbit yang berlaku untuk semester aktif."
	weeklyMeetings, weeklyDetail := "Belum tersedia", "Jumlah pertemuan pada jadwal terbit yang berlaku."
	classes, classesDetail := "Belum tersedia", "Penugasan aktif pada semester berjalan."
	if stats.HasActiveSemester {
		classes, classesDetail = fmt.Sprint(stats.ActiveClasses), "Kelas dengan penugasan mengajar aktif."
	}
	if stats.HasPublishedTimetable {
		meetingsToday = fmt.Sprint(stats.TodayMeetings)
		meetingsTodayDetail = "Pertemuan terjadwal pada " + stats.Date + " (WIB); bukan jumlah yang telah terlaksana."
		weeklyMeetings = fmt.Sprint(stats.WeeklyMeetings)
		weeklyDetail = "Pertemuan terjadwal per minggu pada jadwal terbit yang berlaku."
	}
	attendance := "Belum dicatat"
	switch {
	case stats.HasCheckIn && stats.HasCheckOut:
		attendance = "Masuk & Pulang"
	case stats.HasCheckIn:
		attendance = "Masuk tercatat"
	case stats.HasCheckOut:
		attendance = "Pulang tercatat"
	}
	return &dashboardPresentation{
		Eyebrow: "Ringkasan Guru Mapel", Heading: "Jadwal dan kehadiran pribadi",
		TitleID: "teacher-overview-title", QuickAccessID: "teacher-quick-access-title",
		Cards: []dashboardStatCard{
			{Label: "Pertemuan terjadwal hari ini", Value: meetingsToday, Detail: meetingsTodayDetail, Icon: dashboardIconTimetable, IconBackground: "bg-brand-50", IconColor: "text-brand-600"},
			{Label: "Pertemuan terjadwal per minggu", Value: weeklyMeetings, Detail: weeklyDetail, Icon: dashboardIconCalendar, IconBackground: "bg-blue-50", IconColor: "text-blue-600"},
			{Label: "Kelas dengan penugasan aktif", Value: classes, Detail: classesDetail, Icon: dashboardIconClasses, IconBackground: "bg-green-50", IconColor: "text-green-700"},
			{Label: "Presensi pribadi hari ini", Value: attendance, Detail: "Status untuk " + stats.Date + " WIB.", Icon: dashboardIconTeacher, IconBackground: "bg-orange-50", IconColor: "text-orange-600"},
		},
		QuickLinks: []dashboardQuickLink{
			{Label: "Jadwal Saya", Href: "/jadwal-saya", Icon: smallDashboardIcon(dashboardIconTimetable)},
			{Label: "Absensi Pelajaran", Href: "/jadwal-saya/absensi", Icon: smallDashboardIcon(dashboardIconCheck)},
			{Label: "Kehadiran Saya", Href: "/kehadiran-saya", Icon: smallDashboardIcon(dashboardIconTeacher)},
			{Label: "Rekap Kehadiran Saya", Href: "/kehadiran-saya/rekap", Icon: smallDashboardIcon(dashboardIconCalendar)},
		},
	}
}

func homeroomDashboardPresentation(stats academic.HomeroomDashboardStats) *dashboardPresentation {
	classValue, classDetail := "Belum tersedia", "Belum ada penugasan Wali Kelas aktif pada tahun ajaran aktif."
	studentValue, studentDetail := "Belum tersedia", "Jumlah siswa mengikuti penempatan aktif pada kelas binaan."
	attendanceValue, attendanceDetail := "Belum tersedia", "Belum ada kelas binaan aktif."
	caseValue, caseDetail := "Belum tersedia", "Kasus mengikuti cakupan pemantauan aktif D-042."
	if stats.HasHomeroom {
		classValue = fmt.Sprint(stats.Classes)
		classDetail = strings.Join(stats.ClassNames, ", ")
		studentValue = fmt.Sprint(stats.Students)
		if stats.Students == 0 {
			studentDetail = "Belum ada siswa pada penempatan aktif kelas binaan."
		}
		if stats.HasAttendance {
			attendanceValue = fmt.Sprintf("%d tercatat", stats.AttendanceRecorded)
			attendanceDetail = fmt.Sprintf("Hadir %d · Sakit %d · Izin %d · Alpa %d", stats.Present, stats.Sick, stats.Excused, stats.Absent)
		} else {
			attendanceValue, attendanceDetail = "Belum ada data", "Belum ada pencatatan absensi siswa pada "+stats.Date+" WIB."
		}
		caseValue = fmt.Sprint(stats.OpenCases)
		caseDetail = "Kasus Perlu tindak lanjut dalam cakupan kelas binaan aktif."
	}
	return &dashboardPresentation{
		Eyebrow: "Ringkasan Wali Kelas", Heading: "Kelas binaan dan pemantauan",
		TitleID: "homeroom-overview-title", QuickAccessID: "homeroom-quick-access-title",
		Cards: []dashboardStatCard{
			{Label: "Kelas binaan aktif", Value: classValue, Detail: classDetail, Icon: dashboardIconClasses, IconBackground: "bg-brand-50", IconColor: "text-brand-600"},
			{Label: "Siswa pada kelas binaan", Value: studentValue, Detail: studentDetail, Icon: dashboardIconPeople, IconBackground: "bg-green-50", IconColor: "text-green-700"},
			{Label: "Kehadiran siswa hari ini", Value: attendanceValue, Detail: attendanceDetail, Icon: dashboardIconCalendar, IconBackground: "bg-blue-50", IconColor: "text-blue-600"},
			{Label: "Kasus perlu tindak lanjut", Value: caseValue, Detail: caseDetail, Icon: dashboardIconCase, IconBackground: "bg-orange-50", IconColor: "text-orange-600"},
		},
		QuickLinks: []dashboardQuickLink{
			{Label: "Kelas Saya", Href: "/wali-kelas", Icon: smallDashboardIcon(dashboardIconClasses)},
			{Label: "Kehadiran Siswa", Href: "/absensi/harian", Icon: smallDashboardIcon(dashboardIconCalendar)},
			{Label: "Kasus Siswa", Href: "/kesiswaan/kasus", Icon: smallDashboardIcon(dashboardIconCase)},
		},
	}
}

func counselorDashboardPresentation(stats academic.CaseDashboardStats) *dashboardPresentation {
	return &dashboardPresentation{
		Eyebrow: "Ringkasan BK", Heading: "Kasus dalam penanganan Anda",
		TitleID: "counselor-overview-title", QuickAccessID: "counselor-quick-access-title",
		Cards: []dashboardStatCard{
			{Label: "Kasus tahap BK", Value: fmt.Sprint(stats.Cases), Detail: "Kasus dengan Anda sebagai penanggung jawab aktif.", Icon: dashboardIconCase, IconBackground: "bg-brand-50", IconColor: "text-brand-600"},
			{Label: "Perlu tindak lanjut", Value: fmt.Sprint(stats.OpenCases), Detail: "Kasus tahap BK berstatus Perlu tindak lanjut.", Icon: dashboardIconTimetable, IconBackground: "bg-orange-50", IconColor: "text-orange-600"},
			{Label: "Selesai", Value: fmt.Sprint(stats.CompletedCases), Detail: "Kasus tahap BK berstatus Selesai.", Icon: dashboardIconCheck, IconBackground: "bg-green-50", IconColor: "text-green-700"},
		},
		QuickLinks: []dashboardQuickLink{
			{Label: "Kasus Siswa", Href: "/kesiswaan/kasus", Icon: smallDashboardIcon(dashboardIconCase)},
		},
	}
}

func studentAffairsDashboardPresentation(stats academic.SchoolMonitoringDashboardStats) *dashboardPresentation {
	students, recorded, unrecorded, attendanceDetail := schoolAttendanceCardValues(stats)
	return &dashboardPresentation{
		Eyebrow: "Ringkasan Kesiswaan", Heading: "Kehadiran dan pembinaan siswa",
		Context: "Tanggal " + stats.Date + " WIB",
		TitleID: "student-affairs-overview-title", QuickAccessID: "student-affairs-quick-access-title",
		Cards: []dashboardStatCard{
			{Label: "Siswa pada roster aktif", Value: students, Detail: "Penempatan aktif pada tahun ajaran aktif.", Icon: dashboardIconPeople, IconBackground: "bg-brand-50", IconColor: "text-brand-600"},
			{Label: "Absensi tercatat hari ini", Value: recorded, Detail: attendanceDetail, Icon: dashboardIconCheck, IconBackground: "bg-green-50", IconColor: "text-green-700"},
			{Label: "Belum tercatat hari ini", Value: unrecorded, Detail: "Siswa pada roster aktif tanpa catatan absensi hari ini.", Icon: dashboardIconCalendar, IconBackground: "bg-orange-50", IconColor: "text-orange-600"},
			{Label: "Kasus perlu tindak lanjut", Value: fmt.Sprint(stats.OpenCases), Detail: "Tahap Kesiswaan dengan Anda sebagai penanggung jawab aktif.", Icon: dashboardIconCase, IconBackground: "bg-blue-50", IconColor: "text-blue-600"},
		},
		QuickLinks: []dashboardQuickLink{
			{Label: "Kehadiran Siswa", Href: "/absensi/harian", Icon: smallDashboardIcon(dashboardIconCalendar)},
			{Label: "Kasus Siswa", Href: "/kesiswaan/kasus", Icon: smallDashboardIcon(dashboardIconCase)},
			{Label: "Batas Kedatangan", Href: "/kesiswaan/batas-kedatangan", Icon: smallDashboardIcon(dashboardIconTimetable)},
		},
	}
}

func headmasterDashboardPresentation(stats academic.SchoolMonitoringDashboardStats) *dashboardPresentation {
	students, recorded, unrecorded, attendanceDetail := schoolAttendanceCardValues(stats)
	return &dashboardPresentation{
		Eyebrow: "Ringkasan sekolah", Heading: "Kehadiran dan tindak lanjut",
		Context: "Tanggal " + stats.Date + " WIB",
		TitleID: "headmaster-overview-title", QuickAccessID: "headmaster-quick-access-title",
		Cards: []dashboardStatCard{
			{Label: "Siswa pada roster aktif", Value: students, Detail: "Penempatan aktif pada tahun ajaran aktif.", Icon: dashboardIconPeople, IconBackground: "bg-brand-50", IconColor: "text-brand-600"},
			{Label: "Absensi tercatat hari ini", Value: recorded, Detail: attendanceDetail, Icon: dashboardIconCheck, IconBackground: "bg-green-50", IconColor: "text-green-700"},
			{Label: "Belum tercatat hari ini", Value: unrecorded, Detail: "Siswa pada roster aktif tanpa catatan absensi hari ini.", Icon: dashboardIconCalendar, IconBackground: "bg-orange-50", IconColor: "text-orange-600"},
			{Label: "Kasus perlu tindak lanjut", Value: fmt.Sprint(stats.OpenCases), Detail: "Ringkasan seluruh sekolah; isi konseling pribadi tidak ditampilkan.", Icon: dashboardIconCase, IconBackground: "bg-blue-50", IconColor: "text-blue-600"},
		},
		QuickLinks: []dashboardQuickLink{
			{Label: "Kehadiran Siswa", Href: "/absensi/harian", Icon: smallDashboardIcon(dashboardIconCalendar)},
			{Label: "Kasus Siswa", Href: "/kesiswaan/kasus", Icon: smallDashboardIcon(dashboardIconCase)},
		},
	}
}

func schoolAttendanceCardValues(stats academic.SchoolMonitoringDashboardStats) (students, recorded, unrecorded, detail string) {
	if !stats.HasActiveYear {
		return "Belum tersedia", "Belum tersedia", "Belum tersedia", "Tahun ajaran aktif belum tersedia."
	}
	students = fmt.Sprint(stats.ActiveStudents)
	recorded = "Belum ada pencatatan"
	detail = "Belum ada bukti absensi pada " + stats.Date + " WIB."
	if stats.HasAttendance {
		recorded = fmt.Sprint(stats.Recorded)
		detail = fmt.Sprintf("Hadir %d · Sakit %d · Izin %d · Alpa %d", stats.Present, stats.Sick, stats.Excused, stats.Absent)
	}
	unrecorded = fmt.Sprint(stats.ActiveStudents - stats.Recorded)
	if stats.ActiveStudents < stats.Recorded {
		unrecorded = "Perlu pemeriksaan"
	}
	return students, recorded, unrecorded, detail
}

func smallDashboardIcon(icon template.HTML) template.HTML {
	return template.HTML(strings.Replace(string(icon), "<svg ", `<svg class="size-4" `, 1))
}

var (
	dashboardIconPeople     = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M16 20v-1.5a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4V20m7-9a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm6-7.5a4 4 0 0 1 0 7.7M21 20v-1.5a4 4 0 0 0-3-3.87" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	dashboardIconTeacher    = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 20v-1.5a4 4 0 0 1 4-4h6a4 4 0 0 1 4 4V20M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm-8-1a3 3 0 0 0 2 2.83M20 12.83A3 3 0 0 0 20 7" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	dashboardIconTeacherNav = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 5.5 12 3l8 2.5v6c0 4.5-3.3 7.5-8 9.5-4.7-2-8-5-8-9.5v-6Z" stroke-linejoin="round"/><path d="M9 10a3 3 0 1 0 6 0 3 3 0 0 0-6 0Zm-1 7c.8-1.4 2.2-2.2 4-2.2s3.2.8 4 2.2" stroke-linecap="round"/></svg>`)
	dashboardIconStaff      = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3.5" y="5" width="17" height="15" rx="2"/><path d="M8 5V3.5h8V5m-12 5h16m-11 4h6" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	dashboardIconClasses    = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3.5" y="4" width="17" height="16" rx="2"/><path d="M3.5 9h17M9 9v11m6-11v11" stroke-linecap="round"/></svg>`)
	dashboardIconUsers      = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="9" cy="8" r="3.5"/><path d="M3 20v-1a6 6 0 0 1 12 0v1m3-13v6m-3-3h6" stroke-linecap="round"/></svg>`)
	dashboardIconPlacement  = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 7h12m0 0-3-3m3 3-3 3M20 17H8m0 0 3-3m-3 3 3 3" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	dashboardIconCalendar   = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 10h18m-13 4h.01M12 14h.01" stroke-linecap="round"/></svg>`)
	dashboardIconStructure  = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 5h16v14H4zM8 9h8m-8 4h8" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	dashboardIconAssignment = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="4" y="5" width="16" height="14" rx="1"/><path d="M8 9h8m-8 4h5" stroke-linecap="round"/></svg>`)
	dashboardIconTimetable  = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 10h18m-13 4h3m3 0h3m-9 3h3" stroke-linecap="round"/></svg>`)
	dashboardIconSemester   = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 6h16M4 12h10M4 18h7" stroke-linecap="round"/><circle cx="18" cy="16" r="3"/><path d="M18 14.5v1.7l1.1.7" stroke-linecap="round"/></svg>`)
	dashboardIconCheck      = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m5 12 4 4L19 6" stroke-linecap="round" stroke-linejoin="round"/><path d="M20 12a8 8 0 1 1-2.3-5.6" stroke-linecap="round"/></svg>`)
	dashboardIconCase       = template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M6 3.5h8l4 4V21H6z" stroke-linejoin="round"/><path d="M14 3.5V8h4M9 12h6m-6 4h6" stroke-linecap="round"/></svg>`)
)
