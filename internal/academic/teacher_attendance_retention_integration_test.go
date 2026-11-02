//go:build integration

package academic_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

type retentionPhotoStorage struct {
	academic.TeacherAttendancePhotoStorage
	mu      sync.Mutex
	failKey string
	calls   int
}

func (s *retentionPhotoStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if key == s.failKey {
		return errors.New("synthetic private path must not be persisted")
	}
	return s.TeacherAttendancePhotoStorage.Delete(key)
}

type retentionFixture struct {
	eventID, photoID                     int64
	key                                  string
	eventState, historyState, photoState string
}

func TestPurgeExpiredTeacherAttendancePhotosIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	// Purge menyeleksi metadata global; jangan menjalankannya jika ada bukti nonfixture yang kedaluwarsa.
	var existingExpired int
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_photos WHERE deleted_at IS NULL AND retain_until<=clock_timestamp()`).Scan(&existingExpired); err != nil {
		t.Fatal(err)
	}
	if existingExpired != 0 {
		t.Fatal("retention test requires no pre-existing expired photos; shared metadata left untouched")
	}
	photos := newCapturePhotoStore(t)
	storage := &retentionPhotoStorage{TeacherAttendancePhotoStorage: photos}
	service := &academic.Store{DB: db, Photos: storage}
	officer, _ := testutil.User(t, db, "pegawai_tu", "pengelola_kehadiran_guru")
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1`, officer.ID); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(`DELETE FROM app.users WHERE id=$1`, officer.ID); err != nil {
			t.Error(err)
		}
	})
	dailyBefore, lessonBefore := attendanceStudentCounts(t, db)
	var now time.Time
	if err := db.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	makePhoto := func(deadline time.Time) retentionFixture {
		t.Helper()
		meta, err := photos.Save(bytes.NewReader(captureJPEG(t, false)), "image/jpeg")
		if err != nil {
			t.Fatal(err)
		}
		var f retentionFixture
		f.key = meta.StorageKey
		var teacherID int64
		if err = db.QueryRow(`INSERT INTO app.teachers(name) VALUES('Fixture retensi sintetis') RETURNING id`).Scan(&teacherID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			tx, err := db.Begin()
			if err != nil {
				t.Error(err)
				return
			}
			defer tx.Rollback()
			for _, q := range []string{`DELETE FROM app.teacher_attendance_history WHERE event_id=$1`, `DELETE FROM app.teacher_attendance_photos WHERE event_id=$1`, `DELETE FROM app.teacher_attendance_events WHERE id=$1`} {
				if _, err = tx.Exec(q, f.eventID); err != nil {
					t.Error(err)
					return
				}
			}
			if _, err = tx.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID); err != nil {
				t.Error(err)
				return
			}
			if err = tx.Commit(); err != nil {
				t.Error(err)
			}
		})
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err = tx.QueryRow(`SELECT nextval(pg_get_serial_sequence('app.teacher_attendance_photos','id'))`).Scan(&f.photoID); err != nil {
			t.Fatal(err)
		}
		occurred := now.AddDate(0, -3, 0)
		if err = tx.QueryRow(`INSERT INTO app.teacher_attendance_events(teacher_id,teacher_name,on_date,action,source,actor_id,occurred_at,recorded_at,distance_m,accuracy_m,school_latitude,school_longitude,geofence_radius_m,photo_metadata_id)
   VALUES($1,'Fixture retensi sintetis',($2::timestamptz AT TIME ZONE 'Asia/Jakarta')::date,'check_in','gps_selfie',$3,$2,$2,0,1,0,0,100,$4) RETURNING id`, teacherID, occurred, officer.ID, f.photoID).Scan(&f.eventID); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO app.teacher_attendance_photos(id,event_id,storage_key,mime_type,size_bytes,checksum_sha256,created_at,retain_until)
   OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, f.photoID, f.eventID, f.key, meta.MIMEType, meta.SizeBytes, meta.ChecksumSHA256, occurred, deadline); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO app.teacher_attendance_history(event_id,action,actor_id,after_state) VALUES($1,'recorded',$2,'{"fixture":"retention"}')`, f.eventID, officer.ID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		f.eventState, f.historyState, f.photoState = retentionEvidence(t, db, f)
		return f
	}
	assertState := func(f retentionFixture, deleted bool, attempts int) {
		t.Helper()
		var deletedAt sql.NullTime
		var actualAttempts int
		var cleanupError sql.NullString
		if err := db.QueryRow(`SELECT deleted_at,cleanup_attempts,last_cleanup_error FROM app.teacher_attendance_photos WHERE id=$1`, f.photoID).Scan(&deletedAt, &actualAttempts, &cleanupError); err != nil {
			t.Fatal(err)
		}
		if deletedAt.Valid != deleted || actualAttempts != attempts {
			t.Fatalf("deleted=%v attempts=%d want %v/%d", deletedAt.Valid, actualAttempts, deleted, attempts)
		}
		if deleted && (deletedAt.Time.Before(now) || cleanupError.Valid) {
			t.Fatal("invalid server deletion timestamp or stale cleanup error")
		}
		event, history, photo := retentionEvidence(t, db, f)
		if event != f.eventState || history != f.historyState || photo != f.photoState {
			t.Fatal("purge changed immutable event, history or photo metadata")
		}
		file, err := photos.Open(f.key)
		if deleted {
			if !errors.Is(err, teacherphoto.ErrNotFound) {
				if file != nil {
					file.Close()
				}
				t.Fatalf("deleted photo open error=%v", err)
			}
		} else if err == nil {
			file.Close()
		} else {
			t.Fatal(err)
		}
	}
	future := makePhoto(now.Add(time.Hour))
	atDeadline := makePhoto(now)
	old := makePhoto(now.Add(-time.Hour))
	result, err := service.PurgeExpiredTeacherAttendancePhotos(ctx, 1)
	if err != nil || result.Purged != 1 || result.Examined != 1 {
		t.Fatalf("batch limit result=%+v err=%v", result, err)
	}
	assertState(old, true, 0)
	assertState(atDeadline, false, 0)
	assertState(future, false, 0)
	result, err = service.PurgeExpiredTeacherAttendancePhotos(ctx, 50)
	if err != nil || result.Purged != 1 {
		t.Fatalf("deadline result=%+v err=%v", result, err)
	}
	assertState(atDeadline, true, 0)
	result, err = service.PurgeExpiredTeacherAttendancePhotos(ctx, 50)
	if err != nil || result.Purged != 0 || result.Examined != 0 {
		t.Fatalf("retry result=%+v err=%v", result, err)
	}

	t.Run("storage failure and independent success", func(t *testing.T) {
		bad := makePhoto(now.Add(-time.Hour))
		good := makePhoto(now.Add(-time.Minute))
		storage.failKey = bad.key
		result, err := service.PurgeExpiredTeacherAttendancePhotos(ctx, 50)
		if !errors.Is(err, academic.ErrTeacherAttendanceCleanup) || result.Failed != 1 || result.Purged != 1 {
			t.Fatalf("partial result=%+v err=%v", result, err)
		}
		assertState(bad, false, 1)
		assertState(good, true, 0)
		var message string
		if err = db.QueryRow(`SELECT last_cleanup_error FROM app.teacher_attendance_photos WHERE id=$1`, bad.photoID).Scan(&message); err != nil || message != "photo_delete_failed" {
			t.Fatalf("unsafe cleanup error: %q", message)
		}
		storage.failKey = ""
		result, err = service.PurgeExpiredTeacherAttendancePhotos(ctx, 50)
		if err != nil || result.Purged != 1 {
			t.Fatalf("retry result=%+v err=%v", result, err)
		}
		assertState(bad, true, 1)
	})
	t.Run("missing file", func(t *testing.T) {
		f := makePhoto(now.Add(-time.Minute))
		if err := photos.Delete(f.key); err != nil {
			t.Fatal(err)
		}
		result, err := service.PurgeExpiredTeacherAttendancePhotos(ctx, 50)
		if err != nil || result.Purged != 1 {
			t.Fatalf("missing file result=%+v err=%v", result, err)
		}
		assertState(f, true, 0)
	})
	t.Run("failing candidate does not monopolize single photo batches", func(t *testing.T) {
		bad := makePhoto(now.Add(-time.Hour))
		good := []retentionFixture{
			makePhoto(now.Add(-3 * time.Minute)),
			makePhoto(now.Add(-2 * time.Minute)),
			makePhoto(now.Add(-time.Minute)),
		}
		storage.failKey = bad.key
		result, err := service.PurgeExpiredTeacherAttendancePhotos(ctx, 1)
		if !errors.Is(err, academic.ErrTeacherAttendanceCleanup) || result.Examined != 1 || result.Failed != 1 || result.Purged != 0 {
			t.Fatalf("first failed single-photo batch=%+v err=%v", result, err)
		}
		// Kandidat gagal tetap gagal, tetapi jumlah percobaannya tidak menahan kandidat lain.
		for _, f := range good {
			result, err = service.PurgeExpiredTeacherAttendancePhotos(ctx, 1)
			if err != nil || result.Examined != 1 || result.Purged != 1 || result.Failed != 0 {
				t.Fatalf("healthy single-photo batch=%+v err=%v", result, err)
			}
			assertState(f, true, 0)
		}
		assertState(bad, false, 1)
		storage.failKey = ""
		result, err = service.PurgeExpiredTeacherAttendancePhotos(ctx, 1)
		if err != nil || result.Purged != 1 {
			t.Fatalf("failed candidate retry=%+v err=%v", result, err)
		}
		assertState(bad, true, 1)
	})
	for _, tc := range []struct {
		name          string
		triggerTiming string
		wantError     error
	}{
		{"metadata update failure after deletion", "CREATE TRIGGER %s BEFORE UPDATE ON app.teacher_attendance_photos FOR EACH ROW EXECUTE FUNCTION app.%s()", academic.ErrTeacherAttendanceWrite},
		{"commit failure after deletion", "CREATE CONSTRAINT TRIGGER %s AFTER UPDATE ON app.teacher_attendance_photos DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.%s()", academic.ErrTeacherAttendanceCommit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := makePhoto(now.Add(-time.Minute))
			function := "test_retention_" + testutil.Random(t)[:12]
			trigger := function + "_trigger"
			drop := func() {
				if _, err := db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.teacher_attendance_photos`, trigger)); err != nil {
					t.Error(err)
				}
				if _, err := db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS app.%s()`, function)); err != nil {
					t.Error(err)
				}
			}
			if _, err := db.Exec(fmt.Sprintf(`CREATE FUNCTION app.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id=%d THEN RAISE EXCEPTION 'synthetic retention update failure'; END IF; RETURN NEW; END $$`, function, f.photoID)); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(drop)
			if _, err := db.Exec(fmt.Sprintf(tc.triggerTiming, trigger, function)); err != nil {
				t.Fatal(err)
			}
			result, err := service.PurgeExpiredTeacherAttendancePhotos(ctx, 50)
			if !errors.Is(err, tc.wantError) || result.Purged != 0 || result.Failed != 1 {
				t.Fatalf("update failure result=%+v err=%v", result, err)
			}
			var deleted bool
			if err = db.QueryRow(`SELECT deleted_at IS NOT NULL FROM app.teacher_attendance_photos WHERE id=$1`, f.photoID).Scan(&deleted); err != nil || deleted {
				t.Fatal("failed update marked deletion")
			}
			if file, err := photos.Open(f.key); !errors.Is(err, teacherphoto.ErrNotFound) {
				if file != nil {
					file.Close()
				}
				t.Fatal("file was not removed before database failure")
			}
			drop()
			result, err = service.PurgeExpiredTeacherAttendancePhotos(ctx, 50)
			if err != nil || result.Purged != 1 {
				t.Fatalf("recover update failure result=%+v err=%v", result, err)
			}
			assertState(f, true, 0)
		})
	}
	t.Run("correction preserves retention", func(t *testing.T) {
		f := makePhoto(now.Add(-time.Minute))
		var occurred time.Time
		if err := db.QueryRow(`SELECT occurred_at FROM app.teacher_attendance_events WHERE id=$1`, f.eventID).Scan(&occurred); err != nil {
			t.Fatal(err)
		}
		_, err := service.CorrectTeacherAttendance(ctx, officer.ID, academic.TeacherAttendanceCorrectionInput{EventID: f.eventID, ExpectedVersion: 1, OccurredAt: academic.TeacherAttendanceWIBTime(occurred).Format("2006-01-02") + "T12:00", CorrectionReason: "Fixture koreksi sebelum retensi"})
		if err != nil {
			t.Fatal(err)
		}
		_, _, photo := retentionEvidence(t, db, f)
		if photo != f.photoState {
			t.Fatal("correction changed retention metadata")
		}
		f.eventState, f.historyState, f.photoState = retentionEvidence(t, db, f)
		result, err := service.PurgeExpiredTeacherAttendancePhotos(ctx, 50)
		if err != nil || result.Purged != 1 {
			t.Fatalf("corrected result=%+v err=%v", result, err)
		}
		assertState(f, true, 0)
	})
	t.Run("candidate revalidated after waiting for mutation lock", func(t *testing.T) {
		f := makePhoto(now.Add(-time.Minute))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
			t.Fatal(err)
		}
		purgeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		type outcome struct {
			result academic.TeacherAttendancePhotoPurgeResult
			err    error
		}
		done := make(chan outcome, 1)
		go func() { r, e := service.PurgeExpiredTeacherAttendancePhotos(purgeCtx, 50); done <- outcome{r, e} }()
		// Tunggu sampai seleksi kandidat selesai dan worker benar-benar menunggu lock.
		for {
			var waiting bool
			if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory' AND query LIKE 'SELECT pg_advisory_xact_lock%')`).Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			select {
			case out := <-done:
				t.Fatalf("worker stopped before lock: %+v", out)
			case <-purgeCtx.Done():
				t.Fatal("worker did not reach mutation lock")
			case <-time.After(10 * time.Millisecond):
			}
		}
		// Perubahan ini hanya mensimulasikan kandidat basi pada metadata fixture milik tes.
		if _, err = tx.Exec(`UPDATE app.teacher_attendance_photos SET retain_until=$1 WHERE id=$2`, now.Add(time.Hour), f.photoID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		out := <-done
		if out.err != nil || out.result.Examined != 1 || out.result.Purged != 0 || out.result.Failed != 0 {
			t.Fatalf("stale candidate result=%+v err=%v", out.result, out.err)
		}
		f.eventState, f.historyState, f.photoState = retentionEvidence(t, db, f)
		assertState(f, false, 0)
	})
	t.Run("mutation lock and concurrent purges", func(t *testing.T) {
		first, second := makePhoto(now.Add(-time.Minute)), makePhoto(now.Add(-time.Second))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
			t.Fatal(err)
		}
		type outcome struct {
			result academic.TeacherAttendancePhotoPurgeResult
			err    error
		}
		done := make(chan outcome, 2)
		for i := 0; i < 2; i++ {
			go func() { r, e := service.PurgeExpiredTeacherAttendancePhotos(ctx, 50); done <- outcome{r, e} }()
		}
		select {
		case <-done:
			t.Fatal("purge bypassed mutation lock")
		case <-time.After(100 * time.Millisecond):
		}
		if file, err := photos.Open(first.key); err != nil {
			t.Fatal("file removed while mutation lock held")
		} else {
			file.Close()
		}
		if err = tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		purged := 0
		for i := 0; i < 2; i++ {
			out := <-done
			if out.err != nil {
				t.Fatal(out.err)
			}
			purged += out.result.Purged
		}
		if purged != 2 {
			t.Fatalf("concurrent purged=%d want 2", purged)
		}
		assertState(first, true, 0)
		assertState(second, true, 0)
	})
	assertState(future, false, 0)
	dailyAfter, lessonAfter := attendanceStudentCounts(t, db)
	if dailyAfter != dailyBefore || lessonAfter != lessonBefore {
		t.Fatal("student attendance changed during retention purge")
	}
}

func retentionEvidence(t *testing.T, db *sql.DB, f retentionFixture) (event, history, photo string) {
	t.Helper()
	if err := db.QueryRow(`SELECT to_jsonb(e)::text,(SELECT jsonb_agg(to_jsonb(h) ORDER BY h.id)::text FROM app.teacher_attendance_history h WHERE h.event_id=e.id),
 (SELECT (to_jsonb(p)-'deleted_at'-'cleanup_attempts'-'last_cleanup_error')::text FROM app.teacher_attendance_photos p WHERE p.event_id=e.id)
 FROM app.teacher_attendance_events e WHERE e.id=$1`, f.eventID).Scan(&event, &history, &photo); err != nil {
		t.Fatal(err)
	}
	return
}
