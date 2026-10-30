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
	return ""
}
