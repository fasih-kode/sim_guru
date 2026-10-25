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
	cal, e = s.SemesterCalendar(ctx, cur.ID, sem)
	must(e)
	if len(cal.Exceptions) != 1 || cal.Version != 4 {
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
	must(s.SaveLessonPeriod(ctx, cur.ID, sem, 1, 1, "07:00", "07:40"))
	must(s.SaveLessonPeriod(ctx, cur.ID, sem, 1, 2, "07:40", "08:20"))
	makeRevision := func(date string, first int, publish bool) int64 {
		t.Helper()
		d, e := time.Parse("2006-01-02", date)
		must(e)
		rev, e := s.CreateTimetableDraft(ctx, cur.ID, sem, d)
		must(e)
		must(s.AddTimetableEntry(ctx, cur.ID, rev.ID, 1, a.ID, subject, teacherID, first, first))
		second := 3 - first
		must(s.AddTimetableEntry(ctx, cur.ID, rev.ID, 1, a.ID, otherSubject, otherTeacher, second, second))
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
	later, e := s.AttendanceContextOnDate(ctx, teacher.ID, a.ID, "2020-07-27")
	must(e)
	if later.RevisionID != r2 || len(later.Lessons) != 1 || later.Lessons[0].PeriodStart != 2 {
		t.Fatal("draft/future/mapel scope leaked", later)
	}
	all, e := s.AttendanceContextOnDate(ctx, officer.ID, a.ID, "2020-07-27")
	must(e)
	if len(all.Lessons) != 2 {
		t.Fatal("school reader lost lessons")
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
	scoped, e := s.AttendanceContextOnDate(ctx, teacher.ID, a.ID, "2020-07-27")
	must(e)
	if len(scoped.Lessons) != 1 {
		t.Fatal("Admin role widened teacher scope")
	}
	if _, e = s.AttendanceContextOnDate(ctx, teacher.ID, b.ID, "2020-07-27"); !errors.Is(e, ErrForbidden) {
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
	_, e = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, teacher.ID)
	must(e)
	if _, e = s.AttendanceContextOnDate(ctx, teacher.ID, a.ID, "2020-07-27"); !errors.Is(e, ErrForbidden) {
		t.Fatal("inactive account read context", e)
	}
	history, e := s.FoundationHistory(ctx, admin.ID, "placement_date", p)
	must(e)
	if len(history) != 1 {
		t.Fatal("failed operation wrote history", len(history))
	}
}
