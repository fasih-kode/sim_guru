package academic

import "testing"

func TestCalculateDirectTPNH(t *testing.T) {
	cases := []struct {
		name                        string
		cells                       []directTPNHCell
		complete, evidence, unknown bool
		exact                       string
	}{
		{"nol dan rata-rata eksak", []directTPNHCell{{directTPApplicable, "0"}, {directTPApplicable, "82.50"}}, true, true, false, "165/4"},
		{"kosong bukan nol", []directTPNHCell{{directTPApplicable, "0"}, {directTPApplicable, ""}}, false, true, false, ""},
		{"tidak berlaku dikecualikan", []directTPNHCell{{directTPApplicable, "70"}, {directTPNotApplicable, "0"}}, true, true, false, "70"},
		{"penempatan belum pasti", []directTPNHCell{{directTPApplicable, "70"}, {directTPUnknown, ""}}, false, true, true, ""},
		{"tidak ada penyebut", []directTPNHCell{{directTPNotApplicable, ""}}, false, false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := calculateDirectTPNH(tc.cells)
			if err != nil {
				t.Fatal(err)
			}
			if got.Complete != tc.complete || got.HasEvidence != tc.evidence || got.Unknown != tc.unknown {
				t.Fatalf("hasil: %+v", got)
			}
			if got.Exact != nil && got.Exact.RatString() != tc.exact {
				t.Fatalf("rata-rata: %s", got.Exact)
			}
			if got.Exact == nil && tc.exact != "" {
				t.Fatal("rata-rata tidak tersedia")
			}
		})
	}
}
