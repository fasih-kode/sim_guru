package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func (a *application) scheduleConstraints(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	if r.Method == http.MethodPost && !parseForm(w, r) {
		return
	}
	semesters, err := a.options.Academics.Semesters(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semester := selectedSemester(r, semesters)
	v := view{Page: "schedule-constraints", Title: "Batasan Jadwal", Academic: &academicView{Semesters: semesters, SelectedSemesterID: semester}}
	if semester == 0 {
		v.Message = "Siapkan semester terlebih dahulu."
		a.render(w, r, "page", http.StatusOK, v)
		return
	}
	_, v.Academic.TeachingTeachers, v.Academic.TeachingSubjects, err = a.options.Academics.TeachingOptions(r.Context(), actor, semester)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	p, err := a.options.Academics.SchedulePlan(r.Context(), actor, semester, time.Now())
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	for _, c := range p.Constraints {
		if c.ID > 0 {
			v.Academic.ScheduleConstraints = append(v.Academic.ScheduleConstraints, c)
		}
	}
	v.Academic.ConstraintGrids = p.ConstraintGrids
	v.Academic.TeacherCapacities = academic.TeacherCapacities(p)
	teacher, subject := int64(0), int64(0)
	kind := r.FormValue("target_kind")
	target := formInt64(r, "target_id")
	switch kind {
	case "teacher":
		teacher = target
	case "subject":
		subject = target
	}
	open := r.FormValue("grid") == "1" || r.PostForm.Get("action") == "grid"
	if edit := formInt64(r, "edit"); edit > 0 {
		for _, c := range p.Constraints {
			if c.ID == edit {
				teacher, subject = c.TeacherID, c.SubjectID
				open = true
				break
			}
		}
		if !open {
			http.NotFound(w, r)
			return
		}
	}
	states := map[string]string{}
	if r.Method == http.MethodPost {
		if r.PostForm.Get("action") == "grid" {
			for key, values := range r.PostForm {
				if strings.HasPrefix(key, "cell_") {
					if len(values) != 1 {
						err = academic.ValidationError("Status jam tidak boleh dikirim berulang.")
						break
					}
					states[key] = values[0]
				}
			}
			if err == nil {
				err = a.options.Academics.SaveConstraintGrid(r.Context(), actor, semester, teacher, subject, r.PostForm.Get("grid_token"), states, r.PostForm.Get("reviewed") == "yes")
			}
		} else {
			// Compatibility for already-open legacy forms; new settings use the grid.
			if r.PostForm.Get("action") == "remove-grid" {
				err = a.options.Academics.RemoveConstraintGrid(r.Context(), actor, semester, formInt64(r, "grid_id"), formInt64(r, "grid_version"))
			} else if r.PostForm.Get("action") == "remove" {
				err = a.options.Academics.RemoveScheduleConstraint(r.Context(), actor, semester, formInt64(r, "id"), formInt64(r, "version"))
			} else {
				c := academic.ScheduleConstraint{ID: formInt64(r, "id"), Version: formInt64(r, "version"), SemesterID: semester, TeacherID: teacher, SubjectID: subject, Severity: r.PostForm.Get("severity"), AllDay: r.PostForm.Get("all_day") == "true", StartsAt: r.PostForm.Get("starts_at"), EndsAt: r.PostForm.Get("ends_at")}
				for _, raw := range r.PostForm["weekdays"] {
					d, e := strconv.Atoi(raw)
					if e != nil {
						err = academic.ValidationError("Hari tidak valid.")
						break
					}
					c.Weekdays = append(c.Weekdays, d)
				}
				if err == nil {
					err = a.options.Academics.SaveScheduleConstraint(r.Context(), actor, c)
				}
			}
		}
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/akademik/batasan-jadwal?semester_id=%d&saved=1", semester), http.StatusSeeOther)
			return
		}
		open = teacher > 0 || subject > 0
	}
	if open {
		name := ""
		if teacher > 0 {
			for _, t := range v.Academic.TeachingTeachers {
				if t.ID == teacher {
					name = t.Name
					break
				}
			}
		} else if subject > 0 {
			for _, s := range v.Academic.TeachingSubjects {
				if s.ID == subject {
					name = s.Name
					break
				}
			}
		}
		if name == "" {
			if err == nil {
				err = academic.ValidationError("Pilih guru/akun Guru Mapel atau mata pelajaran aktif.")
			}
		} else {
			v.Academic.ConstraintEditor = academic.BuildConstraintEditor(p, teacher, subject, name)
			if r.Method == http.MethodPost {
				v.Academic.ConstraintEditor.ApplyGridForm(states)
			}
			v.ModalOpen = true
			v.ModalID = "constraint-grid-modal"
		}
	}
	code := http.StatusOK
	if err != nil {
		code, v.Error = academicError(err)
	} else if r.URL.Query().Get("saved") == "1" {
		v.Message = "Kisi tersimpan beserta histori. Generate ulang draf untuk memakai batasan terbaru; versi terbit tetap tersimpan."
	}
	a.render(w, r, "page", code, v)
}
