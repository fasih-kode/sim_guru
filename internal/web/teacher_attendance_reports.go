package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type teacherReportForm struct {
	Kind, StartsOn, EndsOn, Description, Key, Reason, RequestText string
	Version                                                       int64
}

type teacherReportsView struct {
	PreviousURL, NextURL                                      string
	Base                                                      string
	Own, CanCreate                                            bool
	Filter                                                    academic.TeacherAttendanceReportFilter
	Reports                                                   []academic.TeacherAttendanceReport
	Record                                                    *academic.TeacherAttendanceReport
	Form                                                      teacherReportForm
	Modal                                                     string
	Previous, Next, HistoryPage, HistoryPrevious, HistoryNext int
}

func teacherReportRoute(r *http.Request) (string, academic.TeacherReportScope) {
	if strings.HasPrefix(r.URL.Path, "/admin/kehadiran-tu") {
		return "/admin/kehadiran-tu/laporan", academic.TeacherReportAdmin
	}
	if strings.HasPrefix(r.URL.Path, "/tu/kehadiran-guru") {
		return "/tu/kehadiran-guru/laporan", academic.TeacherReportTU
	}
	return "/kehadiran-saya/laporan", academic.TeacherReportOwn
}

func reportPage(raw string) int {
	p, _ := strconv.Atoi(raw)
	if p < 1 {
		return 1
	}
	return p
}

func (a *application) loadTeacherReports(r *http.Request) (*teacherReportsView, error) {
	base, scope := teacherReportRoute(r)
	q := r.URL.Query()
	f := academic.TeacherAttendanceReportFilter{Scope: scope, Search: strings.TrimSpace(q.Get("q")), StartsOn: q.Get("from"), EndsOn: q.Get("to"), Kind: q.Get("kind"), Status: q.Get("status"), Page: reportPage(q.Get("page"))}
	list, e := a.options.Academics.TeacherAttendanceReports(r.Context(), principal(r).ID, f)
	if e != nil {
		return nil, e
	}
	v := &teacherReportsView{Base: base, Own: scope == academic.TeacherReportOwn, Filter: f, Reports: list}
	if f.Page > 1 {
		v.Previous = f.Page - 1
	}
	if len(list) > 10 {
		v.Next = f.Page + 1
		v.Reports = list[:10]
	}
	if v.Own {
		_, e = a.options.Academics.TeacherAttendanceToday(r.Context(), principal(r).ID)
		if e == nil {
			v.CanCreate = true
		} else if !errors.Is(e, academic.ErrForbidden) {
			return nil, e
		}
	}
	if v.Previous > 0 {
		v.PreviousURL = teacherReportListURL(v, v.Previous)
	}
	if v.Next > 0 {
		v.NextURL = teacherReportListURL(v, v.Next)
	}
	return v, nil
}

func (a *application) teacherReports(w http.ResponseWriter, r *http.Request) {
	v, e := a.loadTeacherReports(r)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	if r.URL.Query().Get("new") == "1" {
		if !v.Own || !v.CanCreate {
			a.academicFailure(w, academic.ErrForbidden)
			return
		}
		key, e := academic.NewTeacherAttendanceIdempotencyKey()
		if e != nil {
			a.failure(w, e)
			return
		}
		date := academic.TeacherAttendanceWIBDate(time.Now())
		v.Modal = "create"
		v.Form = teacherReportForm{StartsOn: date, EndsOn: date, Key: key}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "teacher-reports", Title: "Laporan Kehadiran Guru", TeacherReports: v})
}

func reportForm(r *http.Request) teacherReportForm {
	version, _ := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
	return teacherReportForm{Kind: r.PostForm.Get("kind"), StartsOn: r.PostForm.Get("starts_on"), EndsOn: r.PostForm.Get("ends_on"), Description: r.PostForm.Get("description"), Key: r.PostForm.Get("idempotency_key"), Reason: r.PostForm.Get("reason"), RequestText: r.PostForm.Get("request_text"), Version: version}
}

func (a *application) createTeacherReport(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	f := reportForm(r)
	record, e := a.options.Academics.CreateTeacherAttendanceReport(r.Context(), principal(r).ID, academic.TeacherAttendanceReportInput{Kind: f.Kind, StartsOn: f.StartsOn, EndsOn: f.EndsOn, Description: f.Description, IdempotencyKey: f.Key})
	if e == nil {
		http.Redirect(w, r, "/kehadiran-saya/laporan/"+strconv.FormatInt(record.ID, 10)+"?saved=1", http.StatusSeeOther)
		return
	}
	if errors.Is(e, academic.ErrForbidden) {
		a.academicFailure(w, e)
		return
	}
	v, loadErr := a.loadTeacherReports(r)
	if loadErr != nil {
		a.academicFailure(w, loadErr)
		return
	}
	v.Modal = "create"
	v.Form = f
	status, message := teacherReportError(e)
	a.render(w, r, "page", status, view{Page: "teacher-reports", Title: "Laporan Kehadiran Guru", TeacherReports: v, Error: message, ModalOpen: true})
}

func (a *application) loadTeacherReportDetail(r *http.Request) (*teacherReportsView, error) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		return nil, academic.ErrNotFound
	}
	base, scope := teacherReportRoute(r)
	hp := reportPage(r.URL.Query().Get("history_page"))
	record, e := a.options.Academics.TeacherAttendanceReportDetail(r.Context(), principal(r).ID, id, scope, hp)
	if e != nil {
		return nil, e
	}
	v := &teacherReportsView{Base: base, Own: scope == academic.TeacherReportOwn, Record: &record, HistoryPage: hp}
	if hp > 1 {
		v.HistoryPrevious = hp - 1
	}
	if len(record.History) > 10 {
		v.HistoryNext = hp + 1
		record.History = record.History[:10]
	}
	if v.Own {
		_, e = a.options.Academics.TeacherAttendanceToday(r.Context(), principal(r).ID)
		if e == nil {
			v.CanCreate = true
		} else if !errors.Is(e, academic.ErrForbidden) {
			return nil, e
		}
	}
	v.Form = teacherReportForm{Kind: record.Kind, StartsOn: record.StartsOn, EndsOn: record.EndsOn, Description: record.Description, Version: record.Version}
	return v, nil
}

func (a *application) teacherReportDetail(w http.ResponseWriter, r *http.Request) {
	v, e := a.loadTeacherReportDetail(r)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	modal := r.URL.Query().Get("modal")
	if modal != "" {
		allowed := (modal == "request" && v.Own && v.CanCreate) || ((modal == "correct" || modal == "cancel") && !v.Own && v.Record.CanManage)
		if !allowed {
			a.academicFailure(w, academic.ErrForbidden)
			return
		}
		v.Modal = modal
	}
	message := ""
	if r.URL.Query().Get("saved") == "1" {
		message = "Laporan kehadiran tersimpan."
	}
	if r.URL.Query().Get("requested") == "1" {
		message = "Permintaan dikirim kepada TU. Laporan tidak berubah."
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "teacher-report-detail", Title: "Detail Laporan Kehadiran", TeacherReports: v, Message: message})
}

func (a *application) changeTeacherReport(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	f := reportForm(r)
	cancel := strings.HasSuffix(r.URL.Path, "/batalkan")
	_, e := a.options.Academics.CorrectTeacherAttendanceReport(r.Context(), principal(r).ID, academic.TeacherAttendanceReportCorrection{ReportID: id, ExpectedVersion: f.Version, Kind: f.Kind, StartsOn: f.StartsOn, EndsOn: f.EndsOn, Description: f.Description, Reason: f.Reason, Cancel: cancel})
	base, _ := teacherReportRoute(r)
	if e == nil {
		http.Redirect(w, r, base+"/"+strconv.FormatInt(id, 10)+"?saved=1", http.StatusSeeOther)
		return
	}
	if errors.Is(e, academic.ErrForbidden) || errors.Is(e, academic.ErrNotFound) {
		a.academicFailure(w, e)
		return
	}
	v, loadErr := a.loadTeacherReportDetail(r)
	if loadErr != nil {
		a.academicFailure(w, loadErr)
		return
	}
	v.Form = f
	v.Modal = "correct"
	if cancel {
		v.Modal = "cancel"
	}
	status, message := teacherReportError(e)
	a.render(w, r, "page", status, view{Page: "teacher-report-detail", Title: "Detail Laporan Kehadiran", TeacherReports: v, Error: message, ModalOpen: true})
}

func (a *application) requestTeacherReportCorrection(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	f := reportForm(r)
	e := a.options.Academics.RequestTeacherAttendanceReportCorrection(r.Context(), principal(r).ID, id, f.RequestText)
	if e == nil {
		http.Redirect(w, r, "/kehadiran-saya/laporan/"+strconv.FormatInt(id, 10)+"?requested=1", http.StatusSeeOther)
		return
	}
	if errors.Is(e, academic.ErrForbidden) || errors.Is(e, academic.ErrNotFound) {
		a.academicFailure(w, e)
		return
	}
	v, loadErr := a.loadTeacherReportDetail(r)
	if loadErr != nil {
		a.academicFailure(w, loadErr)
		return
	}
	v.Form = f
	v.Modal = "request"
	status, message := teacherReportError(e)
	a.render(w, r, "page", status, view{Page: "teacher-report-detail", Title: "Detail Laporan Kehadiran", TeacherReports: v, Error: message, ModalOpen: true})
}

func teacherReportError(err error) (int, string) {
	if errors.Is(err, academic.ErrTeacherReportOverlap) {
		return http.StatusConflict, academic.ErrTeacherReportOverlap.Error()
	}
	return teacherAttendanceManagementError(err)
}

func teacherReportListURL(v *teacherReportsView, page int) string {
	q := url.Values{"q": {v.Filter.Search}, "from": {v.Filter.StartsOn}, "to": {v.Filter.EndsOn}, "kind": {v.Filter.Kind}, "status": {v.Filter.Status}, "page": {strconv.Itoa(page)}}
	return v.Base + "?" + q.Encode()
}
