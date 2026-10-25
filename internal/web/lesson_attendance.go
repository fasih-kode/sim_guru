package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type lessonAttendanceStudentView struct {
	ID          int64
	Name, NIS   string
	Status      string
	StatusLabel string
	RecordedAt  string
	Saved       bool
}

type lessonAttendanceView struct {
	Date                               string
	Lessons                            []academic.TimetableEntry
	Lesson                             *academic.TimetableEntry
	Students                           []lessonAttendanceStudentView
	Records                            []lessonAttendanceRecordRow
	Record                             *lessonAttendanceRecordView
	CorrectionStatus, CorrectionReason string
	Fingerprint                        string
	Stale                              bool
	RevisionChanged                    bool
}

type lessonAttendanceRecordRow struct {
	ID                                                                              int64
	Date, StudentName, NIS, ClassName, SubjectName, TeacherName, Status, RecordedAt string
}

type lessonAttendanceRecordView struct {
	ID, StudentID, Version                                                                                                     int64
	Date, StudentName, NIS, ClassName, SubjectName, TeacherName, Status, StartsAt, EndsAt, TimeSegments, RecordedAt, UpdatedAt string
	PeriodStart, PeriodEnd                                                                                                     int
	CanCorrect, Administrative                                                                                                 bool
	History                                                                                                                    []lessonAttendanceHistoryView
}

type lessonAttendanceHistoryView struct {
	Action, Kind, Actor, Before, After, Reason, At string
	BeforeVersion, AfterVersion                    int64
}

func (a *application) lessonAttendance(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a.saveLessonAttendance(w, r)
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = dailyToday()
	}
	actor := principal(r).ID
	records, err := a.options.Academics.LessonAttendanceRecordsOnDate(r.Context(), actor, date)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := &lessonAttendanceView{Date: date}
	for _, record := range records {
		v.Records = append(v.Records, lessonAttendanceRecordRow{ID: record.ID, Date: record.Date, StudentName: record.StudentName, NIS: record.NIS, ClassName: record.ClassName, SubjectName: record.SubjectName, TeacherName: record.TeacherName, Status: lessonAttendanceStatusLabel(record.Status), RecordedAt: record.RecordedAt.In(time.FixedZone("WIB", 7*60*60)).Format("15:04 WIB")})
	}
	if principal(r).HasRole("guru_mapel") {
		meetings, meetingErr := a.options.Academics.TeacherLessonsOnDate(r.Context(), actor, date)
		if meetingErr == nil {
			v.Lessons = meetings.Lessons
		} else if !principal(r).HasRole("kurikulum") || !errors.Is(meetingErr, academic.ErrForbidden) {
			a.academicFailure(w, meetingErr)
			return
		}
	}
	if raw := r.URL.Query().Get("entry_id"); raw != "" {
		entryID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || entryID < 1 {
			http.NotFound(w, r)
			return
		}
		context, contextErr := a.options.Academics.TeacherLessonAttendanceOnDate(r.Context(), principal(r).ID, date, entryID)
		if contextErr != nil {
			a.academicFailure(w, contextErr)
			return
		}
		applyLessonAttendanceContext(v, context, nil)
	}
	page := view{Page: "lesson-attendance", Title: "Absensi per Pelajaran", LessonAttendance: v}
	if r.URL.Query().Get("saved") == "1" && hasSavedLessonAttendance(v.Students) {
		page.Message = "Absensi per pelajaran berhasil disimpan."
	}
	a.render(w, r, "page", http.StatusOK, page)
}

func (a *application) lessonAttendanceRecord(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	record, err := a.options.Academics.LessonAttendanceRecord(r.Context(), principal(r).ID, id)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := &lessonAttendanceView{Date: record.Date, Record: toLessonAttendanceRecordView(record)}
	page := view{Page: "lesson-attendance", Title: "Absensi per Pelajaran", LessonAttendance: v}
	if r.URL.Query().Get("edit") == "1" && record.CanCorrect {
		page.ModalOpen = true
	}
	if r.URL.Query().Get("saved") == "1" {
		page.Message = "Koreksi absensi per pelajaran tersimpan dan histori diperbarui."
	} else if r.URL.Query().Get("unchanged") == "1" {
		page.Message = "Status sudah sama; tidak ada perubahan atau histori koreksi baru."
	}
	a.render(w, r, "page", http.StatusOK, page)
}

func (a *application) correctLessonAttendance(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	postedID, parseErr := strconv.ParseInt(r.PostForm.Get("attendance_id"), 10, 64)
	version, versionErr := strconv.ParseInt(r.PostForm.Get("expected_version"), 10, 64)
	if parseErr != nil || versionErr != nil || postedID != id {
		http.NotFound(w, r)
		return
	}
	input := academic.LessonAttendanceCorrectionInput{AttendanceID: postedID, ExpectedVersion: version, NewStatus: r.PostForm.Get("new_status"), Reason: r.PostForm.Get("reason")}
	result, err := a.options.Academics.CorrectLessonAttendance(r.Context(), principal(r).ID, input)
	if err == nil {
		if result.Noop {
			http.Redirect(w, r, fmt.Sprintf("/jadwal-saya/absensi/catatan/%d?unchanged=1", id), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/jadwal-saya/absensi/catatan/%d?saved=1", id), http.StatusSeeOther)
		return
	}
	code, message := academicError(err)
	if code >= http.StatusInternalServerError {
		a.failure(w, err)
		return
	}
	latest, readErr := a.options.Academics.LessonAttendanceRecord(r.Context(), principal(r).ID, id)
	if readErr != nil {
		a.academicFailure(w, readErr)
		return
	}
	v := &lessonAttendanceView{Date: latest.Date, Record: toLessonAttendanceRecordView(latest), CorrectionStatus: input.NewStatus, CorrectionReason: input.Reason}
	page := view{Page: "lesson-attendance", Title: "Absensi per Pelajaran", LessonAttendance: v, Error: message}
	if errors.Is(err, academic.ErrConflict) {
		page.Error = "Catatan telah berubah sejak dibuka. Status terkini ditampilkan; periksa ulang sebelum mengirim koreksi baru."
		code = http.StatusConflict
	} else {
		page.ModalOpen = true
	}
	a.render(w, r, "page", code, page)
}

func toLessonAttendanceRecordView(record academic.LessonAttendanceHistoricalRecord) *lessonAttendanceRecordView {
	v := &lessonAttendanceRecordView{ID: record.ID, StudentID: record.StudentID, Version: record.Version, Date: record.Date, StudentName: record.StudentName, NIS: record.NIS, ClassName: record.ClassName, SubjectName: record.SubjectName, TeacherName: record.TeacherName, Status: lessonAttendanceStatusLabel(record.Status), StartsAt: record.StartsAt, EndsAt: record.EndsAt, TimeSegments: record.TimeSegments, PeriodStart: record.PeriodStart, PeriodEnd: record.PeriodEnd, RecordedAt: record.RecordedAt.In(time.FixedZone("WIB", 7*60*60)).Format("02 Jan 2006 15:04 WIB"), UpdatedAt: record.UpdatedAt.In(time.FixedZone("WIB", 7*60*60)).Format("02 Jan 2006 15:04 WIB"), CanCorrect: record.CanCorrect, Administrative: record.Administrative}
	for _, entry := range record.History {
		kind := "Guru Mapel"
		if entry.Kind == "kurikulum" {
			kind = "Kurikulum · koreksi administratif"
		}
		before, after := lessonAttendanceStatusLabel(entry.Before), lessonAttendanceStatusLabel(entry.After)
		if entry.Action == "recorded" {
			before = "Belum Dicatat"
		}
		v.History = append(v.History, lessonAttendanceHistoryView{Action: entry.Action, Kind: kind, Actor: entry.Actor, Before: before, After: after, Reason: entry.Reason, BeforeVersion: entry.BeforeVersion, AfterVersion: entry.AfterVersion, At: entry.At.In(time.FixedZone("WIB", 7*60*60)).Format("02 Jan 2006 15:04 WIB")})
	}
	return v
}

func (a *application) saveLessonAttendance(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	date := r.PostForm.Get("date")
	entryID, err := strconv.ParseInt(r.PostForm.Get("entry_id"), 10, 64)
	if err != nil || entryID < 1 {
		http.NotFound(w, r)
		return
	}
	actor := principal(r).ID
	current, err := a.options.Academics.TeacherLessonAttendanceOnDate(r.Context(), actor, date, entryID)
	fingerprint := r.PostForm.Get("fingerprint")
	if errors.Is(err, academic.ErrNotFound) {
		statuses, _, _ := parseLessonAttendanceChoices(r.PostForm)
		if changedErr := a.renderLessonAttendanceRevisionConflict(w, r, actor, date, entryID, fingerprint, statuses); changedErr == nil {
			return
		} else {
			err = changedErr
		}
	}
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	statuses, choices, err := parseLessonAttendanceChoices(r.PostForm)
	var saveResult academic.LessonAttendanceSaveResult
	if err == nil {
		saveResult, err = a.options.Academics.SaveLessonAttendance(r.Context(), actor, academic.LessonAttendanceInput{
			Date: date, EntryID: entryID, Fingerprint: fingerprint, Students: choices,
		})
	}
	if err == nil {
		// Baris baru dan retry identik sama-sama menuju status baca ulang.
		if saveResult.Inserted > 0 || saveResult.Idempotent {
			http.Redirect(w, r, fmt.Sprintf("/jadwal-saya/absensi?date=%s&entry_id=%d&saved=1", url.QueryEscape(date), entryID), http.StatusSeeOther)
			return
		}
		err = errors.New("hasil penyimpanan absensi tidak konsisten")
	}
	code, message := academicError(err)
	if code >= http.StatusInternalServerError {
		a.failure(w, err)
		return
	}
	v := &lessonAttendanceView{Date: date, Stale: code == http.StatusConflict}
	latest, readErr := a.options.Academics.TeacherLessonAttendanceOnDate(r.Context(), actor, date, entryID)
	if readErr == nil {
		applyLessonAttendanceContext(v, latest, statuses)
	} else {
		if errors.Is(readErr, academic.ErrNotFound) {
			if changedErr := a.renderLessonAttendanceRevisionConflict(w, r, actor, date, entryID, fingerprint, statuses); changedErr == nil {
				return
			} else {
				readErr = changedErr
			}
		}
		if errors.Is(readErr, academic.ErrForbidden) || errors.Is(readErr, academic.ErrNotFound) {
			a.academicFailure(w, readErr)
			return
		}
		// Jika konteks baru tidak dapat dibaca, pertahankan konteks lama dan
		// pilihan yang dikirim; fingerprint lama tidak pernah diganti diam-diam.
		applyLessonAttendanceContext(v, current, statuses)
		if code == http.StatusConflict {
			v.Stale = true
		}
	}
	if v.Fingerprint != fingerprint {
		v.Stale = true
	}
	if v.Stale {
		// Form basi tidak pernah memperoleh fingerprint baru secara otomatis.
		v.Fingerprint = fingerprint
	}
	page := view{Page: "lesson-attendance", Title: "Absensi per Pelajaran", LessonAttendance: v, Error: message}
	if v.Stale {
		page.Error = "Konteks jadwal, penugasan, roster, atau catatan berubah. Tidak ada data yang disimpan; pilihan pada siswa yang masih cocok dipertahankan. Muat ulang konteks terbaru sebelum mencoba lagi."
	}
	a.render(w, r, "page", code, page)
}

func (a *application) renderLessonAttendanceRevisionConflict(w http.ResponseWriter, r *http.Request, actor int64, date string, oldEntryID int64, fingerprint string, pending map[int64]string) error {
	changed, err := a.options.Academics.TeacherLessonAttendanceAfterRevision(r.Context(), actor, date, oldEntryID)
	if err != nil {
		return err
	}
	v := &lessonAttendanceView{Date: date, Lessons: changed.Lessons, RevisionChanged: true}
	if changed.Replacement != nil {
		applyLessonAttendanceContext(v, *changed.Replacement, pending)
		v.Stale = true
		v.Fingerprint = fingerprint
	}
	page := view{
		Page:             "lesson-attendance",
		Title:            "Absensi per Pelajaran",
		LessonAttendance: v,
		Error:            "Jadwal terbit berubah setelah formulir dibuka. Tidak ada absensi yang disimpan. Tinjau pertemuan terbaru sebelum mencatat kembali.",
	}
	a.render(w, r, "page", http.StatusConflict, page)
	return nil
}

func parseLessonAttendanceChoices(form url.Values) (map[int64]string, []academic.LessonAttendanceChoice, error) {
	statuses := make(map[int64]string, len(form["student_ids"]))
	choices := make([]academic.LessonAttendanceChoice, 0, len(form["student_ids"]))
	for _, rawID := range form["student_ids"] {
		studentID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || studentID < 1 {
			return statuses, choices, academic.ValidationError("Pilihan siswa tidak valid.")
		}
		if _, exists := statuses[studentID]; exists {
			return statuses, choices, academic.ValidationError("Pilihan siswa tidak valid atau berulang.")
		}
		status := form.Get("status_" + rawID)
		statuses[studentID] = status
		choices = append(choices, academic.LessonAttendanceChoice{StudentID: studentID, Status: status})
	}
	return statuses, choices, nil
}

func hasSavedLessonAttendance(students []lessonAttendanceStudentView) bool {
	for _, student := range students {
		if student.Saved {
			return true
		}
	}
	return false
}

func applyLessonAttendanceContext(v *lessonAttendanceView, context academic.TeacherLessonAttendanceContext, pending map[int64]string) {
	v.Date = context.Date
	v.Lesson = &context.Lesson
	v.Fingerprint = context.Fingerprint
	statuses := make(map[int64]academic.LessonAttendanceRecord, len(context.Records))
	for _, record := range context.Records {
		statuses[record.StudentID] = record
	}
	seen := make(map[int64]bool, len(context.Students))
	for _, student := range context.Students {
		row := lessonAttendanceStudentView{ID: student.ID, Name: student.Name, NIS: student.NIS}
		if record, ok := statuses[student.ID]; ok {
			row.Name = record.StudentName
			row.NIS = record.NIS
			row.Status = record.Status
			row.StatusLabel = lessonAttendanceStatusLabel(record.Status)
			row.RecordedAt = record.RecordedAt.In(time.FixedZone("WIB", 7*60*60)).Format("15:04 WIB")
			row.Saved = true
		} else if pending != nil {
			row.Status = pending[student.ID]
			row.StatusLabel = lessonAttendanceStatusLabel(row.Status)
		}
		v.Students = append(v.Students, row)
		seen[student.ID] = true
	}
	// Catatan lama tetap terlihat jika siswa kemudian tidak lagi berada di roster aktif.
	for _, record := range context.Records {
		if seen[record.StudentID] {
			continue
		}
		v.Students = append(v.Students, lessonAttendanceStudentView{
			ID: record.StudentID, Name: record.StudentName, NIS: record.NIS, Status: record.Status, StatusLabel: lessonAttendanceStatusLabel(record.Status),
			RecordedAt: record.RecordedAt.In(time.FixedZone("WIB", 7*60*60)).Format("15:04 WIB"), Saved: true,
		})
	}
}

func lessonAttendanceStatusLabel(status string) string {
	switch status {
	case "hadir":
		return "Hadir"
	case "sakit":
		return "Sakit"
	case "izin":
		return "Izin"
	case "alpa":
		return "Alpa"
	default:
		return "Belum Dicatat"
	}
}
