package web

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
)

const teacherSelfieRequestLimit = (5 << 20) + (64 << 10)

type teacherAttendanceView struct {
	Evaluation        *academic.TeacherAttendanceEvaluation
	Day               academic.TeacherAttendanceDay
	CheckIn, CheckOut *academic.TeacherAttendanceSummary
	Available         bool
	RequestOpen       bool
	RequestDate       string
	RequestKind       string
	RequestAction     academic.TeacherAttendanceAction
	RequestText       string
}

func (a *application) teacherAttendance(w http.ResponseWriter, r *http.Request) {
	day, err := a.options.Academics.TeacherAttendanceToday(r.Context(), principal(r).ID)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	status, message := http.StatusOK, ""
	if r.Method == http.MethodPost {
		if a.options.Academics.Photos == nil {
			status, message = http.StatusServiceUnavailable, "Kehadiran mandiri belum tersedia. Hubungi TU Pengelola Kehadiran Guru."
		} else {
			var input academic.TeacherAttendanceCaptureInput
			input, err = teacherAttendanceUpload(w, r)
			if err == nil {
				var proof academic.TeacherAttendanceCaptureProof
				proof, err = a.options.Academics.CaptureTeacherAttendance(r.Context(), principal(r).ID, input)
				if err == nil {
					http.Redirect(w, r, "/kehadiran-saya?saved=1&recorded_date="+url.QueryEscape(proof.OnDate), http.StatusSeeOther)
					return
				}
			}
			status, message = teacherAttendanceHTTPError(err)
		}
		// Konflik harus menampilkan status terbaru milik guru yang sama, bukan bukti request lama.
		day, err = a.options.Academics.TeacherAttendanceToday(r.Context(), principal(r).ID)
		if err != nil {
			a.academicFailure(w, err)
			return
		}
	}
	f := &teacherAttendanceView{Day: day, Available: a.options.Academics.Photos != nil, RequestDate: day.Date, RequestKind: "correction", RequestAction: academic.TeacherCheckIn, RequestOpen: r.URL.Query().Get("request") == "1"}
	for i := range day.Events {
		event := &day.Events[i]
		if event.Action == academic.TeacherCheckIn {
			f.CheckIn = event
		} else {
			f.CheckOut = event
		}
	}
	f.Evaluation, err = a.teacherTodayEvaluation(r, day.Date)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	v := view{Page: "teacher-attendance", Title: "Kehadiran Saya", TeacherAttendance: f, Error: message}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = teacherAttendanceSavedMessage(day.Date, r.URL.Query().Get("recorded_date"))
	}
	if r.URL.Query().Get("request_saved") == "1" {
		v.Message = "Permintaan terkirim kepada TU. Catatan kehadiran tidak berubah."
	}
	reports, reportErr := a.options.Academics.TeacherAttendanceReports(r.Context(), principal(r).ID, academic.TeacherAttendanceReportFilter{Scope: academic.TeacherReportOwn, Page: 1})
	if reportErr != nil {
		a.academicFailure(w, reportErr)
		return
	}
	v.TeacherReports = &teacherReportsView{Base: "/kehadiran-saya/laporan", Own: true, CanCreate: true, Filter: academic.TeacherAttendanceReportFilter{Page: 1}, Reports: reports}
	if len(reports) > 10 {
		v.TeacherReports.Next = 2
		v.TeacherReports.NextURL = "/kehadiran-saya/laporan?page=2"
		v.TeacherReports.Reports = reports[:10]
	}
	a.render(w, r, "page", status, v)
}

func teacherAttendanceUpload(w http.ResponseWriter, r *http.Request) (academic.TeacherAttendanceCaptureInput, error) {
	var input academic.TeacherAttendanceCaptureInput
	r.Body = http.MaxBytesReader(w, r.Body, teacherSelfieRequestLimit)
	if err := r.ParseMultipartForm(teacherSelfieRequestLimit); err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			return input, err
		}
		return input, academic.ValidationError("Form unggahan tidak valid. Ambil ulang foto.")
	}
	if r.MultipartForm == nil {
		return input, academic.ValidationError("Form kehadiran tidak valid.")
	}
	for _, name := range []string{"action", "idempotency_key", "latitude", "longitude", "accuracy_m"} {
		if len(r.MultipartForm.Value[name]) != 1 {
			return input, academic.ValidationError("Data kehadiran belum lengkap atau tidak valid.")
		}
	}
	files := r.MultipartForm.File["selfie"]
	if len(files) != 1 || len(r.MultipartForm.File) != 1 {
		return input, academic.ValidationError("Ambil satu foto selfie sebelum mengirim.")
	}
	file, err := files[0].Open()
	if err != nil {
		return input, academic.ValidationError("Foto tidak dapat dibaca. Ambil ulang foto.")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (5<<20)+1))
	if err != nil {
		return input, academic.ValidationError("Foto tidak dapat dibaca. Ambil ulang foto.")
	}
	if len(data) > 5<<20 {
		return input, teacherphoto.ErrInputTooLarge
	}
	input.Selfie = bytes.NewReader(data)
	input.SelfieMIMEType = files[0].Header.Get("Content-Type")
	input.Action = academic.TeacherAttendanceAction(r.MultipartForm.Value["action"][0])
	input.IdempotencyKey = r.MultipartForm.Value["idempotency_key"][0]
	input.Latitude, err = strconv.ParseFloat(r.MultipartForm.Value["latitude"][0], 64)
	if err == nil {
		input.Longitude, err = strconv.ParseFloat(r.MultipartForm.Value["longitude"][0], 64)
	}
	if err == nil {
		input.AccuracyM, err = strconv.ParseFloat(r.MultipartForm.Value["accuracy_m"][0], 64)
	}
	if err != nil {
		return academic.TeacherAttendanceCaptureInput{}, academic.ValidationError("Lokasi belum valid. Ambil lokasi kembali.")
	}
	return input, nil
}

func teacherAttendanceHTTPError(err error) (int, string) {
	var tooLarge *http.MaxBytesError
	var validation academic.ValidationError
	switch {
	case errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge, "Ukuran unggahan melebihi batas. Ambil ulang foto."
	case errors.Is(err, academic.ErrForbidden):
		return http.StatusForbidden, "Anda tidak memiliki akses ke kehadiran mandiri."
	case errors.Is(err, academic.ErrConflict):
		return http.StatusConflict, "Kehadiran sudah tercatat atau berubah. Periksa status terbaru sebelum mencoba lagi."
	case errors.Is(err, academic.ErrTeacherAttendanceAction):
		return http.StatusUnprocessableEntity, "Pilih tindakan Masuk atau Pulang."
	case errors.As(err, &validation):
		return http.StatusUnprocessableEntity, string(validation)
	case errors.Is(err, teacherphoto.ErrInvalidImage), errors.Is(err, teacherphoto.ErrUnsupportedMediaType), errors.Is(err, teacherphoto.ErrInputTooLarge), errors.Is(err, teacherphoto.ErrPhotoTooLarge), errors.Is(err, teacherphoto.ErrImageDimensions):
		return http.StatusUnprocessableEntity, "Selfie tidak valid. Ambil ulang foto melalui kamera."
	default:
		return http.StatusServiceUnavailable, "Hasil pencatatan belum dapat dipastikan. Coba kirim ulang dengan bukti yang sama atau hubungi TU Pengelola Kehadiran Guru."
	}
}

// teacherAttendanceSavedMessage membedakan retry lintas hari dari status hari ini.
func teacherAttendanceSavedMessage(today, recorded string) string {
	if _, err := time.Parse("2006-01-02", recorded); err == nil && recorded != today {
		return "Kehadiran untuk tanggal " + recorded + " telah tercatat. Status di bawah untuk hari ini."
	}
	return "Kehadiran berhasil dicatat."
}
