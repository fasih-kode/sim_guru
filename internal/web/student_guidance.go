package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/studentaffairs"
)

type studentGuidanceView struct {
	List          studentaffairs.GuidanceList
	Filter        studentaffairs.GuidanceFilter
	Students      []studentaffairs.GuidanceStudentOption
	Record        studentaffairs.GuidanceRecord
	History       []studentaffairs.GuidanceHistoryEntry
	Input         studentaffairs.GuidanceInput
	ContentChange studentaffairs.GuidanceContentChange
	StatusChange  studentaffairs.GuidanceStatusChange
	StatusError   string
	StatusMessage string
	StatusVersion int64
	CanManage     bool
	ModalOpen     bool
	Editing       bool
	Previous      int
	Next          int
}

func (a *application) studentGuidance(w http.ResponseWriter, r *http.Request) {
	data, status, err := a.studentGuidanceListData(r)
	if err != nil {
		a.guidanceFailure(w, err)
		return
	}
	if r.URL.Query().Get("new") == "1" {
		if !data.CanManage {
			http.Error(w, studentaffairs.ErrGuidanceForbidden.Error(), http.StatusForbidden)
			return
		}
		data.Students, err = a.options.Academics.StudentGuidanceStudents(r.Context(), principal(r).ID)
		if err != nil {
			a.guidanceFailure(w, err)
			return
		}
		data.ModalOpen = true
	}
	page := view{Page: "student-guidance", Title: "Catatan Pembinaan", StudentGuidance: data}
	if r.URL.Query().Get("saved") == "1" {
		page.Message = "Catatan pembinaan dan histori berhasil disimpan."
	}
	a.render(w, r, "page", status, page)
}

func (a *application) studentGuidanceListData(r *http.Request) (*studentGuidanceView, int, error) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	studentID, err := parseOptionalPositive(r.URL.Query().Get("student_id"))
	if err != nil {
		return nil, http.StatusBadRequest, studentaffairs.ValidationError("Filter siswa tidak valid.")
	}
	filter := studentaffairs.GuidanceFilter{Page: page, StudentID: studentID, Search: r.URL.Query().Get("q"), Status: studentaffairs.GuidanceStatus(r.URL.Query().Get("status"))}
	list, err := a.options.Academics.ListStudentGuidance(r.Context(), principal(r).ID, filter)
	if err != nil {
		code, _ := guidanceHTTPError(err)
		return nil, code, err
	}
	u := principal(r)
	data := &studentGuidanceView{List: list, Filter: filter, CanManage: u.HasRole(studentaffairs.GuidanceRoleKesiswaan)}
	if filter.Page > 1 {
		data.Previous = filter.Page - 1
	}
	if list.HasNext {
		data.Next = filter.Page + 1
	}
	return data, http.StatusOK, nil
}

func (a *application) studentGuidanceDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	data, err := a.studentGuidanceDetailData(r.Context(), principal(r).ID, id, principal(r).HasRole(studentaffairs.GuidanceRoleKesiswaan))
	if err != nil {
		a.guidanceFailure(w, err)
		return
	}
	data.Editing = r.URL.Query().Get("edit") == "1"
	if data.Editing && data.CanManage {
		data.ModalOpen = true
		data.ContentChange = studentaffairs.GuidanceContentChange{Incident: data.Record.Incident, Action: data.Record.Action}
	}
	page := view{Page: "student-guidance-detail", Title: "Detail Catatan Pembinaan", StudentGuidance: data}
	if r.URL.Query().Get("saved") == "1" {
		page.Message = "Status catatan berhasil diubah dan histori diperbarui."
	}
	a.render(w, r, "page", http.StatusOK, page)
}

func (a *application) studentGuidanceDetailData(ctx context.Context, actor, id int64, canManage bool) (*studentGuidanceView, error) {
	record, err := a.options.Academics.GetStudentGuidance(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	history, err := a.options.Academics.StudentGuidanceHistory(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return &studentGuidanceView{
		Record: record, History: history,
		CanManage:     canManage,
		StatusVersion: record.Version,
	}, nil
}

func (a *application) createStudentGuidance(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	input := studentaffairs.GuidanceInput{
		StudentID: formInt64(r, "student_id"), Date: r.PostForm.Get("occurred_on"),
		Incident: r.PostForm.Get("incident"), Action: r.PostForm.Get("action"), Status: studentaffairs.GuidanceStatus(r.PostForm.Get("status")),
	}
	_, err := a.options.Academics.CreateStudentGuidance(r.Context(), principal(r).ID, input)
	if err == nil {
		http.Redirect(w, r, "/kesiswaan/pembinaan?saved=1", http.StatusSeeOther)
		return
	}
	data, _, listErr := a.studentGuidanceListData(r)
	if listErr != nil {
		a.guidanceFailure(w, listErr)
		return
	}
	data.Input, data.ModalOpen = input, true
	data.Students, _ = a.options.Academics.StudentGuidanceStudents(r.Context(), principal(r).ID)
	code, message := guidanceHTTPError(err)
	page := view{Page: "student-guidance", Title: "Catatan Pembinaan", StudentGuidance: data, Error: message, ModalOpen: true}
	a.render(w, r, "page", code, page)
}

func (a *application) updateStudentGuidance(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	id, ok := guidancePathID(w, r)
	if !ok {
		return
	}
	change := studentaffairs.GuidanceContentChange{Incident: r.PostForm.Get("incident"), Action: r.PostForm.Get("action"), Reason: r.PostForm.Get("reason")}
	_, err := a.options.Academics.UpdateStudentGuidance(r.Context(), principal(r).ID, id, formInt64(r, "version"), change)
	if err == nil {
		http.Redirect(w, r, "/kesiswaan/pembinaan/"+strconv.FormatInt(id, 10)+"?saved=1", http.StatusSeeOther)
		return
	}
	record, readErr := a.options.Academics.GetStudentGuidance(r.Context(), principal(r).ID, id)
	if readErr != nil {
		a.guidanceFailure(w, readErr)
		return
	}
	history, _ := a.options.Academics.StudentGuidanceHistory(r.Context(), principal(r).ID, id)
	data := &studentGuidanceView{Record: record, History: history, CanManage: principal(r).HasRole(studentaffairs.GuidanceRoleKesiswaan), ModalOpen: true, Editing: true, ContentChange: change}
	code, message := guidanceHTTPError(err)
	a.render(w, r, "page", code, view{Page: "student-guidance-detail", Title: "Detail Catatan Pembinaan", StudentGuidance: data, Error: message, ModalOpen: true})
}

func (a *application) changeStudentGuidanceStatus(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	id, ok := guidancePathID(w, r)
	if !ok {
		return
	}
	change := studentaffairs.GuidanceStatusChange{Status: studentaffairs.GuidanceStatus(r.PostForm.Get("status")), Reason: r.PostForm.Get("reason")}
	_, err := a.options.Academics.ChangeStudentGuidanceStatus(r.Context(), principal(r).ID, id, formInt64(r, "version"), change)
	if err != nil {
		data, readErr := a.studentGuidanceDetailData(r.Context(), principal(r).ID, id, principal(r).HasRole(studentaffairs.GuidanceRoleKesiswaan))
		if readErr != nil {
			a.guidanceFailure(w, readErr)
			return
		}
		_, message := guidanceHTTPError(err)
		data.StatusChange = change
		data.StatusVersion = data.Record.Version
		data.StatusError = message
		a.renderStudentGuidanceDetail(w, r, guidanceHTTPStatus(err), data)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		data, readErr := a.studentGuidanceDetailData(r.Context(), principal(r).ID, id, principal(r).HasRole(studentaffairs.GuidanceRoleKesiswaan))
		if readErr != nil {
			a.guidanceFailure(w, readErr)
			return
		}
		data.StatusVersion = data.Record.Version
		data.StatusMessage = "Status catatan berhasil diubah dan histori diperbarui."
		a.renderStudentGuidanceDetail(w, r, http.StatusOK, data)
		return
	}
	http.Redirect(w, r, "/kesiswaan/pembinaan/"+strconv.FormatInt(id, 10)+"?saved=1", http.StatusSeeOther)
}

func (a *application) renderStudentGuidanceDetail(w http.ResponseWriter, r *http.Request, status int, data *studentGuidanceView) {
	page := view{Page: "student-guidance-detail", Title: "Detail Catatan Pembinaan", StudentGuidance: data}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Add("Vary", "HX-Request")
		a.render(w, r, "student-guidance-detail", status, page)
		return
	}
	a.render(w, r, "page", status, page)
}

func guidanceHTTPStatus(err error) int {
	status, _ := guidanceHTTPError(err)
	return status
}

func guidancePathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

func parseOptionalPositive(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("invalid positive ID")
	}
	return n, nil
}

func guidanceHTTPError(err error) (int, string) {
	var domainValidation studentaffairs.ValidationError
	var academicValidation academic.ValidationError
	switch {
	case errors.As(err, &domainValidation), errors.As(err, &academicValidation):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, studentaffairs.ErrGuidancePlacementUnclear):
		return http.StatusUnprocessableEntity, "Penempatan siswa pada tanggal kejadian tidak dapat dipastikan. Periksa kalender dan riwayat penempatan terlebih dahulu."
	case errors.Is(err, studentaffairs.ErrGuidanceForbidden), errors.Is(err, academic.ErrForbidden):
		return http.StatusForbidden, "Anda tidak memiliki akses ke catatan pembinaan ini."
	case errors.Is(err, studentaffairs.ErrGuidanceNotFound), errors.Is(err, academic.ErrNotFound):
		return http.StatusNotFound, "Catatan pembinaan tidak ditemukan."
	case errors.Is(err, studentaffairs.ErrGuidanceConflict), errors.Is(err, academic.ErrConflict):
		return http.StatusConflict, "Catatan pembinaan berubah. Muat ulang sebelum menyimpan kembali."
	default:
		return http.StatusInternalServerError, "Catatan pembinaan belum dapat diproses. Silakan coba kembali."
	}
}

func (a *application) guidanceFailure(w http.ResponseWriter, err error) {
	code, message := guidanceHTTPError(err)
	http.Error(w, message, code)
}
