//go:build integration

package academic_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestPurgeExpiredStaffAttendancePhotosIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	teacherPhotos, staffPhotos := newCapturePhotoStore(t), newCapturePhotoStore(t)
	service := &academic.Store{DB: db, Photos: teacherPhotos, StaffPhotos: staffPhotos}
	admin, _ := testutil.User(t, db, "admin")
	initial, _, err := service.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: -6.2, Longitude: 106.8, RadiusM: 100, ExpectedVersion: initial.Version, Reason: "Fixture sintetis retensi foto pegawai"}); err != nil {
		t.Fatal(err)
	}
	if initial.Configured {
		t.Cleanup(func() {
			latest, _, e := service.TeacherAttendanceLocation(ctx, admin.ID)
			if e == nil {
				_ = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: initial.Latitude, Longitude: initial.Longitude, RadiusM: initial.RadiusM, ExpectedVersion: latest.Version, Reason: "Pulihkan lokasi sesudah test retensi TU"})
			}
		})
	} else {
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location_history WHERE location_id=1`)
			_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location WHERE id=1`)
		})
	}
	teacherUser, teacherID := captureTeacher(t, db, "guru_mapel")
	staffUser, _ := testutil.User(t, db, "pegawai_tu")
	var staffID int64
	if err = db.QueryRow(`INSERT INTO app.staff_members(name,user_id) VALUES('Pegawai retensi sintetis',$1) RETURNING id`, staffUser.ID).Scan(&staffID); err != nil {
		t.Fatal(err)
	}
	var teacherEvent, staffEvent int64
	t.Cleanup(func() {
		tx, e := db.Begin()
		if e != nil {
			t.Error(e)
			return
		}
		defer tx.Rollback()
		for _, q := range []string{`DELETE FROM app.staff_attendance_history WHERE event_id=$1`, `DELETE FROM app.staff_attendance_photos WHERE event_id=$1`, `DELETE FROM app.staff_attendance_events WHERE id=$1`} {
			if _, e = tx.Exec(q, staffEvent); e != nil {
				t.Error(e)
				return
			}
		}
		for _, q := range []string{`DELETE FROM app.teacher_attendance_history WHERE event_id=$1`, `DELETE FROM app.teacher_attendance_photos WHERE event_id=$1`, `DELETE FROM app.teacher_attendance_events WHERE id=$1`} {
			if _, e = tx.Exec(q, teacherEvent); e != nil {
				t.Error(e)
				return
			}
		}
		if _, e = tx.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID); e != nil {
			t.Error(e)
			return
		}
		if _, e = tx.Exec(`DELETE FROM app.staff_members WHERE id=$1`, staffID); e != nil {
			t.Error(e)
			return
		}
		for _, id := range []int64{staffUser.ID, teacherUser.ID} {
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
		keys, _ := teacherPhotos.ListKeys()
		for _, key := range keys {
			_ = teacherPhotos.Delete(key)
		}
		keys, _ = staffPhotos.ListKeys()
		for _, key := range keys {
			_ = staffPhotos.Delete(key)
		}
	})
	imageBytes := captureJPEG(t, false)
	teacherProof, err := service.CaptureTeacherAttendance(ctx, teacherUser.ID, academic.TeacherAttendanceCaptureInput{IdempotencyKey: "00112233445566778899aabbccddeeff", Action: academic.TeacherCheckIn, Latitude: -6.2, Longitude: 106.8, AccuracyM: 5, Selfie: bytes.NewReader(imageBytes), SelfieMIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	teacherEvent = teacherProof.EventID
	staffProof, err := service.CaptureStaffAttendance(ctx, staffUser.ID, staffCaptureInput(t, "102132435465768798a9bacbdcedfe0f", academic.TeacherCheckIn, -6.2, 106.8, 5, imageBytes))
	if err != nil {
		t.Fatal(err)
	}
	staffEvent = staffProof.ID
	if !staffProof.RetainUntil.Equal(academic.TeacherAttendancePhotoRetainUntil(staffProof.RecordedAt)) {
		t.Fatal("staff photo retention is not two calendar months")
	}
	var teacherKey, staffKey string
	var teacherPhotoID, staffPhotoID int64
	if err = db.QueryRow(`SELECT id,storage_key FROM app.teacher_attendance_photos WHERE event_id=$1`, teacherEvent).Scan(&teacherPhotoID, &teacherKey); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id,storage_key FROM app.staff_attendance_photos WHERE event_id=$1`, staffEvent).Scan(&staffPhotoID, &staffKey); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.staff_attendance_photos SET retain_until=created_at WHERE id=$1`, staffPhotoID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.teacher_attendance_photos SET retain_until=created_at WHERE id=$1`, teacherPhotoID); err != nil {
		t.Fatal(err)
	}
	var teacherEventBefore, teacherHistoryBefore, staffEventBefore, staffHistoryBefore int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.teacher_attendance_events WHERE id=$1),(SELECT count(*) FROM app.teacher_attendance_history WHERE event_id=$1),(SELECT count(*) FROM app.staff_attendance_events WHERE id=$2),(SELECT count(*) FROM app.staff_attendance_history WHERE event_id=$2)`, teacherEvent, staffEvent).Scan(&teacherEventBefore, &teacherHistoryBefore, &staffEventBefore, &staffHistoryBefore); err != nil {
		t.Fatal(err)
	}
	result, err := service.PurgeExpiredStaffAttendancePhotos(ctx, 50)
	if err != nil || result.Examined != 1 || result.Purged != 1 || result.Failed != 0 {
		t.Fatalf("staff purge result=%+v err=%v", result, err)
	}
	if _, err = service.StaffPhotos.Open(staffKey); !errors.Is(err, teacherphoto.ErrNotFound) {
		t.Fatal("expired staff photo file remains available")
	}
	teacherFile, err := teacherPhotos.Open(teacherKey)
	if err != nil {
		t.Fatalf("staff purge touched teacher photo: %v", err)
	}
	_, _ = io.Copy(io.Discard, teacherFile)
	_ = teacherFile.Close()
	var staffDeleted, teacherDeleted sql.NullTime
	var staffEvents, staffHistory, teacherEvents, teacherHistory int
	if err = db.QueryRow(`SELECT (SELECT deleted_at FROM app.staff_attendance_photos WHERE id=$1),(SELECT deleted_at FROM app.teacher_attendance_photos WHERE id=$2),(SELECT count(*) FROM app.staff_attendance_events WHERE id=$3),(SELECT count(*) FROM app.staff_attendance_history WHERE event_id=$3),(SELECT count(*) FROM app.teacher_attendance_events WHERE id=$4),(SELECT count(*) FROM app.teacher_attendance_history WHERE event_id=$4)`, staffPhotoID, teacherPhotoID, staffEvent, teacherEvent).Scan(&staffDeleted, &teacherDeleted, &staffEvents, &staffHistory, &teacherEvents, &teacherHistory); err != nil {
		t.Fatal(err)
	}
	if !staffDeleted.Valid || teacherDeleted.Valid || staffEvents != staffEventBefore || staffHistory != staffHistoryBefore || teacherEvents != teacherEventBefore || teacherHistory != teacherHistoryBefore {
		t.Fatal("retention changed immutable event, history, or teacher photo metadata")
	}
	if _, _, err = service.StaffAttendancePhoto(ctx, staffUser.ID, staffPhotoID); !errors.Is(err, academic.ErrNotFound) {
		t.Fatalf("expired staff evidence served after retention: %v", err)
	}
}
