package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSemesterRecommendationsRenderWithoutResult(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(int, int) int { return 1 },
		"weekdayName": func(int) string { return "Senin" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates}
	selected := academic.LearningObjectiveContext{SemesterID: 1, ClassID: 2, SubjectID: 3, ClassName: "7A"}
	for _, tc := range []struct {
		name     string
		selected bool
		result   *academic.SemesterRecommendation
		err      error
		status   int
	}{
		{"terlarang", true, nil, academic.ErrForbidden, http.StatusForbidden},
		{"validasi", true, nil, academic.ValidationError("Validasi sintetis"), http.StatusUnprocessableEntity},
		{"storage", true, nil, &pgconn.PgError{Code: "42501", Message: "DETAIL_INTERNAL_SINTETIS"}, http.StatusInternalServerError},
		{"tanpa konteks", false, nil, nil, http.StatusOK},
		{"hasil tersedia", true, &academic.SemesterRecommendation{Weights: academic.AssessmentWeights{Configured: true}}, nil, http.StatusOK},
	} {
		for _, htmx := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/htmx=%t", tc.name, htmx), func(t *testing.T) {
				r := httptest.NewRequest(http.MethodGet, "/guru/rekomendasi-semester", nil)
				user := &auth.User{ID: 1, Active: true, Roles: []string{"guru_mapel"}}
				r = r.WithContext(context.WithValue(r.Context(), principalKey{}, user))
				if htmx {
					r.Header.Set("HX-Request", "true")
					r.Header.Set("HX-List-Request", "true")
				}
				status, message := http.StatusOK, ""
				if tc.err != nil {
					status, message = academicError(tc.err)
				}
				w := httptest.NewRecorder()
				app.render(w, r, "page", status, view{
					Page:  "semester-recommendations",
					Error: message,
					SemesterRecommendations: &semesterRecommendationView{
						Contexts: []academic.LearningObjectiveContext{selected},
						Selected: selected, SelectedSet: tc.selected, Result: tc.result,
					},
				})
				body := w.Body.String()
				if w.Code != tc.status || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
					t.Fatalf("status=%d tipe=%q, ingin HTTP %d dengan HTML", w.Code, w.Header().Get("Content-Type"), tc.status)
				}
				if !strings.Contains(body, "Konteks penugasan") || !strings.Contains(body, `value="1:2:3"`) {
					t.Fatal("pemilih konteks atau pilihan berizin hilang")
				}
				if message != "" && !strings.Contains(body, message) {
					t.Fatal("pesan error hilang")
				}
				if strings.Contains(body, "DETAIL_INTERNAL_SINTETIS") || strings.Contains(body, "42501") {
					t.Fatal("respons membocorkan detail internal")
				}
				if strings.Contains(body, "Bobot Kurikulum:") != (tc.result != nil) {
					t.Fatal("blok hasil tidak sesuai dengan ketersediaan rekomendasi")
				}
			})
		}
	}
}
