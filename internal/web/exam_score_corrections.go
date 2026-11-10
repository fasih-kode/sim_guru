package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type examOwnedScoresView struct {
	Page                 academic.ExamOwnedScorePage
	PreviousURL, NextURL string
}
type examCorrectionView struct {
	Detail          academic.ExamScoreCorrectionDetail
	Value, Reason   string
	ExpectedVersion int64
}

func (a *application) examOwnedScores(w http.ResponseWriter, r *http.Request) {
	filter := academic.AssessmentOwnedScoreFilter{Search: strings.TrimSpace(r.URL.Query().Get("q")), Page: 1}
	if raw := r.URL.Query().Get("page"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil || p < 1 {
			a.academicFailure(w, academic.ValidationError("Nomor halaman tidak valid."))
			return
		}
		filter.Page = p
	}
	result, err := a.options.Academics.OwnedExamInitialScores(r.Context(), principal(r).ID, filter)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data := &examOwnedScoresView{Page: result}
	if result.Page > 1 {
		data.PreviousURL = examOwnedScorePageURL(result.Search, result.Page-1)
	}
	if result.Page < result.TotalPages {
		data.NextURL = examOwnedScorePageURL(result.Search, result.Page+1)
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "exam-owned-scores", Title: "Nilai ASTS/ASAS Milik Saya", ExamOwnedScores: data})
}
func examOwnedScorePageURL(search string, page int) string {
	q := url.Values{}
	if search != "" {
		q.Set("q", search)
	}
	q.Set("page", strconv.Itoa(page))
	return "/guru/nilai-ujian-saya?" + q.Encode()
}

func (a *application) examScoreCorrection(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	id, err := strconv.ParseInt(r.PathValue("scoreID"), 10, 64)
	if err != nil || id < 1 {
		a.academicFailure(w, academic.ValidationError("Catatan nilai tidak valid."))
		return
	}
	data := &examCorrectionView{}
	page := func(code int, msg string) {
		a.render(w, r, "page", code, view{Page: "exam-score-correction", Title: "Koreksi Nilai ASTS/ASAS", Error: msg, ExamCorrection: data})
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		data.Value = r.PostForm.Get("value")
		data.Reason = r.PostForm.Get("reason")
		data.ExpectedVersion, err = strconv.ParseInt(r.PostForm.Get("expected_version"), 10, 64)
		if err != nil || data.ExpectedVersion < 0 {
			err = academic.ValidationError("Versi koreksi tidak valid. Muat ulang halaman.")
		} else {
			err = a.options.Academics.CorrectExamInitialScore(r.Context(), actor, id, data.ExpectedVersion, data.Value, data.Reason)
		}
		if err != nil {
			status, message := academicError(err)
			if status >= 500 {
				a.failure(w, err)
				return
			}
			if errors.Is(err, academic.ErrForbidden) {
				http.Error(w, message, status)
				return
			}
			data.Detail, err = a.options.Academics.ExamScoreCorrectionDetail(r.Context(), actor, id)
			if err != nil {
				a.academicFailure(w, err)
				return
			}
			page(status, message)
			return
		}
		http.Redirect(w, r, "/guru/nilai-ujian/"+strconv.FormatInt(id, 10)+"/koreksi?saved=1", http.StatusSeeOther)
		return
	}
	data.Detail, err = a.options.Academics.ExamScoreCorrectionDetail(r.Context(), actor, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.ExpectedVersion = data.Detail.CurrentVersion
	if r.URL.Query().Get("saved") == "1" {
		a.render(w, r, "page", http.StatusOK, view{Page: "exam-score-correction", Title: "Koreksi Nilai ASTS/ASAS", Message: "Koreksi berhasil dicatat. Nilai awal dan riwayat tetap tersimpan.", ExamCorrection: data})
		return
	}
	page(http.StatusOK, "")
}
