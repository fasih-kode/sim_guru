//go:build integration

package academic_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeacherAttendanceLocationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := &academic.Store{DB: db}
	admin, _ := testutil.User(t, db, "admin")
	admin2, _ := testutil.User(t, db, "admin", "kurikulum")
	inactive, _ := testutil.User(t, db, "admin")
	if _, err := db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, inactive.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTeacherAttendanceLocation(ctx, inactive.ID, academic.TeacherAttendanceLocationInput{Latitude: -6.2, Longitude: 106.8, RadiusM: 100, ExpectedVersion: 0, Reason: "akun nonaktif"}); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("inactive admin was not forbidden: %v", err)
	}
	var dailyBefore int
	if err := db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance`).Scan(&dailyBefore); err != nil {
		t.Fatal(err)
	}
	roles := []string{"kurikulum", "kesiswaan", "petugas_absensi", "kepala_sekolah", "guru_mapel", "wali_kelas"}
	for _, role := range roles {
		u, _ := testutil.User(t, db, role)
		if err := s.SaveTeacherAttendanceLocation(ctx, u.ID, academic.TeacherAttendanceLocationInput{Latitude: -6.2, Longitude: 106.8, RadiusM: 100, ExpectedVersion: 0, Reason: "tidak berwenang"}); !errors.Is(err, academic.ErrForbidden) {
			t.Fatalf("role %s was not forbidden: %v", role, err)
		}
	}
	config, history, err := s.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	initial := config
	if !config.Configured {
		if config.Version != 0 || config.RadiusM != 100 || len(history) != 0 {
			t.Fatalf("unexpected unconfigured state: %+v history=%d", config, len(history))
		}
		if err = s.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: -6.2, Longitude: 106.8, RadiusM: 100, ExpectedVersion: 0, Reason: "Titik sekolah awal"}); err != nil {
			t.Fatal(err)
		}
		config, history, err = s.TeacherAttendanceLocation(ctx, admin.ID)
		if err != nil || !config.Configured || config.Version != 1 || config.RadiusM != 100 || len(history) != 1 || history[0].Before != "" {
			t.Fatalf("first save mismatch config=%+v history=%+v err=%v", config, history, err)
		}
	} else if config.Version == 0 {
		t.Fatalf("configured row has invalid version: %+v", config)
	}
	base := config
	initialHistory := len(history)
	if err = s.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: config.Latitude, Longitude: config.Longitude, RadiusM: config.RadiusM, ExpectedVersion: config.Version, Reason: "Tidak ada perubahan"}); err != nil {
		t.Fatal(err)
	}
	_, history, err = s.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil || len(history) != initialHistory {
		t.Fatalf("no-op added history: %d err=%v", len(history), err)
	}
	stale := academic.TeacherAttendanceLocationInput{Latitude: -6.21, Longitude: 106.8, RadiusM: 120, ExpectedVersion: base.Version - 1, Reason: "Versi basi"}
	if err = s.SaveTeacherAttendanceLocation(ctx, admin.ID, stale); !errors.Is(err, academic.ErrConflict) {
		t.Fatalf("stale version error=%v", err)
	}
	if err = s.SaveTeacherAttendanceLocation(ctx, admin2.ID, academic.TeacherAttendanceLocationInput{Latitude: -6.21, Longitude: 106.8, RadiusM: 120, ExpectedVersion: base.Version, Reason: "Penyesuaian titik"}); err != nil {
		t.Fatal(err)
	}
	config, history, err = s.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil || config.Version != base.Version+1 || len(history) != initialHistory+1 || history[0].Before == "" || history[0].After == "" {
		t.Fatalf("update history mismatch config=%+v history=%+v err=%v", config, history, err)
	}
	if err = s.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: 91, Longitude: 0, RadiusM: 100, ExpectedVersion: config.Version, Reason: "Tidak valid"}); err == nil {
		t.Fatal("invalid coordinate unexpectedly accepted")
	}
	configAfterInvalid, historyAfterInvalid, err := s.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil || configAfterInvalid.Version != config.Version || configAfterInvalid.Latitude != config.Latitude || len(historyAfterInvalid) != len(history) {
		t.Fatalf("invalid save changed config/history: config=%+v history=%d err=%v", configAfterInvalid, len(historyAfterInvalid), err)
	}
	var dailyAfter int
	if err = db.QueryRow(`SELECT count(*) FROM app.student_daily_attendance`).Scan(&dailyAfter); err != nil || dailyAfter != dailyBefore {
		t.Fatalf("daily attendance changed before=%d after=%d err=%v", dailyBefore, dailyAfter, err)
	}
	// Memastikan insert histori gagal membatalkan perubahan konfigurasi dalam transaksi yang sama.
	suffix := testutil.Random(t)[:12]
	trigger, fn := "test_location_history_"+suffix, "test_location_history_fn_"+suffix
	if _, err = db.Exec(fmt.Sprintf(`CREATE FUNCTION app.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic history failure'; END $$`, fn)); err != nil {
		t.Fatal("could not create isolated history failure fixture")
	}
	if _, err = db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON app.teacher_attendance_location_history FOR EACH ROW EXECUTE FUNCTION app.%s()`, trigger, fn)); err != nil {
		t.Fatal("could not create isolated history failure trigger")
	}
	t.Cleanup(func() {
		_, _ = db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.teacher_attendance_location_history`, trigger))
		_, _ = db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS app.%s()`, fn))
	})
	if err = s.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: -6.22, Longitude: 106.8, RadiusM: 130, ExpectedVersion: config.Version, Reason: "Harus rollback"}); err == nil {
		t.Fatal("history failure unexpectedly committed")
	}
	configAfterFailure, _, err := s.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil || configAfterFailure.Version != config.Version || configAfterFailure.Latitude != config.Latitude {
		t.Fatalf("failed history changed config: %+v err=%v", configAfterFailure, err)
	}
	if _, err = db.Exec(fmt.Sprintf(`DROP TRIGGER %s ON app.teacher_attendance_location_history`, trigger)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(fmt.Sprintf(`DROP FUNCTION app.%s()`, fn)); err != nil {
		t.Fatal(err)
	}

	// Dua pembaruan dengan versi sama harus menghasilkan satu sukses dan satu konflik.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- s.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: -6.23 - float64(i)/100, Longitude: 106.8, RadiusM: 140 + i, ExpectedVersion: config.Version, Reason: "Pembaruan bersamaan"})
		}(i)
	}
	wg.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for result := range results {
		if result == nil {
			succeeded++
		} else if errors.Is(result, academic.ErrConflict) {
			conflicted++
		} else {
			t.Fatalf("unexpected concurrent result: %v", result)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent results success=%d conflict=%d", succeeded, conflicted)
	}
	if initial.Configured {
		latest, _, loadErr := s.TeacherAttendanceLocation(ctx, admin.ID)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if err = s.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: initial.Latitude, Longitude: initial.Longitude, RadiusM: initial.RadiusM, ExpectedVersion: latest.Version, Reason: "Pulihkan konfigurasi awal setelah tes"}); err != nil {
			t.Fatal(err)
		}
	}
	testTeacherAttendancePhotoConstraint(t, db, admin.ID)
}

func testTeacherAttendancePhotoConstraint(t *testing.T, db *sql.DB, actor int64) {
	t.Helper()
	var teacherID int64
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES($1,$2) RETURNING id`, "Guru GPS "+testutil.Random(t)[:8], actor).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	onDate := time.Now().In(jakarta).Format("2006-01-02")
	occurred := time.Now().UTC()
	var photoID int64
	if err := db.QueryRow(`SELECT nextval(pg_get_serial_sequence('app.teacher_attendance_photos','id'))`).Scan(&photoID); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var eventID int64
	err = tx.QueryRow(`INSERT INTO app.teacher_attendance_events(teacher_id,teacher_name,on_date,action,source,actor_id,occurred_at,distance_m,accuracy_m,school_latitude,school_longitude,geofence_radius_m,photo_metadata_id) VALUES($1,'Guru Uji',$2,'check_in','gps_selfie',$3,$4,10,5,-6.2,106.8,100,$5) RETURNING id`, teacherID, onDate, actor, occurred, photoID).Scan(&eventID)
	if err != nil {
		tx.Rollback()
		t.Fatalf("GPS event fixture insert failed before deferred evidence check: %v", err)
	}
	if err = tx.Commit(); err == nil {
		t.Fatal("GPS/selfie event committed without corresponding photo metadata")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events WHERE teacher_id=$1`, teacherID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("event without photo persisted: count=%d err=%v", count, err)
	}
	// Pasangan metadata yang cocok lolos; bytes foto memang tidak disimpan di database.
	if err = db.QueryRow(`SELECT nextval(pg_get_serial_sequence('app.teacher_attendance_photos','id'))`).Scan(&photoID); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = tx.QueryRow(`INSERT INTO app.teacher_attendance_events(teacher_id,teacher_name,on_date,action,source,actor_id,occurred_at,distance_m,accuracy_m,school_latitude,school_longitude,geofence_radius_m,photo_metadata_id) VALUES($1,'Guru Uji',$2,'check_in','gps_selfie',$3,$4,10,5,-6.2,106.8,100,$5) RETURNING id`, teacherID, onDate, actor, occurred, photoID).Scan(&eventID)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	_, err = tx.Exec(`INSERT INTO app.teacher_attendance_photos(id,event_id,storage_key,mime_type,size_bytes,checksum_sha256,retain_until) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,'image/jpeg',100,'`+strings.Repeat("0", 64)+`',clock_timestamp()+interval '2 months')`, photoID, eventID, testutil.Random(t))
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("valid event/photo metadata pair failed: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO app.teacher_attendance_events(teacher_id,teacher_name,on_date,action,source,actor_id,occurred_at,manual_reason) VALUES($1,'Guru Uji',$2,'check_in','manual',$3,$4,'duplikasi uji')`, teacherID, onDate, actor, occurred); err == nil {
		t.Fatal("duplicate teacher/date/action was accepted")
	}
}
