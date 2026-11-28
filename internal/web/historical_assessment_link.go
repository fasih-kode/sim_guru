package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type historicalAssessmentLinkView struct {
	Contexts               []academic.LearningObjectiveContext
	Objectives, Activities []academic.HistoricalAssessmentLinkChoice
	Context, Submission    string
	Input                  academic.HistoricalAssessmentLinkInput
	Preview                *academic.HistoricalAssessmentLinkPreview
	ResultID               int64
}

func (a *application) historicalAssessmentLink(w http.ResponseWriter, r *http.Request) {
	v := &historicalAssessmentLinkView{}
	var err error
	v.Contexts, err = a.options.Academics.LearningObjectiveContexts(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		form := r.PostForm
		v.Context = form.Get("context")
		semester, class, subject, _, parseErr := parseLearningObjectiveContext(v.Context)
		v.Input = academic.HistoricalAssessmentLinkInput{SemesterID: semester, ClassID: class, SubjectID: subject, Reason: form.Get("reason"), Document: form.Get("document"), Archive: form.Get("archive"), Section: form.Get("section"), Method: form.Get("method"), ResultSource: form.Get("result_source"), DateExplanation: form.Get("date_explanation"), DateEvidence: form.Get("date_evidence"), Acknowledged: form.Get("verified") == "yes"}
		if parseErr == nil {
			v.Objectives, v.Activities, parseErr = a.options.Academics.HistoricalAssessmentLinkChoices(r.Context(), principal(r).ID, semester, class, subject)
		}
		if parseErr == nil && form.Get("action") != "context" {
			v.Input.ObjectiveID, err = strconv.ParseInt(form.Get("objective"), 10, 64)
			if err != nil {
				parseErr = academic.ValidationError("Pilih TP historis.")
			}
			v.Input.ActivityID, err = strconv.ParseInt(form.Get("activity"), 10, 64)
			if err != nil {
				parseErr = academic.ValidationError("Pilih kegiatan Sumatif.")
			}
		}
		if parseErr == nil && form.Get("action") == "confirm" {
			var id int64
			id, parseErr = a.options.Academics.ConfirmHistoricalAssessmentLink(r.Context(), principal(r).ID, academic.HistoricalAssessmentLinkConfirmation{Input: v.Input, SnapshotToken: form.Get("snapshot_token"), SubmissionKey: form.Get("submission_key")})
			if parseErr == nil {
				http.Redirect(w, r, fmt.Sprintf("/guru/tujuan-pembelajaran/relasi-historis/hasil?id=%d", id), http.StatusSeeOther)
				return
			}
		} else if parseErr == nil && form.Get("action") != "context" {
			page := 1
			if form.Has("page") {
				page, err = strconv.Atoi(form.Get("page"))
				if err != nil || page < 1 {
					parseErr = academic.ValidationError("Halaman tidak valid.")
				} else if form.Get("snapshot_token") == "" {
					parseErr = academic.ErrConflict
				}
			}
			if parseErr == nil {
				var p academic.HistoricalAssessmentLinkPreview
				p, parseErr = a.options.Academics.PreviewHistoricalAssessmentLink(r.Context(), principal(r).ID, v.Input, page, form.Get("snapshot_token"))
				if parseErr == nil {
					v.Preview = &p
					v.Submission, parseErr = newFinalSubmissionKey()
				}
			}
		}
		if parseErr != nil {
			status, msg := academicError(parseErr)
			if status == http.StatusForbidden {
				a.academicFailure(w, parseErr)
				return
			}
			a.render(w, r, "page", status, view{Page: "historical-assessment-link", Title: "Koreksi relasi Sumatif historis", Error: msg, HistoricalAssessmentLink: v})
			return
		}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "historical-assessment-link", Title: "Koreksi relasi Sumatif historis", HistoricalAssessmentLink: v})
}

func (a *application) historicalAssessmentLinkResult(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	p, err := a.options.Academics.HistoricalAssessmentLinkResult(r.Context(), principal(r).ID, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "historical-assessment-link-result", Title: "Koreksi relasi tersimpan", HistoricalAssessmentLink: &historicalAssessmentLinkView{Preview: &p, ResultID: id}})
}
