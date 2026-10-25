package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type lessonAttendanceView struct {
	Date     string
	Lessons  []academic.TimetableEntry
	Lesson   *academic.TimetableEntry
	Students []academic.AttendanceStudent
}

func (a *application) lessonAttendance(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	}
	meetings, err := a.options.Academics.TeacherLessonsOnDate(r.Context(), principal(r).ID, date)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := &lessonAttendanceView{Date: date, Lessons: meetings.Lessons}
	if raw := r.URL.Query().Get("entry_id"); raw != "" {
		entryID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || entryID < 1 {
			http.NotFound(w, r)
			return
		}
		context, contextErr := a.options.Academics.TeacherLessonAttendanceOnDate(r.Context(), principal(r).ID, date, entryID)
		if contextErr != nil {
			a.academicFailure(w, contextErr)
			return
		}
		v.Lesson = &context.Lesson
		v.Students = context.Students
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "lesson-attendance", Title: "Absensi per Pelajaran", LessonAttendance: v})
}
