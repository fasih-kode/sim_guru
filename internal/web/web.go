package web

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

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
	mux.HandleFunc("GET /admin/placements", a.require("", a.studentPlacements))
	mux.HandleFunc("POST /admin/placements", a.require("admin", a.studentPlacements))
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
	// Class reads and history are authorized by domain; only Admin may write classes.
	mux.HandleFunc("GET /akademik/kelas", a.require("", a.academicClasses))
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
		protected.ServeHTTP(w, r)
	}), nil
}
