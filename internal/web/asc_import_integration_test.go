//go:build integration

package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestASCImportHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &academic.Store{DB: db}
	ctx := context.Background()
	cur, pw := testutil.User(t, db, "kurikulum")
	admin, apw := testutil.User(t, db, "admin")
	ta, _ := testutil.User(t, db, "guru_mapel")
	tb, _ := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	y, err := s.CreateYear(ctx, cur.ID, "XML HTTP "+testutil.Random(t)[:10])
	must(err)
	var sem, t1, t2, sub1, sub2 int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, y.ID).Scan(&sem))
	cl, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	must(err)
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Teacher A',$1) RETURNING id`, ta.ID).Scan(&t1))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Teacher B',$1) RETURNING id`, tb.ID).Scan(&t2))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Matematika') RETURNING id`, "HX"+testutil.Random(t)[:8]).Scan(&sub1))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Bahasa Inggris') RETURNING id`, "HY"+testutil.Random(t)[:8]).Scan(&sub2))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	must(s.AssignTeacher(ctx, cur.ID, sem, cl.ID, sub1, t1, date))
	must(s.AssignTeacher(ctx, cur.ID, sem, cl.ID, sub2, t2, date))
	raw, err := os.ReadFile("../academic/testdata/asc-basic.xml")
	must(err)
	doc, err := academic.ParseASCXML(bytes.NewReader(raw))
	must(err)
	for day := 1; day <= 2; day++ {
		var periods []academic.LessonPeriod
		for _, p := range doc.Periods {
			periods = append(periods, academic.LessonPeriod{Number: p.Number, StartsAt: p.StartsAt, EndsAt: p.EndsAt})
		}
		must(s.SaveLessonDay(ctx, cur.ID, sem, day, periods))
	}
	for _, r := range []academic.CurriculumRule{{SemesterID: sem, Grade: 7, SubjectID: sub1, WeeklyJP: 2, Meetings: []int{2}}, {SemesterID: sem, Grade: 7, SubjectID: sub2, WeeklyJP: 3, Meetings: []int{3}}} {
		must(s.SaveCurriculumRule(ctx, cur.ID, r))
	}
	rev, err := s.CreateTimetableDraft(ctx, cur.ID, sem, date)
	must(err)
	must(s.AddTimetableEntry(ctx, cur.ID, rev.ID, 1, cl.ID, sub1, t1, 1, 2))
	accounts, err := auth.New(db)
	must(err)
	key := make([]byte, 32)
	_, err = rand.Read(key)
	must(err)
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	must(err)
	server := httptest.NewServer(handler)
	defer server.Close()
	read := func(resp *http.Response, code int) string {
		t.Helper()
		defer resp.Body.Close()
		data, e := io.ReadAll(resp.Body)
		must(e)
		if resp.StatusCode != code {
			t.Fatalf("HTTP %d want %d: %.400s", resp.StatusCode, code, data)
		}
		return string(data)
	}
	get := func(c *http.Client, path string, code int) string {
		t.Helper()
		resp, e := c.Get(server.URL + path)
		must(e)
		return read(resp, code)
	}
	post := func(c *http.Client, path string, values url.Values, code int) *http.Response {
		t.Helper()
		resp, e := c.PostForm(server.URL+path, values)
		must(e)
		if resp.StatusCode != code {
			read(resp, code)
		}
		return resp
	}
	tokenRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	csrfToken := func(body string) string {
		t.Helper()
		m := tokenRE.FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatal("CSRF missing")
		}
		return m[1]
	}
	login := func(u auth.User, pw string) *http.Client {
		jar, e := cookiejar.New(nil)
		must(e)
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		read(post(client, "/login", url.Values{"username": {u.Username}, "password": {pw}, "gorilla.csrf.Token": {csrfToken(get(client, "/login", 200))}}, 303), 303)
		return client
	}
	client, adminClient := login(cur, pw), login(admin, apw)
	path := fmt.Sprintf("/akademik/jadwal/impor?semester_id=%d", sem)
	uploadBody := get(client, path, 200)
	get(adminClient, path, 403)
	if !strings.Contains(uploadBody, "asc-upload-modal") || !strings.Contains(uploadBody, "readonly") {
		t.Fatal("upload modal/date lock missing")
	}
	upload := func(c *http.Client, name string, content []byte, token string, code int) *http.Response {
		t.Helper()
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		must(writer.WriteField("effective_from", "2026-01-01"))
		f, e := writer.CreateFormFile("file", name)
		must(e)
		_, e = f.Write(content)
		must(e)
		must(writer.Close())
		req, e := http.NewRequest("POST", server.URL+path, &b)
		must(e)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		resp, e := c.Do(req)
		must(e)
		if resp.StatusCode != code {
			read(resp, code)
		}
		return resp
	}
	read(upload(client, "sample.xml", raw, "", 403), 403)
	read(upload(client, "sample.txt", raw, csrfToken(uploadBody), 422), 422)
	response := upload(client, "sample.xml", raw, csrfToken(uploadBody), 303)
	location := response.Header.Get("Location")
	read(response, 303)
	parsedURL, err := url.Parse(location)
	must(err)
	importToken := parsedURL.Query().Get("token")
	if len(importToken) != 64 {
		t.Fatal("staging token missing")
	}
	mappingBody := get(client, location, 200)
	stage, _, err := s.ASCImportPreview(ctx, cur.ID, importToken)
	must(err)
	values := url.Values{"action": {"preview"}, "version": {fmt.Sprint(stage.Version)}, "gorilla.csrf.Token": {csrfToken(mappingBody)}}
	for _, g := range []struct {
		prefix  string
		items   []academic.ASCItem
		mapping map[string]int64
	}{{"class", stage.Payload.Document.Classes, stage.Payload.Mapping.Classes}, {"teacher", stage.Payload.Document.Teachers, stage.Payload.Mapping.Teachers}, {"subject", stage.Payload.Document.Subjects, stage.Payload.Mapping.Subjects}} {
		for i, item := range g.items {
			values.Set(fmt.Sprintf("map_%s_%d", g.prefix, i), fmt.Sprint(g.mapping[item.ID]))
		}
	}
	for i, d := range stage.Payload.Document.Days {
		values.Set(fmt.Sprintf("map_day_%d", i), fmt.Sprint(stage.Payload.Mapping.Days[d.ID]))
	}
	for i, k := range stage.Payload.Document.PeriodKeys() {
		values.Set(fmt.Sprintf("map_period_%d", i), fmt.Sprint(stage.Payload.Mapping.Periods[k]))
	}
	previewBody := read(post(client, location, values, 200), 200)
	if !strings.Contains(previewBody, "Pratinjau telah diperiksa") || !strings.Contains(previewBody, `name="replace_draft"`) {
		t.Fatal("valid preview/replace guard missing")
	}
	stage, _, err = s.ASCImportPreview(ctx, cur.ID, importToken)
	must(err)
	confirm := url.Values{"action": {"confirm"}, "version": {fmt.Sprint(stage.Version)}, "gorilla.csrf.Token": {csrfToken(previewBody)}}
	read(post(client, location, confirm, 422), 422)
	confirm.Set("replace_draft", "yes")
	read(post(client, location, confirm, 303), 303)
	read(post(client, location, confirm, 303), 303)
	_, _, entries, err := s.Timetable(ctx, cur.ID, sem)
	must(err)
	if len(entries) != 2 {
		t.Fatal("duplicate/partial HTTP import", len(entries))
	}
	// A genuine truncated upload can be previewed but never confirmed.
	cut := []byte(strings.Replace(string(raw), `<card lessonid="L2" period="3" days="01" weeks="1" terms="1"/>`, "", 1))
	response = upload(client, "truncated.xml", cut, csrfToken(get(client, path, 200)), 303)
	cutLocation := response.Header.Get("Location")
	read(response, 303)
	cutBody := get(client, cutLocation, 200)
	if !strings.Contains(cutBody, "ekspor terpotong/tidak lengkap") {
		t.Fatal("truncation diagnostic missing")
	}
	if dir := os.Getenv("SIM_GURU_VISUAL_DIR"); dir != "" {
		must(os.MkdirAll(dir, 0700))
		for name, body := range map[string]string{"asc-upload.html": uploadBody, "asc-preview.html": previewBody, "asc-truncated.html": cutBody, "asc-weekly.html": get(client, fmt.Sprintf("/akademik/jadwal?semester_id=%d", sem), 200)} {
			body = tokenRE.ReplaceAllString(body, `name="gorilla.csrf.Token" value="visual-fixture"`)
			body = strings.ReplaceAll(body, importToken, "visual-import-token")
			must(os.WriteFile(filepath.Join(dir, name), []byte(body), 0600))
		}
	}
}
