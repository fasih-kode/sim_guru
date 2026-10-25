package academic

import (
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
