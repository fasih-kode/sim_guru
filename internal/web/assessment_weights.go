package web

import (
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func (a *application) assessmentWeights(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	semesters, err := a.options.Academics.Semesters(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semesterID := selectedSemester(r, semesters)
	v := view{Page: "assessment-weights", Title: "Bobot Penilaian", Academic: &academicView{
		Semesters: semesters, SelectedSemesterID: semesterID,
		AssessmentWeight: academic.AssessmentWeights{SemesterID: semesterID},
	}}
	if semesterID == 0 {
		v.Message = "Siapkan tahun ajaran dan semester terlebih dahulu."
		code := http.StatusOK
		if r.Method == http.MethodPost {
			code = http.StatusUnprocessableEntity
		}
		a.render(w, r, "page", code, v)
		return
	}
	load := func() error {
		var loadErr error
		v.Academic.AssessmentWeight, loadErr = a.options.Academics.AssessmentWeight(r.Context(), actor, semesterID)
		return loadErr
	}
	if err = load(); err != nil {
		a.academicFailure(w, err)
		return
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		weights := academic.AssessmentWeights{SemesterID: semesterID}
		weights.Version = v.Academic.AssessmentWeight.Version
		values := []*int{&weights.NH, &weights.ASTS, &weights.ASAS}
		for i, key := range []string{"nh", "asts", "asas"} {
			value, parseErr := strconv.Atoi(r.PostForm.Get(key))
			if parseErr != nil {
				err = academic.ValidationError("Isi bobot NH, ASTS, dan ASAS dengan bilangan bulat 0–100.")
				break
			}
			*values[i] = value
		}
		expectedVersion, versionErr := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
		if err == nil && versionErr != nil {
			err = academic.ValidationError("Versi konfigurasi tidak valid. Muat ulang halaman.")
		}
		v.Academic.AssessmentWeightInput = weights
		v.Academic.AssessmentWeightFormSubmitted = true
		if err == nil {
			err = a.options.Academics.SaveAssessmentWeight(r.Context(), actor, weights, expectedVersion)
		}
		if err == nil {
			http.Redirect(w, r, "/akademik/bobot-penilaian?semester_id="+strconv.FormatInt(semesterID, 10)+"&saved=1", http.StatusSeeOther)
			return
		}
		code, message := academicError(err)
		v.Error = message
		if loadErr := load(); loadErr != nil {
			a.academicFailure(w, loadErr)
			return
		}
		// Pertahankan angka yang dikirim agar Kurikulum bisa memperbaiki totalnya.
		a.render(w, r, "page", code, v)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Bobot penilaian semester berhasil disimpan dan dicatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}
