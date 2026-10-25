package web

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/gorilla/csrf"
)

//go:embed templates/*.html static/* import_templates/*.xlsx
var content embed.FS

type Check func(context.Context) error

func New(check Check, options Options) (http.Handler, error) {
	if options.Accounts == nil || options.Masters == nil || options.Academics == nil || len(options.CSRFKey) != 32 {
		return nil, errors.New("auth/master/academic store dan CSRF key 32 byte wajib tersedia")
	}
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber": func(page, index int) int {
			if page < 1 {
				page = 1
			}
			return (page-1)*10 + index + 1
		},
		"weekdayName": func(day int) string {
			names := []string{"", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}
			if day < 1 || day >= len(names) {
				return strconv.Itoa(day)
			}
			return names[day]
		},
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		return nil, err
	}
	assets, err := fs.Sub(content, "static")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if check(ctx) != nil {
			http.Error(w, "belum siap", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ready\n"))
	})
	a := &application{templates: templates, options: options}
	mux.HandleFunc("GET /kesiswaan/batas-kedatangan", a.require("kesiswaan", a.arrivalRules))
	mux.HandleFunc("POST /kesiswaan/batas-kedatangan", a.require("kesiswaan", a.arrivalRules))
	mux.HandleFunc("GET /absensi/harian/cepat", a.require("petugas_absensi", a.dailyQuick))
	mux.HandleFunc("POST /absensi/harian/cepat", a.require("petugas_absensi", a.dailyQuick))
	mux.HandleFunc("GET /absensi/harian", a.require("", a.dailyAttendance))
	mux.HandleFunc("POST /absensi/harian", a.require("petugas_absensi", a.dailyAttendance))
	mux.HandleFunc("GET /absensi/pemindai", a.require("petugas_absensi", a.dailyScanner))
	mux.HandleFunc("POST /absensi/scan", a.require("petugas_absensi", a.dailyScan))
	mux.HandleFunc("GET /absensi/kartu", a.require("petugas_absensi", a.studentCards))
	mux.HandleFunc("POST /absensi/kartu", a.require("petugas_absensi", a.studentCards))
	mux.HandleFunc("GET /absensi/kartu/{id}/cetak", a.require("petugas_absensi", a.printStudentCard))
	mux.HandleFunc("GET /akademik/kalender-sekolah", a.require("kurikulum", a.schoolCalendar))
	mux.HandleFunc("POST /akademik/kalender-sekolah", a.require("kurikulum", a.schoolCalendar))
	mux.HandleFunc("GET /admin/placements/dates/bulk", a.require("admin", a.placementDatesBulk))
	mux.HandleFunc("POST /admin/placements/dates/bulk", a.require("admin", a.placementDatesBulk))
	mux.HandleFunc("GET /admin/placements/dates", a.require("admin", a.placementDates))
	mux.HandleFunc("POST /admin/placements/dates", a.require("admin", a.placementDates))
	mux.HandleFunc("GET /login", a.login)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", a.require("", a.logout))
	mux.HandleFunc("GET /account/password", a.require("", a.password))
	mux.HandleFunc("POST /account/password", a.require("", a.password))
	mux.HandleFunc("GET /admin/users", a.require("admin", a.users))
	mux.HandleFunc("GET /admin/users/new", a.require("admin", a.userForm))
	mux.HandleFunc("POST /admin/users/new", a.require("admin", a.userForm))
	mux.HandleFunc("GET /admin/users/{id}/edit", a.require("admin", a.userForm))
	mux.HandleFunc("POST /admin/users/{id}/edit", a.require("admin", a.userForm))
	mux.HandleFunc("GET /admin/audit", a.require("admin", a.audit))
	mux.HandleFunc("GET /akademik/penempatan/{id}/pindah-kelas", a.require("kurikulum", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mode", "pindah-kelas")
		a.studentMovement(w, r)
	}))
	mux.HandleFunc("POST /akademik/penempatan/{id}/pindah-kelas", a.require("kurikulum", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mode", "pindah-kelas")
		a.studentMovement(w, r)
	}))
	mux.HandleFunc("GET /master/students/{id}/pindah-sekolah", a.require("admin", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mode", "pindah-sekolah")
		a.studentMovement(w, r)
	}))
	mux.HandleFunc("POST /master/students/{id}/pindah-sekolah", a.require("admin", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mode", "pindah-sekolah")
		a.studentMovement(w, r)
	}))
	mux.HandleFunc("GET /master/students/{id}/terima-kembali", a.require("admin", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mode", "terima-kembali")
		a.studentMovement(w, r)
	}))
	mux.HandleFunc("POST /master/students/{id}/terima-kembali", a.require("admin", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mode", "terima-kembali")
		a.studentMovement(w, r)
	}))
	mux.HandleFunc("GET /students/{id}/perpindahan", a.require("", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mode", "riwayat")
		a.studentMovement(w, r)
	}))
	mux.HandleFunc("GET /admin/placements", a.require("", a.studentPlacements))
	mux.HandleFunc("POST /admin/placements", a.require("admin", a.studentPlacements))
	mux.HandleFunc("GET /akademik/transisi", a.require("kurikulum", a.studentTransition))
	mux.HandleFunc("POST /akademik/transisi", a.require("kurikulum", a.studentTransition))
	mux.HandleFunc("GET /master/{kind}/import", a.require("", a.masterImport))
	mux.HandleFunc("POST /master/{kind}/import", a.require("", a.masterImport))
	mux.HandleFunc("GET /master/{kind}/import/template", a.require("", a.importTemplate))
	mux.HandleFunc("POST /master/{kind}/import/confirm", a.require("", a.confirmImport))
	mux.HandleFunc("GET /master/{kind}", a.require("", a.masterList))
	mux.HandleFunc("GET /master/{kind}/new", a.require("", a.masterForm))
	mux.HandleFunc("POST /master/{kind}/new", a.require("", a.masterForm))
	mux.HandleFunc("GET /master/{kind}/{id}", a.require("", a.masterDetail))
	mux.HandleFunc("GET /master/{kind}/{id}/edit", a.require("", a.masterForm))
	mux.HandleFunc("POST /master/{kind}/{id}/edit", a.require("", a.masterForm))
	mux.HandleFunc("GET /master/{kind}/{id}/history", a.require("", a.masterHistory))
	mux.HandleFunc("GET /akademik", a.require("kurikulum", a.academicHome))
	mux.HandleFunc("GET /akademik/tahun-ajaran", a.require("kurikulum", a.academicYears))
	mux.HandleFunc("POST /akademik/tahun-ajaran", a.require("kurikulum", a.academicYears))
	mux.HandleFunc("POST /akademik/tahun-ajaran/{id}/aktifkan", a.require("kurikulum", a.activateYear))
	mux.HandleFunc("GET /akademik/semester", a.require("kurikulum", a.academicSemesters))
	mux.HandleFunc("POST /akademik/semester/{id}/aktifkan", a.require("kurikulum", a.activateSemester))
	mux.HandleFunc("GET /akademik/pengampu", a.require("kurikulum", a.teachingAssignments))
	mux.HandleFunc("POST /akademik/pengampu", a.require("kurikulum", a.teachingAssignments))
	mux.HandleFunc("GET /akademik/jam-pelajaran", a.require("kurikulum", a.lessonPeriods))
	mux.HandleFunc("POST /akademik/jam-pelajaran", a.require("kurikulum", a.lessonPeriods))
	mux.HandleFunc("GET /akademik/struktur-kurikulum", a.require("kurikulum", a.curriculumSettings))
	mux.HandleFunc("POST /akademik/struktur-kurikulum", a.require("kurikulum", a.curriculumSettings))
	mux.HandleFunc("GET /akademik/waktu-sekolah", a.require("kurikulum", a.curriculumSettings))
	mux.HandleFunc("POST /akademik/waktu-sekolah", a.require("kurikulum", a.curriculumSettings))
	mux.HandleFunc("GET /akademik/batasan-jadwal", a.require("kurikulum", a.scheduleConstraints))
	mux.HandleFunc("POST /akademik/batasan-jadwal", a.require("kurikulum", a.scheduleConstraints))
	mux.HandleFunc("GET /akademik/jadwal/impor", a.require("kurikulum", a.timetableXMLImport))
	mux.HandleFunc("POST /akademik/jadwal/impor", a.require("kurikulum", a.timetableXMLImport))
	mux.HandleFunc("GET /akademik/jadwal", a.require("kurikulum", a.weeklyTimetable))
	mux.HandleFunc("POST /akademik/jadwal", a.require("kurikulum", a.weeklyTimetable))
	mux.HandleFunc("GET /jadwal-saya", a.require("guru_mapel", a.myTimetable))
	// Class reads and history are authorized by domain; only Admin may write classes.
	mux.HandleFunc("GET /akademik/kelas", a.require("", a.academicClasses))
	mux.HandleFunc("GET /akademik/kelas/{id}/wali", a.require("kurikulum", a.classHomeroom))
	mux.HandleFunc("POST /akademik/kelas/{id}/wali", a.require("kurikulum", a.classHomeroom))
	mux.HandleFunc("GET /akademik/kelas/{id}/wali/histori", a.require("kurikulum", a.classHomeroomHistory))
	mux.HandleFunc("GET /wali-kelas", a.require("wali_kelas", a.myHomerooms))
	mux.HandleFunc("GET /wali-kelas/{id}", a.require("wali_kelas", a.myHomeroomRoster))
	mux.HandleFunc("GET /akademik/kelas/new", a.require("admin", a.academicClassForm))
	mux.HandleFunc("POST /akademik/kelas/new", a.require("admin", a.academicClassForm))
	mux.HandleFunc("GET /akademik/kelas/{id}/edit", a.require("admin", a.academicClassForm))
	mux.HandleFunc("POST /akademik/kelas/{id}/edit", a.require("admin", a.academicClassForm))
	mux.HandleFunc("GET /akademik/histori/{entity}/{id}", a.require("", a.academicHistory))
	mux.HandleFunc("GET /akademik/histori/penempatan/{id}", a.require("", a.placementHistory))
	mux.HandleFunc("GET /{$}", a.require("", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "HX-Request")
		name := "page"
		if r.Header.Get("HX-Request") == "true" {
			name = "overview"
		}
		a.render(w, r, name, 200, view{Page: "home", Title: "Beranda"})
	}))
	csrfMiddleware := csrf.Protect(options.CSRFKey, csrf.Secure(options.SecureCookies), csrf.Path("/"), csrf.SameSite(csrf.SameSiteLaxMode), csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Form kedaluwarsa atau tidak sah. Muat ulang halaman dan coba kembali.", 403)
	})))
	authenticated := a.authenticate(mux)
	protectedApp := csrfMiddleware(authenticated)
	appHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/health/") {
			mux.ServeHTTP(w, r)
			return
		}
		limit := int64(32 << 10)
		if r.Method == "POST" && (r.URL.Path == "/master/students/import" || r.URL.Path == "/master/teachers/import") {
			limit = master.MaxImportBytes + (256 << 10)
		}
		if r.Method == "POST" && r.URL.Path == "/akademik/jadwal/impor" {
			if r.URL.Query().Get("token") == "" {
				limit = academic.MaxASCBytes + (256 << 10)
			} else {
				limit = 512 << 10
			}
		}
		if r.ContentLength > limit {
			http.Error(w, "Ukuran unggahan melebihi batas.", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		defer func() {
			if r.MultipartForm != nil {
				r.MultipartForm.RemoveAll()
			}
		}()
		if !options.SecureCookies {
			r = csrf.PlaintextHTTPRequest(r)
		}
		protectedApp.ServeHTTP(w, r)
	})
	protected := http.NewCrossOriginProtection().Handler(appHandler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if r.URL.Path == "/absensi/pemindai" {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; worker-src 'self' blob:; media-src 'self' blob:; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		}
		protected.ServeHTTP(w, r)
	}), nil
}
