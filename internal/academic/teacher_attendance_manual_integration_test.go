//go:build integration

package academic_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestRecordTeacherAttendanceManualIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	service := &academic.Store{DB: db}
	petugas, _ := testutil.User(t, db, "petugas_absensi")
	photos := newCapturePhotoStore(t)
	var eventIDs []int64
	var teacherIDs []int64
	t.Cleanup(func() {
		cleanupTeacherAttendanceManualEvents(t, db, photos, eventIDs, teacherIDs)
	})
	studentDailyBefore, studentLessonBefore := attendanceStudentCounts(t, db)
	date := manualAttendanceToday(t, db)
	input := manualAttendanceInput(t, 101, academic.TeacherCheckIn, date+"T00:00", "Kamera guru bermasalah")

	// Petugas dapat mencatat guru aktif yang tidak mempunyai akun pengguna.
	teacherID := createManualTeacher(t, db, "Guru manual tanpa akun")
	teacherIDs = append(teacherIDs, teacherID)
	first, err := service.RecordTeacherAttendanceManual(ctx, petugas.ID, inputForTeacher(input, teacherID))
	if err != nil {
		t.Fatalf("manual check-in for an unlinked active teacher failed: %v", err)
	}
	eventIDs = append(eventIDs, first.EventID)
	if first.TeacherID != teacherID || first.Action != academic.TeacherCheckIn || first.Idempotent || first.Version != 1 {
		t.Fatalf("unexpected manual proof: %+v", first)
	}
	if first.OccurredAt.Equal(first.RecordedAt) || first.OnDate != date || !strings.HasPrefix(first.TeacherName, "Guru manual tanpa akun ") {
		t.Fatalf("manual timestamps or teacher snapshot are incorrect: %+v", first)
	}
	assertManualStoredEvidence(t, db, first, input.Reason, input.IdempotencyKey)
	assertManualCounts(t, db, teacherID, date, academic.TeacherCheckIn, 1, 1)

	// Retry identik mengembalikan snapshot lama tanpa histori baru.
	retry, err := service.RecordTeacherAttendanceManual(ctx, petugas.ID, inputForTeacher(input, teacherID))
	if err != nil || !retry.Idempotent || retry.EventID != first.EventID || !retry.OccurredAt.Equal(first.OccurredAt) || !retry.RecordedAt.Equal(first.RecordedAt) || retry.Reason != first.Reason {
		t.Fatalf("identical manual retry mismatch: proof=%+v err=%v", retry, err)
	}
	assertManualCounts(t, db, teacherID, date, academic.TeacherCheckIn, 1, 1)
	if _, err = db.Exec(`UPDATE app.teachers SET name='Nama master berubah' WHERE id=$1`, teacherID); err != nil {
		t.Fatal("change teacher master name")
	}
	retry, err = service.RecordTeacherAttendanceManual(ctx, petugas.ID, inputForTeacher(input, teacherID))
	if err != nil || retry.TeacherName != first.TeacherName {
		t.Fatalf("retry changed the captured teacher name: proof=%+v err=%v", retry, err)
	}

	// Isi berbeda dengan kunci sama dan bukti identik dengan kunci baru harus konflik tanpa menulis.
	changedReason := inputForTeacher(input, teacherID)
	changedReason.Reason = "Alasan berbeda"
	assertManualRejectedWithoutWrites(t, db, service, teacherID, date, academic.TeacherCheckIn, changedReason, academic.ErrConflict, petugas.ID)
	changedTime := inputForTeacher(input, teacherID)
	changedTime.OccurredAt = date + "T00:01"
	assertManualRejectedWithoutWrites(t, db, service, teacherID, date, academic.TeacherCheckIn, changedTime, academic.ErrConflict, petugas.ID)
	newKey := inputForTeacher(input, teacherID)
	newKey.IdempotencyKey, err = academic.NewTeacherAttendanceIdempotencyKey()
	if err != nil {
		t.Fatal(err)
	}
	assertManualRejectedWithoutWrites(t, db, service, teacherID, date, academic.TeacherCheckIn, newKey, academic.ErrConflict, petugas.ID)

	// Retry atas bukti yang sah tetap mengembalikan event lama setelah jendela susulan lewat.
	oldDate := manualAttendanceDateDaysBefore(t, date, 8)
	oldOccurredAt := manualAttendanceTime(oldDate + "T00:00")
	setManualEventDateForRetryTest(t, db, first.EventID, oldDate, oldOccurredAt, petugas.ID, teacherID, first.TeacherName, input.Action, input.Reason)
	oldInput := inputForTeacher(input, teacherID)
	oldInput.OccurredAt = oldDate + "T00:00"
	oldRetry, err := service.RecordTeacherAttendanceManual(ctx, petugas.ID, oldInput)
	if err != nil || !oldRetry.Idempotent || oldRetry.EventID != first.EventID || oldRetry.OnDate != oldDate || !oldRetry.RecordedAt.Equal(first.RecordedAt) {
		t.Fatalf("retry outside the makeup window did not return the original proof: proof=%+v err=%v", oldRetry, err)
	}
	assertManualCounts(t, db, teacherID, oldDate, academic.TeacherCheckIn, 1, 1)
	assertManualCounts(t, db, teacherID, date, academic.TeacherCheckIn, 0, 0)
	oldNewKey := oldInput
	oldNewKey.IdempotencyKey, err = academic.NewTeacherAttendanceIdempotencyKey()
	if err != nil {
		t.Fatal(err)
	}
	assertManualRejectedWithoutWrites(t, db, service, teacherID, oldDate, academic.TeacherCheckIn, oldNewKey, academic.ErrConflict, petugas.ID)
	newTeacherOutsideWindow := createManualTeacher(t, db, "Guru tanggal lama")
	teacherIDs = append(teacherIDs, newTeacherOutsideWindow)
	newAttemptOutsideWindow := inputForTeacher(oldInput, newTeacherOutsideWindow)
	assertManualRejectedWithoutWrites(t, db, service, newTeacherOutsideWindow, oldDate, academic.TeacherCheckIn, newAttemptOutsideWindow, nil, petugas.ID)

	// Pulang tanpa Masuk diizinkan; Masuk dan Pulang pada tanggal yang sama juga sah.
	checkoutTeacherID := createManualTeacher(t, db, "Guru manual pulang")
	teacherIDs = append(teacherIDs, checkoutTeacherID)
	checkout := manualAttendanceInput(t, 102, academic.TeacherCheckOut, date+"T00:00", "Pulang dicatat oleh petugas")
	checkoutProof, err := service.RecordTeacherAttendanceManual(ctx, petugas.ID, inputForTeacher(checkout, checkoutTeacherID))
	if err != nil || checkoutProof.Action != academic.TeacherCheckOut {
		t.Fatalf("checkout without check-in failed: proof=%+v err=%v", checkoutProof, err)
	}
	eventIDs = append(eventIDs, checkoutProof.EventID)
	checkin := manualAttendanceInput(t, 103, academic.TeacherCheckIn, date+"T00:00", "Masuk dicatat oleh petugas")
	checkinProof, err := service.RecordTeacherAttendanceManual(ctx, petugas.ID, inputForTeacher(checkin, checkoutTeacherID))
	if err != nil {
		t.Fatalf("check-in on the same date as checkout failed: %v", err)
	}
	eventIDs = append(eventIDs, checkinProof.EventID)
	assertManualCounts(t, db, checkoutTeacherID, date, academic.TeacherCheckIn, 1, 1)
	assertManualCounts(t, db, checkoutTeacherID, date, academic.TeacherCheckOut, 1, 1)

	// Role gabungan hanya berhasil melalui role Petugas Absensi.
	combined, _ := testutil.User(t, db, "petugas_absensi", "guru_mapel")
	combinedTeacher := createManualTeacher(t, db, "Guru untuk petugas role gabungan")
	teacherIDs = append(teacherIDs, combinedTeacher)
	combinedInput := manualAttendanceInput(t, 104, academic.TeacherCheckIn, date+"T00:00", "Dicatat petugas role gabungan")
	combinedProof, err := service.RecordTeacherAttendanceManual(ctx, combined.ID, inputForTeacher(combinedInput, combinedTeacher))
	if err != nil {
		t.Fatalf("active combined Petugas role was denied: %v", err)
	}
	eventIDs = append(eventIDs, combinedProof.EventID)

	// Role lain dan akun Petugas nonaktif tidak boleh membuat catatan.
	deniedTeacher := createManualTeacher(t, db, "Guru untuk otorisasi")
	teacherIDs = append(teacherIDs, deniedTeacher)
	deniedInput := manualAttendanceInput(t, 105, academic.TeacherCheckIn, date+"T00:00", "Uji penolakan role")
	for _, role := range []string{"admin", "kurikulum", "kesiswaan", "kepala_sekolah", "guru_mapel", "wali_kelas"} {
		user, _ := testutil.User(t, db, role)
		assertManualRejectedWithoutWrites(t, db, service, deniedTeacher, date, academic.TeacherCheckIn, inputForTeacher(deniedInput, deniedTeacher), academic.ErrForbidden, user.ID)
	}
	inactivePetugas, _ := testutil.User(t, db, "petugas_absensi")
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, inactivePetugas.ID); err != nil {
		t.Fatal("deactivate synthetic Petugas account")
	}
	assertManualRejectedWithoutWrites(t, db, service, deniedTeacher, date, academic.TeacherCheckIn, inputForTeacher(deniedInput, deniedTeacher), academic.ErrForbidden, inactivePetugas.ID)

	// Guru dengan akun aktif dapat dicatat, sedangkan guru nonaktif ditolak.
	_, linkedTeacherID := captureTeacher(t, db, "guru_mapel")
	teacherIDs = append(teacherIDs, linkedTeacherID)
	linkedInput := manualAttendanceInput(t, 106, academic.TeacherCheckIn, date+"T00:00", "Guru tertaut dicatat manual")
	linkedProof, err := service.RecordTeacherAttendanceManual(ctx, petugas.ID, inputForTeacher(linkedInput, linkedTeacherID))
	if err != nil || linkedProof.TeacherID != linkedTeacherID {
		t.Fatalf("active linked teacher was not recordable: proof=%+v err=%v", linkedProof, err)
	}
	eventIDs = append(eventIDs, linkedProof.EventID)
	if _, err = db.Exec(`UPDATE app.teachers SET active=false WHERE id=$1`, linkedTeacherID); err != nil {
		t.Fatal("deactivate synthetic teacher")
	}
	assertManualRejectedWithoutWrites(t, db, service, linkedTeacherID, date, academic.TeacherCheckOut,
		inputForTeacher(manualAttendanceInput(t, 107, academic.TeacherCheckOut, date+"T00:00", "Guru nonaktif"), linkedTeacherID), academic.ErrNotFound)

	// Dua submit simultan dengan kunci yang sama menghasilkan satu event dan satu histori.
	concurrentTeacher := createManualTeacher(t, db, "Guru manual bersamaan")
	teacherIDs = append(teacherIDs, concurrentTeacher)
	concurrentInput := manualAttendanceInput(t, 108, academic.TeacherCheckIn, date+"T00:00", "Submit bersamaan")
	concurrentInput = inputForTeacher(concurrentInput, concurrentTeacher)
	proofs := make(chan academic.TeacherAttendanceManualProof, 2)
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			proof, callErr := service.RecordTeacherAttendanceManual(ctx, petugas.ID, concurrentInput)
			proofs <- proof
			errs <- callErr
		}()
	}
	wait.Wait()
	close(proofs)
	close(errs)
	var concurrentProofs []academic.TeacherAttendanceManualProof
	for callErr := range errs {
		if callErr != nil {
			t.Fatalf("concurrent manual submit failed: %v", callErr)
		}
	}
	for proof := range proofs {
		concurrentProofs = append(concurrentProofs, proof)
	}
	if len(concurrentProofs) != 2 || concurrentProofs[0].EventID != concurrentProofs[1].EventID || concurrentProofs[0].Idempotent == concurrentProofs[1].Idempotent {
		t.Fatalf("concurrent manual submits did not converge: %+v", concurrentProofs)
	}
	eventIDs = append(eventIDs, concurrentProofs[0].EventID)
	assertManualCounts(t, db, concurrentTeacher, date, academic.TeacherCheckIn, 1, 1)

	// Histori gagal membuat seluruh transaksi batal.
	failingTeacher := createManualTeacher(t, db, "Guru histori gagal")
	teacherIDs = append(teacherIDs, failingTeacher)
	failingInput := manualAttendanceInput(t, 109, academic.TeacherCheckIn, date+"T00:00", "Uji rollback histori")
	triggerName := "test_manual_attendance_" + testutil.Random(t)[:12]
	functionName := triggerName + "_fn"
	if _, err = db.Exec(fmt.Sprintf(`CREATE FUNCTION app.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic manual attendance history failure'; END $$`, functionName)); err != nil {
		t.Fatal("create manual history failure fixture")
	}
	t.Cleanup(func() {
		_, _ = db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.teacher_attendance_history`, triggerName))
		_, _ = db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS app.%s()`, functionName))
	})
	if _, err = db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON app.teacher_attendance_history FOR EACH ROW EXECUTE FUNCTION app.%s()`, triggerName, functionName)); err != nil {
		t.Fatal("install manual history failure trigger")
	}
	func() {
		if _, err = service.RecordTeacherAttendanceManual(ctx, petugas.ID, inputForTeacher(failingInput, failingTeacher)); err == nil {
			t.Error("manual record succeeded although history insertion failed")
		}
	}()
	if _, err = db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.teacher_attendance_history`, triggerName)); err != nil {
		t.Fatal("remove manual history failure trigger")
	}
	assertManualCounts(t, db, failingTeacher, date, academic.TeacherCheckIn, 0, 0)

	// Manual event tidak boleh mengambil alih event GPS/selfie untuk aksi yang sama.
	t.Run("gps event remains protected", func(t *testing.T) {
		admin, _ := testutil.User(t, db, "admin")
		captureService := &academic.Store{DB: db, Photos: photos}
		location, _, locationErr := captureService.TeacherAttendanceLocation(ctx, admin.ID)
		if locationErr != nil {
			t.Fatal("read location for GPS fixture")
		}
		if !location.Configured {
			t.Skip("GPS conflict fixture needs an already configured test location")
		}
		gpsUser, gpsTeacherID := captureTeacher(t, db, "guru_mapel")
		teacherIDs = append(teacherIDs, gpsTeacherID)
		gpsInput := validCaptureInput(captureJPEG(t, false))
		gpsInput.Latitude, gpsInput.Longitude = location.Latitude, location.Longitude
		gpsInput.IdempotencyKey, err = academic.NewTeacherAttendanceIdempotencyKey()
		if err != nil {
			t.Fatal(err)
		}
		gpsProof, captureErr := captureService.CaptureTeacherAttendance(ctx, gpsUser.ID, gpsInput)
		if captureErr != nil {
			t.Fatalf("create GPS fixture: %v", captureErr)
		}
		eventIDs = append(eventIDs, gpsProof.EventID)
		gpsDate := gpsProof.OnDate
		manualInput := manualAttendanceInput(t, 110, academic.TeacherCheckIn, gpsProof.OccurredAt.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02T15:04"), "Fallback petugas setelah GPS")
		assertManualRejectedWithoutWrites(t, db, service, gpsTeacherID, gpsDate, academic.TeacherCheckIn, inputForTeacher(manualInput, gpsTeacherID), academic.ErrConflict, petugas.ID)
	})

	studentDailyAfter, studentLessonAfter := attendanceStudentCounts(t, db)
	if studentDailyAfter != studentDailyBefore || studentLessonAfter != studentLessonBefore {
		t.Fatalf("student attendance changed: daily %d->%d lesson %d->%d", studentDailyBefore, studentDailyAfter, studentLessonBefore, studentLessonAfter)
	}
}

func manualAttendanceToday(t *testing.T, db *sql.DB) string {
	t.Helper()
	var date string
	if err := db.QueryRow(`SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&date); err != nil {
		t.Fatal("read database WIB date")
	}
	return date
}

func manualAttendanceInput(t *testing.T, seed int, action academic.TeacherAttendanceAction, occurredAt, reason string) academic.TeacherAttendanceManualInput {
	t.Helper()
	key, err := academic.NewTeacherAttendanceIdempotencyKey()
	if err != nil {
		t.Fatal(err)
	}
	return academic.TeacherAttendanceManualInput{
		TeacherID: int64(seed), Action: action, OccurredAt: occurredAt, Reason: reason, IdempotencyKey: key,
	}
}

func inputForTeacher(input academic.TeacherAttendanceManualInput, teacherID int64) academic.TeacherAttendanceManualInput {
	input.TeacherID = teacherID
	return input
}

func manualAttendanceDateDaysBefore(t *testing.T, date string, days int) string {
	t.Helper()
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatal("parse synthetic attendance date")
	}
	return day.AddDate(0, 0, -days).Format("2006-01-02")
}

func manualAttendanceTime(value string) time.Time {
	parsed, _ := time.ParseInLocation("2006-01-02T15:04", value, time.FixedZone("WIB", 7*60*60))
	return parsed
}

func setManualEventDateForRetryTest(t *testing.T, db *sql.DB, eventID int64, date string, occurredAt time.Time, actorID, teacherID int64, teacherName string, action academic.TeacherAttendanceAction, reason string) {
	t.Helper()
	var snapshot map[string]any
	var encodedSnapshot []byte
	if err := db.QueryRow(`SELECT after_state FROM app.teacher_attendance_history WHERE event_id=$1 AND action='recorded'`, eventID).Scan(&encodedSnapshot); err != nil {
		t.Fatal("read synthetic recorded snapshot")
	}
	if err := json.Unmarshal(encodedSnapshot, &snapshot); err != nil {
		t.Fatal("decode synthetic recorded snapshot")
	}
	snapshot["on_date"] = date
	snapshot["occurred_at"] = occurredAt.Format(time.RFC3339Nano)
	evidence := struct {
		ActorID     int64                            `json:"actor_id"`
		TeacherID   int64                            `json:"teacher_id"`
		TeacherName string                           `json:"teacher_name"`
		OnDate      string                           `json:"on_date"`
		Action      academic.TeacherAttendanceAction `json:"action"`
		Source      string                           `json:"source"`
		OccurredAt  string                           `json:"occurred_at"`
		Reason      string                           `json:"reason"`
	}{actorID, teacherID, teacherName, date, action, "manual", occurredAt.Format(time.RFC3339Nano), reason}
	encodedEvidence, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal("encode synthetic retry evidence")
	}
	fingerprint := sha256.Sum256(encodedEvidence)
	snapshot["evidence_fingerprint"] = hex.EncodeToString(fingerprint[:])
	encodedSnapshot, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal("encode synthetic recorded snapshot")
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal("begin synthetic historic retry setup")
	}
	if _, err = tx.Exec(`UPDATE app.teacher_attendance_events SET on_date=$2::date,occurred_at=$3 WHERE id=$1`, eventID, date, occurredAt); err != nil {
		_ = tx.Rollback()
		t.Fatal("set synthetic historic event date")
	}
	if _, err = tx.Exec(`UPDATE app.teacher_attendance_history SET after_state=$2 WHERE event_id=$1 AND action='recorded'`, eventID, encodedSnapshot); err != nil {
		_ = tx.Rollback()
		t.Fatal("set synthetic historic retry snapshot")
	}
	if err = tx.Commit(); err != nil {
		t.Fatal("commit synthetic historic retry setup")
	}
}

func createManualTeacher(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	var teacherID int64
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES($1,NULL) RETURNING id`, name+" "+testutil.Random(t)[:6]).Scan(&teacherID); err != nil {
		t.Fatal("create synthetic teacher without account")
	}
	return teacherID
}

func assertManualStoredEvidence(t *testing.T, db *sql.DB, proof academic.TeacherAttendanceManualProof, reason, rawKey string) {
	t.Helper()
	var source, date, manualReason string
	var occurredAt, recordedAt sql.NullTime
	var photoID, distance, accuracy, schoolLatitude sql.NullFloat64
	var historyCount int
	var history string
	err := db.QueryRow(`
		SELECT e.source,e.on_date::text,e.manual_reason,e.occurred_at,e.recorded_at,e.photo_metadata_id,
		       e.distance_m,e.accuracy_m,e.school_latitude,
	       (SELECT count(*) FROM app.teacher_attendance_history h WHERE h.event_id=e.id AND h.action='recorded'),
	       (SELECT after_state::text FROM app.teacher_attendance_history h WHERE h.event_id=e.id AND h.action='recorded' LIMIT 1)
		FROM app.teacher_attendance_events e WHERE e.id=$1`, proof.EventID).Scan(
		&source, &date, &manualReason, &occurredAt, &recordedAt, &photoID, &distance, &accuracy, &schoolLatitude, &historyCount, &history)
	if err != nil {
		t.Fatal("read manual attendance evidence")
	}
	if source != "manual" || date != proof.OnDate || manualReason != reason || !occurredAt.Valid || !recordedAt.Valid ||
		occurredAt.Time.Equal(recordedAt.Time) || photoID.Valid || distance.Valid || accuracy.Valid || schoolLatitude.Valid || historyCount != 1 {
		t.Fatalf("manual event has unexpected GPS/photo/time/history fields: source=%s date=%s reason=%s photo=%v distance=%v accuracy=%v latitude=%v histories=%d", source, date, manualReason, photoID.Valid, distance.Valid, accuracy.Valid, schoolLatitude.Valid, historyCount)
	}
	if !occurredAt.Time.Equal(proof.OccurredAt) || !recordedAt.Time.Equal(proof.RecordedAt) {
		t.Fatalf("database timestamps differ from proof: occurred=%s/%s recorded=%s/%s", occurredAt.Time, proof.OccurredAt, recordedAt.Time, proof.RecordedAt)
	}
	if !strings.Contains(history, "idempotency_key_hash") || !strings.Contains(history, "evidence_fingerprint") || strings.Contains(history, rawKey) {
		t.Fatalf("recorded history lacks retry evidence or contains the raw request token: %s", history)
	}
}

func assertManualCounts(t *testing.T, db *sql.DB, teacherID int64, date string, action academic.TeacherAttendanceAction, events, histories int) {
	t.Helper()
	var eventCount, historyCount int
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1 AND on_date=$2::date AND action=$3`, teacherID, date, string(action)).Scan(&eventCount); err != nil {
		t.Fatal("count manual attendance events")
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1 AND e.on_date=$2::date AND e.action=$3`, teacherID, date, string(action)).Scan(&historyCount); err != nil {
		t.Fatal("count manual attendance history")
	}
	if eventCount != events || historyCount != histories {
		t.Fatalf("unexpected event/history counts for teacher %d: %d/%d, expected %d/%d", teacherID, eventCount, historyCount, events, histories)
	}
}

func assertManualRejectedWithoutWrites(t *testing.T, db *sql.DB, service *academic.Store, teacherID int64, date string, action academic.TeacherAttendanceAction, input academic.TeacherAttendanceManualInput, expected error, actorIDs ...int64) {
	t.Helper()
	actorID := int64(0)
	if len(actorIDs) > 0 {
		actorID = actorIDs[0]
	}
	var beforeEvents, beforePhotos, beforeHistory int
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1 AND on_date=$2::date AND action=$3`, teacherID, date, string(action)).Scan(&beforeEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_photos p JOIN app.teacher_attendance_events e ON e.id=p.event_id WHERE e.teacher_id=$1 AND e.on_date=$2::date AND e.action=$3`, teacherID, date, string(action)).Scan(&beforePhotos); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1 AND e.on_date=$2::date AND e.action=$3`, teacherID, date, string(action)).Scan(&beforeHistory); err != nil {
		t.Fatal(err)
	}
	if actorID == 0 {
		user, _ := testutil.User(t, db, "petugas_absensi")
		actorID = user.ID
	}
	_, err := service.RecordTeacherAttendanceManual(context.Background(), actorID, input)
	if err == nil || (expected != nil && !errors.Is(err, expected)) {
		t.Fatalf("manual record error=%v, expected=%v", err, expected)
	}
	var afterEvents, afterPhotos, afterHistory int
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1 AND on_date=$2::date AND action=$3`, teacherID, date, string(action)).Scan(&afterEvents); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_photos p JOIN app.teacher_attendance_events e ON e.id=p.event_id WHERE e.teacher_id=$1 AND e.on_date=$2::date AND e.action=$3`, teacherID, date, string(action)).Scan(&afterPhotos); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1 AND e.on_date=$2::date AND e.action=$3`, teacherID, date, string(action)).Scan(&afterHistory); err != nil {
		t.Fatal(err)
	}
	if beforeEvents != afterEvents || beforePhotos != afterPhotos || beforeHistory != afterHistory {
		t.Fatalf("rejected manual record changed evidence: events %d->%d photos %d->%d history %d->%d", beforeEvents, afterEvents, beforePhotos, afterPhotos, beforeHistory, afterHistory)
	}
}

func cleanupTeacherAttendanceManualEvents(t *testing.T, db *sql.DB, photos *teacherphoto.Store, eventIDs, teacherIDs []int64) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Errorf("begin synthetic manual event cleanup: %v", err)
		return
	}
	for _, eventID := range eventIDs {
		if _, err = tx.Exec(`DELETE FROM app.teacher_attendance_history WHERE event_id=$1`, eventID); err != nil {
			_ = tx.Rollback()
			t.Errorf("delete synthetic attendance history: %v", err)
			return
		}
		if _, err = tx.Exec(`DELETE FROM app.teacher_attendance_photos WHERE event_id=$1`, eventID); err != nil {
			_ = tx.Rollback()
			t.Errorf("delete synthetic attendance photo: %v", err)
			return
		}
		if _, err = tx.Exec(`DELETE FROM app.teacher_attendance_events WHERE id=$1`, eventID); err != nil {
			_ = tx.Rollback()
			t.Errorf("delete synthetic attendance event: %v", err)
			return
		}
	}
	for _, teacherID := range teacherIDs {
		if _, err = tx.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID); err != nil {
			_ = tx.Rollback()
			t.Errorf("delete synthetic attendance teacher: %v", err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		t.Errorf("commit synthetic attendance cleanup: %v", err)
		return
	}
	keys, err := photos.ListKeys()
	if err != nil {
		t.Errorf("list synthetic photo keys during cleanup: %v", err)
		return
	}
	for _, key := range keys {
		if err = photos.Delete(key); err != nil {
			t.Errorf("delete synthetic photo during cleanup: %v", err)
		}
	}
}
