package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type teacherRecapView struct {
	Base, From, To, Search                                    string
	Managed                                                   bool
	TeacherID                                                 int64
	Page, DayPage                                             int
	PreviousURL, NextURL, DayPreviousURL, DayNextURL, BackURL string
	Teachers                                                  []academic.TeacherAttendanceRecap
	Selected                                                  *academic.TeacherAttendanceRecap
	Detail                                                    *academic.TeacherAttendanceEvaluation
}

func (a *application) teacherAttendanceRecap(w http.ResponseWriter, r *http.Request) {
	managed := strings.HasPrefix(r.URL.Path, "/tu/")
	base, title := "/kehadiran-saya/rekap", "Rekap Kehadiran Saya"
	if managed {
		base, title = "/tu/kehadiran-guru/rekap", "Rekap Kehadiran Guru"
	}
	now := academic.TeacherAttendanceWIBTime(time.Now())
	f := academic.TeacherAttendanceRecapFilter{Managed: managed, StartsOn: r.URL.Query().Get("from"), EndsOn: r.URL.Query().Get("to"), Search: r.URL.Query().Get("q"), Page: reportPage(r.URL.Query().Get("page"))}
	if f.StartsOn == "" && f.EndsOn == "" {
		f.StartsOn = now.Format("2006-01-") + "01"
		f.EndsOn = now.Format("2006-01-02")
	}
	if raw := r.URL.Query().Get("teacher_id"); raw != "" {
		id, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || id < 1 {
			http.NotFound(w, r)
			return
		}
		f.TeacherID = id
	}
	data, e := a.options.Academics.TeacherAttendanceRecaps(r.Context(), principal(r).ID, f)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	v := &teacherRecapView{Base: base, From: f.StartsOn, To: f.EndsOn, Search: f.Search, Managed: managed, TeacherID: f.TeacherID, Page: f.Page, DayPage: reportPage(r.URL.Query().Get("day_page")), Teachers: data.Teachers}
	if v.DayPage > 100000 {
		a.academicFailure(w, academic.ValidationError("Halaman rekap tidak valid."))
		return
	}
	link := func(key, value string) string {
		q := url.Values{"from": {f.StartsOn}, "to": {f.EndsOn}, "q": {f.Search}, "page": {strconv.Itoa(f.Page)}}
		if f.TeacherID > 0 {
			q.Set("teacher_id", strconv.FormatInt(f.TeacherID, 10))
		}
		q.Set(key, value)
		return base + "?" + q.Encode()
	}
	v.BackURL = base + "?" + url.Values{"from": {f.StartsOn}, "to": {f.EndsOn}, "q": {f.Search}, "page": {strconv.Itoa(f.Page)}}.Encode()
	if !managed || f.TeacherID > 0 {
		if len(data.Teachers) > 0 {
			selected := data.Teachers[0]
			v.Selected = &selected
			if date := r.URL.Query().Get("date"); date != "" {
				for _, d := range selected.Days {
					if d.Date == date {
						day := d
						v.Detail = &day
						break
					}
				}
				if v.Detail == nil {
					http.NotFound(w, r)
					return
				}
			}
			offset := (v.DayPage - 1) * 10
			if offset > len(selected.Days) {
				offset = len(selected.Days)
			}
			if v.DayPage > 1 {
				v.DayPreviousURL = link("day_page", strconv.Itoa(v.DayPage-1))
			}
			if offset+10 < len(selected.Days) {
				v.DayNextURL = link("day_page", strconv.Itoa(v.DayPage+1))
				selected.Days = selected.Days[offset : offset+10]
			} else {
				selected.Days = selected.Days[offset:]
			}
		}
	} else {
		if len(v.Teachers) > 10 {
			v.Teachers = v.Teachers[:10]
			v.NextURL = link("page", strconv.Itoa(f.Page+1))
		}
		if f.Page > 1 {
			v.PreviousURL = link("page", strconv.Itoa(f.Page-1))
		}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "teacher-recap", Title: title, TeacherRecap: v})
}

func (a *application) teacherTodayEvaluation(r *http.Request, date string) (*academic.TeacherAttendanceEvaluation, error) {
	data, e := a.options.Academics.TeacherAttendanceRecaps(r.Context(), principal(r).ID, academic.TeacherAttendanceRecapFilter{StartsOn: date, EndsOn: date, Page: 1})
	if e != nil {
		return nil, e
	}
	if len(data.Teachers) == 0 || len(data.Teachers[0].Days) == 0 {
		return nil, nil
	}
	return &data.Teachers[0].Days[0], nil
}
