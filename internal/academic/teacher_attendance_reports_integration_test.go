//go:build integration

package academic_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherAttendanceReportsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := &academic.Store{DB: db}
	var users, teachers []int64
	actor := func(roles ...string) (auth.User, int64) {
		u, _ := testutil.User(t, db, roles...)
		var id int64
		if e := db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES($1,$2) RETURNING id`, "G2 sintetis "+testutil.Random(t)[:8], u.ID).Scan(&id); e != nil {
			t.Fatal(e)
		}
		users = append(users, u.ID)
		teachers = append(teachers, id)
		return u, id
	}
	teacher, teacherID := actor("guru_mapel", "kurikulum", "kesiswaan")
	other, _ := actor("guru_mapel")
	manager, managerTeacherID := actor("pegawai_tu", "pengelola_kehadiran_guru", "guru_mapel")
	plainTU, _ := actor("pegawai_tu")
	officer, _ := actor("petugas_absensi")
	admin, _ := actor("admin", "guru_mapel")
	t.Cleanup(func() {
		tx, e := db.Begin()
		if e != nil {
			t.Error(e)
			return
		}
		defer tx.Rollback()
		for _, id := range teachers {
			for _, q := range []string{
				`DELETE FROM app.teacher_attendance_correction_requests WHERE teacher_id=$1`,
				`DELETE FROM app.teacher_attendance_report_history WHERE report_id IN(SELECT id FROM app.teacher_attendance_reports WHERE teacher_id=$1)`,
				`DELETE FROM app.teacher_attendance_reports WHERE teacher_id=$1`,
				`DELETE FROM app.teacher_attendance_history WHERE event_id IN(SELECT id FROM app.teacher_attendance_events WHERE teacher_id=$1)`,
				`DELETE FROM app.teacher_attendance_events WHERE teacher_id=$1`,
				`DELETE FROM app.teachers WHERE id=$1`,
			} {
				if _, e = tx.Exec(q, id); e != nil {
					t.Error("fixture cleanup:", e)
					return
				}
			}
		}
		for _, id := range users {
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
	key := func() string {
		k, e := academic.NewTeacherAttendanceIdempotencyKey()
		if e != nil {
			t.Fatal(e)
		}
		return k
	}
	input := func(kind, start, end string) academic.TeacherAttendanceReportInput {
		return academic.TeacherAttendanceReportInput{Kind: kind, StartsOn: start, EndsOn: end, Description: "Laporan sintetis G2", IdempotencyKey: key()}
	}
	create := func(u auth.User, in academic.TeacherAttendanceReportInput) academic.TeacherAttendanceReport {
		r, e := s.CreateTeacherAttendanceReport(ctx, u.ID, in)
		if e != nil {
			t.Fatal("create report:", e)
		}
		return r
	}
	read := func(u auth.User, id int64, scope academic.TeacherReportScope) academic.TeacherAttendanceReport {
		r, e := s.TeacherAttendanceReportDetail(ctx, u.ID, id, scope, 1)
		if e != nil {
			t.Fatal("read report:", e)
		}
		return r
	}
	historyCount := func(id int64) int {
		var n int
		if e := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_report_history WHERE report_id=$1`, id).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	studentDigest := func() string {
		var digest string
		if e := db.QueryRow(`SELECT md5(coalesce((SELECT string_agg(row_to_json(t)::text,'' ORDER BY id) FROM app.student_daily_attendance t),'') || coalesce((SELECT string_agg(row_to_json(t)::text,'' ORDER BY id) FROM app.student_lesson_attendance t),''))`).Scan(&digest); e != nil {
			t.Fatal(e)
		}
		return digest
	}
	studentBefore := studentDigest()
	occurred := academic.TeacherAttendanceWIBTime(time.Now()).Add(-time.Minute).Format("2006-01-02T15:04")
	proof, e := s.RecordTeacherAttendanceManual(ctx, manager.ID, academic.TeacherAttendanceManualInput{TeacherID: teacherID, Action: academic.TeacherCheckIn, OccurredAt: occurred, Reason: "Fixture G2 existing event", IdempotencyKey: key()})
	if e != nil {
		t.Fatal(e)
	}
	eventDigest := func() string {
		var digest string
		if e := db.QueryRow(`SELECT md5(row_to_json(e)::text || (SELECT string_agg(row_to_json(h)::text,'' ORDER BY h.id) FROM app.teacher_attendance_history h WHERE h.event_id=e.id)) FROM app.teacher_attendance_events e WHERE e.id=$1`, proof.EventID).Scan(&digest); e != nil {
			t.Fatal(e)
		}
		return digest
	}
	eventBefore := eventDigest()
	firstInput := input("izin", "2000-01-01", "2000-01-01")
	first := create(teacher, firstInput)
	if first.TeacherID != teacherID || first.RecordedBy != teacher.ID || first.Version != 1 || !first.Active || first.RecordedAt.IsZero() || historyCount(first.ID) != 1 {
		t.Fatal("incorrect identity/time/history", first)
	}
	create(teacher, input("sakit", "2026-10-28", "2026-10-30"))
	create(teacher, input("dinas_luar", "2040-01-01", "2040-02-28"))
	retry, e := s.CreateTeacherAttendanceReport(ctx, teacher.ID, firstInput)
	if e != nil || retry.ID != first.ID || historyCount(first.ID) != 1 {
		t.Fatal("retry duplicated report", e)
	}
	changed := firstInput
	changed.Description = "Isi berbeda"
	if _, e = s.CreateTeacherAttendanceReport(ctx, teacher.ID, changed); !errors.Is(e, academic.ErrConflict) {
		t.Fatal("same key changed content", e)
	}
	if _, e = s.CreateTeacherAttendanceReport(ctx, teacher.ID, input("izin", "2026-10-30", "2026-11-01")); !errors.Is(e, academic.ErrTeacherReportOverlap) {
		t.Fatal("inclusive boundary overlap not rejected", e)
	}
	if _, e = s.TeacherAttendanceReportDetail(ctx, other.ID, first.ID, academic.TeacherReportOwn, 1); !errors.Is(e, academic.ErrNotFound) {
		t.Fatal("other teacher accessed report", e)
	}
	own := read(teacher, first.ID, academic.TeacherReportOwn)
	if own.CanManage {
		t.Fatal("owner can manage report")
	}
	correction := academic.TeacherAttendanceReportCorrection{ReportID: first.ID, ExpectedVersion: 1, Kind: "sakit", StartsOn: first.StartsOn, EndsOn: first.EndsOn, Description: "Keterangan diperbaiki", Reason: "Jenis yang benar adalah Sakit"}
	for _, u := range []auth.User{teacher, other, plainTU, officer, admin} {
		for _, cancel := range []bool{false, true} {
			attempt := correction
			attempt.Cancel = cancel
			if _, e = s.CorrectTeacherAttendanceReport(ctx, u.ID, attempt); !errors.Is(e, academic.ErrForbidden) {
				t.Fatalf("unauthorized actor %d cancel=%v error=%v", u.ID, cancel, e)
			}
		}
	}
	for _, u := range []auth.User{plainTU, officer} {
		if _, e = s.TeacherAttendanceReports(ctx, u.ID, academic.TeacherAttendanceReportFilter{Scope: academic.TeacherReportTU, Page: 1}); !errors.Is(e, academic.ErrForbidden) {
			t.Fatal("unauthorized list", e)
		}
	}
	self := create(manager, input("izin", "2041-01-01", "2041-01-01"))
	corSelf := correction
	corSelf.ReportID = self.ID
	corSelf.StartsOn = self.StartsOn
	corSelf.EndsOn = self.EndsOn
	if _, e = s.CorrectTeacherAttendanceReport(ctx, manager.ID, corSelf); !errors.Is(e, academic.ErrForbidden) {
		t.Fatal("TU self correction allowed", e)
	}
	if read(manager, self.ID, academic.TeacherReportTU).CanManage {
		t.Fatal("TU self UI scope can manage")
	}
	if managerTeacherID != self.TeacherID {
		t.Fatal("TU identity duplicated")
	}
	adminSelf := create(admin, input("izin", "2042-01-01", "2042-01-01"))
	adminCor := correction
	adminCor.ReportID = adminSelf.ID
	adminCor.StartsOn = adminSelf.StartsOn
	adminCor.EndsOn = adminSelf.EndsOn
	if _, e = s.CorrectTeacherAttendanceReport(ctx, admin.ID, adminCor); e != nil {
		t.Fatal("admin own fallback", e)
	}
	blank := correction
	blank.Reason = " \t "
	if _, e = s.CorrectTeacherAttendanceReport(ctx, manager.ID, blank); e == nil {
		t.Fatal("missing correction reason accepted")
	}
	if historyCount(first.ID) != 1 {
		t.Fatal("failed validation changed history")
	}
	corrected, e := s.CorrectTeacherAttendanceReport(ctx, manager.ID, correction)
	if e != nil || corrected.Version != 2 {
		t.Fatal("correct report", e)
	}
	if corrected.RecordedBy != first.RecordedBy || !corrected.RecordedAt.Equal(first.RecordedAt) {
		t.Fatal("correction replaced recording context")
	}
	detail := read(manager, first.ID, academic.TeacherReportTU)
	if len(detail.History) != 2 || detail.History[0].Before == nil || detail.History[0].Before.Kind != "izin" || detail.History[0].After.Kind != "sakit" || detail.History[0].Reason != correction.Reason {
		t.Fatal("before/after incomplete", detail.History)
	}
	if _, e = s.CorrectTeacherAttendanceReport(ctx, manager.ID, correction); !errors.Is(e, academic.ErrConflict) {
		t.Fatal("stale version accepted", e)
	}
	correction.ExpectedVersion = 2
	if _, e = s.CorrectTeacherAttendanceReport(ctx, manager.ID, correction); e != nil || historyCount(first.ID) != 2 {
		t.Fatal("no-op created history", e)
	}
	beforeRequest := historyCount(first.ID)
	if e = s.RequestTeacherAttendanceReportCorrection(ctx, teacher.ID, first.ID, "Mohon periksa keterangan laporan"); e != nil {
		t.Fatal(e)
	}
	if historyCount(first.ID) != beforeRequest || read(teacher, first.ID, academic.TeacherReportOwn).Version != 2 {
		t.Fatal("request mutated report")
	}
	inbox, e := s.TeacherAttendanceCorrectionRequests(ctx, manager.ID, 1)
	if e != nil || len(inbox) == 0 || inbox[0].ReportID != first.ID {
		t.Fatal("report request not in existing inbox", e)
	}
	if e = s.RequestTeacherAttendanceReportCorrection(ctx, other.ID, first.ID, "Tidak boleh melihat laporan orang lain"); !errors.Is(e, academic.ErrNotFound) {
		t.Fatal("request ownership bypass", e)
	}
	overlappingCorrection := correction
	overlappingCorrection.StartsOn = "2026-10-29"
	overlappingCorrection.EndsOn = "2026-10-30"
	if _, e = s.CorrectTeacherAttendanceReport(ctx, manager.ID, overlappingCorrection); !errors.Is(e, academic.ErrTeacherReportOverlap) {
		t.Fatal("correction overlap accepted", e)
	}
	if read(teacher, first.ID, academic.TeacherReportOwn).Version != 2 || historyCount(first.ID) != 2 {
		t.Fatal("failed overlap changed report")
	}
	// Pembatalan mempertahankan laporan dan membebaskan tanggal untuk laporan baru.
	cancel := academic.TeacherAttendanceReportCorrection{ReportID: first.ID, ExpectedVersion: 2, Cancel: true, Reason: "Laporan keliru dibatalkan"}
	cancelled, e := s.CorrectTeacherAttendanceReport(ctx, manager.ID, cancel)
	if e != nil || cancelled.Active || cancelled.Version != 3 || historyCount(first.ID) != 3 {
		t.Fatal("cancel report", e)
	}
	create(teacher, input("dinas_luar", first.StartsOn, first.EndsOn))
	retry, e = s.CreateTeacherAttendanceReport(ctx, teacher.ID, firstInput)
	if e != nil || retry.ID != first.ID || retry.Active {
		t.Fatal("retry resurrected cancelled report", e)
	}
	// Status/profil terbaru tidak menghilangkan hak baca sejarah pemilik.
	if _, e = db.Exec(`UPDATE app.teachers SET active=false WHERE id=$1`, teacherID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.TeacherAttendanceReportDetail(ctx, teacher.ID, first.ID, academic.TeacherReportOwn, 1); e != nil {
		t.Fatal("historical read after teacher inactive", e)
	}
	if _, e = s.CreateTeacherAttendanceReport(ctx, teacher.ID, input("izin", "2043-01-01", "2043-01-01")); !errors.Is(e, academic.ErrForbidden) {
		t.Fatal("inactive teacher creates report", e)
	}
	if _, e = db.Exec(`UPDATE app.teachers SET active=true WHERE id=$1`, teacherID); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1 AND role_code='guru_mapel'`, teacher.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateTeacherAttendanceReport(ctx, teacher.ID, input("izin", "2043-01-01", "2043-01-01")); !errors.Is(e, academic.ErrForbidden) {
		t.Fatal("revoked role can create", e)
	}
	if _, e = s.TeacherAttendanceReportDetail(ctx, teacher.ID, first.ID, academic.TeacherReportOwn, 1); !errors.Is(e, academic.ErrForbidden) {
		t.Fatal("akun tanpa role presensi guru yang aktif dapat membaca laporan sendiri", e)
	}
	if _, e = db.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'guru_mapel')`, teacher.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.TeacherAttendanceReportDetail(ctx, teacher.ID, first.ID, academic.TeacherReportOwn, 1); e != nil {
		t.Fatal("role presensi aktif tidak memulihkan pembacaan laporan sendiri", e)
	}
	if _, e = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, teacher.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.TeacherAttendanceReportDetail(ctx, teacher.ID, first.ID, academic.TeacherReportOwn, 1); !errors.Is(e, academic.ErrForbidden) {
		t.Fatal("inactive account can read", e)
	}
	if _, e = db.Exec(`UPDATE app.users SET active=true WHERE id=$1`, teacher.ID); e != nil {
		t.Fatal(e)
	}
	// Dua formulir berbeda pada rentang yang sama: hanya satu laporan aktif.
	concurrent := input("sakit", "2050-01-01", "2050-01-03")
	second := concurrent
	second.Kind = "izin"
	second.IdempotencyKey = key()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, in := range []academic.TeacherAttendanceReportInput{concurrent, second} {
		wg.Add(1)
		go func(in academic.TeacherAttendanceReportInput) {
			defer wg.Done()
			_, e := s.CreateTeacherAttendanceReport(ctx, teacher.ID, in)
			errs <- e
		}(in)
	}
	wg.Wait()
	close(errs)
	passed, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			passed++
		} else if errors.Is(e, academic.ErrTeacherReportOverlap) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if passed != 1 || conflicts != 1 {
		t.Fatal("concurrent overlap", passed, conflicts)
	}
	identical := input("izin", "2051-01-01", "2051-01-02")
	ids := make(chan int64, 2)
	errs = make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.CreateTeacherAttendanceReport(ctx, teacher.ID, identical)
			ids <- r.ID
			errs <- e
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	var identicalID int64
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	for id := range ids {
		if identicalID != 0 && identicalID != id {
			t.Fatal("retry IDs differ")
		}
		identicalID = id
	}
	if historyCount(identicalID) != 1 {
		t.Fatal("retry history duplicated")
	}
	concurrentCorrection := academic.TeacherAttendanceReportCorrection{ReportID: identicalID, ExpectedVersion: 1, Kind: "sakit", StartsOn: identical.StartsOn, EndsOn: identical.EndsOn, Description: "Koreksi bersamaan", Reason: "Perbaikan sintetis"}
	errs = make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.CorrectTeacherAttendanceReport(ctx, manager.ID, concurrentCorrection)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	passed, conflicts = 0, 0
	for e := range errs {
		if e == nil {
			passed++
		} else if errors.Is(e, academic.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if passed != 1 || conflicts != 1 || historyCount(identicalID) != 2 {
		t.Fatal("lost update", passed, conflicts)
	}
	// Constraint melindungi jalur SQL langsung, termasuk overlap dari tanggal batas.
	hash := sha256.Sum256([]byte(testutil.Random(t)))
	directKey := hex.EncodeToString(hash[:])
	_, e = db.Exec(`INSERT INTO app.teacher_attendance_reports(teacher_id,teacher_name,kind,starts_on,ends_on,description,recorded_by,request_key_hash,creation_fingerprint) VALUES($1,'Fixture','izin','2051-01-02','2051-01-03','Fixture',$2,$3,$3)`, teacherID, teacher.ID, directKey)
	if e == nil {
		t.Fatal("database accepted direct overlap")
	}
	// Kegagalan histori membatalkan update laporan.
	rollback := create(teacher, input("izin", "2060-01-01", "2060-01-01"))
	suffix := testutil.Random(t)[:12]
	trigger := "g2_history_" + suffix
	sql := fmt.Sprintf(`CREATE FUNCTION app.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.report_id=%d AND NEW.action='corrected' THEN RAISE EXCEPTION 'synthetic report history failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON app.teacher_attendance_report_history FOR EACH ROW EXECUTE FUNCTION app.%s()`, trigger, rollback.ID, trigger, trigger)
	if _, e = db.Exec(sql); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, e := db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.teacher_attendance_report_history; DROP FUNCTION IF EXISTS app.%s()`, trigger, trigger))
		if e != nil {
			t.Error(e)
		}
	})
	failed := concurrentCorrection
	failed.ReportID = rollback.ID
	failed.StartsOn = rollback.StartsOn
	failed.EndsOn = rollback.EndsOn
	if _, e = s.CorrectTeacherAttendanceReport(ctx, manager.ID, failed); e == nil {
		t.Fatal("history failure accepted")
	}
	if read(teacher, rollback.ID, academic.TeacherReportOwn).Version != 1 || historyCount(rollback.ID) != 1 {
		t.Fatal("history failure did not roll back report")
	}
	if eventDigest() != eventBefore || studentDigest() != studentBefore {
		t.Fatal("report operations mutated attendance evidence")
	}
}

func TestTeacherAttendanceReportLongNameIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := &academic.Store{DB: db}
	// Role Petugas tidak menggandakan identitas atau menghapus hak pribadi Guru Mapel.
	u, teacherID := captureTeacher(t, db, "guru_mapel", "petugas_absensi")
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM app.teacher_attendance_correction_requests WHERE teacher_id=$1`, `DELETE FROM app.teacher_attendance_report_history WHERE report_id IN(SELECT id FROM app.teacher_attendance_reports WHERE teacher_id=$1)`, `DELETE FROM app.teacher_attendance_reports WHERE teacher_id=$1`, `DELETE FROM app.teachers WHERE id=$1`} {
			if _, e := db.Exec(q, teacherID); e != nil {
				t.Error(e)
				return
			}
		}
		if _, e := db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1`, u.ID); e != nil {
			t.Error(e)
		}
		if _, e := db.Exec(`DELETE FROM app.users WHERE id=$1`, u.ID); e != nil {
			t.Error(e)
		}
	})
	name := strings.Repeat("é", 200)
	if _, e := db.Exec(`UPDATE app.teachers SET name=$1 WHERE id=$2`, name, teacherID); e != nil {
		t.Fatal(e)
	}
	key, _ := academic.NewTeacherAttendanceIdempotencyKey()
	r, e := s.CreateTeacherAttendanceReport(ctx, u.ID, academic.TeacherAttendanceReportInput{Kind: "izin", StartsOn: "2040-10-01", EndsOn: "2040-10-01", Description: "Nama panjang yang sah", IdempotencyKey: key})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RequestTeacherAttendanceReportCorrection(ctx, u.ID, r.ID, "Mohon periksa keterangan"); e != nil {
		t.Fatal(e)
	}
	var snapshot string
	if e = db.QueryRow(`SELECT teacher_name FROM app.teacher_attendance_correction_requests WHERE report_id=$1`, r.ID).Scan(&snapshot); e != nil || snapshot != name {
		t.Fatal("name snapshot truncated", e)
	}
	if _, e = s.CorrectTeacherAttendanceReport(ctx, u.ID, academic.TeacherAttendanceReportCorrection{ReportID: r.ID, ExpectedVersion: 1, Cancel: true, Reason: "Petugas tidak mengelola laporan guru"}); !errors.Is(e, academic.ErrForbidden) {
		t.Fatal("officer personal role expanded management", e)
	}
}
