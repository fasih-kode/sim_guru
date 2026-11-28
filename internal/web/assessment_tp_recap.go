package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type assessmentTPRecapView struct {
	Contexts    []academic.LearningObjectiveContext
	Selected    academic.LearningObjectiveContext
	SelectedSet bool
	Recap       *academic.AssessmentTPRecap
	Student     *academic.AssessmentTPRecapStudent
	Rows        []assessmentTPRecapRow
	Search      string
	ListURL     string
}

type assessmentTPRecapRow struct {
	Number, Complete, Applicable, Uncertain, NotApplicable int
	Name, NIS, NH, URL                                     string
	NHComplete, PlacementUncertain                         bool
}

func (v *assessmentTPRecapView) setRecap(recap *academic.AssessmentTPRecap, studentID string) error {
	v.Recap = recap
	q := url.Values{"context": {fmt.Sprintf("%d:%d:%d", v.Selected.SemesterID, v.Selected.ClassID, v.Selected.SubjectID)}}
	if v.Search != "" {
		q.Set("q", v.Search)
	}
	const path = "/guru/rekap-nilai-tp?"
	v.ListURL = path + q.Encode()
	if studentID != "" {
		for i := range recap.Students {
			if recap.Students[i].ID == studentID {
				v.Student = &recap.Students[i]
				return nil
			}
		}
		return academic.ErrNotFound
	}
	for i, student := range recap.Students {
		q.Set("student_id", student.ID)
		row := assessmentTPRecapRow{
			Number: i + 1, Name: student.Name, NIS: student.NIS, NH: student.NH,
			NHComplete: student.NHComplete, PlacementUncertain: student.PlacementUncertain,
			URL: path + q.Encode(),
		}
		for _, tp := range student.Objectives {
			switch tp.HistoricalApplicability {
			case "belum pasti":
				row.Uncertain++
			case "tidak berlaku":
				row.NotApplicable++
			default:
				row.Applicable++
				if tp.Complete {
					row.Complete++
				}
			}
		}
		v.Rows = append(v.Rows, row)
	}
	return nil
}

func assessmentTPRecapStudentID(query url.Values, selected bool) (string, error) {
	values, requested := query["student_id"]
	if !requested {
		return "", nil
	}
	if !selected || len(values) != 1 {
		return "", academic.ValidationError("Pilih konteks dan satu siswa yang valid.")
	}
	id, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || id < 1 {
		return "", academic.ValidationError("Siswa tidak valid.")
	}
	return strconv.FormatInt(id, 10), nil
}

func (a *application) assessmentTPRecap(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		a.academicFailure(w, academic.ValidationError("Parameter rekap tidak valid."))
		return
	}
	search := []rune(strings.TrimSpace(query.Get("q")))
	if len(search) > 120 {
		search = search[:120]
	}
	data := &assessmentTPRecapView{Search: string(search)}
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
	studentID, err := assessmentTPRecapStudentID(query, selected)
	if err != nil {
		a.academicFailure(w, err)
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
		if err := data.setRecap(&recap, studentID); err != nil {
			a.academicFailure(w, err)
			return
		}
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
