package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type academicView struct {
	Years                        []academic.Year
	Semesters                    []academic.Semester
	Classes                      []academic.Class
	Class                        academic.Class
	History                      []academic.History
	CurrentYear, CurrentSemester string
	Editing                      int64
	HistoryEntity                string
	PlacementCounts              map[int64]int
	NewYearName                  string
}

type placementView struct {
	Years         []academic.Year
	Classes       []academic.Class
	SelectedClass academic.Class
	Candidates    []academic.StudentOption
	Placements    []academic.Placement
	YearID        int64
	ClassID       int64
	Page          int
	Previous      int
	Next          int
	PreviousURL   string
	NextURL       string
	Search        string
}

func academicError(err error) (int, string) {
	var validation academic.ValidationError
	switch {
	case errors.As(err, &validation):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, academic.ErrForbidden):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, academic.ErrNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, academic.ErrConflict):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "Data akademik belum dapat diproses. Silakan coba kembali."
	}
}

func (a *application) academicFailure(w http.ResponseWriter, err error) {
	code, message := academicError(err)
	if code >= 500 {
		a.failure(w, err)
		return
	}
	http.Error(w, message, code)
}

func (a *application) academicHome(w http.ResponseWriter, r *http.Request) {
	years, err := a.options.Academics.Years(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semesters, err := a.options.Academics.Semesters(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	classes, _, err := a.options.Academics.Classes(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	av := &academicView{Years: years, Semesters: semesters, Classes: classes}
	for _, year := range years {
		if year.Active {
			av.CurrentYear = year.Name
		}
	}
	for _, semester := range semesters {
		if semester.Active {
			av.CurrentSemester = "Semester " + strconv.Itoa(semester.Number)
		}
	}
	v := view{Page: "academic", Title: "Akademik", Academic: av}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Perubahan data akademik tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) academicYears(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	years, err := a.options.Academics.Years(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := view{Page: "academic-years", Title: "Tahun Ajaran", Academic: &academicView{Years: years}}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		_, err = a.options.Academics.CreateYear(r.Context(), actor, r.PostForm.Get("name"))
		if err == nil {
			http.Redirect(w, r, "/akademik/tahun-ajaran?saved=1", http.StatusSeeOther)
			return
		}
		code, message := academicError(err)
		v.ModalOpen = true
		v.Error = message
		v.Academic.NewYearName = r.PostForm.Get("name")
		a.render(w, r, "page", code, v)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Tahun ajaran dan dua semesternya tersimpan."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) activateYear(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err = a.options.Academics.ActivateYear(r.Context(), principal(r).ID, id); err != nil {
		a.academicFailure(w, err)
		return
	}
	http.Redirect(w, r, "/akademik/tahun-ajaran?saved=1", http.StatusSeeOther)
}

func (a *application) academicSemesters(w http.ResponseWriter, r *http.Request) {
	terms, err := a.options.Academics.Semesters(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	years, err := a.options.Academics.Years(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := view{Page: "academic-semesters", Title: "Semester", Academic: &academicView{Years: years, Semesters: terms}}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Semester aktif diperbarui dan histori tercatat."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) activateSemester(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err = a.options.Academics.ActivateSemester(r.Context(), principal(r).ID, id); err != nil {
		a.academicFailure(w, err)
		return
	}
	http.Redirect(w, r, "/akademik/semester?saved=1", http.StatusSeeOther)
}

func (a *application) academicClasses(w http.ResponseWriter, r *http.Request) {
	classes, years, err := a.options.Academics.Classes(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	counts, err := a.options.Academics.PlacementCounts(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := view{Page: "academic-classes", Title: "Kelas", Academic: &academicView{Classes: classes, Years: years, PlacementCounts: counts}}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Data kelas tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func placementPage(a *application, r *http.Request, yearID, classID int64, search string, page int) (*placementView, int, error) {
	if page < 1 {
		page = 1
	}
	if page > 10000 || len(search) > 480 {
		return nil, http.StatusBadRequest, academic.ValidationError("Pencarian atau halaman tidak valid.")
	}
	actor := principal(r).ID
	years, err := a.options.Academics.Years(r.Context(), actor)
	if err != nil {
		code, _ := academicError(err)
		return nil, code, err
	}
	classes, _, err := a.options.Academics.Classes(r.Context(), actor)
	if err != nil {
		code, _ := academicError(err)
		return nil, code, err
	}
	if yearID == 0 {
		for _, year := range years {
			if year.Active {
				yearID = year.ID
				break
			}
		}
		if yearID == 0 && len(years) > 0 {
			yearID = years[0].ID
		}
	}
	if yearID > 0 {
		found := false
		for _, year := range years {
			if year.ID == yearID {
				found = true
				break
			}
		}
		if !found {
			return nil, http.StatusNotFound, academic.ErrNotFound
		}
	}
	p := &placementView{Years: years, YearID: yearID, ClassID: classID, Page: page, Search: search}
	for _, class := range classes {
		if class.SchoolYearID == yearID {
			p.Classes = append(p.Classes, class)
			if classID == class.ID {
				p.SelectedClass = class
				classID = 0
			}
		}
	}
	if p.ClassID != 0 && classID != 0 {
		return nil, http.StatusNotFound, academic.ErrNotFound
	}
	if yearID == 0 {
		return p, http.StatusOK, nil
	}
	p.Placements, err = a.options.Academics.Placements(r.Context(), actor, yearID, p.ClassID, search, page)
	if err != nil {
		code, _ := academicError(err)
		return nil, code, err
	}
	for i := range p.Placements {
		p.Placements[i].At = p.Placements[i].At.In(time.FixedZone("WIB", 7*3600))
	}
	p.Previous = 0
	if page > 1 {
		p.Previous = page - 1
	}
	if len(p.Placements) > 10 {
		p.Next = page + 1
		p.Placements = p.Placements[:10]
	}
	if p.ClassID > 0 && principal(r).HasRole("admin") {
		p.Candidates, err = a.options.Academics.UnplacedStudents(r.Context(), actor, yearID, search, page)
		if err != nil {
			code, _ := academicError(err)
			return nil, code, err
		}
		if len(p.Candidates) > 10 {
			p.Candidates = p.Candidates[:10]
			if p.Next == 0 {
				p.Next = page + 1
			}
		}
	}
	if p.Previous > 0 {
		p.PreviousURL = placementPageURL(yearID, p.ClassID, search, p.Previous)
	}
	if p.Next > 0 {
		p.NextURL = placementPageURL(yearID, p.ClassID, search, p.Next)
	}
	return p, http.StatusOK, nil
}

func placementPageURL(yearID, classID int64, search string, page int) string {
	query := url.Values{}
	query.Set("year_id", strconv.FormatInt(yearID, 10))
	query.Set("class_id", strconv.FormatInt(classID, 10))
	query.Set("q", search)
	query.Set("page", strconv.Itoa(page))
	return "/admin/placements?" + query.Encode()
}

func (a *application) studentPlacements(w http.ResponseWriter, r *http.Request) {
	var yearID, classID int64
	var page int
	var search string
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		var err error
		yearID, err = strconv.ParseInt(r.PostForm.Get("year_id"), 10, 64)
		if err != nil || yearID < 1 {
			http.Error(w, "Pilih tahun ajaran yang valid.", http.StatusUnprocessableEntity)
			return
		}
		classID, err = strconv.ParseInt(r.PostForm.Get("class_id"), 10, 64)
		if err != nil || classID < 1 {
			http.Error(w, "Pilih kelas yang valid.", http.StatusUnprocessableEntity)
			return
		}
		studentID, err := strconv.ParseInt(r.PostForm.Get("student_id"), 10, 64)
		if err != nil || studentID < 1 {
			http.Error(w, "Pilih siswa yang valid.", http.StatusUnprocessableEntity)
			return
		}
		search = r.PostForm.Get("q")
		page, _ = strconv.Atoi(r.PostForm.Get("page"))
		if _, code, err := placementPage(a, r, yearID, classID, search, page); err != nil {
			code, message := academicError(err)
			http.Error(w, message, code)
			return
		} else if code != http.StatusOK {
			http.Error(w, "Pilihan tahun atau kelas tidak ditemukan.", code)
			return
		}
		if _, err = a.options.Academics.CreatePlacement(r.Context(), principal(r).ID, studentID, classID); err != nil {
			p, _, loadErr := placementPage(a, r, yearID, classID, search, page)
			if loadErr != nil {
				a.academicFailure(w, loadErr)
				return
			}
			_, message := academicError(err)
			code, _ := academicError(err)
			a.render(w, r, "page", code, view{Page: "student-placements", Title: "Penempatan Siswa", Error: message, Placement: p})
			return
		}
		query := make(url.Values)
		query.Set("year_id", strconv.FormatInt(yearID, 10))
		query.Set("class_id", strconv.FormatInt(classID, 10))
		query.Set("q", search)
		if page > 0 {
			query.Set("page", strconv.Itoa(page))
		}
		query.Set("saved", "1")
		http.Redirect(w, r, "/admin/placements?"+query.Encode(), http.StatusSeeOther)
		return
	}
	yearID, _ = strconv.ParseInt(r.URL.Query().Get("year_id"), 10, 64)
	classID, _ = strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	search = r.URL.Query().Get("q")
	p, code, err := placementPage(a, r, yearID, classID, search, page)
	if err != nil {
		if code >= 500 {
			a.failure(w, err)
			return
		}
		_, message := academicError(err)
		http.Error(w, message, code)
		return
	}
	v := view{Page: "student-placements", Title: "Penempatan Siswa", Placement: p}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Penempatan awal tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) placementHistory(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	history, err := a.options.Academics.PlacementHistory(r.Context(), principal(r).ID, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	for i := range history {
		history[i].At = history[i].At.In(time.FixedZone("WIB", 7*3600))
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "placement-history", Title: "Histori Penempatan", PlacementHistory: history})
}

func (a *application) academicClassForm(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	id := int64(0)
	var err error
	if r.PathValue("id") != "" {
		id, err = parseAcademicID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	classes, years, err := a.options.Academics.Classes(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	av := &academicView{Classes: classes, Years: years, Editing: id}
	if id != 0 {
		av.Class, err = a.options.Academics.Class(r.Context(), actor, id)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
	}
	counts, err := a.options.Academics.PlacementCounts(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	av.PlacementCounts = counts
	v := view{Page: "academic-classes", Title: "Kelas", Academic: av, ModalOpen: true}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		yearID, yearErr := strconv.ParseInt(r.PostForm.Get("school_year_id"), 10, 64)
		grade, gradeErr := strconv.Atoi(r.PostForm.Get("grade"))
		version, versionErr := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
		if yearErr != nil || gradeErr != nil || versionErr != nil {
			av.Class.Name = r.PostForm.Get("name")
			v.Error = "Tingkat, tahun ajaran, atau versi data tidak valid."
			a.render(w, r, "page", http.StatusUnprocessableEntity, v)
			return
		}
		className, reason := r.PostForm.Get("name"), r.PostForm.Get("reason")
		av.Class.Name = className
		av.Class.Grade = grade
		av.Class.SchoolYearID = yearID
		if id == 0 {
			for _, year := range years {
				if year.ID == yearID {
					av.Class.YearName = year.Name
					break
				}
			}
		}
		submitted := av.Class
		av.Class, err = a.options.Academics.SaveClass(r.Context(), actor, id, yearID, grade, className, reason, version)
		if err == nil {
			http.Redirect(w, r, "/akademik/kelas?saved=1", http.StatusSeeOther)
			return
		}
		av.Class = submitted
		code, message := academicError(err)
		v.Error = message
		a.render(w, r, "page", code, v)
		return
	}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) academicHistory(w http.ResponseWriter, r *http.Request) {
	id, err := parseAcademicID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	entity := r.PathValue("entity")
	history, err := a.options.Academics.History(r.Context(), principal(r).ID, entity, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	for i := range history {
		history[i].At = history[i].At.In(time.FixedZone("WIB", 7*3600))
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "academic-history", Title: "Histori Akademik", Academic: &academicView{History: history, Class: academic.Class{ID: id}, Editing: id, HistoryEntity: entity}})
}

func parseAcademicID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid academic ID")
	}
	return id, nil
}
