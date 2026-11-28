package web

import (
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type tpCriteriaView struct {
	Contexts                                  []academic.LearningObjectiveContext
	Selected                                  academic.LearningObjectiveContext
	SelectedSet                               bool
	Criteria                                  academic.TPCriteria
	Objectives                                []academic.LearningObjective
	Guidance                                  string
	Submitted                                 bool
	InputObjective                            int64
	InputThreshold, InputReason, InputVersion string
}

func (a *application) tpCriteria(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &tpCriteriaView{}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.Contexts = contexts
	semester, class, subject, selected, err := learningObjectiveSelection(r)
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		semester, class, subject, err = learningObjectiveFormSelection(r)
		selected = true
	}
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	if selected {
		for _, c := range contexts {
			if c.SemesterID == semester && c.ClassID == class && c.SubjectID == subject {
				data.Selected = c
				data.SelectedSet = true
				break
			}
		}
		if !data.SelectedSet {
			a.academicFailure(w, academic.ErrForbidden)
			return
		}
		if r.Method == http.MethodPost {
			objective, e1 := strconv.ParseInt(r.PostForm.Get("objective_id"), 10, 64)
			version, e2 := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
			if e1 == nil {
				data.Submitted = true
				data.InputObjective = objective
				data.InputThreshold = r.PostForm.Get("threshold")
				data.InputReason = r.PostForm.Get("reason")
				data.InputVersion = r.PostForm.Get("version")
			}
			if e1 != nil || e2 != nil {
				err = academic.ValidationError("Versi atau TP tidak valid.")
			} else {
				err = a.options.Academics.SaveTPCriterion(r.Context(), actor, semester, class, subject, objective, version, r.PostForm.Get("threshold"), r.PostForm.Get("reason"), r.PostForm.Get("action") == "reset")
			}
			if err == nil {
				http.Redirect(w, r, "/guru/kriteria-tp?semester_id="+strconv.FormatInt(semester, 10)+"&class_id="+strconv.FormatInt(class, 10)+"&subject_id="+strconv.FormatInt(subject, 10)+"&saved=1", http.StatusSeeOther)
				return
			}
		}
		var loadErr error
		data.Criteria, loadErr = a.options.Academics.TPCriteriaForContext(r.Context(), actor, semester, class, subject)
		if loadErr != nil {
			a.academicFailure(w, loadErr)
			return
		}
		data.Objectives, loadErr = a.options.Academics.LearningObjectives(r.Context(), actor, semester, class, subject)
		if loadErr != nil {
			a.academicFailure(w, loadErr)
			return
		}
		data.Guidance, loadErr = a.options.Academics.TPGuidanceForTeacher(r.Context(), actor, semester, class, subject)
		if loadErr != nil {
			a.academicFailure(w, loadErr)
			return
		}
	}
	v := view{Page: "tp-criteria", Title: "Kriteria Ketercapaian TP", TPCriteria: data}
	status := http.StatusOK
	if err != nil {
		status, v.Error = academicError(err)
	} else if r.URL.Query().Get("saved") == "1" {
		v.Message = "Kriteria tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", status, v)
}
func (a *application) tpGuidance(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	var err error
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		version, e := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
		if e != nil {
			err = academic.ValidationError("Versi pedoman tidak valid.")
		} else {
			err = a.options.Academics.SaveTPGuidance(r.Context(), actor, version, r.PostForm.Get("guidance"), r.PostForm.Get("reason"))
		}
		if err == nil {
			http.Redirect(w, r, "/akademik/pedoman-tp?saved=1", http.StatusSeeOther)
			return
		}
	}
	data, e := a.options.Academics.TPGuidance(r.Context(), actor)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	v := view{Page: "tp-guidance", Title: "Pedoman Ketercapaian", TPGuidance: &data}
	status := http.StatusOK
	if err != nil {
		status, v.Error = academicError(err)
	} else if r.URL.Query().Get("saved") == "1" {
		v.Message = "Pedoman tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", status, v)
}
