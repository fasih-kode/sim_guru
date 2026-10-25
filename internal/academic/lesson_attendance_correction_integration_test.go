//go:build integration

package academic

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestCorrectLessonAttendanceIntegration(t *testing.T) {
	f := newLessonAttendanceFixture(t)
	form := f.formFor(t, f.teacher.ID, f.date, f.entryID)
	choices := []LessonAttendanceChoice{
		{StudentID: f.students[0], Status: "hadir"},
		{StudentID: f.students[1], Status: "hadir"},
		{StudentID: f.students[2], Status: "hadir"},
		{StudentID: f.students[3], Status: "hadir"},
		{StudentID: f.students[4], Status: "hadir"},
		{StudentID: f.students[5], Status: "hadir"},
	}
	if _, err := f.store.SaveLessonAttendance(f.ctx, f.teacher.ID, f.input(f.date, f.entryID, form.Fingerprint, choices...)); err != nil {
		t.Fatal(err)
	}
	ids := make(map[int64]int64)
	for _, studentID := range f.students[:6] {
		var attendanceID int64
		if err := f.db.QueryRow(`SELECT id FROM app.student_lesson_attendance WHERE timetable_entry_id=$1 AND meeting_date=$2 AND student_id=$3`, f.entryID, f.date, studentID).Scan(&attendanceID); err != nil {
			t.Fatal(err)
		}
		ids[studentID] = attendanceID
	}
	attendanceID := ids[f.students[0]]
	var originalSnapshot []byte
	var originalRecordedAt string
	var originalRecordedBy int64
	if err := f.db.QueryRow(`SELECT snapshot,recorded_at::text,recorded_by FROM app.student_lesson_attendance WHERE id=$1`, attendanceID).Scan(&originalSnapshot, &originalRecordedAt, &originalRecordedBy); err != nil {
		t.Fatal(err)
	}

	for _, input := range []LessonAttendanceCorrectionInput{
		{AttendanceID: attendanceID, ExpectedVersion: 1, NewStatus: "hadir", Reason: " "},
		{AttendanceID: attendanceID, ExpectedVersion: 1, NewStatus: "tidak-hadir", Reason: "Status tidak valid"},
		{AttendanceID: attendanceID, ExpectedVersion: 1, NewStatus: "sakit", Reason: string(make([]byte, 501))},
	} {
		if _, err := f.store.CorrectLessonAttendance(f.ctx, f.teacher.ID, input); err == nil {
			t.Fatal("invalid correction was accepted")
		}
	}
	if _, err := f.store.CorrectLessonAttendance(f.ctx, f.otherTeacher.ID, LessonAttendanceCorrectionInput{AttendanceID: attendanceID, ExpectedVersion: 1, NewStatus: "sakit", Reason: "Guru lain"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replacement/other teacher got %v, want not found", err)
	}
	if _, err := f.store.CorrectLessonAttendance(f.ctx, f.curriculum.ID, LessonAttendanceCorrectionInput{AttendanceID: attendanceID, ExpectedVersion: 1, NewStatus: "sakit", Reason: "Guru masih aktif"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Kurikulum before fallback got %v, want not found", err)
	}
	if _, err := f.store.CorrectLessonAttendance(f.ctx, f.admin.ID, LessonAttendanceCorrectionInput{AttendanceID: attendanceID, ExpectedVersion: 1, NewStatus: "sakit", Reason: "Admin"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Admin got %v, want forbidden", err)
	}

	// Riwayat historis tidak bergantung pada penugasan yang masih berlaku sekarang.
	if _, err := f.db.Exec(`UPDATE app.teaching_assignments SET effective_from=$1::date+1 WHERE semester_id=$2 AND class_id=$3 AND subject_id=$4 AND teacher_id=(SELECT id FROM app.teachers WHERE user_id=$5)`, f.date, f.semester, f.classID, f.subjectID, f.teacher.ID); err != nil {
		t.Fatal(err)
	}
	corrected, err := f.store.CorrectLessonAttendance(f.ctx, f.teacher.ID, LessonAttendanceCorrectionInput{AttendanceID: attendanceID, ExpectedVersion: 1, NewStatus: "sakit", Reason: "Guru mencatat ulang hasil pertemuan"})
	if err != nil || corrected.Version != 2 || corrected.Administrative {
		t.Fatalf("historical teacher correction result=%+v err=%v", corrected, err)
	}
	if _, err = f.store.CorrectLessonAttendance(f.ctx, f.teacher.ID, LessonAttendanceCorrectionInput{AttendanceID: attendanceID, ExpectedVersion: 2, NewStatus: "sakit", Reason: "Tidak ada perubahan"}); err != nil {
		t.Fatalf("same-status no-op: %v", err)
	}
	if _, err = f.store.CorrectLessonAttendance(f.ctx, f.teacher.ID, LessonAttendanceCorrectionInput{AttendanceID: attendanceID, ExpectedVersion: 1, NewStatus: "sakit", Reason: "Versi basi"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale same-status correction got %v", err)
	}
	var status string
	var version int64
	var snapshot []byte
	var recordedAt string
	var recordedBy int64
	var updatedAt string
	if err = f.db.QueryRow(`SELECT status,version,snapshot,recorded_at::text,recorded_by,updated_at::text FROM app.student_lesson_attendance WHERE id=$1`, attendanceID).Scan(&status, &version, &snapshot, &recordedAt, &recordedBy, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if status != "sakit" || version != 2 || string(snapshot) != string(originalSnapshot) || recordedAt != originalRecordedAt || recordedBy != originalRecordedBy {
		t.Fatal("correction changed immutable event evidence")
	}
	var histories int
	var beforeStatus, afterStatus, reason, correctionType string
	var beforeVersion, afterVersion int64
	var historySnapshotMatches bool
	var historyAt string
	if err = f.db.QueryRow(`SELECT count(*),max(before_state->>'status'),max(after_state->>'status'),max(reason),max(after_state->>'correction_type'),max((before_state->>'version')::bigint),max((after_state->>'version')::bigint),bool_and(before_state->'snapshot'=after_state->'snapshot' AND after_state->'snapshot'=(SELECT snapshot FROM app.student_lesson_attendance WHERE id=$1)),max(created_at::text) FROM app.student_lesson_attendance_history WHERE attendance_id=$1`, attendanceID).Scan(&histories, &beforeStatus, &afterStatus, &reason, &correctionType, &beforeVersion, &afterVersion, &historySnapshotMatches, &historyAt); err != nil {
		t.Fatal(err)
	}
	if histories != 2 || beforeStatus != "hadir" || afterStatus != "sakit" || reason != "Guru mencatat ulang hasil pertemuan" || correctionType != "guru_mapel" || beforeVersion != 1 || afterVersion != 2 || !historySnapshotMatches || updatedAt < recordedAt || historyAt < updatedAt {
		t.Fatalf("correction history incomplete: rows=%d before=%q after=%q reason=%q kind=%q versions=%d/%d", histories, beforeStatus, afterStatus, reason, correctionType, beforeVersion, afterVersion)
	}

	// Akun historis nonaktif membuka fallback Kurikulum, tetapi guru itu sendiri ditolak.
	secondID := ids[f.students[2]]
	if _, err = f.db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, f.teacher.ID); err != nil {
		t.Fatal(err)
	}
	adminResult, err := f.store.CorrectLessonAttendance(f.ctx, f.curriculum.ID, LessonAttendanceCorrectionInput{AttendanceID: secondID, ExpectedVersion: 1, NewStatus: "izin", Reason: "Koreksi administratif setelah akun dinonaktifkan"})
	if err != nil || !adminResult.Administrative || adminResult.Version != 2 {
		t.Fatalf("inactive-account fallback result=%+v err=%v", adminResult, err)
	}
	if _, err = f.store.LessonAttendanceRecord(f.ctx, f.teacher.ID, secondID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("inactive historic teacher read returned %v", err)
	}
	if _, err = f.db.Exec(`UPDATE app.users SET active=true WHERE id=$1`, f.teacher.ID); err != nil {
		t.Fatal(err)
	}

	// Pencabutan role Guru Mapel juga membuka fallback; role Admin yang tersisa bukan bypass.
	if _, err = f.db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1 AND role_code='guru_mapel'`, f.teacher.ID); err != nil {
		t.Fatal(err)
	}
	thirdID := ids[f.students[3]]
	if _, err = f.store.CorrectLessonAttendance(f.ctx, f.teacher.ID, LessonAttendanceCorrectionInput{AttendanceID: thirdID, ExpectedVersion: 1, NewStatus: "izin", Reason: "Guru sudah tidak memiliki role pengampu"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("teacher without role got %v", err)
	}
	adminResult, err = f.store.CorrectLessonAttendance(f.ctx, f.curriculum.ID, LessonAttendanceCorrectionInput{AttendanceID: thirdID, ExpectedVersion: 1, NewStatus: "alpa", Reason: "Koreksi administratif setelah role dicabut"})
	if err != nil || !adminResult.Administrative {
		t.Fatalf("role-removal fallback result=%+v err=%v", adminResult, err)
	}
	if _, err = f.db.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'guru_mapel')`, f.teacher.ID); err != nil {
		t.Fatal(err)
	}

	// Profil guru nonaktif saja tidak mengaktifkan fallback bila akun dan role aktif.
	fourthID := ids[f.students[4]]
	if _, err = f.db.Exec(`UPDATE app.teachers SET active=false WHERE user_id=$1`, f.teacher.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CorrectLessonAttendance(f.ctx, f.curriculum.ID, LessonAttendanceCorrectionInput{AttendanceID: fourthID, ExpectedVersion: 1, NewStatus: "sakit", Reason: "Tidak boleh fallback dari profil saja"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("inactive teacher profile improperly allowed fallback: %v", err)
	}
	if _, err = f.db.Exec(`UPDATE app.teachers SET active=true WHERE user_id=$1`, f.teacher.ID); err != nil {
		t.Fatal(err)
	}

	// Dua pengiriman dengan versi yang sama hanya dapat menghasilkan satu koreksi.
	fifthID := ids[f.students[4]]
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, candidate := range []string{"sakit", "izin"} {
		wg.Add(1)
		go func(newStatus string) {
			defer wg.Done()
			_, e := f.store.CorrectLessonAttendance(f.ctx, f.teacher.ID, LessonAttendanceCorrectionInput{AttendanceID: fifthID, ExpectedVersion: 1, NewStatus: newStatus, Reason: "Koreksi serentak"})
			errs <- e
		}(candidate)
	}
	wg.Wait()
	close(errs)
	var successes, conflicts int
	for e := range errs {
		if e == nil {
			successes++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("concurrent correction returned %v", e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent correction success/conflict=%d/%d", successes, conflicts)
	}

	// Kegagalan histori harus membatalkan perubahan status dan versi.
	rollbackID := ids[f.students[1]]
	name := "fail_lesson_history_" + testutil.Random(t)[:12]
	if _, err = f.db.Exec(fmt.Sprintf(`CREATE FUNCTION app.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test history failure'; END $$`, name)); err != nil {
		t.Fatal(err)
	}
	trigger := "trg_" + name
	if _, err = f.db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON app.student_lesson_attendance_history FOR EACH ROW WHEN (NEW.attendance_id=%d AND NEW.action='corrected') EXECUTE FUNCTION app.%s()`, trigger, rollbackID, name)); err != nil {
		_, _ = f.db.Exec("DROP FUNCTION app." + name + "()")
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.db.Exec("DROP TRIGGER IF EXISTS " + trigger + " ON app.student_lesson_attendance_history")
		_, _ = f.db.Exec("DROP FUNCTION IF EXISTS app." + name + "()")
	})
	var beforeRollbackStatus string
	var beforeRollbackVersion int64
	if err = f.db.QueryRow(`SELECT status,version FROM app.student_lesson_attendance WHERE id=$1`, rollbackID).Scan(&beforeRollbackStatus, &beforeRollbackVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CorrectLessonAttendance(f.ctx, f.teacher.ID, LessonAttendanceCorrectionInput{AttendanceID: rollbackID, ExpectedVersion: 1, NewStatus: "sakit", Reason: "Histori gagal"}); err == nil {
		t.Fatal("history trigger failure did not fail correction")
	}
	var afterRollbackStatus string
	var afterRollbackVersion int64
	if err = f.db.QueryRow(`SELECT status,version FROM app.student_lesson_attendance WHERE id=$1`, rollbackID).Scan(&afterRollbackStatus, &afterRollbackVersion); err != nil {
		t.Fatal(err)
	}
	if afterRollbackStatus != beforeRollbackStatus || afterRollbackVersion != beforeRollbackVersion {
		t.Fatalf("history failure did not roll back: status %s/%s version %d/%d", beforeRollbackStatus, afterRollbackStatus, beforeRollbackVersion, afterRollbackVersion)
	}

	// Identitas guru yang tidak lagi tertaut ke akun membuka fallback bila skema mengizinkan.
	if _, err = f.db.Exec(`UPDATE app.teachers SET user_id=NULL WHERE user_id=$1`, f.teacher.ID); err != nil {
		t.Fatal(err)
	}
	linkedID := ids[f.students[5]]
	if _, err = f.store.CorrectLessonAttendance(f.ctx, f.teacher.ID, LessonAttendanceCorrectionInput{AttendanceID: linkedID, ExpectedVersion: 1, NewStatus: "izin", Reason: "Guru tidak lagi tertaut ke akun"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked historic user got %v, want not found", err)
	}
	adminResult, err = f.store.CorrectLessonAttendance(f.ctx, f.curriculum.ID, LessonAttendanceCorrectionInput{AttendanceID: linkedID, ExpectedVersion: 1, NewStatus: "sakit", Reason: "Koreksi administratif tanpa akun guru terhubung"})
	if err != nil || !adminResult.Administrative {
		t.Fatalf("unlinked-account fallback result=%+v err=%v", adminResult, err)
	}
	listed, err := f.store.LessonAttendanceRecordsOnDate(f.ctx, f.curriculum.ID, f.date)
	if err != nil {
		t.Fatal(err)
	}
	var historicalTeacherID int64
	if err = f.db.QueryRow(`SELECT teacher_id FROM app.student_lesson_attendance WHERE id=$1`, linkedID).Scan(&historicalTeacherID); err != nil {
		t.Fatal(err)
	}
	var visible int
	for _, record := range listed {
		if record.TeacherID == historicalTeacherID {
			visible++
		}
	}
	if visible != 6 {
		t.Fatalf("Kurikulum could not find this teacher's records after account link was removed: got %d", visible)
	}
}
