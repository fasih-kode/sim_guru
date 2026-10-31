//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeachingJournalReadHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &academic.Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	teacher, teacherPassword := testutil.User(t, db, "guru_mapel")
	otherTeacher, _ := testutil.User(t, db, "guru_mapel")
	var oldYear, oldSemester int64
	_ = db.QueryRow(`SELECT id FROM app.school_years WHERE active`).Scan(&oldYear)
	_ = db.QueryRow(`SELECT id FROM app.semesters WHERE active`).Scan(&oldSemester)
	date := time.Now().In(time.FixedZone("WIB", 7*60*60))
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	year, err := s.CreateYear(ctx, cur.ID, "Read jurnal "+testutil.Random(t)[:10])
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateYear(ctx, cur.ID, year.ID); err != nil {
		t.Fatal(err)
	}
	var semester int64
	if err = db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester); err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateSemester(ctx, cur.ID, semester); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`UPDATE app.school_years SET active=false WHERE id=$1`, year.ID)
		if oldYear > 0 {
			_, _ = db.Exec(`UPDATE app.school_years SET active=true WHERE id=$1`, oldYear)
		}
		_, _ = db.Exec(`UPDATE app.semesters SET active=false WHERE id=$1`, semester)
		if oldSemester > 0 {
			_, _ = db.Exec(`UPDATE app.semesters SET active=true WHERE id=$1`, oldSemester)
		}
	})
	if err = s.SaveSemesterCalendar(ctx, cur.ID, academic.SemesterCalendar{SemesterID: semester, StartsOn: date.AddDate(0, 0, -30).Format("2006-01-02"), EndsOn: date.AddDate(0, 0, 120).Format("2006-01-02"), SchoolDays: []int{weekday}}, "Kalender jurnal HTTP"); err != nil {
		t.Fatal(err)
	}
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "R"+testutil.Random(t)[:5], "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var teacherID, subjectID int64
	if err = db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru baca jurnal',$1) RETURNING id`, teacher.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel baca jurnal') RETURNING id`, "R"+testutil.Random(t)[:10]).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	if err = s.AssignTeacher(ctx, cur.ID, semester, class.ID, subjectID, teacherID, date.AddDate(0, 0, -20)); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveLessonDay(ctx, cur.ID, semester, weekday, []academic.LessonPeriod{{Number: 1, StartsAt: "07:00", EndsAt: "07:40"}}); err != nil {
		t.Fatal(err)
	}
	revision, err := s.CreateTimetableDraft(ctx, cur.ID, semester, date.AddDate(0, 0, -10))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, revision.ID, weekday, class.ID, subjectID, teacherID, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.PublishTimetable(ctx, cur.ID, revision.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CutoverTeachingJournalLineage(ctx, cur.ID, "Persiapan route baca jurnal"); err != nil {
		t.Fatal(err)
	}
	var logicalID int64
	if err = db.QueryRow(`SELECT logical_lesson_id FROM app.teaching_journal_lineage_decisions WHERE to_revision_id=$1 AND decision='cutover'`, revision.ID).Scan(&logicalID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveTeachingJournal(ctx, teacher.ID, academic.TeachingJournalInput{LogicalLessonID: logicalID, OccurrenceDate: date, RequestKey: "abcd1234-1111-4111-8111-123456789012", State: "draft", Outcome: "held"}); err != nil {
		t.Fatal(err)
	}
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(day int) string { return strconv.Itoa(day) },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templates, options: Options{Academics: s, Accounts: &auth.Store{}, Masters: &master.Store{}}}
	request := func(handler http.Handler, user auth.User, path string, pathValues map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		for key, value := range pathValues {
			r.SetPathValue(key, value)
		}
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	list := request(a.require("guru_mapel", a.teachingJournalList), teacher, "/jurnal-saya?semester_id="+strconv.FormatInt(semester, 10)+"&class_id="+strconv.FormatInt(class.ID, 10)+"&subject_id="+strconv.FormatInt(subjectID, 10)+"&state=draft", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Mapel baca jurnal") {
		t.Fatalf("authorized journal list failed: %d %s", list.Code, list.Body.String())
	}
	newDate := date.AddDate(0, 0, 7).Format("2006-01-02")
	if _, _, occurrenceErr := s.TeachingJournalOccurrences(ctx, teacher.ID, newDate); occurrenceErr != nil {
		t.Fatalf("eligible occurrence service failed: %v", occurrenceErr)
	}
	newForm := request(a.require("guru_mapel", a.teachingJournalNew), teacher, "/jurnal-saya/baru?date="+newDate, nil)
	if newForm.Code != http.StatusOK || !strings.Contains(newForm.Body.String(), "Isi jurnal") || !strings.Contains(newForm.Body.String(), "Jam ke-1") {
		t.Fatalf("eligible occurrence form failed: %d %s", newForm.Code, newForm.Body.String())
	}
	var journalID int64
	if err = db.QueryRow(`SELECT id FROM app.teaching_journals WHERE logical_lesson_id=$1 AND meeting_date=$2`, logicalID, date.Format("2006-01-02")).Scan(&journalID); err != nil {
		t.Fatal(err)
	}
	detail := request(a.require("guru_mapel", a.teachingJournalDetail), teacher, "/jurnal-saya/"+strconv.FormatInt(journalID, 10), map[string]string{"id": strconv.FormatInt(journalID, 10)})
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "Mapel baca jurnal") || !strings.Contains(detail.Body.String(), "Jam ke-1") {
		t.Fatalf("authorized journal detail failed: %d %s", detail.Code, detail.Body.String())
	}
	version := request(a.require("guru_mapel", a.teachingJournalVersion), teacher, "/jurnal-saya/"+strconv.FormatInt(journalID, 10)+"/versi/1", map[string]string{"id": strconv.FormatInt(journalID, 10), "version": "1"})
	if version.Code != http.StatusOK || !strings.Contains(version.Body.String(), "Versi 1") {
		t.Fatalf("authorized historical version route failed: %d %s", version.Code, version.Body.String())
	}
	crossOwner := request(a.require("guru_mapel", a.teachingJournalDetail), otherTeacher, "/jurnal-saya/"+strconv.FormatInt(journalID, 10), map[string]string{"id": strconv.FormatInt(journalID, 10)})
	if crossOwner.Code != http.StatusNotFound {
		t.Fatalf("cross-owner journal id was not hidden at HTTP boundary: %d", crossOwner.Code)
	}
	adminResponse := request(a.require("guru_mapel", a.teachingJournalList), admin, "/jurnal-saya", nil)
	if adminResponse.Code != http.StatusForbidden {
		t.Fatalf("Admin unexpectedly received journal read permission: %d", adminResponse.Code)
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	sessionToken, err := accounts.Login(ctx, teacher.Username, teacherPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	csrfKey := make([]byte, 32)
	if _, err = rand.Read(csrfKey); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: csrfKey})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	serverURL, _ := url.Parse(server.URL)
	jar.SetCookies(serverURL, []*http.Cookie{{Name: sessionCookie, Value: sessionToken}})
	readPage := func(path string, expected int) string {
		t.Helper()
		response, getErr := client.Get(server.URL + path)
		if getErr != nil {
			t.Fatal(getErr)
		}
		defer response.Body.Close()
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != expected {
			t.Fatalf("GET %s got %d want %d: %s", path, response.StatusCode, expected, body)
		}
		return string(body)
	}
	csrfToken := func(body string) string {
		t.Helper()
		match := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(body)
		if len(match) != 2 {
			t.Fatal("Jurnal form does not contain CSRF token")
		}
		return match[1]
	}
	postJournal := func(path string, values url.Values, expected int) *http.Response {
		t.Helper()
		response, postErr := client.PostForm(server.URL+path, values)
		if postErr != nil {
			t.Fatal(postErr)
		}
		if response.StatusCode != expected {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("POST %s got %d want %d: %s", path, response.StatusCode, expected, body)
		}
		return response
	}
	newFormPath := "/jurnal-saya/baru?date=" + newDate + "&lesson_id=" + strconv.FormatInt(logicalID, 10)
	newFormBody := readPage(newFormPath, http.StatusOK)
	noCSRF := url.Values{"action": {"save_draft"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"0"}, "request_key": {"21111111-1111-4111-8111-111111111111"}, "outcome": {"held"}}
	noCSRFAttempt := postJournal("/jurnal-saya", noCSRF, http.StatusForbidden)
	noCSRFAttempt.Body.Close()
	csrf := csrfToken(newFormBody)
	requestKey1 := "21111111-1111-4111-8111-111111111111"
	createValues := url.Values{"gorilla.csrf.Token": {csrf}, "action": {"save_draft"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"0"}, "request_key": {requestKey1}, "outcome": {"held"}, "material": {"Draf dari form operasional"}}
	created := postJournal("/jurnal-saya", createValues, http.StatusSeeOther)
	location := created.Header.Get("Location")
	created.Body.Close()
	if !strings.HasPrefix(location, "/jurnal-saya/") {
		t.Fatalf("successful save did not redirect to journal detail: %q", location)
	}
	replayed := postJournal("/jurnal-saya", createValues, http.StatusSeeOther)
	if replayed.Header.Get("Location") != location {
		t.Fatalf("idempotent retry redirected elsewhere: %q want %q", replayed.Header.Get("Location"), location)
	}
	replayed.Body.Close()
	var createdJournalID int64
	if _, err = fmt.Sscanf(strings.TrimPrefix(location, "/jurnal-saya/"), "%d", &createdJournalID); err != nil {
		t.Fatal(err)
	}
	var versionRows int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_versions WHERE journal_id=$1`, createdJournalID).Scan(&versionRows); err != nil || versionRows != 1 {
		t.Fatalf("double submit created duplicate journal version: rows=%d err=%v", versionRows, err)
	}
	editPath := "/jurnal-saya/" + strconv.FormatInt(createdJournalID, 10) + "/edit"
	editBody := readPage(editPath, http.StatusOK)
	updateValues := url.Values{"gorilla.csrf.Token": {csrfToken(editBody)}, "action": {"save_draft"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"1"}, "request_key": {"31111111-1111-4111-8111-111111111111"}, "outcome": {"held"}, "material": {"Draf dilanjutkan"}}
	updated := postJournal("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/simpan", updateValues, http.StatusSeeOther)
	updated.Body.Close()
	editBody = readPage(editPath, http.StatusOK)
	finalValues := url.Values{"gorilla.csrf.Token": {csrfToken(editBody)}, "action": {"finalize"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"2"}, "request_key": {"41111111-1111-4111-8111-111111111111"}, "outcome": {"held"}, "material": {"Materi final"}}
	finalized := postJournal("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/simpan", finalValues, http.StatusSeeOther)
	finalized.Body.Close()
	editBody = readPage(editPath, http.StatusOK)
	staleValues := url.Values{"gorilla.csrf.Token": {csrfToken(editBody)}, "action": {"finalize"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"2"}, "request_key": {"51111111-1111-4111-8111-111111111111"}, "outcome": {"held"}, "material": {"Versi basi"}, "correction_reason": {"Uji konflik"}}
	stale := postJournal("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/simpan", staleValues, http.StatusConflict)
	stale.Body.Close()
	editBody = readPage(editPath, http.StatusOK)
	invalidTransition := url.Values{"gorilla.csrf.Token": {csrfToken(editBody)}, "action": {"save_draft"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"3"}, "request_key": {"61111111-1111-4111-8111-111111111111"}, "outcome": {"held"}, "material": {"Kembali draf"}, "correction_reason": {"Alasan percobaan"}}
	rejectedTransition := postJournal("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/simpan", invalidTransition, http.StatusUnprocessableEntity)
	rejectedTransition.Body.Close()
	editBody = readPage(editPath, http.StatusOK)
	correction := url.Values{"gorilla.csrf.Token": {csrfToken(editBody)}, "action": {"finalize"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"3"}, "request_key": {"71111111-1111-4111-8111-111111111111"}, "outcome": {"held"}, "material": {"Materi final dikoreksi"}, "activity_notes": {"Catatan final versi terkini"}, "correction_reason": {"Koreksi sesuai materi yang disampaikan"}}
	corrected := postJournal("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/simpan", correction, http.StatusSeeOther)
	corrected.Body.Close()
	for i := 0; i < 10; i++ {
		occurrenceDate := date.AddDate(0, 0, 21+7*i)
		input := academic.TeachingJournalInput{
			LogicalLessonID: logicalID, OccurrenceDate: occurrenceDate,
			RequestKey: fmt.Sprintf("92222222-2222-4222-8222-%012x", i+1), State: "final", Outcome: "held",
			Material: fmt.Sprintf("Materi pertemuan %02d", i+1), ActivityNotes: fmt.Sprintf("Catatan pertemuan %02d", i+1),
		}
		if _, err = s.SaveTeachingJournal(ctx, teacher.ID, input); err != nil {
			t.Fatalf("cannot create isolated pagination occurrence %d: %v", i+1, err)
		}
	}
	filteredURL := "/jurnal-saya?semester_id=" + strconv.FormatInt(semester, 10) + "&class_id=" + strconv.FormatInt(class.ID, 10) + "&subject_id=" + strconv.FormatInt(subjectID, 10) + "&from=" + date.Format("2006-01-02") + "&to=" + date.AddDate(0, 0, 120).Format("2006-01-02") + "&state=final&outcome=held"
	filteredFirstPage := readPage(filteredURL, http.StatusOK)
	if !strings.Contains(filteredFirstPage, "Materi pertemuan 10") || !strings.Contains(filteredFirstPage, "Catatan pertemuan 10") || !strings.Contains(filteredFirstPage, "Berikutnya") {
		t.Fatal("filtered first page did not render current material/activity or pagination")
	}
	filteredSecondPage := readPage(filteredURL+"&page=2", http.StatusOK)
	if !strings.Contains(filteredSecondPage, "Materi final dikoreksi") || !strings.Contains(filteredSecondPage, "Catatan final versi terkini") || !strings.Contains(filteredSecondPage, "Halaman 2") || strings.Contains(filteredSecondPage, "Draf dari form operasional") {
		t.Fatal("filtered second page did not render only the latest corrected version content")
	}
	hxListRequest, _ := http.NewRequest(http.MethodGet, server.URL+filteredURL+"&page=2", nil)
	hxListRequest.Header.Set("HX-Request", "true")
	hxListRequest.Header.Set("HX-List-Request", "true")
	hxListResponse, err := client.Do(hxListRequest)
	if err != nil {
		t.Fatal(err)
	}
	hxListBody, err := io.ReadAll(hxListResponse.Body)
	hxListResponse.Body.Close()
	if err != nil || hxListResponse.StatusCode != http.StatusOK || strings.Contains(string(hxListBody), "<!doctype html>") || !strings.Contains(string(hxListBody), "Materi final dikoreksi") || !strings.Contains(string(hxListBody), "Catatan final versi terkini") {
		t.Fatalf("HTMX page-two fragment lost current journal content: status=%d err=%v", hxListResponse.StatusCode, err)
	}
	historyBody := readPage("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/versi/1", http.StatusOK)
	if !strings.Contains(historyBody, "Versi 1") || strings.Contains(historyBody, "Koreksi dengan Alasan") {
		t.Fatal("historical version was not read-only")
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_versions WHERE journal_id=$1`, createdJournalID).Scan(&versionRows); err != nil || versionRows != 4 {
		t.Fatalf("journal version history count mismatch: %d %v", versionRows, err)
	}
	notHeldDate := date.AddDate(0, 0, 14).Format("2006-01-02")
	notHeldForm := readPage("/jurnal-saya/baru?date="+notHeldDate+"&lesson_id="+strconv.FormatInt(logicalID, 10), http.StatusOK)
	notHeldKey := "81111111-1111-4111-8111-111111111111"
	notHeldValues := url.Values{"gorilla.csrf.Token": {csrfToken(notHeldForm)}, "action": {"finalize"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {notHeldDate}, "expected_version": {"0"}, "request_key": {notHeldKey}, "outcome": {"not_held"}}
	invalidNotHeld := postJournal("/jurnal-saya", notHeldValues, http.StatusUnprocessableEntity)
	invalidNotHeld.Body.Close()
	notHeldValues.Set("not_held_reason", "Kegiatan sekolah dibatalkan")
	notHeldValues.Set("material", "Rencana pembelajaran sebagai konteks")
	notHeldValues.Set("activity_notes", "Catatan konteks kegiatan")
	notHeld := postJournal("/jurnal-saya", notHeldValues, http.StatusSeeOther)
	notHeldLocation := notHeld.Header.Get("Location")
	notHeld.Body.Close()
	var notHeldOutcome string
	if err = db.QueryRow(`SELECT v.outcome FROM app.teaching_journals j JOIN app.teaching_journal_versions v ON v.journal_id=j.id AND v.version_no=j.current_version_no WHERE j.logical_lesson_id=$1 AND j.meeting_date=$2::date`, logicalID, notHeldDate).Scan(&notHeldOutcome); err != nil || notHeldOutcome != "not_held" || !strings.HasPrefix(notHeldLocation, "/jurnal-saya/") {
		t.Fatalf("Tidak Terlaksana did not save with required reason: outcome=%q location=%q err=%v", notHeldOutcome, notHeldLocation, err)
	}
	notHeldList := readPage("/jurnal-saya?semester_id="+strconv.FormatInt(semester, 10)+"&class_id="+strconv.FormatInt(class.ID, 10)+"&subject_id="+strconv.FormatInt(subjectID, 10)+"&outcome=not_held", http.StatusOK)
	if !strings.Contains(notHeldList, "Rencana pembelajaran sebagai konteks") || !strings.Contains(notHeldList, "Catatan konteks kegiatan") || !strings.Contains(notHeldList, "Konteks, bukan bukti pembelajaran") {
		t.Fatal("not-held journal list omitted current context or its explanation")
	}
}
