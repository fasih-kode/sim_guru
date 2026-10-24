package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type movementView struct {
	Student                                      academic.StudentMovement
	History                                      []academic.StudentMovement
	Classes                                      []academic.Class
	Mode, Date, Reason, School, Back, Action     string
	ExpectedExit, Target, ExpectedClass, Version int64
	Stage                                        bool
}

func (a *application) studentMovement(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	actor := principal(r).ID
	mode := r.PathValue("mode")
	student, placement := id, int64(0)
	if mode == "pindah-kelas" {
		student, placement = 0, id
	}
	v := view{Page: "student-movement", Title: "Riwayat Perpindahan Siswa"}
	mv := &movementView{Mode: mode, Date: time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02"), Back: "/master/students", Action: r.URL.Path}
	mv.Student, err = a.options.Academics.MovementStudent(r.Context(), actor, student, placement)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	mv.ExpectedClass, mv.Version = mv.Student.ClassID, mv.Student.Version
	mv.ExpectedExit = mv.Student.ExitID
	mv.History, err = a.options.Academics.StudentMovements(r.Context(), actor, mv.Student.StudentID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	if mode == "pindah-kelas" {
		v.Title = "Pindah Kelas"
		mv.Back = "/admin/placements"
		classes, _, e := a.options.Academics.Classes(r.Context(), actor)
		if e != nil {
			a.academicFailure(w, e)
			return
		}
		for _, c := range classes {
			if c.YearName == mv.Student.YearName && c.Grade == mv.Student.Grade && c.ID != mv.Student.ClassID {
				mv.Classes = append(mv.Classes, c)
			}
		}
	} else if mode == "terima-kembali" {
		v.Title = "Terima Kembali"
		classes, years, e := a.options.Academics.Classes(r.Context(), actor)
		if e != nil {
			a.academicFailure(w, e)
			return
		}
		for _, c := range classes {
			for _, y := range years {
				if c.SchoolYearID == y.ID && y.Active && (y.ID != mv.Student.OriginYear || c.Grade == mv.Student.OriginGrade) {
					mv.Classes = append(mv.Classes, c)
				}
			}
		}
	} else if mode == "pindah-sekolah" {
		v.Title = "Pindah Sekolah"
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		mv.Date = r.PostForm.Get("effective_date")
		mv.Reason = r.PostForm.Get("reason")
		mv.School = r.PostForm.Get("school")
		mv.Target = formInt64(r, "target_class_id")
		mv.ExpectedExit = formInt64(r, "exit_id")
		mv.ExpectedClass = formInt64(r, "expected_class_id")
		mv.Version = formInt64(r, "version")
		if _, e := time.Parse("2006-01-02", mv.Date); e != nil {
			v.Error = "Isi tanggal berlaku yang benar."
		} else if mv.Reason == "" {
			v.Error = "Alasan wajib diisi."
		} else if (mode == "pindah-kelas" || mode == "terima-kembali") && mv.Target == 0 {
			v.Error = "Pilih kelas tujuan."
		} else if r.PostForm.Get("action") == "preview" {
			mv.Stage = true
		} else if r.PostForm.Get("action") == "edit" {
			// Return the submitted values to the editable form.
		} else if r.PostForm.Get("confirm") != "yes" {
			v.Error = "Konfirmasi ringkasan terlebih dahulu."
		} else {
			if mode == "pindah-kelas" {
				err = a.options.Academics.MoveStudentClass(r.Context(), actor, placement, mv.ExpectedClass, mv.Target, mv.Date, mv.Reason)
			} else if mode == "terima-kembali" {
				err = a.options.Academics.ReadmitStudent(r.Context(), actor, student, mv.Version, mv.ExpectedExit, mv.Target, mv.Date, mv.Reason, mv.School)
			} else {
				err = a.options.Academics.ExitStudentSchool(r.Context(), actor, student, mv.Version, mv.Date, mv.Reason, mv.School)
			}
			if err != nil {
				_, v.Error = academicError(err)
			} else {
				http.Redirect(w, r, "/students/"+strconv.FormatInt(mv.Student.StudentID, 10)+"/perpindahan?saved=1", http.StatusSeeOther)
				return
			}
		}
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Perpindahan tersimpan. Daftar aktif mengikuti tanggal berlaku."
	}
	v.Movement = mv
	a.render(w, r, "page", http.StatusOK, v)
}
