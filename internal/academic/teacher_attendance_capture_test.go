package academic

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"testing"
)

type captureLocationRow struct{ err error }

func (r captureLocationRow) Scan(...any) error { return r.err }

func TestTeacherAttendanceCaptureRejectsMissingLocation(t *testing.T) {
	_, err := teacherAttendanceCaptureLocation(captureLocationRow{err: sql.ErrNoRows})
	if err == nil || err != ValidationError("Lokasi sekolah belum diatur oleh Admin.") {
		t.Fatalf("missing location error=%v", err)
	}

	databaseErr := errors.New("database unavailable")
	if _, err = teacherAttendanceCaptureLocation(captureLocationRow{err: databaseErr}); !errors.Is(err, databaseErr) {
		t.Fatalf("database error was not preserved: %v", err)
	}
}

func TestValidateTeacherAttendanceCapture(t *testing.T) {
	const key = "00112233445566778899aabbccddeeff"
	tests := []struct {
		name  string
		input TeacherAttendanceCaptureInput
		valid bool
	}{
		{"aksi masuk", TeacherAttendanceCaptureInput{IdempotencyKey: key, Action: TeacherCheckIn, Latitude: -6, Longitude: 106, AccuracyM: 50}, true},
		{"aksi pulang", TeacherAttendanceCaptureInput{IdempotencyKey: key, Action: TeacherCheckOut, Latitude: 90, Longitude: 180, AccuracyM: 0}, true},
		{"aksi lain", TeacherAttendanceCaptureInput{IdempotencyKey: key, Action: "other", Latitude: 0, Longitude: 0, AccuracyM: 1}, false},
		{"kunci pengiriman kosong", TeacherAttendanceCaptureInput{Action: TeacherCheckIn, Latitude: -6, Longitude: 106, AccuracyM: 1}, false},
		{"kunci pengiriman bukan hex", TeacherAttendanceCaptureInput{IdempotencyKey: "not-a-key", Action: TeacherCheckIn, Latitude: -6, Longitude: 106, AccuracyM: 1}, false},
		{"lintang di luar batas", TeacherAttendanceCaptureInput{IdempotencyKey: key, Action: TeacherCheckIn, Latitude: 90.01, Longitude: 0, AccuracyM: 1}, false},
		{"bujur di luar batas", TeacherAttendanceCaptureInput{IdempotencyKey: key, Action: TeacherCheckIn, Latitude: 0, Longitude: 180.01, AccuracyM: 1}, false},
		{"koordinat bukan angka", TeacherAttendanceCaptureInput{IdempotencyKey: key, Action: TeacherCheckIn, Latitude: math.NaN(), Longitude: 0, AccuracyM: 1}, false},
		{"akurasi lebih dari 50 meter", TeacherAttendanceCaptureInput{IdempotencyKey: key, Action: TeacherCheckIn, Latitude: 0, Longitude: 0, AccuracyM: 50.01}, false},
		{"akurasi bukan angka", TeacherAttendanceCaptureInput{IdempotencyKey: key, Action: TeacherCheckIn, Latitude: 0, Longitude: 0, AccuracyM: math.Inf(1)}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTeacherAttendanceCapture(test.input)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}

func TestNewTeacherAttendanceIdempotencyKey(t *testing.T) {
	first, err := NewTeacherAttendanceIdempotencyKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewTeacherAttendanceIdempotencyKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || len(second) != 32 || first == second {
		t.Fatalf("expected distinct 128-bit hexadecimal keys, first length=%d second length=%d equal=%v", len(first), len(second), first == second)
	}
	if _, err = hex.DecodeString(first); err != nil {
		t.Fatalf("first token is not hexadecimal: %v", err)
	}
}

func TestTeacherAttendanceEvidenceFingerprint(t *testing.T) {
	base := teacherAttendanceEvidence{
		ActorID: 10, TeacherID: 20, OnDate: "2026-10-28", Action: TeacherCheckIn,
		Source: "gps_selfie", DistanceM: 30, AccuracyM: 8, SchoolLatitude: -6.2,
		SchoolLongitude: 106.8, GeofenceRadiusM: 100, LocationVersion: 3,
		PhotoMIMEType: "image/jpeg", PhotoSizeBytes: 8000,
		PhotoChecksumSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	want, err := teacherAttendanceEvidenceFingerprint(base)
	if err != nil || want == "" {
		t.Fatalf("fingerprint=%q err=%v", want, err)
	}
	again, err := teacherAttendanceEvidenceFingerprint(base)
	if err != nil || again != want {
		t.Fatalf("fingerprint not deterministic: %q %q err=%v", want, again, err)
	}
	changed := base
	changed.PhotoChecksumSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	different, err := teacherAttendanceEvidenceFingerprint(changed)
	if err != nil || different == want {
		t.Fatalf("changed evidence kept fingerprint: %q %q err=%v", want, different, err)
	}
}
