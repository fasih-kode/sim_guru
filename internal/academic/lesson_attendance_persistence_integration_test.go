//go:build integration

package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

type lessonAttendanceFixture struct {
	db                                                   *sql.DB
	store                                                *Store
	ctx                                                  context.Context
	admin, curriculum, teacher, otherTeacher             authUser
	semester, classID, classBID, subjectID, otherEntryID int64
	date, today                                          string
	revisionID, entryID                                  int64
	students                                             []int64
}

// alias lokal menjaga fixture tetap ringkas tanpa menjadikan identitas user input service.
type authUser struct{ ID int64 }

func newLessonAttendanceFixture(t *testing.T) lessonAttendanceFixture {
	t.Helper()
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel", "admin")
	other, _ := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().In(attendanceWIB)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, attendanceWIB)
	date := today.AddDate(0, 0, -3)
	dateText := date.Format("2006-01-02")
	year, err := s.CreateYear(ctx, cur.ID, "Lesson attendance "+testutil.Random(t)[:10])
	must(err)
	must(s.ActivateYear(ctx, cur.ID, year.ID))
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	classB, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "B", "", 0)
	must(err)
	var semester int64
	if err = db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester); err != nil {
		t.Fatal(err)
	}
	calStart := today.AddDate(0, 0, -45).Format("2006-01-02")
	calEnd := today.AddDate(0, 0, 45).Format("2006-01-02")
	must(s.SaveSemesterCalendar(ctx, cur.ID, SemesterCalendar{SemesterID: semester, StartsOn: calStart, EndsOn: calEnd, SchoolDays: []int{1, 2, 3, 4, 5, 6, 7}}, "fixture kalender"))
	var teacherID, otherTeacherID int64
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru absensi per pelajaran',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru mapel lain',$1) RETURNING id`, other.ID).Scan(&otherTeacherID))
	var subject, otherSubject int64
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel absensi') RETURNING id`, "LA"+testutil.Random(t)[:10]).Scan(&subject))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel lain') RETURNING id`, "LB"+testutil.Random(t)[:10]).Scan(&otherSubject))
	start, _ := time.Parse("2006-01-02", today.AddDate(0, 0, -45).Format("2006-01-02"))
	must(s.AssignTeacher(ctx, cur.ID, semester, class.ID, subject, teacherID, start))
	must(s.AssignTeacher(ctx, cur.ID, semester, class.ID, otherSubject, otherTeacherID, start))
	students := make([]int64, 0, 8)
	for i := 0; i < 8; i++ {
		var id int64
		must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, "Siswa pelajaran "+testutil.Random(t)[:8]).Scan(&id))
		_, err = s.CreatePlacement(ctx, admin.ID, id, class.ID, calStart)
		must(err)
		students = append(students, id)
	}
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	for day := 1; day <= 7; day++ {
		must(s.SaveLessonPeriod(ctx, cur.ID, semester, day, 1, "00:01", "00:40"))
		must(s.SaveLessonPeriod(ctx, cur.ID, semester, day, 2, "00:40", "01:20"))
		must(s.SaveLessonPeriod(ctx, cur.ID, semester, day, 3, "01:30", "02:10"))
		must(s.SaveLessonPeriod(ctx, cur.ID, semester, day, 4, "02:10", "02:50"))
	}
	buildRevision := func(effective string) TimetableRevision {
		t.Helper()
		d, e := time.Parse("2006-01-02", effective)
		must(e)
		rev, e := s.CreateTimetableDraft(ctx, cur.ID, semester, d)
		must(e)
		for day := 1; day <= 7; day++ {
			must(s.AddTimetableEntry(ctx, cur.ID, rev.ID, day, class.ID, subject, teacherID, 1, 3))
		}
		must(s.AddTimetableEntry(ctx, cur.ID, rev.ID, weekday, class.ID, otherSubject, otherTeacherID, 4, 4))
		return rev
	}
	initialDate := today.AddDate(0, 0, -30).Format("2006-01-02")
	initial := buildRevision(initialDate)
	must(s.PublishTimetable(ctx, cur.ID, initial.ID))
	var entryID, otherEntryID int64
	must(db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND weekday=$2 AND subject_id=$3`, initial.ID, weekday, subject).Scan(&entryID))
	must(db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND weekday=$2 AND subject_id=$3`, initial.ID, weekday, otherSubject).Scan(&otherEntryID))
	// Kejadian siswa dicatat sebelum tanggal rapat uji agar roster pada tiap tanggal dapat dibuktikan.
	must(s.MoveStudentClass(ctx, cur.ID, placementForStudent(t, db, students[5]), class.ID, classB.ID, date.AddDate(0, 0, 1).Format("2006-01-02"), "Uji pindah kelas"))
	must(s.ExitStudentSchool(ctx, admin.ID, students[6], studentVersion(t, db, students[6]), date.AddDate(0, 0, 1).Format("2006-01-02"), "Uji keluar sekolah", ""))
	must(s.ExitStudentSchool(ctx, admin.ID, students[7], studentVersion(t, db, students[7]), date.AddDate(0, 0, -3).Format("2006-01-02"), "Uji keluar lalu kembali", ""))
	movement, err := s.MovementStudent(ctx, admin.ID, students[7], 0)
	must(err)
	must(s.ReadmitStudent(ctx, admin.ID, students[7], movement.Version, movement.ExitID, class.ID, date.AddDate(0, 0, -1).Format("2006-01-02"), "Uji diterima kembali", ""))
	return lessonAttendanceFixture{db: db, store: s, ctx: ctx, admin: authUser{admin.ID}, curriculum: authUser{cur.ID}, teacher: authUser{teacher.ID}, otherTeacher: authUser{other.ID}, semester: semester, classID: class.ID, classBID: classB.ID, subjectID: subject, otherEntryID: otherEntryID, date: dateText, today: today.Format("2006-01-02"), revisionID: initial.ID, entryID: entryID, students: students}
}

func (f lessonAttendanceFixture) formFor(t *testing.T, actor int64, date string, entryID int64) TeacherLessonAttendanceContext {
	t.Helper()
	context, err := f.store.TeacherLessonAttendanceOnDate(f.ctx, actor, date, entryID)
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func placementForStudent(t *testing.T, db *sql.DB, student int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM app.student_placements WHERE student_id=$1`, student).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func studentVersion(t *testing.T, db *sql.DB, student int64) int64 {
	t.Helper()
	var version int64
	if err := db.QueryRow(`SELECT version FROM app.students WHERE id=$1`, student).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func (f lessonAttendanceFixture) input(date string, entryID int64, fingerprint string, choices ...LessonAttendanceChoice) LessonAttendanceInput {
	return LessonAttendanceInput{Date: date, EntryID: entryID, Fingerprint: fingerprint, Students: choices}
}

func TestSaveLessonAttendanceIntegration(t *testing.T) {
	f := newLessonAttendanceFixture(t)
	teacherID := f.teacher.ID
	form := f.formFor(t, teacherID, f.date, f.entryID)
	if form.Fingerprint == "" || len(form.Students) != len(f.students) {
		t.Fatal("form context or roster fingerprint missing", form)
	}
	var baseHasMoved, baseHasReturned bool
	for _, student := range form.Students {
		baseHasMoved = baseHasMoved || student.ID == f.students[5]
		baseHasReturned = baseHasReturned || student.ID == f.students[7]
	}
	if !baseHasMoved || !baseHasReturned {
		t.Fatal("historical roster did not include moved or returned students", form.Students)
	}
	baseDate, _ := time.Parse("2006-01-02", f.date)
	postMoveDate := baseDate.AddDate(0, 0, 1).Format("2006-01-02")
	postWeekday := int(baseDate.AddDate(0, 0, 1).Weekday())
	if postWeekday == 0 {
		postWeekday = 7
	}
	var postMoveEntry int64
	if err := f.db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND weekday=$2 AND subject_id=$3`, f.revisionID, postWeekday, f.subjectID).Scan(&postMoveEntry); err != nil {
		t.Fatal(err)
	}
	postMoveForm := f.formFor(t, teacherID, postMoveDate, postMoveEntry)
	var postHasMoved, postHasExited, postHasReturned bool
	for _, student := range postMoveForm.Students {
		postHasMoved = postHasMoved || student.ID == f.students[5]
		postHasExited = postHasExited || student.ID == f.students[6]
		postHasReturned = postHasReturned || student.ID == f.students[7]
	}
	if postHasMoved || postHasExited || !postHasReturned {
		t.Fatal("dated move/exit/readmission roster incorrect", postMoveForm.Students)
	}
	if _, err := f.store.SaveLessonAttendance(f.ctx, teacherID, f.input(postMoveDate, postMoveEntry, postMoveForm.Fingerprint, LessonAttendanceChoice{StudentID: f.students[5], Status: "hadir"})); err == nil {
		t.Fatal("student moved out of class remained eligible")
	}
	if _, err := f.store.SaveLessonAttendance(f.ctx, teacherID, f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[0]})); err == nil {
		t.Fatal("blank status batch accepted")
	}
	var count int
	mustQuery := func(query string, args ...any) {
		t.Helper()
		if err := f.db.QueryRow(query, args...).Scan(&count); err != nil {
			t.Fatal(err)
		}
	}
	mustQuery(`SELECT count(*) FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2`, f.entryID, f.date)
	if count != 0 {
		t.Fatal("blank status created a row", count)
	}

	input := f.input(f.date, f.entryID, form.Fingerprint,
		LessonAttendanceChoice{StudentID: f.students[0], Status: "hadir"},
		LessonAttendanceChoice{StudentID: f.students[1]},
	)
	saved, err := f.store.SaveLessonAttendance(f.ctx, teacherID, input)
	if err != nil || saved.Inserted != 1 || saved.Idempotent {
		t.Fatal("partial save", saved, err)
	}
	mustQuery(`SELECT count(*) FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2`, f.entryID, f.date)
	if count != 1 {
		t.Fatal("partial save wrote wrong row count", count)
	}
	mustQuery(`SELECT count(*) FROM app.student_lesson_attendance_history h JOIN app.student_lesson_attendance a ON a.id=h.attendance_id WHERE a.timetable_entry_id=$1 AND a.meeting_date=$2 AND h.action='recorded' AND h.before_state IS NULL AND h.reason IS NULL`, f.entryID, f.date)
	if count != 1 {
		t.Fatal("initial history missing", count)
	}

	retry, err := f.store.SaveLessonAttendance(f.ctx, teacherID, input)
	if err != nil || !retry.Idempotent || retry.Inserted != 0 {
		t.Fatal("identical retry was not idempotent", retry, err)
	}
	mustQuery(`SELECT count(*) FROM app.student_lesson_attendance_history h JOIN app.student_lesson_attendance a ON a.id=h.attendance_id WHERE a.timetable_entry_id=$1 AND a.meeting_date=$2`, f.entryID, f.date)
	if count != 1 {
		t.Fatal("retry added history", count)
	}
	different := f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[0], Status: "izin"})
	if _, err = f.store.SaveLessonAttendance(f.ctx, teacherID, different); !errors.Is(err, ErrConflict) {
		t.Fatal("different retry did not conflict", err)
	}

	if _, err = f.store.SaveLessonAttendance(f.ctx, f.admin.ID, input); !errors.Is(err, ErrForbidden) {
		t.Fatal("Admin role bypassed Guru Mapel", err)
	}
	if _, err = f.store.SaveLessonAttendance(f.ctx, teacherID, f.input(f.date, f.otherEntryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[0], Status: "hadir"})); !errors.Is(err, ErrNotFound) {
		t.Fatal("combined admin role widened lesson access", err)
	}
	if _, err = f.store.SaveLessonAttendance(f.ctx, teacherID, f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: 999999, Status: "hadir"})); err == nil {
		t.Fatal("student outside roster accepted")
	}
	if _, err = f.store.SaveLessonAttendance(f.ctx, f.otherTeacher.ID, f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[2], Status: "hadir"})); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrForbidden) {
		t.Fatal("different teacher opened entry", err)
	}

	// Satu kegagalan histori harus membatalkan seluruh batch, termasuk baris yang sempat diinsert.
	suffix := testutil.Random(t)[:10]
	functionName := "fail_lesson_history_" + suffix
	triggerName := "fail_lesson_history_trigger_" + suffix
	if _, err = f.db.Exec(fmt.Sprintf(`CREATE FUNCTION app.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture failure'; END $$; CREATE TRIGGER %s BEFORE INSERT ON app.student_lesson_attendance_history FOR EACH ROW EXECUTE FUNCTION app.%s()`, functionName, triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.student_lesson_attendance_history; DROP FUNCTION IF EXISTS app.%s()`, triggerName, functionName))
	})
	rollbackInput := f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[2], Status: "sakit"}, LessonAttendanceChoice{StudentID: f.students[3], Status: "alpa"})
	if _, err = f.store.SaveLessonAttendance(f.ctx, teacherID, rollbackInput); err == nil {
		t.Fatal("history failure did not fail batch")
	}
	_, _ = f.db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.student_lesson_attendance_history; DROP FUNCTION IF EXISTS app.%s()`, triggerName, functionName))
	mustQuery(`SELECT count(*) FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id IN($3,$4)`, f.entryID, f.date, f.students[2], f.students[3])
	if count != 0 {
		t.Fatal("failed batch left partial rows", count)
	}

	// Dua request identik berjalan bersamaan; mutation lock dan unique constraint membatasi satu bukti.
	concurrentInput := f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[4], Status: "hadir"})
	results := make(chan LessonAttendanceSaveResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.store.SaveLessonAttendance(f.ctx, teacherID, concurrentInput)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	totalInserted := 0
	idempotent := 0
	for r := range results {
		totalInserted += r.Inserted
		if r.Idempotent {
			idempotent++
		}
	}
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent identical submit failed", e)
		}
	}
	if totalInserted != 1 || idempotent != 1 {
		t.Fatal("concurrent retry duplicated", totalInserted, idempotent)
	}
	mustQuery(`SELECT count(*) FROM app.student_lesson_attendance_history h JOIN app.student_lesson_attendance a ON a.id=h.attendance_id WHERE a.timetable_entry_id=$1 AND a.meeting_date=$2 AND a.student_id=$3`, f.entryID, f.date, f.students[4])
	if count != 1 {
		t.Fatal("concurrent retry added duplicate history", count)
	}
	// Status berbeda yang masuk bersamaan tidak menimpa hasil pertama.
	differentInput := f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[1], Status: "hadir"})
	otherInput := f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[1], Status: "izin"})
	concurrentErrors := make(chan error, 2)
	var differentWG sync.WaitGroup
	for _, candidate := range []LessonAttendanceInput{differentInput, otherInput} {
		differentWG.Add(1)
		go func(input LessonAttendanceInput) {
			defer differentWG.Done()
			_, saveErr := f.store.SaveLessonAttendance(f.ctx, teacherID, input)
			concurrentErrors <- saveErr
		}(candidate)
	}
	differentWG.Wait()
	close(concurrentErrors)
	accepted, conflicted := 0, 0
	for saveErr := range concurrentErrors {
		if saveErr == nil {
			accepted++
		} else if errors.Is(saveErr, ErrConflict) {
			conflicted++
		} else {
			t.Fatal("unexpected concurrent status result", saveErr)
		}
	}
	if accepted != 1 || conflicted != 1 {
		t.Fatal("different concurrent statuses were not serialized", accepted, conflicted)
	}

	// Isian dari form lama ditolak setelah roster tanggal kejadian berubah.
	if err = f.store.MoveStudentClass(f.ctx, f.curriculum.ID, placementForStudent(t, f.db, f.students[1]), f.classID, f.classBID, f.date, "Uji perubahan roster setelah form dibuka"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveLessonAttendance(f.ctx, teacherID, f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[1], Status: "sakit"})); !errors.Is(err, ErrConflict) {
		t.Fatal("stale roster fingerprint accepted", err)
	}

	// Libur memakai tanggal yang sama tetapi tanpa pertemuan yang dapat disimpan.
	cal, err := f.store.SemesterCalendar(f.ctx, f.curriculum.ID, f.semester)
	if err != nil {
		t.Fatal(err)
	}
	holiday := time.Now().In(attendanceWIB).AddDate(0, 0, -4).Format("2006-01-02")
	if err = f.store.SaveSchoolDateException(f.ctx, f.curriculum.ID, f.semester, cal.Version, SchoolDateException{Date: holiday, SchoolDay: false, Label: "Hari libur uji"}, false, "fixture libur"); err != nil {
		t.Fatal(err)
	}
	holidayLessons, err := f.store.TeacherLessonsOnDate(f.ctx, teacherID, holiday)
	if err != nil || len(holidayLessons.Lessons) != 0 {
		t.Fatal("holiday lesson listed", holidayLessons, err)
	}

	// Waktu mulai dan segmen jeda diambil dari snapshot revisi yang tersimpan.
	var revisionID int64
	var startsAt, segments string
	var recordedAt time.Time
	var snapshotBefore []byte
	if err = f.db.QueryRow(`SELECT revision_id,starts_at::text,time_segments,recorded_at,snapshot FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id=$3`, f.entryID, f.date, f.students[0]).Scan(&revisionID, &startsAt, &segments, &recordedAt, &snapshotBefore); err != nil {
		t.Fatal(err)
	}
	meetingDate, err := time.ParseInLocation("2006-01-02", f.date, attendanceWIB)
	if err != nil {
		t.Fatal(err)
	}
	meetingStart := time.Date(meetingDate.Year(), meetingDate.Month(), meetingDate.Day(), 0, 1, 0, 0, attendanceWIB)
	if revisionID != f.revisionID || startsAt != "00:01:00" || segments != "00:01–01:20 · jeda · 01:30–02:10" || recordedAt.IsZero() || recordedAt.Before(meetingStart) {
		t.Fatal("lesson snapshot incomplete", revisionID, startsAt, segments, recordedAt)
	}
	var savedSnapshot map[string]any
	if err = json.Unmarshal(snapshotBefore, &savedSnapshot); err != nil {
		t.Fatal("stored snapshot is invalid JSON", err)
	}
	encodedSnapshot := string(snapshotBefore)
	if strings.Contains(encodedSnapshot, "plan_snapshot") || strings.Contains(encodedSnapshot, "exceptions") || len(snapshotBefore) > 1800 {
		t.Fatal("stored snapshot contains excessive shared context", len(snapshotBefore), encodedSnapshot)
	}
	meetingContext, ok := savedSnapshot["meeting"].(map[string]any)
	if !ok {
		t.Fatal("stored meeting context is missing", savedSnapshot)
	}
	periods, ok := meetingContext["periods"].([]any)
	if !ok || len(periods) != 3 {
		t.Fatal("snapshot should retain only periods for this lesson", savedSnapshot)
	}
	var afterState struct {
		Status      string          `json:"status"`
		MeetingDate string          `json:"meeting_date"`
		Snapshot    json.RawMessage `json:"snapshot"`
	}
	if err = f.db.QueryRow(`SELECT h.after_state FROM app.student_lesson_attendance_history h JOIN app.student_lesson_attendance a ON a.id=h.attendance_id WHERE a.timetable_entry_id=$1 AND a.meeting_date=$2 AND a.student_id=$3 AND h.action='recorded'`, f.entryID, f.date, f.students[0]).Scan(&snapshotBefore); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(snapshotBefore, &afterState); err != nil {
		t.Fatal("initial history is invalid JSON", err)
	}
	var historySnapshot map[string]any
	if err = json.Unmarshal(afterState.Snapshot, &historySnapshot); err != nil {
		t.Fatal("initial history snapshot is invalid JSON", err)
	}
	if afterState.Status != "hadir" || afterState.MeetingDate != f.date || !reflect.DeepEqual(savedSnapshot, historySnapshot) {
		t.Fatal("stored attendance and initial history do not match", afterState)
	}
	// Revisi yang berlaku pada/tanggal sebelum bukti harus ditolak.
	backdated := f.buildDraftForTest(t, f.date)
	if err = f.store.PublishTimetable(f.ctx, f.curriculum.ID, backdated.ID); err == nil {
		t.Fatal("retroactive publication over saved attendance accepted")
	}
	oldContext, err := f.store.TeacherLessonAttendanceOnDate(f.ctx, teacherID, f.date, f.entryID)
	if err != nil || oldContext.Lesson.ID != f.entryID {
		t.Fatal("stored historical revision is no longer available", oldContext, err)
	}
	var status string
	if err = f.db.QueryRow(`SELECT status FROM app.weekly_timetable_revisions WHERE id=$1`, backdated.ID).Scan(&status); err != nil || status != "draft" {
		t.Fatal("blocked revision state changed", status, err)
	}
	if _, err = f.store.SaveLessonAttendance(f.ctx, teacherID, f.input(f.date, backdatedEntryID(t, f.db, backdated.ID, f.date), oldContext.Fingerprint, LessonAttendanceChoice{StudentID: f.students[5], Status: "izin"})); !errors.Is(err, ErrNotFound) {
		t.Fatal("draft entry accepted for attendance", err)
	}

	dailyCount := 0
	for _, studentID := range f.students {
		var n int
		if err = f.db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE student_id=$1`, studentID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		dailyCount += n
	}
	if dailyCount != 0 {
		t.Fatal("lesson attendance changed daily attendance", dailyCount)
	}
}

func TestLessonAttendanceFingerprintTracksContextChangesIntegration(t *testing.T) {
	f := newLessonAttendanceFixture(t)
	teacherID := f.teacher.ID
	initial := f.formFor(t, teacherID, f.date, f.entryID).Fingerprint
	if initial == "" {
		t.Fatal("initial fingerprint missing")
	}

	var assignmentID int64
	if err := f.db.QueryRow(`SELECT assignment_id FROM app.weekly_timetable_entries WHERE id=$1`, f.entryID).Scan(&assignmentID); err != nil {
		t.Fatal(err)
	}
	meetingDate, err := time.Parse("2006-01-02", f.date)
	if err != nil {
		t.Fatal(err)
	}
	assignmentUntil := meetingDate.AddDate(0, 0, 10).Format("2006-01-02")
	if _, err = f.db.Exec(`UPDATE app.teaching_assignments SET effective_until=$1::date WHERE id=$2`, assignmentUntil, assignmentID); err != nil {
		t.Fatal(err)
	}
	afterAssignment := f.formFor(t, teacherID, f.date, f.entryID).Fingerprint
	if afterAssignment == initial {
		t.Fatal("assignment change did not invalidate fingerprint")
	}

	if _, err = f.db.Exec(`UPDATE app.classes SET version=version+1 WHERE id=$1`, f.classID); err != nil {
		t.Fatal(err)
	}
	afterClass := f.formFor(t, teacherID, f.date, f.entryID).Fingerprint
	if afterClass == afterAssignment {
		t.Fatal("class version change did not invalidate fingerprint")
	}

	calendar, err := f.store.SemesterCalendar(f.ctx, f.curriculum.ID, f.semester)
	if err != nil {
		t.Fatal(err)
	}
	dateAfterMeeting := meetingDate.AddDate(0, 0, 1).Format("2006-01-02")
	if err = f.store.SaveSchoolDateException(f.ctx, f.curriculum.ID, f.semester, calendar.Version, SchoolDateException{Date: dateAfterMeeting, SchoolDay: false, Label: "Perubahan setelah pertemuan"}, false, "Uji perubahan konteks fingerprint"); err != nil {
		t.Fatal(err)
	}
	afterCalendar := f.formFor(t, teacherID, f.date, f.entryID).Fingerprint
	if afterCalendar == afterClass {
		t.Fatal("calendar change did not invalidate fingerprint")
	}

	effective := meetingDate.AddDate(0, 0, -1).Format("2006-01-02")
	draft := f.buildDraftForTest(t, effective)
	if err = f.store.PublishTimetable(f.ctx, f.curriculum.ID, draft.ID); err != nil {
		t.Fatal("changed published schedule should be selectable before attendance exists", err)
	}
	weekday := int(meetingDate.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	var entryID int64
	if err = f.db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND weekday=$2`, draft.ID, weekday).Scan(&entryID); err != nil {
		t.Fatal(err)
	}
	afterSchedule := f.formFor(t, teacherID, f.date, entryID).Fingerprint
	if afterSchedule == afterCalendar {
		t.Fatal("published schedule change did not invalidate fingerprint")
	}
}

func TestLessonAttendanceAndRetroactivePublishShareMutationLockIntegration(t *testing.T) {
	f := newLessonAttendanceFixture(t)
	teacherID := f.teacher.ID
	form := f.formFor(t, teacherID, f.date, f.entryID)
	d, err := time.Parse("2006-01-02", f.date)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := f.store.CreateTimetableDraft(f.ctx, f.curriculum.ID, f.semester, d)
	if err != nil {
		t.Fatal(err)
	}
	for day := 1; day <= 7; day++ {
		if err = f.store.AddTimetableEntry(f.ctx, f.curriculum.ID, draft.ID, day, f.classID, f.subjectID, mustTeacherID(t, f.db, teacherID), 1, 3); err != nil {
			t.Fatal(err)
		}
	}
	input := f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[0], Status: "hadir"})
	type result struct {
		err  error
		save LessonAttendanceSaveResult
	}
	saveDone := make(chan result, 1)
	publishDone := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		saved, saveErr := f.store.SaveLessonAttendance(f.ctx, teacherID, input)
		saveDone <- result{err: saveErr, save: saved}
	}()
	go func() { defer wg.Done(); publishDone <- f.store.PublishTimetable(f.ctx, f.curriculum.ID, draft.ID) }()
	wg.Wait()
	saveResult := <-saveDone
	publishErr := <-publishDone
	var revisionStatus string
	if err = f.db.QueryRow(`SELECT status FROM app.weekly_timetable_revisions WHERE id=$1`, draft.ID).Scan(&revisionStatus); err != nil {
		t.Fatal(err)
	}
	var attendanceCount int
	if err = f.db.QueryRow(`SELECT count(*) FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id=$3`, f.entryID, f.date, f.students[0]).Scan(&attendanceCount); err != nil {
		t.Fatal(err)
	}
	if revisionStatus == "published" {
		if publishErr != nil || saveResult.err == nil || (!errors.Is(saveResult.err, ErrConflict) && !errors.Is(saveResult.err, ErrNotFound)) || attendanceCount != 0 {
			t.Fatal("publication won but stale attendance was accepted", revisionStatus, saveResult, publishErr, attendanceCount)
		}
	} else if revisionStatus == "draft" {
		if saveResult.err != nil || saveResult.save.Inserted != 1 || publishErr == nil || attendanceCount != 1 {
			t.Fatal("attendance won but retroactive publication was accepted", revisionStatus, saveResult, publishErr, attendanceCount)
		}
	} else {
		t.Fatal("unexpected revision status", revisionStatus)
	}
}

func TestPublishTimetableAfterSavedLessonAttendanceIntegration(t *testing.T) {
	f := newLessonAttendanceFixture(t)
	teacherID := f.teacher.ID
	form := f.formFor(t, teacherID, f.date, f.entryID)
	input := f.input(f.date, f.entryID, form.Fingerprint, LessonAttendanceChoice{StudentID: f.students[0], Status: "hadir"})
	if _, err := f.store.SaveLessonAttendance(f.ctx, teacherID, input); err != nil {
		t.Fatal(err)
	}
	var before []byte
	if err := f.db.QueryRow(`SELECT snapshot FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id=$3`, f.entryID, f.date, f.students[0]).Scan(&before); err != nil {
		t.Fatal(err)
	}
	later := f.buildDraftForTest(t, f.today)
	if err := f.store.PublishTimetable(f.ctx, f.curriculum.ID, later.ID); err != nil {
		t.Fatal("revision after last stored meeting should be allowed", err)
	}
	var after []byte
	if err := f.db.QueryRow(`SELECT snapshot FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id=$3`, f.entryID, f.date, f.students[0]).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("later publication changed historical evidence")
	}
	var revisionID int64
	if err := f.db.QueryRow(`SELECT revision_id FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id=$3`, f.entryID, f.date, f.students[0]).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if revisionID != f.revisionID {
		t.Fatal("historical attendance revision was reassigned", revisionID, f.revisionID)
	}
	old, err := f.store.TeacherLessonAttendanceOnDate(f.ctx, teacherID, f.date, f.entryID)
	if err != nil || old.Lesson.ID != f.entryID {
		t.Fatal("historical published revision no longer selected", old, err)
	}
}

func (f lessonAttendanceFixture) buildDraftForTest(t *testing.T, effective string) TimetableRevision {
	t.Helper()
	d, err := time.Parse("2006-01-02", effective)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := f.store.CreateTimetableDraft(f.ctx, f.curriculum.ID, f.semester, d)
	if err != nil {
		t.Fatal(err)
	}
	for day := 1; day <= 7; day++ {
		if err = f.store.AddTimetableEntry(f.ctx, f.curriculum.ID, rev.ID, day, f.classID, f.subjectID, mustTeacherID(t, f.db, f.teacher.ID), 1, 3); err != nil {
			t.Fatal(err)
		}
	}
	return rev
}

func mustTeacherID(t *testing.T, db *sql.DB, userID int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM app.teachers WHERE user_id=$1`, userID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func backdatedEntryID(t *testing.T, db *sql.DB, revisionID int64, date string) int64 {
	t.Helper()
	d, _ := time.Parse("2006-01-02", date)
	day := int(d.Weekday())
	if day == 0 {
		day = 7
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND weekday=$2 ORDER BY id LIMIT 1`, revisionID, day).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
