package web

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
)

func TestStaffAttendanceHTTPErrorExplainsOperationalFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		messageHas string
		class      string
	}{
		{"storage belum dikonfigurasi", academic.ErrStaffAttendancePhoto, http.StatusServiceUnavailable, "Penyimpanan foto kehadiran Pegawai TU belum aktif", "photo_storage"},
		{"storage foto gagal", teacherphoto.ErrStorage, http.StatusServiceUnavailable, "Penyimpanan selfie sedang bermasalah", "photo_storage"},
		{"storage tertutup", teacherphoto.ErrClosed, http.StatusServiceUnavailable, "Penyimpanan selfie sedang bermasalah", "photo_storage"},
		{"gagal tulis database", academic.ErrStaffAttendanceWrite, http.StatusServiceUnavailable, "Server gagal menyimpan catatan presensi", "database_write"},
		{"pembersihan bukti perlu rekonsiliasi", academic.ErrTeacherAttendanceCleanup, http.StatusServiceUnavailable, "Penyimpanan bukti perlu diperiksa Admin", "photo_cleanup"},
		{"commit belum pasti tetap idempoten", academic.ErrTeacherAttendanceCommit, http.StatusServiceUnavailable, "Hasil pencatatan belum dapat dipastikan", "commit_uncertain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := staffAttendanceHTTPError(tt.err)
			if status != tt.status || !strings.Contains(message, tt.messageHas) {
				t.Fatalf("status=%d message=%q", status, message)
			}
			if got := staffAttendanceFailureClass(tt.err); got != tt.class {
				t.Fatalf("class=%q want=%q", got, tt.class)
			}
		})
	}
}

func TestStaffAttendanceHistoryHasMobileCardsAndDesktopTable(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(int) string { return "Senin" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	data := view{
		Page: "staff-attendance", Title: "Kehadiran Saya",
		User: &auth.User{Roles: []string{"pegawai_tu"}},
		StaffAttendance: &staffAttendanceView{
			Day: academic.StaffAttendanceDay{Name: "Pegawai Uji", Date: "2026-10-30", Active: true},
			History: academic.StaffAttendanceHistory{Page: 1, Next: 2, Events: []academic.StaffAttendanceEvent{{
				PhotoID: 18, Date: "2026-10-30", Action: academic.TeacherCheckIn,
				OccurredAt: time.Date(2026, 10, 30, 7, 56, 0, 0, time.FixedZone("WIB", 7*60*60)), DistanceM: 32, AccuracyM: 26,
			}}},
			HistoryBase: "/pegawai/kehadiran-saya",
		},
	}
	var out bytes.Buffer
	if err = templates.ExecuteTemplate(&out, "page", data); err != nil {
		t.Fatal(err)
	}
	page := out.String()
	for _, want := range []string{
		`data-staff-attendance-mobile-history`, `class="mt-4 grid gap-3 sm:hidden"`,
		`class="table-surface hidden sm:block"`, `2026-10-30`, `Masuk`, `07:56 WIB`,
		`32 m · akurasi 26 m`, `href="/pegawai/kehadiran-saya/foto/18"`,
		`aria-label="Halaman riwayat kehadiran"`, `href="/pegawai/kehadiran-saya?page=2"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("tampilan riwayat tidak memuat %q", want)
		}
	}
}
