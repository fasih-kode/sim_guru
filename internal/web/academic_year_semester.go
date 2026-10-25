package web

import "net/http"

func (a *application) academicYears(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	years, err := a.options.Academics.Years(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := view{Page: "academic-years", Title: "Tahun Ajaran", Academic: &academicView{Years: years}}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		_, err = a.options.Academics.CreateYear(r.Context(), actor, r.PostForm.Get("name"))
		if err == nil {
			http.Redirect(w, r, "/akademik/tahun-ajaran?saved=1", http.StatusSeeOther)
			return
		}
		code, message := academicError(err)
		v.ModalOpen = true
		v.Error = message
		v.Academic.NewYearName = r.PostForm.Get("name")
		a.render(w, r, "page", code, v)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Tahun ajaran dan dua semesternya tersimpan."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) activateYear(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err = a.options.Academics.ActivateYear(r.Context(), principal(r).ID, id); err != nil {
		a.academicFailure(w, err)
		return
	}
	http.Redirect(w, r, "/akademik/tahun-ajaran?saved=1", http.StatusSeeOther)
}

func (a *application) academicSemesters(w http.ResponseWriter, r *http.Request) {
	terms, err := a.options.Academics.Semesters(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	years, err := a.options.Academics.Years(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := view{Page: "academic-semesters", Title: "Semester", Academic: &academicView{Years: years, Semesters: terms}}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Semester aktif diperbarui dan histori tercatat."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) activateSemester(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err = a.options.Academics.ActivateSemester(r.Context(), principal(r).ID, id); err != nil {
		a.academicFailure(w, err)
		return
	}
	http.Redirect(w, r, "/akademik/semester?saved=1", http.StatusSeeOther)
}
