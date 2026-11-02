package web

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type teachingJournalForm struct {
	LogicalLessonID                         int64
	Date, RequestKey                        string
	ExpectedVersion                         int
	State, Outcome, Material, ActivityNotes string
	NotHeldReason, CorrectionReason         string
}

type teachingJournalView struct {
	BasePath          string
	Filter            academic.TeachingJournalReadFilter
	Page              academic.TeachingJournalPage
	FilterOptions     academic.TeachingJournalFilterOptions
	Detail            *academic.TeachingJournalDetail
	Selected          *academic.TeachingJournalVersionRecord
	History           academic.TeachingJournalHistory
	Occurrences       []academic.TeachingJournalOccurrenceOption
	Occurrence        *academic.TeachingJournalOccurrenceOption
	Form              *teachingJournalForm
	PreviousURL       string
	NextURL           string
	PDFExportURL      string
	XLSXExportURL     string
	PendingLineage    bool
	CanEdit           bool
	AttendanceBlocked string
}

func parseJournalFilter(r *http.Request) (academic.TeachingJournalReadFilter, error) {
	parseID := func(name string) (int64, error) {
		raw := r.URL.Query().Get(name)
		if raw == "" {
			return 0, nil
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			return 0, academic.ValidationError("Filter jurnal tidak valid.")
		}
		return id, nil
	}
	semesterID, err := parseID("semester_id")
	if err != nil {
		return academic.TeachingJournalReadFilter{}, err
	}
	classID, err := parseID("class_id")
	if err != nil {
		return academic.TeachingJournalReadFilter{}, err
	}
	subjectID, err := parseID("subject_id")
	if err != nil {
		return academic.TeachingJournalReadFilter{}, err
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil {
			return academic.TeachingJournalReadFilter{}, academic.ValidationError("Nomor halaman tidak valid.")
		}
	}
	filter := academic.BuildTeachingJournalReadFilter(page, semesterID, classID, subjectID,
		r.URL.Query().Get("from"), r.URL.Query().Get("to"), r.URL.Query().Get("state"), r.URL.Query().Get("outcome"))
	filter.Search = strings.TrimSpace(r.URL.Query().Get("q"))
	return filter, nil
}

func journalPageURL(filter academic.TeachingJournalReadFilter, page int) string {
	values := url.Values{}
	if filter.SemesterID > 0 {
		values.Set("semester_id", strconv.FormatInt(filter.SemesterID, 10))
	}
	if filter.ClassID > 0 {
		values.Set("class_id", strconv.FormatInt(filter.ClassID, 10))
	}
	if filter.SubjectID > 0 {
		values.Set("subject_id", strconv.FormatInt(filter.SubjectID, 10))
	}
	if filter.FromDate != "" {
		values.Set("from", filter.FromDate)
	}
	if filter.ToDate != "" {
		values.Set("to", filter.ToDate)
	}
	if filter.State != "" {
		values.Set("state", filter.State)
	}
	if filter.Outcome != "" {
		values.Set("outcome", filter.Outcome)
	}
	if filter.Search != "" {
		values.Set("q", filter.Search)
	}
	values.Set("page", strconv.Itoa(page))
	return "/jurnal-saya?" + values.Encode()
}

func journalRequestKey() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func (a *application) teachingJournalList(w http.ResponseWriter, r *http.Request) {
	filter, err := parseJournalFilter(r)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	page, err := a.options.Academics.TeachingJournals(r.Context(), principal(r).ID, filter)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	filterOptions, err := a.options.Academics.TeachingJournalFilterOptions(r.Context(), principal(r).ID)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	v := &teachingJournalView{BasePath: "/jurnal-saya", Filter: filter, Page: page, FilterOptions: filterOptions}
	v.PDFExportURL = journalExportURL(filter, "pdf")
	v.XLSXExportURL = journalExportURL(filter, "xlsx")
	if filter.Page > 1 {
		v.PreviousURL = journalPageURL(filter, filter.Page-1)
	}
	if page.HasNext {
		v.NextURL = journalPageURL(filter, filter.Page+1)
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journals", Title: "Jurnal Mengajar Saya", TeachingJournal: v})
}

func (a *application) teachingJournalDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseJournalPathID(r)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	actor := principal(r).ID
	detail, err := a.options.Academics.TeachingJournalDetail(r.Context(), actor, id)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	history, err := a.options.Academics.TeachingJournalVersions(r.Context(), actor, id)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journal-detail", Title: "Detail Jurnal Mengajar", TeachingJournal: &teachingJournalView{BasePath: "/jurnal-saya", Detail: &detail, Selected: &detail.CurrentVersion, History: history, CanEdit: true}})
}

func (a *application) teachingJournalVersion(w http.ResponseWriter, r *http.Request) {
	id, err := parseJournalPathID(r)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	versionNo, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || versionNo < 1 {
		a.journalFailure(w, r, academic.ValidationError("Nomor versi tidak valid."))
		return
	}
	actor := principal(r).ID
	detail, err := a.options.Academics.TeachingJournalDetail(r.Context(), actor, id)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	selected, err := a.options.Academics.TeachingJournalVersionDetail(r.Context(), actor, id, versionNo)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	history, err := a.options.Academics.TeachingJournalVersions(r.Context(), actor, id)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journal-version", Title: "Versi Jurnal Mengajar", TeachingJournal: &teachingJournalView{BasePath: "/jurnal-saya", Detail: &detail, Selected: &selected, History: history}})
}

func parseJournalPathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, academic.ValidationError("ID jurnal tidak valid.")
	}
	return id, nil
}

func (a *application) teachingJournalNew(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	}
	options, pending, err := a.options.Academics.TeachingJournalOccurrences(r.Context(), principal(r).ID, date)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	form := &teachingJournalForm{Date: date, State: "draft", Outcome: "held"}
	form.RequestKey, err = journalRequestKey()
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	v := &teachingJournalView{BasePath: "/jurnal-saya", Occurrences: options, Form: form, PendingLineage: pending}
	if raw := r.URL.Query().Get("lesson_id"); raw != "" {
		lessonID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || lessonID < 1 {
			a.journalFailure(w, r, academic.ValidationError("Pertemuan tidak valid."))
			return
		}
		for i := range options {
			if options[i].LogicalLessonID == lessonID {
				v.Occurrence = &options[i]
				v.AttendanceBlocked = options[i].AttendanceReason
				form.LogicalLessonID = lessonID
				break
			}
		}
		if v.Occurrence == nil {
			a.journalFailure(w, r, academic.ErrNotFound)
			return
		}
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journal-form", Title: "Isi Jurnal Mengajar", TeachingJournal: v})
}

func formFromVersion(detail academic.TeachingJournalDetail, key string) *teachingJournalForm {
	return &teachingJournalForm{LogicalLessonID: detail.LogicalLessonID, Date: detail.MeetingDate, RequestKey: key,
		ExpectedVersion: detail.CurrentVersion.Number, State: detail.CurrentVersion.State, Outcome: detail.CurrentVersion.Outcome,
		Material: detail.CurrentVersion.Material, ActivityNotes: detail.CurrentVersion.ActivityNotes,
		NotHeldReason: detail.CurrentVersion.NotHeldReason}
}

func journalEditTitle(state string) string {
	if state == "final" {
		return "Koreksi Jurnal Final"
	}
	return "Lanjutkan Draf"
}

func (a *application) teachingJournalEdit(w http.ResponseWriter, r *http.Request) {
	id, err := parseJournalPathID(r)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	detail, err := a.options.Academics.TeachingJournalDetail(r.Context(), principal(r).ID, id)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	key, err := journalRequestKey()
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	history, err := a.options.Academics.TeachingJournalVersions(r.Context(), principal(r).ID, id)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	eligibility, err := a.options.Academics.TeacherLessonEligibilityForDate(r.Context(), principal(r).ID, detail.MeetingDate, journalPeriodEnd(detail.CurrentVersion.Periods))
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	blocked := ""
	if !eligibility.Allowed {
		blocked = eligibility.Reason
	}
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journal-form", Title: journalEditTitle(detail.CurrentVersion.State), TeachingJournal: &teachingJournalView{BasePath: "/jurnal-saya", Detail: &detail, Selected: &detail.CurrentVersion, History: history, Form: formFromVersion(detail, key), Occurrence: &academic.TeachingJournalOccurrenceOption{LogicalLessonID: detail.LogicalLessonID, SemesterID: detail.SemesterID, RevisionID: detail.RevisionID, EntryID: detail.EntryID, Date: detail.MeetingDate, SemesterSnapshot: detail.SemesterSnapshot, RevisionNumber: detail.RevisionNumber, ClassSnapshot: detail.ClassSnapshot, SubjectSnapshot: detail.SubjectSnapshot, TeacherSnapshot: detail.TeacherSnapshot, Periods: detail.CurrentVersion.Periods}, CanEdit: true, AttendanceBlocked: blocked}})
}

func journalPeriodEnd(periods []academic.TeachingJournalPeriodSnapshot) string {
	if len(periods) == 0 {
		return ""
	}
	return periods[len(periods)-1].EndsAt
}

func (a *application) teachingJournalSave(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	if strings.TrimSpace(r.PostForm.Get("actual_start")) != "" || strings.TrimSpace(r.PostForm.Get("actual_end")) != "" {
		a.journalFailure(w, r, academic.ValidationError("Nomor dan waktu JP mengikuti snapshot jadwal resmi; waktu aktual tidak dapat diinput."))
		return
	}
	action := r.PostForm.Get("action")
	state := ""
	switch action {
	case "save_draft":
		state = "draft"
	case "finalize":
		state = "final"
	default:
		a.journalFailure(w, r, academic.ValidationError("Pilih Simpan Draf atau Finalisasi."))
		return
	}
	logicalID, err := strconv.ParseInt(r.PostForm.Get("logical_lesson_id"), 10, 64)
	if err != nil || logicalID < 1 {
		a.journalFailure(w, r, academic.ValidationError("Pertemuan jurnal tidak valid."))
		return
	}
	date := r.PostForm.Get("meeting_date")
	occurrenceDate, err := time.ParseInLocation("2006-01-02", date, time.FixedZone("WIB", 7*60*60))
	if err != nil || occurrenceDate.Format("2006-01-02") != date {
		a.journalFailure(w, r, academic.ValidationError("Tanggal pertemuan tidak valid."))
		return
	}
	expected, err := strconv.Atoi(r.PostForm.Get("expected_version"))
	if err != nil || expected < 0 {
		a.journalFailure(w, r, academic.ValidationError("Versi jurnal tidak valid."))
		return
	}
	input := academic.TeachingJournalInput{LogicalLessonID: logicalID, OccurrenceDate: occurrenceDate,
		RequestKey: r.PostForm.Get("request_key"), State: state, Outcome: r.PostForm.Get("outcome"),
		Material: r.PostForm.Get("material"), ActivityNotes: r.PostForm.Get("activity_notes"),
		NotHeldReason:    r.PostForm.Get("not_held_reason"),
		CorrectionReason: r.PostForm.Get("correction_reason"), ExpectedVersion: expected}
	var current *academic.TeachingJournalDetail
	if rawID := r.PathValue("id"); rawID != "" {
		id, parseErr := strconv.ParseInt(rawID, 10, 64)
		if parseErr != nil || id < 1 {
			a.journalFailure(w, r, academic.ValidationError("ID jurnal tidak valid."))
			return
		}
		detail, readErr := a.options.Academics.TeachingJournalDetail(r.Context(), principal(r).ID, id)
		if readErr != nil {
			a.journalFailure(w, r, readErr)
			return
		}
		if detail.LogicalLessonID != logicalID || detail.MeetingDate != date {
			a.journalFailure(w, r, academic.ErrConflict)
			return
		}
		current = &detail
	}
	result, err := a.options.Academics.SaveTeachingJournal(r.Context(), principal(r).ID, input)
	if err != nil {
		if errors.Is(err, academic.ErrConflict) && current != nil {
			latest, readErr := a.options.Academics.TeachingJournalDetail(r.Context(), principal(r).ID, current.ID)
			if readErr != nil {
				a.journalFailure(w, r, readErr)
				return
			}
			history, readErr := a.options.Academics.TeachingJournalVersions(r.Context(), principal(r).ID, current.ID)
			if readErr != nil {
				a.journalFailure(w, r, readErr)
				return
			}
			key, _ := journalRequestKey()
			a.render(w, r, "page", http.StatusConflict, view{Page: "teaching-journal-detail", Title: "Jurnal berubah", Error: "Jurnal sudah berubah sejak dibuka. Perubahan Anda belum disimpan; tinjau versi terbaru sebelum mengoreksi.", TeachingJournal: &teachingJournalView{BasePath: "/jurnal-saya", Detail: &latest, Selected: &latest.CurrentVersion, History: history, CanEdit: true, Form: formFromVersion(latest, key)}})
			return
		}
		if current != nil {
			a.renderJournalEditError(w, r, *current, input, err)
			return
		}
		a.renderJournalNewError(w, r, date, input, err)
		return
	}
	http.Redirect(w, r, "/jurnal-saya/"+strconv.FormatInt(result.ID, 10), http.StatusSeeOther)
}

func formFromInput(input academic.TeachingJournalInput) *teachingJournalForm {
	return &teachingJournalForm{LogicalLessonID: input.LogicalLessonID, Date: input.OccurrenceDate.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02"), RequestKey: input.RequestKey, ExpectedVersion: input.ExpectedVersion, State: input.State, Outcome: input.Outcome, Material: input.Material, ActivityNotes: input.ActivityNotes, NotHeldReason: input.NotHeldReason, CorrectionReason: input.CorrectionReason}
}

func (a *application) renderJournalNewError(w http.ResponseWriter, r *http.Request, date string, input academic.TeachingJournalInput, cause error) {
	options, pending, err := a.options.Academics.TeachingJournalOccurrences(r.Context(), principal(r).ID, date)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	var selected *academic.TeachingJournalOccurrenceOption
	for i := range options {
		if options[i].LogicalLessonID == input.LogicalLessonID {
			selected = &options[i]
			break
		}
	}
	code, message := academicError(cause)
	blocked := ""
	if isTeacherAttendanceBlocked(cause) {
		blocked = cause.Error()
	}
	a.render(w, r, "page", code, view{Page: "teaching-journal-form", Title: "Isi Jurnal Mengajar", Error: message, TeachingJournal: &teachingJournalView{BasePath: "/jurnal-saya", Occurrences: options, Occurrence: selected, PendingLineage: pending, Form: formFromInput(input), AttendanceBlocked: blocked}})
}

func (a *application) renderJournalEditError(w http.ResponseWriter, r *http.Request, detail academic.TeachingJournalDetail, input academic.TeachingJournalInput, cause error) {
	latest, err := a.options.Academics.TeachingJournalDetail(r.Context(), principal(r).ID, detail.ID)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	history, err := a.options.Academics.TeachingJournalVersions(r.Context(), principal(r).ID, detail.ID)
	if err != nil {
		a.journalFailure(w, r, err)
		return
	}
	code, message := academicError(cause)
	form := formFromInput(input)
	form.ExpectedVersion = latest.CurrentVersion.Number
	blocked := ""
	if isTeacherAttendanceBlocked(cause) {
		blocked = cause.Error()
	}
	a.render(w, r, "page", code, view{Page: "teaching-journal-form", Title: journalEditTitle(latest.CurrentVersion.State), Error: message, TeachingJournal: &teachingJournalView{BasePath: "/jurnal-saya", Detail: &latest, Selected: &latest.CurrentVersion, History: history, Occurrence: &academic.TeachingJournalOccurrenceOption{LogicalLessonID: latest.LogicalLessonID, SemesterID: latest.SemesterID, RevisionID: latest.RevisionID, EntryID: latest.EntryID, Date: latest.MeetingDate, SemesterSnapshot: latest.SemesterSnapshot, RevisionNumber: latest.RevisionNumber, ClassSnapshot: latest.ClassSnapshot, SubjectSnapshot: latest.SubjectSnapshot, TeacherSnapshot: latest.TeacherSnapshot, Periods: latest.CurrentVersion.Periods}, Form: form, CanEdit: true, AttendanceBlocked: blocked}})
}

func isTeacherAttendanceBlocked(err error) bool {
	return errors.Is(err, academic.ErrTeacherLessonIneligible)
}

func (a *application) journalFailure(w http.ResponseWriter, r *http.Request, err error) {
	code, message := academicError(err)
	if code >= 500 {
		a.failure(w, err)
		return
	}
	a.render(w, r, "page", code, view{Page: "teaching-journal-error", Title: "Jurnal Mengajar", Error: message})
}
