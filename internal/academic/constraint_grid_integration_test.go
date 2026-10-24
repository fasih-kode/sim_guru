//go:build integration

package academic

import (
	"context"
	"github.com/fasih-kode/sim_guru/internal/testutil"
	"testing"
	"time"
)

func TestConstraintGridLifecycleIntegration(t *testing.T) {
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
	y, err := s.CreateYear(ctx, cur.ID, "Grid "+testutil.Random(t)[:10])
	must(err)
	var sem, tid, sub int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, y.ID).Scan(&sem))
	cl, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	must(err)
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Grid Teacher',$1) RETURNING id`, teacher.ID).Scan(&tid))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Grid Subject') RETURNING id`, "GRID"+testutil.Random(t)[:8]).Scan(&sub))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	must(s.AssignTeacher(ctx, cur.ID, sem, cl.ID, sub, tid, date))
	periods := []LessonPeriod{{Number: 1, StartsAt: "07:00", EndsAt: "07:40"}, {Number: 2, StartsAt: "07:40", EndsAt: "08:20"}, {Number: 3, StartsAt: "08:20", EndsAt: "09:00"}}
	for day := 1; day <= 2; day++ {
		must(s.SaveLessonDay(ctx, cur.ID, sem, day, periods))
	}
	must(s.SaveSchoolBlock(ctx, cur.ID, SchoolTimeBlock{SemesterID: sem, Weekday: 1, Kind: "activity", Label: "Upacara", PeriodNo: 1, StartsAt: "07:00", EndsAt: "07:40"}))
	must(s.SaveCurriculumRule(ctx, cur.ID, CurriculumRule{SemesterID: sem, Grade: 7, SubjectID: sub, WeeklyJP: 2, Meetings: []int{1, 1}}))
	legacy := ScheduleConstraint{SemesterID: sem, TeacherID: tid, Severity: "soft", Weekdays: []int{1}, StartsAt: "07:40", EndsAt: "08:20"}
	must(s.SaveScheduleConstraint(ctx, cur.ID, legacy))
	plan := func() SchedulePlan { t.Helper(); p, e := s.SchedulePlan(ctx, cur.ID, sem, date); must(e); return p }
	makeStates := func(e ConstraintEditor, state string) map[string]string {
		out := map[string]string{}
		for _, d := range e.Days {
			for _, c := range d.Cells {
				if c.Blocked == "" {
					out[c.Field()] = state
				}
			}
		}
		return out
	}
	editor := BuildConstraintEditor(plan(), tid, 0, "Grid Teacher")
	states := makeStates(editor, "hard")
	states["cell_2_1"] = "available"
	states["cell_2_2"] = "soft"
	if s.SaveConstraintGrid(ctx, admin.ID, sem, tid, 0, editor.Token, states, true) != ErrForbidden {
		t.Fatal("admin wrote grid")
	}
	if s.SaveConstraintGrid(ctx, cur.ID, sem, tid, 0, editor.Token, states, false) == nil {
		t.Fatal("legacy conversion not reviewed")
	}
	states["cell_1_1"] = "available"
	if s.SaveConstraintGrid(ctx, cur.ID, sem, tid, 0, editor.Token, states, true) == nil {
		t.Fatal("activity cell accepted")
	}
	delete(states, "cell_1_1")
	must(s.SaveConstraintGrid(ctx, cur.ID, sem, tid, 0, editor.Token, states, true))
	if s.SaveConstraintGrid(ctx, cur.ID, sem, tid, 0, editor.Token, states, true) != ErrConflict {
		t.Fatal("stale tab overwrote grid")
	}
	var count int
	must(db.QueryRow(`SELECT count(*) FROM app.schedule_constraints WHERE semester_id=$1`, sem).Scan(&count))
	if count != 1 {
		t.Fatal("legacy data removed")
	}
	if s.SaveScheduleConstraint(ctx, cur.ID, legacy) == nil {
		t.Fatal("legacy write bypassed active grid")
	}
	rev, err := s.GenerateTimetable(ctx, cur.ID, sem, date, 0, false)
	must(err)
	_, _, entries, err := s.Timetable(ctx, cur.ID, sem)
	must(err)
	for _, e := range entries {
		if e.Kind == "" && e.Weekday != 2 {
			t.Fatal("hard slot used")
		}
	}
	var first TimetableEntry
	for _, e := range entries {
		if e.Kind == "" {
			first = e
			break
		}
	}
	if s.CorrectTimetableEntry(ctx, cur.ID, rev.ID, first.ID, 1, cl.ID, sub, tid, 2, 2) == nil {
		t.Fatal("edit ignored hard grid cell")
	}
	must(s.PublishTimetable(ctx, cur.ID, rev.ID))
	report, err := s.RevisionPreferenceReport(ctx, cur.ID, rev.ID)
	must(err)
	if len(report) != 1 || report[0].Weekday != 2 {
		t.Fatal("conditional JP not reported", report)
	}
	var original string
	must(db.QueryRow(`SELECT md5(plan_snapshot::text) FROM app.weekly_timetable_revisions WHERE id=$1`, rev.ID).Scan(&original))
	subjectEditor := BuildConstraintEditor(plan(), 0, sub, "Grid Subject")
	subjectStates := makeStates(subjectEditor, "available")
	subjectStates["cell_2_1"] = "hard"
	must(s.SaveConstraintGrid(ctx, cur.ID, sem, 0, sub, subjectEditor.Token, subjectStates, false))
	if _, err = s.GenerateTimetable(ctx, cur.ID, sem, date.AddDate(0, 0, 1), 0, false); err == nil {
		t.Fatal("teacher available overrode subject unavailable")
	}
	subjectEditor = BuildConstraintEditor(plan(), 0, sub, "Grid Subject")
	must(s.SaveConstraintGrid(ctx, cur.ID, sem, 0, sub, subjectEditor.Token, makeStates(subjectEditor, "available"), false))
	editor = BuildConstraintEditor(plan(), tid, 0, "Grid Teacher")
	must(s.SaveConstraintGrid(ctx, cur.ID, sem, tid, 0, editor.Token, makeStates(editor, "available"), false))
	for _, c := range plan().Constraints {
		if c.TeacherID == tid {
			t.Fatal("reset all available resurrected legacy preference")
		}
	}
	draft, err := s.GenerateTimetable(ctx, cur.ID, sem, date.AddDate(0, 0, 1), 0, false)
	must(err)
	shifted := []LessonPeriod{{Number: 1, StartsAt: "07:05", EndsAt: "07:45"}, {Number: 2, StartsAt: "07:45", EndsAt: "08:25"}, {Number: 3, StartsAt: "08:25", EndsAt: "09:05"}}
	must(s.SaveLessonDay(ctx, cur.ID, sem, 2, shifted))
	if _, err = s.GenerateTimetable(ctx, cur.ID, sem, draft.EffectiveFrom, draft.ID, true); err == nil {
		t.Fatal("clock changes silently applied to grid")
	}
	if s.PublishTimetable(ctx, cur.ID, draft.ID) == nil {
		t.Fatal("stale clock grid published")
	}
	for _, pair := range [][2]int64{{tid, 0}, {0, sub}} {
		editor = BuildConstraintEditor(plan(), pair[0], pair[1], "Grid target")
		if !editor.Review {
			t.Fatal("missing review warning")
		}
		if s.SaveConstraintGrid(ctx, cur.ID, sem, pair[0], pair[1], editor.Token, makeStates(editor, "available"), false) == nil {
			t.Fatal("unreviewed clock accepted")
		}
		must(s.SaveConstraintGrid(ctx, cur.ID, sem, pair[0], pair[1], editor.Token, makeStates(editor, "available"), true))
	}
	_, err = s.GenerateTimetable(ctx, cur.ID, sem, draft.EffectiveFrom, draft.ID, true)
	must(err)
	var after string
	must(db.QueryRow(`SELECT md5(plan_snapshot::text) FROM app.weekly_timetable_revisions WHERE id=$1`, rev.ID).Scan(&after))
	if after != original {
		t.Fatal("published snapshot changed")
	}
	report, err = s.RevisionPreferenceReport(ctx, cur.ID, rev.ID)
	must(err)
	if len(report) != 1 || report[0].StartsAt != "07:40" {
		t.Fatal("published report changed with school clock", report)
	}
	currentPlan := plan()
	var deletable ConstraintGrid
	for _, grid := range currentPlan.ConstraintGrids {
		if grid.TeacherID == tid {
			deletable = grid
			break
		}
	}
	if deletable.ID == 0 {
		t.Fatal("saved subject grid missing before delete")
	}
	if s.RemoveConstraintGrid(ctx, admin.ID, sem, deletable.ID, deletable.Version) != ErrForbidden {
		t.Fatal("admin deleted grid")
	}
	must(s.RemoveConstraintGrid(ctx, cur.ID, sem, deletable.ID, deletable.Version))
	if s.RemoveConstraintGrid(ctx, cur.ID, sem, deletable.ID, deletable.Version) != ErrNotFound {
		t.Fatal("deleted grid remained active")
	}
	currentPlan = plan()
	for _, grid := range currentPlan.ConstraintGrids {
		if grid.ID == deletable.ID {
			t.Fatal("deleted grid still returned as active")
		}
	}
	for _, rule := range currentPlan.Constraints {
		if rule.TeacherID == tid {
			t.Fatal("deleted grid's replaced legacy constraints became active again")
		}
	}
	var deletedAt bool
	must(db.QueryRow(`SELECT deleted_at IS NOT NULL FROM app.schedule_constraint_grids WHERE id=$1`, deletable.ID).Scan(&deletedAt))
	if !deletedAt {
		t.Fatal("grid deletion was not retained as an audit record")
	}
	newEditor := BuildConstraintEditor(currentPlan, tid, 0, "Grid Teacher")
	must(s.SaveConstraintGrid(ctx, cur.ID, sem, tid, 0, newEditor.Token, makeStates(newEditor, "available"), false))
	var recreatedID int64
	must(db.QueryRow(`SELECT id FROM app.schedule_constraint_grids WHERE semester_id=$1 AND teacher_id=$2 AND deleted_at IS NULL`, sem, tid).Scan(&recreatedID))
	if recreatedID == deletable.ID {
		t.Fatal("re-creating a deleted grid reused the archived row")
	}
	must(db.QueryRow(`SELECT count(*) FROM app.schedule_constraint_grid_history h JOIN app.schedule_constraint_grids g ON g.id=h.grid_id WHERE g.semester_id=$1`, sem).Scan(&count))
	if count < 6 {
		t.Fatal("grid history incomplete", count)
	}
}
