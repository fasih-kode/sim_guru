package academic

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestValidateTeacherAttendanceCorrectionInput(t *testing.T) {
	valid := TeacherAttendanceCorrectionInput{
		EventID: 1, ExpectedVersion: 1, OccurredAt: "2026-10-28T09:30",
		CorrectionReason: "Waktu dikoreksi berdasarkan buku piket",
	}
	tests := []struct {
		name    string
		change  func(*TeacherAttendanceCorrectionInput)
		wantErr bool
	}{
		{"valid", func(*TeacherAttendanceCorrectionInput) {}, false},
		{"event id invalid", func(in *TeacherAttendanceCorrectionInput) { in.EventID = 0 }, true},
		{"version invalid", func(in *TeacherAttendanceCorrectionInput) { in.ExpectedVersion = 0 }, true},
		{"time invalid", func(in *TeacherAttendanceCorrectionInput) { in.OccurredAt = "2026-10-28T09:30Z" }, true},
		{"reason empty", func(in *TeacherAttendanceCorrectionInput) { in.CorrectionReason = " \t" }, true},
		{"reason too long", func(in *TeacherAttendanceCorrectionInput) { in.CorrectionReason = strings.Repeat("x", 501) }, true},
		{"manual reason valid", func(in *TeacherAttendanceCorrectionInput) {
			reason := "Alat presensi bermasalah"
			in.ManualReason = &reason
		}, false},
		{"manual reason too long", func(in *TeacherAttendanceCorrectionInput) {
			reason := strings.Repeat("x", 501)
			in.ManualReason = &reason
		}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.change(&input)
			_, err := validateTeacherAttendanceCorrectionInput(input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validation error=%v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestValidateTeacherAttendanceCorrectionTime(t *testing.T) {
	zone := teacherAttendanceWIBLocation()
	serverNow := time.Date(2026, 10, 28, 10, 0, 0, 0, zone)
	tests := []struct {
		name     string
		occurred time.Time
		onDate   string
		wantErr  bool
	}{
		{"same WIB date", time.Date(2026, 10, 28, 9, 0, 0, 0, zone), "2026-10-28", false},
		{"exactly midnight WIB", time.Date(2026, 10, 28, 0, 0, 0, 0, zone), "2026-10-28", false},
		{"future", serverNow.Add(time.Minute), "2026-10-28", true},
		{"different WIB date", time.Date(2026, 10, 27, 23, 59, 0, 0, zone), "2026-10-28", true},
		{"same instant represented in UTC", time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC), "2026-10-28", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTeacherAttendanceCorrectionTime(test.occurred, test.onDate, serverNow)
			if (err != nil) != test.wantErr {
				t.Fatalf("validation error=%v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestTeacherAttendanceCorrectionManualReason(t *testing.T) {
	old := "Catatan awal"
	manual := sql.NullString{String: old, Valid: true}
	unchanged, err := teacherAttendanceCorrectionManualReason("manual", manual, nil)
	if err != nil || unchanged != old {
		t.Fatalf("unchanged manual reason=%q err=%v", unchanged, err)
	}
	newReason := "Alasan baru"
	changed, err := teacherAttendanceCorrectionManualReason("manual", manual, &newReason)
	if err != nil || changed != newReason {
		t.Fatalf("changed manual reason=%q err=%v", changed, err)
	}
	if _, err = teacherAttendanceCorrectionManualReason("gps_selfie", sql.NullString{}, &newReason); err == nil {
		t.Fatal("GPS event accepted a manual reason")
	}
	if _, err = teacherAttendanceCorrectionManualReason("manual", sql.NullString{}, nil); err == nil {
		t.Fatal("manual event without its required original reason was accepted")
	}
}
