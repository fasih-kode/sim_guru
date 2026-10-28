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

type teacherAttendanceManagementView struct {
	RequestPage, RequestPrevious, RequestNext int
	RequestPreviousURL, RequestNextURL        string
	Date, Search, Message, BasePath           string
	Admin                                     bool
	Page, Previous, Next                      int
	Teachers                                  []academic.TeacherAttendanceManagedTeacher
	Requests                                  []academic.TeacherAttendanceCorrectionRequest
	Manual                                    *teacherAttendanceManualForm
}

type teacherAttendanceManualForm struct {
	TeacherID                          int64
	TeacherName                        string
	Action                             academic.TeacherAttendanceAction
	OccurredAt, Reason, IdempotencyKey string
}

type teacherAttendanceRecordView struct {
	Record                                     academic.TeacherAttendanceManagedRecord
	OccurredAt, ManualReason, CorrectionReason string
	BasePath                                   string
	ReturnSearch                               string
	Admin                                      bool
	Version                                    int64
}

func (a *application) teacherAttendanceManagement(w http.ResponseWriter, r *http.Request) {
	basePath := teacherAttendanceManagementBase(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = academic.TeacherAttendanceWIBTime(time.Now()).Format("2006-01-02")
	}
	data, err := a.loadTeacherAttendanceManagement(r, date, strings.TrimSpace(r.URL.Query().Get("q")), page)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.BasePath, data.Admin = basePath, basePath == "/admin/kehadiran-tu"
	if r.URL.Query().Get("saved") == "manual" {
		data.Message = "Catatan kehadiran guru tersimpan."
	}
	if rawID, rawAction := r.URL.Query().Get("teacher_id"), r.URL.Query().Get("action"); rawID != "" && rawAction != "" {
		id, parseErr := strconv.ParseInt(rawID, 10, 64)
		if parseErr != nil || id < 1 || !validTeacherAttendanceAction(academic.TeacherAttendanceAction(rawAction)) {
			http.NotFound(w, r)
			return
		}
		for _, teacher := range data.Teachers {
			if teacher.ID == id {
				if !teacher.CanManage {
					break
				}
				key, keyErr := academic.NewTeacherAttendanceIdempotencyKey()
				if keyErr != nil {
					a.failure(w, keyErr)
					return
				}
				data.Manual = &teacherAttendanceManualForm{TeacherID: id, TeacherName: teacher.Name, Action: academic.TeacherAttendanceAction(rawAction), OccurredAt: academic.TeacherAttendanceWIBTime(time.Now()).Format("2006-01-02T15:04"), IdempotencyKey: key}
				break
			}
		}
		if data.Manual == nil {
			http.NotFound(w, r)
			return
		}
	}
	v := view{Page: "teacher-attendance-management", Title: "Kelola Kehadiran Guru", TeacherAttendanceManagement: data}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) loadTeacherAttendanceManagement(r *http.Request, date, search string, page int) (*teacherAttendanceManagementView, error) {
	var teachers []academic.TeacherAttendanceManagedTeacher
	var requests []academic.TeacherAttendanceCorrectionRequest
	var err error
	if teacherAttendanceManagementBase(r) == "/admin/kehadiran-tu" {
		teachers, err = a.options.Academics.TeacherAttendanceManagedForAdmin(r.Context(), principal(r).ID, date, search, page)
	} else {
		teachers, err = a.options.Academics.TeacherAttendanceManaged(r.Context(), principal(r).ID, date, search, page)
		if err == nil {
			requests, err = a.options.Academics.TeacherAttendanceCorrectionRequests(r.Context(), principal(r).ID, reportPage(r.URL.Query().Get("request_page")))
		}
	}
	if err != nil {
		return nil, err
	}
	data := &teacherAttendanceManagementView{Date: date, Search: search, Page: page, Teachers: teachers, Requests: requests}
	if page > 1 {
		data.Previous = page - 1
	}
	if len(teachers) > 10 {
		data.Next = page + 1
		data.Teachers = teachers[:10]
	}
	data.RequestPage = reportPage(r.URL.Query().Get("request_page"))
	if data.RequestPage > 1 {
		data.RequestPrevious = data.RequestPage - 1
	}
	if len(requests) > 10 {
		data.Requests = requests[:10]
		data.RequestNext = data.RequestPage + 1
	}
	requestURL := func(page int) string {
		q := r.URL.Query()
		q.Set("request_page", strconv.Itoa(page))
		return teacherAttendanceManagementBase(r) + "?" + q.Encode()
	}
	if data.RequestPrevious > 0 {
		data.RequestPreviousURL = requestURL(data.RequestPrevious)
	}
	if data.RequestNext > 0 {
		data.RequestNextURL = requestURL(data.RequestNext)
	}
	return data, nil
}

func (a *application) recordTeacherAttendanceManual(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	form := teacherAttendanceManualForm{TeacherName: r.PostForm.Get("teacher_name"), OccurredAt: r.PostForm.Get("occurred_at"), Reason: r.PostForm.Get("reason"), IdempotencyKey: r.PostForm.Get("idempotency_key"), Action: academic.TeacherAttendanceAction(r.PostForm.Get("action"))}
	form.TeacherID, _ = strconv.ParseInt(r.PostForm.Get("teacher_id"), 10, 64)
	date := r.PostForm.Get("return_date")
	search := r.PostForm.Get("return_q")
	page, _ := strconv.Atoi(r.PostForm.Get("return_page"))
	if page < 1 {
		page = 1
	}
	_, err := a.options.Academics.RecordTeacherAttendanceManual(r.Context(), principal(r).ID, academic.TeacherAttendanceManualInput{TeacherID: form.TeacherID, Action: form.Action, OccurredAt: form.OccurredAt, Reason: form.Reason, IdempotencyKey: form.IdempotencyKey})
	if err == nil {
		http.Redirect(w, r, teacherAttendanceManagementURL(teacherAttendanceManagementBase(r), date, search, page, "manual"), http.StatusSeeOther)
		return
	}
	data, loadErr := a.loadTeacherAttendanceManagement(r, date, search, page)
	if loadErr != nil {
		a.academicFailure(w, loadErr)
		return
	}
	data.Manual = &form
	data.BasePath, data.Admin = teacherAttendanceManagementBase(r), teacherAttendanceManagementBase(r) == "/admin/kehadiran-tu"
	status, message := teacherAttendanceManagementError(err)
	v := view{Page: "teacher-attendance-management", Title: "Kelola Kehadiran Guru", TeacherAttendanceManagement: data, Error: message, ModalOpen: true}
	a.render(w, r, "page", status, v)
}

func (a *application) teacherAttendanceManagedRecord(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	var record academic.TeacherAttendanceManagedRecord
	if teacherAttendanceManagementBase(r) == "/admin/kehadiran-tu" {
		record, err = a.options.Academics.TeacherAttendanceManagedRecordDetailForAdmin(r.Context(), principal(r).ID, id)
	} else {
		record, err = a.options.Academics.TeacherAttendanceManagedRecordDetail(r.Context(), principal(r).ID, id)
	}
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data := &teacherAttendanceRecordView{Record: record, OccurredAt: record.OccurredAt.Format("2006-01-02T15:04"), ManualReason: record.ManualReason, Version: record.Version, BasePath: teacherAttendanceManagementBase(r), Admin: teacherAttendanceManagementBase(r) == "/admin/kehadiran-tu"}
	v := view{Page: "teacher-attendance-record", Title: "Detail Kehadiran Guru", TeacherAttendanceRecord: data}
	a.render(w, r, "page", http.StatusOK, v)
}

func (a *application) correctManagedTeacherAttendance(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	version, _ := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
	var manualReason *string
	if value, ok := r.PostForm["manual_reason"]; ok && len(value) > 0 {
		manualReason = &value[0]
	}
	input := academic.TeacherAttendanceCorrectionInput{EventID: id, ExpectedVersion: version, OccurredAt: r.PostForm.Get("occurred_at"), ManualReason: manualReason, CorrectionReason: r.PostForm.Get("correction_reason")}
	if _, err = a.options.Academics.CorrectTeacherAttendance(r.Context(), principal(r).ID, input); err == nil {
		http.Redirect(w, r, teacherAttendanceManagementURL(teacherAttendanceManagementBase(r), r.PostForm.Get("return_date"), r.PostForm.Get("return_q"), 1, "corrected"), http.StatusSeeOther)
		return
	}
	var record academic.TeacherAttendanceManagedRecord
	var loadErr error
	if teacherAttendanceManagementBase(r) == "/admin/kehadiran-tu" {
		record, loadErr = a.options.Academics.TeacherAttendanceManagedRecordDetailForAdmin(r.Context(), principal(r).ID, id)
	} else {
		record, loadErr = a.options.Academics.TeacherAttendanceManagedRecordDetail(r.Context(), principal(r).ID, id)
	}
	if loadErr != nil {
		a.academicFailure(w, loadErr)
		return
	}
	status, message := teacherAttendanceManagementError(err)
	data := &teacherAttendanceRecordView{Record: record, OccurredAt: input.OccurredAt, CorrectionReason: input.CorrectionReason, Version: version, BasePath: teacherAttendanceManagementBase(r), Admin: teacherAttendanceManagementBase(r) == "/admin/kehadiran-tu", ReturnSearch: r.PostForm.Get("return_q")}
	if manualReason != nil {
		data.ManualReason = *manualReason
	} else {
		data.ManualReason = record.ManualReason
	}
	v := view{Page: "teacher-attendance-record", Title: "Detail Kehadiran Guru", TeacherAttendanceRecord: data, Error: message, ModalOpen: true}
	a.render(w, r, "page", status, v)
}

func (a *application) teacherAttendanceRequest(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	input := academic.TeacherAttendanceCorrectionRequestInput{OnDate: r.PostForm.Get("on_date"), Action: academic.TeacherAttendanceAction(r.PostForm.Get("action")), Kind: r.PostForm.Get("kind"), RequestText: r.PostForm.Get("request_text")}
	err := a.options.Academics.RequestTeacherAttendanceCorrection(r.Context(), principal(r).ID, input)
	if err == nil {
		http.Redirect(w, r, "/kehadiran-saya?request_saved=1", http.StatusSeeOther)
		return
	}
	status, message := teacherAttendanceManagementError(err)
	a.renderTeacherAttendancePage(w, r, status, message, &input, true)
}

func (a *application) renderTeacherAttendancePage(w http.ResponseWriter, r *http.Request, status int, message string, request *academic.TeacherAttendanceCorrectionRequestInput, open bool) {
	day, err := a.options.Academics.TeacherAttendanceToday(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	f := teacherAttendanceView{Day: day, Available: a.options.Academics.Photos != nil, RequestOpen: open, RequestDate: day.Date, RequestKind: "correction", RequestAction: academic.TeacherCheckIn}
	if request != nil {
		f.RequestDate = request.OnDate
		f.RequestKind = request.Kind
		f.RequestAction = request.Action
		f.RequestText = request.RequestText
	}
	for i := range day.Events {
		event := &day.Events[i]
		if event.Action == academic.TeacherCheckIn {
			f.CheckIn = event
		} else {
			f.CheckOut = event
		}
	}
	page := view{Page: "teacher-attendance", Title: "Kehadiran Saya", TeacherAttendance: &f, Error: message, Message: map[bool]string{true: "Permintaan Anda terkirim kepada TU. Catatan kehadiran tidak berubah.", false: ""}[r.URL.Query().Get("request_saved") == "1"]}
	a.render(w, r, "page", status, page)
}

func teacherAttendanceManagementError(err error) (int, string) {
	var validation academic.ValidationError
	switch {
	case errors.Is(err, academic.ErrForbidden):
		return http.StatusForbidden, "Anda tidak memiliki kewenangan mengelola kehadiran guru."
	case errors.Is(err, academic.ErrNotFound):
		return http.StatusNotFound, "Catatan kehadiran tidak ditemukan."
	case errors.Is(err, academic.ErrConflict):
		return http.StatusConflict, "Catatan berubah. Muat ulang dan periksa kembali sebelum menyimpan."
	case errors.As(err, &validation):
		return http.StatusUnprocessableEntity, string(validation)
	default:
		return http.StatusInternalServerError, "Permintaan kehadiran belum dapat diproses."
	}
}

func validTeacherAttendanceAction(action academic.TeacherAttendanceAction) bool {
	return action == academic.TeacherCheckIn || action == academic.TeacherCheckOut
}
func teacherAttendanceManagementURL(basePath, date, search string, page int, saved string) string {
	values := url.Values{}
	values.Set("date", date)
	if search != "" {
		values.Set("q", search)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if saved != "" {
		values.Set("saved", saved)
	}
	if basePath != "/admin/kehadiran-tu" {
		basePath = "/tu/kehadiran-guru"
	}
	return basePath + "?" + values.Encode()
}

func teacherAttendanceManagementBase(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/admin/kehadiran-tu") {
		return "/admin/kehadiran-tu"
	}
	return "/tu/kehadiran-guru"
}
