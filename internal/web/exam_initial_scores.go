package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type examInitialView struct {
	Contexts    []academic.LearningObjectiveContext
	Selected    academic.LearningObjectiveContext
	SelectedSet bool
	Date        string
	Tab         string
	Draft       bool
	LegacyDraft bool
	Sheet       academic.ExamInitialSheet
}

type examGradebookRow struct {
	Student   academic.ExamInitialStudent
	Cell      academic.ExamInitialCell
	Component string
}

func (v *examInitialView) Rows() []examGradebookRow {
	var rows []examGradebookRow
	for _, student := range v.Sheet.Students {
		if v.Tab == "asts" || v.LegacyDraft {
			rows = append(rows, examGradebookRow{student, student.ASTS, "asts"})
		}
		if v.Tab == "asas" || v.LegacyDraft {
			rows = append(rows, examGradebookRow{student, student.ASAS, "asas"})
		}
	}
	return rows
}

func (a *application) examInitialScores(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	tab, err := examGradebookTab(r.URL.Query())
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data := &examInitialView{Tab: tab}
	legacyPost := r.Method == http.MethodPost && !r.URL.Query().Has("tab")
	page := func(status int, msg string) {
		a.render(w, r, "page", status, view{Page: "exam-initial-scores", Title: "Buku Nilai Guru", Error: msg, ExamInitial: data})
	}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		code, msg := academicError(err)
		page(code, msg)
		return
	}
	data.Contexts = contexts
	semester, class, subject, selected, err := learningObjectiveSelection(r)
	if err != nil {
		code, msg := academicError(err)
		page(code, msg)
		return
	}
	data.Date = r.URL.Query().Get("date")
	if data.Date == "" {
		data.Date = time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02")
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		semester, class, subject, err = learningObjectiveFormSelection(r)
		if err != nil {
			code, msg := academicError(err)
			page(code, msg)
			return
		}
		selected = true
		data.Date = r.PostForm.Get("roster_date")
	}
	if selected {
		for _, item := range contexts {
			if item.SemesterID == semester && item.ClassID == class && item.SubjectID == subject {
				data.Selected = item
				data.SelectedSet = true
				break
			}
		}
		if !data.SelectedSet {
			a.academicFailure(w, academic.ErrForbidden)
			return
		}
	}
	if r.Method == http.MethodPost {
		inputs, err := examGradebookInputs(r.PostForm, tab, legacyPost)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
		err = a.options.Academics.SaveExamInitialScores(r.Context(), actor, semester, class, subject, inputs)
		if err != nil {
			code, msg := academicError(err)
			if code != http.StatusForbidden && code < 500 {
				data.Sheet, err = a.options.Academics.ExamInitialScores(r.Context(), actor, semester, class, subject, data.Date)
				if err != nil {
					code, msg = academicError(err)
				} else {
					preserveExamInitialInputs(&data.Sheet, inputs)
					data.Draft, data.LegacyDraft = true, legacyPost
				}
			}
			page(code, msg)
			return
		}
		v := url.Values{"context": {strconv.FormatInt(semester, 10) + ":" + strconv.FormatInt(class, 10) + ":" + strconv.FormatInt(subject, 10)}, "date": {data.Date}, "saved": {"1"}}
		if !legacyPost {
			v.Set("tab", tab)
		}
		http.Redirect(w, r, examGradebookPath+"?"+v.Encode(), http.StatusSeeOther)
		return
	}
	if selected {
		data.Sheet, err = a.options.Academics.ExamInitialScores(r.Context(), actor, semester, class, subject, data.Date)
		if err != nil {
			code, msg := academicError(err)
			page(code, msg)
			return
		}
	}
	if r.URL.Query().Get("saved") == "1" {
		a.render(w, r, "page", http.StatusOK, view{Page: "exam-initial-scores", Title: "Buku Nilai Guru", Message: "Nilai awal berhasil disimpan.", ExamInitial: data})
		return
	}
	page(http.StatusOK, "")
}

func examGradebookTab(query url.Values) (string, error) {
	tabs, explicit := query["tab"]
	if !explicit {
		return "asts", nil
	}
	if len(tabs) != 1 || (tabs[0] != "asts" && tabs[0] != "asas") {
		return "", academic.ValidationError("Tab ujian tidak valid.")
	}
	return tabs[0], nil
}

// POST lama tanpa tab tetap dapat mengirim kedua komponen.
func examGradebookInputs(form url.Values, tab string, legacy bool) ([]academic.ExamInitialInput, error) {
	var inputs []academic.ExamInitialInput
	for name, values := range form {
		if !strings.HasPrefix(name, "score_") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(name, "score_"), "_")
		if len(parts) != 2 || len(values) != 1 {
			return nil, academic.ValidationError("Kolom nilai tidak valid.")
		}
		component := parts[0]
		if (component != "asts" && component != "asas") || (!legacy && component != tab) {
			return nil, academic.ValidationError("Komponen nilai tidak sesuai dengan tab ujian.")
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || id < 1 {
			return nil, academic.ValidationError("Siswa tidak valid.")
		}
		dates := form["date_"+component+"_"+parts[1]]
		if len(dates) != 1 {
			return nil, academic.ValidationError("Tanggal pelaksanaan tidak valid.")
		}
		inputs = append(inputs, academic.ExamInitialInput{StudentID: id, Component: component, Value: values[0], Date: dates[0]})
	}
	return inputs, nil
}

// preserveExamInitialInputs hanya mengisi sel kosong setelah pembacaan berotorisasi.
func preserveExamInitialInputs(sheet *academic.ExamInitialSheet, inputs []academic.ExamInitialInput) {
	for _, input := range inputs {
		for i := range sheet.Students {
			if sheet.Students[i].ID != input.StudentID {
				continue
			}
			cell := &sheet.Students[i].ASTS
			if input.Component == "asas" {
				cell = &sheet.Students[i].ASAS
			} else if input.Component != "asts" {
				continue
			}
			if cell.Recorded {
				cell.AttemptedValue, cell.AttemptedDate, cell.Entered = input.Value, input.Date, true
			} else {
				cell.Value = input.Value
				cell.Date = input.Date
				cell.Entered = true
			}
		}
	}
}
