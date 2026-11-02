package academic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrTeacherLessonIneligible = errors.New("presensi guru belum memenuhi syarat untuk pertemuan ini")

type TeacherLessonEligibility struct {
	Allowed bool
	Reason  string
}

type TeacherLessonIneligibleError struct{ Reason string }

func (e *TeacherLessonIneligibleError) Error() string { return e.Reason }
func (e *TeacherLessonIneligibleError) Is(target error) bool {
	return target == ErrTeacherLessonIneligible
}

type teacherLessonAttendanceEvidence struct {
	Today         string
	CheckIn       sql.NullTime
	CheckOut      sql.NullTime
	CheckInValid  bool
	CheckOutValid bool
	ReportKind    string
}

// teacherLessonAttendanceEvidenceTx membaca bukti sumber tanpa menyimpulkan status
// dari label rekap yang dapat berubah selama hari masih berjalan.
func teacherLessonAttendanceEvidenceTx(ctx context.Context, tx *sql.Tx, teacherID int64, date string) (teacherLessonAttendanceEvidence, error) {
	var evidence teacherLessonAttendanceEvidence
	err := tx.QueryRowContext(ctx, `SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&evidence.Today)
	if err != nil {
		return evidence, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.action,e.occurred_at,
		EXISTS(SELECT 1 FROM app.teacher_attendance_history h WHERE h.event_id=e.id AND h.action='recorded'),
		CASE e.source WHEN 'gps_selfie' THEN e.photo_metadata_id IS NOT NULL AND EXISTS(SELECT 1 FROM app.teacher_attendance_photos p WHERE p.id=e.photo_metadata_id AND p.event_id=e.id)
			WHEN 'manual' THEN length(trim(coalesce(e.manual_reason,''))) BETWEEN 1 AND 500 ELSE false END
		FROM app.teacher_attendance_events e WHERE e.teacher_id=$1 AND e.on_date=$2::date`, teacherID, date)
	if err != nil {
		return evidence, err
	}
	for rows.Next() {
		var action string
		var occurred time.Time
		var hasRecorded, validSource bool
		if err = rows.Scan(&action, &occurred, &hasRecorded, &validSource); err != nil {
			rows.Close()
			return evidence, err
		}
		valid := hasRecorded && validSource && TeacherAttendanceWIBDate(occurred) == date
		switch action {
		case string(TeacherCheckIn):
			evidence.CheckIn = sql.NullTime{Time: occurred, Valid: true}
			evidence.CheckInValid = valid
		case string(TeacherCheckOut):
			evidence.CheckOut = sql.NullTime{Time: occurred, Valid: true}
			evidence.CheckOutValid = valid
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return evidence, err
	}
	err = tx.QueryRowContext(ctx, `SELECT coalesce((SELECT kind FROM app.teacher_attendance_reports
		WHERE teacher_id=$1 AND active AND starts_on<=$2::date AND ends_on>=$2::date ORDER BY id DESC LIMIT 1),'')`, teacherID, date).Scan(&evidence.ReportKind)
	return evidence, err
}

func evaluateTeacherLessonEligibility(e teacherLessonAttendanceEvidence, date, lessonEnd string) TeacherLessonEligibility {
	blocked := func(reason string) TeacherLessonEligibility { return TeacherLessonEligibility{Reason: reason} }
	switch e.ReportKind {
	case "izin":
		return blocked("Laporan Izin aktif pada tanggal pertemuan. Silakan minta petugas berwenang memeriksa atau mengoreksi laporan tersebut.")
	case "sakit":
		return blocked("Laporan Sakit aktif pada tanggal pertemuan. Silakan minta petugas berwenang memeriksa atau mengoreksi laporan tersebut.")
	case "dinas_luar":
		return blocked("Laporan Dinas Luar aktif; akses pertemuan diblokir secara default sampai laporan diperiksa petugas berwenang.")
	}
	if date > e.Today {
		return blocked("Presensi untuk tanggal pertemuan mendatang belum tersedia.")
	}
	if !e.CheckIn.Valid {
		if date == e.Today {
			return blocked("Presensi Masuk hari ini belum tercatat. Catat Masuk sebelum mengisi jurnal atau absensi pelajaran.")
		}
		return blocked("Presensi Masuk pada tanggal pertemuan belum tercatat atau tidak dapat diverifikasi.")
	}
	if !e.CheckInValid {
		return blocked("Bukti presensi Masuk belum dapat diverifikasi. Silakan minta petugas berwenang memeriksanya.")
	}
	if date != e.Today {
		if !e.CheckOut.Valid {
			return blocked("Presensi Pulang pada tanggal pertemuan belum tercatat lengkap.")
		}
		if !e.CheckOutValid {
			return blocked("Bukti presensi Pulang belum dapat diverifikasi. Silakan minta petugas berwenang memeriksanya.")
		}
	}
	if e.CheckOut.Valid {
		if !e.CheckOutValid {
			return blocked("Bukti presensi Pulang belum dapat diverifikasi. Silakan minta petugas berwenang memeriksanya.")
		}
		if !e.CheckOut.Time.After(e.CheckIn.Time) {
			return blocked("Waktu presensi Pulang bertentangan dengan presensi Masuk. Silakan minta petugas berwenang memeriksanya.")
		}
		end, err := time.Parse("15:04:05", strings.TrimSpace(lessonEnd))
		if err != nil {
			end, err = time.Parse("15:04", strings.TrimSpace(lessonEnd))
		}
		if err != nil {
			return blocked("Waktu resmi pertemuan belum dapat diverifikasi.")
		}
		meetingEnd := time.Date(e.CheckIn.Time.In(teacherAttendanceZone).Year(), e.CheckIn.Time.In(teacherAttendanceZone).Month(), e.CheckIn.Time.In(teacherAttendanceZone).Day(), end.Hour(), end.Minute(), end.Second(), 0, teacherAttendanceZone)
		if e.CheckOut.Time.Before(meetingEnd) {
			return blocked("Presensi Pulang mendahului waktu selesai pertemuan ini. Silakan minta petugas berwenang memeriksa presensi.")
		}
	}
	return TeacherLessonEligibility{Allowed: true}
}

func teacherLessonEligibilityTx(ctx context.Context, tx *sql.Tx, teacherID int64, date, lessonEnd string) (TeacherLessonEligibility, error) {
	evidence, err := teacherLessonAttendanceEvidenceTx(ctx, tx, teacherID, date)
	if err != nil {
		return TeacherLessonEligibility{}, err
	}
	return evaluateTeacherLessonEligibility(evidence, date, lessonEnd), nil
}

// TeacherLessonEligibilityForDate adalah pembacaan bersama untuk form jurnal
// yang memerlukan identitas guru aktif dan waktu selesai dari snapshot resmi.
func (s *Store) TeacherLessonEligibilityForDate(ctx context.Context, actor int64, date, lessonEnd string) (TeacherLessonEligibility, error) {
	if _, err := civilDate(date); err != nil {
		return TeacherLessonEligibility{}, err
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return TeacherLessonEligibility{}, err
	}
	defer tx.Rollback()
	var teacherID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active`, actor).Scan(&teacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return TeacherLessonEligibility{}, ErrForbidden
	}
	if err != nil {
		return TeacherLessonEligibility{}, err
	}
	return teacherLessonEligibilityTx(ctx, tx, teacherID, date, lessonEnd)
}

func teacherLessonEligibilityError(eligibility TeacherLessonEligibility) error {
	if eligibility.Allowed {
		return nil
	}
	if eligibility.Reason == "" {
		return fmt.Errorf("%w: %s", ErrTeacherLessonIneligible, "Presensi guru belum dapat diverifikasi untuk pertemuan ini.")
	}
	return &TeacherLessonIneligibleError{Reason: eligibility.Reason}
}
