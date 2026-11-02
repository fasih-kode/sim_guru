//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAttendanceFoundationHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &academic.Store{DB: db}
	ctx := context.Background()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	admin, apw := testutil.User(t, db, "admin")
	cur, cpw := testutil.User(t, db, "kurikulum")
	teacher, tpw := testutil.User(t, db, "guru_mapel")
	officer, opw := testutil.User(t, db, "petugas_absensi")
	y, e := s.CreateYear(ctx, cur.ID, "Foundation HTTP "+testutil.Random(t)[:8])
	must(e)
	c, e := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	must(e)
	var sem, student int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, y.ID).Scan(&sem))
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Contoh','2013-01-01','L') RETURNING id`).Scan(&student))
	accounts, e := auth.New(db)
	must(e)
	key := make([]byte, 32)
	_, e = rand.Read(key)
	must(e)
	h, e := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	must(e)
	server := httptest.NewServer(h)
	defer server.Close()
	tokenRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	read := func(resp *http.Response, code int) string {
		t.Helper()
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		must(e)
		if resp.StatusCode != code {
			t.Fatalf("HTTP %d want %d: %s", resp.StatusCode, code, string(b))
		}
		return string(b)
	}
	get := func(client *http.Client, path string, code int) string {
		t.Helper()
		r, e := client.Get(server.URL + path)
		must(e)
		return read(r, code)
	}
	post := func(client *http.Client, path string, v url.Values, code int) string {
		t.Helper()
		r, e := client.PostForm(server.URL+path, v)
		must(e)
		return read(r, code)
	}
	token := func(body string) string {
		t.Helper()
		m := tokenRE.FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatal("missing csrf")
		}
		return m[1]
	}
	login := func(u auth.User, pw string) *http.Client {
		jar, e := cookiejar.New(nil)
		must(e)
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		post(client, "/login", url.Values{"username": {u.Username}, "password": {pw}, "gorilla.csrf.Token": {token(get(client, "/login", 200))}}, 303)
		return client
	}
	ac, cc, tc, oc := login(admin, apw), login(cur, cpw), login(teacher, tpw), login(officer, opw)
	calendar := fmt.Sprintf("/akademik/kalender-sekolah?semester_id=%d", sem)
	dates := fmt.Sprintf("/admin/placements/dates?year_id=%d", y.ID)
	get(ac, calendar, 403)
	get(tc, calendar, 403)
	get(oc, calendar, 403)
	get(cc, dates, 403)
	get(tc, dates, 403)
	get(oc, dates, 403)
	body := get(cc, calendar, 200)
	if !strings.Contains(body, "Belum diatur") {
		t.Fatal("unconfigured calendar hidden")
	}
	ct := token(body)
	values := url.Values{"action": {"calendar"}, "starts_on": {"2020-07-01"}, "ends_on": {"2020-12-31"}, "school_days": {"1", "2", "3", "4", "5"}, "reason": {"Kalender sekolah"}, "version": {"0"}, "gorilla.csrf.Token": {ct}}
	post(ac, calendar, values, 403)
	post(cc, calendar, url.Values{"action": {"calendar"}}, 403)
	bad := url.Values{"action": {"calendar"}, "starts_on": {"2020-12-31"}, "ends_on": {"2020-07-01"}, "school_days": {"1"}, "reason": {"invalid"}, "version": {"0"}, "gorilla.csrf.Token": {ct}}
	post(cc, calendar, bad, 422)
	post(cc, calendar, values, 303)
	post(cc, calendar, values, 409)
	post(cc, calendar, url.Values{"action": {"exception"}, "date": {"2020-08-17"}, "label": {"Libur sekolah"}, "mode": {"holiday"}, "version": {"1"}, "reason": {"Hari libur"}, "gorilla.csrf.Token": {ct}}, 303)
	list := get(cc, calendar, 200)
	modal := get(cc, calendar+"&edit_date=2020-08-17", 200)
	if strings.Contains(modal, `app-notification-message"></p>`) {
		t.Fatal("empty error notification rendered")
	}
	if !strings.Contains(list, "Libur sekolah") || !strings.Contains(modal, "data-modal-open-on-load") {
		t.Fatal("exception/modal missing")
	}
	// New HTTP placement requires an explicit date. Old service API remains for legacy fixtures/import callers.
	initial := fmt.Sprintf("/admin/placements?year_id=%d&class_id=%d", y.ID, c.ID)
	initialHTML := get(ac, initial, 200)
	at := token(initialHTML)
	pvals := url.Values{"student_id": {fmt.Sprint(student)}, "class_id": {fmt.Sprint(c.ID)}, "year_id": {fmt.Sprint(y.ID)}, "gorilla.csrf.Token": {at}}
	post(ac, "/admin/placements", pvals, 422)
	pvals.Set("starts_on", "2020-07-01")
	post(ac, "/admin/placements", pvals, 303)
	var p int64
	must(db.QueryRow(`SELECT id FROM app.student_placements WHERE student_id=$1 AND school_year_id=$2`, student, y.ID).Scan(&p))
	dbefore := get(ac, dates, 200)
	dt := token(dbefore)
	vals := url.Values{"id": {fmt.Sprint(p)}, "version": {"1"}, "starts_on": {"2020-07-02"}, "reason": {"Koreksi tanggal sesuai berkas"}, "gorilla.csrf.Token": {dt}}
	post(cc, dates, vals, 403)
	post(ac, dates, vals, 303)
	post(ac, dates, vals, 409)
	dafter := get(ac, dates, 200)
	dmodal := get(ac, dates+fmt.Sprintf("&edit=%d", p), 200)
	if !strings.Contains(dafter, "2020-07-02") || !strings.Contains(dmodal, "Koreksi tanggal sesuai berkas") {
		t.Fatal("date/history missing")
	}

	if !strings.Contains(initialHTML, `name="starts_on" type="date" value="2020-07-01"`) {
		t.Fatal("semester date not suggested")
	}
	bulk := fmt.Sprintf("/admin/placements/dates/bulk?year_id=%d", y.ID)
	get(cc, bulk, 403)
	var bulkIDs []string
	for i := 0; i < 12; i++ {
		var sid int64
		must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, fmt.Sprintf("Siswa Massal %02d", i)).Scan(&sid))
		id, e := s.CreatePlacement(ctx, admin.ID, sid, c.ID)
		must(e)
		bulkIDs = append(bulkIDs, fmt.Sprint(id))
	}
	bulkHTML := get(ac, bulk, 200)
	bt := token(bulkHTML)
	post(ac, bulk, url.Values{"action": {"preview"}}, 403)
	bulkValues := url.Values{"action": {"preview"}, "starts_on": {"2020-07-01"}, "reason": {"Tanggal awal bersama"}, "placement_ids": bulkIDs[:11], "gorilla.csrf.Token": {bt}}
	previewHTML := post(ac, bulk, bulkValues, 200)
	previewRE := regexp.MustCompile(`name="preview_token" value="([^"]+)"`)
	matched := previewRE.FindStringSubmatch(previewHTML)
	if len(matched) != 2 {
		t.Fatal("missing signed preview")
	}
	confirm := url.Values{"action": {"confirm"}, "preview_token": {matched[1]}, "gorilla.csrf.Token": {bt}}
	post(ac, bulk, confirm, 422)
	confirm.Set("confirmed", "yes")
	confirm.Set("preview_token", "x"+matched[1])
	post(ac, bulk, confirm, 409)
	confirm.Set("preview_token", matched[1])
	post(ac, bulk, confirm, 303)
	post(ac, bulk, confirm, 409)
	var batchCount int
	must(db.QueryRow(`SELECT count(*) FROM app.student_placements WHERE id=ANY($1::bigint[]) AND starts_on='2020-07-01'`, bulkIDs).Scan(&batchCount))
	if batchCount != 11 {
		t.Fatal("bulk/exclusion", batchCount)
	}
	if dir := os.Getenv("SIM_GURU_VISUAL_DIR"); dir != "" {
		must(os.MkdirAll(dir, 0700))
		for name, html := range map[string]string{"calendar.html": list, "calendar-modal.html": modal, "dates.html": dafter, "date-modal.html": dmodal, "new-placement.html": initialHTML, "bulk.html": bulkHTML, "bulk-preview.html": previewRE.ReplaceAllString(previewHTML, `name="preview_token" value="visual-fixture"`)} {
			html = tokenRE.ReplaceAllString(html, `name="gorilla.csrf.Token" value="visual-fixture"`)
			must(os.WriteFile(filepath.Join(dir, name), []byte(html), 0600))
		}
	}
}
