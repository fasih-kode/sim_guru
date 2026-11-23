package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type directTPApplicabilityView struct {
	Context                                   academic.LearningObjectiveContext
	Student                                   academic.DirectTPBookStudent
	Cell                                      academic.DirectTPBookCell
	Submission, Date, Reason, Choice, Version string
}

func (a *application) directTPApplicability(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	values := r.URL.Query()
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Form tidak valid.", http.StatusBadRequest)
			return
		}
		values = r.PostForm
	}
	semester, class, subject, selected, err := learningObjectiveSelectionValues(values)
	student, e1 := strconv.ParseInt(values.Get("student_id"), 10, 64)
	objective, e2 := strconv.ParseInt(values.Get("objective_id"), 10, 64)
	if err != nil || !selected || e1 != nil || e2 != nil || student < 1 || objective < 1 {
		a.academicFailure(w, academic.ValidationError("Pilih siswa dan TP yang valid."))
		return
	}
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	var data directTPApplicabilityView
	found := false
	for _, c := range contexts {
		if c.SemesterID == semester && c.ClassID == class && c.SubjectID == subject {
			data.Context = c
			found = true
			break
		}
	}
	if !found {
		a.academicFailure(w, academic.ErrForbidden)
		return
	}
	book, err := a.options.Academics.DirectTPGradebook(r.Context(), actor, semester, class, subject)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	found = false
	for _, st := range book.Students {
		if st.ID == student {
			for _, cell := range st.Cells {
				if cell.ObjectiveID == objective && cell.CanSetApplicability {
					data.Student = st
					data.Cell = cell
					found = true
				}
			}
		}
	}
	if !found {
		a.academicFailure(w, academic.ErrForbidden)
		return
	}
	data.Submission = values.Get("submission_key")
	data.Date = values.Get("effective_date")
	data.Reason = values.Get("reason")
	data.Choice = values.Get("applicable")
	data.Version = strconv.FormatInt(data.Cell.ApplicabilityVersion, 10)
	status := http.StatusOK
	message := ""
	if r.Method == http.MethodPost {
		data.Version = values.Get("version")
		version, e := strconv.ParseInt(values.Get("version"), 10, 64)
		if e != nil || (data.Choice != "true" && data.Choice != "false") {
			err = academic.ValidationError("Keputusan atau versi tidak valid.")
		} else {
			err = a.options.Academics.SaveDirectTPApplicability(r.Context(), actor, academic.DirectTPApplicabilityInput{SemesterID: semester, ClassID: class, SubjectID: subject, StudentID: student, ObjectiveID: objective, ExpectedVersion: version, EffectiveDate: data.Date, Applicable: data.Choice == "true", EvidenceVerified: values.Get("evidence_verified") == "yes", Reason: data.Reason, SubmissionKey: data.Submission})
		}
		if err == nil {
			q := url.Values{"context": {fmt.Sprintf("%d:%d:%d", semester, class, subject)}}
			http.Redirect(w, r, "/guru/buku-nilai?"+q.Encode(), http.StatusSeeOther)
			return
		}
		status, message = academicError(err)
		if status == http.StatusInternalServerError {
			a.failure(w, err)
			return
		}
	}
	if data.Submission == "" {
		data.Submission, err = newFinalSubmissionKey()
		if err != nil {
			a.failure(w, err)
			return
		}
	}
	a.render(w, r, "page", status, view{Page: "direct-tp-applicability", Title: "Penetapan Keberlakuan TP", DirectTPApplicability: &data, Error: message})
}
