package web

import (
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type foundationView struct {
	Batch                 *placementBatchView
	Semesters             []academic.Semester
	Years                 []academic.Year
	Calendar              academic.SemesterCalendar
	CalendarForm          academic.SemesterCalendar
	Days                  []int
	SemesterID, YearID    int64
	PlacementDates        []academic.PlacementDate
	PlacementForm         academic.PlacementDate
	Exception             academic.SchoolDateException
	Reason, ExceptionMode string
	History               []academic.History
}

func (a *application) schoolCalendar(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	s := a.options.Academics
	semesters, err := s.Semesters(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	f := &foundationView{Semesters: semesters, SemesterID: selectedSemester(r, semesters), Days: []int{1, 2, 3, 4, 5, 6, 7}}
	v := view{Page: "school-calendar", Title: "Kalender Sekolah", Foundation: f}
	if f.SemesterID == 0 {
		a.render(w, r, "page", 200, v)
		return
	}
	f.Calendar, err = s.SemesterCalendar(r.Context(), actor, f.SemesterID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	f.CalendarForm = f.Calendar
	status := 200
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		f.Reason = r.PostForm.Get("reason")
		switch r.PostForm.Get("action") {
		case "calendar":
			c := academic.SemesterCalendar{SemesterID: f.SemesterID, StartsOn: r.PostForm.Get("starts_on"), EndsOn: r.PostForm.Get("ends_on"), Version: formInt64(r, "version")}
			for _, raw := range r.PostForm["school_days"] {
				d, e := strconv.Atoi(raw)
				if e != nil {
					d = 0
				}
				c.SchoolDays = append(c.SchoolDays, d)
			}
			f.CalendarForm = c
			err = s.SaveSemesterCalendar(r.Context(), actor, c, f.Reason)
			v.ModalID = "calendar-config-modal"
		case "exception":
			f.Exception = academic.SchoolDateException{Date: r.PostForm.Get("date"), Label: r.PostForm.Get("label"), SchoolDay: r.PostForm.Get("mode") == "school"}
			f.ExceptionMode = r.PostForm.Get("mode")
			if f.ExceptionMode != "school" && f.ExceptionMode != "holiday" && f.ExceptionMode != "reset" {
				err = academic.ValidationError("Pilih jenis hari yang valid.")
			} else {
				err = s.SaveSchoolDateException(r.Context(), actor, f.SemesterID, formInt64(r, "version"), f.Exception, f.ExceptionMode == "reset", f.Reason)
			}
			v.ModalID = "calendar-exception-modal"
		default:
			err = academic.ValidationError("Aksi tidak valid.")
		}
		if err == nil {
			http.Redirect(w, r, "/akademik/kalender-sekolah?semester_id="+strconv.FormatInt(f.SemesterID, 10)+"&saved=1", 303)
			return
		}
		status, v.Error = academicError(err)
		if status >= 500 {
			a.failure(w, err)
			return
		}
		v.ModalOpen = true
	} else if date := r.URL.Query().Get("edit_date"); date != "" {
		found := false
		for _, x := range f.Calendar.Exceptions {
			if x.Date == date {
				f.Exception = x
				found = true
				f.ExceptionMode = "holiday"
				if x.SchoolDay {
					f.ExceptionMode = "school"
				}
			}
		}
		if !found {
			a.academicFailure(w, academic.ErrNotFound)
			return
		}
		v.ModalOpen = true
		v.ModalID = "calendar-exception-modal"
	}
	f.History, err = s.FoundationHistory(r.Context(), actor, "calendar", f.SemesterID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Kalender sekolah tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", status, v)
}
func (a *application) placementDates(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	s := a.options.Academics
	years, err := s.Years(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	year, _ := strconv.ParseInt(r.URL.Query().Get("year_id"), 10, 64)
	if year == 0 {
		for _, y := range years {
			if y.Active {
				year = y.ID
				break
			}
		}
		if year == 0 && len(years) > 0 {
			year = years[0].ID
		}
	}
	if year != 0 {
		found := false
		for _, y := range years {
			if y.ID == year {
				found = true
			}
		}
		if !found {
			a.academicFailure(w, academic.ErrNotFound)
			return
		}
	}
	f := &foundationView{Years: years, YearID: year}
	v := view{Page: "placement-dates", Title: "Tanggal Mulai Penempatan", Foundation: f}
	status := 200
	f.PlacementDates, err = s.PlacementDates(r.Context(), actor, year)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	edit, _ := strconv.ParseInt(r.URL.Query().Get("edit"), 10, 64)
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		edit = formInt64(r, "id")
	}
	if edit > 0 {
		found := false
		for _, p := range f.PlacementDates {
			if p.ID == edit {
				f.PlacementForm = p
				found = true
				break
			}
		}
		if !found {
			a.academicFailure(w, academic.ErrNotFound)
			return
		}
		v.ModalOpen = true
		v.ModalID = "placement-date-modal"
	}
	if r.Method == http.MethodPost {
		if edit < 1 {
			a.academicFailure(w, academic.ErrNotFound)
			return
		}
		f.PlacementForm.StartsOn = r.PostForm.Get("starts_on")
		f.PlacementForm.Version = formInt64(r, "version")
		f.Reason = r.PostForm.Get("reason")
		err = s.SavePlacementStart(r.Context(), actor, edit, f.PlacementForm.Version, f.PlacementForm.StartsOn, f.Reason)
		if err == nil {
			http.Redirect(w, r, "/admin/placements/dates?year_id="+strconv.FormatInt(year, 10)+"&saved=1", 303)
			return
		}
		status, v.Error = academicError(err)
		if status >= 500 {
			a.failure(w, err)
			return
		}
	}
	if edit > 0 {
		f.History, err = s.FoundationHistory(r.Context(), actor, "placement_date", edit)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Tanggal mulai penempatan tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", status, v)
}
