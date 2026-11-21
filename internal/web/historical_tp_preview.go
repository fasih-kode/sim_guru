package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type historicalTPPreviewView struct {
	Contexts         []academic.LearningObjectiveContext
	Context          string
	Input            academic.HistoricalTPPreviewInput
	Preview          *academic.HistoricalTPPreview
	Submission       string
	ResultID         int64
	ResultAuditID    int64
	ResultDefinition string
}

func (a *application) historicalTPPreview(w http.ResponseWriter, r *http.Request) {
	data := &historicalTPPreviewView{}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.Contexts = contexts
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		v := r.PostForm
		data.Context = v.Get("context")
		semester, class, subject, _, parseErr := parseLearningObjectiveContext(data.Context)
		data.Input = academic.HistoricalTPPreviewInput{SemesterID: semester, ClassID: class, SubjectID: subject, Definition: v.Get("definition"), Reason: v.Get("reason"), AssessmentDate: v.Get("assessment_date"), DateSource: v.Get("date_source"),
			Learning:   academic.HistoricalTPReference{Kind: v.Get("learning_kind"), SourceID: v.Get("learning_id"), Document: v.Get("learning_document"), Archive: v.Get("learning_archive"), Teacher: v.Get("learning_teacher"), Date: v.Get("learning_date")},
			Assessment: academic.HistoricalTPReference{Kind: v.Get("assessment_kind"), SourceID: v.Get("assessment_id"), Document: v.Get("assessment_document"), Archive: v.Get("assessment_archive"), Teacher: v.Get("assessment_teacher"), ResultSource: v.Get("result_source")}}
		if parseErr == nil && v.Get("action") == "confirm" {
			id, confirmErr := a.options.Academics.ConfirmHistoricalLearningObjective(r.Context(), principal(r).ID, academic.HistoricalTPConfirmation{Preview: data.Input, Verification: academic.HistoricalTPVerification{LearningSection: v.Get("learning_section"), AssessmentSection: v.Get("assessment_section"), Method: v.Get("verification_method"), ResultSource: v.Get("verified_result_source"), Acknowledged: v.Get("verified") == "yes"}, SnapshotToken: v.Get("snapshot_token"), SubmissionKey: v.Get("submission_key")})
			if confirmErr != nil {
				status, msg := academicError(confirmErr)
				if status == http.StatusForbidden {
					a.academicFailure(w, confirmErr)
					return
				}
				a.render(w, r, "page", status, view{Page: "historical-tp-preview", Title: "Pratinjau TP Historis", Error: msg, HistoricalTPPreview: data})
				return
			}
			http.Redirect(w, r, fmt.Sprintf("/guru/tujuan-pembelajaran/historis/hasil?id=%d", id), http.StatusSeeOther)
			return
		}
		page := 1
		if v.Has("page") {
			page, err = strconv.Atoi(v.Get("page"))
			if err != nil || page < 1 {
				parseErr = academic.ValidationError("Halaman tidak valid.")
			}
			if v.Get("snapshot_token") == "" {
				parseErr = academic.ErrConflict
			}
		}
		if parseErr == nil {
			var p academic.HistoricalTPPreview
			p, parseErr = a.options.Academics.PreviewHistoricalTP(r.Context(), principal(r).ID, data.Input, page, v.Get("snapshot_token"))
			if parseErr == nil {
				data.Preview = &p
				data.Submission, parseErr = newFinalSubmissionKey()
			}
		}
		if parseErr != nil {
			status, msg := academicError(parseErr)
			if status == http.StatusForbidden {
				a.academicFailure(w, parseErr)
				return
			}
			a.render(w, r, "page", status, view{Page: "historical-tp-preview", Title: "Pratinjau TP Historis", Error: msg, HistoricalTPPreview: data})
			return
		}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "historical-tp-preview", Title: "Pratinjau TP Historis", HistoricalTPPreview: data})
}

func (a *application) historicalTPResult(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	definition, auditID, err := a.options.Academics.HistoricalLearningObjectiveResult(r.Context(), principal(r).ID, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "historical-tp-result", Title: "TP historis tersimpan", HistoricalTPPreview: &historicalTPPreviewView{ResultID: id, ResultAuditID: auditID, ResultDefinition: definition}})
}
