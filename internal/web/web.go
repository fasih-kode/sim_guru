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

	"github.com/gorilla/csrf"
)

//go:embed templates/*.html static/*
var content embed.FS

type Check func(context.Context) error

func New(check Check, options Options) (http.Handler, error) {
	if options.Accounts == nil || len(options.CSRFKey) != 32 {
		return nil, errors.New("auth store dan CSRF key 32 byte wajib tersedia")
	}
	templates, err := template.ParseFS(content, "templates/*.html")
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
	mux.HandleFunc("GET /akademik", a.require("kurikulum", func(w http.ResponseWriter, r *http.Request) {
		a.render(w, r, "page", 200, view{Page: "academic", Title: "Akademik"})
	}))
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
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
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
