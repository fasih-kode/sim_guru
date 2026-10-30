package web

import (
	"net/http"
)

func (a *application) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "HX-Request")
	user := principal(r)
	name := "page"
	if r.Header.Get("HX-Request") == "true" {
		name = "overview"
	}
	v := view{Page: "home", Title: "Beranda"}
	if user != nil && user.HasRole("admin") {
		stats, err := a.options.Masters.AdminDashboard(r.Context(), user.ID)
		if err != nil {
			a.failure(w, err)
			return
		}
		v.AdminDashboard = &stats
	}
	a.render(w, r, name, http.StatusOK, v)
}
