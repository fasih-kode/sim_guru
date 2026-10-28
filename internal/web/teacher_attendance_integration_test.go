//go:build integration

package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherAttendanceHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	base := t.TempDir()
	repo, public, private := filepath.Join(base, "repo"), filepath.Join(base, "public"), filepath.Join(base, "private")
	for _, p := range []string{repo, public, private} {
		if err := os.Mkdir(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	photos, err := teacherphoto.NewStore(teacherphoto.Config{Directory: filepath.Join(private, "photos"), RepositoryRoot: repo, PublicRoots: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	defer photos.Close()
	service := &academic.Store{DB: db, Photos: photos}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: service, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var actors, teachers []int64
	defer func() {
		tx, err := db.Begin()
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback()
		for _, teacher := range teachers {
			for _, q := range []string{`DELETE FROM app.teacher_attendance_history WHERE event_id IN(SELECT id FROM app.teacher_attendance_events WHERE teacher_id=$1)`, `DELETE FROM app.teacher_attendance_photos WHERE event_id IN(SELECT id FROM app.teacher_attendance_events WHERE teacher_id=$1)`, `DELETE FROM app.teacher_attendance_events WHERE teacher_id=$1`, `DELETE FROM app.teachers WHERE id=$1`} {
				if _, err = tx.Exec(q, teacher); err != nil {
					t.Error(err)
					return
				}
			}
		}
		for _, actor := range actors {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, err = tx.Exec(q, actor); err != nil {
					t.Error(err)
					return
				}
			}
		}
		if err = tx.Commit(); err != nil {
			t.Error(err)
		}
	}()
	csrfRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	read := func(resp *http.Response, want int) string {
		t.Helper()
		defer resp.Body.Close()
		body, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		if resp.StatusCode != want {
			t.Fatalf("HTTP status=%d want %d", resp.StatusCode, want)
		}
		return string(body)
	}
	get := func(client *http.Client, path string, want int) string {
		t.Helper()
		resp, e := client.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		return read(resp, want)
	}
	type fixture struct {
		user    auth.User
		teacher int64
		client  *http.Client
		csrf    string
	}
	makeUser := func(link bool, roles ...string) fixture {
		t.Helper()
		u, password := testutil.User(t, db, roles...)
		actors = append(actors, u.ID)
		f := fixture{user: u}
		if link {
			if e := db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru sintetis F1',$1) RETURNING id`, u.ID).Scan(&f.teacher); e != nil {
				t.Fatal(e)
			}
			teachers = append(teachers, f.teacher)
		}
		jar, _ := cookiejar.New(nil)
		f.client = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		token := csrfRE.FindStringSubmatch(get(f.client, "/login", 200))
		if len(token) != 2 {
			t.Fatal("login csrf missing")
		}
		resp, e := f.client.PostForm(server.URL+"/login", url.Values{"username": {u.Username}, "password": {password}, "gorilla.csrf.Token": {token[1]}})
		if e != nil {
			t.Fatal(e)
		}
		read(resp, 303)
		return f
	}
	admin := makeUser(false, "admin")
	location, _, err := service.TeacherAttendanceLocation(ctx, admin.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !location.Configured {
		t.Skip("test memerlukan konfigurasi sekolah yang sudah ada; tidak mengubah konfigurasi bersama")
	}
	var imageBytes bytes.Buffer
	if err = jpeg.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 32, 32)), nil); err != nil {
		t.Fatal(err)
	}
	jpegBytes := imageBytes.Bytes()
	fields := func(action, token string) map[string]string {
		return map[string]string{"action": action, "idempotency_key": token, "latitude": strconv.FormatFloat(location.Latitude, 'f', -1, 64), "longitude": strconv.FormatFloat(location.Longitude, 'f', -1, 64), "accuracy_m": "5"}
	}
	upload := func(f fixture, values map[string]string, photo []byte, csrfToken string, want int, chunked bool) string {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for k, v := range values {
			if e := writer.WriteField(k, v); e != nil {
				t.Fatal(e)
			}
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="selfie"; filename="selfie.jpg"`)
		header.Set("Content-Type", "image/jpeg")
		part, e := writer.CreatePart(header)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = part.Write(photo); e != nil {
			t.Fatal(e)
		}
		writer.Close()
		var source io.Reader = bytes.NewReader(body.Bytes())
		if chunked {
			source = struct{ io.Reader }{source}
		}
		req, e := http.NewRequest("POST", server.URL+"/kehadiran-saya", source)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if csrfToken != "" {
			req.Header.Set("X-CSRF-Token", csrfToken)
		}
		resp, e := f.client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		return read(resp, want)
	}
	counts := func(teacher int64) (int, int, int) {
		t.Helper()
		var e, p, h int
		err := db.QueryRow(`SELECT (SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1),(SELECT count(*) FROM app.teacher_attendance_photos p JOIN app.teacher_attendance_events e ON e.id=p.event_id WHERE teacher_id=$1),(SELECT count(*) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE teacher_id=$1)`, teacher).Scan(&e, &p, &h)
		if err != nil {
			t.Fatal(err)
		}
		return e, p, h
	}
	assertCount := func(teacher int64, n int) {
		t.Helper()
		e, p, h := counts(teacher)
		if e != n || p != n || h != n {
			t.Fatalf("event/photo/history=%d/%d/%d want %d", e, p, h, n)
		}
	}
	var dailyBefore, lessonBefore int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.student_daily_attendance),(SELECT count(*) FROM app.student_lesson_attendance)`).Scan(&dailyBefore, &lessonBefore); err != nil {
		t.Fatal(err)
	}
	teacher := makeUser(true, "guru_mapel")
	body := get(teacher.client, "/kehadiran-saya?teacher_id="+fmt.Sprint(admin.user.ID), 200)
	token := csrfRE.FindStringSubmatch(body)
	if len(token) != 2 {
		t.Fatal("attendance csrf missing")
	}
	teacher.csrf = token[1]
	if !strings.Contains(body, "Belum Dicatat") || !strings.Contains(body, "Guru sintetis F1") {
		t.Fatal("missing initial attendance state")
	}
	key1, _ := academic.NewTeacherAttendanceIdempotencyKey()
	upload(teacher, fields("check_in", key1), jpegBytes, "", 403, false)
	upload(teacher, fields("check_in", key1), []byte("invalid-selfie"), teacher.csrf, 422, false)
	inaccurate := fields("check_in", key1)
	inaccurate["accuracy_m"] = "51"
	upload(teacher, inaccurate, jpegBytes, teacher.csrf, 422, false)
	outside := fields("check_in", key1)
	outside["latitude"] = "80"
	if location.Latitude >= 0 {
		outside["latitude"] = "-80"
	}
	upload(teacher, outside, jpegBytes, teacher.csrf, 422, false)
	upload(teacher, fields("check_in", key1), bytes.Repeat([]byte("x"), int(teacherSelfieRequestLimit)+1), teacher.csrf, 413, false)
	upload(teacher, fields("check_in", key1), bytes.Repeat([]byte("x"), int(teacherSelfieRequestLimit)+1), teacher.csrf, 413, true)
	assertCount(teacher.teacher, 0)
	upload(teacher, fields("check_in", key1), jpegBytes, teacher.csrf, 303, false)
	upload(teacher, fields("check_in", key1), jpegBytes, teacher.csrf, 303, false)
	assertCount(teacher.teacher, 1)
	key2, _ := academic.NewTeacherAttendanceIdempotencyKey()
	conflict := upload(teacher, fields("check_in", key2), jpegBytes, teacher.csrf, 409, false)
	if !strings.Contains(conflict, "Sudah Dicatat") || !strings.Contains(conflict, "Periksa status terbaru") {
		t.Fatal("conflict did not refresh own status")
	}
	assertCount(teacher.teacher, 1)
	for _, roles := range [][]string{{"wali_kelas"}, {"guru_mapel", "wali_kelas", "admin", "kurikulum"}} {
		f := makeUser(true, roles...)
		page := get(f.client, "/kehadiran-saya", 200)
		f.csrf = csrfRE.FindStringSubmatch(page)[1]
		key, _ := academic.NewTeacherAttendanceIdempotencyKey()
		upload(f, fields("check_out", key), jpegBytes, f.csrf, 303, false)
		assertCount(f.teacher, 1)
		day, e := service.TeacherAttendanceToday(ctx, f.user.ID)
		if e != nil || len(day.Events) != 1 || day.Events[0].Action != academic.TeacherCheckOut {
			t.Fatal("checkout without checkin failed")
		}
	}
	for _, roles := range [][]string{{"admin"}, {"petugas_absensi"}, {"kurikulum", "kesiswaan"}} {
		f := makeUser(true, roles...)
		get(f.client, "/kehadiran-saya", 403)
	}
	unlinked := makeUser(false, "guru_mapel")
	get(unlinked.client, "/kehadiran-saya", 403)
	inactiveProfile := makeUser(true, "guru_mapel")
	if _, e := db.Exec(`UPDATE app.teachers SET active=false WHERE id=$1`, inactiveProfile.teacher); e != nil {
		t.Fatal(e)
	}
	get(inactiveProfile.client, "/kehadiran-saya", 403)
	inactiveAccount := makeUser(true, "guru_mapel")
	if _, e := db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, inactiveAccount.user.ID); e != nil {
		t.Fatal(e)
	}
	get(inactiveAccount.client, "/kehadiran-saya", 303)
	if _, e := service.TeacherAttendanceToday(ctx, inactiveAccount.user.ID); e != academic.ErrForbidden {
		t.Fatal("reader accepted inactive account")
	}
	concurrent := makeUser(true, "guru_mapel")
	concurrent.csrf = csrfRE.FindStringSubmatch(get(concurrent.client, "/kehadiran-saya", 200))[1]
	concurrentKey, _ := academic.NewTeacherAttendanceIdempotencyKey()
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			upload(concurrent, fields("check_in", concurrentKey), jpegBytes, concurrent.csrf, 303, false)
		}()
	}
	wait.Wait()
	assertCount(concurrent.teacher, 1)
	day, e := service.TeacherAttendanceToday(ctx, teacher.user.ID)
	if e != nil || len(day.Events) != 1 {
		t.Fatal("reader failed")
	}
	var date string
	if e = db.QueryRow(`SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&date); e != nil || day.Date != date {
		t.Fatal("reader date is not server WIB")
	}
	isolated := makeUser(true, "guru_mapel")
	if d, e := service.TeacherAttendanceToday(ctx, isolated.user.ID); e != nil || len(d.Events) != 0 {
		t.Fatal("reader leaked another teacher")
	}
	officer := makeUser(false, "petugas_absensi")
	manualKey, _ := academic.NewTeacherAttendanceIdempotencyKey()
	_, err = service.RecordTeacherAttendanceManual(ctx, officer.user.ID, academic.TeacherAttendanceManualInput{TeacherID: isolated.teacher, Action: academic.TeacherCheckIn, OccurredAt: date + "T00:00", Reason: "Fixture bantuan petugas F1", IdempotencyKey: manualKey})
	if err != nil {
		t.Fatal("manual fixture:", err)
	}
	if !strings.Contains(get(isolated.client, "/kehadiran-saya", 200), "Dicatat Petugas") {
		t.Fatal("manual source not shown")
	}
	noStorage := &academic.Store{DB: db}
	handler2, e := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: noStorage, CSRFKey: key})
	if e != nil {
		t.Fatal(e)
	}
	disabled := httptest.NewServer(handler2)
	defer disabled.Close()
	req, _ := http.NewRequest("GET", disabled.URL+"/kehadiran-saya", nil)
	for _, cookie := range teacher.client.Jar.Cookies(mustTeacherURL(t, server.URL)) {
		req.AddCookie(cookie)
	}
	resp, e := teacher.client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(read(resp, 200), "Kehadiran mandiri belum tersedia") {
		t.Fatal("missing safe unavailable message")
	}
	var dailyAfter, lessonAfter int
	if e = db.QueryRow(`SELECT (SELECT count(*) FROM app.student_daily_attendance),(SELECT count(*) FROM app.student_lesson_attendance)`).Scan(&dailyAfter, &lessonAfter); e != nil || dailyAfter != dailyBefore || lessonAfter != lessonBefore {
		t.Fatal("student attendance changed")
	}
	// Opt-in lokal menyediakan fixture browser sementara tanpa memakai database development.
	if directory := os.Getenv("SIM_GURU_F1_UAT_DIR"); directory != "" {
		visual := makeUser(true, "guru_mapel")
		cookies := visual.client.Jar.Cookies(mustTeacherURL(t, server.URL))
		data, _ := json.Marshal(map[string]any{"url": server.URL, "cookies": cookies, "latitude": location.Latitude, "longitude": location.Longitude})
		if e = os.WriteFile(filepath.Join(directory, "fixture.json"), data, 0o600); e != nil {
			t.Fatal(e)
		}
		defer os.Remove(filepath.Join(directory, "fixture.json"))
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.NewTimer(10 * time.Minute)
		defer timeout.Stop()
		for {
			select {
			case <-ticker.C:
				if _, e := os.Stat(filepath.Join(directory, "done")); e == nil {
					return
				}
			case <-timeout.C:
				t.Fatal("UAT fixture timed out")
			}
		}
	}
}

func mustTeacherURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
