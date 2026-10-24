//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/database"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/migrations"
	"github.com/joho/godotenv"
)

func TestAccountsHTTPIntegration(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil && !os.IsNotExist(err) {
		t.Fatal("cannot read test configuration")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || dsn == "" || !strings.HasSuffix(parsed.Path, "_test") || dsn == os.Getenv("DATABASE_URL") {
		t.Fatal("a separate _test database is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var actual string
	var privileged bool
	if err = db.QueryRowContext(ctx, `SELECT current_database(),rolsuper OR rolcreatedb OR rolcreaterole FROM pg_roles WHERE rolname=current_user`).Scan(&actual, &privileged); err != nil || privileged || !strings.HasSuffix(actual, "_test") {
		t.Fatal("unsafe test database")
	}
	provider, err := migrations.Provider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	random := func() string {
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(b)
	}
	suffix := random()[:16]
	password := random()
	adminInput := auth.Input{Username: "test_admin_" + suffix, DisplayName: "Test Admin", Password: password, Active: true, Roles: []string{"admin"}}
	admin, bootstrapErr := accounts.Bootstrap(ctx, adminInput)
	if bootstrapErr != nil {
		var validation auth.ValidationError
		if !errors.As(bootstrapErr, &validation) {
			t.Fatal(bootstrapErr)
		}
		// Retain existing test data. Insert only this run's independent fixture, without changing any existing account.
		hash, err := auth.HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO app.users(username,display_name,password_hash) VALUES($1,$2,$3) RETURNING id`, adminInput.Username, adminInput.DisplayName, hash).Scan(&admin.ID)
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'admin')`, admin.ID); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		admin, err = accounts.Get(ctx, admin.ID)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		// A freshly bootstrapped store must protect its sole active administrator.
		_, err = accounts.Update(ctx, admin.ID, admin.ID, auth.Input{Username: admin.Username, DisplayName: admin.DisplayName, Active: false, Roles: []string{"admin"}, Version: admin.Version})
		if !errors.Is(err, auth.ErrLastAdmin) {
			t.Fatal("last administrator was not protected")
		}
	}
	if _, err = accounts.Bootstrap(ctx, adminInput); err == nil {
		t.Fatal("bootstrap allowed over existing accounts")
	}
	key := make([]byte, 32)
	rand.Read(key)
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db}, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	bodyOf := func(resp *http.Response) string {
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	get := func(c *http.Client, path string, status int) string {
		resp, err := c.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := bodyOf(resp)
		if resp.StatusCode != status {
			t.Fatalf("GET %s: status %d expected %d", path, resp.StatusCode, status)
		}
		return body
	}
	csrfPattern := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	tokenFrom := func(body string) string {
		m := csrfPattern.FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatal("CSRF field missing")
		}
		return m[1]
	}
	post := func(c *http.Client, path string, values url.Values, status int) string {
		resp, err := c.PostForm(server.URL+path, values)
		if err != nil {
			t.Fatal(err)
		}
		body := bodyOf(resp)
		if resp.StatusCode != status {
			t.Fatalf("POST %s: status %d expected %d", path, resp.StatusCode, status)
		}
		return body
	}
	login := func(c *http.Client, name, pass string) {
		token := tokenFrom(get(c, "/login", 200))
		post(c, "/login", url.Values{"gorilla.csrf.Token": {token}, "username": {name}, "password": {pass}}, 303)
	}
	sessionToken := func(c *http.Client) string {
		base, _ := url.Parse(server.URL)
		for _, cookie := range c.Jar.Cookies(base) {
			if cookie.Name == sessionCookie {
				return cookie.Value
			}
		}
		t.Fatal("session cookie missing")
		return ""
	}
	adminClient := newClient()
	get(adminClient, "/admin/users", 303)
	post(adminClient, "/login", url.Values{"username": {admin.Username}, "password": {password}}, 403)
	loginPage := get(adminClient, "/login", 200)
	csrfToken := tokenFrom(loginPage)
	failure := post(adminClient, "/login", url.Values{"gorilla.csrf.Token": {csrfToken}, "username": {admin.Username}, "password": {random()}}, 401)
	unknown := post(adminClient, "/login", url.Values{"gorilla.csrf.Token": {csrfToken}, "username": {"' OR 1=1 --"}, "password": {random()}}, 401)
	if !strings.Contains(failure, auth.ErrCredentials.Error()) || !strings.Contains(unknown, auth.ErrCredentials.Error()) {
		t.Fatal("login exposed account existence")
	}
	login(adminClient, admin.Username, password)
	dashboard := get(adminClient, "/", 200)
	if !strings.Contains(dashboard, "Pengguna dan Role") || strings.Contains(dashboard, password) {
		t.Fatal("invalid administrator dashboard")
	}
	// A missing session logout is harmless and does not create an audit entry.
	if err = accounts.Logout(ctx, random()); err != nil {
		t.Fatal(err)
	}
	rawToken := sessionToken(adminClient)
	var stored string
	if err = db.QueryRowContext(ctx, `SELECT token_hash FROM app.sessions WHERE token_hash=$1`, auth.TokenHash(rawToken)).Scan(&stored); err != nil || stored == rawToken {
		t.Fatal("raw session token stored")
	}
	get(adminClient, "/akademik", 403)
	teacherPassword := random()
	username := "test_guru_" + suffix
	form := get(adminClient, "/admin/users/new", 200)
	post(adminClient, "/admin/users/new", url.Values{"gorilla.csrf.Token": {tokenFrom(form)}, "username": {username}, "display_name": {"Guru <script>alert(1)</script>"}, "password": {teacherPassword}, "active": {"true"}, "roles": {"guru_mapel"}}, 303)
	var teacherID int64
	if err = db.QueryRowContext(ctx, `SELECT id FROM app.users WHERE username=$1`, username).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	teacher, err := accounts.Get(ctx, teacherID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = accounts.Create(ctx, admin.ID, auth.Input{Username: username, DisplayName: "Duplicate", Password: random(), Active: true, Roles: []string{"admin"}}); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("duplicate username accepted")
	}
	teacherClient := newClient()
	login(teacherClient, username, teacherPassword)
	teacherPage := get(teacherClient, "/", 200)
	if strings.Contains(teacherPage, "<script>alert(1)</script>") || strings.Contains(teacherPage, "/admin/users") {
		t.Fatal("XSS or admin menu leaked")
	}

	reqHX, _ := http.NewRequest("GET", server.URL+"/", nil)
	reqHX.Header.Set("HX-Request", "true")
	fragmentResp, err := teacherClient.Do(reqHX)
	if err != nil {
		t.Fatal(err)
	}
	fragmentBody := bodyOf(fragmentResp)
	if fragmentResp.StatusCode != 200 || fragmentResp.Header.Get("Vary") != "HX-Request" || strings.Contains(fragmentBody, "<!doctype html>") {
		t.Fatal("invalid authenticated htmx fragment")
	}
	get(teacherClient, "/admin/users", 403)
	get(teacherClient, "/admin/audit", 403)
	get(teacherClient, "/akademik", 403)
	teacherCSRF := tokenFrom(get(teacherClient, "/account/password", 200))
	post(teacherClient, "/admin/users/new", url.Values{"gorilla.csrf.Token": {teacherCSRF}}, 403)
	if _, err = accounts.Create(ctx, teacherID, adminInput); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("service allowed privilege escalation")
	}
	updated, err := accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: teacher.DisplayName, Active: true, Roles: []string{"kurikulum", "guru_mapel"}, Version: teacher.Version})
	if err != nil {
		t.Fatal(err)
	}
	get(teacherClient, "/akademik", 200)
	if !strings.Contains(get(teacherClient, "/", 200), "/akademik") {
		t.Fatal("new role did not appear in existing session")
	}
	// Optimistic version prevents silently overwriting a newer role assignment.
	if _, err = accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: "stale", Active: true, Roles: []string{"admin"}, Version: teacher.Version}); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("stale form overwrote access")
	}
	updated, err = accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: teacher.DisplayName, Active: true, Roles: []string{"guru_mapel"}, Version: updated.Version})
	if err != nil {
		t.Fatal(err)
	}
	get(teacherClient, "/akademik", 403)
	if _, err = accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: teacher.DisplayName, Active: true, Roles: []string{"pengganti"}, Version: updated.Version}); err == nil {
		t.Fatal("unknown role accepted")
	}
	// CSRF and Origin checks still reject mutation by a logged-in Admin.
	post(adminClient, "/admin/users/new", url.Values{"username": {"bad"}}, 403)
	req, _ := http.NewRequest("POST", server.URL+"/admin/users/new", strings.NewReader(url.Values{"gorilla.csrf.Token": {tokenFrom(get(adminClient, "/admin/users/new", 200))}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := adminClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	bodyOf(resp)
	if resp.StatusCode != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
	// Status change revokes sessions; reactivation does not revive old tokens.
	updated, err = accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: teacher.DisplayName, Active: false, Roles: []string{"guru_mapel"}, Version: updated.Version})
	if err != nil {
		t.Fatal(err)
	}
	get(teacherClient, "/", 303)
	if _, err = accounts.Login(ctx, username, teacherPassword, ""); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal("inactive user logged in")
	}
	updated, err = accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: teacher.DisplayName, Active: true, Roles: []string{"guru_mapel"}, Version: updated.Version})
	if err != nil {
		t.Fatal(err)
	}
	get(teacherClient, "/", 303)
	login(teacherClient, username, teacherPassword)
	// Session rotation invalidates the old token.
	old := sessionToken(teacherClient)
	rotated, err := accounts.Login(ctx, username, teacherPassword, old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = accounts.Session(ctx, old); !errors.Is(err, auth.ErrSession) {
		t.Fatal("session fixation possible")
	}
	if _, err = accounts.Session(ctx, rotated); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE app.sessions SET last_seen_at=now()-interval '31 minutes' WHERE token_hash=$1`, auth.TokenHash(rotated)); err != nil {
		t.Fatal(err)
	}
	if _, err = accounts.Session(ctx, rotated); !errors.Is(err, auth.ErrSession) {
		t.Fatal("idle session did not expire")
	}
	login(teacherClient, username, teacherPassword)
	expiry := sessionToken(teacherClient)
	if _, err = db.ExecContext(ctx, `UPDATE app.sessions SET expires_at=now()-interval '1 minute' WHERE token_hash=$1`, auth.TokenHash(expiry)); err != nil {
		t.Fatal(err)
	}
	get(teacherClient, "/", 303)
	login(teacherClient, username, teacherPassword)
	nextPassword := random()
	changeToken := tokenFrom(get(teacherClient, "/account/password", 200))
	post(teacherClient, "/account/password", url.Values{"gorilla.csrf.Token": {changeToken}, "current_password": {teacherPassword}, "password": {nextPassword}, "confirm_password": {nextPassword}}, 303)
	get(teacherClient, "/", 303)
	if _, err = accounts.Login(ctx, username, teacherPassword, ""); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal("old password still accepted")
	}
	login(teacherClient, username, nextPassword)
	logoutToken := tokenFrom(get(teacherClient, "/", 200))
	loggedOut := sessionToken(teacherClient)
	post(teacherClient, "/logout", url.Values{"gorilla.csrf.Token": {logoutToken}}, 303)
	if _, err = accounts.Session(ctx, loggedOut); !errors.Is(err, auth.ErrSession) {
		t.Fatal("logout did not revoke server session")
	}
	// Audit contains access changes but no plaintext/hash credentials or raw session tokens.
	audit := get(adminClient, "/admin/audit", 200)
	if !strings.Contains(audit, "user.update") {
		t.Fatal("access audit not visible")
	}
	for _, secret := range []string{password, teacherPassword, nextPassword, rawToken, loggedOut} {
		if strings.Contains(audit, secret) {
			t.Fatal("audit leaked credentials")
		}
	}
	filtered := get(adminClient, "/admin/users?q="+username+"&role=guru_mapel&status=active", 200)
	if !strings.Contains(filtered, username) {
		t.Fatal("search/filter omitted matching account")
	}
	// Concurrent edits of the same version must have exactly one winner.
	teacher, err = accounts.Get(ctx, teacherID)
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan error, 2)
	for _, label := range []string{"Concurrent A", "Concurrent B"} {
		go func(name string) {
			_, err := accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: name, Active: true, Roles: []string{"guru_mapel"}, Version: teacher.Version})
			outcomes <- err
		}(label)
	}
	winners, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-outcomes
		if err == nil {
			winners++
		} else if errors.Is(err, auth.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatal("concurrent edits lost version protection")
	}
	teacher, err = accounts.Get(ctx, teacherID)
	if err != nil {
		t.Fatal(err)
	}
	// Administrative password reset revokes all sessions and invalidates the old password.
	resetSession, err := accounts.Login(ctx, username, nextPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	resetPassword := random()
	updated, err = accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: teacher.DisplayName, Password: resetPassword, Active: true, Roles: []string{"guru_mapel"}, Version: teacher.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = accounts.Session(ctx, resetSession); !errors.Is(err, auth.ErrSession) {
		t.Fatal("admin reset did not revoke session")
	}
	if _, err = accounts.Login(ctx, username, nextPassword, ""); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal("old password accepted after admin reset")
	}
	if _, err = accounts.Login(ctx, username, resetPassword, ""); err != nil {
		t.Fatal(err)
	}
	// Leave only this run's fixtures inactive, with sessions revoked; retain audit history.
	_, err = accounts.Update(ctx, admin.ID, teacherID, auth.Input{Username: username, DisplayName: teacher.DisplayName, Active: false, Roles: []string{"guru_mapel"}, Version: updated.Version})
	if err != nil {
		t.Fatal(err)
	}
}
