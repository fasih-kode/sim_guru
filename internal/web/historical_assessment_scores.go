package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

const historicalAssessmentScoresPath = "/guru/kegiatan-penilaian/nilai-historis"

type historicalAssessmentScoreView struct {
	Choices       []academic.HistoricalAssessmentScoreActivityChoice
	Input         academic.HistoricalAssessmentScoreInput
	Preview       *academic.HistoricalAssessmentScorePreview
	Result        *academic.HistoricalAssessmentScoreResult
	Submission    string
	HiddenEntries []academic.HistoricalAssessmentScoreEntry
}

func (a *application) historicalAssessmentScores(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &historicalAssessmentScoreView{}
	choices, err := a.options.Academics.HistoricalAssessmentScoreChoices(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.Choices = choices
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		form := r.PostForm
		data.Input, err = historicalAssessmentScoreInput(form)
		if err != nil {
			status, message := academicError(err)
			a.render(w, r, "page", status, view{Page: "historical-assessment-scores", Title: "Pencatatan Nilai Sumatif historis", Error: message, HistoricalAssessmentScores: data})
			return
		}
		data.Submission = form.Get("submission_key")
		if data.Submission == "" {
			data.Submission, err = newFinalSubmissionKey()
			if err != nil {
				a.failure(w, err)
				return
			}
		}
		switch form.Get("action") {
		case "load", "preview":
			page := 1
			pageValue := form.Get("page_override")
			if pageValue == "" {
				pageValue = form.Get("page")
			}
			if pageValue != "" {
				page, err = strconv.Atoi(pageValue)
				if err != nil || page < 1 {
					err = academic.ValidationError("Halaman nilai historis tidak valid.")
				}
			}
			if err == nil {
				var preview academic.HistoricalAssessmentScorePreview
				expectedSource := form.Get("source_fingerprint")
				if form.Get("action") == "load" {
					expectedSource = ""
				}
				preview, err = a.options.Academics.PreviewHistoricalAssessmentScores(r.Context(), actor, data.Input, page, expectedSource)
				if err == nil {
					data.Preview = &preview
					data.HiddenEntries = hiddenHistoricalScoreEntries(data.Input, preview)
					if r.Header.Get("HX-Request") == "true" {
						a.render(w, r, "historical-assessment-scores-preview", http.StatusOK, view{Page: "historical-assessment-scores", Title: "Pencatatan Nilai Sumatif historis", HistoricalAssessmentScores: data})
						return
					}
				}
			}
		case "confirm":
			var batchID int64
			batchID, err = a.options.Academics.ConfirmHistoricalAssessmentScores(r.Context(), actor, academic.HistoricalAssessmentScoreConfirmation{
				Input: data.Input, SnapshotToken: form.Get("snapshot_token"), SubmissionKey: data.Submission,
			})
			if err == nil {
				http.Redirect(w, r, historicalAssessmentScoresPath+"/hasil?id="+url.QueryEscape(strconv.FormatInt(batchID, 10)), http.StatusSeeOther)
				return
			}
		default:
			err = academic.ValidationError("Aksi formulir tidak dikenal.")
		}
		if err != nil {
			status, message := academicError(err)
			if status == http.StatusForbidden {
				a.academicFailure(w, err)
				return
			}
			a.render(w, r, "page", status, view{Page: "historical-assessment-scores", Title: "Pencatatan Nilai Sumatif historis", Error: message, HistoricalAssessmentScores: data})
			return
		}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "historical-assessment-scores", Title: "Pencatatan Nilai Sumatif historis", HistoricalAssessmentScores: data})
}

func historicalAssessmentScoreInput(form url.Values) (academic.HistoricalAssessmentScoreInput, error) {
	activityID, err := strconv.ParseInt(form.Get("activity_id"), 10, 64)
	if err != nil || activityID < 1 {
		return academic.HistoricalAssessmentScoreInput{}, academic.ValidationError("Pilih kegiatan Sumatif historis.")
	}
	in := academic.HistoricalAssessmentScoreInput{
		ActivityID: activityID, Reason: form.Get("reason"), Verification: form.Get("verification"), Acknowledged: form.Get("acknowledged") == "yes",
	}
	students, objectives := form["entry_student_id"], form["entry_objective_id"]
	values, kinds := form["entry_value"], form["entry_source_kind"]
	identities, locators := form["entry_source_identity"], form["entry_source_locator"]
	results, verifications := form["entry_source_result"], form["entry_verification"]
	length := len(students)
	for _, current := range []int{len(objectives), len(values), len(kinds), len(identities), len(locators), len(results), len(verifications)} {
		if current != length {
			return academic.HistoricalAssessmentScoreInput{}, academic.ValidationError("Daftar sel nilai tidak lengkap.")
		}
	}
	for i := 0; i < length; i++ {
		studentID, e1 := strconv.ParseInt(students[i], 10, 64)
		objectiveID, e2 := strconv.ParseInt(objectives[i], 10, 64)
		if e1 != nil || e2 != nil || studentID < 1 || objectiveID < 1 {
			return academic.HistoricalAssessmentScoreInput{}, academic.ValidationError("Identitas siswa atau TP tidak valid.")
		}
		fields := []string{values[i], kinds[i], identities[i], locators[i], results[i], verifications[i]}
		blank := true
		for _, field := range fields {
			if strings.TrimSpace(field) != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		in.Entries = append(in.Entries, academic.HistoricalAssessmentScoreEntry{
			StudentID: studentID, ObjectiveID: objectiveID, Value: values[i], SourceKind: kinds[i], SourceIdentity: identities[i], SourceLocator: locators[i], SourceResult: results[i], Verification: verifications[i],
		})
	}
	return in, nil
}

func hiddenHistoricalScoreEntries(in academic.HistoricalAssessmentScoreInput, preview academic.HistoricalAssessmentScorePreview) []academic.HistoricalAssessmentScoreEntry {
	visible := map[[2]int64]bool{}
	for _, student := range preview.Students {
		for _, cell := range student.Cells {
			visible[[2]int64{cell.StudentID, cell.ObjectiveID}] = true
		}
	}
	var hidden []academic.HistoricalAssessmentScoreEntry
	for _, entry := range in.Entries {
		if !visible[[2]int64{entry.StudentID, entry.ObjectiveID}] {
			hidden = append(hidden, entry)
		}
	}
	return hidden
}

func (a *application) historicalAssessmentScoresResult(w http.ResponseWriter, r *http.Request) {
	batchID, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || batchID < 1 {
		http.NotFound(w, r)
		return
	}
	result, err := a.options.Academics.HistoricalAssessmentScoreResult(r.Context(), principal(r).ID, batchID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "historical-assessment-scores-result", Title: "Nilai Sumatif historis tersimpan", HistoricalAssessmentScores: &historicalAssessmentScoreView{Result: &result}})
}
