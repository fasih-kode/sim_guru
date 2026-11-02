//go:build integration

package academic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAttendanceFoundationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	officer, _ := testutil.User(t, db, "petugas_absensi")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	wali, _ := testutil.User(t, db, "wali_kelas")
	stranger, _ := testutil.User(t, db, "wali_kelas")
	kes, _ := testutil.User(t, db, "kesiswaan")
	head, _ := testutil.User(t, db, "kepala_sekolah")
	bk, _ := testutil.User(t, db, "bk")
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	y, e := s.CreateYear(ctx, cur.ID, "Dates "+testutil.Random(t)[:10])
	must(e)
	must(s.ActivateYear(ctx, cur.ID, y.ID))
	a, e := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	must(e)
	b, e := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "B", "", 0)
	must(e)
	var sem, sem2 int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, y.ID).Scan(&sem))
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=2`, y.ID).Scan(&sem2))
	cal := SemesterCalendar{SemesterID: sem, StartsOn: "2020-07-01", EndsOn: "2020-12-31", SchoolDays: []int{1, 2, 3, 4, 5}}
	if e = s.SaveSemesterCalendar(ctx, admin.ID, cal, "test"); !errors.Is(e, ErrForbidden) {
		t.Fatal("Admin calendar access", e)
	}
	must(s.SaveSemesterCalendar(ctx, cur.ID, cal, "Pengaturan awal"))
	cal.Version = 1
	if e = s.SaveSemesterCalendar(ctx, cur.ID, SemesterCalendar{SemesterID: sem2, StartsOn: "2020-12-01", EndsOn: "2021-05-31", SchoolDays: []int{1}}, "overlap"); e == nil {
		t.Fatal("overlap accepted")
	}
	if e = s.SaveSemesterCalendar(ctx, cur.ID, SemesterCalendar{SemesterID: sem, Version: 0, StartsOn: cal.StartsOn, EndsOn: cal.EndsOn, SchoolDays: []int{1}}, "stale"); !errors.Is(e, ErrConflict) {
		t.Fatal("stale calendar accepted", e)
	}
	must(s.SaveSchoolDateException(ctx, cur.ID, sem, 1, SchoolDateException{Date: "2020-07-06", SchoolDay: false, Label: "Libur sekolah"}, false, "Libur"))
	must(s.SaveSchoolDateException(ctx, cur.ID, sem, 2, SchoolDateException{Date: "2020-07-11", SchoolDay: true, Label: "Sabtu khusus"}, false, "Kegiatan sekolah"))
	if e = s.SaveSchoolDateException(ctx, cur.ID, sem, 2, SchoolDateException{Date: "2020-07-11"}, true, "reset stale"); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	must(s.SaveSchoolDateException(ctx, cur.ID, sem, 3, SchoolDateException{Date: "2020-07-11"}, true, "Kembali mingguan"))
	must(s.SaveSchoolDateException(ctx, cur.ID, sem, 4, SchoolDateException{Date: "2020-07-21", SchoolDay: false, Label: "Libur khusus"}, false, "Hari libur"))
	cal, e = s.SemesterCalendar(ctx, cur.ID, sem)
	must(e)
	if len(cal.Exceptions) != 2 || cal.Version != 5 {
		t.Fatal(cal)
	}
	if e = s.SaveSchoolDateException(ctx, cur.ID, sem, 4, SchoolDateException{Date: "2021-01-01", Label: "outside"}, false, "invalid"); e == nil {
		t.Fatal("outside exception accepted")
	}
	var student, teacherID, waliID, subject int64
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Fondasi','2013-01-01','L') RETURNING id`).Scan(&student))
	p, e := s.CreatePlacement(ctx, admin.ID, student, a.ID)
	must(e)
	var startIsNull bool
	must(db.QueryRow(`SELECT starts_on IS NULL FROM app.student_placements WHERE id=$1`, p).Scan(&startIsNull))
	if !startIsNull {
		t.Fatal("legacy date invented")
	}
	if _, e = s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-06"); e == nil {
		t.Fatal("unknown date allowed")
	}
	if e = s.SavePlacementStart(ctx, cur.ID, p, 1, "2020-07-05", "test"); !errors.Is(e, ErrForbidden) {
		t.Fatal("Kurikulum changed placement date", e)
	}
	must(s.SavePlacementStart(ctx, admin.ID, p, 1, "2020-07-05", "Tanggal masuk menurut sekolah"))
	if e = s.SavePlacementStart(ctx, admin.ID, p, 1, "2020-07-04", "stale"); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Fondasi',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Wali Fondasi',$1) RETURNING id`, wali.ID).Scan(&waliID))
	must(s.AssignHomeroom(ctx, cur.ID, a.ID, waliID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel Fondasi') RETURNING id`, "FD"+testutil.Random(t)[:10]).Scan(&subject))
	from, _ := time.Parse("2006-01-02", "2020-07-01")
	must(s.AssignTeacher(ctx, cur.ID, sem, a.ID, subject, teacherID, from))
	other, _ := testutil.User(t, db, "guru_mapel")
	var otherTeacher, otherSubject int64
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Lain',$1) RETURNING id`, other.ID).Scan(&otherTeacher))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel Lain') RETURNING id`, "FO"+testutil.Random(t)[:10]).Scan(&otherSubject))
	must(s.AssignTeacher(ctx, cur.ID, sem, a.ID, otherSubject, otherTeacher, from))
	for _, sub := range []int64{subject, otherSubject} {
		must(s.SaveCurriculumRule(ctx, cur.ID, CurriculumRule{SemesterID: sem, Grade: 7, ClassID: a.ID, SubjectID: sub, WeeklyJP: 1, Meetings: []int{1}}))
	}
	must(s.SaveCurriculumRule(ctx, cur.ID, CurriculumRule{SemesterID: sem, Grade: 7, ClassID: b.ID, SubjectID: subject, WeeklyJP: 0, Meetings: []int{}}))
	must(s.SaveLessonPeriod(ctx, cur.ID, sem, 2, 1, "07:00", "07:40"))
	must(s.SaveLessonPeriod(ctx, cur.ID, sem, 2, 2, "07:40", "08:20"))
	makeRevision := func(date string, first int, publish bool) int64 {
		t.Helper()
		d, e := time.Parse("2006-01-02", date)
		must(e)
		rev, e := s.CreateTimetableDraft(ctx, cur.ID, sem, d)
		must(e)
		must(s.AddTimetableEntry(ctx, cur.ID, rev.ID, 2, a.ID, subject, teacherID, first, first))
		second := 3 - first
		must(s.AddTimetableEntry(ctx, cur.ID, rev.ID, 2, a.ID, otherSubject, otherTeacher, second, second))
		if publish {
			must(s.PublishTimetable(ctx, cur.ID, rev.ID))
		}
		return rev.ID
	}
	r1 := makeRevision("2020-07-01", 1, true)
	r2 := makeRevision("2020-07-13", 2, true)
	_ = makeRevision("2020-07-20", 1, false)
	first, e := s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-07")
	must(e)
	if first.RevisionID != r1 {
		t.Fatal("historical revision lost")
	}
	later, e := s.AttendanceContextOnDate(ctx, teacher.ID, a.ID, "2020-07-28")
	must(e)
	if later.RevisionID != r2 || len(later.Lessons) != 1 || later.Lessons[0].PeriodStart != 2 {
		t.Fatal("draft/future/mapel scope leaked", later)
	}
	all, e := s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-28")
	must(e)
	if len(all.Lessons) != 2 {
		t.Fatal("school reader lost lessons")
	}

	// Guru hanya melihat pertemuan dari penugasan sendiri; jadwal historis mengikuti tanggal.
	historical, e := s.TeacherLessonsOnDate(ctx, teacher.ID, "2020-07-07")
	must(e)
	if len(historical.Lessons) != 1 || historical.Lessons[0].PeriodStart != 1 {
		t.Fatal("historical published revision or teacher scope incorrect", historical)
	}
	var datedStudent int64
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Bertanggal Pertemuan','2013-01-01','L') RETURNING id`).Scan(&datedStudent))
	if _, e = s.CreatePlacement(ctx, admin.ID, datedStudent, a.ID, "2020-07-28"); e != nil {
		t.Fatal(e)
	}
	meetings, e := s.TeacherLessonsOnDate(ctx, teacher.ID, "2020-07-28")
	must(e)
	if len(meetings.Lessons) != 1 || meetings.Lessons[0].PeriodStart != 2 {
		t.Fatal("draft revision was used or role widened meeting scope", meetings)
	}
	lesson, e := s.TeacherLessonAttendanceOnDate(ctx, teacher.ID, "2020-07-28", meetings.Lessons[0].ID)
	must(e)
	containsDatedStudent := false
	for _, student := range lesson.Students {
		containsDatedStudent = containsDatedStudent || student.ID == datedStudent
	}
	if !containsDatedStudent || lesson.Lesson.TeacherID != teacherID {
		t.Fatal("lesson roster did not use placement date", lesson)
	}
	var otherEntryID int64
	for _, entry := range all.Lessons {
		if entry.TeacherID == otherTeacher {
			otherEntryID = entry.ID
		}
	}
	if otherEntryID == 0 {
		t.Fatal("other-subject fixture missing")
	}
	if _, e = s.TeacherLessonAttendanceOnDate(ctx, teacher.ID, "2020-07-28", otherEntryID); !errors.Is(e, ErrNotFound) {
		t.Fatal("teacher opened another subject's meeting", e)
	}
	otherLessons, e := s.TeacherLessonsOnDate(ctx, other.ID, "2020-07-28")
	must(e)
	if len(otherLessons.Lessons) != 1 || otherLessons.Lessons[0].SubjectID != otherSubject {
		t.Fatal("same-class different-subject assignment scope incorrect", otherLessons)
	}
	if _, e = s.TeacherLessonAttendanceOnDate(ctx, other.ID, "2020-07-28", meetings.Lessons[0].ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("teacher for another subject opened the meeting", e)
	}
	if _, e = s.TeacherLessonsOnDate(ctx, admin.ID, "2020-07-28"); !errors.Is(e, ErrForbidden) {
		t.Fatal("Admin without Guru Mapel opened lessons", e)
	}
	holidayLessons, e := s.TeacherLessonsOnDate(ctx, teacher.ID, "2020-07-21")
	must(e)
	if len(holidayLessons.Lessons) != 0 {
		t.Fatal("holiday produced an openable meeting", holidayLessons)
	}
	noTeacher, _ := testutil.User(t, db, "guru_mapel")
	if _, e = s.TeacherLessonsOnDate(ctx, noTeacher.ID, "2020-07-28"); !errors.Is(e, ErrForbidden) {
		t.Fatal("unmapped teacher opened lessons", e)
	}
	for _, u := range []int64{officer.ID, teacher.ID, wali.ID, kes.ID, head.ID} {
		v, e := s.AttendanceContextOnDate(ctx, u, a.ID, "2020-07-07")
		must(e)
		if len(v.Students) != 1 || !v.SchoolDay {
			t.Fatal(v)
		}
	}
	for _, u := range []int64{admin.ID, cur.ID, stranger.ID, bk.ID} {
		if _, e = s.AttendanceContextOnDate(ctx, u, a.ID, "2020-07-07"); !errors.Is(e, ErrForbidden) {
			t.Fatal("unexpected roster access", u, e)
		}
	}
	if _, e = s.AttendanceContextOnDate(ctx, teacher.ID, b.ID, "2020-07-07"); !errors.Is(e, ErrForbidden) {
		t.Fatal("teacher cross class", e)
	}
	v, e := s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-03")
	must(e)
	if len(v.Students) != 0 {
		t.Fatal("student before admission")
	}
	v, e = s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-06")
	must(e)
	if v.SchoolDay || len(v.Lessons) != 0 {
		t.Fatal("holiday generated lessons")
	}
	must(s.MoveStudentClass(ctx, cur.ID, p, a.ID, b.ID, "2020-07-08", "Pindah kelas"))
	v, e = s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-07")
	must(e)
	if len(v.Students) != 1 {
		t.Fatal("historical class lost")
	}
	v, e = s.AttendanceContextOnDate(ctx, officer.ID, b.ID, "2020-07-08")
	must(e)
	if len(v.Students) != 1 {
		t.Fatal("class move missing")
	}
	must(s.ExitStudentSchool(ctx, admin.ID, student, 1, "2020-07-10", "Pindah sekolah", "Sekolah lain"))
	v, e = s.AttendanceContextOnDate(ctx, officer.ID, b.ID, "2020-07-10")
	must(e)
	if len(v.Students) != 0 {
		t.Fatal("exited student included")
	}
	info, e := s.MovementStudent(ctx, admin.ID, student, 0)
	must(e)
	must(s.ReadmitStudent(ctx, admin.ID, student, info.Version, info.ExitID, b.ID, "2020-07-15", "Kembali", "Sekolah lain"))
	v, e = s.AttendanceContextOnDate(ctx, officer.ID, b.ID, "2020-07-14")
	must(e)
	if len(v.Students) != 0 {
		t.Fatal("outside-school gap lost")
	}
	v, e = s.AttendanceContextOnDate(ctx, officer.ID, b.ID, "2020-07-15")
	must(e)
	if len(v.Students) != 1 {
		t.Fatal("returned student missing")
	}
	if e = s.SavePlacementStart(ctx, admin.ID, p, 2, "2020-07-09", "invalid later date"); e == nil {
		t.Fatal("start after existing move accepted")
	}

	// A combined Admin+Guru role still has only the assigned lesson scope.
	_, e = db.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'admin')`, teacher.ID)
	must(e)
	scoped, e := s.AttendanceContextOnDate(ctx, teacher.ID, a.ID, "2020-07-28")
	must(e)
	if len(scoped.Lessons) != 1 {
		t.Fatal("Admin role widened teacher scope")
	}
	if _, e = s.AttendanceContextOnDate(ctx, teacher.ID, b.ID, "2020-07-28"); !errors.Is(e, ErrForbidden) {
		t.Fatal("Admin role widened class scope", e)
	}
	var futureStudent int64
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Baru Bertanggal','2013-01-01','L') RETURNING id`).Scan(&futureStudent))
	_, e = s.CreatePlacement(ctx, admin.ID, futureStudent, a.ID, "2020-07-20")
	must(e)
	before, e := s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-19")
	must(e)
	after, e := s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-20")
	must(e)
	if len(before.Students) != 0 || len(after.Students) != 1 {
		t.Fatal("dated placement boundary ignored")
	}
	// Role tambahan tidak memperluas daftar pertemuan di luar penugasan Guru Mapel.
	_, e = db.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'kesiswaan')`, teacher.ID)
	must(e)
	combined, e := s.TeacherLessonsOnDate(ctx, teacher.ID, "2020-07-28")
	must(e)
	if len(combined.Lessons) != 1 || combined.Lessons[0].TeacherID != teacherID {
		t.Fatal("combined role widened lesson scope", combined)
	}
	_, e = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, teacher.ID)
	must(e)
	if _, e = s.AttendanceContextOnDate(ctx, teacher.ID, a.ID, "2020-07-27"); !errors.Is(e, ErrForbidden) {
		t.Fatal("inactive account read context", e)
	}
	if _, e = s.TeacherLessonsOnDate(ctx, teacher.ID, "2020-07-28"); !errors.Is(e, ErrForbidden) {
		t.Fatal("inactive account read lesson list", e)
	}
	history, e := s.FoundationHistory(ctx, admin.ID, "placement_date", p)
	must(e)
	if len(history) != 1 {
		t.Fatal("failed operation wrote history", len(history))
	}
}

func TestTeacherLessonRosterFailureIsolationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	other, _ := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	year, err := s.CreateYear(ctx, cur.ID, "Roster isolation "+testutil.Random(t)[:10])
	must(err)
	must(s.ActivateYear(ctx, cur.ID, year.ID))
	classA, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	classB, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "B", "", 0)
	must(err)
	var semester int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester))
	must(s.SaveSemesterCalendar(ctx, cur.ID, SemesterCalendar{SemesterID: semester, StartsOn: "2020-07-01", EndsOn: "2020-12-31", SchoolDays: []int{1, 2, 3, 4, 5}}, "Test roster isolation"))

	var teacherID, otherTeacherID, subjectA, subjectB int64
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Roster 7A/7B',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Roster Lain',$1) RETURNING id`, other.ID).Scan(&otherTeacherID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel Roster Utama') RETURNING id`, "RI"+testutil.Random(t)[:10]).Scan(&subjectA))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel Roster Lain') RETURNING id`, "RJ"+testutil.Random(t)[:10]).Scan(&subjectB))
	starts, _ := time.Parse("2006-01-02", "2020-07-01")
	must(s.AssignTeacher(ctx, cur.ID, semester, classA.ID, subjectA, teacherID, starts))
	must(s.AssignTeacher(ctx, cur.ID, semester, classB.ID, subjectA, teacherID, starts))
	must(s.AssignTeacher(ctx, cur.ID, semester, classA.ID, subjectB, otherTeacherID, starts))
	must(s.SaveCurriculumRule(ctx, cur.ID, CurriculumRule{SemesterID: semester, Grade: 7, ClassID: classA.ID, SubjectID: subjectA, WeeklyJP: 1, Meetings: []int{1}}))
	must(s.SaveCurriculumRule(ctx, cur.ID, CurriculumRule{SemesterID: semester, Grade: 7, ClassID: classB.ID, SubjectID: subjectA, WeeklyJP: 1, Meetings: []int{1}}))
	must(s.SaveCurriculumRule(ctx, cur.ID, CurriculumRule{SemesterID: semester, Grade: 7, ClassID: classA.ID, SubjectID: subjectB, WeeklyJP: 1, Meetings: []int{1}}))
	must(s.SaveLessonDay(ctx, cur.ID, semester, 2, []LessonPeriod{
		{Number: 1, StartsAt: "07:00", EndsAt: "07:40"},
		{Number: 2, StartsAt: "07:40", EndsAt: "08:20"},
		{Number: 3, StartsAt: "08:20", EndsAt: "09:00"},
	}))

	var studentA, studentB, placementB int64
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Roster Lengkap','2013-01-01','L') RETURNING id`).Scan(&studentA))
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Roster Belum Lengkap','2013-01-01','L') RETURNING id`).Scan(&studentB))
	_, err = s.CreatePlacement(ctx, admin.ID, studentA, classA.ID, "2020-07-01")
	must(err)
	placementB, err = s.CreatePlacement(ctx, admin.ID, studentB, classB.ID)
	must(err)

	revision, err := s.CreateTimetableDraft(ctx, cur.ID, semester, starts)
	must(err)
	must(s.AddTimetableEntry(ctx, cur.ID, revision.ID, 2, classA.ID, subjectA, teacherID, 1, 1))
	must(s.AddTimetableEntry(ctx, cur.ID, revision.ID, 2, classB.ID, subjectA, teacherID, 2, 2))
	must(s.AddTimetableEntry(ctx, cur.ID, revision.ID, 2, classA.ID, subjectB, otherTeacherID, 3, 3))
	must(s.PublishTimetable(ctx, cur.ID, revision.ID))

	// Role kesiswaan yang ditambahkan tidak boleh membuka cakupan roster Guru Mapel.
	_, err = db.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'kesiswaan')`, teacher.ID)
	must(err)
	const date = "2020-07-07"
	meetings, err := s.TeacherLessonsOnDate(ctx, teacher.ID, date)
	must(err)
	if len(meetings.Lessons) != 2 {
		t.Fatalf("expected only the two assigned meetings, got %d", len(meetings.Lessons))
	}
	entries := make(map[int64]TimetableEntry)
	for _, lesson := range meetings.Lessons {
		entries[lesson.ClassID] = lesson
	}
	if entries[classA.ID].ID == 0 || entries[classB.ID].ID == 0 {
		t.Fatalf("published meetings for both assigned classes must remain listed: %+v", meetings.Lessons)
	}
	var dailyBefore, dailyAfter int
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE student_id IN($1,$2)`, studentA, studentB).Scan(&dailyBefore))
	lessonA, err := s.TeacherLessonAttendanceOnDate(ctx, teacher.ID, date, entries[classA.ID].ID)
	if err != nil {
		t.Fatalf("complete 7A roster should open: %v", err)
	}
	if len(lessonA.Students) != 1 || lessonA.Students[0].ID != studentA {
		t.Fatalf("7A should return only its complete roster: %+v", lessonA.Students)
	}
	var rosterErr ValidationError
	lessonB, err := s.TeacherLessonAttendanceOnDate(ctx, teacher.ID, date, entries[classB.ID].ID)
	if !errors.As(err, &rosterErr) || rosterErr != ValidationError("Tanggal mulai penempatan siswa belum lengkap. Minta Admin melengkapinya.") {
		t.Fatalf("incomplete 7B roster should return validation error, got %v", err)
	}
	if len(lessonB.Students) != 0 {
		t.Fatalf("validation failure exposed a partial roster: %+v", lessonB.Students)
	}
	if _, err = s.TeacherLessonAttendanceOnDate(ctx, other.ID, date, entries[classA.ID].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another teacher opened 7A meeting: %v", err)
	}
	otherMeetings, err := s.TeacherLessonsOnDate(ctx, other.ID, date)
	must(err)
	if len(otherMeetings.Lessons) != 1 || otherMeetings.Lessons[0].ClassID != classA.ID || otherMeetings.Lessons[0].SubjectID != subjectB {
		t.Fatalf("another teacher should see only their own assigned meeting: %+v", otherMeetings.Lessons)
	}
	must(s.SavePlacementStart(ctx, admin.ID, placementB, 1, "2020-07-01", "Tanggal penempatan dikonfirmasi"))
	lessonB, err = s.TeacherLessonAttendanceOnDate(ctx, teacher.ID, date, entries[classB.ID].ID)
	if err != nil {
		t.Fatalf("7B detail should open after placement date is completed: %v", err)
	}
	if len(lessonB.Students) != 1 || lessonB.Students[0].ID != studentB {
		t.Fatalf("7B should return its roster after date completion: %+v", lessonB.Students)
	}
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE student_id IN($1,$2)`, studentA, studentB).Scan(&dailyAfter))
	if dailyAfter != dailyBefore {
		t.Fatalf("GET contexts changed daily attendance count: before %d, after %d", dailyBefore, dailyAfter)
	}
}
