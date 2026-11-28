package academic

import (
	"strings"
	"testing"
)

func TestHistoricalTPPreviewInputBoundsAndDate(t *testing.T) {
	valid := HistoricalTPPreviewInput{
		SemesterID: 1, ClassID: 2, SubjectID: 3,
		Definition: "Menjelaskan perubahan iklim", Reason: "Koreksi catatan historis",
		AssessmentDate: "2026-11-10", DateSource: "Arsip nilai",
		Learning: HistoricalTPReference{Kind: "manual"}, Assessment: HistoricalTPReference{Kind: "app"},
	}
	if err := validateHistoricalTPPreviewInput(valid); err != nil {
		t.Fatalf("input sah ditolak: %v", err)
	}
	for name, mutate := range map[string]func(*HistoricalTPPreviewInput){
		"konteks kosong":            func(v *HistoricalTPPreviewInput) { v.ClassID = 0 },
		"uraian kosong":             func(v *HistoricalTPPreviewInput) { v.Definition = " " },
		"uraian terlalu panjang":    func(v *HistoricalTPPreviewInput) { v.Definition = strings.Repeat("a", 4001) },
		"tanggal keliru":            func(v *HistoricalTPPreviewInput) { v.AssessmentDate = "2026-02-30" },
		"jenis tidak sah":           func(v *HistoricalTPPreviewInput) { v.Learning.Kind = "lain" },
		"referensi terlalu panjang": func(v *HistoricalTPPreviewInput) { v.Assessment.Archive = strings.Repeat("é", 501) },
	} {
		t.Run(name, func(t *testing.T) {
			v := valid
			mutate(&v)
			if err := validateHistoricalTPPreviewInput(v); err == nil {
				t.Fatal("input tidak sah diterima")
			}
		})
	}
}
