package academic

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestEvaluateTeacherLessonEligibility(t *testing.T) {
	today := "2026-11-01"
	past := "2026-10-31"
	checkIn := time.Date(2026, 11, 1, 7, 0, 0, 0, teacherAttendanceZone)
	checkOutAfter := time.Date(2026, 11, 1, 9, 0, 0, 0, teacherAttendanceZone)
	checkOutEarly := time.Date(2026, 11, 1, 7, 45, 0, 0, teacherAttendanceZone)
	baseToday := teacherLessonAttendanceEvidence{Today: today, CheckIn: sql.NullTime{Time: checkIn, Valid: true}, CheckInValid: true}
	basePast := teacherLessonAttendanceEvidence{Today: today, CheckIn: sql.NullTime{Time: checkIn.AddDate(0, 0, -1), Valid: true}, CheckInValid: true, CheckOut: sql.NullTime{Time: checkOutAfter.AddDate(0, 0, -1), Valid: true}, CheckOutValid: true}
	tests := []struct {
		name     string
		date     string
		end      string
		evidence teacherLessonAttendanceEvidence
		allowed  bool
		message  string
	}{
		{name: "hari berjalan Masuk cukup sebelum JP berakhir", date: today, end: "08:00:00", evidence: baseToday, allowed: true},
		{name: "hari berjalan tanpa Masuk diblokir", date: today, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today}, message: "Masuk hari ini belum tercatat"},
		{name: "hari lampau Masuk Pulang lengkap", date: past, end: "08:00:00", evidence: basePast, allowed: true},
		{name: "hari lampau hanya Masuk", date: past, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: basePast.CheckIn, CheckInValid: true}, message: "Pulang pada tanggal pertemuan belum tercatat"},
		{name: "Masuk belum sah", date: past, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: basePast.CheckIn, CheckInValid: false}, message: "Masuk belum dapat diverifikasi"},
		{name: "Pulang bertentangan sebelum Masuk", date: past, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: sql.NullTime{Time: checkOutAfter, Valid: true}, CheckInValid: true, CheckOut: sql.NullTime{Time: checkIn, Valid: true}, CheckOutValid: true}, message: "Pulang bertentangan"},
		{name: "Pulang cepat sebelum akhir JP memblokir pertemuan", date: today, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: sql.NullTime{Time: checkIn, Valid: true}, CheckInValid: true, CheckOut: sql.NullTime{Time: checkOutEarly, Valid: true}, CheckOutValid: true}, message: "mendahului waktu selesai pertemuan"},
		{name: "Pulang tepat saat akhir JP diperbolehkan", date: today, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: sql.NullTime{Time: checkIn, Valid: true}, CheckInValid: true, CheckOut: sql.NullTime{Time: time.Date(2026, 11, 1, 8, 0, 0, 0, teacherAttendanceZone), Valid: true}, CheckOutValid: true}, allowed: true},
		{name: "presensi terlambat tanpa ambang tambahan tetap sah", date: today, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: sql.NullTime{Time: time.Date(2026, 11, 1, 7, 55, 0, 0, teacherAttendanceZone), Valid: true}, CheckInValid: true}, allowed: true},
		{name: "Izin aktif memblokir", date: today, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: baseToday.CheckIn, CheckInValid: true, ReportKind: "izin"}, message: "Laporan Izin aktif"},
		{name: "Sakit aktif memblokir", date: today, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: baseToday.CheckIn, CheckInValid: true, ReportKind: "sakit"}, message: "Laporan Sakit aktif"},
		{name: "Dinas Luar memblokir secara default", date: today, end: "08:00:00", evidence: teacherLessonAttendanceEvidence{Today: today, CheckIn: baseToday.CheckIn, CheckInValid: true, ReportKind: "dinas_luar"}, message: "Dinas Luar aktif"},
		{name: "tanggal mendatang tidak dapat memakai bukti hari ini", date: "2026-11-02", end: "08:00:00", evidence: baseToday, message: "tanggal pertemuan mendatang"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluateTeacherLessonEligibility(test.evidence, test.date, test.end)
			if got.Allowed != test.allowed {
				t.Fatalf("allowed=%t, want %t; reason=%q", got.Allowed, test.allowed, got.Reason)
			}
			if test.message != "" && !strings.Contains(got.Reason, test.message) {
				t.Fatalf("reason=%q does not contain %q", got.Reason, test.message)
			}
		})
	}
}
