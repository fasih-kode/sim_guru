package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

const historicalAssessmentActivityPath = "/guru/kegiatan-penilaian/historis"

type historicalAssessmentActivityView struct {
	Contexts               []academic.LearningObjectiveContext
	Objectives, Activities []academic.HistoricalAssessmentLinkChoice
	SelectedObjectiveIDs   map[int64]bool
	Context                string
	Input                  academic.HistoricalAssessmentActivityInput
	Preview                *academic.HistoricalAssessmentActivityPreview
	Result                 *academic.HistoricalAssessmentActivityResult
	Submission             string
	ResultID               int64
}

func (a *application) historicalAssessmentActivity(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &historicalAssessmentActivityView{}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.Contexts = contexts
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		form := r.PostForm
		data.Context = form.Get("context")
		semester, class, subject, parseErr := parseHistoricalContext(data.Context)
		data.Input = historicalAssessmentActivityInput(form, semester, class, subject)
		data.selectObjectives()
		if parseErr == nil && form.Get("action") != "" {
			data.Objectives, data.Activities, parseErr = a.options.Academics.HistoricalAssessmentLinkChoices(r.Context(), actor, semester, class, subject)
		}
		if parseErr == nil {
			switch form.Get("action") {
			case "context", "":
			case "confirm":
				var auditID int64
				auditID, _, parseErr = a.options.Academics.ConfirmHistoricalAssessmentActivity(r.Context(), actor, academic.HistoricalAssessmentActivityConfirmation{
					Input: data.Input, SnapshotToken: form.Get("snapshot_token"), SubmissionKey: form.Get("submission_key"),
				})
				if parseErr == nil {
					http.Redirect(w, r, historicalAssessmentActivityPath+"/hasil?id="+url.QueryEscape(strconv.FormatInt(auditID, 10)), http.StatusSeeOther)
					return
				}
			case "preview":
				page := 1
				if raw := form.Get("page"); raw != "" {
					page, err = strconv.Atoi(raw)
					if err != nil || page < 1 {
						parseErr = academic.ValidationError("Halaman pratinjau tidak valid.")
					} else if form.Get("snapshot_token") == "" {
						parseErr = academic.ErrConflict
					}
				}
				if parseErr == nil {
					var preview academic.HistoricalAssessmentActivityPreview
					preview, parseErr = a.options.Academics.PreviewHistoricalAssessmentActivity(r.Context(), actor, data.Input, page, form.Get("snapshot_token"))
					if parseErr == nil {
						data.Preview = &preview
						data.Submission = form.Get("submission_key")
						if data.Submission == "" {
							data.Submission, parseErr = newFinalSubmissionKey()
						}
						if parseErr == nil && r.Header.Get("HX-Request") == "true" {
							a.render(w, r, "historical-assessment-activity-preview", http.StatusOK, view{Page: "historical-assessment-activity", Title: "Kegiatan Sumatif historis", HistoricalAssessmentActivity: data})
							return
						}
					}
				}
			default:
				parseErr = academic.ValidationError("Aksi formulir tidak dikenal.")
			}
		}
		if parseErr != nil {
			status, message := academicError(parseErr)
			if status == http.StatusForbidden {
				a.academicFailure(w, parseErr)
				return
			}
			if strings.TrimSpace(data.Context) != "" && len(data.Objectives) == 0 {
				if semester > 0 && class > 0 && subject > 0 {
					data.Objectives, data.Activities, _ = a.options.Academics.HistoricalAssessmentLinkChoices(r.Context(), actor, semester, class, subject)
				}
			}
			a.render(w, r, "page", status, view{Page: "historical-assessment-activity", Title: "Kegiatan Sumatif historis", Error: message, HistoricalAssessmentActivity: data})
			return
		}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "historical-assessment-activity", Title: "Kegiatan Sumatif historis", HistoricalAssessmentActivity: data})
}

func (a *application) historicalAssessmentActivityResult(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || auditID < 1 {
		http.NotFound(w, r)
		return
	}
	result, err := a.options.Academics.HistoricalAssessmentActivityResult(r.Context(), principal(r).ID, auditID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "historical-assessment-activity-result", Title: "Kegiatan Sumatif historis tersimpan", HistoricalAssessmentActivity: &historicalAssessmentActivityView{ResultID: auditID, Result: &result}})
}

func parseHistoricalContext(value string) (int64, int64, int64, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, 0, 0, academic.ValidationError("Pilih semester, kelas, dan mata pelajaran.")
	}
	semester, err1 := strconv.ParseInt(parts[0], 10, 64)
	class, err2 := strconv.ParseInt(parts[1], 10, 64)
	subject, err3 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || semester < 1 || class < 1 || subject < 1 {
		return 0, 0, 0, academic.ValidationError("Konteks penugasan tidak valid.")
	}
	return semester, class, subject, nil
}

func historicalAssessmentActivityInput(form url.Values, semester, class, subject int64) academic.HistoricalAssessmentActivityInput {
	ids := make([]int64, 0, len(form["objective_ids"]))
	for _, raw := range form["objective_ids"] {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			ids = append(ids, 0)
			continue
		}
		ids = append(ids, id)
	}
	return academic.HistoricalAssessmentActivityInput{
		SemesterID: semester, ClassID: class, SubjectID: subject, ObjectiveIDs: ids,
		Name: form.Get("name"), ActivityDate: form.Get("activity_date"), Reason: form.Get("reason"),
		SourceIdentity: form.Get("source_identity"), SourceDocument: form.Get("source_document"), SourceArchive: form.Get("source_archive"),
		SourceTeacher: form.Get("source_teacher"), SourceSection: form.Get("source_section"), ResultSource: form.Get("result_source"), Method: form.Get("method"),
		OutsideCalendarReason: form.Get("outside_calendar_reason"), DuplicateExplanation: form.Get("duplicate_explanation"),
		Acknowledged: form.Get("acknowledged") == "yes", DuplicateChecked: form.Get("duplicate_checked") == "yes",
	}
}

func (v *historicalAssessmentActivityView) selectObjectives() {
	v.SelectedObjectiveIDs = make(map[int64]bool, len(v.Input.ObjectiveIDs))
	for _, id := range v.Input.ObjectiveIDs {
		v.SelectedObjectiveIDs[id] = true
	}
}
