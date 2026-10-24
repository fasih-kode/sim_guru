package academic

import (
	"context"
	"strings"
	"testing"
)

func TestFlexibleScheduleSolver(t *testing.T) {
	p := SchedulePlan{Rules: []EffectiveCurriculum{
		{CurriculumRule: CurriculumRule{ID: 1, SemesterID: 1, Grade: 7, ClassID: 1, SubjectID: 1, WeeklyJP: 2, Meetings: []int{2}, DifferentDays: true}, TeacherID: 1, AssignmentID: 1},
		{CurriculumRule: CurriculumRule{ID: 2, SemesterID: 1, Grade: 7, ClassID: 2, SubjectID: 1, WeeklyJP: 5, Meetings: []int{3, 2}, DifferentDays: true}, TeacherID: 1, AssignmentID: 2},
	}, Periods: []LessonPeriod{
		{Weekday: 1, Number: 1, StartsAt: "06:30", EndsAt: "07:00"},
		{Weekday: 1, Number: 2, StartsAt: "07:00", EndsAt: "07:40"},
		{Weekday: 1, Number: 3, StartsAt: "08:00", EndsAt: "08:40"},
		{Weekday: 1, Number: 4, StartsAt: "08:40", EndsAt: "09:20"},
		{Weekday: 2, Number: 1, StartsAt: "07:00", EndsAt: "07:40"},
		{Weekday: 2, Number: 2, StartsAt: "07:40", EndsAt: "08:20"},
		{Weekday: 2, Number: 3, StartsAt: "08:20", EndsAt: "09:00"},
		{Weekday: 2, Number: 4, StartsAt: "09:00", EndsAt: "09:40"},
	}, Blocks: []SchoolTimeBlock{
		{Weekday: 1, Kind: "activity", Label: "Upacara", PeriodNo: 1, StartsAt: "06:30", EndsAt: "07:00"},
		{Weekday: 1, Kind: "break", Label: "Istirahat", StartsAt: "07:40", EndsAt: "08:00"},
	}}
	entries, err := SolveSchedule(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatal("expected 3 meetings")
	}
	totals := map[int64]int{}
	hasResumed := false
	for _, e := range entries {
		totals[e.ClassID] += e.PeriodEnd - e.PeriodStart + 1
		if e.Weekday == 1 && e.PeriodStart == 1 {
			t.Fatal("lesson occupied ceremony")
		}
		if e.PeriodEnd-e.PeriodStart+1 == 3 && e.Weekday == 1 {
			hasResumed = strings.Contains(entrySegments(e, p.Periods), "jeda")
		}
	}
	if totals[1] != 2 || totals[2] != 5 || !hasResumed {
		t.Fatalf("wrong allocation/resumption: %v %v", totals, hasResumed)
	}
	duplicate := append(append([]TimetableEntry(nil), entries...), entries[0])
	if ValidatePlannedEntries(p, duplicate, false) == nil {
		t.Fatal("conflict accepted")
	}
	p.Rules[0].WeeklyJP = 8
	p.Rules[0].Meetings = []int{4, 4}
	if _, err = SolveSchedule(context.Background(), p, nil); err == nil || !strings.Contains(err.Error(), "melebihi") {
		t.Fatal("capacity overflow not diagnosed", err)
	}
}
func TestScheduleSolverCancellationAndPattern(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := SchedulePlan{Rules: []EffectiveCurriculum{{CurriculumRule: CurriculumRule{ID: 1, SemesterID: 1, Grade: 7, ClassID: 1, SubjectID: 1, WeeklyJP: 1, Meetings: []int{1}}, TeacherID: 1, AssignmentID: 1}}, Periods: []LessonPeriod{{Weekday: 1, Number: 1, StartsAt: "07:00", EndsAt: "07:40"}}}
	if _, err := SolveSchedule(ctx, p, nil); err == nil || strings.Contains(err.Error(), "Tidak ada susunan") {
		t.Fatal("cancelled search misreported", err)
	}
	for _, bad := range []string{"3++2", "0+2", "x", "2+"} {
		if _, err := ParseMeetings(bad); err == nil {
			t.Fatal("invalid pattern accepted", bad)
		}
	}
	p.Rules[0].WeeklyJP = 8
	p.Rules[0].Meetings = []int{4, 4}
	if err := validateCurriculumRule(p.Rules[0].CurriculumRule); err != nil {
		t.Fatal("8 JP should be configurable", err)
	}
}
