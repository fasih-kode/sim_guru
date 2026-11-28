package academic

import (
	"math/big"
	"testing"
)

func TestSemesterRoundExact(t *testing.T) {
	rat := func(s string) *big.Rat {
		v, ok := new(big.Rat).SetString(s)
		if !ok {
			t.Fatal(s)
		}
		return v
	}
	tests := []struct {
		name, nh, asts, asas string
		w                    AssessmentWeights
		want                 string
	}{
		{"contoh", "80", "70", "90", AssessmentWeights{NH: 60, ASTS: 20, ASAS: 20}, "80"},
		{"setengah naik", "82.5", "0", "0", AssessmentWeights{NH: 100}, "83"},
		{"di bawah setengah", "82.49", "0", "0", AssessmentWeights{NH: 100}, "82"},
		{"tanpa pembulatan antara", "74.995", "80", "0", AssessmentWeights{NH: 50, ASTS: 50}, "77"},
		{"nol sah", "0", "0", "0", AssessmentWeights{NH: 100}, "0"},
		{"seratus sah", "100", "0", "0", AssessmentWeights{NH: 100}, "100"},
		{"bobot nol", "80", "0", "90", AssessmentWeights{NH: 70, ASAS: 30}, "83"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := semesterRound(rat(tt.nh), rat(tt.asts), rat(tt.asas), tt.w)
			if got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}
