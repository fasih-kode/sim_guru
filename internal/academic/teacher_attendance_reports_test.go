package academic

import (
	"strings"
	"testing"
)

func TestValidateTeacherAttendanceReport(t *testing.T) {
	base := TeacherAttendanceReportInput{Kind: "izin", StartsOn: "2026-10-28", EndsOn: "2026-10-28", Description: "Keperluan keluarga"}
	tests := []struct {
		name   string
		change func(*TeacherAttendanceReportInput)
		valid  bool
	}{
		{"single day", func(i *TeacherAttendanceReportInput) {}, true},
		{"future service report", func(i *TeacherAttendanceReportInput) {
			i.Kind = "dinas_luar"
			i.StartsOn = "2030-01-01"
			i.EndsOn = "2031-02-01"
		}, true},
		{"unlimited past range", func(i *TeacherAttendanceReportInput) {
			i.Kind = "sakit"
			i.StartsOn = "2000-01-01"
			i.EndsOn = "2001-12-31"
		}, true},
		{"normalization", func(i *TeacherAttendanceReportInput) {
			i.Description = "  Sakit\nperlu istirahat  "
			i.Kind = " sakit "
		}, true},
		{"invalid type", func(i *TeacherAttendanceReportInput) { i.Kind = "alpa" }, false},
		{"invalid day", func(i *TeacherAttendanceReportInput) { i.EndsOn = "2026-02-30" }, false},
		{"timestamp is not a calendar date", func(i *TeacherAttendanceReportInput) { i.StartsOn = "2026-10-28T00:00:00Z" }, false},
		{"reversed range", func(i *TeacherAttendanceReportInput) { i.StartsOn = "2026-10-29" }, false},
		{"empty description", func(i *TeacherAttendanceReportInput) { i.Description = " \n\t " }, false},
		{"length", func(i *TeacherAttendanceReportInput) { i.Description = strings.Repeat("a", 501) }, false},
		{"unicode limit", func(i *TeacherAttendanceReportInput) { i.Description = strings.Repeat("é", 500) }, true},
		{"nul", func(i *TeacherAttendanceReportInput) { i.Description = "a\x00b" }, false},
		{"invalid utf8", func(i *TeacherAttendanceReportInput) { i.Description = string([]byte{0xff}) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.change(&in)
			v, e := validateTeacherReport(in)
			if (e == nil) != tt.valid {
				t.Fatalf("error=%v valid=%v", e, tt.valid)
			}
			if e == nil && v.Description != strings.TrimSpace(in.Description) {
				t.Fatal("description not normalized")
			}
		})
	}
}

func TestTeacherReportCategoryLabels(t *testing.T) {
	for kind, want := range map[string]string{"izin": "Tidak Hadir (Izin)", "sakit": "Tidak Hadir (Sakit)", "dinas_luar": "Hadir (Dinas Luar)"} {
		if got := (TeacherAttendanceReport{Kind: kind}).CategoryLabel(); got != want {
			t.Fatalf("%s = %q", kind, got)
		}
	}
}
