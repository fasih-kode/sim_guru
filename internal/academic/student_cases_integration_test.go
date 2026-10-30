//go:build integration

package academic_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/studentcases"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentCaseCoordinationAccessIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	store := &academic.Store{DB: db}

	wali, _ := testutil.User(t, db, "wali_kelas")
	newWali, _ := testutil.User(t, db, "wali_kelas")
	bk, _ := testutil.User(t, db, "bk")
	otherBK, _ := testutil.User(t, db, "bk")
	kesiswaan, _ := testutil.User(t, db, "kesiswaan")
	head, _ := testutil.User(t, db, "kepala_sekolah")
	admin, _ := testutil.User(t, db, "admin")
	guru, _ := testutil.User(t, db, "guru_mapel")
	combined, _ := testutil.User(t, db, "wali_kelas", "admin")

	suffix := testutil.Random(t)[:12]
	yearID, _ := testutil.ActiveCalendar(t, db, suffix)
	var classA, classB, oldTeacher, newTeacher, studentID, placementID int64
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "KasusA"+suffix).Scan(&classA); err != nil {
		t.Fatal("create isolated class")
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "KasusB"+suffix).Scan(&classB); err != nil {
		t.Fatal("create isolated class")
	}
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali kasus "+suffix, wali.ID).Scan(&oldTeacher); err != nil {
		t.Fatal("create homeroom identity")
	}
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali baru kasus "+suffix, newWali.ID).Scan(&newTeacher); err != nil {
		t.Fatal("create homeroom identity")
	}
	for _, assignment := range []struct{ classID, teacherID int64 }{{classA, oldTeacher}, {classB, newTeacher}} {
		if _, err := db.Exec(`INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3)`, assignment.classID, assignment.teacherID, admin.ID); err != nil {
			t.Fatal("create active homeroom assignment")
		}
	}
	if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-03-04','P') RETURNING id`, "Siswa kasus "+suffix).Scan(&studentID); err != nil {
		t.Fatal("create isolated student")
	}
	if err := db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01') RETURNING id`, yearID, studentID, classA, admin.ID).Scan(&placementID); err != nil {
		t.Fatal("create active placement")
	}

	item, err := store.CreateStudentCase(ctx, wali.ID, studentcases.CreateInput{StudentID: studentID, Content: "Ringkasan pembinaan awal"})
	if err != nil {
		t.Fatalf("Wali Kelas could not create a formal case: %v", err)
	}
	createCase := func(classID int64, suffix string) (studentcases.Case, int64) {
		t.Helper()
		var sid, pid int64
		if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-03-04','P') RETURNING id`, "Siswa handoff "+suffix).Scan(&sid); err != nil {
			t.Fatal("create isolated handoff student")
		}
		if err := db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,CURRENT_DATE-10) RETURNING id`, yearID, sid, classID, admin.ID).Scan(&pid); err != nil {
			t.Fatal("create isolated handoff placement")
		}
		created, err := store.CreateStudentCase(ctx, wali.ID, studentcases.CreateInput{StudentID: sid, Content: "Ringkasan pembinaan awal " + suffix})
		if err != nil {
			t.Fatalf("create isolated handoff case: %v", err)
		}
		return created, pid
	}
	handoffCase, handoffPlacement := createCase(classA, "aktif")
	bkCase, bkPlacement := createCase(classA, "tahap-bk")
	if err = store.ReferStudentCaseToBK(ctx, wali.ID, bkCase.ID, bkCase.Version, studentcases.ReferralInput{TargetUserID: bk.ID, Reason: "Rujukan pengujian tahap BK"}); err != nil {
		t.Fatal("prepare BK-stage case for handoff denial")
	}
	completedCase, completedPlacement := createCase(classA, "selesai")
	if err = store.ChangeStudentCaseStatus(ctx, wali.ID, completedCase.ID, completedCase.Version, studentcases.StatusInput{Status: studentcases.StatusCompleted, Reason: "Kasus uji diselesaikan"}); err != nil {
		t.Fatal("prepare completed case for handoff denial")
	}
	unclearCase, unclearPlacement := createCase(classA, "tanpa-wali")
	var classWithoutWali int64
	if err = db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "KasusTanpaWali"+suffix).Scan(&classWithoutWali); err != nil {
		t.Fatal("create class without active homeroom assignment")
	}
	if _, err = store.GetStudentCase(ctx, head.ID, item.ID); err != nil {
		t.Fatal("headmaster summary access denied")
	}
	if _, err = store.GetStudentCase(ctx, bk.ID, item.ID); !errors.Is(err, studentcases.ErrNotFound) {
		t.Fatal("unassigned BK read a case before referral")
	}
	for _, denied := range []int64{admin.ID, guru.ID} {
		if _, err = store.ListStudentCases(ctx, denied, studentcases.Filter{Page: 1}); !errors.Is(err, studentcases.ErrForbidden) {
			t.Fatal("role without case access was allowed")
		}
	}
	if list, listErr := store.ListStudentCases(ctx, otherBK.ID, studentcases.Filter{Page: 1}); listErr != nil || len(list.Cases) != 0 {
		t.Fatal("unassigned BK could see another person's active case")
	}
	if err = store.ReferStudentCaseToBK(ctx, wali.ID, item.ID, item.Version, studentcases.ReferralInput{TargetUserID: bk.ID, Reason: "Perlu konseling lanjutan"}); err != nil {
		t.Fatal("formal Wali-to-BK referral failed")
	}
	assigned, err := store.GetStudentCase(ctx, bk.ID, item.ID)
	if err != nil || !assigned.CanReadCounseling {
		t.Fatal("active BK could not read the private section")
	}
	if err = store.AddStudentCaseCounselingNote(ctx, bk.ID, item.ID, assigned.Case.Version, studentcases.CounselingInput{Content: "RAHASIA-UJI-KONSELING"}); err != nil {
		t.Fatal("active BK could not append a private note")
	}
	for _, reader := range []int64{wali.ID, head.ID, admin.ID, otherBK.ID} {
		got, readErr := store.GetStudentCase(ctx, reader, item.ID)
		if readErr == nil && len(got.CounselingNotes) != 0 {
			t.Fatal("private counseling note crossed its access boundary")
		}
	}
	assigned, err = store.GetStudentCase(ctx, bk.ID, item.ID)
	if err != nil || len(assigned.CounselingNotes) != 1 {
		t.Fatal("active BK private note was not retained")
	}
	if err = store.ChangeStudentCaseStatus(ctx, bk.ID, item.ID, assigned.Case.Version, studentcases.StatusInput{Status: studentcases.StatusCompleted, Reason: "Penanganan konseling sementara selesai"}); err != nil {
		t.Fatal("active BK could not complete the case")
	}
	closedForBK, err := store.GetStudentCase(ctx, bk.ID, item.ID)
	if err != nil || !closedForBK.CanReadCounseling || len(closedForBK.CounselingNotes) != 1 {
		t.Fatal("active BK lost private history after completing the case")
	}
	if err = store.ChangeStudentCaseStatus(ctx, bk.ID, item.ID, closedForBK.Case.Version, studentcases.StatusInput{Status: studentcases.StatusOpen, Reason: "Perlu koordinasi lanjutan"}); err != nil {
		t.Fatal("active BK could not reopen the case")
	}
	assigned, err = store.GetStudentCase(ctx, bk.ID, item.ID)
	if err != nil {
		t.Fatal("active BK could not reload the case")
	}
	if err = store.ReferStudentCaseToKesiswaan(ctx, bk.ID, item.ID, assigned.Case.Version, studentcases.ReferralInput{TargetUserID: kesiswaan.ID, Reason: "Perlu penanganan kedisiplinan"}); err != nil {
		t.Fatal("BK-to-Kesiswaan referral failed")
	}
	if _, err = store.GetStudentCase(ctx, bk.ID, item.ID); !errors.Is(err, studentcases.ErrNotFound) {
		t.Fatal("former BK handler retained case access")
	}
	if _, err = store.GetStudentCase(ctx, kesiswaan.ID, item.ID); err != nil {
		t.Fatal("assigned Kesiswaan could not read coordination summary")
	}

	// Perpindahan kelas mengalihkan hak pantau Wali mengikuti penempatan saat ini.
	effective := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -1).Format("2006-01-02")
	if _, err = db.Exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,$4,'Isolasi cakupan uji kasus',$5)`, placementID, classA, classB, effective, admin.ID); err != nil {
		t.Fatal("create isolated class move")
	}
	for _, placement := range []int64{handoffPlacement, bkPlacement, completedPlacement} {
		if _, err = db.Exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,$4,'Uji handoff Wali setelah perpindahan',$5)`, placement, classA, classB, effective, admin.ID); err != nil {
			t.Fatal("move isolated case student to newly assigned Wali")
		}
	}
	if _, err = db.Exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,$4,'Uji penugasan Wali tidak tersedia',$5)`, unclearPlacement, classA, classWithoutWali, effective, admin.ID); err != nil {
		t.Fatal("move isolated student to class without Wali")
	}
	if _, err = store.GetStudentCase(ctx, wali.ID, item.ID); !errors.Is(err, studentcases.ErrNotFound) {
		t.Fatal("former Wali retained monitoring access after class transfer")
	}
	if _, err = store.GetStudentCase(ctx, newWali.ID, item.ID); err != nil {
		t.Fatal("new active Wali could not monitor case")
	}
	if _, err = store.GetStudentCase(ctx, wali.ID, handoffCase.ID); !errors.Is(err, studentcases.ErrNotFound) {
		t.Fatal("former Wali retained access to a case still at the Wali stage")
	}
	readable, err := store.GetStudentCase(ctx, newWali.ID, handoffCase.ID)
	if err != nil || !readable.CanTakeOver || readable.CanWrite {
		t.Fatal("new Wali should receive read access and explicit handoff action only")
	}
	var beforeVersion int64
	var beforeHistory int
	if err = db.QueryRow(`SELECT version,(SELECT count(*) FROM app.student_case_history WHERE case_id=$1) FROM app.student_cases WHERE id=$1`, handoffCase.ID).Scan(&beforeVersion, &beforeHistory); err != nil {
		t.Fatal("read handoff baseline")
	}
	if _, err = store.GetStudentCase(ctx, newWali.ID, handoffCase.ID); err != nil {
		t.Fatal("read-only GET for takeover case failed")
	}
	var afterVersion int64
	var afterHistory int
	if err = db.QueryRow(`SELECT version,(SELECT count(*) FROM app.student_case_history WHERE case_id=$1) FROM app.student_cases WHERE id=$1`, handoffCase.ID).Scan(&afterVersion, &afterHistory); err != nil || beforeVersion != afterVersion || beforeHistory != afterHistory {
		t.Fatal("GET changed case owner/version/history")
	}
	if err = store.TakeOverStudentCase(ctx, newWali.ID, handoffCase.ID, readable.Case.Version, studentcases.HandoffInput{}); !errors.Is(err, studentcases.ErrInvalid) {
		t.Fatal("handoff without a reason was accepted")
	}
	if err = store.TakeOverStudentCase(ctx, wali.ID, handoffCase.ID, readable.Case.Version, studentcases.HandoffInput{Reason: "Percobaan Wali lama"}); !errors.Is(err, studentcases.ErrForbidden) {
		t.Fatal("former owner could take over after losing current assignment")
	}
	if err = store.TakeOverStudentCase(ctx, newWali.ID, handoffCase.ID, readable.Case.Version+1, studentcases.HandoffInput{Reason: "Versi kedaluwarsa"}); !errors.Is(err, studentcases.ErrConflict) {
		t.Fatal("stale handoff version was not rejected")
	}
	startHandoff := make(chan struct{})
	handoffResults := make(chan error, 2)
	var handoffWG sync.WaitGroup
	for i := 0; i < 2; i++ {
		handoffWG.Add(1)
		go func() {
			defer handoffWG.Done()
			<-startHandoff
			handoffResults <- store.TakeOverStudentCase(ctx, newWali.ID, handoffCase.ID, readable.Case.Version, studentcases.HandoffInput{Reason: "Melanjutkan penanganan setelah perubahan kelas"})
		}()
	}
	close(startHandoff)
	handoffWG.Wait()
	close(handoffResults)
	handoffSucceeded, handoffConflicted := 0, 0
	for result := range handoffResults {
		if result == nil {
			handoffSucceeded++
		} else if errors.Is(result, studentcases.ErrConflict) {
			handoffConflicted++
		}
	}
	if handoffSucceeded != 1 || handoffConflicted != 1 {
		t.Fatalf("concurrent handoff did not have one atomic winner: succeeded=%d conflicted=%d", handoffSucceeded, handoffConflicted)
	}
	ownedByNewWali, err := store.GetStudentCase(ctx, newWali.ID, handoffCase.ID)
	if err != nil || !ownedByNewWali.CanWrite || ownedByNewWali.CanTakeOver || ownedByNewWali.Case.ResponsibleID != newWali.ID {
		t.Fatal("new Wali did not receive operational ownership after handoff")
	}
	var handoffCount int
	var handoffActor, oldOwner, newOwner int64
	var handoffReason string
	if err = db.QueryRow(`SELECT count(*),min(actor_id),min(from_responsible_user_id),min(to_responsible_user_id),min(reason) FROM app.student_case_history WHERE case_id=$1 AND event_type='wali_handoff'`, handoffCase.ID).Scan(&handoffCount, &handoffActor, &oldOwner, &newOwner, &handoffReason); err != nil || handoffCount != 1 || handoffActor != newWali.ID || oldOwner != wali.ID || newOwner != newWali.ID || handoffReason == "" {
		t.Fatal("handoff history is not complete or was duplicated")
	}
	if err = store.AddStudentCaseEntry(ctx, newWali.ID, handoffCase.ID, ownedByNewWali.Case.Version, studentcases.EntryInput{Content: "Tindak lanjut setelah pengambilalihan"}); err != nil {
		t.Fatal("new Wali could not continue case operations")
	}
	if _, err = db.Exec(`UPDATE app.student_cases SET responsible_user_id=$1 WHERE id=$2`, wali.ID, handoffCase.ID); err == nil {
		t.Fatal("database owner guard allowed handoff back to a Wali without current assignment")
	}
	if err = store.TakeOverStudentCase(ctx, newWali.ID, handoffCase.ID, ownedByNewWali.Case.Version+1, studentcases.HandoffInput{Reason: "Percobaan ambil alih berulang"}); !errors.Is(err, studentcases.ErrForbidden) {
		t.Fatal("repeated handoff by current owner was accepted")
	}
	if err = store.TakeOverStudentCase(ctx, newWali.ID, bkCase.ID, 2, studentcases.HandoffInput{Reason: "Tidak boleh mengubah tahap BK"}); !errors.Is(err, studentcases.ErrForbidden) {
		t.Fatal("handoff of a case already at BK stage was accepted")
	}
	if err = store.TakeOverStudentCase(ctx, newWali.ID, completedCase.ID, 2, studentcases.HandoffInput{Reason: "Kasus selesai"}); !errors.Is(err, studentcases.ErrForbidden) {
		t.Fatal("completed case takeover was accepted")
	}
	if err = store.TakeOverStudentCase(ctx, newWali.ID, unclearCase.ID, unclearCase.Version, studentcases.HandoffInput{Reason: "Wali tujuan tidak tersedia"}); !errors.Is(err, studentcases.ErrScopeUnclear) {
		t.Fatal("handoff with no active Wali assignment was not rejected safely")
	}
	if err = store.TakeOverStudentCase(ctx, combined.ID, handoffCase.ID, ownedByNewWali.Case.Version+1, studentcases.HandoffInput{Reason: "Role gabungan tanpa penugasan"}); !errors.Is(err, studentcases.ErrForbidden) {
		t.Fatal("combined role without the current assignment expanded handoff access")
	}

	current, err := store.GetStudentCase(ctx, kesiswaan.ID, item.ID)
	if err != nil {
		t.Fatal("assigned Kesiswaan lost case access")
	}
	if err = store.ChangeStudentCaseStatus(ctx, kesiswaan.ID, item.ID, current.Case.Version, studentcases.StatusInput{Status: studentcases.StatusCompleted, Reason: "Tindak lanjut selesai"}); err != nil {
		t.Fatal("active handler could not complete the case")
	}
	completed, err := store.GetStudentCase(ctx, head.ID, item.ID)
	if err != nil || completed.Case.Status != studentcases.StatusCompleted || len(completed.History) < 4 {
		t.Fatal("case status/history was not retained")
	}
	if err = store.ChangeStudentCaseStatus(ctx, kesiswaan.ID, item.ID, completed.Case.Version, studentcases.StatusInput{Status: studentcases.StatusOpen, Reason: "Perlu tindak lanjut tambahan"}); err != nil {
		t.Fatal("active handler could not reopen the case")
	}

	latest, err := store.GetStudentCase(ctx, kesiswaan.ID, item.ID)
	if err != nil {
		t.Fatal("assigned handler could not reload case")
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- store.AddStudentCaseEntry(ctx, kesiswaan.ID, item.ID, latest.Case.Version, studentcases.EntryInput{Content: "Pembaruan serentak"})
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for mutationErr := range results {
		if mutationErr == nil {
			succeeded++
		} else if errors.Is(mutationErr, studentcases.ErrConflict) {
			conflicted++
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatal("concurrent case update did not preserve one atomic winner")
	}
}
