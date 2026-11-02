package web

import (
	"context"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestInstantSearchRenderContract(t *testing.T) {
	templates, e := template.New("web").Funcs(template.FuncMap{"rowNumber": func(p, i int) int { return i + 1 }, "weekdayName": func(i int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if e != nil {
		t.Fatal(e)
	}
	app := &application{templates: templates}
	for _, tc := range []struct {
		name, method, hx, list string
		partial                bool
	}{
		{"normal", "GET", "", "", false}, {"fragment", "GET", "true", "true", true}, {"hx biasa", "GET", "true", "", false}, {"mutasi tidak menjadi fragment", "POST", "true", "true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/admin/users", nil)
			r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &auth.User{ID: 1, Active: true, Roles: []string{"admin"}}))
			r.Header.Set("HX-Request", tc.hx)
			r.Header.Set("HX-List-Request", tc.list)
			w := httptest.NewRecorder()
			app.render(w, r, "page", 200, view{Page: "users", Title: "Pengguna"})
			body := w.Body.String()
			if w.Code != 200 {
				t.Fatal(w.Code, body)
			}
			if strings.Contains(body, "<!doctype html>") == tc.partial {
				t.Fatal("kontrak halaman/fragment salah")
			}
			if strings.Count(body, `id="list-region"`) != 1 {
				t.Fatal("region daftar hilang/duplikat")
			}
			if tc.partial && (strings.Contains(body, `id="app-sidebar"`) || strings.Contains(body, `<script src=`)) {
				t.Fatal("fragment membawa layout/script")
			}
		})
	}
}
