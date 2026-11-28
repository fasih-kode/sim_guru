package web

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type assessmentScoresView struct {
	Sheet   academic.AssessmentScoreSheet
	Error   string
	Message string
}

func (a *application) assessmentActivityScores(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	activityID, err := strconv.ParseInt(r.PathValue("activityID"), 10, 64)
	if err != nil || activityID < 1 {
		a.academicFailure(w, academic.ValidationError("Kegiatan penilaian tidak valid."))
		return
	}
	data := &assessmentScoresView{}
	page := func(status int) {
		a.render(w, r, "page", status, view{Page: "assessment-activity-scores", Title: "Input Nilai", Error: data.Error, Message: data.Message, AssessmentScores: data})
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		inputs, parseErr := assessmentScoreInputs(r.PostForm)
		if parseErr == nil {
			parseErr = a.options.Academics.SaveAssessmentActivityInitialScores(r.Context(), actor, activityID, inputs)
		}
		if parseErr != nil {
			status, message := academicError(parseErr)
			if status >= 500 {
				a.failure(w, parseErr)
				return
			}
			data.Error = message
			if sheet, loadErr := a.options.Academics.AssessmentActivityScoreSheet(r.Context(), actor, activityID); loadErr == nil {
				data.Sheet = sheet
				preserveAssessmentScoreInputs(&data.Sheet, inputs)
			} else {
				code, message := academicError(loadErr)
				http.Error(w, message, code)
				return
			}
			page(status)
			return
		}
		http.Redirect(w, r, assessmentScoreURL(activityID)+"?saved=1", http.StatusSeeOther)
		return
	}
	data.Sheet, err = a.options.Academics.AssessmentActivityScoreSheet(r.Context(), actor, activityID)
	if err != nil {
		status, message := academicError(err)
		http.Error(w, message, status)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Message = "Nilai awal berhasil disimpan. Nilai yang sudah tercatat tidak dapat diubah melalui formulir ini."
	}
	page(http.StatusOK)
}

func assessmentScoreURL(activityID int64) string {
	return "/guru/kegiatan-penilaian/" + strconv.FormatInt(activityID, 10) + "/nilai"
}

func assessmentScoreInputs(form map[string][]string) ([]academic.AssessmentScoreInput, error) {
	inputs := make([]academic.AssessmentScoreInput, 0)
	for name, values := range form {
		if !strings.HasPrefix(name, "score_") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(name, "score_"), "_")
		if len(parts) != 2 || len(values) != 1 {
			return nil, academic.ValidationError("Kolom nilai tidak valid. Muat ulang formulir.")
		}
		studentID, studentErr := strconv.ParseInt(parts[0], 10, 64)
		objectiveID, objectiveErr := strconv.ParseInt(parts[1], 10, 64)
		if studentErr != nil || objectiveErr != nil || studentID < 1 || objectiveID < 1 {
			return nil, academic.ValidationError("Siswa atau Tujuan Pembelajaran tidak valid.")
		}
		inputs = append(inputs, academic.AssessmentScoreInput{StudentID: studentID, ObjectiveID: objectiveID, Value: values[0]})
	}
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].StudentID == inputs[j].StudentID {
			return inputs[i].ObjectiveID < inputs[j].ObjectiveID
		}
		return inputs[i].StudentID < inputs[j].StudentID
	})
	return inputs, nil
}

func preserveAssessmentScoreInputs(sheet *academic.AssessmentScoreSheet, inputs []academic.AssessmentScoreInput) {
	entered := make(map[string]string, len(inputs))
	for _, input := range inputs {
		entered[fmt.Sprintf("%d:%d", input.StudentID, input.ObjectiveID)] = input.Value
	}
	for i := range sheet.Students {
		for j := range sheet.Students[i].Scores {
			cell := &sheet.Students[i].Scores[j]
			if cell.Recorded {
				continue
			}
			cell.Value = entered[fmt.Sprintf("%d:%d", sheet.Students[i].ID, cell.ObjectiveID)]
		}
	}
}
