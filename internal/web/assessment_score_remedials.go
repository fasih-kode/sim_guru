package web

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type assessmentRemedialView struct {
	Detail        academic.AssessmentScoreRemedialDetail
	Value         string
	Date          string
	SubmissionKey string
	Error         string
	Message       string
}

func (a *application) assessmentScoreRemedial(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	scoreID, err := strconv.ParseInt(r.PathValue("scoreID"), 10, 64)
	if err != nil || scoreID < 1 {
		a.academicFailure(w, academic.ValidationError("Catatan nilai tidak valid."))
		return
	}
	data := &assessmentRemedialView{}
	data.SubmissionKey, err = newRemedialSubmissionKey()
	if err != nil {
		a.failure(w, err)
		return
	}
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
		data.SubmissionKey = r.PostForm.Get("submission_key")
		err = a.options.Academics.RecordAssessmentScoreRemedial(r.Context(), actor, scoreID, data.SubmissionKey, data.Value, data.Date)
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

func newRemedialSubmissionKey() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("tidak dapat menyiapkan kunci pengiriman remedial: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	value := hex.EncodeToString(bytes[:])
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:], nil
}

func remedialPath(scoreID int64) string {
	return "/guru/nilai-awal/" + strconv.FormatInt(scoreID, 10) + "/remedial"
}
