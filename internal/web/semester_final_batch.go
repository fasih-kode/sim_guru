package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type semesterFinalBatchView struct {
	Preview                           academic.SemesterFinalBatchPreview
	Context, SubmissionKey, ReturnURL string
}

func (a *application) semesterFinalBatch(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		values = r.PostForm
	}
	semester, class, subject, selected, err := learningObjectiveSelectionValues(values)
	if err != nil || !selected {
		a.academicFailure(w, academic.ValidationError("Konteks tidak valid."))
		return
	}
	ids := values["student_id"]
	tokens := values["snapshot_token"]
	if r.Method == http.MethodPost && len(ids) != len(tokens) {
		a.academicFailure(w, academic.ValidationError("Pratinjau tidak valid; batch tidak diproses."))
		return
	}
	items := make([]academic.SemesterFinalBatchItem, 0, len(ids))
	for i, raw := range ids {
		id, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || id < 1 {
			a.academicFailure(w, academic.ValidationError("Siswa tidak valid; batch tidak diproses."))
			return
		}
		item := academic.SemesterFinalBatchItem{StudentID: id}
		if r.Method == http.MethodPost {
			item.SnapshotToken = tokens[i]
		}
		items = append(items, item)
	}
	actor := principal(r).ID
	contextValue := fmt.Sprintf("%d:%d:%d", semester, class, subject)
	returnURL := "/guru/rekomendasi-semester?context=" + url.QueryEscape(contextValue)
	if r.Method == http.MethodPost {
		err = a.options.Academics.FinalizeSemesterBatch(r.Context(), actor, semester, class, subject, items, values.Get("submission_key"))
		if err != nil {
			status, message := academicError(err)
			if status >= 500 {
				a.failure(w, err)
				return
			}
			http.Error(w, message+" Batch tidak diproses; kembali ke daftar dan buat pratinjau baru.", status)
			return
		}
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	preview, err := a.options.Academics.SemesterFinalBatchPreview(r.Context(), actor, semester, class, subject, items)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	key, err := newFinalSubmissionKey()
	if err != nil {
		a.failure(w, err)
		return
	}
	data := &semesterFinalBatchView{Preview: preview, Context: contextValue, SubmissionKey: key, ReturnURL: returnURL}
	a.render(w, r, "page", http.StatusOK, view{Page: "semester-final-batch", Title: "Pratinjau Finalisasi Massal", SemesterFinalBatch: data})
}
