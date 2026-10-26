package web

import (
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type teacherLocationView struct {
	Config                                       academic.TeacherAttendanceLocationConfig
	History                                      []academic.History
	Latitude, Longitude, Radius, Version, Reason string
}

func (a *application) teacherAttendanceLocation(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	f := &teacherLocationView{}
	config, history, err := a.options.Academics.TeacherAttendanceLocation(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	f.Config, f.History = config, history
	f.Latitude = strconv.FormatFloat(config.Latitude, 'f', -1, 64)
	f.Longitude = strconv.FormatFloat(config.Longitude, 'f', -1, 64)
	if !config.Configured {
		f.Latitude, f.Longitude = "", ""
	}
	f.Radius = strconv.Itoa(config.RadiusM)
	f.Version = strconv.FormatInt(config.Version, 10)
	status := http.StatusOK
	v := view{Page: "teacher-attendance-location", Title: "Lokasi Kehadiran Guru", TeacherLocation: f}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		f.Latitude, f.Longitude = r.PostForm.Get("latitude"), r.PostForm.Get("longitude")
		f.Radius, f.Version, f.Reason = r.PostForm.Get("radius_m"), r.PostForm.Get("version"), r.PostForm.Get("reason")
		latitude, latErr := strconv.ParseFloat(f.Latitude, 64)
		longitude, lonErr := strconv.ParseFloat(f.Longitude, 64)
		radius, radiusErr := strconv.Atoi(f.Radius)
		version, versionErr := strconv.ParseInt(f.Version, 10, 64)
		if latErr != nil || lonErr != nil || radiusErr != nil || versionErr != nil {
			err = academic.ValidationError("Koordinat, radius, atau versi tidak valid.")
		} else {
			err = a.options.Academics.SaveTeacherAttendanceLocation(r.Context(), actor, academic.TeacherAttendanceLocationInput{
				Latitude: latitude, Longitude: longitude, RadiusM: radius, ExpectedVersion: version, Reason: f.Reason,
			})
		}
		if err == nil {
			http.Redirect(w, r, "/admin/attendance/location?saved=1", http.StatusSeeOther)
			return
		}
		status, v.Error = academicError(err)
		if status >= 500 {
			a.failure(w, err)
			return
		}
		if status == http.StatusConflict {
			v.Error = "Pengaturan lokasi berubah sejak halaman dibuka. Muat ulang untuk memeriksa versi terbaru sebelum menyimpan."
		}
		v.ModalOpen, v.ModalID = true, "teacher-location-modal"
		config, history, loadErr := a.options.Academics.TeacherAttendanceLocation(r.Context(), actor)
		if loadErr != nil {
			a.academicFailure(w, loadErr)
			return
		}
		f.Config, f.History = config, history
	} else if r.URL.Query().Get("saved") == "1" {
		v.Message = "Pengaturan lokasi tersimpan dan tercatat dalam histori."
	}
	a.render(w, r, "page", status, v)
}
