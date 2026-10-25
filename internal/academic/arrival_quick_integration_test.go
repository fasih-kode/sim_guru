//go:build integration

package academic

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestArrivalRulesAndQuickIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	kes, _ := testutil.User(t, db, "kesiswaan")
	officer, _ := testutil.User(t, db, "petugas_absensi")
	year, e := s.CreateYear(ctx, cur.ID, "Quick "+testutil.Random(t)[:8])
	must(e)
	must(s.ActivateYear(ctx, cur.ID, year.ID))
	c, e := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(e)
	now := time.Now().In(attendanceWIB)
	date := now.AddDate(0, 0, -2).Format("2006-01-02")
	start := now.AddDate(0, 0, -20).Format("2006-01-02")
	end := now.AddDate(0, 0, 20).Format("2006-01-02")
	day, _ := civilDate(date)
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	var sem int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&sem))
	must(s.SaveSemesterCalendar(ctx, cur.ID, SemesterCalendar{SemesterID: sem, StartsOn: start, EndsOn: end, SchoolDays: []int{1, 2, 3, 4, 5, 6, 7}}, "Kalender uji"))
	rule := ArrivalRule{SemesterID: sem, Weekday: weekday, EffectiveFrom: start, Deadline: "07:00", Tolerance: 5, Reason: "Batas sekolah"}
	for _, actor := range []int64{admin.ID, cur.ID, officer.ID} {
		if _, e = s.SaveArrivalRule(ctx, actor, 0, rule); !errors.Is(e, ErrForbidden) {
			t.Fatal("rule write access", e)
		}
	}
	base, e := s.SaveArrivalRule(ctx, kes.ID, 0, rule)
	must(e)
	if _, e = s.SaveArrivalRule(ctx, kes.ID, 0, rule); !errors.Is(e, ErrConflict) {
		t.Fatal("rule concurrency", e)
	}
	settings, e := s.ArrivalSettings(ctx, kes.ID, sem)
	must(e)
	if len(settings.Rules) != 1 || len(settings.Classes) != 1 {
		t.Fatal(settings)
	}
	var ids []int64
	for i := 0; i < 5; i++ {
		var id int64
		must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, fmt.Sprintf("Siswa Cepat %d", i)).Scan(&id))
		_, e = s.CreatePlacement(ctx, admin.ID, id, c.ID, start)
		must(e)
		ids = append(ids, id)
	}
	sheet, e := s.DailyAttendance(ctx, officer.ID, c.ID, date)
	must(e)
	in := DailyQuickInput{ClassID: c.ID, Date: date, Reason: "Pencatatan manual kelas", Fingerprint: sheet.QuickFingerprint, Rows: []DailyQuickRow{{StudentID: ids[0], Status: "hadir", Arrival: "07:08"}, {StudentID: ids[1], Status: "hadir", Arrival: "07:05"}, {StudentID: ids[2], Status: "sakit"}}}
	if _, e = s.SaveQuickDaily(ctx, kes.ID, in); !errors.Is(e, ErrForbidden) {
		t.Fatal("bulk write access", e)
	}
	bad := in
	bad.Rows = append([]DailyQuickRow(nil), in.Rows...)
	bad.Rows[1].Arrival = ""
	if _, e = s.SaveQuickDaily(ctx, officer.ID, bad); e == nil {
		t.Fatal("missing arrival accepted")
	}
	var count int
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance WHERE class_id=$1`, c.ID).Scan(&count))
	if count != 0 {
		t.Fatal("partial batch")
	}
	// A new rule invalidates an open form before any attendance is saved.
	rule.Deadline = "07:01"
	next, e := s.SaveArrivalRule(ctx, kes.ID, base, rule)
	must(e)
	if _, e = s.SaveQuickDaily(ctx, officer.ID, in); !errors.Is(e, ErrConflict) {
		t.Fatal("stale rule", e)
	}
	sheet, e = s.DailyAttendance(ctx, officer.ID, c.ID, date)
	must(e)
	in.Fingerprint = sheet.QuickFingerprint
	n, e := s.SaveQuickDaily(ctx, officer.ID, in)
	must(e)
	if n != 3 {
		t.Fatal(n)
	}
	if _, e = s.SaveQuickDaily(ctx, officer.ID, in); !errors.Is(e, ErrConflict) {
		t.Fatal("replay", e)
	}
	stored, e := s.DailyAttendance(ctx, officer.ID, c.ID, date)
	must(e)
	records := map[int64]DailyStudent{}
	for _, d := range stored.Students {
		records[d.StudentID] = d
	}
	if records[ids[0]].LateMinutes.Int64 != 7 || !records[ids[0]].LateMinutes.Valid || records[ids[1]].LateMinutes.Int64 != 0 || records[ids[2]].ArrivedAt.Valid || records[ids[3]].ID != 0 {
		t.Fatal("assessment", records)
	}
	// Revisions cannot rewrite already recorded assessments, including corrections.
	rule.Deadline = "06:00"
	_, e = s.SaveArrivalRule(ctx, kes.ID, next, rule)
	must(e)
	corrected, e := s.SaveDailyAttendance(ctx, officer.ID, DailyInput{ClassID: c.ID, Date: date, StudentID: ids[0], Version: 1, Status: "hadir", Arrival: "07:09", Reason: "Koreksi bukti"})
	must(e)
	if corrected.Policy.ID != next || corrected.LateMinutes.Int64 != 8 {
		t.Fatal("rule snapshot replaced", corrected)
	}
	assertPolicy := func(want int64) {
		t.Helper()
		tx, e := s.reader(ctx, officer.ID, "petugas_absensi")
		must(e)
		p, e := arrivalPolicy(ctx, tx, sem, c.ID, date)
		tx.Rollback()
		must(e)
		if p == nil || p.ID != want {
			t.Fatal("policy priority", want, p)
		}
	}
	// Priority: date + class, date default, weekly class, weekly default.
	classRule := rule
	classRule.ClassID = c.ID
	classRule.Deadline = "08:00"
	classID, e := s.SaveArrivalRule(ctx, kes.ID, 0, classRule)
	must(e)
	assertPolicy(classID)
	dateRule := rule
	dateRule.Date = date
	dateRule.Deadline = "09:00"
	dateID, e := s.SaveArrivalRule(ctx, kes.ID, 0, dateRule)
	must(e)
	assertPolicy(dateID)
	both := dateRule
	both.ClassID = c.ID
	both.Deadline = "10:00"
	bothID, e := s.SaveArrivalRule(ctx, kes.ID, 0, both)
	must(e)
	tx, e := s.reader(ctx, officer.ID, "petugas_absensi")
	must(e)
	p, e := arrivalPolicy(ctx, tx, sem, c.ID, date)
	must(e)
	tx.Rollback()
	if p.ID != bothID || p.ID == classID || p.ID == dateID {
		t.Fatal("priority", p)
	}
	// An intervening record conflicts with bulk and remains untouched.
	fresh, e := s.DailyAttendance(ctx, officer.ID, c.ID, date)
	must(e)
	_, e = s.SaveDailyAttendance(ctx, officer.ID, DailyInput{ClassID: c.ID, Date: date, StudentID: ids[3], Status: "izin", Reason: "Sudah dicatat petugas lain", Fingerprint: fresh.Fingerprint})
	must(e)
	remainder := DailyQuickInput{ClassID: c.ID, Date: date, Reason: "Manual", Fingerprint: fresh.QuickFingerprint, Rows: []DailyQuickRow{{StudentID: ids[4], Status: "alpa"}}}
	if _, e = s.SaveQuickDaily(ctx, officer.ID, remainder); !errors.Is(e, ErrConflict) {
		t.Fatal("new record lost", e)
	}
	fresh, e = s.DailyAttendance(ctx, officer.ID, c.ID, date)
	must(e)
	remainder.Fingerprint = fresh.QuickFingerprint
	// Two requests with the same form can only apply one batch.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.SaveQuickDaily(ctx, officer.ID, remainder); results <- e }()
	}
	wg.Wait()
	close(results)
	ok, conflicts := 0, 0
	for e := range results {
		if e == nil {
			ok++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatal(ok, conflicts)
	}
	// Direct arrival takes server time only and refuses a historical date.
	if _, e = s.SaveDailyAttendance(ctx, officer.ID, DailyInput{ClassID: c.ID, Date: date, StudentID: ids[0], Version: 2, Status: "hadir", Now: true, Reason: "Sekarang"}); e == nil {
		t.Fatal("historical now accepted")
	}
}
