package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func TestDirectTPApplicabilityTemplate(t *testing.T) {
	tpl, err := template.ParseFiles("templates/direct_tp_applicability.html")
	if err != nil {
		t.Fatal(err)
	}
	v := view{DirectTPApplicability: &directTPApplicabilityView{Context: academic.LearningObjectiveContext{SemesterID: 1, ClassID: 2, SubjectID: 3}, Student: academic.DirectTPBookStudent{ID: 4, Name: "Siswa sintetis"}, Cell: academic.DirectTPBookCell{ObjectiveID: 5, Code: "TP1"}, Version: "2", Submission: "00000000-0000-4000-8000-000000000001", History: []academic.DirectTPApplicabilityHistory{{Version: 2, Reason: "<b>alasan</b>"}, {Version: 1, Applicable: true}}}}
	var b bytes.Buffer
	if err = tpl.ExecuteTemplate(&b, "direct-tp-applicability", v); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`method="post"`, `name="version" value="2"`, `name="effective_date"`, `Tidak Berlaku`, `name="reason"`, `Versi 2`, `Versi 1`, `&lt;b&gt;alasan&lt;/b&gt;`} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("tidak ada %s", want)
		}
	}
}
