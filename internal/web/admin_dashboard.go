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
	}
	a.render(w, r, name, http.StatusOK, v)
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
	return ""
}
