package academic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSemesterExceptionPresentationDoesNotChangeSnapshotJSON(t *testing.T) {
	before := SemesterRecommendationStudent{ID: "1", Status: "Belum Lengkap"}
	after := before
	after.ExceptionEligible = true
	after.ExceptionEvidence = []string{"ASTS: 0,00"}
	after.FinalType = "exception"
	a, e := json.Marshal(before)
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(after)
	if e != nil {
		t.Fatal(e)
	}
	if string(a) != string(b) || strings.Contains(string(b), "Exception") || strings.Contains(string(b), "FinalType") {
		t.Fatalf("field presentasi mengubah snapshot: %s", b)
	}
}
