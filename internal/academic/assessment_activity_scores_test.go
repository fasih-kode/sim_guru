package academic

import "testing"

func TestParseInitialScoreUsesExactDecimalAndAllowsBlank(t *testing.T) {
	tests := []struct {
		input   string
		cents   int64
		present bool
		invalid bool
	}{
		{"", 0, false, false}, {"  ", 0, false, false}, {"0", 0, true, false},
		{"100", 10000, true, false}, {"82,50", 8250, true, false}, {"82.50", 8250, true, false},
		{"0,5", 50, true, false}, {"007.05", 705, true, false},
		{"-1", 0, false, true}, {"100,01", 0, false, true}, {"82,567", 0, false, true},
		{"101", 0, false, true}, {"+1", 0, false, true}, {"1e2", 0, false, true},
		{"1,2.3", 0, false, true}, {"1.", 0, false, true}, {"1, 2", 0, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, present, err := parseInitialScore(tc.input)
			if tc.invalid {
				if err == nil {
					t.Fatalf("nilai %q semestinya ditolak", tc.input)
				}
				return
			}
			if err != nil || got != tc.cents || present != tc.present {
				t.Fatalf("parse %q = (%d,%v,%v), ingin (%d,%v,nil)", tc.input, got, present, err, tc.cents, tc.present)
			}
		})
	}
}

func TestFormatInitialScore(t *testing.T) {
	if got := formatScore(8250); got != "82.50" {
		t.Fatalf("format=%q", got)
	}
	if got := formatScore(0); got != "0.00" {
		t.Fatalf("format nol=%q", got)
	}
}
