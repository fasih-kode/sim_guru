package academic

import "testing"

func TestTransitionGradeOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		grade   int
		outcome string
		target  int64
		valid   bool
	}{
		{"kelas7 naik", 7, "promote", 8, true}, {"kelas8 naik", 8, "promote", 9, true},
		{"kelas7 mengulang", 7, "repeat", 7, true}, {"kelas8 mengulang", 8, "repeat", 8, true},
		{"kelas9 lulus", 9, "graduate", 0, true}, {"kelas9 tidak lulus", 9, "repeat", 9, true},
		{"kelas9 tidak boleh naik", 9, "promote", 9, false}, {"lulus tidak punya kelas", 9, "graduate", 9, false},
		{"mengulang wajib kelas", 9, "repeat", 0, false}, {"mengulang harus tingkat sama", 9, "repeat", 8, false},
		{"naik harus tingkat berikut", 7, "promote", 9, false}, {"kelas8 tidak boleh lulus", 8, "graduate", 0, false},
		{"keluar", 7, "leave", 0, true}, {"keluar tanpa kelas", 7, "leave", 7, false}, {"belum pilih", 9, "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := TransitionPreview{Students: []TransitionStudent{{StudentID: 1, FromGrade: tc.grade}}, Classes: []Class{{ID: 7, Grade: 7}, {ID: 8, Grade: 8}, {ID: 9, Grade: 9}}}
			err := ValidateTransitionDecisions(p, []TransitionDecision{{StudentID: 1, Outcome: tc.outcome, TargetClassID: tc.target}})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
