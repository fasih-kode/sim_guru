package web

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type transitionView struct {
	Years                  []academic.Year
	Preview                academic.TransitionPreview
	Outcome                map[int64]string
	Destination            map[int64]int64
	Stage                  bool
	SourceYear, TargetYear int64
}

func (v *transitionView) TargetGrade(s academic.TransitionStudent) int {
	switch v.Outcome[s.StudentID] {
	case "promote":
		if s.FromGrade < 9 {
			return s.FromGrade + 1
		}
	case "repeat":
		return s.FromGrade
	}
	return 0
}

func (v *transitionView) OutcomeLabel(s academic.TransitionStudent) string {
	switch v.Outcome[s.StudentID] {
	case "promote":
		return "Naik kelas"
	case "repeat":
		if s.FromGrade == 9 {
			return "Tidak lulus — mengulang kelas 9"
		}
		return "Tidak naik"
	case "graduate":
		return "Lulus"
	case "leave":
		return "Keluar"
	}
	return "Pilih hasil"
}

func (a *application) studentTransition(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	v := view{Page: "student-transition", Title: "Transisi Tahun Ajaran"}
	years, err := a.options.Academics.Years(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	source, target := int64(0), int64(0)
	var decisions []academic.TransitionDecision
	stage, execute := false, false
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		source = formInt64(r, "source_year_id")
		target = formInt64(r, "target_year_id")
		stage = r.PostForm.Get("action") == "preview"
		execute = r.PostForm.Get("action") == "execute"
	}
	if r.Method == http.MethodGet {
		source = formInt64(r, "source_year_id")
		target = formInt64(r, "target_year_id")
	}
	if source == 0 {
		for _, y := range years {
			if y.Active {
				source = y.ID
				break
			}
		}
	}
	if target == 0 {
		for _, y := range years {
			if y.ID != source && !y.Active {
				target = y.ID
				break
			}
		}
	}
	tv := &transitionView{Years: years, Outcome: map[int64]string{}, Destination: map[int64]int64{}, SourceYear: source, TargetYear: target}
	if source > 0 && target > 0 {
		tv.Preview, err = a.options.Academics.TransitionPreview(r.Context(), actor, source, target)
		if err != nil {
			_, v.Error = academicError(err)
			v.Transition = tv
			a.render(w, r, "page", http.StatusUnprocessableEntity, v)
			return
		}
		for _, s := range tv.Preview.Students {
			tv.Outcome[s.StudentID] = ""
		}
		if stage || execute {
			for _, s := range tv.Preview.Students {
				outcome := r.PostForm.Get("outcome_" + strconv.FormatInt(s.StudentID, 10))
				dest, _ := strconv.ParseInt(r.PostForm.Get("destination_"+strconv.FormatInt(s.StudentID, 10)), 10, 64)
				tv.Outcome[s.StudentID] = outcome
				tv.Destination[s.StudentID] = dest
				decisions = append(decisions, academic.TransitionDecision{StudentID: s.StudentID, Outcome: outcome, TargetClassID: dest})
			}
			if execute && r.PostForm.Get("confirm") != "yes" {
				v.Error = "Konfirmasi eksekusi transisi terlebih dahulu."
			} else if stage || execute {
				err = academic.ValidateTransitionDecisions(tv.Preview, decisions)
				if err == nil && execute {
					err = a.options.Academics.ExecuteStudentTransition(r.Context(), actor, source, target, decisions)
					if err == nil {
						q := url.Values{"source_year_id": {strconv.FormatInt(source, 10)}, "target_year_id": {strconv.FormatInt(target, 10)}, "done": {"1"}}
						http.Redirect(w, r, "/akademik/transisi?"+q.Encode(), http.StatusSeeOther)
						return
					}
				}
				if err != nil {
					_, v.Error = academicError(err)
				} else {
					tv.Stage = true
				}
			}
		}
	}
	if r.URL.Query().Get("done") == "1" {
		v.Message = "Transisi tahun ajaran berhasil dicatat."
	}
	v.Transition = tv
	a.render(w, r, "page", http.StatusOK, v)
}
