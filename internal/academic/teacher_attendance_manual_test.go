package academic

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestParseTeacherAttendanceManualTime(t *testing.T) {
	parsed, err := parseTeacherAttendanceManualTime("2026-10-28T09:30")
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Format(time.RFC3339); got != "2026-10-28T09:30:00+07:00" {
		t.Fatalf("parsed WIB time=%s", got)
	}
	if parsed.Location() != teacherAttendanceWIBLocation() {
		t.Fatalf("parsed location=%s, expected Asia/Jakarta", parsed.Location())
	}
	for _, value := range []string{"", "2026-10-28", "2026-10-28T09:30Z", "2026-10-28T09:30+07:00", "2026-02-30T09:30"} {
		if _, err := parseTeacherAttendanceManualTime(value); err == nil {
			t.Errorf("ambiguous or invalid time %q accepted", value)
		}
	}
}

func TestValidateTeacherAttendanceManualInput(t *testing.T) {
	const key = "00112233445566778899aabbccddeeff"
	valid := TeacherAttendanceManualInput{
		TeacherID: 7, Action: TeacherCheckIn, OccurredAt: "2026-10-28T09:30",
		Reason: "  Kamera guru bermasalah  ", IdempotencyKey: key,
	}
	validated, err := validateTeacherAttendanceManualInput(valid)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Reason != "Kamera guru bermasalah" || validated.OnDate != "2026-10-28" || len(validated.KeyHash) != 64 {
		t.Fatalf("unexpected validated input: %+v", validated)
	}
	if _, err = hex.DecodeString(validated.KeyHash); err != nil {
		t.Fatalf("key hash is not hexadecimal: %v", err)
	}
	tests := []struct {
		name  string
		input TeacherAttendanceManualInput
	}{
		{"guru kosong", TeacherAttendanceManualInput{Action: TeacherCheckIn, OccurredAt: valid.OccurredAt, Reason: "alasan", IdempotencyKey: key}},
		{"aksi tidak sah", TeacherAttendanceManualInput{TeacherID: 7, Action: "manual", OccurredAt: valid.OccurredAt, Reason: "alasan", IdempotencyKey: key}},
		{"alasan kosong", TeacherAttendanceManualInput{TeacherID: 7, Action: TeacherCheckIn, OccurredAt: valid.OccurredAt, IdempotencyKey: key}},
		{"alasan spasi", TeacherAttendanceManualInput{TeacherID: 7, Action: TeacherCheckIn, OccurredAt: valid.OccurredAt, Reason: " \t ", IdempotencyKey: key}},
		{"alasan lebih dari lima ratus karakter", TeacherAttendanceManualInput{TeacherID: 7, Action: TeacherCheckIn, OccurredAt: valid.OccurredAt, Reason: strings.Repeat("x", 501), IdempotencyKey: key}},
		{"kunci kosong", TeacherAttendanceManualInput{TeacherID: 7, Action: TeacherCheckIn, OccurredAt: valid.OccurredAt, Reason: "alasan"}},
		{"kunci bukan 128 bit", TeacherAttendanceManualInput{TeacherID: 7, Action: TeacherCheckIn, OccurredAt: valid.OccurredAt, Reason: "alasan", IdempotencyKey: "abcd"}},
		{"waktu ambigu", TeacherAttendanceManualInput{TeacherID: 7, Action: TeacherCheckIn, OccurredAt: "2026-10-28T09:30Z", Reason: "alasan", IdempotencyKey: key}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := validateTeacherAttendanceManualInput(test.input); err == nil {
				t.Fatal("invalid input was accepted")
			}
		})
	}
}

func TestValidateTeacherAttendanceManualWindow(t *testing.T) {
	zone := teacherAttendanceWIBLocation()
	now := time.Date(2026, 10, 28, 0, 10, 0, 0, zone)
	tests := []struct {
		name     string
		occurred time.Time
		valid    bool
	}{
		{"hari ini", time.Date(2026, 10, 28, 0, 0, 0, 0, zone), true},
		{"tepat tujuh hari sebelumnya", time.Date(2026, 10, 21, 23, 59, 0, 0, zone), true},
		{"delapan hari sebelumnya", time.Date(2026, 10, 20, 23, 59, 0, 0, zone), true},
		{"tiga puluh hari sebelumnya", time.Date(2026, 9, 28, 23, 59, 0, 0, zone), true},
		{"tepat 365 hari sebelumnya", time.Date(2025, 10, 28, 23, 59, 0, 0, zone), true},
		{"lebih dari setahun sebelumnya", time.Date(2025, 10, 27, 23, 59, 0, 0, zone), true},
		{"masa depan", time.Date(2026, 10, 28, 0, 11, 0, 0, zone), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTeacherAttendanceManualWindow(test.occurred, now)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}

	// Waktu ekuivalen UTC dinilai terhadap waktu server dan tetap tanpa batas usia.
	oldUTC := time.Date(2025, 10, 27, 16, 59, 0, 0, time.UTC)
	if err := validateTeacherAttendanceManualWindow(oldUTC, now); err != nil {
		t.Fatalf("old UTC timestamp should be allowed: %v", err)
	}
}

func TestTeacherAttendanceManualEvidenceFingerprint(t *testing.T) {
	base := teacherAttendanceManualEvidence{
		ActorID: 4, TeacherID: 8, TeacherName: "Guru Sintetis", OnDate: "2026-10-28",
		Action: TeacherCheckIn, Source: "manual", OccurredAt: "2026-10-28T09:30:00+07:00", Reason: "Kamera bermasalah",
	}
	one, err := teacherAttendanceManualEvidenceFingerprint(base)
	if err != nil || one == "" {
		t.Fatalf("fingerprint=%q err=%v", one, err)
	}
	again, err := teacherAttendanceManualEvidenceFingerprint(base)
	if err != nil || again != one {
		t.Fatalf("fingerprint is not stable: %q %q err=%v", one, again, err)
	}
	base.Reason = "Waktu berbeda"
	different, err := teacherAttendanceManualEvidenceFingerprint(base)
	if err != nil || different == one {
		t.Fatalf("changed evidence kept fingerprint: %q %q err=%v", one, different, err)
	}
}
