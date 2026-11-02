package academic

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidateLessonAttendanceAtUsesWIBStartBoundary(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	date := "2026-10-23"
	start := time.Date(2026, 10, 23, 7, 40, 0, 0, wib)
	tests := []struct {
		name    string
		now     time.Time
		wantErr bool
	}{
		{name: "satu menit sebelum mulai", now: start.Add(-time.Minute), wantErr: true},
		{name: "tepat saat mulai", now: start},
		{name: "selama pelajaran", now: start.Add(20 * time.Minute)},
		{name: "setelah selesai", now: start.Add(90 * time.Minute)},
		{name: "hari berikutnya", now: start.AddDate(0, 0, 1).Add(time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLessonAttendanceAt(tt.now, date, "07:40:00")
			if tt.wantErr && err == nil {
				t.Fatal("expected time rejection")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
		})
	}
	if err := validateLessonAttendanceAt(start, date, "07:40:00"); err != nil {
		t.Fatalf("exact start must be accepted: %v", err)
	}
	if err := validateLessonAttendanceAt(start.Add(time.Minute), date, "07:40:00"); err != nil {
		t.Fatalf("during lesson must be accepted: %v", err)
	}
	if err := validateLessonAttendanceAt(start.AddDate(0, 0, 1).Add(time.Hour), date, "07:40:00"); err != nil {
		t.Fatalf("next-day make-up must be accepted: %v", err)
	}
	if err := validateLessonAttendanceAt(start, "2026-10-24", "07:40"); err == nil {
		t.Fatal("future meeting date must be rejected")
	}
}

func TestLessonAttendanceUsesFirstLearningSegmentAcrossBreak(t *testing.T) {
	lesson := TimetableEntry{Weekday: 1, PeriodStart: 2, PeriodEnd: 4}
	periods := []LessonPeriod{
		{Weekday: 1, Number: 2, StartsAt: "07:00:00", EndsAt: "07:40:00"},
		{Weekday: 1, Number: 3, StartsAt: "07:40:00", EndsAt: "08:20:00"},
		{Weekday: 1, Number: 4, StartsAt: "08:30:00", EndsAt: "09:10:00"},
	}
	if got := entrySegments(lesson, periods); got != "07:00–08:20 · jeda · 08:30–09:10" {
		t.Fatalf("unexpected learning segments: %q", got)
	}
	if err := validateLessonAttendanceAt(time.Date(2026, 10, 26, 7, 0, 0, 0, time.FixedZone("WIB", 7*60*60)), "2026-10-26", "07:00:00"); err != nil {
		t.Fatalf("first segment start must be accepted: %v", err)
	}
}

func TestLessonAttendanceSnapshotKeepsOnlyStudentAndMeetingEvidence(t *testing.T) {
	attendance := AttendanceContext{
		SemesterID: 4, RevisionID: 8, Date: "2026-10-23", SchoolDay: true,
	}
	calendar := SemesterCalendar{
		SemesterID: 4, Version: 3, StartsOn: "2026-07-01", EndsOn: "2026-12-31",
		SchoolDays: []int{1, 2, 3, 4, 5},
		Exceptions: []SchoolDateException{
			{Date: attendance.Date, SchoolDay: true, Label: "Uji hari khusus"},
			{Date: "2026-12-01", SchoolDay: false, Label: "EXCEPTION_OTHER_DATE_SENTINEL"},
		},
	}
	lesson := TimetableEntry{
		ID: 12, AssignmentID: 22, ClassID: 2, ClassName: "7A",
		SubjectID: 5, SubjectName: "Matematika", TeacherID: 3, TeacherName: "Guru Contoh",
		Weekday: 5, PeriodStart: 2, PeriodEnd: 4, StartsAt: "07:00:00", EndsAt: "09:10:00",
	}
	assignment := lessonAttendanceAssignmentSnapshot{
		ID: 22, SemesterID: 4, ClassID: 2, SubjectID: 5, TeacherID: 3,
		EffectiveFrom: "2026-07-01", EffectiveUntil: "",
	}
	periods := []LessonPeriod{
		{Weekday: 5, Number: 1, StartsAt: "06:20:00", EndsAt: "07:00:00"},
		{Weekday: 5, Number: 2, StartsAt: "07:00:00", EndsAt: "07:40:00"},
		{Weekday: 5, Number: 3, StartsAt: "07:40:00", EndsAt: "08:20:00"},
		{Weekday: 5, Number: 4, StartsAt: "08:30:00", EndsAt: "09:10:00"},
		{Weekday: 5, Number: 5, StartsAt: "09:10:00", EndsAt: "09:50:00"},
		{Weekday: 4, Number: 3, StartsAt: "10:00:00", EndsAt: "10:40:00"},
	}
	meeting, err := newLessonAttendanceMeetingSnapshot(attendance, calendar, lesson, 6, assignment, periods)
	if err != nil {
		t.Fatal(err)
	}
	if len(meeting.Periods) != 3 || meeting.Segments != "07:00–08:20 · jeda · 08:30–09:10" {
		t.Fatalf("meeting periods/segments do not match the lesson: %+v", meeting)
	}
	if meeting.Calendar.ExceptionLabel != "Uji hari khusus" || meeting.Calendar.Version != calendar.Version {
		t.Fatalf("date-specific calendar evidence missing: %+v", meeting.Calendar)
	}

	first := lessonAttendanceSnapshotForStudent(meeting, AttendanceStudent{
		ID: 101, PlacementID: 201, PlacementVersion: 4, StudentVersion: 7,
		StartsOn: "2026-07-13", Name: "Siswa A", NIS: "NIS-A", NISN: "NISN-A",
	})
	second := lessonAttendanceSnapshotForStudent(meeting, AttendanceStudent{
		ID: 102, PlacementID: 202, PlacementVersion: 5, StudentVersion: 8,
		StartsOn: "2026-07-13", Name: "Siswa B", NIS: "NIS-B", NISN: "NISN-B",
	})
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstJSON) > 1800 {
		t.Fatalf("per-student snapshot is unexpectedly large: %d bytes", len(firstJSON))
	}
	encoded := string(firstJSON)
	for _, forbidden := range []string{"plan_snapshot", "EXCEPTION_OTHER_DATE_SENTINEL", `"number":1`, `"number":5`, `"weekday":4`} {
		if strings.Contains(encoded, forbidden) {
			t.Errorf("snapshot contains unrelated context %q", forbidden)
		}
	}
	for _, required := range []string{"Siswa A", "NIS-A", "NISN-A", "Matematika", "Guru Contoh", "2026-10-23", "Uji hari khusus", "07:00–08:20 · jeda · 08:30–09:10"} {
		if !strings.Contains(encoded, required) {
			t.Errorf("snapshot is missing required evidence %q", required)
		}
	}
	var firstSnapshot, secondSnapshot lessonAttendanceSnapshot
	if err = json.Unmarshal(firstJSON, &firstSnapshot); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(secondJSON, &secondSnapshot); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstSnapshot.Meeting, secondSnapshot.Meeting) || firstSnapshot.Student.ID == secondSnapshot.Student.ID {
		t.Fatal("shared meeting context should be reused while student identity remains distinct")
	}
}
