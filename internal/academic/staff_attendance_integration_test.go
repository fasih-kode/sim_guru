//go:build integration

package academic_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStaffAttendanceCaptureIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	service := &academic.Store{DB: db, StaffPhotos: newCapturePhotoStore(t)}
	admin, _ := testutil.User(t, db, "admin")
	initial, _, err := service.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil {
		t.Fatal("read initial school location")
	}
	if err = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: 0, Longitude: 0, RadiusM: 100, ExpectedVersion: initial.Version, Reason: "Fixture sintetis kehadiran pegawai"}); err != nil {
		t.Fatal("configure isolated test location")
	}
	if initial.Configured {
		t.Cleanup(func() {
			latest, _, readErr := service.TeacherAttendanceLocation(ctx, admin.ID)
			if readErr == nil {
				_ = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: initial.Latitude, Longitude: initial.Longitude, RadiusM: initial.RadiusM, ExpectedVersion: latest.Version, Reason: "Pulihkan konfigurasi setelah test"})
			}
		})
	} else {
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location_history WHERE location_id=1`)
			_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location WHERE id=1`)
		})
	}
	users := []int64{admin.ID}
	staffIDs, eventIDs := []int64{}, []int64{}
	t.Cleanup(func() {
		for _, staffID := range staffIDs {
			rows, queryErr := db.Query(`SELECT id FROM app.staff_attendance_events WHERE staff_member_id=$1`, staffID)
			if queryErr != nil {
				t.Errorf("find synthetic events for cleanup: %v", queryErr)
				continue
			}
			for rows.Next() {
				var id int64
				if scanErr := rows.Scan(&id); scanErr != nil {
					t.Errorf("read synthetic event id: %v", scanErr)
					break
				}
				eventIDs = append(eventIDs, id)
			}
			if rowsErr := rows.Err(); rowsErr != nil {
				t.Errorf("read synthetic event rows: %v", rowsErr)
			}
			_ = rows.Close()
		}
		seenEvents := map[int64]bool{}
		for _, id := range eventIDs {
			if seenEvents[id] {
				continue
			}
			seenEvents[id] = true
			tx, beginErr := db.Begin()
			if beginErr != nil {
				t.Errorf("begin synthetic event cleanup: %v", beginErr)
				continue
			}
			for _, statement := range []string{`DELETE FROM app.staff_attendance_history WHERE event_id=$1`, `DELETE FROM app.staff_attendance_photos WHERE event_id=$1`, `DELETE FROM app.staff_attendance_events WHERE id=$1`} {
				if _, cleanupErr := tx.Exec(statement, id); cleanupErr != nil {
					_ = tx.Rollback()
					t.Errorf("cleanup synthetic staff event: %v", cleanupErr)
					break
				}
			}
			if cleanupErr := tx.Commit(); cleanupErr != nil {
				t.Errorf("commit synthetic event cleanup: %v", cleanupErr)
			}
		}
		for _, id := range staffIDs {
			if _, cleanupErr := db.Exec(`DELETE FROM app.staff_members WHERE id=$1`, id); cleanupErr != nil {
				t.Errorf("cleanup synthetic staff identity: %v", cleanupErr)
			}
		}
		for _, id := range users {
			if id != admin.ID {
				_, _ = db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1`, id)
				_, _ = db.Exec(`DELETE FROM app.users WHERE id=$1`, id)
			}
		}
		keys, _ := service.StaffPhotos.ListKeys()
		for _, key := range keys {
			_ = service.StaffPhotos.Delete(key)
		}
	})
	newStaff := func(name string, roles []string, active bool) (int64, int64) {
		t.Helper()
		u, _ := testutil.User(t, db, roles...)
		users = append(users, u.ID)
		var id int64
		if err := db.QueryRow(`INSERT INTO app.staff_members(name,user_id,active) VALUES($1,$2,$3) RETURNING id`, name, u.ID, active).Scan(&id); err != nil {
			t.Fatal(err)
		}
		staffIDs = append(staffIDs, id)
		return u.ID, id
	}
	userA, staffA := newStaff("Pegawai Sintetis A", []string{"pegawai_tu", "guru_mapel"}, true)
	userB, staffB := newStaff("Pegawai Sintetis B", []string{"pegawai_tu"}, true)
	userInactiveStaff, _ := newStaff("Pegawai Nonaktif", []string{"pegawai_tu"}, false)
	userUnlinked, _ := testutil.User(t, db, "pegawai_tu")
	users = append(users, userUnlinked.ID)
	userNoTU, _ := testutil.User(t, db, "guru_mapel")
	users = append(users, userNoTU.ID)
	userInactiveAccount, _ := testutil.User(t, db, "pegawai_tu")
	users = append(users, userInactiveAccount.ID)
	var inactiveStaffID int64
	if err = db.QueryRow(`INSERT INTO app.staff_members(name,user_id,active) VALUES('Pegawai Akun Nonaktif',$1,true) RETURNING id`, userInactiveAccount.ID).Scan(&inactiveStaffID); err != nil {
		t.Fatal(err)
	}
	staffIDs = append(staffIDs, inactiveStaffID)
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, userInactiveAccount.ID); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []int64{userInactiveStaff, userUnlinked.ID, userNoTU.ID, userInactiveAccount.ID} {
		_, callErr := service.CaptureStaffAttendance(ctx, actor, staffCaptureInput(t, "00112233445566778899aabbccddeeff", academic.TeacherCheckIn, 0, 0, 10, captureJPEG(t, false)))
		if !errors.Is(callErr, academic.ErrForbidden) {
			t.Fatalf("actor %d should not be able to capture: %v", actor, callErr)
		}
	}

	day, err := service.StaffAttendanceToday(ctx, userA)
	if err != nil || day.StaffID != staffA || day.Name != "Pegawai Sintetis A" || day.Date == "" {
		t.Fatalf("own daily identity unavailable: %+v err=%v", day, err)
	}
	const key = "102132435465768798a9bacbdcedfe0f"
	imageBytes := captureJPEG(t, false)
	boundaryLatitude := 100 / 6371008.8 * 180 / math.Pi
	var wg sync.WaitGroup
	proofs := make([]academic.StaffAttendanceEvent, 2)
	errCh := make(chan error, 2)
	for i := range proofs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			proof, captureErr := service.CaptureStaffAttendance(ctx, userA, staffCaptureInput(t, key, academic.TeacherCheckIn, boundaryLatitude, 0, 50, imageBytes))
			proofs[i] = proof
			errCh <- captureErr
		}(i)
	}
	wg.Wait()
	close(errCh)
	for callErr := range errCh {
		if callErr != nil {
			t.Fatalf("concurrent identical check-in failed: %v; results=%+v", callErr, proofs)
		}
	}
	if proofs[0].ID < 1 || proofs[0].ID != proofs[1].ID || proofs[0].PhotoID != proofs[1].PhotoID || proofs[0].Action != academic.TeacherCheckIn || !proofs[0].OccurredAt.Equal(proofs[0].RecordedAt) {
		t.Fatalf("concurrent retry did not resolve to one immutable event: %+v / %+v", proofs[0], proofs[1])
	}
	eventIDs = append(eventIDs, proofs[0].ID)
	var eventCount, photoCount, historyCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.staff_attendance_events WHERE staff_member_id=$1 AND on_date=$2::date AND action='check_in'`, staffA, proofs[0].Date).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("check-in was not unique: count=%d err=%v", eventCount, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.staff_attendance_photos WHERE event_id=$1`, proofs[0].ID).Scan(&photoCount); err != nil || photoCount != 1 {
		t.Fatalf("retry created duplicate photo metadata: count=%d err=%v", photoCount, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.staff_attendance_history WHERE event_id=$1`, proofs[0].ID).Scan(&historyCount); err != nil || historyCount != 1 {
		t.Fatalf("capture history count=%d err=%v", historyCount, err)
	}
	if _, _, err = service.StaffAttendancePhoto(ctx, userB, proofs[0].PhotoID); !errors.Is(err, academic.ErrNotFound) {
		t.Fatalf("cross-staff photo access was not hidden: %v", err)
	}
	photo, mime, err := service.StaffAttendancePhoto(ctx, userA, proofs[0].PhotoID)
	if err != nil || mime != "image/jpeg" {
		t.Fatalf("owner photo access failed: mime=%q err=%v", mime, err)
	}
	_ = photo.Close()
	historyA, err := service.StaffAttendanceOwnHistory(ctx, userA, 1)
	if err != nil || len(historyA.Events) != 1 || historyA.Events[0].ID != proofs[0].ID {
		t.Fatalf("own history did not return capture: %+v err=%v", historyA, err)
	}
	historyB, err := service.StaffAttendanceOwnHistory(ctx, userB, 1)
	if err != nil || len(historyB.Events) != 0 {
		t.Fatalf("history leaked across staff: %+v err=%v", historyB, err)
	}
	day, err = service.StaffAttendanceToday(ctx, userA)
	if err != nil || len(day.Events) != 1 || day.Events[0].Date != proofs[0].Date || academic.TeacherAttendanceWIBDate(day.Events[0].RecordedAt) != day.Events[0].Date {
		t.Fatalf("today state/date not derived in WIB: %+v err=%v", day, err)
	}

	// Pulang tanpa Masuk tetap diizinkan untuk identitas pegawai yang berbeda.
	checkout, err := service.CaptureStaffAttendance(ctx, userB, staffCaptureInput(t, "2031425364758697a8b9cadbecfd0e1f", academic.TeacherCheckOut, 0, 0, 1, imageBytes))
	if err != nil || checkout.Action != academic.TeacherCheckOut {
		t.Fatalf("check-out without check-in rejected: %+v err=%v", checkout, err)
	}
	eventIDs = append(eventIDs, checkout.ID)
	dayB, err := service.StaffAttendanceToday(ctx, userB)
	if err != nil || dayB.StaffID != staffB || len(dayB.Events) != 1 || dayB.Events[0].Action != academic.TeacherCheckOut {
		t.Fatalf("check-out only state missing: %+v err=%v", dayB, err)
	}

	badInputs := []struct {
		name  string
		input academic.StaffAttendanceCaptureInput
	}{
		{"geofence outside", staffCaptureInput(t, testutil.Random(t)[:32], academic.TeacherCheckIn, .01, 0, 1, imageBytes)},
		{"gps accuracy", staffCaptureInput(t, testutil.Random(t)[:32], academic.TeacherCheckIn, 0, 0, 50.01, imageBytes)},
		{"invalid selfie", staffCaptureInput(t, testutil.Random(t)[:32], academic.TeacherCheckIn, 0, 0, 1, []byte("not an image"))},
		{"oversized selfie", staffCaptureInput(t, testutil.Random(t)[:32], academic.TeacherCheckIn, 0, 0, 1, bytes.Repeat([]byte("x"), (5<<20)+1))},
	}
	for _, tc := range badInputs {
		if _, callErr := service.CaptureStaffAttendance(ctx, userA, tc.input); callErr == nil {
			t.Fatalf("%s input should fail", tc.name)
		}
	}
	if _, err = db.Exec(`UPDATE app.staff_members SET active=false WHERE id=$1`, staffA); err != nil {
		t.Fatal(err)
	}
	if _, err = service.CaptureStaffAttendance(ctx, userA, staffCaptureInput(t, testutil.Random(t)[:32], academic.TeacherCheckOut, 0, 0, 1, imageBytes)); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("deactivated staff could capture: %v", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events`).Scan(&eventCount); err != nil {
		t.Fatal("teacher attendance read failed")
	}
}

func staffCaptureInput(t *testing.T, key string, action academic.TeacherAttendanceAction, latitude, longitude, accuracy float64, selfie []byte) academic.StaffAttendanceCaptureInput {
	t.Helper()
	return academic.StaffAttendanceCaptureInput{IdempotencyKey: key, Action: action, Latitude: latitude, Longitude: longitude, AccuracyM: accuracy, Selfie: bytes.NewReader(selfie), SelfieMIMEType: "image/jpeg"}
}
