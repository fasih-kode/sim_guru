package web

import (
	"bytes"
	"github.com/fasih-kode/sim_guru/internal/academic"
	"html/template"
	"strings"
	"testing"
)

func TestTPCriteriaEmptyGuidance(t *testing.T) {
	source, err := content.ReadFile("templates/tp_criteria.html")
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := template.New("test").Parse(string(source))
	if err != nil {
		t.Fatal(err)
	}
	data := view{TPCriteria: &tpCriteriaView{SelectedSet: true, Criteria: academic.TPCriteria{Overrides: map[int64]academic.TPCriterion{}}}}
	var out bytes.Buffer
	if err = tpl.ExecuteTemplate(&out, "tp-criteria", data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Belum ada pedoman Kurikulum.") {
		t.Fatal("kondisi kosong tidak tampil")
	}
}
