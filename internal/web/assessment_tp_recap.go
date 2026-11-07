package web

import (
	"net/http"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type assessmentTPRecapView struct {
	Contexts    []academic.LearningObjectiveContext
	Selected    academic.LearningObjectiveContext
	SelectedSet bool
	Recap       *academic.AssessmentTPRecap
}

func (a *application) assessmentTPRecap(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &assessmentTPRecapView{}
	page := func(status int, message string) {
		a.render(w, r, "page", status, view{Page: "assessment-tp-recap", Title: "Rekap Nilai TP dan NH", Error: message, AssessmentTPRecap: data})
	}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		code, message := academicError(err)
		page(code, message)
		return
	}
	data.Contexts = contexts
	semesterID, classID, subjectID, selected, err := learningObjectiveSelection(r)
	if err != nil {
		code, message := academicError(err)
		page(code, message)
		return
	}
	if selected {
		for _, item := range contexts {
			if item.SemesterID == semesterID && item.ClassID == classID && item.SubjectID == subjectID {
				data.Selected, data.SelectedSet = item, true
				break
			}
		}
		if !data.SelectedSet {
			a.academicFailure(w, academic.ErrForbidden)
			return
		}
		recap, loadErr := a.options.Academics.AssessmentTPRecap(r.Context(), actor, semesterID, classID, subjectID)
		if loadErr != nil {
			code, message := academicError(loadErr)
			page(code, message)
			return
		}
		data.Recap = &recap
	}
	page(http.StatusOK, "")
}
