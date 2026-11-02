//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherAttendanceReportsHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	service := &academic.Store{DB: db}
	accounts, e := auth.New(db)
	if e != nil {
		t.Fatal(e)
	}
	key := make([]byte, 32)
	if _, e = rand.Read(key); e != nil {
		t.Fatal(e)
	}
	handler, e := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: service, CSRFKey: key})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var users, teachers []int64
	t.Cleanup(func() {
		tx, e := db.Begin()
		if e != nil {
			t.Error(e)
			return
		}
		defer tx.Rollback()
		for _, id := range teachers {
			for _, q := range []string{`DELETE FROM app.teacher_attendance_correction_requests WHERE teacher_id=$1`, `DELETE FROM app.teacher_attendance_report_history WHERE report_id IN(SELECT id FROM app.teacher_attendance_reports WHERE teacher_id=$1)`, `DELETE FROM app.teacher_attendance_reports WHERE teacher_id=$1`, `DELETE FROM app.teachers WHERE id=$1`} {
				if _, e = tx.Exec(q, id); e != nil {
					t.Error(e)
					return
				}
			}
		}
		for _, id := range users {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, e = tx.Exec(q, id); e != nil {
					t.Error(e)
					return
				}
			}
		}
		if e = tx.Commit(); e != nil {
			t.Error(e)
		}
	})
	type actor struct {
		user    auth.User
		teacher int64
		client  *http.Client
	}
	makeActor := func(roles ...string) actor {
		u, pw := testutil.User(t, db, roles...)
		users = append(users, u.ID)
		var teacher int64
		if e := db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES($1,$2) RETURNING id`, "HTTP G2 "+testutil.Random(t)[:8], u.ID).Scan(&teacher); e != nil {
			t.Fatal(e)
		}
		teachers = append(teachers, teacher)
		token, e := accounts.Login(ctx, u.Username, pw, "")
		if e != nil {
			t.Fatal(e)
		}
		jar, _ := cookiejar.New(nil)
		base, _ := url.Parse(server.URL)
		jar.SetCookies(base, []*http.Cookie{{Name: sessionCookie, Value: token, Path: "/"}})
		return actor{u, teacher, &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	}
	teacher := makeActor("guru_mapel")
	other := makeActor("guru_mapel")
	manager := makeActor("pegawai_tu", "pengelola_kehadiran_guru")
	officer := makeActor("petugas_absensi")
	plainTU := makeActor("pegawai_tu")
	read := func(resp *http.Response, want int) string {
		t.Helper()
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		if resp.StatusCode != want {
			t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, want, string(b)[:min(len(b), 300)])
		}
		return string(b)
	}
	get := func(a actor, path string, want int) string {
		t.Helper()
		resp, e := a.client.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		return read(resp, want)
	}
	re := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	token := func(a actor, path string) string {
		t.Helper()
		m := re.FindStringSubmatch(get(a, path, 200))
		if len(m) != 2 {
			t.Fatal("csrf field missing")
		}
		return m[1]
	}
	post := func(a actor, path string, values url.Values, want int) (string, string) {
		t.Helper()
		resp, e := a.client.PostForm(server.URL+path, values)
		if e != nil {
			t.Fatal(e)
		}
		location := resp.Header.Get("Location")
		return read(resp, want), location
	}
	formPage := get(teacher, "/kehadiran-saya/laporan?new=1", 200)
	createToken := re.FindStringSubmatch(formPage)[1]
	keyRE := regexp.MustCompile(`name="idempotency_key" value="([^"]+)"`)
	requestKey := keyRE.FindStringSubmatch(formPage)[1]
	values := url.Values{"kind": {"izin"}, "starts_on": {"2026-10-28"}, "ends_on": {"2026-10-30"}, "description": {"<script>uji aman</script>"}, "idempotency_key": {requestKey}, "gorilla.csrf.Token": {createToken}}
	noCSRF := cloneReportValues(values)
	noCSRF.Del("gorilla.csrf.Token")
	post(teacher, "/kehadiran-saya/laporan", noCSRF, 403)
	invalid := cloneReportValues(values)
	invalid.Set("description", "   ")
	body, _ := post(teacher, "/kehadiran-saya/laporan", invalid, 422)
	if !strings.Contains(body, `id="teacher-report-create"`) || !strings.Contains(body, "Keterangan wajib diisi") || strings.Count(body, "Keterangan wajib diisi") != 1 || !strings.Contains(body, `value="izin" selected`) || !strings.Contains(body, `value="2026-10-30"`) {
		t.Fatal("validation modal/fields missing")
	}
	_, location := post(teacher, "/kehadiran-saya/laporan", values, 303)
	if !strings.HasPrefix(location, "/kehadiran-saya/laporan/") {
		t.Fatal("create redirect", location)
	}
	ownPath := strings.Split(location, "?")[0]
	rawID := strings.TrimPrefix(ownPath, "/kehadiran-saya/laporan/")
	id, e := strconv.ParseInt(rawID, 10, 64)
	if e != nil {
		t.Fatal(e)
	}
	_, retryLocation := post(teacher, "/kehadiran-saya/laporan", values, 303)
	if retryLocation != location {
		t.Fatal("retry created another report")
	}
	body = get(teacher, ownPath, 200)
	if strings.Contains(body, "<script>uji aman</script>") || !strings.Contains(body, "&lt;script&gt;uji aman&lt;/script&gt;") || strings.Contains(body, ">Koreksi Laporan</a>") || strings.Contains(body, ">Batalkan Laporan</a>") {
		t.Fatal("unsafe rendering or owner edit controls")
	}
	get(other, ownPath, 404)
	post(teacher, ownPath+"/koreksi", values, 404)
	post(teacher, ownPath+"/batalkan", values, 404)
	for _, a := range []actor{officer, plainTU} {
		get(a, "/tu/kehadiran-guru/laporan", 403)
	}
	tuPath := "/tu/kehadiran-guru/laporan/" + rawID
	editToken := token(manager, tuPath+"?modal=correct")
	edit := url.Values{"kind": {"sakit"}, "starts_on": {"2026-10-28"}, "ends_on": {"2026-10-30"}, "description": {"Keterangan baru"}, "reason": {"   "}, "version": {"1"}, "gorilla.csrf.Token": {editToken}}
	body, _ = post(manager, tuPath+"/koreksi", edit, 422)
	if strings.Count(body, "Alasan koreksi atau pembatalan wajib diisi") != 1 || !strings.Contains(body, `id="teacher-report-change"`) || !strings.Contains(body, "Keterangan baru") || !strings.Contains(body, `value="sakit" selected`) {
		t.Fatal("correction validation not visible/preserved")
	}
	var version, history int
	check := func(wantV, wantH int) {
		t.Helper()
		if e := db.QueryRow(`SELECT version,(SELECT count(*) FROM app.teacher_attendance_report_history WHERE report_id=r.id) FROM app.teacher_attendance_reports r WHERE id=$1`, id).Scan(&version, &history); e != nil {
			t.Fatal(e)
		}
		if version != wantV || history != wantH {
			t.Fatalf("version/history=%d/%d want=%d/%d", version, history, wantV, wantH)
		}
	}
	check(1, 1)
	edit.Set("reason", "Jenis yang benar adalah Sakit")
	post(manager, tuPath+"/koreksi", edit, 303)
	check(2, 2)
	body, _ = post(manager, tuPath+"/koreksi", edit, 409)
	if !strings.Contains(body, "Catatan berubah") || !strings.Contains(body, `name="version" value="1"`) {
		t.Fatal("stale conflict missing or version silently replaced")
	}
	check(2, 2)
	requestToken := token(teacher, ownPath+"?modal=request")
	request := url.Values{"gorilla.csrf.Token": {requestToken}, "request_text": {"Mohon periksa laporan ini"}}
	post(teacher, ownPath+"/permintaan", request, 303)
	check(2, 2)
	body = get(manager, "/tu/kehadiran-guru", 200)
	if !strings.Contains(body, "Laporan #"+rawID) || !strings.Contains(body, "Mohon periksa laporan ini") {
		t.Fatal("TU report request not linked")
	}
	post(other, ownPath+"/permintaan", url.Values{"gorilla.csrf.Token": {token(other, "/kehadiran-saya")}, "request_text": {"Tidak berwenang"}}, 404)
	cancelToken := token(manager, tuPath+"?modal=cancel")
	post(manager, tuPath+"/batalkan", url.Values{"gorilla.csrf.Token": {cancelToken}, "version": {"2"}, "reason": {"Laporan dibatalkan berdasarkan koreksi guru"}}, 303)
	check(3, 3)
	body = get(teacher, ownPath, 200)
	if !strings.Contains(body, "Dibatalkan") || !strings.Contains(body, "Jenis yang benar adalah Sakit") {
		t.Fatal("cancelled report/history lost")
	}
	// Daftar lebih dari satu halaman dan histori panjang tetap dibatasi per halaman.
	for day := 1; day <= 11; day++ {
		k, _ := academic.NewTeacherAttendanceIdempotencyKey()
		date := "2040-02-" + strconv.Itoa(100 + day)[1:]
		if _, e = service.CreateTeacherAttendanceReport(ctx, teacher.user.ID, academic.TeacherAttendanceReportInput{Kind: "izin", StartsOn: date, EndsOn: date, Description: "Pagination", IdempotencyKey: k}); e != nil {
			t.Fatal(e)
		}
	}
	body = get(teacher, "/kehadiran-saya/laporan", 200)
	if strings.Count(body, ">Detail / histori</a>") != 10 || !strings.Contains(body, "page=2") {
		t.Fatal("list pagination not applied")
	}
	get(teacher, "/kehadiran-saya/laporan?page=2", 200)
	var historyReportID int64
	if e = db.QueryRow(`SELECT id FROM app.teacher_attendance_reports WHERE teacher_id=$1 AND active ORDER BY id DESC LIMIT 1`, teacher.teacher).Scan(&historyReportID); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 11; i++ {
		_, e = service.CorrectTeacherAttendanceReport(ctx, manager.user.ID, academic.TeacherAttendanceReportCorrection{ReportID: historyReportID, ExpectedVersion: int64(i + 1), Kind: "izin", StartsOn: "2040-02-11", EndsOn: "2040-02-11", Description: "Histori " + strconv.Itoa(i), Reason: "Pengujian pagination histori"})
		if e != nil {
			t.Fatal(e)
		}
	}
	historyPath := "/kehadiran-saya/laporan/" + strconv.FormatInt(historyReportID, 10)
	body = get(teacher, historyPath, 200)
	if strings.Count(body, "<article ") != 10 || !strings.Contains(body, "history_page=2") {
		t.Fatal("history pagination missing")
	}
	body = get(teacher, historyPath+"?history_page=2", 200)
	if strings.Count(body, "<article ") != 2 || !strings.Contains(body, "Pencatatan awal") {
		t.Fatal("older history inaccessible")
	}
	for i := 0; i < 11; i++ {
		if e = service.RequestTeacherAttendanceReportCorrection(ctx, teacher.user.ID, historyReportID, "Permintaan halaman "+strconv.Itoa(i)); e != nil {
			t.Fatal(e)
		}
	}
	body = get(manager, "/tu/kehadiran-guru", 200)
	if !strings.Contains(body, "request_page=2") {
		t.Fatal("request inbox pagination missing")
	}
	get(manager, "/tu/kehadiran-guru?request_page=2", 200)
	// Filter rentang mencakup laporan yang menyentuh ujung tanggal.
	body = get(manager, "/tu/kehadiran-guru/laporan?from=2026-10-30&to=2026-10-30&status=cancelled&kind=sakit", 200)
	if !strings.Contains(body, tuPath) {
		t.Fatal("range/status/type filters failed")
	}
	get(manager, "/tu/kehadiran-guru/laporan?from=2026-10-30&to=2026-10-01", 422)
	get(teacher, "/kehadiran-saya", 200)
}

func cloneReportValues(in url.Values) url.Values {
	out := url.Values{}
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}
