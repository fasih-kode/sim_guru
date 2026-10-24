package academic

import (
	"context"
	"strings"
	"testing"
)

func preferenceTestPlan() SchedulePlan {
	p := SchedulePlan{Rules: []EffectiveCurriculum{{CurriculumRule: CurriculumRule{ID: 1, SemesterID: 1, Grade: 7, ClassID: 1, SubjectID: 1, WeeklyJP: 4, Meetings: []int{1, 1, 1, 1}}, TeacherID: 1, AssignmentID: 1, TeacherName: "Guru A"}}}
	for day := 1; day <= 2; day++ {
		for i, clock := range [][2]string{{"07:00", "07:40"}, {"07:40", "08:20"}, {"08:20", "09:00"}} {
			p.Periods = append(p.Periods, LessonPeriod{Weekday: day, Number: i + 1, StartsAt: clock[0], EndsAt: clock[1]})
		}
	}
	p.Constraints = []ScheduleConstraint{{ID: 1, SemesterID: 1, TeacherID: 1, Severity: "soft", TargetName: "Guru A", Weekdays: []int{1, 2}, StartsAt: "07:00", EndsAt: "08:20"}}
	return p
}
func TestPreferencesMinimizeTeacherDaysWithoutDroppingJP(t *testing.T) {
	p := preferenceTestPlan()
	preferred := []TimetableEntry{{ClassID: 1, SubjectID: 1, Weekday: 1, PeriodStart: 1, PeriodEnd: 1}, {ClassID: 1, SubjectID: 1, Weekday: 2, PeriodStart: 1, PeriodEnd: 1}}
	got, err := SolveSchedule(context.Background(), p, preferred)
	if err != nil {
		t.Fatal(err)
	}
	capacity := TeacherCapacities(p)
	if len(capacity) != 1 || capacity[0].Load != 4 || capacity[0].Available != 6 {
		t.Fatal("soft preference removed capacity", capacity)
	}
	report := PreferenceViolations(p, got)
	days := map[int]bool{}
	jp := 0
	for _, v := range report {
		days[v.Weekday] = true
		jp += v.JP
	}
	summary := SummarizeTeacherPreferences(report)
	if len(summary) != 1 || summary[0].Days != 1 || summary[0].JP != 2 {
		t.Fatal("teacher day summary missing", summary)
	}
	if len(got) != 4 || len(days) != 1 || jp != 2 {
		t.Fatalf("expected complete 4 JP, early on only one day/2 JP; got %d, %v, %d", len(got), days, jp)
	}
	// Overlapping preferences must not double count the same teaching JP.
	p.Constraints = append(p.Constraints, p.Constraints[0])
	if len(PreferenceViolations(p, got)) != 2 {
		t.Fatal("duplicate preferences double counted JP")
	}
	p.Constraints[0].Severity = "hard"
	if _, err = SolveSchedule(context.Background(), p, nil); err == nil || !strings.Contains(err.Error(), "hanya 2 JP") {
		t.Fatal("mandatory teacher capacity not enforced", err)
	}
	if ValidatePlannedEntries(p, got, true) == nil {
		t.Fatal("shared validator accepted mandatory violation")
	}
}
func TestActualTimeConstraintScopeAndBreak(t *testing.T) {
	c := ScheduleConstraint{TeacherID: 1, Weekdays: []int{2}, StartsAt: "12:00", EndsAt: "13:00", Severity: "hard"}
	if c.matches(2, 1, LessonPeriod{Weekday: 2, Number: 7, StartsAt: "12:25", EndsAt: "13:05"}) {
		t.Fatal("teacher restriction leaked to other teacher")
	}
	if c.matches(1, 1, LessonPeriod{Weekday: 1, Number: 7, StartsAt: "12:25", EndsAt: "13:05"}) {
		t.Fatal("restriction applied on unselected day")
	}
	c.TeacherID = 0
	c.SubjectID = 1
	if c.matches(2, 1, LessonPeriod{Weekday: 2, Number: 7, StartsAt: "10:50", EndsAt: "11:30"}) {
		t.Fatal("period number mistaken for afternoon")
	}
	if !c.matches(2, 1, LessonPeriod{Weekday: 2, Number: 7, StartsAt: "12:25", EndsAt: "13:05"}) {
		t.Fatal("actual afternoon overlap missed")
	}
	if c.matches(2, 2, LessonPeriod{Weekday: 2, StartsAt: "12:25", EndsAt: "13:05"}) {
		t.Fatal("subject restriction leaked")
	}
	p := preferenceTestPlan()
	p.Rules[0].WeeklyJP = 2
	p.Rules[0].Meetings = []int{2}
	p.Constraints = []ScheduleConstraint{{SubjectID: 1, Severity: "hard", AllDay: true, Weekdays: []int{1}}}
	got, err := SolveSchedule(context.Background(), p, nil)
	if err != nil || len(got) != 1 || got[0].Weekday != 2 {
		t.Fatal("full-day subject ban ignored", got, err)
	}
	// An interval entirely inside a break does not ban the teaching on either side.
	p.Periods = []LessonPeriod{{Weekday: 2, Number: 1, StartsAt: "07:00", EndsAt: "07:40"}, {Weekday: 2, Number: 2, StartsAt: "08:00", EndsAt: "08:40"}}
	p.Blocks = []SchoolTimeBlock{{Kind: "break", Weekday: 2, StartsAt: "07:40", EndsAt: "08:00"}}
	p.Constraints = []ScheduleConstraint{{TeacherID: 1, Severity: "hard", Weekdays: []int{2}, StartsAt: "07:40", EndsAt: "08:00"}}
	if _, err = SolveSchedule(context.Background(), p, nil); err != nil {
		t.Fatal("break-only constraint blocked resumed meeting", err)
	}
	p.Constraints[0].EndsAt = "08:01"
	if _, err = SolveSchedule(context.Background(), p, nil); err == nil {
		t.Fatal("meeting crossed mandatory unavailable teaching period")
	}
}
func TestScheduleConstraintValidation(t *testing.T) {
	good := ScheduleConstraint{SemesterID: 1, TeacherID: 1, Severity: "soft", Weekdays: []int{1}, StartsAt: "07:00", EndsAt: "08:20"}
	if err := validateScheduleConstraint(good); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ScheduleConstraint){func(c *ScheduleConstraint) { c.SubjectID = 2 }, func(c *ScheduleConstraint) { c.Weekdays = []int{1, 1} }, func(c *ScheduleConstraint) { c.Weekdays = []int{8} }, func(c *ScheduleConstraint) { c.StartsAt = "25:00" }, func(c *ScheduleConstraint) { c.EndsAt = "06:00" }, func(c *ScheduleConstraint) { c.Severity = "guess" }} {
		c := good
		change(&c)
		if validateScheduleConstraint(c) == nil {
			t.Fatal("invalid constraint accepted", c)
		}
	}
}

func TestSoftOptimizationImprovesCompletePublishedIncumbent(t *testing.T) {
	p := preferenceTestPlan()
	var previous []TimetableEntry
	for day := 1; day <= 2; day++ {
		for period := 1; period <= 2; period++ {
			previous = append(previous, TimetableEntry{ClassID: 1, SubjectID: 1, TeacherID: 1, AssignmentID: 1, Weekday: day, PeriodStart: period, PeriodEnd: period})
		}
	}
	if err := ValidatePlannedEntries(p, previous, true); err != nil {
		t.Fatal(err)
	}
	got, err := SolveSchedule(context.Background(), p, previous)
	if err != nil {
		t.Fatal(err)
	}
	summary := SummarizeTeacherPreferences(PreferenceViolations(p, got))
	if len(summary) != 1 || summary[0].Days != 1 || summary[0].JP != 2 {
		t.Fatal("valid published incumbent was not improved", summary)
	}
	// Subject availability also reduces the teacher capacity when all assignments
	// use that subject; preferences never reduce the same capacity.
	p.Constraints = []ScheduleConstraint{{SubjectID: 1, Severity: "hard", AllDay: true, Weekdays: []int{1}}}
	capacities := TeacherCapacities(p)
	if len(capacities) != 1 || capacities[0].Available != 3 {
		t.Fatal("subject ban missing from teacher capacity", capacities)
	}
}
