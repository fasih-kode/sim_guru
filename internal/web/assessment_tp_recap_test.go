package web

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func TestAssessmentTPRecapStudentID(t *testing.T) {
	for _, tc := range []struct {
		values   url.Values
		selected bool
		want     string
		invalid  bool
	}{
		{nil, false, "", false},
		{url.Values{"student_id": {"001"}}, true, "1", false},
		{url.Values{"student_id": {"5"}}, false, "", true},
		{url.Values{"student_id": {""}}, true, "", true},
		{url.Values{"student_id": {"abc"}}, true, "", true},
		{url.Values{"student_id": {"0"}}, true, "", true},
		{url.Values{"student_id": {"-4"}}, true, "", true},
		{url.Values{"student_id": {"1", "2"}}, true, "", true},
		{url.Values{"student_id": {"9223372036854775808"}}, true, "", true},
	} {
		got, err := assessmentTPRecapStudentID(tc.values, tc.selected)
		if got != tc.want || (err != nil) != tc.invalid {
			t.Errorf("parameter %v, konteks %t: mendapat %q, %v", tc.values, tc.selected, got, err)
		}
	}
}

func TestAssessmentTPRecapClassAndStudentViews(t *testing.T) {
	recap := &academic.AssessmentTPRecap{UnknownPlacementCount: 1, OutOfCalendarActivityCount: 1}
	for i := 1; i <= 39; i++ {
		s := academic.AssessmentTPRecapStudent{ID: fmt.Sprint(i), NIS: fmt.Sprintf("0026%03d", i), Name: fmt.Sprintf("Siswa Sintetis %02d", i), NH: "73,50", NHComplete: true}
		for j := 1; j <= 10; j++ {
			s.Objectives = append(s.Objectives, academic.AssessmentTPRecapObjective{
				ID: fmt.Sprint(j), Definition: fmt.Sprintf("Uraian TP privat %d-%d", i, j), Complete: true,
				RequiredCount: 1, ScoredCount: 1, Average: "73,50", Achievement: "Tercapai", Threshold: "70,00", ThresholdSource: "Bawaan",
			})
		}
		recap.Students = append(recap.Students, s)
	}
	partial := &recap.Students[1]
	partial.NH, partial.NHComplete, partial.PlacementUncertain = "", false, true
	partial.Objectives[0].Average = "0,00"
	partial.Objectives[0].Activities = []academic.AssessmentTPRecapActivity{{ID: "41", Name: "Ujian lama", Date: "2026-11-03", HasValue: true, EffectiveValue: "64,00", OutOfCalendar: true}}
	partial.Objectives[1].Complete = false
	partial.Objectives[1].Average = "99,00" // Status service, bukan keberadaan angka, menentukan kelengkapan.
	partial.Objectives[1].Missing = []string{"Nilai TP belum diisi."}
	partial.Objectives[2].Complete = false
	partial.Objectives[2].HistoricalApplicability = "tidak berlaku"
	partial.Objectives[3].Complete = false
	partial.Objectives[3].HistoricalApplicability = "belum pasti"
	partial.Objectives[3].Missing = []string{"Periksa tanggal penempatan."}
	partial.Missing = []string{partial.Objectives[1].Definition, partial.Objectives[3].Definition}
	recap.Students[38].Name = strings.Repeat("Namapanjang", 8)
	recap.Students[38].NIS, recap.Students[38].NH, recap.Students[38].NHComplete, recap.Students[38].Objectives = "", "", false, nil

	tpl, err := template.ParseFiles("templates/assessment_tp_recap.html", "templates/assessment_context.html")
	if err != nil {
		t.Fatal(err)
	}
	render := func(data *assessmentTPRecapView) string {
		t.Helper()
		v := view{Page: "assessment-tp-recap", AssessmentTPRecap: data}
		v.AssessmentContext = assessmentContextForView(v)
		var b bytes.Buffer
		if err := tpl.ExecuteTemplate(&b, v.Page, v); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	context := academic.LearningObjectiveContext{SemesterID: 4, ClassID: 5, SubjectID: 6}
	list := &assessmentTPRecapView{Selected: context, SelectedSet: true, Search: "Siswa & 02"}
	if err := list.setRecap(recap, ""); err != nil {
		t.Fatal(err)
	}
	if len(list.Rows) != 39 || list.Rows[0].Complete != 10 || list.Rows[0].NH != recap.Students[0].NH || list.Rows[38].Number != 39 {
		t.Fatal("ringkasan kehilangan roster, urutan atau NH dari service")
	}
	if row := list.Rows[1]; row.Complete != 7 || row.Applicable != 8 || row.NotApplicable != 1 || row.Uncertain != 1 || row.NHComplete {
		t.Fatalf("ringkasan status salah: %+v", row)
	}
	href, err := url.Parse(list.Rows[1].URL)
	if err != nil || href.Query().Get("context") != "4:5:6" || href.Query().Get("q") != list.Search || href.Query().Get("student_id") != "2" {
		t.Fatalf("URL detail kehilangan state: %s", list.Rows[1].URL)
	}
	html := render(list)
	if strings.Count(html, "data-table-search-text") != 39 || strings.Contains(html, "Uraian TP privat") || strings.Contains(html, "Ujian lama") {
		t.Fatal("daftar tidak ringkas atau roster tidak lengkap")
	}
	for _, id := range []string{"1", "2", "39"} {
		detail := &assessmentTPRecapView{Selected: context, SelectedSet: true, Search: list.Search}
		if err := detail.setRecap(recap, id); err != nil {
			t.Fatal(err)
		}
		if detail.ListURL != list.ListURL || len(detail.Rows) != 0 || detail.Student.ID != id {
			t.Fatal("detail memuat roster lain atau kehilangan URL kembali")
		}
		body := render(detail)
		switch id {
		case "1":
			if detail.Student.NH != list.Rows[0].NH || !strings.Contains(body, "73,50") {
				t.Fatal("NH detail tidak sama dengan ringkasan")
			}
		case "2":
			for _, want := range []string{"0,00", "Tidak Berlaku", "Belum dapat dipastikan", "Nilai TP belum diisi.", "Periksa tanggal penempatan.", "Uraian TP privat 2-10", "70,00 · Bawaan", "Kegiatan Sumatif historis", "Ujian lama", "2026-11-03", "64,00", "/guru/kegiatan-penilaian/41/nilai"} {
				if !strings.Contains(body, want) {
					t.Errorf("detail kehilangan %q", want)
				}
			}
			for _, unwanted := range []string{"Siswa Sintetis 01", "Uraian TP privat 1-1", "Sumatif penyusun"} {
				if strings.Contains(body, unwanted) {
					t.Errorf("detail memuat %q", unwanted)
				}
			}
		case "39":
			if !strings.Contains(body, recap.Students[38].Name) || !strings.Contains(body, "NIS tidak tersedia") || !strings.Contains(body, "Belum ada TP pada konteks ini.") {
				t.Fatal("detail nama panjang, NIS kosong, atau konteks tanpa TP hilang")
			}
		}
	}
	if err := (&assessmentTPRecapView{}).setRecap(recap, "40"); !errors.Is(err, academic.ErrNotFound) {
		t.Fatalf("siswa di luar roster: %v", err)
	}
}
