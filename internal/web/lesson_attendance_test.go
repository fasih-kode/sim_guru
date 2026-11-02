package web

import (
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func TestApplyLessonAttendanceContextKeepsSavedRowsReadOnly(t *testing.T) {
	context := academic.TeacherLessonAttendanceContext{
		Date:        "2026-10-26",
		Fingerprint: "current-fingerprint",
		Lesson:      academic.TimetableEntry{ID: 12, ClassID: 3, SubjectID: 4, TeacherID: 5},
		Students:    []academic.AttendanceStudent{{ID: 1, Name: "Siswa Aktif Terbaru", NIS: "NIS-1-NEW"}, {ID: 2, Name: "Siswa Baru", NIS: "NIS-2"}},
		Records:     []academic.LessonAttendanceRecord{{StudentID: 1, StudentName: "Siswa Aktif", NIS: "NIS-1", Status: "hadir", RecordedAt: time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)}, {StudentID: 9, StudentName: "Siswa Pindah", NIS: "NIS-9", Status: "izin", RecordedAt: time.Date(2026, 10, 26, 1, 5, 0, 0, time.UTC)}},
	}
	view := &lessonAttendanceView{}
	applyLessonAttendanceContext(view, context, map[int64]string{1: "alpa", 2: "sakit"})
	if len(view.Students) != 3 {
		t.Fatalf("rows=%d, want current roster plus retained saved row", len(view.Students))
	}
	if row := view.Students[0]; !row.Saved || row.Status != "hadir" || row.StatusLabel != "Hadir" || row.Name != "Siswa Aktif" || row.NIS != "NIS-1" {
		t.Fatalf("saved row was changed by pending input: %+v", row)
	}
	if row := view.Students[1]; row.Saved || row.Status != "sakit" || row.StatusLabel != "Sakit" {
		t.Fatalf("pending row was not preserved: %+v", row)
	}
	if row := view.Students[2]; row.ID != 9 || !row.Saved || row.Status != "izin" {
		t.Fatalf("saved student outside current roster was lost: %+v", row)
	}
}

func TestLessonAttendanceStatusLabels(t *testing.T) {
	for status, expected := range map[string]string{"hadir": "Hadir", "sakit": "Sakit", "izin": "Izin", "alpa": "Alpa", "": "Belum Dicatat"} {
		if actual := lessonAttendanceStatusLabel(status); actual != expected {
			t.Errorf("label(%q)=%q, want %q", status, actual, expected)
		}
	}
}
