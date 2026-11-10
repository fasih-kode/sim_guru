package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type examRemedialView struct {
	Detail        academic.ExamScoreRemedialDetail
	Value         string
	Date          string
	SubmissionKey string
	Error         string
	Message       string
}

func (a *application) examScoreRemedial(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	scoreID, err := strconv.ParseInt(r.PathValue("scoreID"), 10, 64)
	if err != nil || scoreID < 1 {
		a.academicFailure(w, academic.ValidationError("Catatan nilai tidak valid."))
		return
	}
	data := &examRemedialView{}
	data.SubmissionKey, err = newRemedialSubmissionKey()
	if err != nil {
		a.failure(w, err)
		return
	}
	pageStatus := http.StatusOK
	page := func(status int) {
		a.render(w, r, "page", status, view{Page: "exam-score-remedial", Title: "Remedial ASTS/ASAS", Error: data.Error, Message: data.Message, ExamRemedial: data})
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		data.Value = r.PostForm.Get("value")
		data.Date = r.PostForm.Get("remedial_date")
		data.SubmissionKey = r.PostForm.Get("submission_key")
		err = a.options.Academics.RecordExamScoreRemedial(r.Context(), actor, scoreID, data.SubmissionKey, data.Value, data.Date)
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
			if errors.Is(err, academic.ErrConflict) {
				data.SubmissionKey, err = newRemedialSubmissionKey()
				if err != nil {
					a.failure(w, err)
					return
				}
			}
			pageStatus = status
			data.Error = message
		} else {
			http.Redirect(w, r, examRemedialPath(scoreID)+"?saved=1", http.StatusSeeOther)
			return
		}
	}
	data.Detail, err = a.options.Academics.ExamScoreRemedialDetail(r.Context(), actor, scoreID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Message = "Remedial berhasil dicatat. Nilai awal dan seluruh riwayat tetap tersimpan."
	}
	page(pageStatus)
}

func examRemedialPath(scoreID int64) string {
	return "/guru/nilai-ujian/" + strconv.FormatInt(scoreID, 10) + "/remedial"
}
