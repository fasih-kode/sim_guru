package web

import (
	"fmt"
	"net/url"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type assessmentContextView struct {
	Contexts     []academic.LearningObjectiveContext
	Selected     academic.LearningObjectiveContext
	SelectedSet  bool
	Action, Date string
	Exam         bool
}

// assessmentContextForView hanya menggunakan konteks yang sudah diperiksa handler.
func assessmentContextForView(v view) *assessmentContextView {
	c := &assessmentContextView{}
	switch {
	case v.Page == "learning-objectives" && v.LearningObjectives != nil:
		x := v.LearningObjectives
		c.Contexts, c.Selected, c.SelectedSet, c.Action = x.Contexts, x.Selected, x.SelectedSet, "/guru/tujuan-pembelajaran"
	case v.Page == "direct-tp-gradebook" && v.DirectTPGradebook != nil:
		x := v.DirectTPGradebook
		c.Contexts, c.Selected, c.SelectedSet, c.Action = x.Contexts, x.Selected, x.SelectedSet, "/guru/buku-nilai"
	case v.Page == "exam-initial-scores" && v.ExamInitial != nil:
		x := v.ExamInitial
		c.Contexts, c.Selected, c.SelectedSet, c.Action, c.Date, c.Exam = x.Contexts, x.Selected, x.SelectedSet, "/guru/nilai-asts-asas", x.Date, true
	case v.Page == "assessment-tp-recap" && v.AssessmentTPRecap != nil:
		x := v.AssessmentTPRecap
		c.Contexts, c.Selected, c.SelectedSet, c.Action = x.Contexts, x.Selected, x.SelectedSet, "/guru/rekap-nilai-tp"
	case v.Page == "semester-recommendations" && v.SemesterRecommendations != nil:
		x := v.SemesterRecommendations
		c.Contexts, c.Selected, c.SelectedSet, c.Action = x.Contexts, x.Selected, x.SelectedSet, "/guru/rekomendasi-semester"
	default:
		return nil
	}
	return c
}

func assessmentContextNavigation(tabs []navigationTab, c *assessmentContextView) {
	if c == nil || !c.SelectedSet {
		return
	}
	value := fmt.Sprintf("%d:%d:%d", c.Selected.SemesterID, c.Selected.ClassID, c.Selected.SubjectID)
	for i := range tabs {
		u, err := url.Parse(tabs[i].Href)
		if err != nil {
			continue
		}
		query := u.Query()
		query.Set("context", value)
		u.RawQuery = query.Encode()
		tabs[i].Href = u.String()
	}
}
