package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func TestTeacherAttendanceLocationRenderUsesWIB(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(int) string { return "Senin" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}

	utc := time.Date(2026, time.December, 30, 18, 0, 0, 123456789, time.UTC)
	config := academic.TeacherAttendanceLocationConfig{Latitude: -6.2, Longitude: 106.8, RadiusM: 100, Version: 2, UpdatedAt: utc, Configured: true}
	location := &teacherLocationView{
		Config:  config,
		History: []academic.History{{Actor: "Admin Uji", Reason: "Perubahan lokasi", At: utc}},
	}
	normalizeTeacherLocationViewTimes(location)

	var rendered bytes.Buffer
	data := view{Page: "teacher-attendance-location", TeacherLocation: location}
	if err := templates.ExecuteTemplate(&rendered, "teacher-attendance-location", data); err != nil {
		t.Fatal(err)
	}
	body := rendered.String()
	want := "31 Dec 2026 01:00 WIB"
	if !strings.Contains(body, "diperbarui "+want) || !strings.Contains(body, "Admin Uji · "+want) {
		t.Fatalf("configuration and history times should render in WIB; body=%s", body)
	}
	if !location.Config.UpdatedAt.Equal(utc) || !location.History[0].At.Equal(utc) || config.UpdatedAt.Location() != time.UTC {
		t.Fatal("WIB normalization changed the configuration/history instant or source timestamp")
	}
}
