package academic

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLearningObjectiveDefinitionValidation(t *testing.T) {
	store := &Store{}
	if err := store.CreateLearningObjective(context.Background(), 1, 1, 1, 1, "  "); err == nil {
		t.Fatal("uraian kosong harus ditolak sebelum akses database")
	} else {
		var validation ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("uraian kosong harus menjadi kesalahan validasi, dapat %T: %v", err, err)
		}
	}
	if err := store.CreateLearningObjective(context.Background(), 1, 0, 1, 1, "TP"); err == nil {
		t.Fatal("konteks tidak lengkap harus ditolak")
	}
}

func TestCopyLearningObjectiveRejectsIncompleteIdentifiers(t *testing.T) {
	store := &Store{}
	if _, err := store.CopyLearningObjective(context.Background(), 1, 0, 2, 3, 4, 5, 6, 7); err == nil {
		t.Fatal("ID TP sumber kosong harus ditolak sebelum mengakses database")
	} else {
		var validation ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("ID TP sumber kosong harus menjadi kesalahan validasi: %v", err)
		}
	}
}

func TestAssessmentActivityInputValidation(t *testing.T) {
	store := &Store{}
	for _, tc := range []struct {
		name, inputName, kind, date string
		ids                         []int64
	}{
		{"tanpa nama", " ", "formatif", "2026-11-02", []int64{1}},
		{"jenis tidak dikenal", "Tes", "ujian", "2026-11-02", []int64{1}},
		{"tanggal tidak valid", "Tes", "sumatif", "02-11-2026", []int64{1}},
		{"tanpa TP", "Tes", "formatif", "2026-11-02", nil},
		{"ID TP tidak valid", "Tes", "sumatif", "2026-11-02", []int64{0}},
		{"ID TP ganda", "Tes", "formatif", "2026-11-02", []int64{1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := store.CreateAssessmentActivity(context.Background(), 1, 1, 1, 1, tc.inputName, tc.kind, tc.date, tc.ids)
			var validation ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("input tidak valid harus ditolak sebelum DB: %v", err)
			}
		})
	}
	if validAssessmentActivityName("Nama\x00Tidak Valid") || validAssessmentActivityName(string([]byte{0xff})) || validAssessmentActivityName(strings.Repeat("界", maxAssessmentActivityName+1)) {
		t.Fatal("nama kegiatan harus berupa teks Unicode valid dan tidak melebihi batas skema")
	}
}
