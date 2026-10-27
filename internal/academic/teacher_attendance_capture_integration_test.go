//go:build integration

package academic_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestCaptureTeacherAttendanceIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	service := &academic.Store{DB: db}
	admin, _ := testutil.User(t, db, "admin")
	initial, _, err := service.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil {
		t.Fatal("read initial attendance location")
	}
	store := newCapturePhotoStore(t)
	service.Photos = store
	var createdEventIDs []int64
	t.Cleanup(func() {
		if len(createdEventIDs) > 0 {
			tx, beginErr := db.BeginTx(ctx, nil)
			if beginErr != nil {
				t.Errorf("begin synthetic attendance cleanup: %v", beginErr)
				return
			}
			for _, eventID := range createdEventIDs {
				if _, cleanupErr := tx.ExecContext(ctx, `DELETE FROM app.teacher_attendance_history WHERE event_id=$1`, eventID); cleanupErr != nil {
					_ = tx.Rollback()
					t.Errorf("delete synthetic attendance history: %v", cleanupErr)
					return
				}
				if _, cleanupErr := tx.ExecContext(ctx, `DELETE FROM app.teacher_attendance_photos WHERE event_id=$1`, eventID); cleanupErr != nil {
					_ = tx.Rollback()
					t.Errorf("delete synthetic attendance photo metadata: %v", cleanupErr)
					return
				}
				if _, cleanupErr := tx.ExecContext(ctx, `DELETE FROM app.teacher_attendance_events WHERE id=$1`, eventID); cleanupErr != nil {
					_ = tx.Rollback()
					t.Errorf("delete synthetic attendance event: %v", cleanupErr)
					return
				}
			}
			if cleanupErr := tx.Commit(); cleanupErr != nil {
				t.Errorf("commit synthetic attendance cleanup: %v", cleanupErr)
				return
			}
		}
		keys, listErr := store.ListKeys()
		if listErr != nil {
			t.Errorf("list synthetic attendance photos during cleanup: %v", listErr)
			return
		}
		for _, key := range keys {
			if deleteErr := store.Delete(key); deleteErr != nil {
				t.Errorf("delete synthetic attendance photo during cleanup: %v", deleteErr)
			}
		}
	})
	if !initial.Configured {
		unconfiguredUser, unconfiguredTeacherID := captureTeacher(t, db, "guru_mapel")
		assertCaptureRejectedWithoutWrites(t, db, service, store, unconfiguredUser.ID, unconfiguredTeacherID, validCaptureInput(captureJPEG(t, false)), academic.ValidationError("Lokasi sekolah belum diatur oleh Admin."))
	}
	if err = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{
		Latitude: 0, Longitude: 0, RadiusM: 100, ExpectedVersion: initial.Version, Reason: "Fixture sintetis DEV-03C2",
	}); err != nil {
		t.Fatal("configure synthetic attendance location")
	}
	if initial.Configured {
		t.Cleanup(func() {
			latest, _, readErr := service.TeacherAttendanceLocation(ctx, admin.ID)
			if readErr == nil {
				_ = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{
					Latitude: initial.Latitude, Longitude: initial.Longitude, RadiusM: initial.RadiusM,
					ExpectedVersion: latest.Version, Reason: "Pulihkan konfigurasi setelah integration test",
				})
			}
		})
	} else {
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location_history WHERE location_id=1`)
			_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location WHERE id=1`)
		})
	}
	config, _, err := service.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil || !config.Configured || config.RadiusM != 100 {
		t.Fatalf("synthetic location unavailable: configured=%v err=%v", config.Configured, err)
	}

	dailyBefore, lessonBefore := attendanceStudentCounts(t, db)

	// Guru Mapel + Wali Kelas tetap memakai satu identitas guru dan satu event.
	teacher, teacherID := captureTeacher(t, db, "guru_mapel", "wali_kelas")
	boundaryLatitude := 100 / 6371008.8 * 180 / math.Pi
	firstImage := captureJPEG(t, false)
	firstRequestKey := testutil.Random(t)[:32]
	first, err := service.CaptureTeacherAttendance(ctx, teacher.ID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: firstRequestKey, Action: academic.TeacherCheckIn, Latitude: boundaryLatitude, Longitude: 0, AccuracyM: 50,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("exact-radius check-in rejected: %v", err)
	}
	createdEventIDs = append(createdEventIDs, first.EventID)
	if first.EventID < 1 || first.TeacherID != teacherID || first.Action != academic.TeacherCheckIn || first.PhotoSizeBytes > teacherphoto.MaxPhotoBytes || first.Idempotent {
		t.Fatalf("unexpected capture proof: %+v", first)
	}
	var eventDate string
	if err = db.QueryRow(`SELECT on_date::text FROM app.teacher_attendance_events WHERE id=$1`, first.EventID).Scan(&eventDate); err != nil || eventDate != academic.TeacherAttendanceWIBDate(first.RecordedAt) {
		t.Fatalf("event date does not follow server WIB time: stored=%s proof=%s err=%v", eventDate, academic.TeacherAttendanceWIBDate(first.RecordedAt), err)
	}
	if !first.OccurredAt.Equal(first.RecordedAt) || !first.RetainUntil.Equal(academic.TeacherAttendancePhotoRetainUntil(first.RecordedAt)) {
		t.Fatalf("timestamp/retention proof mismatch: %+v", first)
	}
	var historyCount int
	var history string
	if err = db.QueryRow(`SELECT count(*),max(after_state::text) FROM app.teacher_attendance_history WHERE event_id=$1 AND action='recorded'`, first.EventID).Scan(&historyCount, &history); err != nil || historyCount != 1 {
		t.Fatalf("expected one initial history row, count=%d err=%v", historyCount, err)
	}
	if strings.Contains(history, "device_latitude") || strings.Contains(history, "device_longitude") || strings.Contains(history, "storage_key") || !strings.Contains(history, "EvidenceFingerprint") {
		t.Fatalf("history contains forbidden data or lacks retry evidence")
	}
	keys, err := store.ListKeys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("capture must create one private photo, keys=%d err=%v", len(keys), err)
	}

	retry, err := service.CaptureTeacherAttendance(ctx, teacher.ID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: firstRequestKey, Action: academic.TeacherCheckIn, Latitude: boundaryLatitude, Longitude: 0, AccuracyM: 50,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	})
	if err != nil || !retry.Idempotent || retry.EventID != first.EventID || retry.PhotoMetadataID != first.PhotoMetadataID {
		t.Fatalf("identical retry not idempotent: proof=%+v err=%v", retry, err)
	}
	keys, err = store.ListKeys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("identical retry created extra photo: keys=%d err=%v", len(keys), err)
	}
	newRequestKeyRetry, err := service.CaptureTeacherAttendance(ctx, teacher.ID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: testutil.Random(t)[:32], Action: academic.TeacherCheckIn, Latitude: boundaryLatitude, Longitude: 0, AccuracyM: 50,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	})
	if err != nil || !newRequestKeyRetry.Idempotent || newRequestKeyRetry.EventID != first.EventID {
		t.Fatalf("same-day identical evidence with a new request key was not idempotent: proof=%+v err=%v", newRequestKeyRetry, err)
	}
	changed, err := service.CaptureTeacherAttendance(ctx, teacher.ID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: firstRequestKey, Action: academic.TeacherCheckIn, Latitude: boundaryLatitude, Longitude: 0, AccuracyM: 49,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	})
	if !errors.Is(err, academic.ErrConflict) {
		t.Fatalf("different evidence should conflict, proof=%+v err=%v", changed, err)
	}
	changedCoordinate, err := service.CaptureTeacherAttendance(ctx, teacher.ID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: firstRequestKey, Action: academic.TeacherCheckIn, Latitude: -boundaryLatitude, Longitude: 0, AccuracyM: 50,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	})
	if !errors.Is(err, academic.ErrConflict) {
		t.Fatalf("different coordinates with identical distance should conflict for the same request key: proof=%+v err=%v", changedCoordinate, err)
	}
	previousDate := shiftCaptureEventToPreviousDay(t, db, first.EventID)
	crossDateRetry, err := service.CaptureTeacherAttendance(ctx, teacher.ID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: firstRequestKey, Action: academic.TeacherCheckIn, Latitude: boundaryLatitude, Longitude: 0, AccuracyM: 50,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	})
	if err != nil || !crossDateRetry.Idempotent || crossDateRetry.EventID != first.EventID || crossDateRetry.OnDate != previousDate {
		t.Fatalf("retry after simulated WIB date change did not resolve prior event: proof=%+v err=%v", crossDateRetry, err)
	}

	// Pulang tanpa Masuk sah, termasuk untuk akun Wali Kelas saja.
	homeroom, _ := captureTeacher(t, db, "wali_kelas")
	checkout, err := service.CaptureTeacherAttendance(ctx, homeroom.ID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: testutil.Random(t)[:32], Action: academic.TeacherCheckOut, Latitude: 0, Longitude: 0, AccuracyM: 1,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	})
	if err != nil || checkout.Action != academic.TeacherCheckOut {
		t.Fatalf("Wali Kelas checkout without check-in rejected: proof=%+v err=%v", checkout, err)
	}
	createdEventIDs = append(createdEventIDs, checkout.EventID)

	// Perubahan lokasi sebelum mutasi dipakai dan disimpan sebagai snapshot event.
	latest, _, err := service.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	newSchoolLatitude := 1.0
	if err = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{
		Latitude: newSchoolLatitude, Longitude: 2, RadiusM: 100, ExpectedVersion: latest.Version, Reason: "Uji snapshot lokasi",
	}); err != nil {
		t.Fatal(err)
	}
	newSchoolConfig, _, err := service.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshotTeacher, _ := captureTeacher(t, db, "guru_mapel")
	snapshotProof, err := service.CaptureTeacherAttendance(ctx, snapshotTeacher.ID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: testutil.Random(t)[:32], Action: academic.TeacherCheckIn, Latitude: newSchoolLatitude, Longitude: 2, AccuracyM: 1,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	})
	if err != nil || snapshotProof.SchoolLatitude != newSchoolLatitude || snapshotProof.GeofenceRadiusM != newSchoolConfig.RadiusM {
		t.Fatalf("new location config not captured: proof=%+v err=%v", snapshotProof, err)
	}
	createdEventIDs = append(createdEventIDs, snapshotProof.EventID)
	latest, _, _ = service.TeacherAttendanceLocation(ctx, admin.ID)
	if err = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{
		Latitude: 3, Longitude: 4, RadiusM: 150, ExpectedVersion: latest.Version, Reason: "Uji perubahan sesudah snapshot",
	}); err != nil {
		t.Fatal(err)
	}
	var snapLat, snapLon float64
	var snapRadius int
	if err = db.QueryRow(`SELECT school_latitude,school_longitude,geofence_radius_m FROM app.teacher_attendance_events WHERE id=$1`, snapshotProof.EventID).Scan(&snapLat, &snapLon, &snapRadius); err != nil || snapLat != newSchoolLatitude || snapLon != 2 || snapRadius != newSchoolConfig.RadiusM {
		t.Fatalf("location update rewrote historical snapshot: %f,%f/%d err=%v", snapLat, snapLon, snapRadius, err)
	}

	// Role selain guru mandiri, identitas tidak tertaut, dan profil nonaktif ditolak.
	deniedRoles := []string{"admin", "kurikulum", "kesiswaan", "petugas_absensi", "kepala_sekolah", "bk"}
	for _, role := range deniedRoles {
		user, _ := testutil.User(t, db, role)
		_, deniedErr := service.CaptureTeacherAttendance(ctx, user.ID, validCaptureInput(firstImage))
		if !errors.Is(deniedErr, academic.ErrForbidden) {
			t.Errorf("role %s unexpectedly allowed: %v", role, deniedErr)
		}
	}
	unlinked, _ := testutil.User(t, db, "guru_mapel")
	if _, deniedErr := service.CaptureTeacherAttendance(ctx, unlinked.ID, validCaptureInput(firstImage)); !errors.Is(deniedErr, academic.ErrForbidden) {
		t.Errorf("teacher role without linked profile unexpectedly allowed: %v", deniedErr)
	}
	inactive, _ := captureTeacher(t, db, "guru_mapel")
	if _, err = db.Exec(`UPDATE app.teachers SET active=false WHERE user_id=$1`, inactive.ID); err != nil {
		t.Fatal(err)
	}
	if _, deniedErr := service.CaptureTeacherAttendance(ctx, inactive.ID, validCaptureInput(firstImage)); !errors.Is(deniedErr, academic.ErrForbidden) {
		t.Errorf("inactive teacher profile unexpectedly allowed: %v", deniedErr)
	}
	revoked, _ := captureTeacher(t, db, "guru_mapel")
	if _, err = db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1 AND role_code='guru_mapel'`, revoked.ID); err != nil {
		t.Fatal(err)
	}
	if _, deniedErr := service.CaptureTeacherAttendance(ctx, revoked.ID, validCaptureInput(firstImage)); !errors.Is(deniedErr, academic.ErrForbidden) {
		t.Errorf("revoked teacher role unexpectedly allowed: %v", deniedErr)
	}
	inactiveAccount, _ := captureTeacher(t, db, "wali_kelas")
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, inactiveAccount.ID); err != nil {
		t.Fatal(err)
	}
	if _, deniedErr := service.CaptureTeacherAttendance(ctx, inactiveAccount.ID, validCaptureInput(firstImage)); !errors.Is(deniedErr, academic.ErrForbidden) {
		t.Errorf("inactive account unexpectedly allowed: %v", deniedErr)
	}

	// Penolakan geofence, akurasi, media, dan storage tidak menambah event atau file.
	rejectUser, rejectTeacherID := captureTeacher(t, db, "guru_mapel")
	assertCaptureRejectedWithoutWrites(t, db, service, store, rejectUser.ID, rejectTeacherID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: testutil.Random(t)[:32], Action: academic.TeacherCheckIn, Latitude: 3 + 151/6371008.8*180/math.Pi, Longitude: 4, AccuracyM: 5,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	}, academic.ValidationError("Lokasi berada di luar radius kehadiran sekolah."))
	assertCaptureRejectedWithoutWrites(t, db, service, store, rejectUser.ID, rejectTeacherID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: testutil.Random(t)[:32], Action: academic.TeacherCheckIn, Latitude: 3, Longitude: 4, AccuracyM: 50.01,
		Selfie: bytes.NewReader(firstImage), SelfieMIMEType: "image/jpeg",
	}, academic.ValidationError("Akurasi lokasi harus 50 meter atau lebih baik."))
	assertCaptureRejectedWithoutWrites(t, db, service, store, rejectUser.ID, rejectTeacherID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: testutil.Random(t)[:32], Action: academic.TeacherCheckIn, Latitude: 3, Longitude: 4, AccuracyM: 5,
		Selfie: bytes.NewReader([]byte("bukan gambar")), SelfieMIMEType: "image/jpeg",
	}, teacherphoto.ErrInvalidImage)
	assertCaptureRejectedWithoutWrites(t, db, service, store, rejectUser.ID, rejectTeacherID, academic.TeacherAttendanceCaptureInput{
		IdempotencyKey: testutil.Random(t)[:32], Action: academic.TeacherCheckIn, Latitude: 3, Longitude: 4, AccuracyM: 5,
		Selfie: bytes.NewReader(captureJPEG(t, true)), SelfieMIMEType: "image/jpeg",
	}, teacherphoto.ErrPhotoTooLarge)
	failingStore := &capturePhotoStorage{delegate: store, failSave: true}
	service.Photos = failingStore
	assertCaptureRejectedWithoutWrites(t, db, service, failingStore, rejectUser.ID, rejectTeacherID, validCaptureInput(firstImage), teacherphoto.ErrStorage)
	service.Photos = store

	// Insert histori gagal: DB rollback dan kompensasi hapus file. Jika hapus gagal,
	// rekonsiliasi ber-lock kemudian membuang file orphan tanpa menyentuh foto sah.
	historyUser, historyTeacherID := captureTeacher(t, db, "guru_mapel")
	triggerName, functionName := "test_capture_history_"+testutil.Random(t)[:12], "test_capture_history_fn_"+testutil.Random(t)[:12]
	if _, err = db.Exec(fmt.Sprintf(`CREATE FUNCTION app.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic capture history failure'; END $$`, functionName)); err != nil {
		t.Fatal("create isolated history failure fixture")
	}
	if _, err = db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON app.teacher_attendance_history FOR EACH ROW EXECUTE FUNCTION app.%s()`, triggerName, functionName)); err != nil {
		t.Fatal("install isolated history failure trigger")
	}
	defer func() {
		_, _ = db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.teacher_attendance_history`, triggerName))
		_, _ = db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS app.%s()`, functionName))
	}()
	cleanupFailStore := &capturePhotoStorage{delegate: store, failDelete: true}
	service.Photos = cleanupFailStore
	assertCaptureRejectedWithoutWrites(t, db, service, cleanupFailStore, historyUser.ID, historyTeacherID, validCaptureInput(firstImage), academic.ErrTeacherAttendanceCleanup)
	service.Photos = store
	removed, err := service.ReconcileTeacherAttendancePhotoFiles(ctx)
	if err != nil || removed != 1 {
		t.Fatalf("orphan reconciliation removed=%d err=%v", removed, err)
	}
	keys, err = store.ListKeys()
	if err != nil || len(keys) != 3 { // check-in, checkout, and location-snapshot event remain referenced
		t.Fatalf("reconciliation removed a referenced photo or left orphan: keys=%d err=%v", len(keys), err)
	}
	if _, err = db.Exec(fmt.Sprintf(`DROP TRIGGER %s ON app.teacher_attendance_history`, triggerName)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(fmt.Sprintf(`DROP FUNCTION app.%s()`, functionName)); err != nil {
		t.Fatal(err)
	}

	// Constraint SQL gagal setelah file dibuat: transaksi rollback dan file dibersihkan.
	constraintUser, constraintTeacherID := captureTeacher(t, db, "guru_mapel")
	badMetadataStore := &capturePhotoStorage{delegate: store, invalidSize: teacherphoto.MaxPhotoBytes + 1}
	service.Photos = badMetadataStore
	assertCaptureRejectedWithoutWrites(t, db, service, badMetadataStore, constraintUser.ID, constraintTeacherID, validCaptureInput(firstImage), nil)
	service.Photos = store

	// Dua request simultan identik menghasilkan satu event/foto/histori.
	concurrentUser, concurrentTeacherID := captureTeacher(t, db, "guru_mapel", "wali_kelas")
	parallelInputA, parallelInputB := validCaptureInput(firstImage), validCaptureInput(firstImage)
	results := make(chan error, 2)
	proofs := make(chan academic.TeacherAttendanceCaptureProof, 2)
	go func() {
		proof, callErr := service.CaptureTeacherAttendance(ctx, concurrentUser.ID, parallelInputA)
		proofs <- proof
		results <- callErr
	}()
	go func() {
		proof, callErr := service.CaptureTeacherAttendance(ctx, concurrentUser.ID, parallelInputB)
		proofs <- proof
		results <- callErr
	}()
	for i := 0; i < 2; i++ {
		if callErr := <-results; callErr != nil {
			t.Fatalf("concurrent identical capture failed: %v", callErr)
		}
	}
	firstConcurrent, secondConcurrent := <-proofs, <-proofs
	if firstConcurrent.EventID != secondConcurrent.EventID || firstConcurrent.Idempotent == secondConcurrent.Idempotent {
		t.Fatalf("concurrent identical requests did not converge: %+v / %+v", firstConcurrent, secondConcurrent)
	}
	createdEventIDs = append(createdEventIDs, firstConcurrent.EventID)
	var concurrentEvents, concurrentPhotos, concurrentHistory int
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1`, concurrentTeacherID).Scan(&concurrentEvents); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_photos p JOIN app.teacher_attendance_events e ON e.id=p.event_id WHERE e.teacher_id=$1`, concurrentTeacherID).Scan(&concurrentPhotos); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1`, concurrentTeacherID).Scan(&concurrentHistory); err != nil {
		t.Fatal(err)
	}
	if concurrentEvents != 1 || concurrentPhotos != 1 || concurrentHistory != 1 {
		t.Fatalf("duplicate concurrent evidence events=%d photos=%d history=%d", concurrentEvents, concurrentPhotos, concurrentHistory)
	}
	keys, err = store.ListKeys()
	if err != nil || len(keys) != 4 { // satu file untuk setiap event sah dalam tes ini
		t.Fatalf("concurrent retry published an extra photo: keys=%d err=%v", len(keys), err)
	}

	dailyAfter, lessonAfter := attendanceStudentCounts(t, db)
	if dailyAfter != dailyBefore || lessonAfter != lessonBefore {
		t.Fatalf("student attendance changed: daily %d->%d, lesson %d->%d", dailyBefore, dailyAfter, lessonBefore, lessonAfter)
	}
}

type capturePhotoStorage struct {
	delegate    *teacherphoto.Store
	failSave    bool
	failDelete  bool
	invalidSize int
}

func (s *capturePhotoStorage) Prepare(source io.Reader, mime string) (teacherphoto.PreparedPhoto, error) {
	return s.delegate.Prepare(source, mime)
}
func (s *capturePhotoStorage) SavePrepared(prepared teacherphoto.PreparedPhoto) (teacherphoto.Metadata, error) {
	if s.failSave {
		return teacherphoto.Metadata{}, teacherphoto.ErrStorage
	}
	metadata, err := s.delegate.SavePrepared(prepared)
	if err == nil && s.invalidSize > 0 {
		metadata.SizeBytes = s.invalidSize
	}
	return metadata, err
}
func (s *capturePhotoStorage) Delete(key string) error {
	if s.failDelete {
		return teacherphoto.ErrStorage
	}
	return s.delegate.Delete(key)
}
func (s *capturePhotoStorage) ListKeys() ([]string, error) { return s.delegate.ListKeys() }

func newCapturePhotoStore(t *testing.T) *teacherphoto.Store {
	t.Helper()
	base := t.TempDir()
	repository, public := filepath.Join(base, "repo"), filepath.Join(base, "public")
	private := filepath.Join(base, "private", "photos")
	for _, directory := range []string{repository, public, filepath.Dir(private)} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := teacherphoto.NewStore(teacherphoto.Config{Directory: private, RepositoryRoot: repository, PublicRoots: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func captureTeacher(t *testing.T, db *sql.DB, roles ...string) (user struct{ ID int64 }, teacherID int64) {
	t.Helper()
	created, _ := testutil.User(t, db, roles...)
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES($1,$2) RETURNING id`, "Guru sintetis "+testutil.Random(t)[:8], created.ID).Scan(&teacherID); err != nil {
		t.Fatal("create linked synthetic teacher")
	}
	user.ID = created.ID
	return user, teacherID
}

func captureJPEG(t *testing.T, noisy bool) []byte {
	t.Helper()
	width, height := 40, 30
	if noisy {
		width, height = 2000, 2000
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	rng := rand.New(rand.NewSource(314159))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if noisy {
				img.Set(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
			} else {
				img.Set(x, y, color.RGBA{uint8(x * 5), uint8(y * 8), 120, 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func validCaptureInput(jpegBytes []byte) academic.TeacherAttendanceCaptureInput {
	return academic.TeacherAttendanceCaptureInput{IdempotencyKey: "00112233445566778899aabbccddeeff", Action: academic.TeacherCheckIn, Latitude: 3, Longitude: 4, AccuracyM: 1, Selfie: bytes.NewReader(jpegBytes), SelfieMIMEType: "image/jpeg"}
}

func attendanceStudentCounts(t *testing.T, db *sql.DB) (daily, lesson int) {
	t.Helper()
	if err := db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance`).Scan(&daily); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.student_lesson_attendance`).Scan(&lesson); err != nil {
		t.Fatal(err)
	}
	return daily, lesson
}

func shiftCaptureEventToPreviousDay(t *testing.T, db *sql.DB, eventID int64) string {
	t.Helper()
	var evidence struct {
		ActorID             int64                            `json:"actor_id"`
		TeacherID           int64                            `json:"teacher_id"`
		OnDate              string                           `json:"on_date"`
		Action              academic.TeacherAttendanceAction `json:"action"`
		Source              string                           `json:"source"`
		DistanceM           float64                          `json:"distance_m"`
		AccuracyM           float64                          `json:"accuracy_m"`
		SchoolLatitude      float64                          `json:"school_latitude"`
		SchoolLongitude     float64                          `json:"school_longitude"`
		GeofenceRadiusM     int                              `json:"geofence_radius_m"`
		LocationVersion     int64                            `json:"location_version"`
		PhotoMIMEType       string                           `json:"photo_mime_type"`
		PhotoSizeBytes      int                              `json:"photo_size_bytes"`
		PhotoChecksumSHA256 string                           `json:"photo_checksum_sha256"`
	}
	var metadata struct {
		LocationVersion     int64  `json:"LocationVersion"`
		EvidenceFingerprint string `json:"EvidenceFingerprint"`
	}
	var snapshotJSON []byte
	var occurredAt, recordedAt time.Time
	if err := db.QueryRow(`
		SELECT e.teacher_id,e.actor_id,e.on_date::text,e.action,e.source,e.distance_m,e.accuracy_m,
		       e.school_latitude,e.school_longitude,e.geofence_radius_m,p.mime_type,p.size_bytes,p.checksum_sha256,
		       h.after_state,e.occurred_at,e.recorded_at
		FROM app.teacher_attendance_events e
		JOIN app.teacher_attendance_photos p ON p.id=e.photo_metadata_id
		JOIN app.teacher_attendance_history h ON h.event_id=e.id AND h.action='recorded'
		WHERE e.id=$1`, eventID).Scan(
		&evidence.TeacherID, &evidence.ActorID, &evidence.OnDate, &evidence.Action, &evidence.Source,
		&evidence.DistanceM, &evidence.AccuracyM, &evidence.SchoolLatitude, &evidence.SchoolLongitude,
		&evidence.GeofenceRadiusM, &evidence.PhotoMIMEType, &evidence.PhotoSizeBytes, &evidence.PhotoChecksumSHA256, &snapshotJSON, &occurredAt, &recordedAt); err != nil {
		t.Fatal("load event snapshot for date-boundary retry fixture")
	}
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		t.Fatal("decode event snapshot for date-boundary retry fixture")
	}
	if err := json.Unmarshal(snapshot["LocationVersion"], &metadata.LocationVersion); err != nil {
		t.Fatal("read location version from retry snapshot")
	}
	evidence.LocationVersion = metadata.LocationVersion
	if err := json.Unmarshal(snapshot["EvidenceFingerprint"], &metadata.EvidenceFingerprint); err != nil {
		t.Fatal("read evidence fingerprint from retry snapshot")
	}
	initialEvidenceBytes, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal("marshal original evidence fingerprint")
	}
	if fmt.Sprintf("%x", sha256.Sum256(initialEvidenceBytes)) != metadata.EvidenceFingerprint {
		t.Fatalf("fixture evidence did not match persisted fingerprint before date shift")
	}
	localDate, err := time.ParseInLocation("2006-01-02", evidence.OnDate, time.FixedZone("WIB", 7*60*60))
	if err != nil {
		t.Fatal("parse existing event date")
	}
	previousDate := localDate.AddDate(0, 0, -1).Format("2006-01-02")
	evidence.OnDate = previousDate
	fingerprintBytes, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal("marshal shifted evidence fingerprint")
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(fingerprintBytes))
	previousOccurred := occurredAt.Add(-24 * time.Hour)
	previousRecorded := recordedAt.Add(-24 * time.Hour)
	previousRetainUntil := academic.TeacherAttendancePhotoRetainUntil(previousRecorded)
	setJSONSnapshotField(t, snapshot, "OnDate", previousDate)
	setJSONSnapshotField(t, snapshot, "OccurredAt", previousOccurred)
	setJSONSnapshotField(t, snapshot, "RecordedAt", previousRecorded)
	setJSONSnapshotField(t, snapshot, "EvidenceFingerprint", fingerprint)
	setJSONSnapshotField(t, snapshot, "RetainUntil", previousRetainUntil)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal("marshal shifted history snapshot")
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal("begin date-boundary retry fixture")
	}
	if _, err = tx.Exec(`UPDATE app.teacher_attendance_events SET on_date=on_date-1,occurred_at=occurred_at-interval '1 day',recorded_at=recorded_at-interval '1 day' WHERE id=$1`, eventID); err != nil {
		tx.Rollback()
		t.Fatal("shift synthetic event date")
	}
	if _, err = tx.Exec(`UPDATE app.teacher_attendance_photos SET retain_until=retain_until-interval '1 day' WHERE event_id=$1`, eventID); err != nil {
		tx.Rollback()
		t.Fatal("shift synthetic retention timestamp")
	}
	if _, err = tx.Exec(`UPDATE app.teacher_attendance_history SET after_state=$2,created_at=created_at-interval '1 day' WHERE event_id=$1 AND action='recorded'`, eventID, encoded); err != nil {
		tx.Rollback()
		t.Fatal("shift synthetic history snapshot")
	}
	if err = tx.Commit(); err != nil {
		t.Fatal("commit date-boundary retry fixture")
	}
	return previousDate
}

func setJSONSnapshotField(t *testing.T, snapshot map[string]json.RawMessage, name string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal snapshot field %s", name)
	}
	snapshot[name] = encoded
}

func assertCaptureRejectedWithoutWrites(t *testing.T, db *sql.DB, service *academic.Store, photos academic.TeacherAttendancePhotoStorage, actorID, teacherID int64, input academic.TeacherAttendanceCaptureInput, expected error) {
	t.Helper()
	var eventCount, photoCount, historyCount int
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1`, teacherID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_photos p JOIN app.teacher_attendance_events e ON e.id=p.event_id WHERE e.teacher_id=$1`, teacherID).Scan(&photoCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1`, teacherID).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	keysBefore, err := photos.ListKeys()
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := service.CaptureTeacherAttendance(context.Background(), actorID, input)
	if callErr == nil || (expected != nil && !errors.Is(callErr, expected)) {
		t.Fatalf("capture error=%v, expected error=%v", callErr, expected)
	}
	var eventAfter, photoAfter, historyAfter int
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1`, teacherID).Scan(&eventAfter); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_photos p JOIN app.teacher_attendance_events e ON e.id=p.event_id WHERE e.teacher_id=$1`, teacherID).Scan(&photoAfter); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1`, teacherID).Scan(&historyAfter); err != nil {
		t.Fatal(err)
	}
	keysAfter, err := photos.ListKeys()
	if err != nil {
		t.Fatal(err)
	}
	if eventAfter != eventCount || photoAfter != photoCount || historyAfter != historyCount || (expected != academic.ErrTeacherAttendanceCleanup && len(keysAfter) != len(keysBefore)) {
		t.Fatalf("rejected capture changed persisted state: event %d->%d photo %d->%d history %d->%d files %d->%d", eventCount, eventAfter, photoCount, photoAfter, historyCount, historyAfter, len(keysBefore), len(keysAfter))
	}
}
