package academic

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestValidateTeacherLocationAndAccuracy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lat, lon float64
		radius   int
		valid    bool
	}{
		{"valid default", -6.2, 106.8, 100, true},
		{"radius lower outside", 0, 0, 24, false},
		{"radius lower", 0, 0, 25, true},
		{"radius upper", 0, 0, 500, true},
		{"radius upper outside", 0, 0, 501, false},
		{"latitude outside", 90.01, 0, 100, false},
		{"longitude outside", 0, -180.01, 100, false},
		{"latitude nan", math.NaN(), 0, 100, false},
		{"longitude infinity", 0, math.Inf(1), 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateTeacherLocation(tc.lat, tc.lon, tc.radius); (err == nil) != tc.valid {
				t.Fatalf("validateTeacherLocation() error=%v, valid=%v", err, tc.valid)
			}
		})
	}
	if err := ValidateTeacherLocationAccuracy(50); err != nil {
		t.Fatalf("50m accuracy must pass: %v", err)
	}
	for _, bad := range []float64{50.001, -1, math.NaN(), math.Inf(1)} {
		if err := ValidateTeacherLocationAccuracy(bad); err == nil {
			t.Fatalf("accuracy %v must fail", bad)
		}
	}
}

func TestTeacherAttendanceDistanceRadiusBoundaries(t *testing.T) {
	// Pada khatulistiwa, perubahan longitude ini membentuk jarak sekitar 100 m.
	const lon100m = 100.0 / 111195.0802335
	inside, err := TeacherAttendanceDistanceM(0, 0, 0, lon100m*0.99)
	if err != nil || inside >= 100 {
		t.Fatalf("inside distance=%v err=%v", inside, err)
	}
	exact, err := TeacherAttendanceDistanceM(0, 0, 0, lon100m)
	if err != nil || math.Abs(exact-100) > 0.01 {
		t.Fatalf("boundary distance=%v err=%v", exact, err)
	}
	outside, err := TeacherAttendanceDistanceM(0, 0, 0, lon100m*1.01)
	if err != nil || outside <= 100 {
		t.Fatalf("outside distance=%v err=%v", outside, err)
	}
	for _, tc := range []struct {
		distance float64
		allowed  bool
	}{{99, true}, {100, true}, {100.01, false}} {
		got, checkErr := TeacherAttendanceWithinRadius(tc.distance, 100)
		if checkErr != nil || got != tc.allowed {
			t.Fatalf("distance=%v allowed=%v err=%v, want %v", tc.distance, got, checkErr, tc.allowed)
		}
	}
}

func TestTeacherAttendanceWIBDateAndMakeupWindow(t *testing.T) {
	before := time.Date(2026, 10, 26, 16, 59, 0, 0, time.UTC)
	after := before.Add(time.Minute)
	if TeacherAttendanceWIBDate(before) != "2026-10-26" || TeacherAttendanceWIBDate(after) != "2026-10-27" {
		t.Fatalf("unexpected WIB dates: %s, %s", TeacherAttendanceWIBDate(before), TeacherAttendanceWIBDate(after))
	}
	today := time.Date(2026, 10, 27, 9, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	for _, tc := range []struct {
		days    int
		allowed bool
	}{{0, true}, {7, true}, {8, false}, {-1, false}} {
		date := today.AddDate(0, 0, -tc.days)
		if got := TeacherAttendanceMakeupAllowed(date, today); got != tc.allowed {
			t.Fatalf("days=%d allowed=%v, want %v", tc.days, got, tc.allowed)
		}
	}
}

func TestTeacherAttendancePhotoRetentionTwoCalendarMonths(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	got := TeacherAttendancePhotoRetainUntil(time.Date(2026, time.December, 31, 12, 30, 0, 0, wib))
	want := time.Date(2027, time.February, 28, 12, 30, 0, 0, wib)
	if !got.Equal(want) {
		t.Fatalf("retention=%v, want %v", got, want)
	}
}

func TestValidateTeacherLocationReason(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{{"", false}, {" \t", false}, {"Alasan", true}, {strings.Repeat("x", 501), false}, {"\x00", false}} {
		if err := validateLocationReason(tc.value); (err == nil) != tc.valid {
			t.Fatalf("reason length=%d err=%v", len(tc.value), err)
		}
	}
}
