package web

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"time"
)

//go:embed templates/*.html static/*
var content embed.FS

type Check func(context.Context) error

func New(check Check) (http.Handler, error) {
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
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		name := "page"
		if r.Header.Get("HX-Request") == "true" {
			name = "overview"
		}
		w.Header().Set("Vary", "HX-Request")
		var buf bytes.Buffer
		if err := templates.ExecuteTemplate(&buf, name, nil); err != nil {
			http.Error(w, "halaman tidak dapat ditampilkan", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(buf.Bytes())
	})
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		protected.ServeHTTP(w, r)
	}), nil
}
