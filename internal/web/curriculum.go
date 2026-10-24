package web

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func formInt64(r *http.Request, key string) int64 {
	n, _ := strconv.ParseInt(r.FormValue(key), 10, 64)
	return n
}
func formInt(r *http.Request, key string) int { n, _ := strconv.Atoi(r.FormValue(key)); return n }

func (a *application) curriculumSettings(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	semesters, err := a.options.Academics.Semesters(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semester := selectedSemester(r, semesters)
	page, title, modal := "curriculum-structure", "Struktur Kurikulum", "curriculum-rule-modal"
	isBlocks := r.URL.Path == "/akademik/waktu-sekolah"
	if isBlocks {
		page, title, modal = "school-time-blocks", "Istirahat dan Kegiatan", "school-block-modal"
	}
	v := view{Page: page, Title: title, Academic: &academicView{Semesters: semesters, SelectedSemesterID: semester, RuleForm: academic.CurriculumRule{SemesterID: semester, Grade: 7, DifferentDays: true}, BlockForm: academic.SchoolTimeBlock{SemesterID: semester, Weekday: 1, Kind: "break"}}}
	if semester == 0 {
		v.Message = "Siapkan tahun ajaran dan semester terlebih dahulu."
		code := http.StatusOK
		if r.Method == http.MethodPost {
			code = http.StatusUnprocessableEntity
		}
		a.render(w, r, "page", code, v)
		return
	}
	date, e := time.Parse("2006-01-02", r.URL.Query().Get("date"))
	if e != nil {
		date = time.Now()
	}
	v.Academic.PlanDate = date.Format("2006-01-02")
	load := func() error {
		classes, teachers, subjects, e := a.options.Academics.TeachingOptions(r.Context(), actor, semester)
		if e != nil {
			return e
		}
		v.Academic.TeachingClasses, v.Academic.TeachingTeachers, v.Academic.TeachingSubjects = classes, teachers, subjects
		if isBlocks {
			v.Academic.SchoolBlocks, e = a.options.Academics.SchoolBlocks(r.Context(), actor, semester)
		} else {
			v.Academic.CurriculumRules, e = a.options.Academics.CurriculumRules(r.Context(), actor, semester)
			if e == nil {
				plan, pe := a.options.Academics.SchedulePlan(r.Context(), actor, semester, date)
				v.Academic.EffectiveRules = plan.Rules
				e = pe
			}
		}
		return e
	}
	if err = load(); err != nil {
		a.academicFailure(w, err)
		return
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		switch r.PostForm.Get("action") {
		case "reset":
			err = a.options.Academics.ResetCurriculumOverride(r.Context(), actor, semester, formInt64(r, "id"), formInt64(r, "version"))
		case "copy":
			err = a.options.Academics.CopyCurriculum(r.Context(), actor, formInt64(r, "source_semester"), semester)
		case "remove-block":
			err = a.options.Academics.RemoveSchoolBlock(r.Context(), actor, semester, formInt64(r, "id"), formInt64(r, "version"))
		default:
			if isBlocks {
				b := academic.SchoolTimeBlock{ID: formInt64(r, "id"), SemesterID: semester, Version: formInt64(r, "version"), Weekday: formInt(r, "weekday"), Kind: r.PostForm.Get("kind"), Label: r.PostForm.Get("label"), PeriodNo: formInt(r, "period_no"), StartsAt: r.PostForm.Get("starts_at"), EndsAt: r.PostForm.Get("ends_at")}
				v.Academic.BlockForm = b
				err = a.options.Academics.SaveSchoolBlock(r.Context(), actor, b)
			} else {
				rule := academic.CurriculumRule{ID: formInt64(r, "id"), SemesterID: semester, Version: formInt64(r, "version"), ClassID: formInt64(r, "class_id"), Grade: formInt(r, "grade"), SubjectID: formInt64(r, "subject_id"), WeeklyJP: formInt(r, "weekly_jp"), DifferentDays: r.PostForm.Get("different_days") == "true"}
				v.Academic.RulePattern = r.PostForm.Get("meetings")
				rule.Meetings, err = academic.ParseMeetings(v.Academic.RulePattern)
				if _, parseErr := strconv.Atoi(r.PostForm.Get("weekly_jp")); parseErr != nil {
					err = academic.ValidationError("JP mingguan harus berupa angka 0–210.")
				}
				v.Academic.RuleForm = rule
				if err == nil {
					err = a.options.Academics.SaveCurriculumRule(r.Context(), actor, rule)
				}
			}
		}
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("%s?semester_id=%d&saved=1", r.URL.Path, semester), http.StatusSeeOther)
			return
		}
		code, message := academicError(err)
		v.Error = message
		v.ModalOpen = true
		v.ModalID = modal
		if r.PostForm.Get("action") == "copy" {
			v.ModalID = "copy-curriculum-modal"
		}
		if r.PostForm.Get("action") == "reset" || r.PostForm.Get("action") == "remove-block" {
			v.ModalOpen = false
		}
		if e := load(); e != nil {
			a.academicFailure(w, e)
			return
		}
		a.render(w, r, "page", code, v)
		return
	}
	edit, _ := strconv.ParseInt(r.URL.Query().Get("edit"), 10, 64)
	if isBlocks {
		for _, b := range v.Academic.SchoolBlocks {
			if b.ID == edit {
				b.StartsAt = b.StartsAt[:5]
				b.EndsAt = b.EndsAt[:5]
				v.Academic.BlockForm = b
				v.ModalOpen = true
				v.ModalID = modal
			}
		}
	} else {
		for _, rule := range v.Academic.CurriculumRules {
			if rule.ID == edit {
				v.Academic.RuleForm = rule
				v.Academic.RulePattern = rule.Pattern()
				v.ModalOpen = true
				v.ModalID = modal
			}
		}
		classID, _ := strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
		subjectID, _ := strconv.ParseInt(r.URL.Query().Get("subject_id"), 10, 64)
		if classID > 0 && subjectID > 0 {
			for _, effective := range v.Academic.EffectiveRules {
				if effective.ClassID == classID && effective.SubjectID == subjectID {
					v.Academic.RuleForm = effective.CurriculumRule
					v.Academic.RuleForm.ID = 0
					v.Academic.RuleForm.Version = 0
					if effective.Missing {
						v.Academic.RuleForm.WeeklyJP = 0
					}
					v.Academic.RulePattern = effective.Pattern()
					v.ModalOpen = true
					v.ModalID = modal
				}
			}
		}
	}
	if edit > 0 && !v.ModalOpen {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Pengaturan tersimpan beserta histori. Draf perlu ditinjau atau digenerate ulang; versi terbit tetap utuh."
	}
	a.render(w, r, "page", http.StatusOK, v)
}
