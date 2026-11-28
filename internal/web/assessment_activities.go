package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type assessmentActivitiesView struct {
	Contexts             []academic.LearningObjectiveContext
	Objectives           []academic.LearningObjective
	Activities           []academic.AssessmentActivity
	Selected             academic.LearningObjectiveContext
	SelectedSet          bool
	Name, Type, Date     string
	SelectedObjectiveIDs map[int64]bool
}

func (a *application) assessmentActivities(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &assessmentActivitiesView{SelectedObjectiveIDs: make(map[int64]bool)}
	page := func(status int, message string) {
		a.render(w, r, "page", status, view{Page: "assessment-activities", Title: "Kegiatan Penilaian", Error: message, AssessmentActivities: data})
	}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		code, message := academicError(err)
		page(code, message)
		return
	}
	data.Contexts = contexts
	semesterID, classID, subjectID, selected, err := learningObjectiveSelection(r)
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		semesterID, classID, subjectID, err = learningObjectiveFormSelection(r)
		selected = true
		data.Name = r.PostForm.Get("name")
		data.Type = r.PostForm.Get("activity_type")
		data.Date = r.PostForm.Get("activity_date")
		ids, parseErr := assessmentObjectiveIDs(r.PostForm["objective_ids"])
		if parseErr != nil {
			code, message := academicError(parseErr)
			if err == nil {
				if loadErr := loadAssessmentActivityContext(r, a, actor, data, semesterID, classID, subjectID); loadErr != nil {
					code, message = academicError(loadErr)
				}
			}
			page(code, message)
			return
		}
		for _, id := range ids {
			data.SelectedObjectiveIDs[id] = true
		}
		if err == nil {
			err = a.options.Academics.CreateAssessmentActivity(r.Context(), actor, semesterID, classID, subjectID, data.Name, data.Type, data.Date, ids)
		}
		if err != nil {
			code, message := academicError(err)
			if code != http.StatusForbidden && code != http.StatusNotFound {
				if loadErr := loadAssessmentActivityContext(r, a, actor, data, semesterID, classID, subjectID); loadErr != nil {
					code, message = academicError(loadErr)
				}
			}
			page(code, message)
			return
		}
		http.Redirect(w, r, assessmentActivityURL(semesterID, classID, subjectID)+"&saved=1", http.StatusSeeOther)
		return
	}
	if err != nil {
		code, message := academicError(err)
		page(code, message)
		return
	}
	if selected {
		if err = loadAssessmentActivityContext(r, a, actor, data, semesterID, classID, subjectID); err != nil {
			code, message := academicError(err)
			page(code, message)
			return
		}
	}
	if r.URL.Query().Get("saved") == "1" {
		a.render(w, r, "page", http.StatusOK, view{Page: "assessment-activities", Title: "Kegiatan Penilaian", Message: "Kegiatan penilaian berhasil disimpan.", AssessmentActivities: data})
		return
	}
	page(http.StatusOK, "")
}

func assessmentActivityURL(semesterID, classID, subjectID int64) string {
	return "/guru/kegiatan-penilaian?semester_id=" + strconv.FormatInt(semesterID, 10) + "&class_id=" + strconv.FormatInt(classID, 10) + "&subject_id=" + strconv.FormatInt(subjectID, 10)
}

func loadAssessmentActivityContext(r *http.Request, a *application, actor int64, data *assessmentActivitiesView, semesterID, classID, subjectID int64) error {
	for _, item := range data.Contexts {
		if item.SemesterID == semesterID && item.ClassID == classID && item.SubjectID == subjectID {
			data.Selected = item
			data.SelectedSet = true
			break
		}
	}
	objectives, err := a.options.Academics.LearningObjectives(r.Context(), actor, semesterID, classID, subjectID)
	if err != nil {
		return err
	}
	data.Objectives = objectives
	activities, err := a.options.Academics.AssessmentActivities(r.Context(), actor, semesterID, classID, subjectID)
	if err != nil {
		return err
	}
	data.Activities = activities
	return nil
}

func assessmentObjectiveIDs(values []string) ([]int64, error) {
	ids := make([]int64, 0, len(values))
	seen := make(map[int64]bool, len(values))
	for _, value := range values {
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || id < 1 {
			return nil, academic.ValidationError("Pilihan Tujuan Pembelajaran tidak valid.")
		}
		if seen[id] {
			return nil, academic.ValidationError("Tujuan Pembelajaran yang sama tidak boleh dipilih lebih dari sekali.")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}
