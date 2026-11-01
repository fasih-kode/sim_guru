package web

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

// teachingJournalLineage menyediakan operasi resmi Kurikulum untuk aktivasi
// lineage dan pemetaan satu occurrence historis yang telah diverifikasi.
func (a *application) teachingJournalLineage(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	date := r.URL.Query().Get("date")
	if date == "" {
		date = "2026-10-30"
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		var err error
		switch r.FormValue("action") {
		case "cutover":
			_, err = a.options.Academics.CutoverTeachingJournalLineage(r.Context(), actor, r.FormValue("reason"))
		case "map":
			revisionID, revisionErr := strconv.ParseInt(r.FormValue("revision_id"), 10, 64)
			entryID, entryErr := strconv.ParseInt(r.FormValue("entry_id"), 10, 64)
			if revisionErr != nil || entryErr != nil {
				err = academic.ValidationError("Slot jadwal resmi tidak valid.")
			} else {
				_, err = a.options.Academics.MapTeachingJournalHistoricalOccurrence(r.Context(), actor, academic.TeachingJournalHistoricalMappingInput{Date: r.FormValue("date"), RevisionID: revisionID, EntryID: entryID, Reason: r.FormValue("reason")})
			}
			date = r.FormValue("date")
		default:
			err = academic.ValidationError("Aksi Jurnal Mengajar tidak valid.")
		}
		if err != nil {
			code, message := academicError(err)
			a.renderLineagePage(w, r, code, date, message, "")
			return
		}
		query := url.Values{}
		query.Set("date", date)
		if r.FormValue("action") == "cutover" {
			query.Set("cutover", "done")
		} else {
			query.Set("mapped", "done")
		}
		http.Redirect(w, r, "/akademik/jurnal-lineage?"+query.Encode(), http.StatusSeeOther)
		return
	}
	message := ""
	if r.URL.Query().Get("cutover") == "done" {
		message = "Cutover tersimpan. Identitas baru dibuat untuk seluruh slot jadwal terbit efektif."
	} else if r.URL.Query().Get("mapped") == "done" {
		message = "Pemetaan occurrence historis tersimpan dan dapat digunakan pemilik jurnal yang sah."
	}
	a.renderLineagePage(w, r, http.StatusOK, date, "", message)
}

func (a *application) renderLineagePage(w http.ResponseWriter, r *http.Request, status int, date, message, errorMessage string) {
	actor := principal(r).ID
	cutoverDate, err := a.options.Academics.TeachingJournalCutoverState(r.Context(), actor)
	if err != nil {
		code, text := academicError(err)
		a.render(w, r, "page", code, view{Page: "journal-lineage", Title: "Aktivasi Jurnal Mengajar", Error: text, Academic: &academicView{JournalHistoricalDate: date}})
		return
	}
	var candidates []academic.TeachingJournalHistoricalCandidate
	if cutoverDate != "" && date != "" {
		candidates, err = a.options.Academics.TeachingJournalHistoricalCandidates(r.Context(), actor, date)
		if err != nil && errorMessage == "" {
			_, errorMessage = academicError(err)
		}
	}
	a.render(w, r, "page", status, view{Page: "journal-lineage", Title: "Aktivasi Jurnal Mengajar", Message: message, Error: errorMessage, Academic: &academicView{JournalCutoverDate: cutoverDate, JournalHistoricalDate: date, JournalHistoricalCandidates: candidates}})
}
