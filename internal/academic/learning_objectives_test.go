package academic

import (
	"context"
	"errors"
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
