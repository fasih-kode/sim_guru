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

func TestScheduleTemplateHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := &academic.Store{DB: db}
	curriculum, password := testutil.User(t, db, "kurikulum")
	admin, adminPassword := testutil.User(t, db, "admin")
	teacher, teacherPassword := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	year, err := s.CreateYear(ctx, curriculum.ID, "HTTP Template "+testutil.Random(t)[:10])
	must(err)
	var semester, teacherID, subjectID int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester))
	must(s.ActivateYear(ctx, curriculum.ID, year.ID))
	must(s.ActivateSemester(ctx, curriculum.ID, semester))
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Imam Pengujian',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$1) RETURNING id`, "Inggris "+testutil.Random(t)[:8]).Scan(&subjectID))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	must(s.AssignTeacher(ctx, curriculum.ID, semester, class.ID, subjectID, teacherID, date))
	must(s.SaveLessonPeriod(ctx, curriculum.ID, semester, 7, 15, "10:00", "10:40"))
	accounts, err := auth.New(db)
	must(err)
	key := make([]byte, 32)
	_, err = rand.Read(key)
	must(err)
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	must(err)
	server := httptest.NewServer(handler)
	defer server.Close()
	tokenPattern := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	read := func(resp *http.Response, code int) string {
		t.Helper()
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		must(e)
		if resp.StatusCode != code {
			t.Fatalf("HTTP %d want %d", resp.StatusCode, code)
		}
		return string(b)
	}
	get := func(c *http.Client, path string, code int) string {
		t.Helper()
		r, e := c.Get(server.URL + path)
		must(e)
		return read(r, code)
	}
	post := func(c *http.Client, path string, v url.Values, code int) string {
		t.Helper()
		r, e := c.PostForm(server.URL+path, v)
		must(e)
		return read(r, code)
	}
	token := func(body string) string {
		t.Helper()
		m := tokenPattern.FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatal("CSRF missing")
		}
		return m[1]
	}
	login := func(user auth.User, pw string) *http.Client {
		jar, e := cookiejar.New(nil)
		must(e)
		c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		post(c, "/login", url.Values{"username": {user.Username}, "password": {pw}, "gorilla.csrf.Token": {token(get(c, "/login", 200))}}, 303)
		return c
	}
	client := login(curriculum, password)
	ac := login(admin, adminPassword)
	teacherClient := login(teacher, teacherPassword)
	for _, path := range []string{"/akademik/struktur-kurikulum", "/akademik/waktu-sekolah", "/akademik/batasan-jadwal"} {
		get(client, fmt.Sprintf("%s?semester_id=%d", path, semester), 200)
		get(ac, fmt.Sprintf("%s?semester_id=%d", path, semester), 403)
		post(ac, fmt.Sprintf("%s?semester_id=%d", path, semester), url.Values{}, 403)
	}
	path := fmt.Sprintf("/akademik/jam-pelajaran?semester_id=%d", semester)
	body := get(client, path, 200)
	for _, day := range []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"} {
		if !strings.Contains(body, day) {
			t.Fatal("missing day", day)
		}
	}
	if !strings.Contains(body, `id="period-day" name="weekday" data-instant-filter`) || strings.Contains(body, "onchange=") {
		t.Fatal("day selection does not use CSP-compatible filter")
	}
	if !strings.Contains(body, `href="/akademik/jadwal"`) || !strings.Contains(body, "Jadwal Mingguan") {
		t.Fatal("Kurikulum schedule navigation missing")
	}
	tuesday := get(client, path+"&weekday=2", 200)
	for _, expected := range []string{`<option value="2" selected>Selasa</option>`, "Atur jam Selasa", "Jam pelajaran · Selasa", `name="weekday" value="2"`} {
		if !strings.Contains(tuesday, expected) {
			t.Fatal("selected day not reflected", expected)
		}
	}
	if strings.Count(body, `type="time"`) != 24 {
		t.Fatal("template not twelve slots")
	}
	get(ac, path, 403)
	post(client, path, url.Values{"action": {"day"}, "weekday": {"1"}, "starts_1": {"07:00"}, "ends_1": {"07:40"}}, 403)
	values := url.Values{"gorilla.csrf.Token": {token(body)}, "action": {"day"}, "weekday": {"1"}, "starts_1": {"07:00"}, "ends_1": {"07:40"}, "starts_2": {"07:40"}, "ends_2": {"08:20"}}
	post(ac, path, values, 403)
	post(client, path, values, 303)
	values.Set("starts_2", "07:20")
	invalid := post(client, path, values, 422)
	if !strings.Contains(invalid, `value="07:20"`) || !strings.Contains(invalid, "data-modal-open-on-load") {
		t.Fatal("validation lost inputs/modal")
	}
	var count int
	must(db.QueryRow(`SELECT count(*) FROM app.lesson_periods WHERE semester_id=$1`, semester).Scan(&count))
	if count != 3 {
		t.Fatal("failed save wrote rows")
	}
	legacy := get(client, path+"&weekday=7", 200)
	if !strings.Contains(legacy, `name="starts_15" value="10:00"`) {
		t.Fatal("legacy period missing")
	}
	schedule := fmt.Sprintf("/akademik/jadwal?semester_id=%d", semester)
	rev, err := s.CreateTimetableDraft(ctx, curriculum.ID, semester, date)
	must(err)
	_, _, _, err = s.TeachingOptions(ctx, curriculum.ID, semester)
	must(err)
	_, _, _, err = s.Timetable(ctx, curriculum.ID, semester)
	must(err)
	_, err = s.ScheduleAssignments(ctx, curriculum.ID, semester, date)
	must(err)
	_, err = s.PublishedTimetables(ctx, curriculum.ID, semester)
	must(err)
	body = get(client, schedule, 200)
	for _, filter := range []string{`data-table-column-filter="day"`, `data-table-column-filter="class"`, `data-table-column-filter="subject"`, `data-table-column-filter="teacher"`} {
		if !strings.Contains(body, filter) {
			t.Fatal("schedule table filter missing", filter)
		}
	}
	if !strings.Contains(body, "data-schedule-form") || strings.Contains(body, `name="teacher_id"`) {
		t.Fatal("teacher not derived")
	}
	entry := url.Values{"gorilla.csrf.Token": {token(body)}, "action": {"entry"}, "class_id": {fmt.Sprint(class.ID)}, "subject_id": {fmt.Sprint(subjectID)}, "teacher_id": {"999999999"}, "weekday": {"1"}, "period_start": {"1"}, "period_end": {"2"}}
	post(client, schedule, entry, 303)
	createdSchedule := get(client, schedule, 200)
	for _, attribute := range []string{
		fmt.Sprintf(`data-filter-day="1" data-filter-class="%d" data-filter-subject="%d" data-filter-teacher="%d"`, class.ID, subjectID, teacherID),
	} {
		if !strings.Contains(createdSchedule, attribute) {
			t.Fatal("schedule row filter values missing", attribute)
		}
	}
	var actual int64
	must(db.QueryRow(`SELECT teacher_id FROM app.weekly_timetable_entries WHERE revision_id=$1`, rev.ID).Scan(&actual))
	if actual != teacherID {
		t.Fatal("submitted teacher trusted")
	}
	post(client, schedule, entry, 422) // same class/teacher/time conflict
	entry.Set("subject_id", "999999999")
	post(client, schedule, entry, 422)
	post(client, schedule, url.Values{"gorilla.csrf.Token": {token(body)}, "action": {"publish"}}, 303)
	mySchedule := get(teacherClient, "/jadwal-saya", 200)
	for _, expected := range []string{
		`id="my-timetable-table"`,
		`data-table-column-filter="day"`,
		`data-table-column-filter="class"`,
		`data-table-column-filter="subject"`,
		fmt.Sprintf(`<option value="%d">7A</option>`, class.ID),
		fmt.Sprintf(`<option value="%d">Inggris`, subjectID),
		fmt.Sprintf(`data-filter-day="1" data-filter-class="%d" data-filter-subject="%d"`, class.ID, subjectID),
	} {
		if !strings.Contains(mySchedule, expected) {
			t.Fatal("teacher schedule filters missing expected markup", expected)
		}
	}
	if !strings.Contains(mySchedule, `aria-controls="my-timetable-filter-panel"`) || strings.Contains(mySchedule, `name="teacher_id"`) {
		t.Fatal("teacher schedule filter controls invalid or expose a teacher selector")
	}
	// Exercise the actual generator POST route on isolated test data.
	must(s.SaveCurriculumRule(ctx, curriculum.ID, academic.CurriculumRule{SemesterID: semester, Grade: 7, SubjectID: subjectID, WeeklyJP: 1, Meetings: []int{1}, DifferentDays: true}))
	must(s.SaveSchoolBlock(ctx, curriculum.ID, academic.SchoolTimeBlock{SemesterID: semester, Weekday: 1, Kind: "activity", Label: "Upacara", PeriodNo: 1, StartsAt: "07:00", EndsAt: "07:40"}))
	publishedBody := get(client, schedule, 200)
	generation := url.Values{"action": {"generate"}, "draft_id": {"0"}, "effective_from": {"2026-01-02"}, "gorilla.csrf.Token": {token(publishedBody)}}
	denied := url.Values{"action": {"generate"}, "draft_id": {"0"}, "effective_from": {"2026-01-02"}}
	post(client, schedule, denied, 403)
	post(ac, schedule, generation, 403)
	post(client, schedule, generation, 303)
	body = get(client, schedule, 200)
	for _, expected := range []string{"Upacara", "Target dan JP terjadwal", "Generate jadwal", "generate-timetable-modal", "Struktur Kurikulum"} {
		if !strings.Contains(body, expected) {
			t.Fatal("generated schedule UI missing", expected)
		}
	}
	generatedRevision, _, generatedEntries, err := s.Timetable(ctx, curriculum.ID, semester)
	must(err)
	for _, entry := range generatedEntries {
		if entry.Kind == "" {
			editBody := get(client, fmt.Sprintf("%s&edit_entry=%d", schedule, entry.ID), 200)
			if !strings.Contains(editBody, `data-modal-open-on-load`) || !strings.Contains(editBody, `value="correct-entry"`) {
				t.Fatal("draft edit modal missing")
			}
			editValues := url.Values{"action": {"correct-entry"}, "entry_id": {fmt.Sprint(entry.ID)}, "class_id": {fmt.Sprint(entry.ClassID)}, "subject_id": {fmt.Sprint(entry.SubjectID)}, "weekday": {fmt.Sprint(entry.Weekday)}, "period_start": {fmt.Sprint(entry.PeriodStart)}, "period_end": {fmt.Sprint(entry.PeriodEnd)}, "gorilla.csrf.Token": {token(editBody)}}
			post(client, schedule, editValues, 303)
			var persisted int64
			must(db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE id=$1 AND revision_id=$2`, entry.ID, generatedRevision.ID).Scan(&persisted))
			if persisted != entry.ID {
				t.Fatal("HTTP edit replaced entry")
			}
			break
		}
	}
	if strings.Contains(body, `value="1" data-weekday="1"`) {
		t.Fatal("ceremony period offered as teaching time")
	}
	// Exercise availability CRUD through authenticated HTTP, including CSRF and
	// stale draft invalidation. The preference includes all teaching times so its
	// unmet report is guaranteed to be visible, without reducing required JP.
	constraintsPath := fmt.Sprintf("/akademik/batasan-jadwal?semester_id=%d", semester)
	constraintsBody := get(client, constraintsPath, 200)
	constraintValues := url.Values{"action": {"save"}, "target_kind": {"teacher"}, "target_id": {fmt.Sprint(teacherID)}, "weekdays": {"1", "7"}, "all_day": {"true"}, "severity": {"soft"}, "gorilla.csrf.Token": {token(constraintsBody)}}
	noCSRF := url.Values{"action": {"save"}}
	post(client, constraintsPath, noCSRF, 403)
	post(client, constraintsPath, constraintValues, 303)
	constraintsBody = get(client, constraintsPath, 200)
	for _, expected := range []string{"Kondisional", "Senin, Minggu", "Sepanjang hari", "data-grid-target-form", "schedule-constraints-table"} {
		if !strings.Contains(constraintsBody, expected) {
			t.Fatal("constraint UI missing", expected)
		}
	}
	configured, e := s.ScheduleConstraints(ctx, curriculum.ID, semester)
	must(e)
	editConstraint := get(client, fmt.Sprintf("%s&edit=%d", constraintsPath, configured[0].ID), 200)
	if !strings.Contains(editConstraint, "data-modal-open-on-load") {
		t.Fatal("constraint edit modal missing")
	}
	constraintValues.Set("weekdays", "8")
	invalidConstraint := post(client, constraintsPath, constraintValues, 422)
	if !strings.Contains(invalidConstraint, "data-modal-open-on-load") {
		t.Fatal("invalid constraint lost modal")
	}
	post(client, schedule, url.Values{"action": {"publish"}, "gorilla.csrf.Token": {token(get(client, schedule, 200))}}, 422)
	generation.Set("draft_id", fmt.Sprint(generatedRevision.ID))
	generation.Set("replace_draft", "yes")
	post(client, schedule, generation, 303)
	body = get(client, schedule, 200)
	if !strings.Contains(body, "Preferensi yang belum terpenuhi") || !strings.Contains(body, "Imam Pengujian") {
		t.Fatal("preference report missing")
	}
	gridURL := fmt.Sprintf("%s&grid=1&target_kind=teacher&target_id=%d", constraintsPath, teacherID)
	gridBody := get(client, gridURL, 200)
	for _, expected := range []string{"data-constraint-grid", "grid-cell--blocked", "Kondisional", "Semua Tersedia", "name=\"reviewed\"", "cell_1_2"} {
		if !strings.Contains(gridBody, expected) {
			t.Fatal("grid UI missing", expected)
		}
	}
	plan, e := s.SchedulePlan(ctx, curriculum.ID, semester, time.Now())
	must(e)
	editor := academic.BuildConstraintEditor(plan, teacherID, 0, "Imam Pengujian")
	gridValues := url.Values{"action": {"grid"}, "target_kind": {"teacher"}, "target_id": {fmt.Sprint(teacherID)}, "grid_token": {editor.Token}, "reviewed": {"yes"}, "gorilla.csrf.Token": {token(gridBody)}}
	for _, day := range editor.Days {
		for _, cell := range day.Cells {
			if cell.Blocked == "" {
				gridValues.Set(cell.Field(), cell.State)
			}
		}
	}
	gridValues.Set("cell_1_1", "available")
	invalidGrid := post(client, constraintsPath, gridValues, 422)
	if !strings.Contains(invalidGrid, "data-modal-open-on-load") {
		t.Fatal("invalid grid lost modal")
	}
	gridValues.Del("cell_1_1")
	post(ac, constraintsPath, gridValues, 403)
	post(client, constraintsPath, gridValues, 303)
	post(client, constraintsPath, gridValues, 409)
	gridEditorBody := get(client, gridURL, 200)
	constraintsBody = get(client, constraintsPath, 200)
	if !strings.Contains(constraintsBody, "Kisi tersimpan") {
		t.Fatal("saved target grid missing")
	}
	if !strings.Contains(constraintsBody, `name="action" value="remove-grid"`) || !strings.Contains(constraintsBody, `Hapus kisi`) {
		t.Fatal("saved grid delete action missing")
	}
	var legacyCount int
	must(db.QueryRow(`SELECT count(*) FROM app.schedule_constraints WHERE semester_id=$1`, semester).Scan(&legacyCount))
	if legacyCount != 1 {
		t.Fatal("grid conversion deleted legacy data")
	}
	var gridID, gridVersion int64
	must(db.QueryRow(`SELECT id,version FROM app.schedule_constraint_grids WHERE semester_id=$1 AND teacher_id=$2 AND deleted_at IS NULL`, semester, teacherID).Scan(&gridID, &gridVersion))
	removeGrid := url.Values{"gorilla.csrf.Token": {token(constraintsBody)}, "action": {"remove-grid"}, "grid_id": {fmt.Sprint(gridID)}, "grid_version": {fmt.Sprint(gridVersion)}}
	post(ac, constraintsPath, removeGrid, 403)
	post(client, constraintsPath, removeGrid, 303)
	var activeGridCount, remainingLegacyCount int
	must(db.QueryRow(`SELECT count(*) FROM app.schedule_constraint_grids WHERE semester_id=$1 AND teacher_id=$2 AND deleted_at IS NULL`, semester, teacherID).Scan(&activeGridCount))
	must(db.QueryRow(`SELECT count(*) FROM app.schedule_constraints WHERE semester_id=$1 AND teacher_id=$2`, semester, teacherID).Scan(&remainingLegacyCount))
	if activeGridCount != 0 || remainingLegacyCount != 0 {
		t.Fatal("deleting grid left effective constraints or an active grid")
	}
	// Optional sanitized SSR fixtures for desktop/mobile visual inspection.
	if dir := os.Getenv("SIM_GURU_VISUAL_DIR"); dir != "" {
		must(os.MkdirAll(dir, 0700))
		fixtures := map[string]string{"periods.html": get(client, path, 200), "periods-tuesday.html": tuesday, "schedule.html": body, "legacy.html": legacy, "structure.html": get(client, fmt.Sprintf("/akademik/struktur-kurikulum?semester_id=%d", semester), 200), "constraints.html": constraintsBody, "constraint-edit.html": gridEditorBody, "grid-conversion.html": gridBody, "blocks.html": get(client, fmt.Sprintf("/akademik/waktu-sekolah?semester_id=%d", semester), 200)}
		for name, html := range fixtures {
			html = tokenPattern.ReplaceAllString(html, `name="gorilla.csrf.Token" value="visual-fixture"`)
			must(os.WriteFile(filepath.Join(dir, name), []byte(html), 0600))
		}
	}
}
