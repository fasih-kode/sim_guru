package academic

import (
	"strings"
	"testing"
)

func TestValidateYear(t *testing.T) {
	if got, err := validateYear("  2026/2027  "); err != nil || got != "2026/2027" {
		t.Fatalf("normalization failed: %q %v", got, err)
	}
	for _, value := range []string{"", " \t ", strings.Repeat("x", 41), "nul\x00value"} {
		if _, err := validateYear(value); err == nil {
			t.Fatalf("accepted invalid year name %q", value)
		}
	}
}

func TestValidateClass(t *testing.T) {
	if got, reason, err := validateClass(1, 7, " A ", "", true); err != nil || got != "A" || reason != "" {
		t.Fatalf("create validation failed: %q %q %v", got, reason, err)
	}
	for _, tc := range []struct {
		yearID int64
		grade  int
		name   string
	}{{0, 7, "A"}, {1, 6, "A"}, {1, 10, "A"}, {1, 8, ""}, {1, 9, strings.Repeat("a", 41)}} {
		if _, _, err := validateClass(tc.yearID, tc.grade, tc.name, "", true); err == nil {
			t.Fatalf("accepted invalid class: %+v", tc)
		}
	}
	for _, reason := range []string{"", strings.Repeat("x", 501)} {
		if _, _, err := validateClass(1, 7, "A", reason, false); err == nil {
			t.Fatalf("accepted invalid correction reason length=%d", len(reason))
		}
	}
}
