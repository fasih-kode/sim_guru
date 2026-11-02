package web

import (
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func (a *application) academicClasses(w http.ResponseWriter, r *http.Request) {
	classes, years, err := a.options.Academics.Classes(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	counts, err := a.options.Academics.PlacementCounts(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	var homerooms []academic.HomeroomAssignment
	if principal(r).HasRole("kurikulum") {
		homerooms, err = a.options.Academics.HomeroomAssignments(r.Context(), principal(r).ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
	}
	byClass := make(map[int64]academic.HomeroomAssignment, len(homerooms))
	for _, assignment := range homerooms {
		byClass[assignment.ClassID] = assignment
	}
	v := view{Page: "academic-classes", Title: "Kelas", Academic: &academicView{Classes: classes, Years: years, PlacementCounts: counts, Homerooms: homerooms, HomeroomByClass: byClass}}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Data kelas tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) academicClassForm(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	id := int64(0)
	var err error
	if r.PathValue("id") != "" {
		id, err = parseAcademicID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	classes, years, err := a.options.Academics.Classes(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	av := &academicView{Classes: classes, Years: years, Editing: id}
	if id != 0 {
		av.Class, err = a.options.Academics.Class(r.Context(), actor, id)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
	}
	counts, err := a.options.Academics.PlacementCounts(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	av.PlacementCounts = counts
	v := view{Page: "academic-classes", Title: "Kelas", Academic: av, ModalOpen: true}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		yearID, yearErr := strconv.ParseInt(r.PostForm.Get("school_year_id"), 10, 64)
		grade, gradeErr := strconv.Atoi(r.PostForm.Get("grade"))
		version, versionErr := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
		if yearErr != nil || gradeErr != nil || versionErr != nil {
			av.Class.Name = r.PostForm.Get("name")
			v.Error = "Tingkat, tahun ajaran, atau versi data tidak valid."
			a.render(w, r, "page", http.StatusUnprocessableEntity, v)
			return
		}
		className, reason := r.PostForm.Get("name"), r.PostForm.Get("reason")
		av.Class.Name = className
		av.Class.Grade = grade
		av.Class.SchoolYearID = yearID
		if id == 0 {
			for _, year := range years {
				if year.ID == yearID {
					av.Class.YearName = year.Name
					break
				}
			}
		}
		submitted := av.Class
		av.Class, err = a.options.Academics.SaveClass(r.Context(), actor, id, yearID, grade, className, reason, version)
		if err == nil {
			http.Redirect(w, r, "/akademik/kelas?saved=1", http.StatusSeeOther)
			return
		}
		av.Class = submitted
		code, message := academicError(err)
		v.Error = message
		a.render(w, r, "page", code, v)
		return
	}
	a.render(w, r, "page", http.StatusOK, v)
}
