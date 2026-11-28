package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func TestHistoricalTPUncertaintyRemainsVisible(t *testing.T) {
	objective := academic.AssessmentTPRecapObjective{ID: "1", Definition: "TP uji", HistoricalApplicability: "belum pasti", Complete: true, Average: "0,00", RequiredCount: 1, ScoredCount: 1}
	tests := []struct {
		name, file, entry string
		data              view
	}{
		{"rekap", "templates/assessment_tp_recap.html", "assessment-tp-recap", view{AssessmentTPRecap: &assessmentTPRecapView{SelectedSet: true, Recap: &academic.AssessmentTPRecap{}, Student: &academic.AssessmentTPRecapStudent{Name: "Siswa", Objectives: []academic.AssessmentTPRecapObjective{objective}, NHComplete: true, NH: "0,00"}}}},
		{"rekomendasi", "templates/semester_recommendations.html", "semester-recommendations", view{SemesterRecommendations: &semesterRecommendationView{SelectedSet: true, Result: &academic.SemesterRecommendation{Students: []academic.SemesterRecommendationStudent{{Name: "Siswa", Objectives: []academic.AssessmentTPRecapObjective{objective}}}}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tpl, err := template.ParseFiles(tc.file, "templates/assessment_context.html")
			if err != nil {
				t.Fatal(err)
			}
			tc.data.Page = tc.entry
			tc.data.AssessmentContext = assessmentContextForView(tc.data)
			var b bytes.Buffer
			if err = tpl.ExecuteTemplate(&b, tc.entry, tc.data); err != nil {
				t.Fatal(err)
			}
			text := b.String()
			if !strings.Contains(text, "belum dapat diverifikasi") || !strings.Contains(text, "tanggal") {
				t.Fatalf("peringatan tidak terlihat: %s", text)
			}
		})
	}
}
