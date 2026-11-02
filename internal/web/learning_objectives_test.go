package web

import (
	"net/http/httptest"
	"testing"
)

func TestLearningObjectiveSelectionParsing(t *testing.T) {
	r := httptest.NewRequest("GET", "/guru/tujuan-pembelajaran?context=12:34:56", nil)
	semester, classID, subject, selected, err := learningObjectiveSelection(r)
	if err != nil || !selected || semester != 12 || classID != 34 || subject != 56 {
		t.Fatalf("konteks gabungan tidak terbaca: %d/%d/%d selected=%v err=%v", semester, classID, subject, selected, err)
	}
	r = httptest.NewRequest("GET", "/guru/tujuan-pembelajaran?context=12:34", nil)
	if _, _, _, _, err = learningObjectiveSelection(r); err == nil {
		t.Fatal("konteks tidak lengkap harus ditolak")
	}
	r = httptest.NewRequest("GET", "/guru/tujuan-pembelajaran?semester_id=12&class_id=34&subject_id=56", nil)
	semester, classID, subject, selected, err = learningObjectiveSelection(r)
	if err != nil || !selected || semester != 12 || classID != 34 || subject != 56 {
		t.Fatalf("deep link konteks tidak terbaca: %d/%d/%d selected=%v err=%v", semester, classID, subject, selected, err)
	}
}
