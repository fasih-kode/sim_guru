package web

import (
	"net/url"
	"testing"
)

func TestAssessmentScoreInputsRejectMalformedAndPreserveExactText(t *testing.T) {
	inputs, err := assessmentScoreInputs(url.Values{
		"score_12_3":         {"0"},
		"score_12_4":         {"82,50"},
		"gorilla.csrf.Token": {"token"},
	})
	if err != nil || len(inputs) != 2 || inputs[0].Value != "0" || inputs[1].Value != "82,50" {
		t.Fatalf("input nilai = %+v, err=%v", inputs, err)
	}
	for _, tc := range []url.Values{
		{"score_invalid": {"20"}},
		{"score_12_3": {"20", "21"}},
		{"score_0_3": {"20"}},
	} {
		if _, err = assessmentScoreInputs(tc); err == nil {
			t.Fatalf("form malformed harus ditolak: %#v", tc)
		}
	}
}
