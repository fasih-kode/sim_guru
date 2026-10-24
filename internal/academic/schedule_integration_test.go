//go:build integration

package academic_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestScheduleTemplateIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := academic.Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	other, _ := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	year, err := s.CreateYear(ctx, curriculum.ID, "Template "+testutil.Random(t)[:10])
	must(err)
	var semester int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester))
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	classB, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "B", "", 0)
	must(err)
	var teacherID, otherID, subjectID int64
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Template',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Lain',$1) RETURNING id`, other.ID).Scan(&otherID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$1) RETURNING id`, "Mapel "+testutil.Random(t)[:10]).Scan(&subjectID))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	must(s.AssignTeacher(ctx, curriculum.ID, semester, class.ID, subjectID, teacherID, date))
	must(s.AssignTeacher(ctx, curriculum.ID, semester, classB.ID, subjectID, teacherID, date))
	periods := []academic.LessonPeriod{{Number: 1, StartsAt: "07:00", EndsAt: "07:40"}, {Number: 2, StartsAt: "07:40", EndsAt: "08:20"}, {Number: 3}}
	if err = s.SaveLessonDay(ctx, admin.ID, semester, 1, periods); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Admin can write periods", err)
	}
	must(s.SaveLessonDay(ctx, curriculum.ID, semester, 1, periods))
	must(s.SaveLessonPeriod(ctx, curriculum.ID, semester, 7, 15, "09:00", "09:40"))
	before, err := s.LessonPeriods(ctx, curriculum.ID, semester)
	must(err)
	if len(before) != 3 {
		t.Fatal("blank row persisted or legacy missing", before)
	}
	var histories int
	must(db.QueryRow(`SELECT count(*) FROM app.lesson_period_history h JOIN app.lesson_periods p ON p.id=h.period_id WHERE p.semester_id=$1`, semester).Scan(&histories))
	// Validate the final batch, not each row against its stale neighbor.
	shifted := []academic.LessonPeriod{{Number: 1, StartsAt: "07:40", EndsAt: "08:20"}, {Number: 2, StartsAt: "08:20", EndsAt: "09:00"}}
	must(s.SaveLessonDay(ctx, curriculum.ID, semester, 1, shifted))
	for _, bad := range [][]academic.LessonPeriod{
		{{Number: 1, StartsAt: "08:00", EndsAt: "08:40"}, {Number: 2, StartsAt: "08:20", EndsAt: "09:00"}},
		{{Number: 1, StartsAt: "06:00", EndsAt: "06:40"}, {Number: 2, StartsAt: "09:00"}},
		{{Number: 1}, {Number: 1}},
		{{Number: 31, StartsAt: "09:00", EndsAt: "10:00"}},
	} {
		if err = s.SaveLessonDay(ctx, curriculum.ID, semester, 1, bad); err == nil {
			t.Fatal("invalid batch accepted")
		}
	}
	must(s.SaveLessonDay(ctx, curriculum.ID, semester, 1, []academic.LessonPeriod{{Number: 1}, {Number: 2}}))
	after, err := s.LessonPeriods(ctx, curriculum.ID, semester)
	must(err)
	if len(after) != 3 || after[0].StartsAt != "07:40:00" {
		t.Fatal("failed save or blanks modified data", after)
	}
	var afterHistory int
	must(db.QueryRow(`SELECT count(*) FROM app.lesson_period_history h JOIN app.lesson_periods p ON p.id=h.period_id WHERE p.semester_id=$1`, semester).Scan(&afterHistory))
	if afterHistory != histories+2 {
		t.Fatal("invalid/no-op batch wrote history", histories, afterHistory)
	}
	options, err := s.ScheduleAssignments(ctx, curriculum.ID, semester, date.AddDate(0, 0, -1))
	must(err)
	if len(options) != 0 {
		t.Fatal("future assignment offered")
	}
	options, err = s.ScheduleAssignments(ctx, curriculum.ID, semester, date)
	must(err)
	if len(options) != 2 {
		t.Fatal("eligible assignments missing", options)
	}
	must(s.AssignTeacher(ctx, curriculum.ID, semester, class.ID, subjectID, otherID, date.AddDate(0, 1, 0)))
	options, err = s.ScheduleAssignments(ctx, curriculum.ID, semester, date)
	must(err)
	for _, o := range options {
		if o.ClassID == class.ID && o.TeacherID != teacherID {
			t.Fatal("historical assignment missing")
		}
	}
	rev, err := s.CreateTimetableDraft(ctx, curriculum.ID, semester, date)
	must(err)
	must(s.AddTimetableEntry(ctx, curriculum.ID, rev.ID, 1, class.ID, subjectID, teacherID, 1, 2))
	if err = s.AddTimetableEntry(ctx, curriculum.ID, rev.ID, 1, classB.ID, subjectID, teacherID, 1, 1); err == nil {
		t.Fatal("teacher conflict accepted")
	}
	if err = s.AddTimetableEntry(ctx, curriculum.ID, rev.ID, 1, class.ID, subjectID, otherID, 1, 1); err == nil {
		t.Fatal("wrong assignment accepted")
	}
	if err = s.AddTimetableEntry(ctx, curriculum.ID, rev.ID, 2, class.ID, subjectID, teacherID, 1, 1); err == nil {
		t.Fatal("unconfigured day accepted")
	}
	must(s.PublishTimetable(ctx, curriculum.ID, rev.ID))
	must(s.SaveLessonDay(ctx, curriculum.ID, semester, 1, []academic.LessonPeriod{{Number: 1, StartsAt: "06:00", EndsAt: "06:40"}}))
	var publishedStart string
	must(db.QueryRow(`SELECT starts_at::text FROM app.weekly_timetable_periods WHERE revision_id=$1 AND weekday=1 AND period_no=1`, rev.ID).Scan(&publishedStart))
	if publishedStart != "07:40:00" {
		t.Fatal("published snapshot changed")
	}
	// Eligible teachers require an active account and Guru Mapel role.
	_, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, teacher.ID)
	must(err)
	options, err = s.ScheduleAssignments(ctx, curriculum.ID, semester, date)
	must(err)
	if len(options) != 0 {
		t.Fatal("inactive teacher offered")
	}
}
