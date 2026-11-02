//go:build integration

package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
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
	"github.com/xuri/excelize/v2"
)

func seedJournalTeacherCheckIn(t *testing.T, s *academic.Store, ctx context.Context, adminID, teacherID int64, date time.Time) {
	t.Helper()
	_, err := s.RecordTeacherAttendanceManual(ctx, adminID, academic.TeacherAttendanceManualInput{
		TeacherID: teacherID, Action: academic.TeacherCheckIn,
		OccurredAt: date.Format("2006-01-02") + "T00:01", Reason: "Bukti presensi sintetis untuk integrasi jurnal",
		IdempotencyKey: testutil.Random(t)[:32],
	})
	if err != nil {
		t.Fatal("cannot prepare eligible teacher attendance evidence", err)
	}
}

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
	classes := []academic.Class{class}
	for i := 2; i <= 12; i++ {
		item, createErr := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, fmt.Sprintf("R%02d%s", i, testutil.Random(t)[:4]), "", 0)
		if createErr != nil {
			t.Fatal(createErr)
		}
		classes = append(classes, item)
	}
	var teacherID, subjectID int64
	if err = db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru baca jurnal',$1) RETURNING id`, teacher.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'admin')`, teacher.ID); err != nil {
		t.Fatal(err)
	}
	seedJournalTeacherCheckIn(t, s, ctx, teacher.ID, teacherID, date)
	if _, err = db.Exec(`INSERT INTO app.teachers(name,user_id) VALUES('Guru lain baca jurnal',$1)`, otherTeacher.ID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel baca jurnal') RETURNING id`, "R"+testutil.Random(t)[:10]).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	if err = s.AssignTeacher(ctx, cur.ID, semester, class.ID, subjectID, teacherID, date.AddDate(0, 0, -20)); err != nil {
		t.Fatal(err)
	}
	for _, item := range classes[1:] {
		if err = s.AssignTeacher(ctx, cur.ID, semester, item.ID, subjectID, teacherID, date.AddDate(0, 0, -20)); err != nil {
			t.Fatal(err)
		}
	}
	periods := make([]academic.LessonPeriod, 0, len(classes))
	firstPeriodStart, _ := time.Parse("15:04", "06:00")
	for i := range classes {
		start := firstPeriodStart.Add(time.Duration(i) * 40 * time.Minute)
		periods = append(periods, academic.LessonPeriod{Number: i + 1, StartsAt: start.Format("15:04"), EndsAt: start.Add(40 * time.Minute).Format("15:04")})
	}
	if err = s.SaveLessonDay(ctx, cur.ID, semester, weekday, periods); err != nil {
		t.Fatal(err)
	}
	revision, err := s.CreateTimetableDraft(ctx, cur.ID, semester, date.AddDate(0, 0, -10))
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range classes {
		if err = s.AddTimetableEntry(ctx, cur.ID, revision.ID, weekday, item.ID, subjectID, teacherID, i+1, i+1); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.PublishTimetable(ctx, cur.ID, revision.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CutoverTeachingJournalLineage(ctx, cur.ID, "Persiapan route baca jurnal"); err != nil {
		t.Fatal(err)
	}
	var logicalID int64
	if err = db.QueryRow(`SELECT d.logical_lesson_id FROM app.teaching_journal_lineage_decisions d JOIN app.weekly_timetable_entries e ON e.id=d.target_entry_id WHERE d.to_revision_id=$1 AND d.decision='cutover' AND e.class_id=$2`, revision.ID, class.ID).Scan(&logicalID); err != nil {
		t.Fatal(err)
	}
	logicalIDs := make([]int64, len(classes))
	logicalIDs[0] = logicalID
	for i, item := range classes[1:] {
		if err = db.QueryRow(`SELECT d.logical_lesson_id FROM app.teaching_journal_lineage_decisions d JOIN app.weekly_timetable_entries e ON e.id=d.target_entry_id WHERE d.to_revision_id=$1 AND d.decision='cutover' AND e.class_id=$2`, revision.ID, item.ID).Scan(&logicalIDs[i+1]); err != nil {
			t.Fatal(err)
		}
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
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Jurnal Mengajar Saya") {
		t.Fatalf("authorized journal list failed: %d %s", list.Code, list.Body.String())
	}
	newDate := date.Format("2006-01-02")
	if _, _, occurrenceErr := s.TeachingJournalOccurrences(ctx, teacher.ID, newDate); occurrenceErr != nil {
		t.Fatalf("eligible occurrence service failed: %v", occurrenceErr)
	}
	schedulePage := request(a.require("guru_mapel", a.myTimetable), teacher, "/jadwal-saya", nil)
	if schedulePage.Code != http.StatusOK || !strings.Contains(schedulePage.Body.String(), `action="/jurnal-saya/baru"`) || !strings.Contains(schedulePage.Body.String(), `name="date"`) || !strings.Contains(schedulePage.Body.String(), `data-list-navigation`) || !strings.Contains(schedulePage.Body.String(), "Lihat Pertemuan") {
		t.Fatalf("schedule page does not provide an actionable date picker: %d %s", schedulePage.Code, schedulePage.Body.String())
	}
	instantSearch, err := content.ReadFile("static/instant-search.js")
	if err != nil || !strings.Contains(string(instantSearch), `form.hasAttribute('data-list-navigation')`) {
		t.Fatalf("native navigation forms are not excluded from HTMX list enhancement: %v", err)
	}
	newForm := request(a.require("guru_mapel", a.teachingJournalNew), teacher, "/jurnal-saya/baru?date="+newDate, nil)
	dateOccurrenceLink := `href="/jurnal-saya/baru?date=` + newDate + `&amp;lesson_id=` + strconv.FormatInt(logicalID, 10) + `"`
	if newForm.Code != http.StatusOK || !strings.Contains(newForm.Body.String(), "Isi jurnal") || !strings.Contains(newForm.Body.String(), "Jam ke-1") || !strings.Contains(newForm.Body.String(), dateOccurrenceLink) {
		t.Fatalf("eligible occurrence form failed: %d %s", newForm.Code, newForm.Body.String())
	}
	invalidDate := request(a.require("guru_mapel", a.teachingJournalNew), teacher, "/jurnal-saya/baru?date=2026-13-01", nil)
	if invalidDate.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid occurrence date was not rejected: %d", invalidDate.Code)
	}
	crossTeacherOccurrence := request(a.require("guru_mapel", a.teachingJournalNew), otherTeacher, "/jurnal-saya/baru?date="+newDate+"&lesson_id="+strconv.FormatInt(logicalID, 10), nil)
	if crossTeacherOccurrence.Code != http.StatusNotFound {
		t.Fatalf("a teacher could select another teacher's occurrence: %d", crossTeacherOccurrence.Code)
	}
	wrongWeekday := date.AddDate(0, 0, 8).Format("2006-01-02")
	wrongDayOccurrences, _, err := s.TeachingJournalOccurrences(ctx, teacher.ID, wrongWeekday)
	if err != nil || len(wrongDayOccurrences) != 0 {
		t.Fatalf("weekly slot was incorrectly presented on another weekday: occurrences=%d err=%v", len(wrongDayOccurrences), err)
	}
	wrongDayPage := request(a.require("guru_mapel", a.teachingJournalNew), teacher, "/jurnal-saya/baru?date="+wrongWeekday, nil)
	if wrongDayPage.Code != http.StatusOK || strings.Contains(wrongDayPage.Body.String(), dateOccurrenceLink) || !strings.Contains(wrongDayPage.Body.String(), "Tidak ada pertemuan yang dapat dijurnal") {
		t.Fatalf("date without a matching weekly slot did not show the empty state: %d %s", wrongDayPage.Code, wrongDayPage.Body.String())
	}
	selectedForm := request(a.require("guru_mapel", a.teachingJournalNew), teacher, "/jurnal-saya/baru?date="+newDate+"&lesson_id="+strconv.FormatInt(logicalID, 10), nil)
	if selectedForm.Code != http.StatusOK || !strings.Contains(selectedForm.Body.String(), `name="logical_lesson_id" value="`+strconv.FormatInt(logicalID, 10)+`"`) || !strings.Contains(selectedForm.Body.String(), `name="meeting_date" value="`+newDate+`"`) {
		t.Fatalf("selected occurrence did not open its matching journal form: %d %s", selectedForm.Code, selectedForm.Body.String())
	}
	var journalCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journals WHERE owner_user_id=$1`, teacher.ID).Scan(&journalCount); err != nil {
		t.Fatal(err)
	}
	if journalCount != 0 {
		t.Fatalf("GET date/occurrence flow created a journal unexpectedly: count=%d", journalCount)
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
	journalID := createdJournalID
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
	var versionRows int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_versions WHERE journal_id=$1`, createdJournalID).Scan(&versionRows); err != nil || versionRows != 1 {
		t.Fatalf("double submit created duplicate journal version: rows=%d err=%v", versionRows, err)
	}
	if savedList := readPage("/jurnal-saya?semester_id="+strconv.FormatInt(semester, 10)+"&class_id="+strconv.FormatInt(class.ID, 10)+"&subject_id="+strconv.FormatInt(subjectID, 10)+"&state=draft", http.StatusOK); !strings.Contains(savedList, "Mapel baca jurnal") {
		t.Fatal("authorized journal list omitted the saved journal")
	}
	legacyTimeInput := url.Values{"gorilla.csrf.Token": {csrf}, "action": {"save_draft"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"1"}, "request_key": {"22111111-2222-4222-8222-111111111111"}, "outcome": {"held"}, "material": {"Draf tanpa waktu input"}, "actual_start": {"07:30"}, "actual_end": {"08:00"}}
	legacyTimeResponse := postJournal("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/simpan", legacyTimeInput, http.StatusUnprocessableEntity)
	legacyTimeBody, readErr := io.ReadAll(legacyTimeResponse.Body)
	legacyTimeResponse.Body.Close()
	if readErr != nil || !strings.Contains(string(legacyTimeBody), "waktu aktual tidak dapat diinput") {
		t.Fatalf("legacy actual-time payload was not rejected with a clear message: read_err=%v", readErr)
	}
	var currentVersionAfterLegacyInput, actualTimeRows int
	if err = db.QueryRow(`SELECT current_version_no FROM app.teaching_journals WHERE id=$1`, createdJournalID).Scan(&currentVersionAfterLegacyInput); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_versions WHERE journal_id=$1 AND (actual_starts_at IS NOT NULL OR actual_ends_at IS NOT NULL)`, createdJournalID).Scan(&actualTimeRows); err != nil || currentVersionAfterLegacyInput != 1 || actualTimeRows != 0 {
		t.Fatalf("rejected legacy time input mutated the journal: version=%d historical_time_rows=%d err=%v", currentVersionAfterLegacyInput, actualTimeRows, err)
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
	for i := 1; i < 11; i++ {
		occurrenceDate := date
		input := academic.TeachingJournalInput{
			LogicalLessonID: logicalIDs[i], OccurrenceDate: occurrenceDate,
			RequestKey: fmt.Sprintf("92222222-2222-4222-8222-%012x", i+1), State: "final", Outcome: "held",
			Material: fmt.Sprintf("Materi halaman %02d", i+1), ActivityNotes: fmt.Sprintf("Catatan halaman %02d", i+1),
		}
		if _, err = s.SaveTeachingJournal(ctx, teacher.ID, input); err != nil {
			t.Fatalf("cannot create isolated pagination occurrence %d: %v", i+1, err)
		}
	}
	filteredURL := "/jurnal-saya?semester_id=" + strconv.FormatInt(semester, 10) + "&subject_id=" + strconv.FormatInt(subjectID, 10) + "&from=" + date.Format("2006-01-02") + "&to=" + date.Format("2006-01-02") + "&state=final&outcome=held"
	filteredFirstPage := readPage(filteredURL, http.StatusOK)
	if !strings.Contains(filteredFirstPage, "Materi halaman") || !strings.Contains(filteredFirstPage, "Catatan halaman") || !strings.Contains(filteredFirstPage, "Berikutnya") {
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
	exportFilter := "semester_id=" + strconv.FormatInt(semester, 10) + "&subject_id=" + strconv.FormatInt(subjectID, 10) + "&from=" + date.Format("2006-01-02") + "&to=" + date.Format("2006-01-02") + "&state=final&outcome=held"
	exportRequest := func(user auth.User, query string) *httptest.ResponseRecorder {
		t.Helper()
		return request(a.require("guru_mapel", a.teachingJournalExport), user, "/jurnal-saya/ekspor?"+query, nil)
	}
	xlsxExport := exportRequest(teacher, exportFilter+"&format=xlsx&page=2")
	if xlsxExport.Code != http.StatusOK || !strings.Contains(xlsxExport.Header().Get("Content-Type"), "spreadsheetml.sheet") {
		t.Fatalf("authorized XLSX export failed: status=%d content-type=%q", xlsxExport.Code, xlsxExport.Header().Get("Content-Type"))
	}
	workbook, openErr := excelize.OpenReader(bytes.NewReader(xlsxExport.Body.Bytes()))
	if openErr != nil {
		t.Fatalf("exported workbook is invalid: %v", openErr)
	}
	rows, rowsErr := workbook.GetRows("Sheet1")
	_ = workbook.Close()
	if rowsErr != nil || len(rows) != 12 {
		t.Fatalf("XLSX export must contain all 11 filtered rows, independent of page: rows=%d err=%v", len(rows), rowsErr)
	}
	allExportRows := make([]string, 0, len(rows))
	for _, row := range rows {
		allExportRows = append(allExportRows, strings.Join(row, "|"))
	}
	if !strings.Contains(strings.Join(allExportRows, "\n"), "Materi final dikoreksi") || !strings.Contains(strings.Join(allExportRows, "\n"), "Catatan final versi terkini") {
		t.Fatalf("XLSX export omitted the latest corrected version: rows=%v", rows)
	}
	pdfExport := exportRequest(teacher, exportFilter+"&format=pdf&page=2")
	if pdfExport.Code != http.StatusOK || !bytes.HasPrefix(pdfExport.Body.Bytes(), []byte("%PDF-")) || !strings.Contains(pdfExport.Header().Get("Content-Disposition"), "jurnal-saya.pdf") {
		t.Fatalf("authorized PDF export failed: status=%d", pdfExport.Code)
	}
	otherExport := exportRequest(otherTeacher, exportFilter+"&format=xlsx")
	if otherExport.Code != http.StatusOK {
		t.Fatalf("other teacher export should return an empty owner-scoped file, got %d", otherExport.Code)
	}
	otherBook, openErr := excelize.OpenReader(bytes.NewReader(otherExport.Body.Bytes()))
	if openErr != nil {
		t.Fatalf("other teacher empty workbook is invalid: %v", openErr)
	}
	otherRows, rowsErr := otherBook.GetRows("Sheet1")
	_ = otherBook.Close()
	if rowsErr != nil || len(otherRows) != 1 {
		t.Fatalf("export leaked another teacher's journals: rows=%d err=%v", len(otherRows), rowsErr)
	}
	adminExport := exportRequest(admin, exportFilter+"&format=pdf")
	if adminExport.Code != http.StatusForbidden {
		t.Fatalf("Admin unexpectedly received journal export permission: %d", adminExport.Code)
	}
	historyBody := readPage("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/versi/1", http.StatusOK)
	if !strings.Contains(historyBody, "Versi 1") || strings.Contains(historyBody, "Koreksi dengan Alasan") {
		t.Fatal("historical version was not read-only")
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_versions WHERE journal_id=$1`, createdJournalID).Scan(&versionRows); err != nil || versionRows != 4 {
		t.Fatalf("journal version history count mismatch: %d %v", versionRows, err)
	}
	notHeldDate := date.Format("2006-01-02")
	notHeldLogicalID := logicalIDs[11]
	notHeldForm := readPage("/jurnal-saya/baru?date="+notHeldDate+"&lesson_id="+strconv.FormatInt(notHeldLogicalID, 10), http.StatusOK)
	notHeldKey := "81111111-1111-4111-8111-111111111111"
	notHeldValues := url.Values{"gorilla.csrf.Token": {csrfToken(notHeldForm)}, "action": {"finalize"}, "logical_lesson_id": {strconv.FormatInt(notHeldLogicalID, 10)}, "meeting_date": {notHeldDate}, "expected_version": {"0"}, "request_key": {notHeldKey}, "outcome": {"not_held"}}
	invalidNotHeld := postJournal("/jurnal-saya", notHeldValues, http.StatusUnprocessableEntity)
	invalidNotHeld.Body.Close()
	notHeldValues.Set("not_held_reason", "Kegiatan sekolah dibatalkan")
	notHeldValues.Set("material", "Rencana pembelajaran sebagai konteks")
	notHeldValues.Set("activity_notes", "Catatan konteks kegiatan")
	notHeld := postJournal("/jurnal-saya", notHeldValues, http.StatusSeeOther)
	notHeldLocation := notHeld.Header.Get("Location")
	notHeld.Body.Close()
	var notHeldOutcome string
	if err = db.QueryRow(`SELECT v.outcome FROM app.teaching_journals j JOIN app.teaching_journal_versions v ON v.journal_id=j.id AND v.version_no=j.current_version_no WHERE j.logical_lesson_id=$1 AND j.meeting_date=$2::date`, notHeldLogicalID, notHeldDate).Scan(&notHeldOutcome); err != nil || notHeldOutcome != "not_held" || !strings.HasPrefix(notHeldLocation, "/jurnal-saya/") {
		t.Fatalf("Tidak Terlaksana did not save with required reason: outcome=%q location=%q err=%v", notHeldOutcome, notHeldLocation, err)
	}
	notHeldList := readPage("/jurnal-saya?semester_id="+strconv.FormatInt(semester, 10)+"&subject_id="+strconv.FormatInt(subjectID, 10)+"&outcome=not_held", http.StatusOK)
	if !strings.Contains(notHeldList, "Rencana pembelajaran sebagai konteks") || !strings.Contains(notHeldList, "Catatan konteks kegiatan") || !strings.Contains(notHeldList, "Konteks, bukan bukti pembelajaran") {
		t.Fatal("not-held journal list omitted current context or its explanation")
	}
	if _, err = s.RecordTeacherAttendanceManual(ctx, teacher.ID, academic.TeacherAttendanceManualInput{TeacherID: teacherID, Action: academic.TeacherCheckOut, OccurredAt: date.Format("2006-01-02") + "T00:02", Reason: "Bukti presensi sintetis untuk uji blok mutasi", IdempotencyKey: testutil.Random(t)[:32]}); err != nil {
		t.Fatal("cannot prepare contradictory teacher attendance evidence", err)
	}
	blockedEdit := readPage(editPath, http.StatusOK)
	if !strings.Contains(blockedEdit, "mendahului waktu selesai pertemuan") {
		t.Fatal("edit form did not explain why attendance now blocks mutation")
	}
	blockedCorrection := url.Values{"gorilla.csrf.Token": {csrfToken(blockedEdit)}, "action": {"finalize"}, "logical_lesson_id": {strconv.FormatInt(logicalID, 10)}, "meeting_date": {newDate}, "expected_version": {"4"}, "request_key": {"a5111111-1111-4111-8111-111111111111"}, "outcome": {"held"}, "material": {"Mutasi yang harus ditolak"}, "correction_reason": {"Percobaan koreksi saat presensi tidak memenuhi syarat"}}
	blockedResponse := postJournal("/jurnal-saya/"+strconv.FormatInt(createdJournalID, 10)+"/simpan", blockedCorrection, http.StatusConflict)
	blockedBody, blockedReadErr := io.ReadAll(blockedResponse.Body)
	blockedResponse.Body.Close()
	if blockedReadErr != nil || !strings.Contains(string(blockedBody), "mendahului waktu selesai pertemuan") || !strings.Contains(string(blockedBody), "Mutasi yang harus ditolak") || !strings.Contains(string(blockedBody), "Percobaan koreksi saat presensi tidak memenuhi syarat") {
		t.Fatalf("direct HTTP journal mutation did not return a clear attendance block while preserving user input: read_err=%v", blockedReadErr)
	}
	if err = db.QueryRow(`SELECT current_version_no FROM app.teaching_journals WHERE id=$1`, createdJournalID).Scan(&currentVersionAfterLegacyInput); err != nil || currentVersionAfterLegacyInput != 4 {
		t.Fatalf("attendance block changed immutable journal history: version=%d err=%v", currentVersionAfterLegacyInput, err)
	}
}

func TestTeachingJournalReviewIndicatorIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &academic.Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
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
	year, err := s.CreateYear(ctx, cur.ID, "Review jurnal "+testutil.Random(t)[:10])
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
	if err = s.SaveSemesterCalendar(ctx, cur.ID, academic.SemesterCalendar{SemesterID: semester, StartsOn: date.AddDate(0, 0, -30).Format("2006-01-02"), EndsOn: date.AddDate(0, 0, 60).Format("2006-01-02"), SchoolDays: []int{weekday}}, "Kalender indikator jurnal"); err != nil {
		t.Fatal(err)
	}
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "V"+testutil.Random(t)[:5], "", 0)
	if err != nil {
		t.Fatal(err)
	}
	stableClass, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "W"+testutil.Random(t)[:5], "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var teacherID, subjectID int64
	if err = db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru indikator jurnal',$1) RETURNING id`, teacher.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'admin')`, teacher.ID); err != nil {
		t.Fatal(err)
	}
	seedJournalTeacherCheckIn(t, s, ctx, teacher.ID, teacherID, date)
	if err = db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel indikator jurnal') RETURNING id`, "V"+testutil.Random(t)[:10]).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	if err = s.AssignTeacher(ctx, cur.ID, semester, class.ID, subjectID, teacherID, date.AddDate(0, 0, -20)); err != nil {
		t.Fatal(err)
	}
	if err = s.AssignTeacher(ctx, cur.ID, semester, stableClass.ID, subjectID, teacherID, date.AddDate(0, 0, -20)); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveLessonDay(ctx, cur.ID, semester, weekday, []academic.LessonPeriod{{Number: 1, StartsAt: "07:00", EndsAt: "07:40"}, {Number: 2, StartsAt: "07:40", EndsAt: "08:20"}, {Number: 3, StartsAt: "08:20", EndsAt: "09:00"}}); err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateTimetableDraft(ctx, cur.ID, semester, date.AddDate(0, 0, -10))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, first.ID, weekday, class.ID, subjectID, teacherID, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, first.ID, weekday, stableClass.ID, subjectID, teacherID, 2, 2); err != nil {
		t.Fatal(err)
	}
	if err = s.PublishTimetable(ctx, cur.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CutoverTeachingJournalLineage(ctx, cur.ID, "Cutover indikator jurnal"); err != nil {
		t.Fatal(err)
	}
	var logicalID, firstEntry, stableLogicalID, stableFirstEntry int64
	if err = db.QueryRow(`SELECT logical_lesson_id,target_entry_id FROM app.teaching_journal_lineage_decisions WHERE to_revision_id=$1 AND decision='cutover' AND target_entry_id=(SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND class_id=$2)`, first.ID, class.ID).Scan(&logicalID, &firstEntry); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT logical_lesson_id,target_entry_id FROM app.teaching_journal_lineage_decisions WHERE to_revision_id=$1 AND decision='cutover' AND target_entry_id=(SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND class_id=$2)`, first.ID, stableClass.ID).Scan(&stableLogicalID, &stableFirstEntry); err != nil {
		t.Fatal(err)
	}
	unchangedDate := date
	changedDate := date
	unchanged, err := s.SaveTeachingJournal(ctx, teacher.ID, academic.TeachingJournalInput{LogicalLessonID: stableLogicalID, OccurrenceDate: unchangedDate, RequestKey: "a1111111-1111-4111-8111-111111111111", State: "draft", Outcome: "not_held", NotHeldReason: "Pertemuan dibatalkan"})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveTeachingJournal(ctx, teacher.ID, academic.TeachingJournalInput{LogicalLessonID: logicalID, OccurrenceDate: changedDate, RequestKey: "a2222222-2222-4222-8222-222222222222", State: "draft", Outcome: "held", Material: "Materi draf sebelum revisi"})
	if err != nil {
		t.Fatal(err)
	}
	unchangedDetail, err := s.TeachingJournalDetail(ctx, teacher.ID, unchanged.ID)
	if err != nil || unchangedDetail.ReviewReason != "" {
		t.Fatalf("Tidak Terlaksana tanpa perubahan jadwal tidak boleh diberi indikator: reason=%q err=%v", unchangedDetail.ReviewReason, err)
	}
	draftBefore, err := s.TeachingJournalDetail(ctx, teacher.ID, draft.ID)
	if err != nil || draftBefore.ReviewReason != "" || len(draftBefore.CurrentVersion.Periods) != 1 || draftBefore.CurrentVersion.Periods[0].StartsAt != "07:00:00" {
		t.Fatalf("draf awal harus belum ditandai dan menyimpan snapshot waktu sendiri: detail=%#v err=%v", draftBefore, err)
	}

	second, err := s.CreateTimetableDraft(ctx, cur.ID, semester, date.AddDate(0, 0, -1))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, second.ID, weekday, class.ID, subjectID, teacherID, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, second.ID, weekday, stableClass.ID, subjectID, teacherID, 2, 2); err != nil {
		t.Fatal(err)
	}
	if err = s.PublishTimetable(ctx, cur.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	var secondEntry, stableSecondEntry int64
	if err = db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND class_id=$2`, second.ID, class.ID).Scan(&secondEntry); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND class_id=$2`, second.ID, stableClass.ID).Scan(&stableSecondEntry); err != nil {
		t.Fatal(err)
	}
	if err = s.ResolveTeachingJournalLineage(ctx, cur.ID, first.ID, second.ID, "Revision baru dengan slot yang sama", []academic.TeachingJournalLineageDecision{{Decision: "continue", SourceEntryID: firstEntry, TargetEntryID: secondEntry, Reason: "Slot dan konteks jadwal tetap sama"}, {Decision: "continue", SourceEntryID: stableFirstEntry, TargetEntryID: stableSecondEntry, Reason: "Slot pembanding tetap sama"}}); err != nil {
		t.Fatal(err)
	}
	unchangedRevisionDetail, err := s.TeachingJournalDetail(ctx, teacher.ID, draft.ID)
	if err != nil || unchangedRevisionDetail.ReviewReason != "" {
		t.Fatalf("revision baru tanpa perubahan konteks tidak boleh menandai draf: reason=%q err=%v", unchangedRevisionDetail.ReviewReason, err)
	}
	if err = s.SaveLessonDay(ctx, cur.ID, semester, weekday, []academic.LessonPeriod{{Number: 1, StartsAt: "07:00", EndsAt: "07:40"}, {Number: 2, StartsAt: "07:40", EndsAt: "08:20"}, {Number: 3, StartsAt: "08:30", EndsAt: "09:10"}}); err != nil {
		t.Fatal(err)
	}
	third, err := s.CreateTimetableDraft(ctx, cur.ID, semester, date)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, third.ID, weekday, class.ID, subjectID, teacherID, 3, 3); err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, third.ID, weekday, stableClass.ID, subjectID, teacherID, 2, 2); err != nil {
		t.Fatal(err)
	}
	if err = s.PublishTimetable(ctx, cur.ID, third.ID); err != nil {
		t.Fatal(err)
	}
	pendingOccurrences, pendingLineage, err := s.TeachingJournalOccurrences(ctx, teacher.ID, changedDate.Format("2006-01-02"))
	if err != nil || !pendingLineage || len(pendingOccurrences) != 0 {
		t.Fatalf("tanggal dengan lineage pending harus menahan pemilihan occurrence: count=%d pending=%t err=%v", len(pendingOccurrences), pendingLineage, err)
	}
	if _, err = s.SaveTeachingJournal(ctx, teacher.ID, academic.TeachingJournalInput{LogicalLessonID: logicalID, OccurrenceDate: changedDate, ExpectedVersion: 1, State: "draft", Outcome: "held", Material: "Perubahan saat lineage pending", RequestKey: "a4444444-4444-4444-8444-444444444444"}); err == nil || !strings.Contains(err.Error(), "belum memiliki keputusan kesinambungan") {
		t.Fatalf("mutasi draf saat lineage efektif pending harus ditolak: %v", err)
	}
	var versionBeforeResolution int
	if err = db.QueryRow(`SELECT current_version_no FROM app.teaching_journals WHERE id=$1`, draft.ID).Scan(&versionBeforeResolution); err != nil || versionBeforeResolution != 1 {
		t.Fatalf("penolakan lineage pending tidak boleh membuat versi parsial: version=%d err=%v", versionBeforeResolution, err)
	}
	var historyBeforeResolution int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_history WHERE journal_id=$1`, draft.ID).Scan(&historyBeforeResolution); err != nil || historyBeforeResolution != 1 {
		t.Fatalf("penolakan lineage pending tidak boleh membuat histori parsial: events=%d err=%v", historyBeforeResolution, err)
	}
	var thirdEntry, stableThirdEntry int64
	if err = db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND class_id=$2`, third.ID, class.ID).Scan(&thirdEntry); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND class_id=$2`, third.ID, stableClass.ID).Scan(&stableThirdEntry); err != nil {
		t.Fatal(err)
	}
	pendingDetail, err := s.TeachingJournalDetail(ctx, teacher.ID, draft.ID)
	if err != nil || !strings.Contains(pendingDetail.ReviewReason, "belum memiliki keputusan kesinambungan") {
		t.Fatalf("draf harus menjelaskan revision lineage yang belum terselesaikan: reason=%q err=%v", pendingDetail.ReviewReason, err)
	}
	if err = s.ResolveTeachingJournalLineage(ctx, cur.ID, second.ID, third.ID, "Ubah blok JP slot untuk uji indikator", []academic.TeachingJournalLineageDecision{{Decision: "continue", SourceEntryID: secondEntry, TargetEntryID: thirdEntry, Reason: "Slot yang sama, blok JP berubah"}, {Decision: "continue", SourceEntryID: stableSecondEntry, TargetEntryID: stableThirdEntry, Reason: "Slot pembanding tetap sama"}}); err != nil {
		t.Fatal(err)
	}
	filter := academic.BuildTeachingJournalReadFilter(1, semester, class.ID, subjectID, changedDate.Format("2006-01-02"), changedDate.Format("2006-01-02"), "draft", "")
	page, err := s.TeachingJournals(ctx, teacher.ID, filter)
	if err != nil || len(page.Items) != 1 || !strings.Contains(page.Items[0].ReviewReason, "berbeda dari snapshot draf") {
		t.Fatalf("draf terdampak revisi efektif belum ditandai: page=%#v err=%v", page, err)
	}
	afterRevision, err := s.TeachingJournalDetail(ctx, teacher.ID, draft.ID)
	if err != nil || afterRevision.ReviewReason == "" || len(afterRevision.CurrentVersion.Periods) != 1 || afterRevision.CurrentVersion.Periods[0].StartsAt != "07:00:00" {
		t.Fatalf("indikator tidak menjelaskan perubahan atau snapshot historis berubah: detail=%#v err=%v", afterRevision, err)
	}
	historicalExport, err := s.TeachingJournalsForExport(ctx, teacher.ID, academic.BuildTeachingJournalReadFilter(1, semester, class.ID, subjectID, changedDate.Format("2006-01-02"), changedDate.Format("2006-01-02"), "draft", "held"))
	if err != nil || len(historicalExport) != 1 || historicalExport[0].PeriodLabel != "Jam ke-1" || len(historicalExport[0].Periods) != 1 || historicalExport[0].Periods[0].StartsAt != "07:00:00" {
		t.Fatalf("ekspor tidak mempertahankan snapshot JP historis setelah revisi: items=%#v err=%v", historicalExport, err)
	}
	if _, err = s.TeachingJournalDetail(ctx, admin.ID, draft.ID); !errors.Is(err, academic.ErrForbidden) && !errors.Is(err, academic.ErrNotFound) {
		t.Fatalf("akses lintas role tidak ditolak: %v", err)
	}
	if _, err = s.TeachingJournalDetail(ctx, otherTeacher.ID, draft.ID); !errors.Is(err, academic.ErrNotFound) {
		t.Fatalf("indikator membocorkan jurnal lintas guru: %v", err)
	}
	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(page, index int) int { return (page-1)*10 + index + 1 }, "weekdayName": func(day int) string { return strconv.Itoa(day) }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templates, options: Options{Academics: s, Accounts: &auth.Store{}, Masters: &master.Store{}}}
	r := httptest.NewRequest(http.MethodGet, "/jurnal-saya?from="+changedDate.Format("2006-01-02")+"&to="+changedDate.Format("2006-01-02"), nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &teacher))
	w := httptest.NewRecorder()
	a.teachingJournalList(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Perlu Ditinjau") || !strings.Contains(w.Body.String(), afterRevision.ReviewReason) {
		t.Fatalf("indikator dan alasannya tidak dirender pada halaman Jurnal Saya: status=%d body=%s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "/jurnal-saya?from="+changedDate.Format("2006-01-02")+"&to="+changedDate.Format("2006-01-02"), nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-List-Request", "true")
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &teacher))
	w = httptest.NewRecorder()
	a.teachingJournalList(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Perlu Ditinjau") || strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("filter/HTMX swap tidak mempertahankan indikator: status=%d", w.Code)
	}
	r = httptest.NewRequest(http.MethodGet, "/jurnal-saya/"+strconv.FormatInt(draft.ID, 10), nil)
	r.SetPathValue("id", strconv.FormatInt(draft.ID, 10))
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &teacher))
	w = httptest.NewRecorder()
	a.teachingJournalDetail(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Perlu Ditinjau") || !strings.Contains(w.Body.String(), "berbeda dari snapshot draf") {
		t.Fatalf("indikator tidak dirender pada detail Draf: status=%d", w.Code)
	}

	final, err := s.SaveTeachingJournal(ctx, teacher.ID, academic.TeachingJournalInput{LogicalLessonID: logicalID, OccurrenceDate: changedDate, ExpectedVersion: 1, State: "final", Outcome: "held", Material: "Materi final draf", RequestKey: "a3333333-3333-4333-8333-333333333333"})
	if err != nil || final.Version != 2 {
		t.Fatalf("finalisasi draf terdampak gagal: result=%#v err=%v", final, err)
	}
	finalDetail, err := s.TeachingJournalDetail(ctx, teacher.ID, draft.ID)
	if err != nil || finalDetail.CurrentVersion.State != "final" || finalDetail.ReviewReason != "" || afterRevision.CurrentVersion.Periods[0].StartsAt != "07:00:00" {
		t.Fatalf("Final harus mempertahankan snapshot tanpa indikator Draf: detail=%#v err=%v", finalDetail, err)
	}
	finalFilter := academic.BuildTeachingJournalReadFilter(1, semester, class.ID, subjectID, changedDate.Format("2006-01-02"), changedDate.Format("2006-01-02"), "final", "")
	finalPage, err := s.TeachingJournals(ctx, teacher.ID, finalFilter)
	if err != nil || len(finalPage.Items) != 1 || finalPage.Items[0].State != "final" || finalPage.Items[0].ReviewReason != "" {
		t.Fatalf("nilai setelah finalisasi tetap diberi indikator Draf: page=%#v err=%v", finalPage, err)
	}
}
