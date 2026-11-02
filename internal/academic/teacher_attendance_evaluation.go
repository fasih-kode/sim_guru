package academic

import "time"

// TeacherObligation adalah batas integrasi tugas wajib bertanggal. Saat ini
// sumber yang tersedia hanya pelajaran dari revisi terbit dan penugasan efektif.
type TeacherObligation struct {
	EntryID, RevisionID, AssignmentID int64
	ClassName, SubjectName, Segments  string
	StartsAt, EndsAt                  time.Time
}

type TeacherAttendanceEvaluationInput struct {
	TeacherID                               int64
	TeacherName, Date, UnavailableReason    string
	Now                                     time.Time
	ScheduleKnown                           bool
	SemesterID, CalendarVersion, RevisionID int64
	CalendarNote                            string
	Obligations                             []TeacherObligation
	CheckIn, CheckOut                       *TeacherAttendanceManagedEvent
	Report                                  *TeacherAttendanceReport
}

type TeacherAttendanceEvaluation struct {
	TeacherAttendanceEvaluationInput
	Required                  bool
	StartsAt, EndsAt          time.Time
	Status, Explanation       string
	LateMinutes, EarlyMinutes int
	NeedsReview               bool
}

const (
	TeacherPresent     = "Hadir"
	TeacherLate        = "Hadir (Terlambat)"
	TeacherEarly       = "Tidak Hadir (Pulang Cepat)"
	TeacherLeave       = "Tidak Hadir (Izin)"
	TeacherSick        = "Tidak Hadir (Sakit)"
	TeacherDuty        = "Hadir (Dinas Luar)"
	TeacherIncomplete  = "Kehadiran Belum Lengkap"
	TeacherUnrecorded  = "Belum Dicatat"
	TeacherUnscheduled = "Tidak Terjadwal"
	TeacherUnevaluable = "Belum Dapat Dinilai"
	TeacherOngoing     = "Sedang Berlangsung"
	TeacherUpcoming    = "Akan Datang"
)

// EvaluateTeacherAttendance tidak membaca database atau jam perangkat. Now
// wajib diberikan pemanggil dari jam server agar pengujian deterministik.
func EvaluateTeacherAttendance(in TeacherAttendanceEvaluationInput) TeacherAttendanceEvaluation {
	out := TeacherAttendanceEvaluation{TeacherAttendanceEvaluationInput: in}
	for _, duty := range in.Obligations {
		if out.StartsAt.IsZero() || duty.StartsAt.Before(out.StartsAt) {
			out.StartsAt = duty.StartsAt
		}
		if duty.EndsAt.After(out.EndsAt) {
			out.EndsAt = duty.EndsAt
		}
	}
	out.Required = in.ScheduleKnown && len(in.Obligations) > 0
	if in.CheckIn != nil && in.CheckOut != nil && in.CheckIn.OccurredAt.After(in.CheckOut.OccurredAt) {
		out.NeedsReview = true
	}
	if in.Report != nil && in.Report.Active && in.Date >= in.Report.StartsOn && in.Date <= in.Report.EndsOn {
		if in.Report.Kind != "dinas_luar" && (in.CheckIn != nil || in.CheckOut != nil) {
			out.NeedsReview = true
		}
	} else {
		out.Report = nil
	}
	if out.Required {
		if in.CheckIn != nil {
			out.LateMinutes = exceededMinutes(in.CheckIn.OccurredAt.Sub(out.StartsAt))
		}
		if in.CheckOut != nil {
			out.EarlyMinutes = exceededMinutes(out.EndsAt.Sub(in.CheckOut.OccurredAt))
		}
	}
	if in.Date > TeacherAttendanceWIBDate(in.Now) {
		out.Status, out.Explanation = TeacherUpcoming, "Tanggal mendatang; belum dinilai sebagai kekurangan kehadiran."
		if !in.ScheduleKnown {
			out.Status, out.Explanation = TeacherUnevaluable, in.UnavailableReason
		}
		return out
	}
	if out.Report != nil {
		switch out.Report.Kind {
		case "izin":
			out.Status = TeacherLeave
		case "sakit":
			out.Status = TeacherSick
		case "dinas_luar":
			out.Status = TeacherDuty
		}
		out.Explanation = "Mengikuti laporan aktif; bukti Masuk/Pulang tetap ditampilkan terpisah."
		if !in.ScheduleKnown {
			out.Explanation += " Kewajiban jadwal belum dapat ditentukan: " + in.UnavailableReason
		}
		return out
	}
	if !in.ScheduleKnown {
		out.Status, out.Explanation = TeacherUnevaluable, in.UnavailableReason
		return out
	}
	if !out.Required {
		out.Status, out.Explanation = TeacherUnscheduled, "Tidak ada kewajiban mengajar terjadwal pada tanggal ini. Catatan sukarela tetap tersimpan."
		return out
	}
	if in.Now.Before(out.StartsAt) {
		out.Status, out.Explanation = TeacherUpcoming, "Kewajiban hari ini belum dimulai."
		return out
	}
	if in.Now.Before(out.EndsAt) {
		out.Status, out.Explanation = TeacherOngoing, "Kewajiban hari ini belum berakhir; hasil akhir menunggu akhir kewajiban."
		return out
	}
	switch {
	case in.CheckIn == nil && in.CheckOut == nil:
		out.Status, out.Explanation = TeacherUnrecorded, "Belum ada bukti Masuk/Pulang atau laporan; tidak otomatis Alpa."
	case in.CheckIn == nil || in.CheckOut == nil:
		out.Status, out.Explanation = TeacherIncomplete, "Salah satu bukti Masuk/Pulang belum tersedia; bukan otomatis tidak hadir atau pulang cepat."
	case out.NeedsReview:
		out.Status, out.Explanation = TeacherUnevaluable, "Urutan Masuk/Pulang bertentangan; perlu pemeriksaan administratif."
	case out.EarlyMinutes > 0:
		out.Status = TeacherEarly
	case out.LateMinutes > 0:
		out.Status = TeacherLate
	default:
		out.Status = TeacherPresent
	}
	return out
}

func exceededMinutes(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int((d-1)/time.Minute) + 1
}

type TeacherAttendanceCount struct {
	Status string
	Count  int
}
type TeacherAttendanceRecap struct {
	TeacherID    int64
	TeacherName  string
	Days         []TeacherAttendanceEvaluation
	RequiredDays int
	Counts       []TeacherAttendanceCount
}

func (r *TeacherAttendanceRecap) summarize() {
	statuses := []string{TeacherPresent, TeacherLate, TeacherDuty, TeacherLeave, TeacherSick, TeacherEarly, TeacherIncomplete, TeacherUnrecorded, TeacherUnscheduled, TeacherUnevaluable, TeacherOngoing, TeacherUpcoming}
	counts := map[string]int{}
	for _, day := range r.Days {
		counts[day.Status]++
		if day.Required {
			r.RequiredDays++
		}
	}
	for _, status := range statuses {
		r.Counts = append(r.Counts, TeacherAttendanceCount{status, counts[status]})
	}
}
