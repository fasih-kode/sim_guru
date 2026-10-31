package web

import (
	"errors"
	"net/http"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
)

func (a *application) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "HX-Request")
	user := principal(r)
	name := "page"
	if r.Header.Get("HX-Request") == "true" {
		name = "overview"
	}
	eligibility, eligibilityErr := a.options.Academics.PersonalAttendanceEligibility(r.Context(), user.ID)
	shortcut := attendanceShortcutsFor(eligibility, eligibilityErr)
	v := view{Page: "home", Title: "Beranda", HomeAttendance: shortcut}
	switch homeDashboardKind(user) {
	case "admin":
		stats, err := a.options.Masters.AdminDashboard(r.Context(), user.ID)
		if err != nil {
			a.failure(w, err)
			return
		}
		v.Dashboard = adminDashboardPresentation(stats)
	case "kurikulum":
		stats, err := a.options.Academics.CurriculumDashboard(r.Context(), user.ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = curriculumDashboardPresentation(stats)
	case "guru-mapel":
		stats, err := a.options.Academics.TeacherDashboard(r.Context(), user.ID)
		if err != nil {
			if errors.Is(err, academic.ErrForbidden) {
				break
			}
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = teacherDashboardPresentation(stats)
	case "wali-kelas":
		stats, err := a.options.Academics.HomeroomDashboard(r.Context(), user.ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = homeroomDashboardPresentation(stats)
	case "guru-wali":
		teacherStats, err := a.options.Academics.TeacherDashboard(r.Context(), user.ID)
		teacherAvailable := err == nil
		if err != nil && !errors.Is(err, academic.ErrForbidden) {
			a.academicFailure(w, err)
			return
		}
		homeroomStats, err := a.options.Academics.HomeroomDashboard(r.Context(), user.ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		if !teacherAvailable {
			v.Dashboard = homeroomDashboardPresentation(homeroomStats)
		} else {
			v.Dashboard = teacherDashboardPresentation(teacherStats)
			v.AdditionalDashboards = []*dashboardPresentation{homeroomDashboardPresentation(homeroomStats)}
		}
	case "bk":
		stats, err := a.options.Academics.CounselorDashboard(r.Context(), user.ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = counselorDashboardPresentation(stats)
	case "kesiswaan":
		stats, err := a.options.Academics.StudentAffairsDashboard(r.Context(), user.ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = studentAffairsDashboardPresentation(stats)
	case "kepala-sekolah":
		stats, err := a.options.Academics.HeadmasterDashboard(r.Context(), user.ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = headmasterDashboardPresentation(stats)
	case "pegawai-tu":
		stats, err := a.options.Academics.StaffSelfDashboard(r.Context(), user.ID)
		if err != nil {
			if errors.Is(err, academic.ErrForbidden) {
				break
			}
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = staffDashboardPresentation(stats)
	case "petugas-absensi":
		stats, err := a.options.Academics.StudentAttendanceOperatorDashboard(r.Context(), user.ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = attendanceOperatorDashboardPresentation(stats)
	case "support-combined":
		for _, role := range supportDashboardRoles(user) {
			dashboard, err := a.supportDashboard(r, user.ID, role)
			if err != nil {
				if role == "pegawai_tu" && errors.Is(err, academic.ErrForbidden) {
					continue
				}
				a.academicFailure(w, err)
				return
			}
			if v.Dashboard == nil {
				v.Dashboard = dashboard
			} else {
				v.AdditionalDashboards = append(v.AdditionalDashboards, dashboard)
			}
		}
	}
	ensureKesiswaanCaseQuickLink(&v, user)
	ensureOperationalQuickLinks(&v, user)
	deduplicateDashboardQuickLinks(&v)
	a.render(w, r, name, http.StatusOK, v)
}

func attendanceShortcutsFor(eligibility academic.PersonalAttendanceEligibility, err error) *homeAttendanceShortcuts {
	shortcut := &homeAttendanceShortcuts{Message: "Tombol aktif untuk akun dengan identitas Guru atau Pegawai TU aktif yang tertaut dan role presensi pribadi."}
	if err != nil {
		shortcut.Message = "Kelayakan presensi belum dapat diperiksa. Tombol dinonaktifkan sementara."
		return shortcut
	}
	// Jika dua identitas aktif tertaut, jalur Guru menjadi jalur pribadi utama.
	if eligibility.Teacher {
		shortcut.Available = true
		shortcut.Href = "/kehadiran-saya"
	} else if eligibility.Staff {
		shortcut.Available = true
		shortcut.Href = "/pegawai/kehadiran-saya"
	}
	return shortcut
}

func deduplicateDashboardQuickLinks(v *view) {
	if v == nil {
		return
	}
	seen := make(map[string]bool)
	for _, dashboard := range append([]*dashboardPresentation{v.Dashboard}, v.AdditionalDashboards...) {
		if dashboard == nil {
			continue
		}
		unique := dashboard.QuickLinks[:0]
		for _, link := range dashboard.QuickLinks {
			if seen[link.Href] {
				continue
			}
			seen[link.Href] = true
			unique = append(unique, link)
		}
		dashboard.QuickLinks = unique
	}
}

func ensureOperationalQuickLinks(v *view, user *auth.User) {
	if v == nil || user == nil || v.Dashboard == nil {
		return
	}
	links := make([]dashboardQuickLink, 0, 6)
	if user.HasRole("pegawai_tu") && v.HomeAttendance != nil && v.HomeAttendance.Available && v.HomeAttendance.Href == "/pegawai/kehadiran-saya" {
		links = append(links, dashboardQuickLink{Label: "Kehadiran Pegawai", Href: "/pegawai/kehadiran-saya", Icon: smallDashboardIcon(dashboardIconCheck)})
	}
	if user.HasRole("pegawai_tu") && user.HasRole("pengelola_kehadiran_guru") {
		links = append(links,
			dashboardQuickLink{Label: "Kelola Kehadiran Guru", Href: "/tu/kehadiran-guru", Icon: smallDashboardIcon(dashboardIconTeacher)},
			dashboardQuickLink{Label: "Rekap Kehadiran Guru", Href: "/tu/kehadiran-guru/rekap", Icon: smallDashboardIcon(dashboardIconCalendar)},
		)
	}
	if user.HasRole("petugas_absensi") {
		links = append(links,
			dashboardQuickLink{Label: "Absensi Harian", Href: "/absensi/harian", Icon: smallDashboardIcon(dashboardIconCalendar)},
			dashboardQuickLink{Label: "Pemindai QR", Href: "/absensi/pemindai", Icon: smallDashboardIcon(dashboardIconCheck)},
			dashboardQuickLink{Label: "Kartu QR Siswa", Href: "/absensi/kartu", Icon: smallDashboardIcon(dashboardIconPeople)},
		)
	}
	hasLink := func(href string) bool {
		for _, dashboard := range append([]*dashboardPresentation{v.Dashboard}, v.AdditionalDashboards...) {
			if dashboard == nil {
				continue
			}
			for _, link := range dashboard.QuickLinks {
				if link.Href == href {
					return true
				}
			}
		}
		return false
	}
	for _, link := range links {
		if !hasLink(link.Href) {
			v.Dashboard.QuickLinks = append(v.Dashboard.QuickLinks, link)
		}
	}
}

func ensureKesiswaanCaseQuickLink(v *view, user *auth.User) {
	if v == nil || user == nil || !user.HasRole("kesiswaan") || v.Dashboard == nil {
		return
	}
	hasCaseLink := func(dashboard *dashboardPresentation) bool {
		if dashboard == nil {
			return false
		}
		for _, link := range dashboard.QuickLinks {
			if link.Href == "/kesiswaan/kasus" {
				return true
			}
		}
		return false
	}
	if hasCaseLink(v.Dashboard) {
		return
	}
	for _, dashboard := range v.AdditionalDashboards {
		if hasCaseLink(dashboard) {
			return
		}
	}
	v.Dashboard.QuickLinks = append(v.Dashboard.QuickLinks, dashboardQuickLink{
		Label: "Kasus Siswa", Href: "/kesiswaan/kasus", Icon: smallDashboardIcon(dashboardIconCase),
	})
}

func (a *application) supportDashboard(r *http.Request, actor int64, role string) (*dashboardPresentation, error) {
	switch role {
	case "bk":
		stats, err := a.options.Academics.CounselorDashboard(r.Context(), actor)
		return counselorDashboardPresentation(stats), err
	case "kesiswaan":
		stats, err := a.options.Academics.StudentAffairsDashboard(r.Context(), actor)
		return studentAffairsDashboardPresentation(stats), err
	case "kepala_sekolah":
		stats, err := a.options.Academics.HeadmasterDashboard(r.Context(), actor)
		return headmasterDashboardPresentation(stats), err
	case "pegawai_tu":
		stats, err := a.options.Academics.StaffSelfDashboard(r.Context(), actor)
		return staffDashboardPresentation(stats), err
	case "pengelola_kehadiran_guru":
		stats, err := a.options.Academics.TeacherAttendanceManagerDashboard(r.Context(), actor)
		return teacherAttendanceManagerDashboardPresentation(stats), err
	case "petugas_absensi":
		stats, err := a.options.Academics.StudentAttendanceOperatorDashboard(r.Context(), actor)
		return attendanceOperatorDashboardPresentation(stats), err
	default:
		return nil, nil
	}
}

func homeDashboardKind(user *auth.User) string {
	if user == nil {
		return ""
	}
	if user.HasRole("admin") {
		return "admin"
	}
	if user.HasRole("kurikulum") {
		return "kurikulum"
	}
	if user.HasRole("guru_mapel") && user.HasRole("wali_kelas") {
		return "guru-wali"
	}
	if user.HasRole("guru_mapel") {
		return "guru-mapel"
	}
	if user.HasRole("wali_kelas") {
		return "wali-kelas"
	}
	roles := supportDashboardRoles(user)
	if len(roles) > 1 {
		return "support-combined"
	}
	if len(roles) == 1 {
		switch roles[0] {
		case "bk":
			return "bk"
		case "kesiswaan":
			return "kesiswaan"
		case "kepala_sekolah":
			return "kepala-sekolah"
		case "pegawai_tu":
			return "pegawai-tu"
		case "petugas_absensi":
			return "petugas-absensi"
		}
	}
	return ""
}

func supportDashboardRoles(user *auth.User) []string {
	if user == nil {
		return nil
	}
	roles := make([]string, 0, 6)
	for _, role := range []string{"bk", "kesiswaan", "kepala_sekolah", "petugas_absensi", "pegawai_tu"} {
		if user.HasRole(role) {
			roles = append(roles, role)
		}
	}
	if user.HasRole("pegawai_tu") && user.HasRole("pengelola_kehadiran_guru") {
		roles = append(roles, "pengelola_kehadiran_guru")
	}
	return roles
}
