package academic

import (
	"testing"
	"time"
)

func TestEvaluateTeacherAttendance(t *testing.T) {
	at := func(raw string) time.Time {
		t.Helper()
		v, e := time.ParseInLocation("2006-01-02 15:04:05.999999999", raw, teacherAttendanceZone)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	base := TeacherAttendanceEvaluationInput{Date: "2026-10-05", ScheduleKnown: true, Now: at("2026-10-06 12:00:00"), Obligations: []TeacherObligation{{StartsAt: at("2026-10-05 07:00:00"), EndsAt: at("2026-10-05 11:00:00")}}}
	tests := []struct {
		name, in, out, now, report string
		known, noDuty, cancel      bool
		want                       string
		late, early                int
		review                     bool
	}{
		{name: "tepat", in: "07:00:00", out: "11:00:00", want: TeacherPresent},
		{name: "terlambat semenit", in: "07:01:00", out: "11:00:00", want: TeacherLate, late: 1},
		{name: "terlambat detik", in: "07:00:01", out: "11:00:00", want: TeacherLate, late: 1},
		{name: "presisi nanodetik", in: "07:00:00.000000001", out: "11:00:00", want: TeacherLate, late: 1},
		{name: "awal", in: "06:55:00", out: "11:01:00", want: TeacherPresent},
		{name: "pulang cepat", in: "06:55:00", out: "10:30:00", want: TeacherEarly, early: 30},
		{name: "pulang cepat detik", in: "07:00:00", out: "10:59:59", want: TeacherEarly, early: 1},
		{name: "keduanya", in: "07:01:01", out: "10:30:00", want: TeacherEarly, late: 2, early: 30},
		{name: "masuk saja", in: "07:00:00", want: TeacherIncomplete},
		{name: "pulang saja", out: "11:00:00", want: TeacherIncomplete},
		{name: "kosong", want: TeacherUnrecorded},
		{name: "izin", report: "izin", want: TeacherLeave},
		{name: "sakit", report: "sakit", want: TeacherSick},
		{name: "dinas tanpa jadwal", report: "dinas_luar", noDuty: true, want: TeacherDuty},
		{name: "dinas membenarkan pulang cepat", report: "dinas_luar", in: "07:00:00", out: "10:00:00", want: TeacherDuty, early: 60},
		{name: "izin dan bukti", report: "izin", in: "07:00:00", out: "11:00:00", want: TeacherLeave, review: true},
		{name: "sakit dan bukti", report: "sakit", in: "07:00:00", want: TeacherSick, review: true},
		{name: "dibatalkan", report: "izin", cancel: true, want: TeacherUnrecorded},
		{name: "libur", noDuty: true, want: TeacherUnscheduled},
		{name: "sukarela", noDuty: true, in: "06:00:00", out: "11:00:00", want: TeacherUnscheduled},
		{name: "jadwal tak diketahui", known: true, want: TeacherUnevaluable},
		{name: "sebelum mulai", now: "2026-10-05 06:59:59", want: TeacherUpcoming},
		{name: "tepat mulai", now: "2026-10-05 07:00:00", want: TeacherOngoing},
		{name: "sedang masuk saja", in: "07:00:00", now: "2026-10-05 10:00:00", want: TeacherOngoing},
		{name: "tepat selesai", now: "2026-10-05 11:00:00", want: TeacherUnrecorded},
		{name: "masa depan", now: "2026-10-04 23:59:59", want: TeacherUpcoming},
		{name: "masa depan sumber belum tersedia", known: true, now: "2026-10-04 23:59:59", want: TeacherUnevaluable},
		{name: "pergantian WIB", now: "2026-10-06 00:00:00", want: TeacherUnrecorded},
		{name: "urutan bertentangan", in: "12:00:00", out: "11:00:00", want: TeacherUnevaluable, late: 300, review: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			if tt.in != "" {
				in.CheckIn = &TeacherAttendanceManagedEvent{OccurredAt: at(in.Date + " " + tt.in)}
			}
			if tt.out != "" {
				in.CheckOut = &TeacherAttendanceManagedEvent{OccurredAt: at(in.Date + " " + tt.out)}
			}
			if tt.now != "" {
				in.Now = at(tt.now)
			}
			if tt.noDuty {
				in.Obligations = nil
			}
			if tt.known {
				in.ScheduleKnown = false
				in.UnavailableReason = "Jadwal belum tersedia"
			}
			if tt.report != "" {
				in.Report = &TeacherAttendanceReport{Kind: tt.report, Active: !tt.cancel, StartsOn: in.Date, EndsOn: in.Date}
			}
			got := EvaluateTeacherAttendance(in)
			if got.Status != tt.want || got.LateMinutes != tt.late || got.EarlyMinutes != tt.early || got.NeedsReview != tt.review {
				t.Fatalf("got %+v want %s late=%d early=%d review=%v", got, tt.want, tt.late, tt.early, tt.review)
			}
			if got.CheckIn != in.CheckIn || got.CheckOut != in.CheckOut {
				t.Fatal("bukti diubah")
			}
		})
	}
}

func TestTeacherAttendanceObligationEnvelopeAndCounts(t *testing.T) {
	start := time.Date(2026, 10, 5, 7, 0, 0, 0, teacherAttendanceZone)
	in := TeacherAttendanceEvaluationInput{Date: "2026-10-05", Now: start.Add(24 * time.Hour), ScheduleKnown: true, Obligations: []TeacherObligation{{StartsAt: start.Add(2 * time.Hour), EndsAt: start.Add(4 * time.Hour)}, {StartsAt: start, EndsAt: start.Add(time.Hour)}}}
	// Batas yang sama juga berlaku saat sumber tugas wajib yang sah kelak tersedia.
	got := EvaluateTeacherAttendance(in)
	if !got.StartsAt.Equal(start) || !got.EndsAt.Equal(start.Add(4*time.Hour)) || !got.Required {
		t.Fatal(got)
	}
	r := TeacherAttendanceRecap{Days: []TeacherAttendanceEvaluation{got, {Status: TeacherUnscheduled}, {Status: TeacherDuty}}}
	r.summarize()
	sum := 0
	for _, c := range r.Counts {
		sum += c.Count
	}
	if sum != 3 || r.RequiredDays != 1 {
		t.Fatal(r)
	}
}
