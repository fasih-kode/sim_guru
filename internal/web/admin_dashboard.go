package web

import (
	"net/http"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

func (a *application) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "HX-Request")
	user := principal(r)
	name := "page"
	if r.Header.Get("HX-Request") == "true" {
		name = "overview"
	}
	v := view{Page: "home", Title: "Beranda"}
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
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		homeroomStats, err := a.options.Academics.HomeroomDashboard(r.Context(), user.ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		v.Dashboard = teacherDashboardPresentation(teacherStats)
		v.AdditionalDashboards = []*dashboardPresentation{homeroomDashboardPresentation(homeroomStats)}
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
	case "support-combined":
		for _, role := range supportDashboardRoles(user) {
			dashboard, err := a.supportDashboard(r, user.ID, role)
			if err != nil {
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
	a.render(w, r, name, http.StatusOK, v)
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
		}
	}
	return ""
}

func supportDashboardRoles(user *auth.User) []string {
	if user == nil {
		return nil
	}
	roles := make([]string, 0, 3)
	for _, role := range []string{"bk", "kesiswaan", "kepala_sekolah"} {
		if user.HasRole(role) {
			roles = append(roles, role)
		}
	}
	return roles
}
