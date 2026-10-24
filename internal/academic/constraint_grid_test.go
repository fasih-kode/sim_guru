package academic

import "testing"

func TestConstraintGridMappingAndLockedCells(t *testing.T) {
	p := preferenceTestPlan()
	p.Blocks = []SchoolTimeBlock{{Weekday: 1, Kind: "activity", Label: "Upacara", PeriodNo: 1, StartsAt: "07:00", EndsAt: "07:40"}}
	p.Constraints = append(p.Constraints, ScheduleConstraint{TeacherID: 1, Severity: "hard", Weekdays: []int{1}, StartsAt: "07:40", EndsAt: "08:20"})
	e := BuildConstraintEditor(p, 1, 0, "Guru A")
	if !e.Legacy || len(e.Columns) != 12 || e.Days[0].Cells[0].Blocked != "Upacara" || e.Days[0].Cells[1].State != "hard" || e.Days[1].Cells[0].State != "soft" {
		t.Fatal("legacy mapping/blocked cells incorrect", e)
	}
	states := map[string]string{}
	for _, d := range e.Days {
		for _, c := range d.Cells {
			if c.Blocked == "" {
				states[c.Field()] = c.State
			}
		}
	}
	if _, err := validateGridSubmission(e, states); err != nil {
		t.Fatal(err)
	}
	states["cell_1_1"] = "available"
	if _, err := validateGridSubmission(e, states); err == nil {
		t.Fatal("ceremony could be reopened")
	}
	delete(states, "cell_1_1")
	delete(states, "cell_2_1")
	if _, err := validateGridSubmission(e, states); err == nil {
		t.Fatal("missing cell accepted")
	}
	states["cell_2_1"] = "guess"
	if _, err := validateGridSubmission(e, states); err == nil {
		t.Fatal("invalid state accepted")
	}
}
func TestSavedGridOverridesOnlyOwnTargetAndRequiresClockReview(t *testing.T) {
	p := preferenceTestPlan()
	p.ConstraintGrids = []ConstraintGrid{{ID: 1, TeacherID: 1, TargetName: "Guru A", Periods: p.Periods, Blocks: p.Blocks, Slots: []ConstraintSlot{{Weekday: 1, Number: 1, State: "available"}, {Weekday: 1, Number: 2, State: "soft"}}}}
	subject := ScheduleConstraint{SubjectID: 1, Severity: "hard", Weekdays: []int{1}, StartsAt: "07:00", EndsAt: "07:40"}
	p.Constraints = effectiveGridConstraints(append(p.Constraints, subject), p.ConstraintGrids)
	if !hardUnavailable(p, 1, 1, p.Periods[0]) {
		t.Fatal("teacher available overrode subject ban")
	}
	for _, c := range p.Constraints {
		if c.TeacherID == 1 && c.PeriodNo != 2 {
			t.Fatal("legacy preference survived own grid override")
		}
	}
	editor := BuildConstraintEditor(p, 1, 0, "Guru A")
	if editor.Legacy || editor.Review {
		t.Fatal("saved grid wrongly needs legacy review")
	}
	token := editor.Token
	p.Periods = append([]LessonPeriod(nil), p.Periods...)
	p.Periods[0].StartsAt = "07:01"
	editor = BuildConstraintEditor(p, 1, 0, "Guru A")
	if !editor.Review || editor.Token == token {
		t.Fatal("clock changes not detected")
	}
	if planReady(p) == nil {
		t.Fatal("stale grid accepted by shared validator")
	}
}
