//go:build integration

package academic

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestCurriculumGeneratorLifecycleIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := &Store{DB: db}
	curriculum, _ := testutil.User(t, db, "kurikulum")
	admin, _ := testutil.User(t, db, "admin")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	year, err := s.CreateYear(ctx, curriculum.ID, "Generator "+testutil.Random(t)[:10])
	must(err)
	var semester, nextSemester, teacherID, subject int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester))
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=2`, year.ID).Scan(&nextSemester))
	ca, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	cb, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "B", "", 0)
	must(err)
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Generator',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Matematika Generator') RETURNING id`, "GEN"+testutil.Random(t)[:8]).Scan(&subject))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []Class{ca, cb} {
		must(s.AssignTeacher(ctx, curriculum.ID, semester, c.ID, subject, teacherID, date))
	}
	periods := []LessonPeriod{{Number: 1, StartsAt: "06:30", EndsAt: "07:00"}, {Number: 2, StartsAt: "07:00", EndsAt: "07:40"}, {Number: 3, StartsAt: "08:00", EndsAt: "08:40"}, {Number: 4, StartsAt: "08:40", EndsAt: "09:20"}}
	must(s.SaveLessonDay(ctx, curriculum.ID, semester, 1, periods))
	must(s.SaveLessonDay(ctx, curriculum.ID, semester, 2, []LessonPeriod{{Number: 1, StartsAt: "07:00", EndsAt: "07:40"}, {Number: 2, StartsAt: "07:40", EndsAt: "08:20"}, {Number: 3, StartsAt: "08:20", EndsAt: "09:00"}, {Number: 4, StartsAt: "09:00", EndsAt: "09:40"}}))
	old, err := s.CreateTimetableDraft(ctx, curriculum.ID, semester, date)
	must(err)
	must(s.AddTimetableEntry(ctx, curriculum.ID, old.ID, 1, ca.ID, subject, teacherID, 2, 3))
	must(s.PublishTimetable(ctx, curriculum.ID, old.ID))
	fingerprint := func() string {
		t.Helper()
		var h string
		must(db.QueryRow(`SELECT md5(jsonb_build_object('revision',(SELECT to_jsonb(r) FROM app.weekly_timetable_revisions r WHERE id=$1),'entries',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM app.weekly_timetable_entries e WHERE revision_id=$1),'periods',(SELECT jsonb_agg(to_jsonb(p) ORDER BY weekday,period_no) FROM app.weekly_timetable_periods p WHERE revision_id=$1))::text)`, old.ID).Scan(&h))
		return h
	}
	original := fingerprint()
	rule := CurriculumRule{SemesterID: semester, Grade: 7, SubjectID: subject, WeeklyJP: 5, Meetings: []int{3, 2}, DifferentDays: true}
	if s.SaveCurriculumRule(ctx, admin.ID, rule) != ErrForbidden {
		t.Fatal("admin wrote curriculum")
	}
	must(s.SaveCurriculumRule(ctx, curriculum.ID, rule))
	rule.ClassID = ca.ID
	rule.WeeklyJP = 2
	rule.Meetings = []int{2}
	must(s.SaveCurriculumRule(ctx, curriculum.ID, rule))
	for _, b := range []SchoolTimeBlock{{SemesterID: semester, Weekday: 1, Kind: "activity", Label: "Upacara", PeriodNo: 1, StartsAt: "06:30", EndsAt: "07:00"}, {SemesterID: semester, Weekday: 1, Kind: "break", Label: "Istirahat", StartsAt: "07:40", EndsAt: "08:00"}} {
		must(s.SaveSchoolBlock(ctx, curriculum.ID, b))
	}
	if s.SaveSchoolBlock(ctx, curriculum.ID, SchoolTimeBlock{SemesterID: semester, Weekday: 1, Kind: "break", Label: "Memotong belajar", StartsAt: "07:10", EndsAt: "07:20"}) == nil {
		t.Fatal("break overlapping lesson accepted")
	}
	newDate := date.AddDate(0, 0, 1)
	rev, err := s.GenerateTimetable(ctx, curriculum.ID, semester, newDate, 0, false)
	must(err)
	if rev.ID == old.ID || rev.Status != "draft" {
		t.Fatal("published version reused")
	}
	got, _, entries, err := s.Timetable(ctx, curriculum.ID, semester)
	must(err)
	if got.ID != rev.ID {
		t.Fatal("new draft not selected")
	}
	totals := map[int64]int{}
	ceremony, rest, split := false, false, false
	var first TimetableEntry
	for _, e := range entries {
		switch e.Kind {
		case "activity":
			ceremony = e.PeriodStart == 1 && e.SubjectName == "Upacara"
		case "break":
			rest = true
		default:
			totals[e.ClassID] += e.PeriodEnd - e.PeriodStart + 1
			first = e
			if strings.Contains(e.TimeSegments, "jeda") {
				split = true
			}
		}
	}
	if totals[ca.ID] != 2 || totals[cb.ID] != 5 || !ceremony || !rest || !split {
		t.Fatalf("bad allocation or timeline: %v ceremony=%v rest=%v split=%v", totals, ceremony, rest, split)
	}
	if _, err = s.GenerateTimetable(ctx, curriculum.ID, semester, newDate, rev.ID, false); err == nil {
		t.Fatal("nonempty draft overwritten without consent")
	}
	must(s.CorrectTimetableEntry(ctx, curriculum.ID, rev.ID, first.ID, first.Weekday, first.ClassID, first.SubjectID, first.TeacherID, first.PeriodStart, first.PeriodEnd))
	var sameID int64
	must(db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE id=$1`, first.ID).Scan(&sameID))
	if sameID != first.ID {
		t.Fatal("edit replaced entry identity")
	}
	if s.CorrectTimetableEntry(ctx, curriculum.ID, rev.ID, first.ID, 1, first.ClassID, first.SubjectID, first.TeacherID, 1, 2) == nil {
		t.Fatal("edit occupied ceremony")
	}
	must(s.RemoveTimetableEntry(ctx, curriculum.ID, rev.ID, first.ID))
	if s.PublishTimetable(ctx, curriculum.ID, rev.ID) == nil {
		t.Fatal("incomplete JP published")
	}
	must(s.AddTimetableEntry(ctx, curriculum.ID, rev.ID, first.Weekday, first.ClassID, first.SubjectID, first.TeacherID, first.PeriodStart, first.PeriodEnd))
	// Impossible new requirements must leave the existing draft untouched.
	rules, err := s.CurriculumRules(ctx, curriculum.ID, semester)
	must(err)
	for _, r := range rules {
		if r.ClassID == ca.ID {
			rule = r
		}
	}
	rule.WeeklyJP = 8
	rule.Meetings = []int{4, 4}
	must(s.SaveCurriculumRule(ctx, curriculum.ID, rule))
	var before int
	must(db.QueryRow(`SELECT count(*) FROM app.weekly_timetable_entries WHERE revision_id=$1`, rev.ID).Scan(&before))
	if _, err = s.GenerateTimetable(ctx, curriculum.ID, semester, newDate, rev.ID, true); err == nil {
		t.Fatal("impossible teacher capacity accepted")
	}
	var after int
	must(db.QueryRow(`SELECT count(*) FROM app.weekly_timetable_entries WHERE revision_id=$1`, rev.ID).Scan(&after))
	if before != after {
		t.Fatal("failed generation changed draft")
	}
	if s.PublishTimetable(ctx, curriculum.ID, rev.ID) == nil {
		t.Fatal("stale draft published")
	}
	rules, err = s.CurriculumRules(ctx, curriculum.ID, semester)
	must(err)
	for _, r := range rules {
		if r.ClassID == ca.ID {
			rule = r
		}
	}
	rule.WeeklyJP = 2
	rule.Meetings = []int{2}
	must(s.SaveCurriculumRule(ctx, curriculum.ID, rule))
	_, err = s.GenerateTimetable(ctx, curriculum.ID, semester, newDate, rev.ID, true)
	must(err)
	must(s.PublishTimetable(ctx, curriculum.ID, rev.ID))
	if fingerprint() != original {
		t.Fatal("original published version changed")
	}
	oldView, _, oldEntries, err := s.TimetableAtRevision(ctx, curriculum.ID, semester, old.ID)
	must(err)
	if oldView.Status != "published" || len(oldEntries) != 1 {
		t.Fatal("historical view lost")
	}
	if _, _, _, err = s.TimetableAtRevision(ctx, admin.ID, semester, old.ID); err != ErrForbidden {
		t.Fatal("admin accessed schedule")
	}
	must(s.CopyCurriculum(ctx, curriculum.ID, semester, nextSemester))
	copied, err := s.CurriculumRules(ctx, curriculum.ID, nextSemester)
	must(err)
	if len(copied) != 2 {
		t.Fatal("default/override not copied", len(copied))
	}
	if fingerprint() != original {
		t.Fatal("copy changed publication")
	}
	// A later semester can change its default to 8 JP without code changes.
	for _, r := range copied {
		if r.ClassID == 0 {
			r.WeeklyJP = 8
			r.Meetings = []int{4, 4}
			must(s.SaveCurriculumRule(ctx, curriculum.ID, r))
		}
	}
	later, err := s.SchedulePlan(ctx, curriculum.ID, nextSemester, newDate)
	must(err)
	foundEight := false
	for _, r := range later.Rules {
		if r.ClassID == cb.ID && r.WeeklyJP == 8 {
			foundEight = true
		}
	}
	if !foundEight {
		t.Fatal("later semester default not flexible")
	}
	rules, err = s.CurriculumRules(ctx, curriculum.ID, semester)
	must(err)
	for _, r := range rules {
		if r.ClassID == ca.ID {
			must(s.ResetCurriculumOverride(ctx, curriculum.ID, semester, r.ID, r.Version))
		}
	}
	inherited, err := s.SchedulePlan(ctx, curriculum.ID, semester, newDate)
	must(err)
	for _, r := range inherited.Rules {
		if r.ClassID == ca.ID && (!r.Inherited || r.WeeklyJP != 5) {
			t.Fatal("reset did not restore default")
		}
	}
	if fingerprint() != original {
		t.Fatal("configuration edit changed historic publication")
	}

	var history int
	must(db.QueryRow(`SELECT count(*) FROM app.schedule_configuration_history WHERE semester_id=$1`, semester).Scan(&history))
	if history < 6 {
		t.Fatal(fmt.Sprintf("configuration history incomplete: %d", history))
	}
}
