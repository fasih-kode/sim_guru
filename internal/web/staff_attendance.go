package web

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
)

type staffAttendanceView struct {
	Day               academic.StaffAttendanceDay
	History           academic.StaffAttendanceHistory
	CheckIn, CheckOut *academic.StaffAttendanceEvent
	Available         bool
	HistoryBase       string
}

func (a *application) staffAttendance(w http.ResponseWriter, r *http.Request) {
	status, message := http.StatusOK, ""
	if r.Method == http.MethodPost {
		if a.options.Academics.StaffPhotos == nil {
			status, message = http.StatusServiceUnavailable, "Kehadiran pegawai belum tersedia. Hubungi Admin sekolah."
			slog.Warn("presensi pegawai gagal", "error_class", "photo_storage_disabled")
		} else {
			capture, err := teacherAttendanceUpload(w, r)
			if err == nil {
				var proof academic.StaffAttendanceEvent
				proof, err = a.options.Academics.CaptureStaffAttendance(r.Context(), principal(r).ID, academic.StaffAttendanceCaptureInput{
					IdempotencyKey: capture.IdempotencyKey, Action: capture.Action, Latitude: capture.Latitude, Longitude: capture.Longitude,
					AccuracyM: capture.AccuracyM, Selfie: capture.Selfie, SelfieMIMEType: capture.SelfieMIMEType,
				})
				if err == nil {
					http.Redirect(w, r, "/pegawai/kehadiran-saya?saved=1&recorded_date="+proof.Date, http.StatusSeeOther)
					return
				}
			}
			status, message = staffAttendanceHTTPError(err)
			slog.Warn("presensi pegawai gagal", "error_class", staffAttendanceFailureClass(err))
		}
	}
	page := reportPage(r.URL.Query().Get("page"))
	if page > 100000 {
		a.academicFailure(w, academic.ValidationError("Halaman riwayat tidak valid."))
		return
	}
	day, err := a.options.Academics.StaffAttendanceToday(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	history, err := a.options.Academics.StaffAttendanceOwnHistory(r.Context(), principal(r).ID, page)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data := &staffAttendanceView{Day: day, History: history, Available: a.options.Academics.StaffPhotos != nil, HistoryBase: "/pegawai/kehadiran-saya"}
	for i := range day.Events {
		event := &day.Events[i]
		if event.Action == academic.TeacherCheckIn {
			data.CheckIn = event
		} else if event.Action == academic.TeacherCheckOut {
			data.CheckOut = event
		}
	}
	v := view{Page: "staff-attendance", Title: "Kehadiran Saya", StaffAttendance: data, Error: message}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = teacherAttendanceSavedMessage(day.Date, r.URL.Query().Get("recorded_date"))
	}
	a.render(w, r, "page", status, v)
}

func staffAttendanceHTTPError(err error) (int, string) {
	var tooLarge *http.MaxBytesError
	var validation academic.ValidationError
	switch {
	case err == nil:
		return http.StatusUnprocessableEntity, "Data kehadiran tidak lengkap."
	case errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge, "Ukuran unggahan melebihi batas. Ambil ulang foto."
	case errors.Is(err, academic.ErrForbidden):
		return http.StatusForbidden, "Akun tidak memiliki identitas personel aktif dan role yang sesuai untuk mencatat kehadiran sendiri."
	case errors.Is(err, academic.ErrConflict):
		return http.StatusConflict, "Aksi kehadiran hari ini sudah tercatat atau bukti retry berbeda. Periksa status terbaru."
	case errors.Is(err, academic.ErrStaffAttendancePhoto):
		return http.StatusServiceUnavailable, "Penyimpanan foto kehadiran Pegawai TU belum aktif. Hubungi Admin sekolah sebelum mencoba kembali."
	case errors.Is(err, teacherphoto.ErrStorage), errors.Is(err, teacherphoto.ErrNotConfigured), errors.Is(err, teacherphoto.ErrClosed):
		return http.StatusServiceUnavailable, "Penyimpanan selfie sedang bermasalah. Muat ulang status kehadiran; jika belum tercatat, coba lagi dengan bukti yang sama atau hubungi Admin sekolah."
	case errors.Is(err, academic.ErrStaffAttendanceWrite):
		return http.StatusServiceUnavailable, "Server gagal menyimpan catatan presensi. Muat ulang status; jika belum tercatat, kirim ulang dengan bukti yang sama atau hubungi Admin sekolah."
	case errors.Is(err, academic.ErrTeacherAttendanceCleanup):
		return http.StatusServiceUnavailable, "Penyimpanan bukti perlu diperiksa Admin. Muat ulang status dan hubungi Admin sebelum memulai tindakan baru."
	case errors.Is(err, academic.ErrTeacherAttendanceAction):
		return http.StatusUnprocessableEntity, "Pilih tindakan Masuk atau Pulang."
	case errors.As(err, &validation):
		return http.StatusUnprocessableEntity, string(validation)
	case errors.Is(err, teacherphoto.ErrInvalidImage), errors.Is(err, teacherphoto.ErrUnsupportedMediaType), errors.Is(err, teacherphoto.ErrInputTooLarge), errors.Is(err, teacherphoto.ErrPhotoTooLarge), errors.Is(err, teacherphoto.ErrImageDimensions):
		return http.StatusUnprocessableEntity, "Selfie tidak valid. Ambil ulang foto melalui kamera."
	default:
		return http.StatusServiceUnavailable, "Hasil pencatatan belum dapat dipastikan. Coba kirim ulang dengan bukti yang sama atau hubungi Admin sekolah."
	}
}

// staffAttendanceFailureClass hanya mencatat kategori aman; jangan log pesan
// driver, input GPS, token idempotensi, foto, identitas, atau storage key.
func staffAttendanceFailureClass(err error) string {
	var tooLarge *http.MaxBytesError
	var validation academic.ValidationError
	switch {
	case err == nil:
		return "missing_input"
	case errors.As(err, &tooLarge):
		return "upload_too_large"
	case errors.Is(err, academic.ErrForbidden):
		return "forbidden"
	case errors.Is(err, academic.ErrConflict):
		return "conflict"
	case errors.Is(err, academic.ErrStaffAttendancePhoto), errors.Is(err, teacherphoto.ErrStorage), errors.Is(err, teacherphoto.ErrNotConfigured), errors.Is(err, teacherphoto.ErrClosed):
		return "photo_storage"
	case errors.Is(err, academic.ErrStaffAttendanceWrite):
		return "database_write"
	case errors.Is(err, academic.ErrTeacherAttendanceCleanup):
		return "photo_cleanup"
	case errors.Is(err, academic.ErrTeacherAttendanceCommit):
		return "commit_uncertain"
	case errors.Is(err, academic.ErrTeacherAttendanceAction):
		return "invalid_action"
	case errors.As(err, &validation):
		return "validation"
	case errors.Is(err, teacherphoto.ErrInvalidImage), errors.Is(err, teacherphoto.ErrUnsupportedMediaType), errors.Is(err, teacherphoto.ErrInputTooLarge), errors.Is(err, teacherphoto.ErrPhotoTooLarge), errors.Is(err, teacherphoto.ErrImageDimensions):
		return "invalid_selfie"
	default:
		return "internal"
	}
}

func (a *application) staffAttendancePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	file, mimeType, err := a.options.Academics.StaffAttendancePhoto(r.Context(), principal(r).ID, id)
	if errors.Is(err, academic.ErrNotFound) || errors.Is(err, academic.ErrForbidden) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	defer file.Close()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, file)
}
