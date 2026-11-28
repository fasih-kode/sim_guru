package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type academicView struct {
	ConstraintEditor                                     academic.ConstraintEditor
	ConstraintGrids                                      []academic.ConstraintGrid
	EditingTimetableEntryID                              int64
	ScheduleConstraints                                  []academic.ScheduleConstraint
	TeacherCapacities                                    []academic.TeacherCapacity
	TeacherPreferenceSummaries                           []academic.TeacherPreferenceSummary
	ConstraintForm                                       academic.ScheduleConstraint
	PreferenceViolations                                 []academic.PreferenceViolation
	CurriculumRules                                      []academic.CurriculumRule
	EffectiveRules                                       []academic.EffectiveCurriculum
	SchoolBlocks                                         []academic.SchoolTimeBlock
	RuleForm                                             academic.CurriculumRule
	BlockForm                                            academic.SchoolTimeBlock
	RulePattern, PlanDate, GenerateDate, ScheduleWarning string
	JournalDate                                          string

	Years                                 []academic.Year
	Semesters                             []academic.Semester
	Classes                               []academic.Class
	Class                                 academic.Class
	History                               []academic.History
	CurrentYear, CurrentSemester          string
	Editing                               int64
	HistoryEntity                         string
	PlacementCounts                       map[int64]int
	NewYearName                           string
	Homerooms                             []academic.HomeroomAssignment
	HomeroomByClass                       map[int64]academic.HomeroomAssignment
	HomeroomTeachers                      []academic.HomeroomTeacher
	Homeroom                              academic.HomeroomAssignment
	HomeroomRoster                        []academic.ClassStudent
	HomeroomHistory                       []academic.HomeroomHistory
	SelectedSemesterID                    int64
	TeachingAssignments                   []academic.TeachingAssignment
	EditingAssignment                     academic.TeachingAssignment
	EditEffectiveFrom, EditEffectiveUntil string
	TeachingClasses                       []academic.TeachingOption
	TeachingTeachers                      []academic.TeachingOption
	TeachingSubjects                      []academic.TeachingOption
	TimetableClasses                      []academic.TeachingOption
	TimetableSubjects                     []academic.TeachingOption
	LessonPeriods                         []academic.LessonPeriod
	Timetable                             academic.TimetableRevision
	TimetableEntries                      []academic.TimetableEntry
	PublishedTimetables                   []academic.TimetableRevision
	SelectedWeekday                       int
	LessonDayRows                         []academic.LessonPeriod
	LessonWeekdays                        []int
	EntryClassID, EntrySubjectID          int64
	EntryWeekday, EntryStart, EntryEnd    int
	TeacherSchedule                       bool
	JournalCutoverDate                    string
	JournalHistoricalDate                 string
	JournalHistoricalCandidates           []academic.TeachingJournalHistoricalCandidate
	AssessmentWeight                      academic.AssessmentWeights
	AssessmentWeightInput                 academic.AssessmentWeights
	AssessmentWeightFormSubmitted         bool
}

type placementView struct {
	CreateStudentID                   int64
	CreateStudentName, CreateStartsOn string
	Years                             []academic.Year
	Classes                           []academic.Class
	SelectedClass                     academic.Class
	Candidates                        []academic.StudentOption
	Placements                        []academic.Placement
	YearID                            int64
	ClassID                           int64
	Page                              int
	Previous                          int
	Next                              int
	PreviousURL                       string
	NextURL                           string
	Search                            string
}

func academicError(err error) (int, string) {
	var validation academic.ValidationError
	switch {
	case errors.As(err, &validation):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, academic.ErrForbidden):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, academic.ErrNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, academic.ErrTeacherLessonIneligible):
		return http.StatusConflict, err.Error()
	case errors.Is(err, academic.ErrConflict):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "Data akademik belum dapat diproses. Silakan coba kembali."
	}
}

func (a *application) academicFailure(w http.ResponseWriter, err error) {
	code, message := academicError(err)
	if code >= 500 {
		a.failure(w, err)
		return
	}
	http.Error(w, message, code)
}

func selectedSemester(r *http.Request, semesters []academic.Semester) int64 {
	if raw := r.URL.Query().Get("semester_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	for _, semester := range semesters {
		if semester.Active {
			return semester.ID
		}
	}
	if len(semesters) > 0 {
		return semesters[0].ID
	}
	return 0
}

func (a *application) teachingAssignments(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	semesters, err := a.options.Academics.Semesters(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semesterID := selectedSemester(r, semesters)
	v := view{Page: "teaching-assignments", Title: "Guru Pengampu", Academic: &academicView{Semesters: semesters, SelectedSemesterID: semesterID}}
	load := func() error {
		if semesterID == 0 {
			return nil
		}
		classes, teachers, subjects, e := a.options.Academics.TeachingOptions(r.Context(), actor, semesterID)
		if e != nil {
			return e
		}
		assignments, e := a.options.Academics.TeachingAssignments(r.Context(), actor, semesterID)
		if e != nil {
			return e
		}
		v.Academic.TeachingClasses, v.Academic.TeachingTeachers, v.Academic.TeachingSubjects, v.Academic.TeachingAssignments = classes, teachers, subjects, assignments
		return nil
	}
	if err = load(); err != nil {
		a.academicFailure(w, err)
		return
	}
	if editID, parseErr := strconv.ParseInt(r.URL.Query().Get("edit"), 10, 64); parseErr == nil && editID > 0 {
		for _, assignment := range v.Academic.TeachingAssignments {
			if assignment.ID == editID {
				v.Academic.EditingAssignment = assignment
				v.Academic.EditEffectiveFrom = assignment.EffectiveFrom.Format("2006-01-02")
				if assignment.EffectiveUntil.Valid {
					v.Academic.EditEffectiveUntil = assignment.EffectiveUntil.Time.Format("2006-01-02")
				}
				v.ModalOpen, v.ModalID = true, "edit-teaching-assignment-modal"
				break
			}
		}
		if !v.ModalOpen {
			a.academicFailure(w, academic.ErrNotFound)
			return
		}
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		action := r.PostForm.Get("action")
		if action == "copy" {
			source, _ := strconv.ParseInt(r.PostForm.Get("source_semester_id"), 10, 64)
			target, _ := strconv.ParseInt(r.PostForm.Get("target_semester_id"), 10, 64)
			date, e := time.Parse("2006-01-02", r.PostForm.Get("effective_from"))
			if e == nil {
				_, err = a.options.Academics.CopyTeachingAssignments(r.Context(), actor, source, target, date)
			} else {
				err = academic.ValidationError("Tanggal berlaku tidak valid.")
			}
		} else if action == "correct" {
			assignmentID, _ := strconv.ParseInt(r.PostForm.Get("assignment_id"), 10, 64)
			classID, _ := strconv.ParseInt(r.PostForm.Get("class_id"), 10, 64)
			subjectID, _ := strconv.ParseInt(r.PostForm.Get("subject_id"), 10, 64)
			teacherID, _ := strconv.ParseInt(r.PostForm.Get("teacher_id"), 10, 64)
			date, dateErr := time.Parse("2006-01-02", r.PostForm.Get("effective_from"))
			var until *time.Time
			if raw := r.PostForm.Get("effective_until"); raw != "" {
				parsedUntil, untilErr := time.Parse("2006-01-02", raw)
				if untilErr != nil {
					dateErr = untilErr
				} else {
					until = &parsedUntil
				}
			}
			if dateErr == nil {
				err = a.options.Academics.CorrectTeachingAssignment(r.Context(), actor, assignmentID, semesterID, classID, subjectID, teacherID, date, until)
			} else {
				err = academic.ValidationError("Tanggal berlaku tidak valid.")
			}
		} else {
			classID, _ := strconv.ParseInt(r.PostForm.Get("class_id"), 10, 64)
			subjectID, _ := strconv.ParseInt(r.PostForm.Get("subject_id"), 10, 64)
			teacherID, _ := strconv.ParseInt(r.PostForm.Get("teacher_id"), 10, 64)
			date, e := time.Parse("2006-01-02", r.PostForm.Get("effective_from"))
			if e == nil {
				err = a.options.Academics.AssignTeacher(r.Context(), actor, semesterID, classID, subjectID, teacherID, date)
			} else {
				err = academic.ValidationError("Tanggal berlaku tidak valid.")
			}
		}
		if err == nil {
			notice := "saved=1"
			if action == "correct" {
				notice = "corrected=1"
			}
			http.Redirect(w, r, fmt.Sprintf("/akademik/pengampu?semester_id=%d&%s", semesterID, notice), http.StatusSeeOther)
			return
		}
		_, v.Error = academicError(err)
		v.ModalOpen = true
		if action == "copy" {
			v.ModalID = "copy-assignment-modal"
		} else if action == "correct" {
			v.ModalID = "edit-teaching-assignment-modal"
			v.Academic.EditingAssignment.ID, _ = strconv.ParseInt(r.PostForm.Get("assignment_id"), 10, 64)
			v.Academic.EditingAssignment.ClassID, _ = strconv.ParseInt(r.PostForm.Get("class_id"), 10, 64)
			v.Academic.EditingAssignment.SubjectID, _ = strconv.ParseInt(r.PostForm.Get("subject_id"), 10, 64)
			v.Academic.EditingAssignment.TeacherID, _ = strconv.ParseInt(r.PostForm.Get("teacher_id"), 10, 64)
			v.Academic.EditEffectiveFrom = r.PostForm.Get("effective_from")
			v.Academic.EditEffectiveUntil = r.PostForm.Get("effective_until")
		} else {
			v.ModalID = "teaching-assignment-modal"
		}
		if loadErr := load(); loadErr != nil {
			a.academicFailure(w, loadErr)
			return
		}
		code, _ := academicError(err)
		a.render(w, r, "page", code, v)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Penugasan guru pengampu tersimpan dan tercatat."
	} else if r.URL.Query().Get("corrected") == "1" {
		v.Message = "Koreksi pengampu tersimpan; nilai sebelumnya tercatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

// lessonDayRows displays the twelve standard slots plus any legacy slots.
func lessonDayRows(periods []academic.LessonPeriod, weekday int) []academic.LessonPeriod {
	byNumber := make(map[int]academic.LessonPeriod)
	for _, p := range periods {
		if p.Weekday == weekday {
			p.StartsAt = shortClock(p.StartsAt)
			p.EndsAt = shortClock(p.EndsAt)
			byNumber[p.Number] = p
		}
	}
	var rows []academic.LessonPeriod
	for n := 1; n <= 30; n++ {
		p, ok := byNumber[n]
		if n <= 12 || ok {
			p.Weekday = weekday
			p.Number = n
			rows = append(rows, p)
		}
	}
	return rows
}
func shortClock(v string) string {
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

func (a *application) lessonPeriods(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	semesters, err := a.options.Academics.Semesters(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semesterID := selectedSemester(r, semesters)
	day, _ := strconv.Atoi(r.URL.Query().Get("weekday"))
	if day < 1 || day > 7 {
		day = 1
	}
	v := view{Page: "lesson-periods", Title: "Jam Pelajaran", Academic: &academicView{Semesters: semesters, SelectedSemesterID: semesterID, SelectedWeekday: day, LessonWeekdays: []int{1, 2, 3, 4, 5, 6}}}
	if semesterID > 0 {
		v.Academic.LessonPeriods, err = a.options.Academics.LessonPeriods(r.Context(), actor, semesterID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
	}
	for _, p := range v.Academic.LessonPeriods {
		if p.Weekday == 7 {
			v.Academic.LessonWeekdays = append(v.Academic.LessonWeekdays, 7)
			break
		}
	}
	v.Academic.LessonDayRows = lessonDayRows(v.Academic.LessonPeriods, day)
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		if r.PostForm.Get("action") != "day" {
			a.academicFailure(w, academic.ValidationError("Gunakan formulir jam per hari."))
			return
		}
		day, _ = strconv.Atoi(r.PostForm.Get("weekday"))
		v.Academic.SelectedWeekday = day
		v.Academic.LessonDayRows = lessonDayRows(v.Academic.LessonPeriods, day)
		for i, p := range v.Academic.LessonDayRows {
			v.Academic.LessonDayRows[i].StartsAt = r.PostForm.Get(fmt.Sprintf("starts_%d", p.Number))
			v.Academic.LessonDayRows[i].EndsAt = r.PostForm.Get(fmt.Sprintf("ends_%d", p.Number))
		}
		err = a.options.Academics.SaveLessonDay(r.Context(), actor, semesterID, day, v.Academic.LessonDayRows)
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/akademik/jam-pelajaran?semester_id=%d&weekday=%d&saved=1", semesterID, day), http.StatusSeeOther)
			return
		}
		code, message := academicError(err)
		v.Error = message
		v.ModalOpen = true
		v.ModalID = "lesson-period-modal"
		a.render(w, r, "page", code, v)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Jam hari ini tersimpan; baris kosong dan jadwal terbit tetap dipertahankan."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) weeklyTimetable(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	semesters, err := a.options.Academics.Semesters(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semesterID := selectedSemester(r, semesters)
	v := view{Page: "weekly-timetable", Title: "Jadwal Mingguan", Academic: &academicView{Semesters: semesters, SelectedSemesterID: semesterID}}
	load := func() error {
		if semesterID == 0 {
			return nil
		}
		classes, teachers, subjects, e := a.options.Academics.TeachingOptions(r.Context(), actor, semesterID)
		if e != nil {
			return e
		}
		v.Academic.TeachingClasses, v.Academic.TeachingTeachers, v.Academic.TeachingSubjects = classes, teachers, subjects
		historyID, _ := strconv.ParseInt(r.URL.Query().Get("revision_id"), 10, 64)
		rev, periods, entries, e := a.options.Academics.TimetableAtRevision(r.Context(), actor, semesterID, historyID)
		if e != nil {
			return e
		}
		if rev.ID == 0 {
			periods, e = a.options.Academics.LessonPeriods(r.Context(), actor, semesterID)
			if e != nil {
				return e
			}
		}
		teachingPeriods := make([]academic.LessonPeriod, 0, len(periods))
		for _, p := range periods {
			blocked := false
			for _, entry := range entries {
				if entry.Kind != "" && entry.Weekday == p.Weekday && p.StartsAt < entry.EndsAt && p.EndsAt > entry.StartsAt {
					blocked = true
					break
				}
			}
			if !blocked {
				teachingPeriods = append(teachingPeriods, p)
			}
		}
		v.Academic.Timetable, v.Academic.LessonPeriods, v.Academic.TimetableEntries = rev, teachingPeriods, entries
		v.Academic.GenerateDate = time.Now().Format("2006-01-02")
		if rev.ID > 0 {
			if rev.Status == "draft" {
				v.Academic.GenerateDate = rev.EffectiveFrom.Format("2006-01-02")
			} else if rev.EffectiveFrom.Format("2006-01-02") >= v.Academic.GenerateDate {
				v.Academic.GenerateDate = rev.EffectiveFrom.AddDate(0, 0, 1).Format("2006-01-02")
			}
			var summaryErr error
			v.Academic.PreferenceViolations, summaryErr = a.options.Academics.RevisionPreferenceReport(r.Context(), actor, rev.ID)
			if summaryErr != nil {
				return summaryErr
			}
			v.Academic.TeacherPreferenceSummaries = academic.SummarizeTeacherPreferences(v.Academic.PreferenceViolations)
			v.Academic.EffectiveRules, v.Academic.ScheduleWarning, summaryErr = a.options.Academics.DraftPlanSummary(r.Context(), actor, rev.ID)
			if summaryErr != nil {
				return summaryErr
			}
		}

		planDate, _ := time.Parse("2006-01-02", v.Academic.GenerateDate)
		currentPlan, planErr := a.options.Academics.SchedulePlan(r.Context(), actor, semesterID, planDate)
		if planErr != nil {
			return planErr
		}
		v.Academic.ScheduleConstraints = currentPlan.Constraints
		v.Academic.TeacherCapacities = academic.TeacherCapacities(currentPlan)

		if rev.ID > 0 {
			v.Academic.TeachingAssignments, e = a.options.Academics.ScheduleAssignments(r.Context(), actor, semesterID, rev.EffectiveFrom)
			if e != nil {
				return e
			}
		}
		v.Academic.PublishedTimetables, e = a.options.Academics.PublishedTimetables(r.Context(), actor, semesterID)
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
		case "generate":
			date, e := time.Parse("2006-01-02", r.PostForm.Get("effective_from"))
			if e != nil {
				err = academic.ValidationError("Tanggal berlaku tidak valid.")
			} else {
				_, err = a.options.Academics.GenerateTimetable(r.Context(), actor, semesterID, date, formInt64(r, "draft_id"), r.PostForm.Get("replace_draft") == "yes")
			}
		case "draft":
			date, e := time.Parse("2006-01-02", r.PostForm.Get("effective_from"))
			if e != nil {
				err = academic.ValidationError("Tanggal berlaku tidak valid.")
			} else {
				_, err = a.options.Academics.CreateTimetableDraft(r.Context(), actor, semesterID, date)
			}
		case "entry", "correct-entry":
			weekday, _ := strconv.Atoi(r.PostForm.Get("weekday"))
			classID, _ := strconv.ParseInt(r.PostForm.Get("class_id"), 10, 64)
			subjectID, _ := strconv.ParseInt(r.PostForm.Get("subject_id"), 10, 64)
			// Resolve the teacher from server-side assignments; ignore submitted teacher IDs.
			var teacherID int64
			for _, assignment := range v.Academic.TeachingAssignments {
				if assignment.ClassID == classID && assignment.SubjectID == subjectID {
					teacherID = assignment.TeacherID
					break
				}
			}
			start, _ := strconv.Atoi(r.PostForm.Get("period_start"))
			end, _ := strconv.Atoi(r.PostForm.Get("period_end"))
			v.Academic.EntryClassID, v.Academic.EntrySubjectID = classID, subjectID
			v.Academic.EntryWeekday, v.Academic.EntryStart, v.Academic.EntryEnd = weekday, start, end
			if r.PostForm.Get("action") == "correct-entry" {
				v.Academic.EditingTimetableEntryID = formInt64(r, "entry_id")
				err = a.options.Academics.CorrectTimetableEntry(r.Context(), actor, v.Academic.Timetable.ID, v.Academic.EditingTimetableEntryID, weekday, classID, subjectID, teacherID, start, end)
			} else {
				err = a.options.Academics.AddTimetableEntry(r.Context(), actor, v.Academic.Timetable.ID, weekday, classID, subjectID, teacherID, start, end)
			}
		case "remove":
			entryID, _ := strconv.ParseInt(r.PostForm.Get("entry_id"), 10, 64)
			err = a.options.Academics.RemoveTimetableEntry(r.Context(), actor, v.Academic.Timetable.ID, entryID)
		case "publish":
			err = a.options.Academics.PublishTimetable(r.Context(), actor, v.Academic.Timetable.ID)
		default:
			err = academic.ValidationError("Aksi jadwal tidak dikenal.")
		}
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/akademik/jadwal?semester_id=%d&saved=1", semesterID), http.StatusSeeOther)
			return
		}
		_, v.Error = academicError(err)
		if action := r.PostForm.Get("action"); action == "draft" || action == "entry" || action == "correct-entry" || action == "generate" {
			v.ModalOpen = true
			if action == "generate" {
				v.ModalID = "generate-timetable-modal"
			} else if action == "draft" {
				v.ModalID = "new-timetable-modal"
			} else {
				v.ModalID = "timetable-entry-modal"
			}
		}
		if loadErr := load(); loadErr != nil {
			a.academicFailure(w, loadErr)
			return
		}
		code, _ := academicError(err)
		a.render(w, r, "page", code, v)
		return
	}
	if editID, _ := strconv.ParseInt(r.URL.Query().Get("edit_entry"), 10, 64); editID > 0 {
		found := false
		if v.Academic.Timetable.Status == "draft" {
			for _, entry := range v.Academic.TimetableEntries {
				if entry.ID == editID && entry.Kind == "" {
					v.Academic.EditingTimetableEntryID = editID
					v.Academic.EntryClassID = entry.ClassID
					v.Academic.EntrySubjectID = entry.SubjectID
					v.Academic.EntryWeekday = entry.Weekday
					v.Academic.EntryStart = entry.PeriodStart
					v.Academic.EntryEnd = entry.PeriodEnd
					v.ModalOpen = true
					v.ModalID = "timetable-entry-modal"
					found = true
					break
				}
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Perubahan jadwal tersimpan dan tercatat."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) myTimetable(w http.ResponseWriter, r *http.Request) {
	rev, entries, err := a.options.Academics.PublishedTimetableForTeacher(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	classes, subjects := timetableFilterOptions(entries)
	journalDate := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	a.render(w, r, "page", http.StatusOK, view{Page: "my-schedule", Title: "Jadwal Saya", Academic: &academicView{Timetable: rev, TimetableEntries: entries, TimetableClasses: classes, TimetableSubjects: subjects, TeacherSchedule: true, JournalDate: journalDate}})
}

func timetableFilterOptions(entries []academic.TimetableEntry) ([]academic.TeachingOption, []academic.TeachingOption) {
	classesByID := make(map[int64]string)
	subjectsByID := make(map[int64]string)
	for _, entry := range entries {
		if entry.Kind != "" {
			continue
		}
		classesByID[entry.ClassID] = entry.ClassName
		subjectsByID[entry.SubjectID] = entry.SubjectName
	}
	options := func(values map[int64]string) []academic.TeachingOption {
		result := make([]academic.TeachingOption, 0, len(values))
		for id, name := range values {
			result = append(result, academic.TeachingOption{ID: id, Name: name})
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		return result
	}
	return options(classesByID), options(subjectsByID)
}

func (a *application) academicHome(w http.ResponseWriter, r *http.Request) {
	years, err := a.options.Academics.Years(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semesters, err := a.options.Academics.Semesters(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	classes, _, err := a.options.Academics.Classes(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	av := &academicView{Years: years, Semesters: semesters, Classes: classes}
	for _, year := range years {
		if year.Active {
			av.CurrentYear = year.Name
		}
	}
	for _, semester := range semesters {
		if semester.Active {
			av.CurrentSemester = "Semester " + strconv.Itoa(semester.Number)
		}
	}
	v := view{Page: "academic", Title: "Akademik", Academic: av}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Perubahan data akademik tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func placementPage(a *application, r *http.Request, yearID, classID int64, search string, page int) (*placementView, int, error) {
	if page < 1 {
		page = 1
	}
	if page > 10000 || len(search) > 480 {
		return nil, http.StatusBadRequest, academic.ValidationError("Pencarian atau halaman tidak valid.")
	}
	actor := principal(r).ID
	years, err := a.options.Academics.Years(r.Context(), actor)
	if err != nil {
		code, _ := academicError(err)
		return nil, code, err
	}
	classes, _, err := a.options.Academics.Classes(r.Context(), actor)
	if err != nil {
		code, _ := academicError(err)
		return nil, code, err
	}
	if yearID == 0 {
		for _, year := range years {
			if year.Active {
				yearID = year.ID
				break
			}
		}
		if yearID == 0 && len(years) > 0 {
			yearID = years[0].ID
		}
	}
	if yearID > 0 {
		found := false
		for _, year := range years {
			if year.ID == yearID {
				found = true
				break
			}
		}
		if !found {
			return nil, http.StatusNotFound, academic.ErrNotFound
		}
	}
	p := &placementView{Years: years, YearID: yearID, ClassID: classID, Page: page, Search: search}
	if principal(r).HasRole("admin") {
		p.CreateStartsOn, err = a.options.Academics.SuggestedPlacementStart(r.Context(), principal(r).ID, yearID)
		if err != nil {
			return nil, 500, err
		}
	}
	for _, class := range classes {
		if class.SchoolYearID == yearID {
			p.Classes = append(p.Classes, class)
			if classID == class.ID {
				p.SelectedClass = class
				classID = 0
			}
		}
	}
	if p.ClassID != 0 && classID != 0 {
		return nil, http.StatusNotFound, academic.ErrNotFound
	}
	if yearID == 0 {
		return p, http.StatusOK, nil
	}
	p.Placements, err = a.options.Academics.Placements(r.Context(), actor, yearID, p.ClassID, search, page)
	if err != nil {
		code, _ := academicError(err)
		return nil, code, err
	}
	for i := range p.Placements {
		p.Placements[i].At = p.Placements[i].At.In(time.FixedZone("WIB", 7*3600))
	}
	p.Previous = 0
	if page > 1 {
		p.Previous = page - 1
	}
	if len(p.Placements) > 10 {
		p.Next = page + 1
		p.Placements = p.Placements[:10]
	}
	if p.ClassID > 0 && principal(r).HasRole("admin") {
		p.Candidates, err = a.options.Academics.UnplacedStudents(r.Context(), actor, yearID, search, page)
		if err != nil {
			code, _ := academicError(err)
			return nil, code, err
		}
		if len(p.Candidates) > 10 {
			p.Candidates = p.Candidates[:10]
			if p.Next == 0 {
				p.Next = page + 1
			}
		}
	}
	if p.Previous > 0 {
		p.PreviousURL = placementPageURL(yearID, p.ClassID, search, p.Previous)
	}
	if p.Next > 0 {
		p.NextURL = placementPageURL(yearID, p.ClassID, search, p.Next)
	}
	return p, http.StatusOK, nil
}

func placementPageURL(yearID, classID int64, search string, page int) string {
	query := url.Values{}
	query.Set("year_id", strconv.FormatInt(yearID, 10))
	query.Set("class_id", strconv.FormatInt(classID, 10))
	query.Set("q", search)
	query.Set("page", strconv.Itoa(page))
	return "/admin/placements?" + query.Encode()
}

func (a *application) studentPlacements(w http.ResponseWriter, r *http.Request) {
	var yearID, classID int64
	var page int
	var search string
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		var err error
		yearID, err = strconv.ParseInt(r.PostForm.Get("year_id"), 10, 64)
		if err != nil || yearID < 1 {
			http.Error(w, "Pilih tahun ajaran yang valid.", http.StatusUnprocessableEntity)
			return
		}
		classID, err = strconv.ParseInt(r.PostForm.Get("class_id"), 10, 64)
		if err != nil || classID < 1 {
			http.Error(w, "Pilih kelas yang valid.", http.StatusUnprocessableEntity)
			return
		}
		studentID, err := strconv.ParseInt(r.PostForm.Get("student_id"), 10, 64)
		if err != nil || studentID < 1 {
			http.Error(w, "Pilih siswa yang valid.", http.StatusUnprocessableEntity)
			return
		}
		search = r.PostForm.Get("q")
		page, _ = strconv.Atoi(r.PostForm.Get("page"))
		if _, code, err := placementPage(a, r, yearID, classID, search, page); err != nil {
			code, message := academicError(err)
			http.Error(w, message, code)
			return
		} else if code != http.StatusOK {
			http.Error(w, "Pilihan tahun atau kelas tidak ditemukan.", code)
			return
		}
		if _, err = a.options.Academics.CreatePlacement(r.Context(), principal(r).ID, studentID, classID, r.PostForm.Get("starts_on")); err != nil {
			p, _, loadErr := placementPage(a, r, yearID, classID, search, page)
			if loadErr != nil {
				a.academicFailure(w, loadErr)
				return
			}
			_, message := academicError(err)
			code, _ := academicError(err)
			p.CreateStudentID = studentID
			p.CreateStartsOn = r.PostForm.Get("starts_on")
			for _, student := range p.Candidates {
				if student.ID == studentID {
					p.CreateStudentName = student.Name
					break
				}
			}
			a.render(w, r, "page", code, view{Page: "student-placements", Title: "Penempatan Siswa", Error: message, Placement: p, ModalOpen: true, ModalID: "new-placement-modal"})
			return
		}
		query := make(url.Values)
		query.Set("year_id", strconv.FormatInt(yearID, 10))
		query.Set("class_id", strconv.FormatInt(classID, 10))
		query.Set("q", search)
		if page > 0 {
			query.Set("page", strconv.Itoa(page))
		}
		query.Set("saved", "1")
		http.Redirect(w, r, "/admin/placements?"+query.Encode(), http.StatusSeeOther)
		return
	}
	yearID, _ = strconv.ParseInt(r.URL.Query().Get("year_id"), 10, 64)
	classID, _ = strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	search = r.URL.Query().Get("q")
	p, code, err := placementPage(a, r, yearID, classID, search, page)
	if err != nil {
		if code >= 500 {
			a.failure(w, err)
			return
		}
		_, message := academicError(err)
		http.Error(w, message, code)
		return
	}
	v := view{Page: "student-placements", Title: "Penempatan Siswa", Placement: p}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Penempatan awal tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) placementHistory(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	history, err := a.options.Academics.PlacementHistory(r.Context(), principal(r).ID, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	for i := range history {
		history[i].At = history[i].At.In(time.FixedZone("WIB", 7*3600))
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "placement-history", Title: "Histori Penempatan", PlacementHistory: history})
}

func (a *application) academicHistory(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	entity := r.PathValue("entity")
	history, err := a.options.Academics.History(r.Context(), principal(r).ID, entity, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	for i := range history {
		history[i].At = history[i].At.In(time.FixedZone("WIB", 7*3600))
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "academic-history", Title: "Histori Akademik", Academic: &academicView{History: history, Class: academic.Class{ID: id}, Editing: id, HistoryEntity: entity}})
}

func parseAcademicID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid academic ID")
	}
	return id, nil
}
