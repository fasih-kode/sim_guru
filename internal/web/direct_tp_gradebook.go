package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type directTPGradebookView struct {
	Contexts    []academic.LearningObjectiveContext
	Selected    academic.LearningObjectiveContext
	SelectedSet bool
	Book        academic.DirectTPBook
	Submission  string
	CellErrors  map[string]string
}

func directTPCellKey(student, objective int64) string {
	return fmt.Sprintf("%d_%d", student, objective)
}

func (a *application) directTPGradebook(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &directTPGradebookView{CellErrors: map[string]string{}}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.Contexts = contexts
	values := r.URL.Query()
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if e := r.ParseForm(); e != nil {
			http.Error(w, "Form tidak valid atau terlalu besar.", http.StatusBadRequest)
			return
		}
		values = r.PostForm
	}
	var semester, class, subject int64
	var selected bool
	if r.Method == http.MethodPost {
		semester, class, subject, selected, err = learningObjectiveSelectionValues(values)
	} else {
		semester, class, subject, selected, err = learningObjectiveSelection(r)
	}
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	if selected {
		for _, c := range contexts {
			if c.SemesterID == semester && c.ClassID == class && c.SubjectID == subject {
				data.Selected = c
				data.SelectedSet = true
				break
			}
		}
		if !data.SelectedSet {
			a.academicFailure(w, academic.ErrForbidden)
			return
		}
	}
	status := http.StatusOK
	message := ""
	draft := map[string]string{}
	versions := map[string]string{}
	if r.Method == http.MethodPost {
		if !data.SelectedSet {
			a.academicFailure(w, academic.ValidationError("Pilih konteks penugasan."))
			return
		}
		var inputs []academic.DirectTPScoreInput
		for name, items := range r.PostForm {
			if !strings.HasPrefix(name, "score_") {
				continue
			}
			key := strings.TrimPrefix(name, "score_")
			parts := strings.Split(key, "_")
			if len(parts) != 2 || len(items) != 1 {
				err = academic.ValidationError("Data sel tidak valid.")
				break
			}
			student, e1 := strconv.ParseInt(parts[0], 10, 64)
			objective, e2 := strconv.ParseInt(parts[1], 10, 64)
			version, e3 := strconv.ParseInt(r.PostForm.Get("version_"+key), 10, 64)
			if e1 != nil || e2 != nil || e3 != nil || student < 1 || objective < 1 {
				err = academic.ValidationError("Identitas atau versi sel tidak valid.")
				break
			}
			draft[key] = items[0]
			versions[key] = r.PostForm.Get("version_" + key)
			if e := academic.ValidateDirectTPValue(items[0]); e != nil {
				data.CellErrors[key] = "Masukkan nilai 0–100 dengan paling banyak dua angka desimal."
			}
			inputs = append(inputs, academic.DirectTPScoreInput{StudentID: student, ObjectiveID: objective, ExpectedVersion: version, Value: items[0]})
		}
		data.Submission = r.PostForm.Get("submission_key")
		if err == nil && len(data.CellErrors) > 0 {
			err = academic.ValidationError("Periksa nilai pada sel yang ditandai. Tidak ada nilai yang disimpan.")
		}
		if err == nil {
			err = a.options.Academics.SaveDirectTPScores(r.Context(), actor, semester, class, subject, data.Submission, inputs)
		}
		if err == nil {
			q := url.Values{"context": {fmt.Sprintf("%d:%d:%d", semester, class, subject)}, "saved": {data.Submission}}
			http.Redirect(w, r, "/guru/buku-nilai?"+q.Encode(), http.StatusSeeOther)
			return
		}
		status, message = academicError(err)
		if status == http.StatusInternalServerError {
			a.failure(w, err)
			return
		}
	}
	if data.SelectedSet {
		data.Book, err = a.options.Academics.DirectTPGradebook(r.Context(), actor, semester, class, subject)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		for i := range data.Book.Students {
			for j := range data.Book.Students[i].Cells {
				cell := &data.Book.Students[i].Cells[j]
				key := directTPCellKey(cell.StudentID, cell.ObjectiveID)
				if val, ok := draft[key]; ok {
					cell.Value = val
					if version, e := strconv.ParseInt(versions[key], 10, 64); e == nil {
						cell.Version = version
					}
				}
			}
		}
	}
	if data.Submission == "" {
		data.Submission, err = newFinalSubmissionKey()
		if err != nil {
			a.failure(w, err)
			return
		}
	}
	v := view{Page: "direct-tp-gradebook", Title: "Buku Nilai Guru", DirectTPGradebook: data, Error: message}
	if r.Method == http.MethodGet && data.SelectedSet && r.URL.Query().Get("saved") != "" {
		saved, e := a.options.Academics.DirectTPSubmissionSaved(r.Context(), actor, semester, class, subject, r.URL.Query().Get("saved"))
		if e != nil {
			a.academicFailure(w, e)
			return
		}
		if saved {
			v.Message = "Nilai berhasil disimpan."
		}
	}
	a.render(w, r, "page", status, v)
}
