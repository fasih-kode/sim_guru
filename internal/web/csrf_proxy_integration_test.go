//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestHTTPSReverseProxyLoginCSRFIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	user, password := testutil.User(t, db, "guru_mapel")
	var teacherID int64
	defer func() {
		tx, cleanupErr := db.BeginTx(context.Background(), nil)
		if cleanupErr != nil {
			t.Error("transaksi pembersihan fixture proxy gagal dimulai")
			return
		}
		defer tx.Rollback()
		cleanup := []struct {
			query string
			id    int64
		}{
			{`DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, user.ID},
			{`DELETE FROM app.sessions WHERE user_id=$1`, user.ID},
			{`DELETE FROM app.teachers WHERE user_id=$1`, user.ID},
			{`DELETE FROM app.user_roles WHERE user_id=$1`, user.ID},
			{`DELETE FROM app.users WHERE id=$1`, user.ID},
		}
		for _, item := range cleanup {
			if _, cleanupErr = tx.ExecContext(context.Background(), item.query, item.id); cleanupErr != nil {
				t.Error("pembersihan fixture sintetis gagal")
				return
			}
		}
		if cleanupErr = tx.Commit(); cleanupErr != nil {
			t.Error("pembersihan fixture sintetis gagal dikonfirmasi")
		}
	}()
	if err = db.QueryRowContext(ctx, `INSERT INTO app.teachers(name,user_id) VALUES('Guru proxy sintetis',$1) RETURNING id`, user.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}

	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	trustedProxy := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	handler, err := New(func(context.Context) error { return nil }, Options{
		Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db},
		CSRFKey: key, TrustedProxyCIDRs: trustedProxy,
	})
	if err != nil {
		t.Fatal(err)
	}

	csrfPattern := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	read := func(response *http.Response, status int) string {
		t.Helper()
		defer response.Body.Close()
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != status {
			t.Fatalf("HTTP status=%d want=%d", response.StatusCode, status)
		}
		return string(body)
	}
	token := func(body string) string {
		t.Helper()
		match := csrfPattern.FindStringSubmatch(body)
		if len(match) != 2 {
			t.Fatal("token CSRF tidak ditemukan")
		}
		return match[1]
	}
	client := func(transport *http.Transport) *http.Client {
		jar, jarErr := cookiejar.New(nil)
		if jarErr != nil {
			t.Fatal(jarErr)
		}
		return &http.Client{Jar: jar, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	postLogin := func(c *http.Client, endpoint, host, origin, csrfToken string) *http.Response {
		t.Helper()
		form := url.Values{"username": {user.Username}, "password": {password}}
		if csrfToken != "" {
			form.Set("gorilla.csrf.Token", csrfToken)
		}
		req, requestErr := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		req.Host = host
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin)
		fetchSite := "same-origin"
		if origin != "http://"+host && origin != "https://"+host {
			fetchSite = "cross-site"
		}
		req.Header.Set("Sec-Fetch-Site", fetchSite)
		response, doErr := c.Do(req)
		if doErr != nil {
			t.Fatal(doErr)
		}
		return response
	}
	assertCookieSecure := func(t *testing.T, cookies []*http.Cookie, name string, want bool) {
		t.Helper()
		for _, cookie := range cookies {
			if cookie.Name == name {
				if cookie.Secure != want || !cookie.HttpOnly && name == sessionCookie {
					t.Fatalf("cookie %q secure=%v httpOnly=%v", name, cookie.Secure, cookie.HttpOnly)
				}
				return
			}
		}
		t.Fatalf("cookie %q tidak ada", name)
	}

	t.Run("http localhost tetap dapat login dan memakai sesi", func(t *testing.T) {
		server := httptest.NewServer(handler)
		defer server.Close()
		c := client(server.Client().Transport.(*http.Transport))
		page, err := c.Get(server.URL + "/login")
		if err != nil {
			t.Fatal(err)
		}
		loginPage := read(page, http.StatusOK)
		assertCookieSecure(t, page.Cookies(), "_gorilla_csrf", false)
		host := strings.TrimPrefix(server.URL, "http://")
		response := postLogin(c, server.URL+"/login", host, "http://"+host, token(loginPage))
		read(response, http.StatusSeeOther)
		assertCookieSecure(t, response.Cookies(), sessionCookie, false)
		attendance, err := c.Get(server.URL + "/kehadiran-saya")
		if err != nil {
			t.Fatal(err)
		}
		if body := read(attendance, http.StatusOK); !strings.Contains(body, "Kehadiran Saya") {
			t.Fatal("sesi HTTP tidak dapat membuka halaman kehadiran")
		}
	})

	t.Run("https reverse proxy menerima login dan sesi aman", func(t *testing.T) {
		backend := httptest.NewServer(handler)
		defer backend.Close()
		target, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatal(err)
		}
		reverse := httputil.NewSingleHostReverseProxy(target)
		const externalHost = "simguru-proxy-test.ngrok-free.dev"
		proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("X-Forwarded-Host", r.Host)
			r.Host = target.Host
			reverse.ServeHTTP(w, r)
		}))
		defer proxy.Close()
		c := proxy.Client()
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		c.Jar = jar
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		get := func(path string) (*http.Response, string) {
			t.Helper()
			req, requestErr := http.NewRequest(http.MethodGet, proxy.URL+path, nil)
			if requestErr != nil {
				t.Fatal(requestErr)
			}
			req.Host = externalHost
			resp, doErr := c.Do(req)
			if doErr != nil {
				t.Fatal(doErr)
			}
			body := read(resp, http.StatusOK)
			return resp, body
		}
		page, loginPage := get("/login")
		assertCookieSecure(t, page.Cookies(), "_gorilla_csrf", true)
		csrfToken := token(loginPage)
		blocked := postLogin(c, proxy.URL+"/login", externalHost, "https://evil.example", csrfToken)
		read(blocked, http.StatusForbidden)
		withoutCookie := &http.Client{
			Transport: proxy.Client().Transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		missingCookie := postLogin(withoutCookie, proxy.URL+"/login", externalHost, "https://"+externalHost, csrfToken)
		read(missingCookie, http.StatusForbidden)
		badRefererRequest, err := http.NewRequest(http.MethodPost, proxy.URL+"/login", strings.NewReader(url.Values{
			"username": {user.Username}, "password": {password}, "gorilla.csrf.Token": {csrfToken},
		}.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		badRefererRequest.Host = externalHost
		badRefererRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		badRefererRequest.Header.Set("Referer", "https://evil.example/login")
		badRefererRequest.Header.Set("Sec-Fetch-Site", "same-origin")
		badReferer, err := c.Do(badRefererRequest)
		if err != nil {
			t.Fatal(err)
		}
		read(badReferer, http.StatusForbidden)
		missing := postLogin(c, proxy.URL+"/login", externalHost, "https://"+externalHost, "")
		read(missing, http.StatusForbidden)
		wrong := postLogin(c, proxy.URL+"/login", externalHost, "https://"+externalHost, "wrong-token")
		read(wrong, http.StatusForbidden)
		response := postLogin(c, proxy.URL+"/login", externalHost, "https://"+externalHost, csrfToken)
		read(response, http.StatusSeeOther)
		assertCookieSecure(t, response.Cookies(), sessionCookie, true)
		attendance, body := get("/kehadiran-saya")
		if attendance.StatusCode != http.StatusOK || !strings.Contains(body, "Kehadiran Saya") || !strings.Contains(body, "Guru proxy sintetis") {
			t.Fatal("cookie sesi HTTPS tidak dapat membuka halaman guru")
		}
		for _, path := range []string{"/logout", "/account/password", "/kehadiran-saya"} {
			req, requestErr := http.NewRequest(http.MethodPost, proxy.URL+path, strings.NewReader(""))
			if requestErr != nil {
				t.Fatal(requestErr)
			}
			req.Host = externalHost
			req.Header.Set("Origin", "https://"+externalHost)
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			resp, doErr := c.Do(req)
			if doErr != nil {
				t.Fatal(doErr)
			}
			read(resp, http.StatusForbidden)
		}
	})

	t.Run("header proxy palsu dari luar CIDR tetap ditolak", func(t *testing.T) {
		get := httptest.NewRequest(http.MethodGet, "http://internal.local/login", nil)
		get.RemoteAddr = "203.0.113.8:4567"
		get.Header.Set("X-Forwarded-Proto", "https")
		get.Header.Set("X-Forwarded-Host", "evil.example")
		getWriter := httptest.NewRecorder()
		handler.ServeHTTP(getWriter, get)
		csrfToken := token(getWriter.Body.String())
		csrfCookie := getWriter.Result().Cookies()[0]
		form := url.Values{"gorilla.csrf.Token": {csrfToken}, "username": {user.Username}, "password": {password}}
		post, err := http.NewRequest(http.MethodPost, "http://internal.local/login", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		post.Host = "internal.local"
		post.RemoteAddr = "203.0.113.8:4567"
		post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		post.Header.Set("Origin", "https://evil.example")
		post.Header.Set("Sec-Fetch-Site", "cross-site")
		post.Header.Set("X-Forwarded-Proto", "https")
		post.Header.Set("X-Forwarded-Host", "evil.example")
		post.AddCookie(csrfCookie)
		postWriter := httptest.NewRecorder()
		handler.ServeHTTP(postWriter, post)
		if postWriter.Code != http.StatusForbidden {
			t.Fatalf("header proxy palsu memperoleh status %d", postWriter.Code)
		}
	})
}
