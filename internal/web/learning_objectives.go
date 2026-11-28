package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type learningObjectivesView struct {
	Contexts    []academic.LearningObjectiveContext
	CopyTargets []academic.LearningObjectiveContext
	Objectives  []academic.LearningObjective
	Selected    academic.LearningObjectiveContext
	SelectedSet bool
	Definition  string
}

func (a *application) learningObjectives(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &learningObjectivesView{}
	page := func(status int, message string) {
		a.render(w, r, "page", status, view{Page: "learning-objectives", Title: "Tujuan Pembelajaran", Error: message, LearningObjectives: data})
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
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		semesterID, classID, subjectID, err = learningObjectiveFormSelection(r)
		if err != nil {
			code, message := academicError(err)
			page(code, message)
			return
		}
		selected = true
		data.Definition = r.PostForm.Get("definition")
		err = a.options.Academics.CreateLearningObjective(r.Context(), actor, semesterID, classID, subjectID, data.Definition)
		if err != nil {
			code, message := academicError(err)
			if code != http.StatusForbidden && code != http.StatusNotFound {
				if loadErr := loadLearningObjectiveContext(r, a, actor, data, semesterID, classID, subjectID); loadErr != nil {
					code, message = academicError(loadErr)
				}
			}
			page(code, message)
			return
		}
		http.Redirect(w, r, learningObjectiveURL(semesterID, classID, subjectID)+"&saved=1", http.StatusSeeOther)
		return
	}
	if selected {
		if err = loadLearningObjectiveContext(r, a, actor, data, semesterID, classID, subjectID); err != nil {
			code, message := academicError(err)
			page(code, message)
			return
		}
	}
	if r.URL.Query().Get("saved") == "1" {
		pageWithMessage := view{Page: "learning-objectives", Title: "Tujuan Pembelajaran", Message: "Tujuan Pembelajaran berhasil disimpan.", LearningObjectives: data}
		a.render(w, r, "page", http.StatusOK, pageWithMessage)
		return
	}
	if r.URL.Query().Get("copied") == "1" {
		pageWithMessage := view{Page: "learning-objectives", Title: "Tujuan Pembelajaran", Message: "Definisi TP berhasil disalin. TP sumber tetap tidak berubah.", LearningObjectives: data}
		a.render(w, r, "page", http.StatusOK, pageWithMessage)
		return
	}
	page(http.StatusOK, "")
}

func (a *application) copyLearningObjective(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &learningObjectivesView{}
	page := func(status int, message string) {
		a.render(w, r, "page", status, view{Page: "learning-objectives", Title: "Tujuan Pembelajaran", Error: message, LearningObjectives: data})
	}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		code, message := academicError(err)
		page(code, message)
		return
	}
	data.Contexts = contexts
	if !parseForm(w, r) {
		return
	}
	sourceID, parseErr := strconv.ParseInt(r.PostForm.Get("source_id"), 10, 64)
	sourceSemesterID, sourceClassID, sourceSubjectID, _, sourceErr := parseLearningObjectiveContext(r.PostForm.Get("source_context"))
	targetSemesterID, targetClassID, targetSubjectID, _, targetErr := parseLearningObjectiveContext(r.PostForm.Get("target_context"))
	if parseErr != nil || sourceErr != nil || targetErr != nil || sourceID < 1 {
		page(http.StatusUnprocessableEntity, "Pilih TP sumber dan konteks kelas tujuan yang valid.")
		return
	}
	_, err = a.options.Academics.CopyLearningObjective(r.Context(), actor, sourceID, sourceSemesterID, sourceClassID, sourceSubjectID, targetSemesterID, targetClassID, targetSubjectID)
	if err != nil {
		code, message := academicError(err)
		if loadErr := loadLearningObjectiveContext(r, a, actor, data, sourceSemesterID, sourceClassID, sourceSubjectID); loadErr != nil {
			code, message = academicError(loadErr)
		}
		page(code, message)
		return
	}
	http.Redirect(w, r, learningObjectiveURL(targetSemesterID, targetClassID, targetSubjectID)+"&copied=1", http.StatusSeeOther)
}

func loadLearningObjectiveContext(r *http.Request, a *application, actor int64, data *learningObjectivesView, semesterID, classID, subjectID int64) error {
	for _, item := range data.Contexts {
		if item.SemesterID == semesterID && item.ClassID == classID && item.SubjectID == subjectID {
			data.Selected = item
			data.SelectedSet = true
			break
		}
	}
	data.CopyTargets = nil
	for _, item := range data.Contexts {
		if item.ClassID != classID && item.SemesterID == semesterID && item.SubjectID == subjectID {
			data.CopyTargets = append(data.CopyTargets, item)
		}
	}
	objectives, err := a.options.Academics.LearningObjectives(r.Context(), actor, semesterID, classID, subjectID)
	if err != nil {
		return err
	}
	data.Objectives = objectives
	return nil
}

func learningObjectiveSelection(r *http.Request) (semesterID, classID, subjectID int64, selected bool, err error) {
	if raw := r.URL.Query().Get("context"); raw != "" {
		parts := strings.Split(raw, ":")
		if len(parts) != 3 {
			return 0, 0, 0, true, academic.ValidationError("Konteks penugasan tidak valid.")
		}
		return parseLearningObjectiveIDs(parts[0], parts[1], parts[2])
	}
	return parseLearningObjectiveIDs(r.URL.Query().Get("semester_id"), r.URL.Query().Get("class_id"), r.URL.Query().Get("subject_id"))
}

func learningObjectiveFormSelection(r *http.Request) (semesterID, classID, subjectID int64, err error) {
	semesterID, classID, subjectID, _, err = parseLearningObjectiveIDs(r.PostForm.Get("semester_id"), r.PostForm.Get("class_id"), r.PostForm.Get("subject_id"))
	return
}

func parseLearningObjectiveIDs(semesterRaw, classRaw, subjectRaw string) (semesterID, classID, subjectID int64, selected bool, err error) {
	if semesterRaw == "" && classRaw == "" && subjectRaw == "" {
		return 0, 0, 0, false, nil
	}
	semesterID, err = strconv.ParseInt(semesterRaw, 10, 64)
	if err != nil || semesterID < 1 {
		return 0, 0, 0, true, academic.ValidationError("Konteks semester tidak valid.")
	}
	classID, err = strconv.ParseInt(classRaw, 10, 64)
	if err != nil || classID < 1 {
		return 0, 0, 0, true, academic.ValidationError("Konteks kelas tidak valid.")
	}
	subjectID, err = strconv.ParseInt(subjectRaw, 10, 64)
	if err != nil || subjectID < 1 {
		return 0, 0, 0, true, academic.ValidationError("Konteks mata pelajaran tidak valid.")
	}
	return semesterID, classID, subjectID, true, nil
}

func parseLearningObjectiveContext(raw string) (semesterID, classID, subjectID int64, selected bool, err error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, 0, 0, true, academic.ValidationError("Konteks penugasan tidak valid.")
	}
	return parseLearningObjectiveIDs(parts[0], parts[1], parts[2])
}

func learningObjectiveURL(semesterID, classID, subjectID int64) string {
	return "/guru/tujuan-pembelajaran?semester_id=" + strconv.FormatInt(semesterID, 10) + "&class_id=" + strconv.FormatInt(classID, 10) + "&subject_id=" + strconv.FormatInt(subjectID, 10)
}
