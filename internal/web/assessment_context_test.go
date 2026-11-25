package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestAssessmentContextCarriesOnlyValidatedSelection(t *testing.T) {
	a := academic.LearningObjectiveContext{SemesterID: 1, ClassID: 2, SubjectID: 3, SemesterName: "Semester 1", ClassName: "7A", SubjectName: "Bahasa Indonesia"}
	b := academic.LearningObjectiveContext{SemesterID: 4, ClassID: 5, SubjectID: 6, SemesterName: "Semester 2", ClassName: "7B", SubjectName: "Informatika"}
	for _, x := range []academic.LearningObjectiveContext{a, b} {
		views := []view{
			{Page: "learning-objectives", LearningObjectives: &learningObjectivesView{Contexts: []academic.LearningObjectiveContext{a, b}, Selected: x, SelectedSet: true}},
			{Page: "direct-tp-gradebook", DirectTPGradebook: &directTPGradebookView{Selected: x, SelectedSet: true}},
			{Page: "exam-initial-scores", ExamInitial: &examInitialView{Selected: x, SelectedSet: true}},
			{Page: "assessment-tp-recap", AssessmentTPRecap: &assessmentTPRecapView{Selected: x, SelectedSet: true}},
			{Page: "semester-recommendations", SemesterRecommendations: &semesterRecommendationView{Selected: x, SelectedSet: true}},
		}
		for _, v := range views {
			c := assessmentContextForView(v)
			if c == nil || c.Selected != x || !c.SelectedSet {
				t.Fatalf("%s tidak mempertahankan konteks", v.Page)
			}
			tabs := sectionNavigationTabs("Penilaian", "", &auth.User{Roles: []string{"guru_mapel", "kurikulum"}})
			assessmentContextNavigation(tabs, c)
			want := urlContext(x)
			for _, tab := range tabs {
				if !strings.HasSuffix(tab.Href, want) {
					t.Errorf("%s: %s tidak membawa %s", v.Page, tab.Href, want)
				}
			}
		}
	}
	tabs := sectionNavigationTabs("Penilaian", "", &auth.User{Roles: []string{"guru_mapel"}})
	assessmentContextNavigation(tabs, &assessmentContextView{Selected: a})
	for _, tab := range tabs {
		if strings.Contains(tab.Href, "?") {
			t.Error("konteks yang belum tervalidasi bocor ke tautan")
		}
	}
}

func urlContext(c academic.LearningObjectiveContext) string {
	return fmt.Sprintf("?context=%d%%3A%d%%3A%d", c.SemesterID, c.ClassID, c.SubjectID)
}

func TestAssessmentContextTemplateRendersSummaryAndChoices(t *testing.T) {
	tpl, err := template.ParseFiles("templates/assessment_context.html")
	if err != nil {
		t.Fatal(err)
	}
	selected := academic.LearningObjectiveContext{SemesterID: 1, ClassID: 2, SubjectID: 3, SemesterName: "Tahun 2026 Semester 1", ClassName: "Kelas 7B", SubjectName: "Pendidikan Agama dan Budi Pekerti"}
	data := view{GradebookNavigation: &gradebookNavigationView{}, AssessmentContext: &assessmentContextView{Contexts: []academic.LearningObjectiveContext{selected}, Selected: selected, SelectedSet: true, Action: "/guru/buku-nilai"}}
	var output bytes.Buffer
	if err = tpl.ExecuteTemplate(&output, "assessment-context", data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{selected.SemesterName, selected.ClassName, selected.SubjectName, "Ubah Konteks", `value="1:2:3"`, "data-gradebook-context"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("tidak ada %q pada komponen", want)
		}
	}
}

func TestAssessmentContextNavigationPreservesQueryAndFragment(t *testing.T) {
	tabs := []navigationTab{{Href: "/guru/nilai-asts-asas?date=2026-11-25&context=old#roster"}}
	assessmentContextNavigation(tabs, &assessmentContextView{SelectedSet: true, Selected: academic.LearningObjectiveContext{SemesterID: 1, ClassID: 2, SubjectID: 3}})
	u, err := url.Parse(tabs[0].Href)
	if err != nil || u.Query().Get("context") != "1:2:3" || len(u.Query()["context"]) != 1 || u.Query().Get("date") != "2026-11-25" || u.Fragment != "roster" {
		t.Fatalf("tautan tidak konsisten: %q", tabs[0].Href)
	}
}
