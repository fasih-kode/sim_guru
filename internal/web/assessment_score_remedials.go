package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type assessmentRemedialView struct {
	Detail  academic.AssessmentScoreRemedialDetail
	Value   string
	Date    string
	Error   string
	Message string
}

func (a *application) assessmentScoreRemedial(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	scoreID, err := strconv.ParseInt(r.PathValue("scoreID"), 10, 64)
	if err != nil || scoreID < 1 {
		a.academicFailure(w, academic.ValidationError("Catatan nilai tidak valid."))
		return
	}
	data := &assessmentRemedialView{}
	pageStatus := http.StatusOK
	page := func(status int) {
		a.render(w, r, "page", status, view{Page: "assessment-score-remedial", Title: "Remedial Sumatif", Error: data.Error, Message: data.Message, AssessmentRemedial: data})
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		data.Value = r.PostForm.Get("value")
		data.Date = r.PostForm.Get("remedial_date")
		err = a.options.Academics.RecordAssessmentScoreRemedial(r.Context(), actor, scoreID, data.Value, data.Date)
		if err != nil {
			status, message := academicError(err)
			if status >= 500 {
				a.failure(w, err)
				return
			}
			if errors.Is(err, academic.ErrForbidden) || errors.Is(err, academic.ErrNotFound) {
				http.Error(w, message, status)
				return
			}
			pageStatus = status
			data.Error = message
		} else {
			http.Redirect(w, r, remedialPath(scoreID)+"?saved=1", http.StatusSeeOther)
			return
		}
	}
	data.Detail, err = a.options.Academics.AssessmentScoreRemedialDetail(r.Context(), actor, scoreID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Message = "Remedial berhasil dicatat. Nilai awal dan seluruh riwayat tetap tersimpan."
	}
	page(pageStatus)
}

func remedialPath(scoreID int64) string {
	return "/guru/nilai-awal/" + strconv.FormatInt(scoreID, 10) + "/remedial"
}
