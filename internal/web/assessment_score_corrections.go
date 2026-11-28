package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type assessmentOwnedScoresView struct {
	Page                 academic.AssessmentOwnedScorePage
	PreviousURL, NextURL string
}

type assessmentCorrectionView struct {
	Detail          academic.AssessmentScoreCorrectionDetail
	Value, Reason   string
	ExpectedVersion int64
}

func (a *application) assessmentOwnedScores(w http.ResponseWriter, r *http.Request) {
	filter := academic.AssessmentOwnedScoreFilter{Search: strings.TrimSpace(r.URL.Query().Get("q")), Page: 1}
	if raw := r.URL.Query().Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			a.academicFailure(w, academic.ValidationError("Nomor halaman tidak valid."))
			return
		}
		filter.Page = page
	}
	page, err := a.options.Academics.OwnedAssessmentInitialScores(r.Context(), principal(r).ID, filter)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data := &assessmentOwnedScoresView{Page: page}
	if page.Page > 1 {
		data.PreviousURL = ownedScorePageURL(page.Search, page.Page-1)
	}
	if page.Page < page.TotalPages {
		data.NextURL = ownedScorePageURL(page.Search, page.Page+1)
	}
	status := http.StatusOK
	if r.URL.Query().Get("saved") == "1" {
		a.render(w, r, "page", status, view{Page: "assessment-owned-scores", Title: "Nilai Milik Saya", Message: "Koreksi nilai berhasil disimpan.", AssessmentOwnedScores: data})
		return
	}
	a.render(w, r, "page", status, view{Page: "assessment-owned-scores", Title: "Nilai Milik Saya", AssessmentOwnedScores: data})
}

func ownedScorePageURL(search string, page int) string {
	query := url.Values{}
	if search != "" {
		query.Set("q", search)
	}
	query.Set("page", strconv.Itoa(page))
	return "/guru/nilai-awal-saya?" + query.Encode()
}

func (a *application) assessmentScoreCorrection(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	scoreID, err := strconv.ParseInt(r.PathValue("scoreID"), 10, 64)
	if err != nil || scoreID < 1 {
		a.academicFailure(w, academic.ValidationError("Catatan nilai tidak valid."))
		return
	}
	data := &assessmentCorrectionView{}
	page := func(status int, message string) {
		a.render(w, r, "page", status, view{Page: "assessment-score-correction", Title: "Koreksi Nilai Awal", Error: message, AssessmentCorrection: data})
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		data.Value = r.PostForm.Get("value")
		data.Reason = r.PostForm.Get("reason")
		expectedVersion, parseErr := strconv.ParseInt(r.PostForm.Get("expected_version"), 10, 64)
		data.ExpectedVersion = expectedVersion
		if parseErr != nil || expectedVersion < 0 {
			parseErr = academic.ValidationError("Versi koreksi tidak valid. Muat ulang halaman.")
		} else {
			parseErr = a.options.Academics.CorrectAssessmentInitialScore(r.Context(), actor, scoreID, expectedVersion, data.Value, data.Reason)
		}
		if parseErr != nil {
			status, message := academicError(parseErr)
			if status >= 500 {
				a.failure(w, parseErr)
				return
			}
			if errors.Is(parseErr, academic.ErrForbidden) {
				http.Error(w, message, status)
				return
			}
			data.Detail, err = a.options.Academics.AssessmentScoreCorrectionDetail(r.Context(), actor, scoreID)
			if err != nil {
				code, denied := academicError(err)
				http.Error(w, denied, code)
				return
			}
			page(status, message)
			return
		}
		http.Redirect(w, r, "/guru/nilai-awal/"+strconv.FormatInt(scoreID, 10)+"/koreksi?saved=1", http.StatusSeeOther)
		return
	}
	data.Detail, err = a.options.Academics.AssessmentScoreCorrectionDetail(r.Context(), actor, scoreID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.ExpectedVersion = data.Detail.CurrentVersion
	if r.URL.Query().Get("saved") == "1" {
		a.render(w, r, "page", http.StatusOK, view{Page: "assessment-score-correction", Title: "Koreksi Nilai Awal", Message: "Koreksi berhasil dicatat. Nilai awal tetap tersimpan dan riwayat perubahan diperbarui.", AssessmentCorrection: data})
		return
	}
	page(http.StatusOK, "")
}
