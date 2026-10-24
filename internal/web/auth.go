package web

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/gorilla/csrf"
	"github.com/jackc/pgx/v5/pgconn"
)

const sessionCookie = "sim_guru_session"

type Options struct {
	Accounts      *auth.Store
	Masters       *master.Store
	Academics     *academic.Store
	CSRFKey       []byte
	SecureCookies bool
}
type principalKey struct{}
type application struct {
	templates *template.Template
	options   Options
	limiter   loginLimiter
}
type roleChoice struct {
	Code, Name string
	Checked    bool
}
type view struct {
	Movement                    *movementView
	ASCImport                   *ascImportView
	User                        *auth.User
	CSRF                        template.HTML
	Page, Title, Error, Message string
	NavActive, NavGroup         string
	ModalOpen                   bool
	ModalID                     string
	Users                       []auth.User
	Input                       auth.Input
	Editing                     int64
	RoleChoices                 []roleChoice
	Audits                      []auth.Audit
	Filter                      auth.Filter
	Previous, Next              int
	Master                      *masterView
	Import                      *importView
	Academic                    *academicView
	Placement                   *placementView
	PlacementHistory            []academic.PlacementHistory
	Transition                  *transitionView
}

func principal(r *http.Request) *auth.User {
	u, _ := r.Context().Value(principalKey{}).(*auth.User)
	return u
}
func (a *application) render(w http.ResponseWriter, r *http.Request, name string, status int, v view) {
	v.User = principal(r)
	v.CSRF = csrf.TemplateField(r)
	v.NavActive, v.NavGroup = sidebarState(r.URL.Path, v.User)
	var buf bytes.Buffer
	if err := a.templates.ExecuteTemplate(&buf, name, v); err != nil {
		slog.Error("template gagal", "template", name, "error", err)
		http.Error(w, "Halaman tidak dapat ditampilkan.", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func sidebarState(path string, user *auth.User) (active, group string) {
	adminMenu := user != nil && user.HasRole("admin")
	roleMenu := "curriculum"
	if adminMenu {
		roleMenu = "admin"
	}
	switch {
	case path == "/":
		return "home", "menu"
	case strings.HasPrefix(path, "/akademik/penempatan/"):
		if roleMenu == "curriculum" {
			return "placements", "curriculum-students"
		}
		return "placements", roleMenu
	case strings.HasPrefix(path, "/students/"):
		if roleMenu == "curriculum" {
			return "students", "curriculum-students"
		}
		return "students", roleMenu
	case strings.HasPrefix(path, "/master/students"):
		if roleMenu == "curriculum" {
			return "students", "curriculum-students"
		}
		return "students", roleMenu
	case strings.HasPrefix(path, "/master/teachers"):
		if roleMenu == "curriculum" {
			return "teachers", "curriculum-students"
		}
		return "teachers", roleMenu
	case strings.HasPrefix(path, "/master/subjects"):
		return "subjects", "curriculum-subjects"
	case strings.HasPrefix(path, "/admin/placements"), strings.HasPrefix(path, "/akademik/histori/penempatan/"):
		if roleMenu == "curriculum" {
			return "placements", "curriculum-students"
		}
		return "placements", roleMenu
	case strings.HasPrefix(path, "/akademik/transisi"):
		return "transitions", "curriculum-students"
	case strings.HasPrefix(path, "/admin/users"):
		return "users", "admin"
	case strings.HasPrefix(path, "/admin/audit"):
		return "audit", "admin"
	case path == "/akademik/tahun-ajaran" || strings.HasPrefix(path, "/akademik/tahun-ajaran/"):
		return "years", "curriculum-calendar"
	case path == "/akademik/semester" || strings.HasPrefix(path, "/akademik/semester/"):
		return "semesters", "curriculum-calendar"
	case path == "/akademik/pengampu":
		return "teaching-assignments", "curriculum-schedule"
	case path == "/akademik/jam-pelajaran":
		return "lesson-periods", "curriculum-schedule"
	case path == "/akademik/struktur-kurikulum":
		return "curriculum-structure", "curriculum-subjects"
	case path == "/akademik/batasan-jadwal":
		return "schedule-constraints", "curriculum-schedule"
	case path == "/akademik/waktu-sekolah":
		return "school-time-blocks", "curriculum-schedule"
	case path == "/akademik/jadwal" || path == "/akademik/jadwal/impor":
		return "weekly-timetable", "curriculum-schedule"
	case path == "/jadwal-saya":
		return "my-schedule", "schedule"
	case strings.HasPrefix(path, "/wali-kelas"):
		return "my-classes", "wali"
	case path == "/akademik/kelas" || strings.HasPrefix(path, "/akademik/kelas/") || strings.HasPrefix(path, "/akademik/histori/class/"):
		if roleMenu == "curriculum" {
			return "classes", "curriculum-students"
		}
		return "classes", roleMenu
	case path == "/akademik":
		return "academic", "curriculum-calendar"
	case path == "/account/password":
		return "password", "account"
	default:
		return "", ""
	}
}
func (a *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookie); err == nil {
			u, err := a.options.Accounts.Session(r.Context(), cookie.Value)
			if err == nil {
				r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &u))
			} else if errors.Is(err, auth.ErrSession) || errors.Is(err, auth.ErrNotFound) {
				a.clearSession(w)
			} else {
				a.failure(w, err)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (a *application) require(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := principal(r)
		if u == nil {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
			return
		}
		if role != "" && !u.HasRole(role) {
			http.Error(w, "Anda tidak memiliki akses ke halaman ini.", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
func (a *application) failure(w http.ResponseWriter, err error) {
	// Never log raw SQL errors: driver errors can include submitted credentials or personal data.
	var pgError *pgconn.PgError
	category := "storage_or_internal"
	if errors.As(err, &pgError) {
		category = "sqlstate_" + pgError.Code
	} else if errors.Is(err, context.DeadlineExceeded) {
		category = "timeout"
	}
	slog.Error("operasi akun gagal", "category", category)
	http.Error(w, "Layanan belum dapat memproses permintaan. Coba kembali.", http.StatusInternalServerError)
}
func publicError(err error) (int, string) {
	var validation auth.ValidationError
	switch {
	case errors.As(err, &validation):
		return 422, err.Error()
	case errors.Is(err, auth.ErrCredentials):
		return 401, err.Error()
	case errors.Is(err, auth.ErrForbidden):
		return 403, err.Error()
	case errors.Is(err, auth.ErrNotFound):
		return 404, err.Error()
	case errors.Is(err, auth.ErrConflict), errors.Is(err, auth.ErrLastAdmin):
		return 409, err.Error()
	default:
		return 500, "Layanan belum dapat memproses permintaan. Coba kembali."
	}
}
func parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form tidak valid atau terlalu besar.", http.StatusBadRequest)
		return false
	}
	return true
}
func (a *application) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: a.options.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(auth.SessionLifetime.Seconds())})
}
func (a *application) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.options.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (a *application) login(w http.ResponseWriter, r *http.Request) {
	if principal(r) != nil {
		http.Redirect(w, r, "/", 303)
		return
	}
	v := view{Title: "Masuk"}
	if r.Method == "GET" {
		if r.URL.Query().Get("changed") == "1" {
			v.Message = "Kata sandi diubah. Silakan masuk kembali."
		}
		a.render(w, r, "login", 200, v)
		return
	}
	if !parseForm(w, r) {
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	username := auth.NormalizeUsername(r.PostForm.Get("username"))
	keyUsername := username
	if len(keyUsername) > 64 {
		keyUsername = "invalid_username"
	}
	if !a.limiter.allow(ip, keyUsername, time.Now()) {
		w.Header().Set("Retry-After", "900")
		v.Error = "Terlalu banyak percobaan masuk. Coba kembali dalam 15 menit."
		a.render(w, r, "login", 429, v)
		return
	}
	password := r.PostForm.Get("password")
	if len(password) > 72 || len(username) > 64 {
		v.Error = auth.ErrCredentials.Error()
		a.render(w, r, "login", 401, v)
		return
	}
	old := ""
	if c, err := r.Cookie(sessionCookie); err == nil {
		old = c.Value
	}
	token, err := a.options.Accounts.Login(r.Context(), username, password, old)
	if err != nil {
		code, message := publicError(err)
		v.Error = message
		a.render(w, r, "login", code, v)
		return
	}
	a.setSession(w, token)
	http.Redirect(w, r, "/", 303)
}
func (a *application) logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err == nil {
		if err = a.options.Accounts.Logout(r.Context(), c.Value); err != nil {
			a.failure(w, err)
			return
		}
	}
	a.clearSession(w)
	http.Redirect(w, r, "/login", 303)
}
func (a *application) password(w http.ResponseWriter, r *http.Request) {
	v := view{Page: "password", Title: "Kata sandi"}
	if r.Method == "GET" {
		v.ModalOpen = r.URL.Query().Get("modal") == "1"
		a.render(w, r, "page", 200, v)
		return
	}
	if !parseForm(w, r) {
		return
	}
	if r.PostForm.Get("password") != r.PostForm.Get("confirm_password") {
		v.ModalOpen = true
		v.Error = "Konfirmasi kata sandi tidak sesuai."
		a.render(w, r, "page", 422, v)
		return
	}
	err := a.options.Accounts.ChangePassword(r.Context(), principal(r).ID, r.PostForm.Get("current_password"), r.PostForm.Get("password"))
	if err != nil {
		v.ModalOpen = true
		code, message := publicError(err)
		v.Error = message
		a.render(w, r, "page", code, v)
		return
	}
	a.clearSession(w)
	http.Redirect(w, r, "/login?changed=1", 303)
}
func (a *application) users(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	f := auth.Filter{Search: strings.TrimSpace(r.URL.Query().Get("q")), Role: r.URL.Query().Get("role"), Status: r.URL.Query().Get("status"), Page: page}
	if f.Status != "" && f.Status != "active" && f.Status != "inactive" {
		http.Error(w, "Filter status tidak valid.", 400)
		return
	}
	if f.Role != "" {
		valid := false
		for _, role := range auth.Roles {
			if role.Code == f.Role {
				valid = true
			}
		}
		if !valid {
			http.Error(w, "Filter role tidak valid.", 400)
			return
		}
	}
	if len(f.Search) > 120 {
		http.Error(w, "Pencarian terlalu panjang.", 400)
		return
	}
	users, err := a.options.Accounts.List(r.Context(), f)
	if err != nil {
		code, message := publicError(err)
		http.Error(w, message, code)
		return
	}
	v := view{Page: "users", Title: "Pengguna dan role", Users: users, Filter: f}
	if page > 1 {
		v.Previous = page - 1
	}
	if len(users) > 10 {
		v.Next = page + 1
		users = users[:10]
		v.Users = users
	}
	for _, role := range auth.Roles {
		v.RoleChoices = append(v.RoleChoices, roleChoice{Code: role.Code, Name: role.Name})
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Perubahan akun dan role tersimpan."
	}
	a.render(w, r, "page", 200, v)
}
func (a *application) userForm(w http.ResponseWriter, r *http.Request) {
	id := int64(0)
	if raw := r.PathValue("id"); raw != "" {
		var err error
		id, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			http.NotFound(w, r)
			return
		}
	}
	v := view{Page: "users", Title: "Pengguna dan role", ModalOpen: true, Editing: id, Input: auth.Input{Active: true}}
	users, err := a.options.Accounts.List(r.Context(), auth.Filter{Page: 1})
	if err != nil {
		a.failure(w, err)
		return
	}
	if len(users) > 10 {
		users = users[:10]
	}
	v.Users = users
	if id != 0 {
		u, err := a.options.Accounts.Get(r.Context(), id)
		if err != nil {
			code, message := publicError(err)
			http.Error(w, message, code)
			return
		}
		v.Input = auth.Input{Username: u.Username, DisplayName: u.DisplayName, Active: u.Active, Roles: u.Roles, Version: u.Version}
	}
	status := 200
	if r.Method == "POST" {
		if !parseForm(w, r) {
			return
		}
		version, _ := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
		v.Input = auth.Input{Username: r.PostForm.Get("username"), DisplayName: r.PostForm.Get("display_name"), Active: r.PostForm.Get("active") == "true", Roles: r.PostForm["roles"], Password: r.PostForm.Get("password"), Version: version}
		var err error
		if id == 0 {
			_, err = a.options.Accounts.Create(r.Context(), principal(r).ID, v.Input)
		} else {
			_, err = a.options.Accounts.Update(r.Context(), principal(r).ID, id, v.Input)
		}
		v.Input.Password = "" // Never render credentials back into HTML.
		if err == nil {
			http.Redirect(w, r, "/admin/users?saved=1", 303)
			return
		}
		status, v.Error = publicError(err)
	}
	for _, role := range auth.Roles {
		checked := false
		for _, code := range v.Input.Roles {
			if code == role.Code {
				checked = true
			}
		}
		v.RoleChoices = append(v.RoleChoices, roleChoice{role.Code, role.Name, checked})
	}
	a.render(w, r, "page", status, v)
}
func (a *application) audit(w http.ResponseWriter, r *http.Request) {
	rows, err := a.options.Accounts.Audits(r.Context())
	if err != nil {
		a.failure(w, err)
		return
	}
	for i := range rows {
		rows[i].At = rows[i].At.In(time.FixedZone("WIB", 7*60*60))
	}
	a.render(w, r, "page", 200, view{Page: "audit", Title: "Riwayat perubahan akses", Audits: rows})
}

// A bounded, process-local limiter. RemoteAddr is used; spoofable forwarding headers are ignored.
type bucket struct {
	count   int
	expires time.Time
}
type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
}

func (l *loginLimiter) allow(ip, username string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = make(map[string]bucket)
	}
	for k, b := range l.buckets {
		if !now.Before(b.expires) {
			delete(l.buckets, k)
		}
	}
	if len(l.buckets) > 10000 {
		return false
	}
	keys := []string{"ip:" + ip, "user:" + username}
	limits := []int{30, 10}
	for i, k := range keys {
		if l.buckets[k].count >= limits[i] {
			return false
		}
	}
	for _, k := range keys {
		b := l.buckets[k]
		if b.count == 0 {
			b.expires = now.Add(15 * time.Minute)
		}
		b.count++
		l.buckets[k] = b
	}
	return true
}
