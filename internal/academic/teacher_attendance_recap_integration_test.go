//go:build integration

package academic

import (
	"context"
	"errors"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherAttendanceRecapIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := &Store{DB: db}
	teacher, _ := testutil.User(t, db, "guru_mapel", "admin", "kurikulum", "petugas_absensi")
	other, _ := testutil.User(t, db, "wali_kelas")
	manager, _ := testutil.User(t, db, "pegawai_tu", "pengelola_kehadiran_guru")
	plain, _ := testutil.User(t, db, "pegawai_tu")
	permissionOnly, _ := testutil.User(t, db, "pengelola_kehadiran_guru")
	officer, _ := testutil.User(t, db, "petugas_absensi")
	var teacherID, otherID, year, sem, sem2, class, subject, assignment, revision, revision2, entry int64
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('G3 sintetis',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('G3 lain',$1) RETURNING id`, other.ID).Scan(&otherID))
	// Tanggal lama memastikan koreksi lebih dari setahun, tanpa menimpa kalender bersama.
	var occupied bool
	must(db.QueryRow(`SELECT EXISTS(SELECT 1 FROM app.semester_calendars WHERE starts_on<='1906-02-28' AND ends_on>='1906-01-01')`).Scan(&occupied))
	if occupied {
		t.Fatal("rentang fixture G3 sudah dipakai; jangan mengubah kalender bersama")
	}
	must(db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "G3 "+testutil.Random(t)[:12]).Scan(&year))
	must(db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, year).Scan(&sem))
	must(db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,2) RETURNING id`, year).Scan(&sem2))
	must(db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,'G3') RETURNING id`, year).Scan(&class))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Mapel G3') RETURNING id`, "G3"+testutil.Random(t)[:8]).Scan(&subject))
	must(db.QueryRow(`INSERT INTO app.teaching_assignments(semester_id,class_id,subject_id,teacher_id,effective_from,assigned_by) VALUES($1,$2,$3,$4,'1906-01-01',$5) RETURNING id`, sem, class, subject, teacherID, manager.ID).Scan(&assignment))
	t.Cleanup(func() {
		tx, e := db.Begin()
		if e != nil {
			t.Error(e)
			return
		}
		defer tx.Rollback()
		for _, id := range []int64{teacherID, otherID} {
			for _, q := range []string{
				`DELETE FROM app.teacher_attendance_report_history WHERE report_id IN(SELECT id FROM app.teacher_attendance_reports WHERE teacher_id=$1)`,
				`DELETE FROM app.teacher_attendance_reports WHERE teacher_id=$1`,
				`DELETE FROM app.teacher_attendance_history WHERE event_id IN(SELECT id FROM app.teacher_attendance_events WHERE teacher_id=$1)`,
				`DELETE FROM app.teacher_attendance_events WHERE teacher_id=$1`,
			} {
				if _, e = tx.Exec(q, id); e != nil {
					t.Error(e)
					return
				}
			}
		}
		for _, q := range []string{
			`DELETE FROM app.weekly_timetable_entries WHERE revision_id IN(SELECT id FROM app.weekly_timetable_revisions WHERE semester_id IN(SELECT id FROM app.semesters WHERE school_year_id=$1))`,
			`DELETE FROM app.weekly_timetable_periods WHERE revision_id IN(SELECT id FROM app.weekly_timetable_revisions WHERE semester_id IN(SELECT id FROM app.semesters WHERE school_year_id=$1))`,
			`DELETE FROM app.weekly_timetable_revisions WHERE semester_id IN(SELECT id FROM app.semesters WHERE school_year_id=$1)`,
			`DELETE FROM app.teaching_assignments WHERE semester_id IN(SELECT id FROM app.semesters WHERE school_year_id=$1)`,
			`DELETE FROM app.school_date_exceptions WHERE semester_id IN(SELECT id FROM app.semesters WHERE school_year_id=$1)`,
			`DELETE FROM app.semester_calendars WHERE semester_id IN(SELECT id FROM app.semesters WHERE school_year_id=$1)`,
			`DELETE FROM app.classes WHERE school_year_id=$1`, `DELETE FROM app.semesters WHERE school_year_id=$1`, `DELETE FROM app.school_years WHERE id=$1`,
		} {
			if _, e = tx.Exec(q, year); e != nil {
				t.Error(e)
				return
			}
		}
		if _, e = tx.Exec(`DELETE FROM app.subjects WHERE id=$1`, subject); e != nil {
			t.Error(e)
			return
		}
		for _, id := range []int64{teacherID, otherID} {
			if _, e = tx.Exec(`DELETE FROM app.teachers WHERE id=$1`, id); e != nil {
				t.Error(e)
				return
			}
		}
		for _, id := range []int64{teacher.ID, other.ID, manager.ID, plain.ID, officer.ID, permissionOnly.ID} {
			for _, q := range []string{`DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
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
	_, e := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'1906-01-01','1906-01-31','[1,2,3,4,5,6,7]'),($2,'1906-02-01','1906-02-28','[1,2,3,4,5,6,7]')`, sem, sem2)
	must(e)
	build := func(number int, date, start string) int64 {
		t.Helper()
		var rev int64
		must(db.QueryRow(`INSERT INTO app.weekly_timetable_revisions(semester_id,revision_no,status,effective_from,created_by,published_by,published_at) VALUES($1,$2,'published',$3,$4,$4,clock_timestamp()) RETURNING id`, sem, number, date, manager.ID).Scan(&rev))
		for day := 1; day <= 7; day++ {
			_, e := db.Exec(`INSERT INTO app.weekly_timetable_periods(revision_id,weekday,period_no,starts_at,ends_at) VALUES($1,$2,1,$3::time,($3::time+interval '1 hour')),($1,$2,2,($3::time+interval '2 hours'),($3::time+interval '4 hours'))`, rev, day, start)
			must(e)
			var id int64
			must(db.QueryRow(`INSERT INTO app.weekly_timetable_entries(revision_id,weekday,class_id,subject_id,teacher_id,assignment_id,period_start,period_end) VALUES($1,$2,$3,$4,$5,$6,1,2) RETURNING id`, rev, day, class, subject, teacherID, assignment).Scan(&id))
			if day == 1 && number == 1 {
				entry = id
			}
		}
		return rev
	}
	revision = build(1, "1906-01-01", "07:00")
	revision2 = build(2, "1906-01-15", "09:00")
	_, e = db.Exec(`INSERT INTO app.school_date_exceptions(semester_id,on_date,school_day,label) VALUES($1,'1906-01-03',false,'Libur sintetis')`, sem)
	must(e)
	read := func(actor int64, f TeacherAttendanceRecapFilter) TeacherAttendanceRecapResult {
		t.Helper()
		r, e := s.TeacherAttendanceRecaps(ctx, actor, f)
		must(e)
		return r
	}
	filter := TeacherAttendanceRecapFilter{StartsOn: "1906-01-01", EndsOn: "1906-02-02", Page: 1}
	r := read(teacher.ID, filter)
	if len(r.Teachers) != 1 || len(r.Teachers[0].Days) != 33 {
		t.Fatal(r)
	}
	day := func(r TeacherAttendanceRecapResult, date string) TeacherAttendanceEvaluation {
		t.Helper()
		for _, d := range r.Teachers[0].Days {
			if d.Date == date {
				return d
			}
		}
		t.Fatal("tanggal hilang", date)
		return TeacherAttendanceEvaluation{}
	}
	d := day(r, "1906-01-01")
	if d.Status != TeacherUnrecorded || d.RevisionID != revision || d.Obligations[0].EntryID != entry || d.StartsAt.Hour() != 7 || d.EndsAt.Hour() != 11 || d.Obligations[0].Segments != "07:00–08:00 · jeda · 09:00–11:00" {
		t.Fatal(d)
	}
	if d = day(r, "1906-01-15"); d.RevisionID != revision2 || d.StartsAt.Hour() != 9 {
		t.Fatal(d)
	}
	if d = day(r, "1906-01-03"); d.Status != TeacherUnscheduled || d.Required || d.CalendarNote != "Libur sintetis" {
		t.Fatal(d)
	}
	if d = day(r, "1906-02-01"); d.Status != TeacherUnevaluable || d.SemesterID != sem2 {
		t.Fatal(d)
	}
	others := read(other.ID, filter)
	if others.Teachers[0].Days[0].Status != TeacherUnscheduled {
		t.Fatal(others)
	}
	foreign := filter
	foreign.TeacherID = otherID
	if _, e = s.TeacherAttendanceRecaps(ctx, teacher.ID, foreign); !errors.Is(e, ErrNotFound) {
		t.Fatal("IDOR", e)
	}
	managed := filter
	managed.Managed = true
	managed.TeacherID = teacherID
	if len(read(manager.ID, managed).Teachers) != 1 {
		t.Fatal("TU ditolak")
	}
	for _, id := range []int64{teacher.ID, other.ID, plain.ID, officer.ID, permissionOnly.ID} {
		if _, e = s.TeacherAttendanceRecaps(ctx, id, managed); !errors.Is(e, ErrForbidden) {
			t.Fatal("role memperluas akses", id, e)
		}
	}
	event := func(date, action, clock string) TeacherAttendanceManualProof {
		t.Helper()
		key, e := NewTeacherAttendanceIdempotencyKey()
		must(e)
		result, e := s.RecordTeacherAttendanceManual(ctx, manager.ID, TeacherAttendanceManualInput{TeacherID: teacherID, Action: TeacherAttendanceAction(action), OccurredAt: date + "T" + clock, Reason: "Bukti sintetis G3", IdempotencyKey: key})
		must(e)
		return result
	}
	in := event("1906-01-01", "check_in", "07:00")
	event("1906-01-01", "check_out", "11:00")
	r = read(teacher.ID, filter)
	if d = day(r, "1906-01-01"); d.Status != TeacherPresent || d.CheckIn.Source != "manual" {
		t.Fatal(d)
	}
	_, e = s.CorrectTeacherAttendance(ctx, manager.ID, TeacherAttendanceCorrectionInput{EventID: in.EventID, ExpectedVersion: 1, OccurredAt: "1906-01-01T07:01", CorrectionReason: "Koreksi historis lebih dari satu tahun"})
	must(e)
	if d = day(read(teacher.ID, filter), "1906-01-01"); d.Status != TeacherLate || d.LateMinutes != 1 || d.CheckIn.Version != 2 {
		t.Fatal(d)
	}
	createReport := func(kind, from, to string) TeacherAttendanceReport {
		t.Helper()
		key, e := NewTeacherAttendanceIdempotencyKey()
		must(e)
		r, e := s.CreateTeacherAttendanceReport(ctx, teacher.ID, TeacherAttendanceReportInput{Kind: kind, StartsOn: from, EndsOn: to, Description: "G3 sintetis", IdempotencyKey: key})
		must(e)
		return r
	}
	report := createReport("izin", "1906-01-01", "1906-01-02")
	r = read(teacher.ID, filter)
	if d = day(r, "1906-01-01"); d.Status != TeacherLeave || !d.NeedsReview || d.CheckIn == nil || d.CheckOut == nil {
		t.Fatal(d)
	}
	if d = day(r, "1906-01-02"); d.Status != TeacherLeave {
		t.Fatal("inklusif", d)
	}
	_, e = s.CorrectTeacherAttendanceReport(ctx, manager.ID, TeacherAttendanceReportCorrection{ReportID: report.ID, ExpectedVersion: 1, Reason: "Pembatalan sintetis", Cancel: true})
	must(e)
	if d = day(read(teacher.ID, filter), "1906-01-01"); d.Status != TeacherLate || d.Report != nil {
		t.Fatal(d)
	}
	createReport("sakit", "1906-01-04", "1906-01-06")
	createReport("dinas_luar", "1906-01-03", "1906-01-03")
	r = read(teacher.ID, filter)
	for _, date := range []string{"1906-01-04", "1906-01-05", "1906-01-06"} {
		if day(r, date).Status != TeacherSick {
			t.Fatal("sakit rentang", date)
		}
	}
	if d = day(r, "1906-01-03"); d.Status != TeacherDuty || d.Required {
		t.Fatal(d)
	}
	// GET berulang tidak menulis event, metadata, histori atau absensi siswa.
	digest := func() string {
		t.Helper()
		var v string
		must(db.QueryRow(`SELECT md5(coalesce((SELECT string_agg(to_jsonb(e)::text,'' ORDER BY id) FROM app.teacher_attendance_events e WHERE teacher_id=$1),'')||coalesce((SELECT string_agg(to_jsonb(h)::text,'' ORDER BY h.id) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1),'')||(SELECT count(*)::text FROM app.student_daily_attendance))`, teacherID).Scan(&v))
		return v
	}
	before := digest()
	read(teacher.ID, filter)
	read(manager.ID, managed)
	if digest() != before {
		t.Fatal("rekap menulis bukti")
	}
	// Satu revisi baru tidak mengubah periode lama dan draft tidak menjadi sumber.
	_, e = db.Exec(`INSERT INTO app.weekly_timetable_revisions(semester_id,revision_no,status,effective_from,created_by) VALUES($1,3,'draft','1906-01-20',$2)`, sem, manager.ID)
	must(e)
	if d = day(read(teacher.ID, filter), "1906-01-21"); d.RevisionID != revision2 {
		t.Fatal("draft dipakai", d)
	}
	_, e = db.Exec(`UPDATE app.teaching_assignments SET effective_until='1906-01-25' WHERE id=$1`, assignment)
	must(e)
	if d = day(read(teacher.ID, filter), "1906-01-26"); d.Status != TeacherUnevaluable {
		t.Fatal("penugasan basi", d)
	}
	// Batas UTC tidak memengaruhi tanggal sipil yang diminta.
	if local := TeacherAttendanceWIBTime(day(r, "1906-01-01").StartsAt); local.Format("2006-01-02 15:04") != "1906-01-01 07:00" {
		t.Fatal("WIB")
	}
}
