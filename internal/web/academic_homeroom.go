package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func (a *application) classHomeroom(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	c, err := a.options.Academics.Class(r.Context(), actor, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	teachers, err := a.options.Academics.HomeroomTeachers(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	assignments, err := a.options.Academics.HomeroomAssignments(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	av := &academicView{Class: c, HomeroomTeachers: teachers}
	for _, assignment := range assignments {
		if assignment.ClassID == id {
			av.Homeroom = assignment
			break
		}
	}
	v := view{Page: "homeroom-form", Title: "Penugasan Wali Kelas", Academic: av}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		teacherID, parseErr := strconv.ParseInt(r.PostForm.Get("teacher_id"), 10, 64)
		if parseErr != nil {
			err = academic.ValidationError("Pilih guru wali kelas yang valid.")
		} else {
			err = a.options.Academics.AssignHomeroom(r.Context(), actor, id, teacherID)
		}
		if err == nil {
			http.Redirect(w, r, "/akademik/kelas?saved=1", http.StatusSeeOther)
			return
		}
		code, message := academicError(err)
		v.Error = message
		v.ModalOpen = true
		av.Homeroom.TeacherID = teacherID
		a.render(w, r, "page", code, v)
		return
	}
	if len(teachers) == 0 {
		v.Message = "Belum ada guru aktif yang terhubung ke akun aktif dengan role Wali Kelas."
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Penugasan wali kelas tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) classHomeroomHistory(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	actor := principal(r).ID
	c, err := a.options.Academics.Class(r.Context(), actor, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	history, err := a.options.Academics.HomeroomAssignmentHistory(r.Context(), actor, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	for i := range history {
		history[i].At = history[i].At.In(time.FixedZone("WIB", 7*3600))
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "homeroom-history", Title: "Histori Wali Kelas", Academic: &academicView{Class: c, HomeroomHistory: history}})
}

func (a *application) myHomerooms(w http.ResponseWriter, r *http.Request) {
	classes, err := a.options.Academics.MyHomerooms(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "my-homerooms", Title: "Kelas Saya", Academic: &academicView{Homerooms: classes}})
}

func (a *application) myHomeroomRoster(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	roster, err := a.options.Academics.HomeroomRoster(r.Context(), principal(r).ID, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	classes, err := a.options.Academics.MyHomerooms(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	var selected academic.HomeroomAssignment
	for _, item := range classes {
		if item.ClassID == id {
			selected = item
			break
		}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "my-homeroom-roster", Title: "Daftar Siswa", Academic: &academicView{Homeroom: selected, HomeroomRoster: roster}})
}
