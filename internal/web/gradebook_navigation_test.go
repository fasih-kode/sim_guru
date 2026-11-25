package web

import (
	"bytes"
	"html/template"
	"net/url"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func TestGradebookNavigationKeepsContextAndRosterDate(t *testing.T) {
	for _, class := range []int64{2, 4} {
		v := view{Page: "exam-initial-scores", ExamInitial: &examInitialView{SelectedSet: true, Selected: academic.LearningObjectiveContext{SemesterID: 1, ClassID: class, SubjectID: 3}, Date: "2026-11-25", Tab: "asas"}}
		v.AssessmentContext = assessmentContextForView(v)
		nav := gradebookNavigationForView(v, "")
		if nav.NavigationActive != "asas" || len(nav.NavigationTabs) != 3 {
			t.Fatalf("tab salah: %+v", nav)
		}
		for _, tab := range nav.NavigationTabs {
			u, err := url.Parse(tab.Href)
			if err != nil || u.Query().Get("date") != v.ExamInitial.Date || !strings.HasSuffix(tab.Href, u.RawQuery) || !strings.Contains(tab.Href, urlContext(v.ExamInitial.Selected)[1:]) {
				t.Fatalf("tab kehilangan konteks atau tanggal: %s", tab.Href)
			}
			if tab.ID != "tp" && u.Query().Get("tab") != tab.ID {
				t.Fatalf("tautan ujian tidak memilih tab: %s", tab.Href)
			}
		}
	}
}

func TestExamGradebookRendersOnlyActiveComponentAndLocksRecordedScore(t *testing.T) {
	tpl, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(int, int) int { return 1 }, "weekdayName": func(int) string { return "" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, tab := range []string{"asts", "asas"} {
		t.Run(tab, func(t *testing.T) {
			recorded := academic.ExamInitialCell{Recorded: true, Value: "0", Date: "2026-11-22", CorrectedValue: "82,50", CanCorrect: true, ScoreID: 9}
			context := academic.LearningObjectiveContext{SemesterID: 1, ClassID: 2, SubjectID: 3}
			v := view{Page: "exam-initial-scores", ExamInitial: &examInitialView{Tab: tab, Date: "2026-11-25", SelectedSet: true, Contexts: []academic.LearningObjectiveContext{context}, Selected: context, Sheet: academic.ExamInitialSheet{Students: []academic.ExamInitialStudent{{ID: 10, Name: "Siswa awal", ASTS: recorded, ASAS: recorded}, {ID: 11, Name: "Siswa berikutnya"}}}}}
			v.AssessmentContext = assessmentContextForView(v)
			v.GradebookNavigation = gradebookNavigationForView(v, "")
			var out bytes.Buffer
			if err := tpl.ExecuteTemplate(&out, "exam-initial-scores", v); err != nil {
				t.Fatal(err)
			}
			body := out.String()
			for _, want := range []string{
				`method="post" action="/guru/nilai-asts-asas?tab=` + tab + `"`,
				`name="roster_date" value="2026-11-25"`,
				`name="score_` + tab + `_11" inputmode="decimal" autocomplete="off" value=""`,
				`name="date_` + tab + `_11" type="date" value="2026-11-25"`,
				`<strong>0</strong>`, "82,50", "/guru/nilai-ujian/9/remedial",
				`data-navigation-tab="` + tab + `"`, `aria-current="page"`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("markup kehilangan %q", want)
				}
			}
			other := "asts"
			if tab == other {
				other = "asas"
			}
			for _, key := range []string{"score_" + other + "_", "date_" + other + "_", "score_" + tab + "_10", "date_" + tab + "_10"} {
				if strings.Contains(body, `name="`+key) {
					t.Errorf("nilai terkunci atau komponen lain ikut menjadi input: %s", key)
				}
			}
		})
	}
}

func TestExamGradebookRejectsForeignComponentButKeepsLegacyPost(t *testing.T) {
	form := url.Values{"score_asts_1": {"0"}, "date_asts_1": {"2026-11-20"}, "score_asas_2": {""}, "date_asas_2": {"2026-11-21"}}
	if _, err := examGradebookInputs(form, "asts", false); err == nil {
		t.Fatal("komponen asing diterima pada POST eksplisit")
	}
	if entries, err := examGradebookInputs(form, "asts", true); err != nil || len(entries) != 2 {
		t.Fatalf("POST lama: %+v, %v", entries, err)
	}
	form.Del("score_asas_2")
	if entries, err := examGradebookInputs(form, "asts", false); err != nil || len(entries) != 1 || entries[0].Value != "0" || entries[0].Date != "2026-11-20" {
		t.Fatalf("POST ASTS: %+v, %v", entries, err)
	}
	for _, raw := range []string{"tab=", "tab=tp", "tab=asts&tab=asas"} {
		q, _ := url.ParseQuery(raw)
		if _, err := examGradebookTab(q); err == nil {
			t.Fatalf("tab ambigu diterima: %s", raw)
		}
	}
}
