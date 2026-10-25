package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type arrivalView struct {
	Settings   academic.ArrivalSettings
	SemesterID int64
	Form       academic.ArrivalRule
	Expected   int64
	Mode       string
	Latest     map[int64]bool
}

func (a *application) arrivalRules(w http.ResponseWriter, r *http.Request) {
	s := a.options.Academics
	actor := principal(r).ID
	settings, e := s.ArrivalSettings(r.Context(), actor, 0)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	semester := selectedSemester(r, settings.Semesters)
	av := &arrivalView{SemesterID: semester, Latest: map[int64]bool{}, Mode: "weekly"}
	v := view{Page: "arrival-rules", Title: "Batas Kedatangan Siswa", Arrival: av}
	code := 200
	if semester == 0 {
		av.Settings = settings
		a.render(w, r, "page", 200, v)
		return
	}
	found := false
	for _, sm := range settings.Semesters {
		if sm.ID == semester {
			found = true
		}
	}
	if !found {
		a.academicFailure(w, academic.ErrNotFound)
		return
	}
	av.Settings, e = s.ArrivalSettings(r.Context(), actor, semester)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	seen := map[string]bool{}
	for _, rule := range av.Settings.Rules {
		key := fmt.Sprintf("%d/%d/%s", rule.ClassID, rule.Weekday, rule.Date)
		av.Latest[rule.ID] = !seen[key]
		seen[key] = true
	}
	av.Form.SemesterID = semester
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		av.Mode = r.PostForm.Get("mode")
		tol, err := strconv.Atoi(r.PostForm.Get("tolerance"))
		class, classErr := strconv.ParseInt(r.PostForm.Get("class_id"), 10, 64)
		if classErr != nil {
			err = classErr
		}
		av.Expected = formInt64(r, "expected")
		av.Form = academic.ArrivalRule{SemesterID: semester, ClassID: class, Weekday: int(formInt64(r, "weekday")), EffectiveFrom: r.PostForm.Get("effective_from"), Deadline: r.PostForm.Get("deadline"), Tolerance: tol, Reason: r.PostForm.Get("reason")}
		if av.Mode == "date" {
			av.Form.Date = r.PostForm.Get("on_date")
			if av.Form.Date == "" {
				err = academic.ValidationError("Isi tanggal pengecualian.")
			}
		} else if av.Mode != "weekly" {
			err = academic.ValidationError("Pilih jenis aturan.")
		}
		if err != nil {
			e = academic.ValidationError("Periksa jenis aturan, tanggal dan toleransi.")
		} else {
			_, e = s.SaveArrivalRule(r.Context(), actor, av.Expected, av.Form)
		}
		if e == nil {
			http.Redirect(w, r, fmt.Sprintf("/kesiswaan/batas-kedatangan?semester_id=%d&saved=1", semester), 303)
			return
		}
		code, v.Error = academicError(e)
		if code >= 500 {
			a.failure(w, e)
			return
		}
		v.ModalOpen = true
	} else if edit, _ := strconv.ParseInt(r.URL.Query().Get("edit"), 10, 64); edit > 0 {
		found = false
		for _, rule := range av.Settings.Rules {
			if rule.ID == edit {
				av.Form = rule
				av.Expected = edit
				found = true
				break
			}
		}
		if !found {
			a.academicFailure(w, academic.ErrNotFound)
			return
		}
		if !av.Latest[edit] {
			a.academicFailure(w, academic.ErrConflict)
			return
		}
		if av.Form.Date != "" {
			av.Mode = "date"
		} else {
			av.Form.EffectiveFrom = ""
		}
		av.Form.Reason = ""
		v.ModalOpen = true
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Versi batas kedatangan tersimpan. Catatan absensi lama tetap menggunakan aturan sebelumnya."
	}
	a.render(w, r, "page", code, v)
}
