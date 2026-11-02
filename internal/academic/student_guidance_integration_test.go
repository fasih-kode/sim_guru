//go:build integration

package academic_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/studentaffairs"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentGuidanceAuthorizationHistoryAndConcurrencyIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	store := &academic.Store{DB: db}
	kes, _ := testutil.User(t, db, "kesiswaan")
	head, _ := testutil.User(t, db, "kepala_sekolah")
	admin, _ := testutil.User(t, db, "admin")
	oldWali, _ := testutil.User(t, db, "wali_kelas")
	newWali, _ := testutil.User(t, db, "wali_kelas", "bk")
	bkOnly, _ := testutil.User(t, db, "bk")
	guruOnly, _ := testutil.User(t, db, "guru_mapel")
	kurikulum, _ := testutil.User(t, db, "kurikulum")
	users := []int64{kes.ID, head.ID, admin.ID, oldWali.ID, newWali.ID, bkOnly.ID, guruOnly.ID, kurikulum.ID}
	var yearID, classA, classB, studentID int64
	var placementID, oldTeacherID, newTeacherID, recordID int64
	suffix := testutil.Random(t)[:12]
	yearID, eventDate := testutil.ActiveCalendar(t, db, suffix)
	t.Cleanup(func() {
		if recordID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_guidance_history WHERE record_id=$1`, recordID)
			_, _ = db.Exec(`DELETE FROM app.student_guidance_records WHERE id=$1`, recordID)
		}
		if placementID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_class_moves WHERE placement_id=$1`, placementID)
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, placementID)
		}
		if classA > 0 || classB > 0 {
			_, _ = db.Exec(`DELETE FROM app.homeroom_assignment_history WHERE class_id=ANY($1::bigint[])`, []int64{classA, classB})
			_, _ = db.Exec(`DELETE FROM app.homeroom_assignments WHERE class_id=ANY($1::bigint[])`, []int64{classA, classB})
		}
		if oldTeacherID > 0 || newTeacherID > 0 {
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=ANY($1::bigint[])`, []int64{oldTeacherID, newTeacherID})
		}
		if studentID > 0 {
			_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, studentID)
		}
		if classA > 0 || classB > 0 {
			_, _ = db.Exec(`DELETE FROM app.classes WHERE id=ANY($1::bigint[])`, []int64{classA, classB})
		}
		_, _ = db.Exec(`DELETE FROM app.user_roles WHERE user_id=ANY($1::bigint[])`, users)
		_, _ = db.Exec(`DELETE FROM app.users WHERE id=ANY($1::bigint[])`, users)
	})

	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "A"+suffix).Scan(&classA); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "B"+suffix).Scan(&classB); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali lama "+suffix, oldWali.ID).Scan(&oldTeacherID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali baru "+suffix, newWali.ID).Scan(&newTeacherID); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ class, teacher int64 }{{classA, oldTeacherID}, {classB, newTeacherID}} {
		if _, err := db.Exec(`INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3)`, item.class, item.teacher, admin.ID); err != nil {
			t.Fatal("create homeroom assignment fixture")
		}
	}
	if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','P') RETURNING id`, "Siswa pembinaan "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01') RETURNING id`, yearID, studentID, classA, admin.ID).Scan(&placementID); err != nil {
		t.Fatal(err)
	}
	input := studentaffairs.GuidanceInput{StudentID: studentID, Date: eventDate, Incident: "Kejadian awal", Action: "Pembinaan awal", Status: studentaffairs.GuidanceNeedsFollowUp}
	record, err := store.CreateStudentGuidance(ctx, kes.ID, input)
	if err != nil {
		t.Fatalf("create guidance record: %v", err)
	}
	recordID = record.ID
	if record.Version != 1 || record.RecordedByID != kes.ID || record.RecordedByName != kes.DisplayName || record.ClassIDAtEvent != classA || record.ClassNameAtEvent != "7A"+suffix {
		t.Fatalf("invalid actor or event snapshot: %+v", record)
	}
	if history, err := store.StudentGuidanceHistory(ctx, kes.ID, record.ID); err != nil || len(history) != 1 || history[0].Change != studentaffairs.GuidanceRecorded || history[0].Before != nil || history[0].After.ClassIDAtEvent != classA {
		t.Fatalf("initial history missing: %+v err=%v", history, err)
	}
	if _, err = store.GetStudentGuidance(ctx, head.ID, record.ID); err != nil {
		t.Fatalf("headmaster read denied: %v", err)
	}
	if _, err = store.GetStudentGuidance(ctx, oldWali.ID, record.ID); err != nil {
		t.Fatalf("current homeroom read denied: %v", err)
	}
	for _, denied := range []int64{admin.ID, bkOnly.ID, guruOnly.ID} {
		if _, err = store.ListStudentGuidance(ctx, denied, studentaffairs.GuidanceFilter{Page: 1}); !errors.Is(err, studentaffairs.ErrGuidanceForbidden) {
			t.Fatalf("role without guidance access %d was allowed: %v", denied, err)
		}
	}
	if _, err = store.GetStudentGuidance(ctx, newWali.ID, record.ID); !errors.Is(err, studentaffairs.ErrGuidanceNotFound) {
		t.Fatalf("unassigned new homeroom read before transfer: %v", err)
	}
	if _, err = store.UpdateStudentGuidance(ctx, head.ID, record.ID, 1, studentaffairs.GuidanceContentChange{Incident: "Dilarang", Action: "Dilarang", Reason: "Tidak boleh"}); !errors.Is(err, studentaffairs.ErrGuidanceForbidden) {
		t.Fatalf("headmaster mutation was not forbidden: %v", err)
	}

	moveDate := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -1).Format("2006-01-02")
	if _, err = db.Exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,$4,'Uji perpindahan terisolasi',$5)`, placementID, classA, classB, moveDate, kurikulum.ID); err != nil {
		t.Fatal("create transfer fixture")
	}
	if got, err := store.GetStudentGuidance(ctx, oldWali.ID, record.ID); !errors.Is(err, studentaffairs.ErrGuidanceNotFound) {
		t.Fatalf("former homeroom retained historical access: record=%+v err=%v", got, err)
	}
	if got, err := store.GetStudentGuidance(ctx, newWali.ID, record.ID); err != nil || got.ClassIDAtEvent != classA || got.ClassNameAtEvent != "7A"+suffix {
		t.Fatalf("new homeroom did not inherit current read scope or snapshot changed: record=%+v err=%v", got, err)
	}
	if got, err := store.ListStudentGuidance(ctx, newWali.ID, studentaffairs.GuidanceFilter{Page: 1}); err != nil || len(got.Records) != 1 || got.Records[0].ID != record.ID {
		t.Fatalf("new homeroom list did not follow active placement: %+v err=%v", got, err)
	}
	if got, err := store.ListStudentGuidance(ctx, newWali.ID, studentaffairs.GuidanceFilter{Page: 1, Search: "tidak cocok"}); err != nil || len(got.Records) != 0 {
		t.Fatalf("server search did not filter records: %+v err=%v", got, err)
	}

	change := studentaffairs.GuidanceContentChange{Incident: "Kejadian diperjelas", Action: "Pembinaan lanjutan", Reason: "Informasi tambahan"}
	updated, err := store.UpdateStudentGuidance(ctx, kes.ID, record.ID, 1, change)
	if err != nil || updated.Version != 2 || updated.Incident != change.Incident {
		t.Fatalf("content update failed: %+v err=%v", updated, err)
	}
	if _, err = store.UpdateStudentGuidance(ctx, kes.ID, record.ID, 1, change); !errors.Is(err, studentaffairs.ErrGuidanceConflict) {
		t.Fatalf("stale content update accepted: %v", err)
	}
	updated, err = store.ChangeStudentGuidanceStatus(ctx, kes.ID, record.ID, 2, studentaffairs.GuidanceStatusChange{Status: studentaffairs.GuidanceCompleted, Reason: "Tindak lanjut selesai"})
	if err != nil || updated.Version != 3 || updated.Status != studentaffairs.GuidanceCompleted {
		t.Fatalf("status completion failed: %+v err=%v", updated, err)
	}
	updated, err = store.ChangeStudentGuidanceStatus(ctx, kes.ID, record.ID, 3, studentaffairs.GuidanceStatusChange{Status: studentaffairs.GuidanceNeedsFollowUp, Reason: "Perlu pemantauan kembali"})
	if err != nil || updated.Version != 4 || updated.Status != studentaffairs.GuidanceNeedsFollowUp {
		t.Fatalf("status reversal failed: %+v err=%v", updated, err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, value := range []string{"Kejadian bersamaan A", "Kejadian bersamaan B"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			<-start
			_, updateErr := store.UpdateStudentGuidance(ctx, kes.ID, record.ID, 4, studentaffairs.GuidanceContentChange{Incident: value, Action: "Tindakan", Reason: fmt.Sprintf("Konkuren %s", value)})
			results <- updateErr
		}(value)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		if result == nil {
			successes++
		} else if errors.Is(result, studentaffairs.ErrGuidanceConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent update error: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent optimistic updates successes=%d conflicts=%d", successes, conflicts)
	}
	history, err := store.StudentGuidanceHistory(ctx, newWali.ID, record.ID)
	if err != nil || len(history) != 5 || history[0].Change != studentaffairs.GuidanceContentChanged || history[0].Before == nil || history[0].After.Incident == "" || history[0].ActorName != kes.DisplayName {
		t.Fatalf("before/after history incomplete: len=%d first=%+v err=%v", len(history), history[0], err)
	}
	if _, err = store.StudentGuidanceHistory(ctx, oldWali.ID, record.ID); !errors.Is(err, studentaffairs.ErrGuidanceNotFound) {
		t.Fatalf("former homeroom read history via IDOR: %v", err)
	}

	var unclearStudent, unclearPlacement int64
	if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-02-02','L') RETURNING id`, "Siswa penempatan belum jelas "+suffix).Scan(&unclearStudent); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unclearPlacement > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, unclearPlacement)
		}
		_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, unclearStudent)
	}()
	if err = db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,NULL) RETURNING id`, yearID, unclearStudent, classB, admin.ID).Scan(&unclearPlacement); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateStudentGuidance(ctx, kes.ID, studentaffairs.GuidanceInput{StudentID: unclearStudent, Date: eventDate, Incident: "Kejadian", Action: "Pembinaan", Status: studentaffairs.GuidanceNeedsFollowUp}); !errors.Is(err, studentaffairs.ErrGuidancePlacementUnclear) {
		t.Fatalf("ambiguous event-date placement was guessed: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.student_placements SET starts_on=NULL WHERE student_id=$1`, studentID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetStudentGuidance(ctx, newWali.ID, record.ID); !errors.Is(err, studentaffairs.ErrGuidanceNotFound) {
		t.Fatalf("unclear current placement did not fail closed: %v", err)
	}
}
