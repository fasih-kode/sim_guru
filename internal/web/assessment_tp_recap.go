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

type semesterRecommendationView struct {
	Contexts    []academic.LearningObjectiveContext
	Selected    academic.LearningObjectiveContext
	SelectedSet bool
	Result      *academic.SemesterRecommendation
	OwnedFinals []academic.OwnedSemesterFinal
}

func (a *application) semesterRecommendations(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &semesterRecommendationView{}
	page := func(status int, message string) {
		a.render(w, r, "page", status, view{Page: "semester-recommendations", Title: "Rekomendasi Nilai Semester", Error: message, SemesterRecommendations: data})
	}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		code, msg := academicError(err)
		page(code, msg)
		return
	}
	data.Contexts = contexts
	data.OwnedFinals, err = a.options.Academics.OwnedSemesterFinals(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semesterID, classID, subjectID, selected, err := learningObjectiveSelection(r)
	if err != nil {
		code, msg := academicError(err)
		page(code, msg)
		return
	}
	if selected {
		for _, item := range contexts {
			if item.SemesterID == semesterID && item.ClassID == classID && item.SubjectID == subjectID {
				data.Selected = item
				data.SelectedSet = true
				break
			}
		}
		if !data.SelectedSet {
			a.academicFailure(w, academic.ErrForbidden)
			return
		}
		result, e := a.options.Academics.SemesterRecommendations(r.Context(), actor, semesterID, classID, subjectID)
		if e != nil {
			code, msg := academicError(e)
			page(code, msg)
			return
		}
		data.Result = &result
	}
	page(http.StatusOK, "")
}
