package academic

import (
	"strings"
	"testing"
)

func TestValidateLessonAttendanceCorrection(t *testing.T) {
	base := LessonAttendanceCorrectionInput{AttendanceID: 1, ExpectedVersion: 1, NewStatus: "hadir", Reason: "Koreksi yang diperiksa"}
	if _, _, err := validateLessonAttendanceCorrection(base); err != nil {
		t.Fatalf("valid correction: %v", err)
	}
	for name, mutate := range map[string]func(*LessonAttendanceCorrectionInput){
		"id kosong":              func(in *LessonAttendanceCorrectionInput) { in.AttendanceID = 0 },
		"version kosong":         func(in *LessonAttendanceCorrectionInput) { in.ExpectedVersion = 0 },
		"status invalid":         func(in *LessonAttendanceCorrectionInput) { in.NewStatus = "belum_dicatat" },
		"reason kosong":          func(in *LessonAttendanceCorrectionInput) { in.Reason = " \t" },
		"reason terlalu panjang": func(in *LessonAttendanceCorrectionInput) { in.Reason = strings.Repeat("a", 501) },
		"reason NUL":             func(in *LessonAttendanceCorrectionInput) { in.Reason = "alasan\x00tidak valid" },
	} {
		t.Run(name, func(t *testing.T) {
			in := base
			mutate(&in)
			if _, _, err := validateLessonAttendanceCorrection(in); err == nil {
				t.Fatal("invalid correction diterima")
			}
		})
	}
	for _, status := range []string{"hadir", "sakit", "izin", "alpa"} {
		t.Run(status, func(t *testing.T) {
			in := base
			in.NewStatus = status
			if _, _, err := validateLessonAttendanceCorrection(in); err != nil {
				t.Fatalf("status valid ditolak: %v", err)
			}
		})
	}
}
