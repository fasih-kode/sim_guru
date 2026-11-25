package web

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
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
	Draft       bool
	Conflict    bool
	Reviewed    bool
	Conflicts   []directTPConflict
	Missing     []directTPMissingDraft
	Originals   map[string]string
	DraftKeys   map[string]bool
}

type directTPConflict struct {
	Student, Objective, Stored, Draft string
}

type directTPMissingDraft struct {
	Key, Value, Version, Original string
}

func directTPCellKey(student, objective int64) string {
	return fmt.Sprintf("%d_%d", student, objective)
}

func (a *application) directTPGradebook(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	data := &directTPGradebookView{CellErrors: map[string]string{}, Originals: map[string]string{}, DraftKeys: map[string]bool{}}
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
	originals := map[string]string{}
	unchanged := map[string]int64{}
	review := r.Method == http.MethodPost && r.PostForm.Get("action") == "review_latest"
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
			original, hasOriginal := r.PostForm["original_"+key]
			if e1 != nil || e2 != nil || e3 != nil || student < 1 || objective < 1 || !hasOriginal || len(original) != 1 {
				err = academic.ValidationError("Identitas atau versi sel tidak valid.")
				break
			}
			if items[0] == original[0] {
				unchanged[key] = version
				continue
			}
			draft[key] = items[0]
			versions[key] = r.PostForm.Get("version_" + key)
			originals[key] = original[0]
			data.DraftKeys[key] = true
			if e := academic.ValidateDirectTPValue(items[0]); e != nil {
				data.CellErrors[key] = "Masukkan nilai 0–100 dengan paling banyak dua angka desimal."
			}
			inputs = append(inputs, academic.DirectTPScoreInput{StudentID: student, ObjectiveID: objective, ExpectedVersion: version, Value: items[0]})
		}
		data.Submission = r.PostForm.Get("submission_key")
		if err == nil && len(data.CellErrors) > 0 {
			err = academic.ValidationError("Periksa nilai pada sel yang ditandai. Tidak ada nilai yang disimpan.")
		}
		if err == nil && !review && len(inputs) == 0 {
			err = academic.ValidationError("Tidak ada perubahan untuk disimpan.")
		}
		// Sel yang tidak diedit hanya diperiksa untuk menginformasikan perubahan
		// tabel; sel tersebut tidak pernah diteruskan sebagai mutasi.
		if err == nil && !review && len(unchanged) > 0 {
			var saved bool
			saved, err = a.options.Academics.DirectTPSubmissionSaved(r.Context(), actor, semester, class, subject, data.Submission)
			if err == nil && !saved {
				var book academic.DirectTPBook
				book, err = a.options.Academics.DirectTPGradebook(r.Context(), actor, semester, class, subject)
				if err == nil {
					for _, student := range book.Students {
						for _, cell := range student.Cells {
							if v, ok := unchanged[directTPCellKey(cell.StudentID, cell.ObjectiveID)]; ok && v != cell.Version {
								err = academic.ErrConflict
							}
						}
					}
				}
			}
		}
		if err == nil && !review {
			err = a.options.Academics.SaveDirectTPScores(r.Context(), actor, semester, class, subject, data.Submission, inputs)
		}
		if err == nil && !review {
			q := url.Values{"context": {fmt.Sprintf("%d:%d:%d", semester, class, subject)}, "saved": {data.Submission}}
			if date := gradebookDate(r.PostForm.Get("date")); date != "" {
				q.Set("date", date)
			}
			http.Redirect(w, r, "/guru/buku-nilai?"+q.Encode(), http.StatusSeeOther)
			return
		}
		if err != nil {
			status, message = academicError(err)
		}
		if status == http.StatusInternalServerError {
			a.failure(w, err)
			return
		}
		data.Draft = len(draft) > 0
		data.Conflict = status == http.StatusConflict
		data.Reviewed = review && err == nil
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
				data.Originals[key] = cell.Value
				if val, ok := draft[key]; ok {
					if old, e := strconv.ParseInt(versions[key], 10, 64); e == nil && (data.Conflict || old != cell.Version || !cell.Editable) {
						stored := cell.Value
						if stored == "" {
							stored = "Belum dinilai"
						}
						data.Conflicts = append(data.Conflicts, directTPConflict{Student: data.Book.Students[i].Name, Objective: cell.Code, Stored: stored, Draft: val})
					}
					cell.Value = val
					if !data.Reviewed {
						data.Originals[key] = originals[key]
					}
					if version, e := strconv.ParseInt(versions[key], 10, 64); e == nil && !data.Reviewed {
						cell.Version = version
					}
					delete(draft, key)
				}
			}
		}
		keys := make([]string, 0, len(draft))
		for key := range draft {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			data.Missing = append(data.Missing, directTPMissingDraft{Key: key, Value: draft[key], Version: versions[key], Original: originals[key]})
		}
	}
	if data.Reviewed {
		data.Submission = ""
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
