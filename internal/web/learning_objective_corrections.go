package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type learningObjectiveCorrectionView struct {
	State      academic.LearningObjectiveEditState
	Preview    *academic.LearningObjectiveCorrectionPreview
	History    []academic.LearningObjectiveDefinitionRevision
	Definition string
	ChangeType string
	Reason     string
	Snapshot   string
	Submission string
	ReturnURL  string
	Saved      bool
	Stale      bool
}

func (a *application) learningObjectiveCorrection(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	id, err := pathInt64(r, "id")
	if err != nil {
		a.academicFailure(w, academic.ValidationError("Tujuan Pembelajaran tidak valid."))
		return
	}
	state, err := a.options.Academics.LearningObjectiveEdit(r.Context(), actor, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	key, err := newFinalSubmissionKey()
	if err != nil {
		a.failure(w, err)
		return
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "learning-objective-correction", Title: "Koreksi Tujuan Pembelajaran", LearningObjectiveCorrection: &learningObjectiveCorrectionView{
		State: state, Definition: state.Objective.Definition, Submission: key,
		ReturnURL: learningObjectiveURL(state.Objective.SemesterID, state.Objective.ClassID, state.Objective.SubjectID),
	}})
}

func (a *application) saveRoutineLearningObjectiveCorrection(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	actor := principal(r).ID
	id, err := pathInt64(r, "id")
	if err != nil {
		a.academicFailure(w, academic.ValidationError("Tujuan Pembelajaran tidak valid."))
		return
	}
	version, parseErr := strconv.ParseInt(r.PostForm.Get("expected_version"), 10, 64)
	if parseErr != nil {
		parseErr = academic.ValidationError("Versi Tujuan Pembelajaran tidak valid.")
	}
	if parseErr == nil {
		parseErr = a.options.Academics.CorrectLearningObjectiveRoutine(r.Context(), actor, id, version, r.PostForm.Get("definition"), r.PostForm.Get("snapshot_token"), r.PostForm.Get("submission_key"))
	}
	if parseErr != nil {
		state, stateErr := a.options.Academics.LearningObjectiveEdit(r.Context(), actor, id)
		if stateErr != nil {
			a.academicFailure(w, stateErr)
			return
		}
		status, message := academicError(parseErr)
		a.render(w, r, "page", status, view{Page: "learning-objective-correction", Title: "Koreksi Tujuan Pembelajaran", Error: message, LearningObjectiveCorrection: &learningObjectiveCorrectionView{
			State: state, Definition: r.PostForm.Get("definition"), Submission: r.PostForm.Get("submission_key"),
			ReturnURL: learningObjectiveURL(state.Objective.SemesterID, state.Objective.ClassID, state.Objective.SubjectID), Stale: status == http.StatusConflict,
		}})
		return
	}
	http.Redirect(w, r, "/guru/tujuan-pembelajaran/"+strconv.FormatInt(id, 10)+"/riwayat?saved=1&key="+url.QueryEscape(r.PostForm.Get("submission_key")), http.StatusSeeOther)
}

func (a *application) previewLearningObjectiveCorrection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && !parseForm(w, r) {
		return
	}
	values := r.URL.Query()
	if r.Method == http.MethodPost {
		values = r.PostForm
	}
	id, err := pathInt64(r, "id")
	if err != nil {
		a.academicFailure(w, academic.ValidationError("Tujuan Pembelajaran tidak valid."))
		return
	}
	version, parseErr := strconv.ParseInt(values.Get("expected_version"), 10, 64)
	page, pageErr := strconv.Atoi(values.Get("page"))
	if pageErr != nil || page < 1 {
		page = 1
	}
	var preview academic.LearningObjectiveCorrectionPreview
	if parseErr == nil {
		preview, parseErr = a.options.Academics.PreviewLearningObjectiveCorrection(r.Context(), principal(r).ID, id, version,
			values.Get("definition"), values.Get("change_type"), values.Get("reason"), page, values.Get("submission_key"))
	}
	if parseErr != nil {
		state, stateErr := a.options.Academics.LearningObjectiveEdit(r.Context(), principal(r).ID, id)
		if stateErr != nil {
			a.academicFailure(w, stateErr)
			return
		}
		status, message := academicError(parseErr)
		a.render(w, r, "page", status, view{Page: "learning-objective-correction", Title: "Koreksi Tujuan Pembelajaran", Error: message, LearningObjectiveCorrection: &learningObjectiveCorrectionView{
			State: state, Definition: values.Get("definition"), ChangeType: values.Get("change_type"), Reason: values.Get("reason"), Submission: values.Get("submission_key"),
			ReturnURL: learningObjectiveURL(state.Objective.SemesterID, state.Objective.ClassID, state.Objective.SubjectID), Stale: status == http.StatusConflict,
		}})
		return
	}
	returnURL := learningObjectiveURL(preview.State.Objective.SemesterID, preview.State.Objective.ClassID, preview.State.Objective.SubjectID)
	a.render(w, r, "page", http.StatusOK, view{Page: "learning-objective-correction-preview", Title: "Pratinjau Dampak Koreksi TP", LearningObjectiveCorrection: &learningObjectiveCorrectionView{
		State: preview.State, Preview: &preview, Definition: preview.DefinitionAfter, ChangeType: preview.ChangeType,
		Reason: preview.Reason, Snapshot: preview.SnapshotToken, Submission: preview.SubmissionKey, ReturnURL: returnURL,
	}})
}

func (a *application) confirmLearningObjectiveCorrection(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	id, err := pathInt64(r, "id")
	if err != nil {
		a.academicFailure(w, academic.ValidationError("Tujuan Pembelajaran tidak valid."))
		return
	}
	values := r.PostForm
	version, parseErr := strconv.ParseInt(values.Get("expected_version"), 10, 64)
	if parseErr != nil {
		parseErr = academic.ValidationError("Versi Tujuan Pembelajaran tidak valid.")
	}
	if parseErr == nil {
		parseErr = a.options.Academics.ConfirmLearningObjectiveCorrection(r.Context(), principal(r).ID, id, version,
			values.Get("definition"), values.Get("change_type"), values.Get("reason"), values.Get("snapshot_token"), values.Get("submission_key"))
	}
	if parseErr != nil {
		status, message := academicError(parseErr)
		if status == http.StatusConflict {
			state, stateErr := a.options.Academics.LearningObjectiveEdit(r.Context(), principal(r).ID, id)
			if stateErr != nil {
				a.academicFailure(w, stateErr)
				return
			}
			a.render(w, r, "page", status, view{Page: "learning-objective-correction", Title: "Koreksi Tujuan Pembelajaran", Error: message, LearningObjectiveCorrection: &learningObjectiveCorrectionView{
				State: state, Definition: values.Get("definition"), ChangeType: values.Get("change_type"), Reason: values.Get("reason"), Submission: values.Get("submission_key"),
				ReturnURL: learningObjectiveURL(state.Objective.SemesterID, state.Objective.ClassID, state.Objective.SubjectID), Stale: true,
			}})
			return
		}
		a.academicFailure(w, parseErr)
		return
	}
	http.Redirect(w, r, "/guru/tujuan-pembelajaran/"+strconv.FormatInt(id, 10)+"/riwayat?saved=1&key="+url.QueryEscape(values.Get("submission_key")), http.StatusSeeOther)
}

func (a *application) learningObjectiveDefinitionHistory(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		a.academicFailure(w, academic.ValidationError("Tujuan Pembelajaran tidak valid."))
		return
	}
	state, history, err := a.options.Academics.LearningObjectiveDefinitionHistory(r.Context(), principal(r).ID, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	saved := false
	if r.URL.Query().Get("saved") == "1" {
		saved, err = a.options.Academics.LearningObjectiveCorrectionSaved(r.Context(), principal(r).ID, id, r.URL.Query().Get("key"))
		if err != nil {
			a.academicFailure(w, err)
			return
		}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "learning-objective-history", Title: "Riwayat Perubahan TP", LearningObjectiveCorrection: &learningObjectiveCorrectionView{
		State: state, History: history, Saved: saved,
		ReturnURL: learningObjectiveURL(state.Objective.SemesterID, state.Objective.ClassID, state.Objective.SubjectID),
	}})
}

func pathInt64(r *http.Request, name string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(r.PathValue(name)), 10, 64)
	if err != nil || value < 1 {
		return 0, academic.ValidationError("ID halaman tidak valid.")
	}
	return value, nil
}
