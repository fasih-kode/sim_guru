//go:build integration

package academic

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestScheduleConstraintsLifecycleIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	cur, _ := testutil.User(t, db, "kurikulum")
	admin, _ := testutil.User(t, db, "admin")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	year, err := s.CreateYear(ctx, cur.ID, "Availability "+testutil.Random(t)[:10])
	must(err)
	var sem, teacherID, subject int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&sem))
	cl, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Preferensi',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel Preferensi') RETURNING id`, "AV"+testutil.Random(t)[:8]).Scan(&subject))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	must(s.AssignTeacher(ctx, cur.ID, sem, cl.ID, subject, teacherID, date))
	for d := 1; d <= 2; d++ {
		must(s.SaveLessonDay(ctx, cur.ID, sem, d, []LessonPeriod{{Number: 1, StartsAt: "07:00", EndsAt: "07:40"}, {Number: 2, StartsAt: "07:40", EndsAt: "08:20"}, {Number: 3, StartsAt: "08:20", EndsAt: "09:00"}}))
	}
	must(s.SaveCurriculumRule(ctx, cur.ID, CurriculumRule{SemesterID: sem, Grade: 7, SubjectID: subject, WeeklyJP: 4, Meetings: []int{1, 1, 1, 1}}))
	soft := ScheduleConstraint{SemesterID: sem, TeacherID: teacherID, Severity: "soft", Weekdays: []int{1, 2}, StartsAt: "07:00", EndsAt: "08:20"}
	if s.SaveScheduleConstraint(ctx, admin.ID, soft) != ErrForbidden {
		t.Fatal("admin changed restrictions")
	}
	must(s.SaveScheduleConstraint(ctx, cur.ID, soft))
	list, err := s.ScheduleConstraints(ctx, cur.ID, sem)
	must(err)
	if len(list) != 1 {
		t.Fatal(list)
	}
	soft = list[0]
	stale := soft
	soft.EndsAt = "08:20"
	must(s.SaveScheduleConstraint(ctx, cur.ID, soft))
	if s.SaveScheduleConstraint(ctx, cur.ID, stale) != ErrConflict {
		t.Fatal("stale update accepted")
	}
	rev, err := s.GenerateTimetable(ctx, cur.ID, sem, date, 0, false)
	must(err)
	report, err := s.RevisionPreferenceReport(ctx, cur.ID, rev.ID)
	must(err)
	if len(report) != 2 {
		t.Fatalf("soft report lost: %v", report)
	}
	snapshot := func() string {
		var value string
		must(db.QueryRow(`SELECT md5(jsonb_agg(to_jsonb(e) ORDER BY id)::text) FROM app.weekly_timetable_entries e WHERE revision_id=$1`, rev.ID).Scan(&value))
		return value
	}
	before := snapshot()
	hard := ScheduleConstraint{SemesterID: sem, TeacherID: teacherID, Severity: "hard", Weekdays: []int{2}, AllDay: true}
	must(s.SaveScheduleConstraint(ctx, cur.ID, hard))
	if s.PublishTimetable(ctx, cur.ID, rev.ID) == nil {
		t.Fatal("stale snapshot published")
	}
	if _, err = s.GenerateTimetable(ctx, cur.ID, sem, date, rev.ID, true); err == nil {
		t.Fatal("impossible hard rule accepted")
	}
	if snapshot() != before {
		t.Fatal("failed generation changed draft")
	}
	list, err = s.ScheduleConstraints(ctx, cur.ID, sem)
	must(err)
	for _, c := range list {
		if c.Severity == "hard" {
			must(s.RemoveScheduleConstraint(ctx, cur.ID, sem, c.ID, c.Version))
		}
	}
	hard.TeacherID = 0
	hard.SubjectID = subject
	hard.AllDay = false
	hard.StartsAt = "07:00"
	hard.EndsAt = "07:40"
	must(s.SaveScheduleConstraint(ctx, cur.ID, hard))
	_, err = s.GenerateTimetable(ctx, cur.ID, sem, date, rev.ID, true)
	must(err)
	_, _, entries, err := s.Timetable(ctx, cur.ID, sem)
	must(err)
	if len(entries) != 4 {
		t.Fatal("JP dropped")
	}
	e := entries[0]
	if s.CorrectTimetableEntry(ctx, cur.ID, rev.ID, e.ID, 2, cl.ID, subject, teacherID, 1, 1) == nil {
		t.Fatal("manual edit violated mandatory subject availability")
	}
	// Soft violations remain publishable and reportable using the revision snapshot.
	must(s.PublishTimetable(ctx, cur.ID, rev.ID))
	published := snapshot()
	list, err = s.ScheduleConstraints(ctx, cur.ID, sem)
	must(err)
	for _, c := range list {
		must(s.RemoveScheduleConstraint(ctx, cur.ID, sem, c.ID, c.Version))
	}
	mustExec := func(query string, id int64) { t.Helper(); _, e := db.Exec(query, id); must(e) }
	mustExec(`UPDATE app.teachers SET name='Renamed after publication' WHERE id=$1`, teacherID)
	mustExec(`UPDATE app.subjects SET name='Renamed subject' WHERE id=$1`, subject)
	report, err = s.RevisionPreferenceReport(ctx, cur.ID, rev.ID)
	must(err)
	if len(report) > 0 && (report[0].TeacherName != "Guru Preferensi" || report[0].SubjectName != "Mapel Preferensi") {
		t.Fatal("master correction rewrote historical report", report)
	}
	if len(report) != 2 || snapshot() != published {
		t.Fatal("config removal changed published report/data", fmt.Sprint(report))
	}
	if _, err = s.ScheduleConstraints(ctx, teacher.ID, sem); err != ErrForbidden {
		t.Fatal("teacher read school restrictions", err)
	}
	var histories int
	must(db.QueryRow(`SELECT count(*) FROM app.schedule_constraint_history WHERE semester_id=$1`, sem).Scan(&histories))
	if histories < 7 {
		t.Fatal("constraint history missing")
	}
}
