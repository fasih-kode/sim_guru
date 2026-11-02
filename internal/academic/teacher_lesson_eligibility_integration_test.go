//go:build integration

package academic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherLessonEligibilityUsesCorrectableAttendanceEvidenceIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	teacher, _ := testutil.User(t, db, "admin", "guru_mapel")
	var teacherID int64
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru kelayakan jurnal',$1) RETURNING id`, teacher.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	var date string
	if err := db.QueryRow(`SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&date); err != nil {
		t.Fatal(err)
	}
	partialUser, _ := testutil.User(t, db, "admin", "guru_mapel")
	var partialTeacherID int64
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru presensi parsial sintetis',$1) RETURNING id`, partialUser.ID).Scan(&partialTeacherID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		tx, err := db.Begin()
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback()
		for _, query := range []string{
			`DELETE FROM app.teacher_attendance_report_history WHERE report_id IN(SELECT id FROM app.teacher_attendance_reports WHERE teacher_id=$1)`,
			`DELETE FROM app.teacher_attendance_reports WHERE teacher_id=$1`,
			`DELETE FROM app.teacher_attendance_history WHERE event_id IN(SELECT id FROM app.teacher_attendance_events WHERE teacher_id=$1)`,
			`DELETE FROM app.teacher_attendance_events WHERE teacher_id=$1`,
		} {
			if _, err = tx.Exec(query, teacherID); err != nil {
				t.Error("cleanup synthetic teacher attendance evidence:", err)
				return
			}
		}
		if _, err = tx.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID); err != nil {
			t.Error("cleanup synthetic teacher identity:", err)
			return
		}
		for _, query := range []string{
			`DELETE FROM app.teacher_attendance_report_history WHERE report_id IN(SELECT id FROM app.teacher_attendance_reports WHERE teacher_id=$1)`,
			`DELETE FROM app.teacher_attendance_reports WHERE teacher_id=$1`,
			`DELETE FROM app.teacher_attendance_history WHERE event_id IN(SELECT id FROM app.teacher_attendance_events WHERE teacher_id=$1)`,
			`DELETE FROM app.teacher_attendance_events WHERE teacher_id=$1`,
			`DELETE FROM app.teachers WHERE id=$1`,
		} {
			if _, err = tx.Exec(query, partialTeacherID); err != nil {
				t.Error("cleanup partial synthetic teacher attendance evidence:", err)
				return
			}
		}
		if _, err = tx.Exec(`DELETE FROM app.user_roles WHERE user_id=$1`, teacher.ID); err != nil {
			t.Error(err)
			return
		}
		if _, err = tx.Exec(`DELETE FROM app.users WHERE id=$1`, teacher.ID); err != nil {
			t.Error(err)
			return
		}
		if _, err = tx.Exec(`DELETE FROM app.user_roles WHERE user_id=$1`, partialUser.ID); err != nil {
			t.Error(err)
			return
		}
		if _, err = tx.Exec(`DELETE FROM app.users WHERE id=$1`, partialUser.ID); err != nil {
			t.Error(err)
			return
		}
		if err = tx.Commit(); err != nil {
			t.Error(err)
		}
	})

	checkIn, err := s.RecordTeacherAttendanceManual(ctx, teacher.ID, TeacherAttendanceManualInput{
		TeacherID: teacherID, Action: TeacherCheckIn, OccurredAt: date + "T00:01", Reason: "Bukti sintetis uji kelayakan", IdempotencyKey: testutil.Random(t)[:32],
	})
	if err != nil {
		t.Fatal(err)
	}
	pastDate := time.Now().In(teacherAttendanceZone).AddDate(0, 0, -1).Format("2006-01-02")
	noCheckIn, err := s.TeacherLessonEligibilityForDate(ctx, partialUser.ID, pastDate, "00:03")
	if err != nil || noCheckIn.Allowed || !containsText(noCheckIn.Reason, "Masuk") {
		t.Fatalf("past date without Check-In should be blocked: %#v %v", noCheckIn, err)
	}
	if _, err = s.RecordTeacherAttendanceManual(ctx, partialUser.ID, TeacherAttendanceManualInput{TeacherID: partialTeacherID, Action: TeacherCheckIn, OccurredAt: pastDate + "T00:01", Reason: "Bukti sintetis uji tanggal lampau", IdempotencyKey: testutil.Random(t)[:32]}); err != nil {
		t.Fatal(err)
	}
	partialDate, err := s.TeacherLessonEligibilityForDate(ctx, partialUser.ID, pastDate, "00:03")
	if err != nil || partialDate.Allowed || !containsText(partialDate.Reason, "Pulang") {
		t.Fatalf("past date with Check-In only should be blocked: %#v %v", partialDate, err)
	}
	if _, err = s.RecordTeacherAttendanceManual(ctx, partialUser.ID, TeacherAttendanceManualInput{TeacherID: partialTeacherID, Action: TeacherCheckOut, OccurredAt: pastDate + "T00:04", Reason: "Bukti sintetis uji tanggal lampau", IdempotencyKey: testutil.Random(t)[:32]}); err != nil {
		t.Fatal(err)
	}
	completePast, err := s.TeacherLessonEligibilityForDate(ctx, partialUser.ID, pastDate, "00:03")
	if err != nil || !completePast.Allowed {
		t.Fatalf("complete past Check-In/Check-Out should allow: %#v %v", completePast, err)
	}
	eligibility, err := s.TeacherLessonEligibilityForDate(ctx, teacher.ID, date, "00:03")
	if err != nil || !eligibility.Allowed {
		t.Fatalf("valid check-in on the current WIB date should allow entry: %#v %v", eligibility, err)
	}
	noIdentity, _ := testutil.User(t, db, "guru_mapel")
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1`, noIdentity.ID); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(`DELETE FROM app.users WHERE id=$1`, noIdentity.ID); err != nil {
			t.Error(err)
		}
	})
	if _, err = s.TeacherLessonEligibilityForDate(ctx, noIdentity.ID, date, "00:03"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("account without an active teacher identity unexpectedly received eligibility: %v", err)
	}

	checkOut, err := s.RecordTeacherAttendanceManual(ctx, teacher.ID, TeacherAttendanceManualInput{
		TeacherID: teacherID, Action: TeacherCheckOut, OccurredAt: date + "T00:02", Reason: "Bukti sintetis uji kelayakan", IdempotencyKey: testutil.Random(t)[:32],
	})
	if err != nil {
		t.Fatal(err)
	}
	eligibility, err = s.TeacherLessonEligibilityForDate(ctx, teacher.ID, date, "00:03")
	if err != nil || eligibility.Allowed || !containsText(eligibility.Reason, "mendahului waktu selesai pertemuan") {
		t.Fatalf("early check-out should block only the affected occurrence: %#v %v", eligibility, err)
	}
	if _, err = s.CorrectTeacherAttendance(ctx, teacher.ID, TeacherAttendanceCorrectionInput{
		EventID: checkOut.EventID, ExpectedVersion: checkOut.Version, OccurredAt: date + "T00:04", CorrectionReason: "Koreksi bukti sintetis resmi",
	}); err != nil {
		t.Fatal(err)
	}
	eligibility, err = s.TeacherLessonEligibilityForDate(ctx, teacher.ID, date, "00:03")
	if err != nil || !eligibility.Allowed {
		t.Fatalf("official correction completing attendance should reopen access: %#v %v", eligibility, err)
	}

	for _, reportCase := range []struct{ kind, expected string }{{"izin", "Laporan Izin aktif"}, {"sakit", "Laporan Sakit aktif"}, {"dinas_luar", "Dinas Luar aktif"}} {
		report, reportErr := s.CreateTeacherAttendanceReport(ctx, teacher.ID, TeacherAttendanceReportInput{
			Kind: reportCase.kind, StartsOn: date, EndsOn: date, Description: "Laporan sintetis untuk uji gate", IdempotencyKey: testutil.Random(t)[:32],
		})
		if reportErr != nil {
			t.Fatal(reportErr)
		}
		eligibility, err = s.TeacherLessonEligibilityForDate(ctx, teacher.ID, date, "00:03")
		if err != nil || eligibility.Allowed || !containsText(eligibility.Reason, reportCase.expected) {
			t.Fatalf("active %s report should block access: %#v %v", reportCase.kind, eligibility, err)
		}
		if _, err = s.CorrectTeacherAttendanceReport(ctx, teacher.ID, TeacherAttendanceReportCorrection{ReportID: report.ID, ExpectedVersion: report.Version, Reason: "Laporan sintetis diselesaikan secara resmi", Cancel: true}); err != nil {
			t.Fatal(err)
		}
		eligibility, err = s.TeacherLessonEligibilityForDate(ctx, teacher.ID, date, "00:03")
		if err != nil || !eligibility.Allowed {
			t.Fatalf("official %s correction should reopen access automatically: %#v %v", reportCase.kind, eligibility, err)
		}
	}

	if _, err = s.CorrectTeacherAttendance(ctx, teacher.ID, TeacherAttendanceCorrectionInput{
		EventID: checkOut.EventID, ExpectedVersion: checkOut.Version + 1, OccurredAt: date + "T00:02", CorrectionReason: "Uji pencabutan kelayakan",
	}); err != nil {
		t.Fatal(err)
	}
	eligibility, err = s.TeacherLessonEligibilityForDate(ctx, teacher.ID, date, "00:03")
	if err != nil || eligibility.Allowed {
		t.Fatalf("a later official correction that revokes eligibility was not reflected: %#v %v", eligibility, err)
	}
	var eventHistory int
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_history WHERE event_id=$1`, checkIn.EventID).Scan(&eventHistory); err != nil || eventHistory < 1 {
		t.Fatalf("attendance evidence history was not preserved: rows=%d err=%v", eventHistory, err)
	}
}

func containsText(value, expected string) bool {
	return strings.Contains(value, expected)
}
