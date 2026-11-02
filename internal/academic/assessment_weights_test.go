package academic

import (
	"errors"
	"testing"
)

func TestValidateAssessmentWeights(t *testing.T) {
	tests := []struct {
		name    string
		weights AssessmentWeights
		valid   bool
	}{
		{"total tepat 100", AssessmentWeights{SemesterID: 1, NH: 60, ASTS: 20, ASAS: 20}, true},
		{"bobot nol diizinkan", AssessmentWeights{SemesterID: 1, NH: 100, ASTS: 0, ASAS: 0}, true},
		{"total 110 ditolak", AssessmentWeights{SemesterID: 1, NH: 60, ASTS: 30, ASAS: 20}, false},
		{"angka di atas batas ditolak", AssessmentWeights{SemesterID: 1, NH: 101, ASTS: 0, ASAS: 0}, false},
		{"semester wajib", AssessmentWeights{NH: 100}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAssessmentWeights(tc.weights)
			if tc.valid && err != nil {
				t.Fatalf("konfigurasi valid ditolak: %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("konfigurasi tidak valid diterima")
			}
		})
	}
	var validation ValidationError
	if err := validateAssessmentWeights(AssessmentWeights{SemesterID: 1, NH: 110}); !errors.As(err, &validation) {
		t.Fatalf("validasi harus memberi pesan pengguna, dapat=%v", err)
	}
}
