//go:build integration

package academic_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherAttendanceCorrectionRequestDoesNotMutateAttendanceIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	service := &academic.Store{DB: db}
	teacher, teacherID := captureTeacher(t, db, "guru_mapel")
	manager, _ := testutil.User(t, db, "pegawai_tu", "pengelola_kehadiran_guru")
	petugas, _ := testutil.User(t, db, "petugas_absensi")
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.teacher_attendance_correction_requests WHERE actor_id=$1`, teacher.ID)
		_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID)
		_, _ = db.Exec(`DELETE FROM app.user_roles WHERE user_id IN($1,$2,$3)`, teacher.ID, manager.ID, petugas.ID)
		_, _ = db.Exec(`DELETE FROM app.users WHERE id IN($1,$2,$3)`, teacher.ID, manager.ID, petugas.ID)
	})
	onDate := manualAttendanceDateDaysBefore(t, manualAttendanceToday(t, db), 400)
	beforeEvents, beforeHistory := attendanceEventHistoryCounts(t, db, teacherID)
	input := academic.TeacherAttendanceCorrectionRequestInput{OnDate: onDate, Action: academic.TeacherCheckIn, Kind: "correction", RequestText: "Waktu Masuk perlu dilengkapi dari catatan sekolah."}
	if err := service.RequestTeacherAttendanceCorrection(ctx, teacher.ID, input); err != nil {
		t.Fatalf("teacher correction request failed: %v", err)
	}
	afterEvents, afterHistory := attendanceEventHistoryCounts(t, db, teacherID)
	if beforeEvents != afterEvents || beforeHistory != afterHistory {
		t.Fatalf("request changed attendance data: events %d->%d, history %d->%d", beforeEvents, afterEvents, beforeHistory, afterHistory)
	}
	requests, err := service.TeacherAttendanceCorrectionRequests(ctx, manager.ID, 1)
	if err != nil || len(requests) == 0 || requests[0].OnDate != onDate || requests[0].Teacher == "" || requests[0].Text != input.RequestText {
		t.Fatalf("request inbox did not show submitted request: rows=%+v err=%v", requests, err)
	}
	if _, err = service.TeacherAttendanceCorrectionRequests(ctx, petugas.ID, 1); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("student attendance officer inbox error=%v, want forbidden", err)
	}
}

func TestTeacherAttendanceManagerListsAllTeachersAndCannotManageSelfIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	service := &academic.Store{DB: db}
	manager, _ := testutil.User(t, db, "pegawai_tu", "pengelola_kehadiran_guru")
	managerTeacherID := createManualTeacher(t, db, "DEV03G1 profil pengelola")
	targetTeacherID := createManualTeacher(t, db, "DEV03G1 guru yang dikelola")
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.teachers WHERE id IN($1,$2)`, managerTeacherID, targetTeacherID)
		_, _ = db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1`, manager.ID)
		_, _ = db.Exec(`DELETE FROM app.users WHERE id=$1`, manager.ID)
	})
	if _, err := db.Exec(`UPDATE app.teachers SET user_id=$1 WHERE id=$2`, manager.ID, managerTeacherID); err != nil {
		t.Fatal("link manager teacher profile:", err)
	}
	onDate := manualAttendanceToday(t, db)

	rows, err := service.TeacherAttendanceManaged(ctx, manager.ID, onDate, "DEV03G1 guru yang dikelola", 1)
	if err != nil || len(rows) != 1 || rows[0].ID != targetTeacherID || !rows[0].CanManage {
		t.Fatalf("manager could not list an ordinary teacher: rows=%+v err=%v", rows, err)
	}

	rows, err = service.TeacherAttendanceManaged(ctx, manager.ID, onDate, "DEV03G1 profil pengelola", 1)
	if err != nil || len(rows) != 1 || rows[0].ID != managerTeacherID || !rows[0].IsActor || rows[0].CanManage {
		t.Fatalf("manager self row should be visible but not manageable: rows=%+v err=%v", rows, err)
	}
}

func TestTeacherAttendanceAdminFallbackIsOwnTURecordOnlyIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	service := &academic.Store{DB: db}
	admin, _ := testutil.User(t, db, "admin")
	manager, _ := testutil.User(t, db, "pegawai_tu", "pengelola_kehadiran_guru")
	teacherID := createManualTeacher(t, db, "Admin pegawai TU")
	managerTeacherID := createManualTeacher(t, db, "Pengelola TU untuk uji diri sendiri")
	recorder, _ := testutil.User(t, db, "pegawai_tu", "pengelola_kehadiran_guru")
	photos := newCapturePhotoStore(t)
	var eventIDs []int64
	t.Cleanup(func() {
		cleanupTeacherAttendanceManualEvents(t, db, photos, eventIDs, []int64{teacherID, managerTeacherID})
		_, _ = db.Exec(`DELETE FROM app.user_roles WHERE user_id IN($1,$2,$3)`, admin.ID, manager.ID, recorder.ID)
		_, _ = db.Exec(`DELETE FROM app.users WHERE id IN($1,$2,$3)`, admin.ID, manager.ID, recorder.ID)
	})
	if _, err := db.Exec(`UPDATE app.teachers SET user_id=$1 WHERE id=$2`, admin.ID, teacherID); err != nil {
		t.Fatal("link admin teacher profile:", err)
	}
	if _, err := db.Exec(`UPDATE app.teachers SET user_id=$1 WHERE id=$2`, manager.ID, managerTeacherID); err != nil {
		t.Fatal("link TU teacher profile:", err)
	}
	date := manualAttendanceToday(t, db)
	occurred := teacherAttendanceCorrectionTime(t, db, 10)
	input := manualAttendanceInput(t, 801, academic.TeacherCheckIn, occurred, "Catatan dibuat oleh pengelola TU")
	input = inputForTeacher(input, teacherID)
	proof, err := service.RecordTeacherAttendanceManual(ctx, manager.ID, input)
	if err != nil {
		t.Fatalf("manager failed to create synthetic event: %v", err)
	}
	eventIDs = append(eventIDs, proof.EventID)
	checkoutTime := teacherAttendanceCorrectionTime(t, db, 8)
	checkoutInput := inputForTeacher(manualAttendanceInput(t, 802, academic.TeacherCheckOut, checkoutTime, "Pulang TU tercatat lewat waktu"), teacherID)
	checkoutProof, err := service.RecordTeacherAttendanceManual(ctx, manager.ID, checkoutInput)
	if err != nil {
		t.Fatalf("manager failed to create synthetic check-out: %v", err)
	}
	eventIDs = append(eventIDs, checkoutProof.EventID)
	rows, err := service.TeacherAttendanceManagedForAdmin(ctx, admin.ID, date, "", 1)
	if err != nil || len(rows) != 1 || rows[0].ID != teacherID || !rows[0].IsActor || !rows[0].CanManage || rows[0].CheckIn == nil || rows[0].CheckOut == nil {
		t.Fatalf("admin fallback list=%+v err=%v", rows, err)
	}
	record, err := service.TeacherAttendanceManagedRecordDetailForAdmin(ctx, admin.ID, proof.EventID)
	if err != nil || !record.CanCorrect || record.RecordedByName == "" {
		t.Fatalf("admin self detail=%+v err=%v", record, err)
	}
	if len(record.History) != 1 || record.History[0].AfterReason != input.Reason {
		t.Fatalf("initial manual evidence reason is missing from history: %+v", record.History)
	}
	selfTime := teacherAttendanceCorrectionTime(t, db, 7)
	selfInput := inputForTeacher(manualAttendanceInput(t, 803, academic.TeacherCheckIn, selfTime, "Dicatat oleh pengelola lain"), managerTeacherID)
	selfProof, err := service.RecordTeacherAttendanceManual(ctx, recorder.ID, selfInput)
	if err != nil {
		t.Fatalf("another TU manager failed to create the self-test fixture: %v", err)
	}
	eventIDs = append(eventIDs, selfProof.EventID)
	selfRecord, err := service.TeacherAttendanceManagedRecordDetail(ctx, manager.ID, selfProof.EventID)
	if err != nil || selfRecord.CanCorrect {
		t.Fatalf("TU self record should remain readable but not correctable: record=%+v err=%v", selfRecord, err)
	}
	if _, err = service.CorrectTeacherAttendance(ctx, manager.ID, academic.TeacherAttendanceCorrectionInput{EventID: selfProof.EventID, ExpectedVersion: 1, OccurredAt: selfTime, CorrectionReason: "Tidak boleh mengoreksi catatan sendiri"}); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("TU self correction error=%v, want forbidden", err)
	}
	correctedAt := teacherAttendanceCorrectionTime(t, db, 9)
	result, err := service.CorrectTeacherAttendance(ctx, admin.ID, academic.TeacherAttendanceCorrectionInput{EventID: proof.EventID, ExpectedVersion: 1, OccurredAt: correctedAt, CorrectionReason: "Admin menangani catatan kehadiran dirinya sebagai pegawai TU"})
	if err != nil || result.CorrectedBy != admin.ID || result.Version != 2 {
		t.Fatalf("admin self fallback correction=%+v err=%v", result, err)
	}
	if _, err = service.TeacherAttendanceManagedRecordDetailForAdmin(ctx, admin.ID, proof.EventID+999999); !errors.Is(err, academic.ErrNotFound) {
		t.Fatalf("admin could inspect unrelated event id: %v", err)
	}
}

func attendanceEventHistoryCounts(t *testing.T, db *sql.DB, teacherID int64) (int, int) {
	t.Helper()
	var events, history int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1),(SELECT count(*) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1)`, teacherID).Scan(&events, &history); err != nil {
		t.Fatal("count synthetic attendance data:", err)
	}
	return events, history
}
