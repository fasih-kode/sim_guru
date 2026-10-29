package master

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := Record{Name: "Siswa", BirthDate: "2013-03-20", Gender: "L", Active: true}
	for _, tc := range []struct {
		name         string
		kind         Kind
		record       Record
		reason       string
		creating, ok bool
	}{
		{"optional student numbers", Students, valid, "", true, true},
		{"required birth date", Students, Record{Name: "A", Gender: "L"}, "", true, false},
		{"invalid calendar date", Students, Record{Name: "A", Gender: "L", BirthDate: "2013-02-30"}, "", true, false},
		{"future date", Students, Record{Name: "A", Gender: "L", BirthDate: "2999-01-01"}, "", true, false},
		{"required gender", Students, Record{Name: "A", BirthDate: "2013-01-01"}, "", true, false},
		{"optional teacher identifiers", Teachers, Record{Name: "Guru"}, "", true, true},
		{"optional staff number and contact", Staff, Record{Name: "Pegawai"}, "", true, true},
		{"bounded staff number", Staff, Record{Name: "Pegawai", EmployeeNumber: strings.Repeat("x", 65)}, "", true, false},
		{"invalid staff email", Staff, Record{Name: "Pegawai", Email: "bukan-email"}, "", true, false},
		{"invalid email", Teachers, Record{Name: "Guru", Email: "not-an-email"}, "", true, false},
		{"mail display name rejected", Teachers, Record{Name: "Guru", Email: "Guru <guru@example.test>"}, "", true, false},
		{"valid email", Teachers, Record{Name: "Guru", Email: "guru@example.test"}, "", true, true},
		{"invalid account", Teachers, Record{Name: "Guru", UserID: -1}, "", true, false},
		{"subject code", Subjects, Record{Name: "Mapel", Code: " pai "}, "", true, true},
		{"required subject code", Subjects, Record{Name: "Mapel"}, "", true, false},
		{"correction requires reason", Students, valid, "", false, false},
		{"correction preserves id", Students, valid, "Pembetulan nama", false, true},
		{"bounded name", Teachers, Record{Name: strings.Repeat("a", 201)}, "", true, false},
		{"unknown kind", Kind("students;DROP TABLE"), valid, "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := Input{Record: tc.record, Reason: tc.reason}
			err := input.Validate(tc.kind, tc.creating)
			if (err == nil) != tc.ok {
				t.Fatalf("valid=%v error=%v", tc.ok, err)
			}
			if tc.name == "subject code" && input.Code != "PAI" {
				t.Fatal("code not normalized")
			}
		})
	}
}
