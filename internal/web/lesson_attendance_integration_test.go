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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestLessonAttendanceContextHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := &academic.Store{DB: db}
	admin, adminPassword := testutil.User(t, db, "admin")
	curriculum, curriculumPassword := testutil.User(t, db, "kurikulum")
	teacher, teacherPassword := testutil.User(t, db, "guru_mapel")
	combined, combinedPassword := testutil.User(t, db, "admin", "guru_mapel")
	unassigned, unassignedPassword := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	year, err := s.CreateYear(ctx, curriculum.ID, "Lesson context "+testutil.Random(t)[:10])
	must(err)
	must(s.ActivateYear(ctx, curriculum.ID, year.ID))
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	var semester, teacherID, subjectID, studentID, secondStudentID int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Konteks Uji',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "LC"+testutil.Random(t)[:8], "Mapel Konteks Uji").Scan(&subjectID))
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Konteks Uji','2013-01-01','L') RETURNING id`).Scan(&studentID))
	studentPlacementID, err := s.CreatePlacement(ctx, admin.ID, studentID, class.ID, "2020-07-01")
	must(err)
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Kedua Konteks Uji','2013-02-01','P') RETURNING id`).Scan(&secondStudentID))
	_, err = s.CreatePlacement(ctx, admin.ID, secondStudentID, class.ID, "2020-07-01")
	must(err)
	calendar := academic.SemesterCalendar{SemesterID: semester, StartsOn: "2020-07-01", EndsOn: "2020-12-31", SchoolDays: []int{1, 2, 3, 4, 5}}
	must(s.SaveSemesterCalendar(ctx, curriculum.ID, calendar, "Kalender tes"))
	from, _ := time.Parse("2006-01-02", "2020-07-01")
	must(s.AssignTeacher(ctx, curriculum.ID, semester, class.ID, subjectID, teacherID, from))
	must(s.SaveCurriculumRule(ctx, curriculum.ID, academic.CurriculumRule{SemesterID: semester, Grade: 7, ClassID: class.ID, SubjectID: subjectID, WeeklyJP: 1, Meetings: []int{1}}))
	must(s.SaveLessonPeriod(ctx, curriculum.ID, semester, 2, 1, "07:00", "07:40"))
	revision, err := s.CreateTimetableDraft(ctx, curriculum.ID, semester, from)
	must(err)
	must(s.AddTimetableEntry(ctx, curriculum.ID, revision.ID, 2, class.ID, subjectID, teacherID, 1, 1))
	must(s.PublishTimetable(ctx, curriculum.ID, revision.ID))

	accounts, err := auth.New(db)
	must(err)
	key := make([]byte, 32)
	_, err = rand.Read(key)
	must(err)
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	must(err)
	server := httptest.NewServer(handler)
	defer server.Close()
	get := func(client *http.Client, path string, status int) string {
		t.Helper()
		response, getErr := client.Get(server.URL + path)
		must(getErr)
		defer response.Body.Close()
		body, readErr := io.ReadAll(response.Body)
		must(readErr)
		if response.StatusCode != status {
			t.Fatalf("GET %s: HTTP %d, want %d", path, response.StatusCode, status)
		}
		return string(body)
	}
	login := func(user auth.User, password string) *http.Client {
		t.Helper()
		jar, jarErr := cookiejar.New(nil)
		must(jarErr)
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		page := get(client, "/login", http.StatusOK)
		match := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(page)
		if len(match) != 2 {
			t.Fatal("login form CSRF token missing")
		}
		csrf := match[1]
		response, postErr := client.PostForm(server.URL+"/login", url.Values{"username": {user.Username}, "password": {password}, "gorilla.csrf.Token": {csrf}})
		must(postErr)
		response.Body.Close()
		if response.StatusCode != http.StatusSeeOther {
			t.Fatalf("login status %d", response.StatusCode)
		}
		return client
	}
	adminClient := login(admin, adminPassword)
	curriculumClient := login(curriculum, curriculumPassword)
	teacherClient := login(teacher, teacherPassword)
	combinedClient := login(combined, combinedPassword)
	unassignedClient := login(unassigned, unassignedPassword)
	dateURL := "/jadwal-saya/absensi?date=2020-07-07"
	if body := get(teacherClient, dateURL, http.StatusOK); !strings.Contains(body, "Mapel Konteks Uji") || !strings.Contains(body, "Buka roster") {
		t.Fatal("teacher's dated published meeting is missing")
	}
	var entryID int64
	must(db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1`, revision.ID).Scan(&entryID))
	detailURL := fmt.Sprintf("%s&entry_id=%d", dateURL, entryID)
	detail := get(teacherClient, detailURL, http.StatusOK)
	for _, expected := range []string{"Siswa Konteks Uji", "Belum Dicatat", "Guru Konteks Uji", "07:00", "Tandai semua Hadir", "lesson-attendance.js"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("lesson detail missing %q", expected)
		}
	}
	if !strings.Contains(detail, `data-attendance-save>`) {
		t.Fatal("manual submit button is unavailable when JavaScript is not running")
	}
	if strings.Contains(detail, `value="hadir" selected`) || strings.Contains(detail, `value="alpa" selected`) {
		t.Fatal("GET selected an attendance status without user action")
	}
	get(adminClient, dateURL, http.StatusForbidden)
	get(curriculumClient, dateURL, http.StatusForbidden)
	get(combinedClient, dateURL, http.StatusForbidden)
	get(unassignedClient, dateURL, http.StatusForbidden)
	get(teacherClient, dateURL+"&entry_id=999999999", http.StatusNotFound)
	get(teacherClient, "/jadwal-saya/absensi?date=not-a-date", http.StatusUnprocessableEntity)
	var before, after int
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE student_id=$1`, studentID).Scan(&before))
	response, postErr := teacherClient.PostForm(server.URL+"/jadwal-saya/absensi", url.Values{"date": {"2020-07-07"}})
	must(postErr)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without CSRF: HTTP %d, want 403", response.StatusCode)
	}
	csrfToken := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(detail)
	if len(csrfToken) != 2 {
		t.Fatal("detail form CSRF token missing")
	}
	baseForm := url.Values{"gorilla.csrf.Token": {csrfToken[1]}, "date": {"2020-07-07"}, "entry_id": {fmt.Sprint(entryID)}}
	post := func(values url.Values, status int) *http.Response {
		t.Helper()
		response, postErr := teacherClient.PostForm(server.URL+"/jadwal-saya/absensi", values)
		must(postErr)
		if response.StatusCode != status {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("POST lesson attendance: HTTP %d, want %d: %s", response.StatusCode, status, body)
		}
		return response
	}
	csrfRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	for _, client := range []*http.Client{adminClient, curriculumClient} {
		account := get(client, "/account/password", http.StatusOK)
		token := csrfRE.FindStringSubmatch(account)
		if len(token) != 2 {
			t.Fatal("role-negative CSRF token missing")
		}
		values := cloneValues(baseForm)
		values.Set("gorilla.csrf.Token", token[1])
		denied, postErr := client.PostForm(server.URL+"/jadwal-saya/absensi", values)
		must(postErr)
		denied.Body.Close()
		if denied.StatusCode != http.StatusForbidden {
			t.Fatalf("non-teacher POST status %d, want 403", denied.StatusCode)
		}
	}
	missingChoice := cloneValues(baseForm)
	missingChoice.Set("gorilla.csrf.Token", "invalid-token")
	response = post(missingChoice, http.StatusForbidden)
	response.Body.Close()
	noSelection := cloneValues(baseForm)
	noSelection.Set("fingerprint", regexp.MustCompile(`name="fingerprint" value="([^"]+)"`).FindStringSubmatch(detail)[1])
	noSelection.Add("student_ids", fmt.Sprint(studentID))
	noSelection.Add("student_ids", fmt.Sprint(secondStudentID))
	response = post(noSelection, http.StatusUnprocessableEntity)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), "Pilih status sedikitnya satu siswa") || !strings.Contains(string(body), "lesson-status-") {
		t.Fatal("empty selection did not preserve the form and explain validation")
	}
	invalidStatus := cloneValues(baseForm)
	invalidStatus.Set("fingerprint", noSelection.Get("fingerprint"))
	invalidStatus.Add("student_ids", fmt.Sprint(studentID))
	invalidStatus.Add("status_"+fmt.Sprint(studentID), "hadir-palsu")
	response = post(invalidStatus, http.StatusUnprocessableEntity)
	response.Body.Close()
	outsider := cloneValues(baseForm)
	outsider.Set("fingerprint", noSelection.Get("fingerprint"))
	outsider.Add("student_ids", "999999999")
	outsider.Add("status_999999999", "hadir")
	response = post(outsider, http.StatusUnprocessableEntity)
	response.Body.Close()
	partial := cloneValues(baseForm)
	partial.Set("fingerprint", noSelection.Get("fingerprint"))
	partial.Add("student_ids", fmt.Sprint(studentID))
	partial.Add("student_ids", fmt.Sprint(secondStudentID))
	partial.Add("status_"+fmt.Sprint(studentID), "hadir")
	response = post(partial, http.StatusSeeOther)
	savedLocation := response.Header.Get("Location")
	if !strings.Contains(savedLocation, "saved=1") {
		t.Fatal("successful save did not redirect to saved view")
	}
	response.Body.Close()
	saved := get(teacherClient, savedLocation, http.StatusOK)
	if !strings.Contains(saved, "Absensi per pelajaran berhasil disimpan") || !strings.Contains(saved, "Tersimpan · tidak dapat diubah") || !strings.Contains(saved, "Siswa Kedua Konteks Uji") || !strings.Contains(saved, "Hadir") {
		t.Fatal("saved status was not rendered read-only with remaining roster")
	}
	if strings.Contains(saved, `id="lesson-status-`+fmt.Sprint(studentID)+`"`) {
		t.Fatal("saved student unexpectedly has an editable status control")
	}
	response = post(partial, http.StatusSeeOther) // Retry identik: aman dan tidak membuat histori ganda.
	response.Body.Close()
	different := cloneValues(partial)
	different.Set("status_"+fmt.Sprint(studentID), "izin")
	response = post(different, http.StatusConflict)
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), "Konteks jadwal") || !strings.Contains(string(body), "Hadir") {
		t.Fatal("conflicting retry did not retain the old status and explain conflict")
	}
	classB, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "B", "", 0)
	must(err)
	must(s.MoveStudentClass(ctx, curriculum.ID, studentPlacementID, class.ID, classB.ID, "2020-07-07", "Perubahan roster untuk uji bukti lama"))
	var thirdStudentID int64
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Baru Setelah Form','2013-03-01','L') RETURNING id`).Scan(&thirdStudentID))
	_, err = s.CreatePlacement(ctx, admin.ID, thirdStudentID, class.ID, "2020-07-01")
	must(err)
	staleForm := cloneValues(partial)
	staleForm.Set("status_"+fmt.Sprint(secondStudentID), "sakit")
	response = post(staleForm, http.StatusConflict)
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), "Siswa Baru Setelah Form") || !strings.Contains(string(body), "Siswa Konteks Uji") || !strings.Contains(string(body), "Tersimpan · tidak dapat diubah") || !strings.Contains(string(body), "Pilihan dipertahankan: Sakit") || !strings.Contains(string(body), "Muat ulang konteks") || !strings.Contains(string(body), "value=\""+noSelection.Get("fingerprint")+"\"") {
		t.Fatal("stale roster was not explained while preserving the old fingerprint and choices")
	}
	var lessonRows, lessonHistoryRows, dailyRows int
	must(db.QueryRow(`SELECT count(*) FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date='2020-07-07'`, entryID).Scan(&lessonRows))
	must(db.QueryRow(`SELECT count(*) FROM app.student_lesson_attendance_history h JOIN app.student_lesson_attendance a ON a.id=h.attendance_id WHERE a.timetable_entry_id=$1 AND a.meeting_date='2020-07-07'`, entryID).Scan(&lessonHistoryRows))
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE student_id IN ($1,$2)`, studentID, secondStudentID).Scan(&dailyRows))
	if lessonRows != 1 || lessonHistoryRows != 1 || dailyRows != 0 {
		t.Fatalf("unexpected persisted records: lesson=%d history=%d daily=%d", lessonRows, lessonHistoryRows, dailyRows)
	}
	var savedStatus, savedName, savedNIS, snapshotName, snapshotNIS, historyAction, historyStatus string
	var historyActor int64
	var historyHasBefore, historySnapshotMatches bool
	must(db.QueryRow(`SELECT a.status,a.student_name,a.nis,a.snapshot->'student'->>'name',a.snapshot->'student'->>'nis',h.action,h.actor_id,h.before_state IS NOT NULL,h.after_state->>'status',h.after_state->'snapshot'=a.snapshot FROM app.student_lesson_attendance a JOIN app.student_lesson_attendance_history h ON h.attendance_id=a.id WHERE a.timetable_entry_id=$1 AND a.meeting_date='2020-07-07' AND a.student_id=$2`, entryID, studentID).Scan(&savedStatus, &savedName, &savedNIS, &snapshotName, &snapshotNIS, &historyAction, &historyActor, &historyHasBefore, &historyStatus, &historySnapshotMatches))
	if savedStatus != "hadir" || savedName != "Siswa Konteks Uji" || snapshotName != savedName || snapshotNIS != savedNIS || historyAction != "recorded" || historyActor != teacher.ID || historyHasBefore || historyStatus != savedStatus || !historySnapshotMatches {
		t.Fatalf("saved row/history evidence mismatch: status=%q name=%q snapshotName=%q nisMatch=%t action=%q actor=%d hasBefore=%t historyStatus=%q snapshotMatches=%t", savedStatus, savedName, snapshotName, snapshotNIS == savedNIS, historyAction, historyActor, historyHasBefore, historyStatus, historySnapshotMatches)
	}
	if script := get(teacherClient, "/static/lesson-attendance.js", http.StatusOK); !strings.Contains(script, "data-attendance-mark-all") {
		t.Fatal("explicit mark-all behavior asset is unavailable")
	}
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE student_id=$1`, studentID).Scan(&after))
	if after != before {
		t.Fatal("read-only GET/route altered daily attendance")
	}
}

func cloneValues(input url.Values) url.Values {
	clone := make(url.Values, len(input))
	for key, values := range input {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
