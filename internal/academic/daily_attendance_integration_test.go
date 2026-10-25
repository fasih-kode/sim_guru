//go:build integration

package academic

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentDailyAttendanceIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	officer, _ := testutil.User(t, db, "petugas_absensi")
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	wali, _ := testutil.User(t, db, "wali_kelas")
	head, _ := testutil.User(t, db, "kepala_sekolah")
	kes, _ := testutil.User(t, db, "kesiswaan")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	year, e := s.CreateYear(ctx, cur.ID, "Daily "+testutil.Random(t)[:8])
	must(e)
	must(s.ActivateYear(ctx, cur.ID, year.ID))
	c, e := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(e)
	other, e := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "B", "", 0)
	must(e)
	now := time.Now().In(attendanceWIB)
	today := now.Format("2006-01-02")
	past := now.AddDate(0, 0, -2).Format("2006-01-02")
	start := now.AddDate(0, 0, -20).Format("2006-01-02")
	end := now.AddDate(0, 0, 20).Format("2006-01-02")
	var sem int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&sem))
	must(s.SaveSemesterCalendar(ctx, cur.ID, SemesterCalendar{SemesterID: sem, StartsOn: start, EndsOn: end, SchoolDays: []int{1, 2, 3, 4, 5, 6, 7}}, "Kalender uji"))
	var sid1, sid2, tid int64
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa QR Uji','2013-01-01','L') RETURNING id`).Scan(&sid1))
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Manual Uji','2013-01-01','P') RETURNING id`).Scan(&sid2))
	p1, e := s.CreatePlacement(ctx, admin.ID, sid1, c.ID, start)
	must(e)
	_, e = s.CreatePlacement(ctx, admin.ID, sid2, c.ID, start)
	must(e)
	must(db.QueryRow(`INSERT INTO app.teachers(user_id,name,active) VALUES($1,'Wali Uji',true) RETURNING id`, wali.ID).Scan(&tid))
	must(s.AssignHomeroom(ctx, cur.ID, c.ID, tid))
	sheet, e := s.DailyAttendance(ctx, officer.ID, c.ID, today)
	must(e)
	if len(sheet.Students) != 2 || sheet.Students[0].Status != "" {
		t.Fatal("default status", sheet)
	}
	for _, actor := range []int64{admin.ID, cur.ID, teacher.ID} {
		if _, e = s.DailyAttendance(ctx, actor, c.ID, today); !errors.Is(e, ErrForbidden) {
			t.Fatal("read access", actor, e)
		}
	}
	for _, actor := range []int64{wali.ID, head.ID, kes.ID} {
		_, e = s.DailyAttendance(ctx, actor, c.ID, today)
		must(e)
	}
	if _, e = s.DailyAttendance(ctx, wali.ID, other.ID, today); !errors.Is(e, ErrForbidden) {
		t.Fatal("wali scope", e)
	}
	card, e := s.ManageStudentCard(ctx, officer.ID, sid1, 0, "issue", "Kartu pertama")
	must(e)
	if !validStudentQR(card.Token) {
		t.Fatal("invalid QR generated")
	}
	if _, e = s.StudentCard(ctx, admin.ID, sid1); !errors.Is(e, ErrForbidden) {
		t.Fatal("card access", e)
	}
	repeat, e := s.ManageStudentCard(ctx, officer.ID, sid1, card.CardID, "reprint", "Kartu rusak")
	must(e)
	if repeat.Token != card.Token {
		t.Fatal("reprint rotated")
	}
	replaced, e := s.ManageStudentCard(ctx, officer.ID, sid1, card.CardID, "replace", "Kartu hilang")
	must(e)
	if replaced.Token == card.Token {
		t.Fatal("rotation failed")
	}
	if _, e = s.ScanStudentQR(ctx, officer.ID, card.Token); e == nil {
		t.Fatal("revoked accepted")
	}
	// Concurrent duplicate scans create one record and one initial history only.
	var wg sync.WaitGroup
	results := make(chan DailyScanResult, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.ScanStudentQR(ctx, officer.ID, replaced.Token)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		must(e)
	}
	duplicates := 0
	var scan DailyScanResult
	for r := range results {
		scan = r
		if r.Duplicate {
			duplicates++
		}
	}
	if duplicates != 1 {
		t.Fatal("duplicate scans", duplicates)
	}
	if scan.Student.Date != today || scan.Student.ClassID != c.ID || scan.Student.Source != "qr" || !scan.Student.ArrivedAt.Valid {
		t.Fatal(scan)
	}
	var n int
	must(db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance_history WHERE attendance_id=$1`, scan.Student.ID).Scan(&n))
	if n != 1 {
		t.Fatal("duplicate history", n)
	}
	in := DailyInput{ClassID: c.ID, StudentID: sid2, Date: today, Status: "sakit", Reason: "Surat orang tua", Fingerprint: sheet.Fingerprint}
	if _, e = s.SaveDailyAttendance(ctx, admin.ID, in); !errors.Is(e, ErrForbidden) {
		t.Fatal("write access", e)
	}
	manual, e := s.SaveDailyAttendance(ctx, officer.ID, in)
	must(e)
	if manual.ArrivedAt.Valid {
		t.Fatal("non hadir timestamp")
	}
	card2, e := s.ManageStudentCard(ctx, officer.ID, sid2, 0, "issue", "Awal")
	must(e)
	if _, e = s.ScanStudentQR(ctx, officer.ID, card2.Token); e == nil {
		t.Fatal("scan overwrote sick")
	}
	for _, bad := range []string{"6:11", "06:11 PM", "24:00", "06:60"} {
		invalid := in
		invalid.Version = manual.Version
		invalid.Status = "hadir"
		invalid.Arrival = bad
		if _, e = s.SaveDailyAttendance(ctx, officer.ID, invalid); e == nil {
			t.Fatalf("invalid arrival accepted: %s", bad)
		}
	}
	in.Version = manual.Version
	in.Status = "hadir"
	in.Arrival = "00:00"
	in.Reason = "Koreksi bukti hadir"
	corrected, e := s.SaveDailyAttendance(ctx, officer.ID, in)
	must(e)
	if corrected.Source != "correction" || corrected.Version != 2 {
		t.Fatal(corrected)
	}
	if _, e = s.SaveDailyAttendance(ctx, officer.ID, in); !errors.Is(e, ErrConflict) {
		t.Fatal("stale correction", e)
	}
	history, e := s.DailyHistory(ctx, wali.ID, corrected.ID)
	must(e)
	if len(history) != 2 {
		t.Fatal("history", len(history))
	}

	// A returning student is absent during the gap, then belongs to the new class.
	var sid3 int64
	must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa Kembali Uji','2013-01-01','L') RETURNING id`).Scan(&sid3))
	thirdStart := now.AddDate(0, 0, -12).Format("2006-01-02")
	_, e = s.CreatePlacement(ctx, admin.ID, sid3, c.ID, thirdStart)
	must(e)
	must(s.ExitStudentSchool(ctx, admin.ID, sid3, 1, now.AddDate(0, 0, -10).Format("2006-01-02"), "Pindah sekolah", "Sekolah lain"))
	var exitID int64
	must(db.QueryRow(`SELECT id FROM app.student_school_exits WHERE student_id=$1`, sid3).Scan(&exitID))
	must(s.ReadmitStudent(ctx, admin.ID, sid3, 2, exitID, other.ID, now.AddDate(0, 0, -8).Format("2006-01-02"), "Kembali", "Sekolah lain"))
	gapDate := now.AddDate(0, 0, -9).Format("2006-01-02")
	gap, e := s.DailyAttendance(ctx, officer.ID, c.ID, gapDate)
	must(e)
	for _, row := range gap.Students {
		if row.StudentID == sid3 {
			t.Fatal("outside-school gap included")
		}
	}
	if _, e = s.SaveDailyAttendance(ctx, officer.ID, DailyInput{ClassID: c.ID, StudentID: sid3, Date: gapDate, Status: "hadir", Reason: "Tidak boleh saat jeda", Fingerprint: gap.Fingerprint}); e == nil {
		t.Fatal("gap attendance accepted")
	}
	returned, e := s.DailyAttendance(ctx, officer.ID, other.ID, past)
	must(e)
	foundReturn := false
	for _, row := range returned.Students {
		if row.StudentID == sid3 {
			foundReturn = true
		}
	}
	if !foundReturn {
		t.Fatal("return class missing")
	}
	before, e := s.DailyAttendance(ctx, officer.ID, c.ID, now.AddDate(0, 0, -15).Format("2006-01-02"))
	must(e)
	for _, row := range before.Students {
		if row.StudentID == sid3 {
			t.Fatal("before enrollment included")
		}
	}
	// Roster change after form opens must reject a new record, not silently place it elsewhere.
	oldSheet, e := s.DailyAttendance(ctx, officer.ID, c.ID, past)
	must(e)
	must(s.MoveStudentClass(ctx, cur.ID, p1, c.ID, other.ID, past, "Pindah rombel"))
	if _, e = s.SaveDailyAttendance(ctx, officer.ID, DailyInput{ClassID: c.ID, StudentID: sid1, Date: past, Status: "hadir", Reason: "Form lama", Fingerprint: oldSheet.Fingerprint}); !errors.Is(e, ErrConflict) {
		t.Fatal("stale roster", e)
	}
	// Evidence remains in the recorded class despite retroactive roster/name changes.
	mustExec := func(query string, args ...any) { t.Helper(); _, e := db.Exec(query, args...); must(e) }
	mustExec(`UPDATE app.students SET name='Nama Diubah' WHERE id=$1`, sid1)
	frozen, e := s.DailyAttendance(ctx, officer.ID, c.ID, today)
	must(e)
	found := false
	for _, row := range frozen.Students {
		if row.StudentID == sid1 {
			found = true
			if row.Name != "Siswa QR Uji" || row.ClassID != c.ID {
				t.Fatal("snapshot rewritten", row)
			}
		}
	}
	if !found {
		t.Fatal("snapshot disappeared")
	}
	if _, e = s.DailyHistory(ctx, wali.ID, scan.Student.ID); e != nil {
		t.Fatal(e)
	}
	// Holiday blocks new manual/QR records, but corrections and history survive.
	must(s.SaveSchoolDateException(ctx, cur.ID, sem, 1, SchoolDateException{Date: today, Label: "Libur mendadak"}, false, "Libur"))
	if _, e = s.ScanStudentQR(ctx, officer.ID, replaced.Token); e == nil {
		t.Fatal("holiday scan")
	}
	current, e := s.DailyAttendance(ctx, officer.ID, other.ID, today)
	must(e)
	if current.SchoolDay {
		t.Fatal("holiday")
	}
	if _, e = s.SaveDailyAttendance(ctx, officer.ID, DailyInput{ClassID: other.ID, StudentID: sid1, Date: today, Status: "izin", Reason: "Salah kelas", Fingerprint: current.Fingerprint}); !errors.Is(e, ErrConflict) {
		t.Fatal("cross-class duplicate", e)
	}
	future := now.AddDate(0, 0, 1).Format("2006-01-02")
	if _, e = s.DailyAttendance(ctx, officer.ID, c.ID, future); e == nil {
		t.Fatal("future accepted")
	}
	// Aborted audit write rolls back an attendance correction as well.
	mustExec(`CREATE FUNCTION app.fail_daily_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test rollback'; END $$; CREATE TRIGGER fail_daily_history BEFORE INSERT ON app.student_daily_attendance_history FOR EACH ROW EXECUTE FUNCTION app.fail_daily_history()`)
	t.Cleanup(func() {
		db.Exec(`DROP TRIGGER IF EXISTS fail_daily_history ON app.student_daily_attendance_history; DROP FUNCTION IF EXISTS app.fail_daily_history()`)
	})
	in.Version = 2
	in.Status = "izin"
	in.Arrival = ""
	if _, e = s.SaveDailyAttendance(ctx, officer.ID, in); e == nil {
		t.Fatal("failure missing")
	}
	var status string
	must(db.QueryRow(`SELECT status FROM app.student_daily_attendance WHERE id=$1`, corrected.ID).Scan(&status))
	if status != "hadir" {
		t.Fatal("partial correction")
	}
}
