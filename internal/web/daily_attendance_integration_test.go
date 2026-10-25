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
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentDailyHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &academic.Store{DB: db}
	ctx := context.Background()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	officer, opw := testutil.User(t, db, "petugas_absensi")
	admin, apw := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	kes, kpw := testutil.User(t, db, "kesiswaan")
	wali, wpw := testutil.User(t, db, "wali_kelas")
	y, e := s.CreateYear(ctx, cur.ID, "HTTP Daily "+testutil.Random(t)[:8])
	must(e)
	must(s.ActivateYear(ctx, cur.ID, y.ID))
	c, e := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	must(e)
	now := time.Now().In(time.FixedZone("WIB", 7*3600))
	today := now.Format("2006-01-02")
	start := now.AddDate(0, 0, -10).Format("2006-01-02")
	var sem, student int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, y.ID).Scan(&sem))
	must(s.SaveSemesterCalendar(ctx, cur.ID, academic.SemesterCalendar{SemesterID: sem, StartsOn: start, EndsOn: now.AddDate(0, 0, 10).Format("2006-01-02"), SchoolDays: []int{1, 2, 3, 4, 5, 6, 7}}, "Kalender"))
	for i := 0; i < 12; i++ {
		var sid int64
		must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, fmt.Sprintf("Siswa Uji %02d", i)).Scan(&sid))
		_, e = s.CreatePlacement(ctx, admin.ID, sid, c.ID, start)
		must(e)
		if i == 0 {
			student = sid
		}
	}
	accounts, e := auth.New(db)
	must(e)
	key := make([]byte, 32)
	_, e = rand.Read(key)
	must(e)
	h, e := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	must(e)
	server := httptest.NewServer(h)
	defer server.Close()
	csrfRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	read := func(resp *http.Response, code int) string {
		t.Helper()
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		must(e)
		if resp.StatusCode != code {
			t.Fatalf("HTTP %d want %d: %.500s", resp.StatusCode, code, b)
		}
		return string(b)
	}
	get := func(client *http.Client, path string, code int) string {
		t.Helper()
		r, e := client.Get(server.URL + path)
		must(e)
		return read(r, code)
	}
	post := func(client *http.Client, path string, values url.Values, code int) string {
		t.Helper()
		r, e := client.PostForm(server.URL+path, values)
		must(e)
		return read(r, code)
	}
	token := func(body string) string {
		t.Helper()
		m := csrfRE.FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatal("missing csrf")
		}
		return m[1]
	}
	login := func(user auth.User, pw string) *http.Client {
		jar, e := cookiejar.New(nil)
		must(e)
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		post(client, "/login", url.Values{"username": {user.Username}, "password": {pw}, "gorilla.csrf.Token": {token(get(client, "/login", 200))}}, 303)
		return client
	}
	oc, ac, kc, wc := login(officer, opw), login(admin, apw), login(kes, kpw), login(wali, wpw)
	daily := fmt.Sprintf("/absensi/harian?class_id=%d&date=%s", c.ID, today)
	get(ac, daily, 403)
	get(wc, daily, 403)
	get(kc, daily, 200)
	get(kc, "/absensi/kartu", 403)
	get(ac, "/absensi/pemindai", 403)
	rulesURL := fmt.Sprintf("/kesiswaan/batas-kedatangan?semester_id=%d", sem)
	get(ac, rulesURL, 403)
	get(oc, rulesURL, 403)
	rulesHTML := get(kc, rulesURL, 200)
	day := int(now.Weekday())
	if day == 0 {
		day = 7
	}
	ruleValues := url.Values{"mode": {"weekly"}, "class_id": {"0"}, "weekday": {fmt.Sprint(day)}, "effective_from": {start}, "deadline": {"07:00"}, "tolerance": {"0"}, "reason": {"Batas kedatangan uji"}, "expected": {"0"}, "gorilla.csrf.Token": {token(rulesHTML)}}
	post(kc, rulesURL, ruleValues, 303)
	post(kc, rulesURL, ruleValues, 409)
	rulesHTML = get(kc, rulesURL, 200)
	initial := get(oc, daily, 200)
	if !strings.Contains(initial, "Belum Dicatat") || !strings.Contains(initial, "data-table-paginate") {
		t.Fatal("roster markup")
	}
	scanner := get(oc, "/absensi/pemindai", 200)
	cardModal := get(oc, fmt.Sprintf("/absensi/kartu?edit=%d", student), 200)
	post(oc, "/absensi/scan", url.Values{"token": {"invalid"}}, 403)
	ct := token(cardModal)
	cardValues := url.Values{"student_id": {fmt.Sprint(student)}, "card_id": {"0"}, "action": {"issue"}, "reason": {"Kartu pertama"}, "confirmed": {"yes"}, "gorilla.csrf.Token": {ct}}
	post(ac, "/absensi/kartu", cardValues, 403)
	post(oc, "/absensi/kartu", cardValues, 303)
	post(oc, "/absensi/kartu", cardValues, 409)
	card, e := s.StudentCard(ctx, officer.ID, student)
	must(e)
	printed := get(oc, fmt.Sprintf("/absensi/kartu/%d/cetak", student), 200)
	if !strings.Contains(printed, "data:image/png;base64,") || strings.Contains(printed, card.Token) {
		t.Fatal("QR print/payload exposure")
	}
	get(ac, fmt.Sprintf("/absensi/kartu/%d/cetak", student), 403)
	if !strings.Contains(scanner, "student-qr.js") {
		t.Fatal("scanner module missing")
	}
	st := token(scanner)
	scanValues := url.Values{"token": {card.Token}, "date": {"2099-01-01"}, "arrival": {"01:23"}, "gorilla.csrf.Token": {st}}
	body := post(oc, "/absensi/scan", scanValues, 200)
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, today) {
		t.Fatal("scan failed")
	}
	body = post(oc, "/absensi/scan", scanValues, 200)
	if !strings.Contains(body, `"duplicate":true`) {
		t.Fatal("duplicate not detected")
	}
	list := get(oc, daily, 200)
	modal := get(oc, daily+fmt.Sprintf("&edit=%d", student), 200)
	if !strings.Contains(modal, `type="text" id="daily-arrival"`) || !strings.Contains(modal, `data-time-24`) || strings.Contains(modal, `type="time" id="daily-arrival"`) {
		t.Fatal("24-hour arrival field missing")
	}
	if !strings.Contains(modal, "Sebelum:") || !strings.Contains(modal, "Sesudah:") {
		t.Fatal("readable correction history missing")
	}
	sheet, e := s.DailyAttendance(ctx, officer.ID, c.ID, today)
	must(e)
	var row academic.DailyStudent
	for _, r := range sheet.Students {
		if r.StudentID == student {
			row = r
		}
	}
	if row.Policy == nil || !row.LateMinutes.Valid {
		t.Fatal("QR rule snapshot missing")
	}
	vals := url.Values{"student_id": {fmt.Sprint(student)}, "version": {fmt.Sprint(row.Version)}, "status": {"izin"}, "reason": {"Koreksi sesuai bukti"}, "gorilla.csrf.Token": {token(modal)}}
	post(kc, daily, vals, 403)
	post(oc, daily, vals, 303)
	post(oc, daily, vals, 409)
	post(oc, "/absensi/scan", scanValues, 422)
	cv := url.Values{"student_id": {fmt.Sprint(student)}, "card_id": {fmt.Sprint(card.CardID)}, "action": {"replace"}, "reason": {"Kartu hilang"}, "confirmed": {"yes"}, "gorilla.csrf.Token": {ct}}
	post(oc, "/absensi/kartu", cv, 303)
	post(oc, "/absensi/scan", scanValues, 422)
	cardList := get(oc, "/absensi/kartu", 200)
	cardReplace := get(oc, fmt.Sprintf("/absensi/kartu?edit=%d", student), 200)

	quickURL := fmt.Sprintf("/absensi/harian/cepat?class_id=%d&date=%s", c.ID, today)
	get(ac, quickURL, 403)
	get(kc, quickURL, 403)
	quickHTML := get(oc, quickURL, 200)
	idsRE := regexp.MustCompile(`name="student_ids" value="([^"]+)"`)
	ids := idsRE.FindAllStringSubmatch(quickHTML, -1)
	if len(ids) != 11 {
		t.Fatal("existing record offered for overwrite", len(ids))
	}
	fpRE := regexp.MustCompile(`name="fingerprint" value="([^"]+)"`)
	fp := fpRE.FindStringSubmatch(quickHTML)
	if len(fp) != 2 {
		t.Fatal("missing quick fingerprint")
	}
	qvalues := url.Values{"student_ids": {ids[0][1], ids[1][1]}, "status_" + ids[0][1]: {"hadir"}, "status_" + ids[1][1]: {"sakit"}, "reason_kind": {"manual"}, "fingerprint": {fp[1]}, "gorilla.csrf.Token": {token(quickHTML)}}
	post(oc, quickURL, qvalues, 422)
	qvalues.Set("arrival_"+ids[0][1], "07:08")
	var expected int64
	must(db.QueryRow(`SELECT max(id) FROM app.student_arrival_rules WHERE semester_id=$1`, sem).Scan(&expected))
	_, e = s.SaveArrivalRule(ctx, kes.ID, expected, academic.ArrivalRule{SemesterID: sem, Weekday: day, EffectiveFrom: start, Deadline: "07:10", Reason: "Perubahan ketika form terbuka"})
	must(e)
	conflictHTML := post(oc, quickURL, qvalues, 409)
	if !strings.Contains(conflictHTML, `value="07:08"`) || !strings.Contains(conflictHTML, "Isian Anda tetap ditampilkan") {
		t.Fatal("draft lost on conflict")
	}
	newFP := fpRE.FindStringSubmatch(conflictHTML)
	qvalues.Set("fingerprint", newFP[1])
	post(oc, quickURL, qvalues, 303)
	post(oc, quickURL, qvalues, 409)
	// Fast direct arrival is an explicit action with server time; no manual clock.
	nextStudent := ids[2][1]
	latestSheet, e := s.DailyAttendance(ctx, officer.ID, c.ID, today)
	must(e)
	post(oc, daily, url.Values{"action": {"now"}, "student_id": {nextStudent}, "fingerprint": {latestSheet.Fingerprint}, "gorilla.csrf.Token": {token(initial)}}, 303)
	var clocked bool
	must(db.QueryRow(`SELECT arrived_at IS NOT NULL FROM app.student_daily_attendance WHERE student_id=$1 AND on_date=$2`, nextStudent, today).Scan(&clocked))
	if !clocked {
		t.Fatal("direct arrival missing time")
	}
	must(db.QueryRow(`SELECT max(id) FROM app.student_arrival_rules WHERE semester_id=$1`, sem).Scan(&expected))
	rulesEdit := get(kc, rulesURL+fmt.Sprintf("&edit=%d", expected), 200)
	if dir := os.Getenv("SIM_GURU_VISUAL_DIR"); dir != "" {
		must(os.MkdirAll(dir, 0700))
		for name, html := range map[string]string{"daily.html": list, "daily-modal.html": modal, "scanner.html": scanner, "cards.html": cardList, "card-modal.html": cardReplace, "card-print.html": printed, "arrival-rules.html": rulesHTML, "arrival-rule-modal.html": rulesEdit, "quick.html": quickHTML, "quick-conflict.html": conflictHTML} {
			html = csrfRE.ReplaceAllString(html, `name="gorilla.csrf.Token" value="visual-fixture"`)
			must(os.WriteFile(filepath.Join(dir, name), []byte(html), 0600))
		}
	}
}
