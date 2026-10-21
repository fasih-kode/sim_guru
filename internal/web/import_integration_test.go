//go:build integration

package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
	"github.com/xuri/excelize/v2"
)

func TestImportHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	admin, password := testutil.User(t, db, "admin")
	curriculum, kPassword := testutil.User(t, db, "kurikulum")
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	rand.Read(key)
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db}, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	bodyOf := func(resp *http.Response, status int) string {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status {
			t.Fatalf("HTTP %d expected %d", resp.StatusCode, status)
		}
		return string(body)
	}
	get := func(c *http.Client, path string, status int) string {
		resp, err := c.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return bodyOf(resp, status)
	}
	field := func(body, name string) string {
		pattern := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`)
		matches := pattern.FindStringSubmatch(body)
		if len(matches) != 2 {
			t.Fatalf("missing form field %s", name)
		}
		return matches[1]
	}
	login := func(user auth.User, pass string) *http.Client {
		c := client()
		token := field(get(c, "/login", 200), "gorilla.csrf.Token")
		resp, err := c.PostForm(server.URL+"/login", url.Values{"gorilla.csrf.Token": {token}, "username": {user.Username}, "password": {pass}})
		if err != nil {
			t.Fatal(err)
		}
		bodyOf(resp, 303)
		return c
	}
	ac := login(admin, password)
	kc := login(curriculum, kPassword)
	makeFile := func(kind master.Kind, values [][]any) []byte {
		data, err := content.ReadFile("import_templates/" + string(kind) + ".xlsx")
		if err != nil {
			t.Fatal(err)
		}
		book, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		defer book.Close()
		for r, cols := range values {
			for c, value := range cols {
				cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
				if err = book.SetCellValue("Data", cell, value); err != nil {
					t.Fatal(err)
				}
			}
		}
		buffer, err := book.WriteToBuffer()
		if err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	upload := func(c *http.Client, kind, filename, token string, data []byte, status int) string {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		if token != "" {
			form.WriteField("gorilla.csrf.Token", token)
		}
		file, err := form.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		file.Write(data)
		form.Close()
		resp, err := c.Post(server.URL+"/master/"+kind+"/import", form.FormDataContentType(), &body)
		if err != nil {
			t.Fatal(err)
		}
		return bodyOf(resp, status)
	}
	confirm := func(kind, token, csrf string, status int) string {
		resp, err := ac.PostForm(server.URL+"/master/"+kind+"/import/confirm", url.Values{"gorilla.csrf.Token": {csrf}, "import_token": {token}})
		if err != nil {
			t.Fatal(err)
		}
		return bodyOf(resp, status)
	}
	get(client(), "/master/students/import", 303)
	get(kc, "/master/students/import", 403)
	get(kc, "/master/teachers/import/template", 403)
	get(ac, "/master/subjects/import", 404)
	templateResponse, err := ac.Get(server.URL + "/master/students/import/template")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(templateResponse.Header.Get("Content-Type"), "spreadsheetml.sheet") || !strings.Contains(templateResponse.Header.Get("Content-Disposition"), "attachment") {
		t.Fatal("template download headers incorrect")
	}
	bodyOf(templateResponse, 200)
	page := get(ac, "/master/students/import", 200)
	csrf := field(page, "gorilla.csrf.Token")
	suffix := testutil.Random(t)[:12]
	values := [][]any{{"Upload A " + suffix, "2013-01-02", "L", "000" + suffix, "", "0812", "YA"}, {"Upload B " + suffix, "2013-02-03", "P", "001" + suffix, "", "", "TIDAK"}}
	data := makeFile(master.Students, values)
	upload(ac, "students", "data.xlsx", "", data, 403)
	upload(ac, "students", "data.xls", csrf, data, 422)
	upload(ac, "students", "data.xlsx", csrf, []byte("not Excel"), 422)
	kCSRF := field(get(kc, "/account/password", 200), "gorilla.csrf.Token")
	upload(kc, "students", "data.xlsx", kCSRF, data, 403)
	preview := upload(ac, "students", "data.xlsx", csrf, data, 200)
	if !strings.Contains(preview, "000"+suffix) || !strings.Contains(preview, "2013-01-02") {
		t.Fatal("preview changed identifier/date")
	}
	token := field(preview, "import_token")
	csrf = field(preview, "gorilla.csrf.Token")
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.students WHERE name LIKE $1`, "Upload %"+suffix).Scan(&count); err != nil || count != 0 {
		t.Fatal("upload persisted before confirmation")
	}
	complete := confirm("students", token, csrf, 200)
	if !strings.Contains(complete, "2 data") {
		t.Fatal("import result count missing")
	}
	repeated := confirm("students", token, csrf, 200)
	if !strings.Contains(repeated, "sudah diproses") {
		t.Fatal("repeated submission duplicated data")
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.students WHERE name LIKE $1`, "Upload %"+suffix).Scan(&count); err != nil || count != 2 {
		t.Fatal("wrong committed count")
	}
	duplicate := upload(ac, "students", "again.xlsx", csrf, data, 422)
	if strings.Contains(duplicate, `name="import_token"`) {
		t.Fatal("invalid preview allowed confirmation")
	}
	guru := makeFile(master.Teachers, [][]any{{"Upload Guru " + suffix, "000000000" + suffix, "", "guru@example.test", "0812", "YA"}})
	teacherPreview := upload(ac, "teachers", "guru.xlsx", csrf, guru, 200)
	confirm("teachers", field(teacherPreview, "import_token"), field(teacherPreview, "gorilla.csrf.Token"), 200)
	// A real workbook above 32 KiB must pass the upload-specific body allowance.
	largeValues := make([][]any, 500)
	for i := range largeValues {
		largeValues[i] = []any{testutil.Random(t) + testutil.Random(t) + testutil.Random(t), "2013-01-02", "L", "", "", "", "YA"}
	}
	large := makeFile(master.Students, largeValues)
	if len(large) <= 32<<10 {
		t.Fatal("large upload fixture too small to exercise multipart limit")
	}
	largePreview := upload(ac, "students", "large.xlsx", csrf, large, 200)
	largeToken := field(largePreview, "import_token")
	if _, err = db.Exec(`UPDATE app.master_imports SET expires_at=now()-interval '1 minute' WHERE token_hash=$1`, auth.TokenHash(largeToken)); err != nil {
		t.Fatal(err)
	}
	confirm("students", largeToken, field(largePreview, "gorilla.csrf.Token"), 422)
}
