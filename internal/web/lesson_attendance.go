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
	Date        string
	Lessons     []academic.TimetableEntry
	Lesson      *academic.TimetableEntry
	Students    []lessonAttendanceStudentView
	Fingerprint string
	Stale       bool
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
	meetings, err := a.options.Academics.TeacherLessonsOnDate(r.Context(), principal(r).ID, date)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := &lessonAttendanceView{Date: date, Lessons: meetings.Lessons}
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
		v.Lessons = meetings.Lessons
		applyLessonAttendanceContext(v, context, nil)
	}
	page := view{Page: "lesson-attendance", Title: "Absensi per Pelajaran", LessonAttendance: v}
	if r.URL.Query().Get("saved") == "1" && hasSavedLessonAttendance(v.Students) {
		page.Message = "Absensi per pelajaran berhasil disimpan."
	}
	a.render(w, r, "page", http.StatusOK, page)
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
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	statuses := make(map[int64]string, len(r.PostForm["student_ids"]))
	choices := make([]academic.LessonAttendanceChoice, 0, len(r.PostForm["student_ids"]))
	for _, rawID := range r.PostForm["student_ids"] {
		studentID, parseErr := strconv.ParseInt(rawID, 10, 64)
		if parseErr != nil || studentID < 1 {
			err = academic.ValidationError("Pilihan siswa tidak valid.")
			break
		}
		if _, exists := statuses[studentID]; exists {
			err = academic.ValidationError("Pilihan siswa tidak valid atau berulang.")
			break
		}
		status := r.PostForm.Get("status_" + rawID)
		statuses[studentID] = status
		choices = append(choices, academic.LessonAttendanceChoice{StudentID: studentID, Status: status})
	}
	fingerprint := r.PostForm.Get("fingerprint")
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
