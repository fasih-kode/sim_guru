package academic

import (
	"context"
	"strings"
	"testing"
)

func TestHistoricalTPConfirmationRequiresVerifiedEvidence(t *testing.T) {
	valid := HistoricalTPConfirmation{Preview: HistoricalTPPreviewInput{SemesterID: 1, ClassID: 2, SubjectID: 3, Definition: "TP", Reason: "alasan", AssessmentDate: "2026-01-01", DateSource: "dokumen", Learning: HistoricalTPReference{Kind: "manual"}, Assessment: HistoricalTPReference{Kind: "manual"}}, Verification: HistoricalTPVerification{LearningSection: "halaman 1", AssessmentSection: "halaman 2", Method: "diperiksa langsung", ResultSource: "lembar hasil", Acknowledged: true}, SnapshotToken: strings.Repeat("0", 64), SubmissionKey: "00000000-0000-4000-8000-000000002053"}
	for name, change := range map[string]func(*HistoricalTPConfirmation){
		"tidak diakui":               func(v *HistoricalTPConfirmation) { v.Verification.Acknowledged = false },
		"bagian pembelajaran kosong": func(v *HistoricalTPConfirmation) { v.Verification.LearningSection = "" },
		"bagian sumatif kosong":      func(v *HistoricalTPConfirmation) { v.Verification.AssessmentSection = "" },
		"metode kosong":              func(v *HistoricalTPConfirmation) { v.Verification.Method = "" },
		"sumber hasil kosong":        func(v *HistoricalTPConfirmation) { v.Verification.ResultSource = "" },
		"tanggal kosong":             func(v *HistoricalTPConfirmation) { v.Preview.AssessmentDate = "" },
		"token keliru":               func(v *HistoricalTPConfirmation) { v.SnapshotToken = "bad" },
	} {
		t.Run(name, func(t *testing.T) {
			v := valid
			change(&v)
			if _, err := (&Store{}).ConfirmHistoricalLearningObjective(context.Background(), 1, v); err == nil {
				t.Fatal("input tidak sah diterima")
			}
		})
	}
}
