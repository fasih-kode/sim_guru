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
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	year, err := s.CreateYear(ctx, curriculum.ID, "Lesson context "+testutil.Random(t)[:10])
	must(err)
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	var semester, teacherID, subjectID, studentID int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Konteks Uji',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "LC"+testutil.Random(t)[:8], "Mapel Konteks Uji").Scan(&subjectID))
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Konteks Uji','2013-01-01','L') RETURNING id`).Scan(&studentID))
	_, err = s.CreatePlacement(ctx, admin.ID, studentID, class.ID, "2020-07-01")
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
	dateURL := "/jadwal-saya/absensi?date=2020-07-07"
	if body := get(teacherClient, dateURL, http.StatusOK); !strings.Contains(body, "Mapel Konteks Uji") || !strings.Contains(body, "Buka roster") {
		t.Fatal("teacher's dated published meeting is missing")
	}
	var entryID int64
	must(db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1`, revision.ID).Scan(&entryID))
	detailURL := fmt.Sprintf("%s&entry_id=%d", dateURL, entryID)
	detail := get(teacherClient, detailURL, http.StatusOK)
	for _, expected := range []string{"Siswa Konteks Uji", "Belum Dicatat", "Guru Konteks Uji", "07:00"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("lesson detail missing %q", expected)
		}
	}
	get(adminClient, dateURL, http.StatusForbidden)
	get(curriculumClient, dateURL, http.StatusForbidden)
	get(teacherClient, dateURL+"&entry_id=999999999", http.StatusNotFound)
	get(teacherClient, "/jadwal-saya/absensi?date=not-a-date", http.StatusUnprocessableEntity)
	var before, after int
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE student_id=$1`, studentID).Scan(&before))
	response, postErr := teacherClient.PostForm(server.URL+"/jadwal-saya/absensi", url.Values{"date": {"2020-07-07"}})
	must(postErr)
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed && response.StatusCode != http.StatusForbidden {
		t.Fatalf("POST unexpectedly accepted: HTTP %d", response.StatusCode)
	}
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE student_id=$1`, studentID).Scan(&after))
	if after != before {
		t.Fatal("read-only GET/route altered daily attendance")
	}
}
