package web

import "testing"

func TestStudentGuidanceUsesSharedInstantSearchOnlyForList(t *testing.T) {
	if !liveListPage("student-guidance") {
		t.Fatal("daftar pembinaan harus memakai Instant Search bersama")
	}
	if liveListPage("student-guidance-detail") {
		t.Fatal("halaman detail dan formulir mutasi tidak boleh menjadi fragmen daftar")
	}
}
