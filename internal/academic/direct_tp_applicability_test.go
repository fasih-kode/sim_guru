package academic

import (
	"context"
	"testing"
)

func TestDirectTPApplicabilityRejectsInvalidBeforeDatabase(t *testing.T) {
	valid := DirectTPApplicabilityInput{SemesterID: 1, ClassID: 2, SubjectID: 3, StudentID: 4, ObjectiveID: 5, ExpectedVersion: 0, EffectiveDate: "2026-11-20", Applicable: true, EvidenceVerified: true, Reason: "Bukti pembelajaran", SubmissionKey: "00000000-0000-4000-8000-000000000001"}
	tests := []struct {
		name   string
		change func(*DirectTPApplicabilityInput)
	}{
		{"tanggal", func(v *DirectTPApplicabilityInput) { v.EffectiveDate = "2026-02-30" }},
		{"alasan", func(v *DirectTPApplicabilityInput) { v.Reason = "  " }},
		{"bukti", func(v *DirectTPApplicabilityInput) { v.EvidenceVerified = false }},
		{"versi", func(v *DirectTPApplicabilityInput) { v.ExpectedVersion = -1 }},
		{"UUID", func(v *DirectTPApplicabilityInput) { v.SubmissionKey = "invalid" }},
		{"konteks", func(v *DirectTPApplicabilityInput) { v.ObjectiveID = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := valid
			tt.change(&in)
			if err := (&Store{}).SaveDirectTPApplicability(context.Background(), 1, in); err == nil {
				t.Fatal("input tidak valid diterima")
			}
		})
	}
}
