package web

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestSemesterFinalReviewStates(t *testing.T) {
	tmpl, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber": func(int, int) int { return 1 }, "weekdayName": func(int) string { return "Senin" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: tmpl}
	selected := academic.LearningObjectiveContext{SemesterID: 1, ClassID: 2, SubjectID: 3, SemesterName: "Semester 1", ClassName: "7A", SubjectName: "Matematika"}
	weights := academic.AssessmentWeights{NH: 40, ASTS: 30, ASAS: 30, Configured: true}
	for _, tc := range []struct {
		name             string
		needs, uncertain bool
	}{
		{"normal", false, false},
		{"review", true, false},
		{"uncertain", false, true},
		{"uncertain takes precedence", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := academic.SemesterRecommendationStudent{ID: "4", Name: "Siswa sintetis", Status: "Final", Finalized: true, FinalValue: "72", FinalVersion: 1,
				NeedsReview: tc.needs, ReviewUncertain: tc.uncertain}
			for _, v := range []view{
				{Page: "semester-finalization", SemesterFinalization: &semesterFinalView{
					Selected: selected, URL: "/guru/rekomendasi-semester/final", ReturnURL: "/guru/rekomendasi-semester",
					Preview: academic.SemesterFinalPreview{
						Student: st, Weights: weights, Finalized: true, FinalValue: "72", FinalVersion: 1, HistoricalAccess: true,
						NeedsReview: tc.needs, ReviewUncertain: tc.uncertain,
						History:    []academic.SemesterFinalVersion{{Version: 1, Value: "72"}},
						Retentions: []academic.SemesterFinalRetention{{Version: 1, Reason: "Riwayat keputusan sintetis"}},
					},
				}},
				{Page: "semester-recommendations", SemesterRecommendations: &semesterRecommendationView{
					Contexts: []academic.LearningObjectiveContext{selected}, Selected: selected, SelectedSet: true,
					Result: &academic.SemesterRecommendation{Weights: weights, Students: []academic.SemesterRecommendationStudent{st}},
				}},
			} {
				t.Run(v.Page, func(t *testing.T) {
					r := httptest.NewRequest(http.MethodGet, "/guru/rekomendasi-semester", nil)
					r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &auth.User{ID: 1, Active: true, Roles: []string{"guru_mapel"}}))
					w := httptest.NewRecorder()
					app.render(w, r, "page", http.StatusOK, v)
					body := w.Body.String()
					if w.Code != http.StatusOK || !strings.Contains(body, st.Name) {
						t.Fatalf("render %s: HTTP %d", v.Page, w.Code)
					}
					needs := tc.needs && !tc.uncertain
					if strings.Contains(body, "Perlu Ditinjau") != needs ||
						strings.Contains(body, "Bukti akademik belum cukup untuk memastikan perubahan Final.") != tc.uncertain {
						t.Fatal("penanda tidak sesuai hasil pembanding")
					}
					if v.Page == "semester-finalization" {
						for _, marker := range []string{`name="action" value="revision"`, `name="action" value="retain"`, `name="snapshot_token"`} {
							if strings.Contains(body, marker) == tc.uncertain {
								t.Fatalf("form mutasi %s tidak sesuai status", marker)
							}
						}
						if !strings.Contains(body, "Riwayat Versi Final") || !strings.Contains(body, "Riwayat keputusan sintetis") {
							t.Fatal("histori harus tetap terlihat")
						}
					} else if !strings.Contains(body, "Lihat snapshot Final") || strings.Contains(body, `name="student_id" value="4"`) {
						t.Fatal("akses snapshot harus tersedia dan Final tidak boleh dipilih ulang")
					}
				})
			}
		})
	}
}
