package web

import (
	"net/url"
	"strings"
	"time"
)

const examGradebookPath = "/guru/nilai-asts-asas"

type gradebookNavigationView struct {
	NavigationSection, NavigationActive string
	NavigationTabs                      []navigationTab
	Date                                string
}

func gradebookDate(value string) string {
	if date, err := time.Parse(time.DateOnly, value); err == nil && date.Format(time.DateOnly) == value {
		return value
	}
	return ""
}

func gradebookNavigationForView(v view, date string) *gradebookNavigationView {
	active := "tp"
	switch v.Page {
	case "direct-tp-gradebook":
	case "exam-initial-scores":
		if v.ExamInitial == nil {
			return nil
		}
		active, date = v.ExamInitial.Tab, v.ExamInitial.Date
	default:
		return nil
	}
	date = gradebookDate(date)
	nav := &gradebookNavigationView{NavigationSection: "Buku Nilai", NavigationActive: active, Date: date}
	for _, id := range []string{"tp", "asts", "asas"} {
		query := url.Values{}
		if date != "" {
			query.Set("date", date)
		}
		tab := navigationTab{ID: id, Label: strings.ToUpper(id), Href: examGradebookPath,
			Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 4h14v16H5zM8 8h8m-8 4h8m-8 4h5" stroke-linecap="round"/></svg>`}
		if id == "tp" {
			tab.Label, tab.Href = "Nilai TP", "/guru/buku-nilai"
		} else {
			query.Set("tab", id)
		}
		tab.Href = (&url.URL{Path: tab.Href, RawQuery: query.Encode()}).String()
		nav.NavigationTabs = append(nav.NavigationTabs, tab)
	}
	assessmentContextNavigation(nav.NavigationTabs, v.AssessmentContext)
	return nav
}
