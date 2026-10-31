//go:build integration

package academic_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/studentcases"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestSupportDashboardsRespectCaseScopeAndAttendanceSources(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	store := &academic.Store{DB: db}
	var baselineOpenCases int64
	if err := db.QueryRow(`SELECT count(*) FROM app.student_cases WHERE status='perlu_tindak_lanjut'`).Scan(&baselineOpenCases); err != nil {
		t.Fatal(err)
	}
	wali, _ := testutil.User(t, db, "wali_kelas")
	bk, _ := testutil.User(t, db, "bk")
	otherBK, _ := testutil.User(t, db, "bk")
	kes, _ := testutil.User(t, db, "kesiswaan")
	head, _ := testutil.User(t, db, "kepala_sekolah")
	admin, _ := testutil.User(t, db, "admin")
	users := []int64{wali.ID, bk.ID, otherBK.ID, kes.ID, head.ID, admin.ID}
	suffix := testutil.Random(t)[:12]
	yearID, _ := testutil.ActiveCalendar(t, db, suffix)
	var semesterID, classID, teacherID, studentID, placementID int64
	caseIDs := make([]int64, 0, 4)
	t.Cleanup(func() {
		if studentID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_daily_attendance WHERE student_id=$1`, studentID)
		}
		if len(caseIDs) > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_case_counseling_notes WHERE case_id=ANY($1::bigint[])`, caseIDs)
			_, _ = db.Exec(`DELETE FROM app.student_case_entries WHERE case_id=ANY($1::bigint[])`, caseIDs)
			_, _ = db.Exec(`DELETE FROM app.student_case_history WHERE case_id=ANY($1::bigint[])`, caseIDs)
			_, _ = db.Exec(`DELETE FROM app.student_cases WHERE id=ANY($1::bigint[])`, caseIDs)
		}
		if placementID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, placementID)
		}
		if studentID > 0 {
			_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, studentID)
		}
		if classID > 0 {
			_, _ = db.Exec(`DELETE FROM app.homeroom_assignments WHERE class_id=$1`, classID)
			_, _ = db.Exec(`DELETE FROM app.classes WHERE id=$1`, classID)
		}
		if teacherID > 0 {
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID)
		}
		_, _ = db.Exec(`DELETE FROM app.user_roles WHERE user_id=ANY($1::bigint[])`, users)
		_, _ = db.Exec(`DELETE FROM app.users WHERE id=ANY($1::bigint[])`, users)
	})
	if err := db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "D"+suffix).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali dashboard "+suffix, wali.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3)`, classID, teacherID, admin.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','P') RETURNING id`, "Dashboard support "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	if err := db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date) RETURNING id`, yearID, studentID, classID, admin.ID, today).Scan(&placementID); err != nil {
		t.Fatal(err)
	}
	createCase := func() studentcases.Case {
		t.Helper()
		item, err := store.CreateStudentCase(ctx, wali.ID, studentcases.CreateInput{StudentID: studentID, Content: "Ringkasan koordinasi dashboard"})
		if err != nil {
			t.Fatal(err)
		}
		caseIDs = append(caseIDs, item.ID)
		return item
	}
	toBK := func(item studentcases.Case, target int64) studentcases.Case {
		t.Helper()
		if err := store.ReferStudentCaseToBK(ctx, wali.ID, item.ID, item.Version, studentcases.ReferralInput{TargetUserID: target, Reason: "Rujukan untuk pengujian dashboard"}); err != nil {
			t.Fatal(err)
		}
		item.Stage = studentcases.StageCounselor
		item.ResponsibleID = target
		item.Version++
		return item
	}
	toKes := func(item studentcases.Case) studentcases.Case {
		t.Helper()
		if err := store.ReferStudentCaseToKesiswaan(ctx, item.ResponsibleID, item.ID, item.Version, studentcases.ReferralInput{TargetUserID: kes.ID, Reason: "Penerusan untuk pengujian dashboard"}); err != nil {
			t.Fatal(err)
		}
		item.Stage = studentcases.StageDiscipline
		item.ResponsibleID = kes.ID
		item.Version++
		return item
	}
	openBK := toBK(createCase(), bk.ID)
	if err := store.AddStudentCaseCounselingNote(ctx, bk.ID, openBK.ID, openBK.Version, studentcases.CounselingInput{Content: "RAHASIA_KONSELING_FIXTURE"}); err != nil {
		t.Fatal(err)
	}
	completedBK := toBK(createCase(), bk.ID)
	if err := store.ChangeStudentCaseStatus(ctx, bk.ID, completedBK.ID, completedBK.Version, studentcases.StatusInput{Status: studentcases.StatusCompleted, Reason: "Selesai untuk pengujian dashboard"}); err != nil {
		t.Fatal(err)
	}
	_ = toKes(toBK(createCase(), bk.ID))
	completedKes := toKes(toBK(createCase(), bk.ID))
	if err := store.ChangeStudentCaseStatus(ctx, kes.ID, completedKes.ID, completedKes.Version, studentcases.StatusInput{Status: studentcases.StatusCompleted, Reason: "Selesai untuk pengujian dashboard"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app.student_daily_attendance(student_id,placement_id,class_id,semester_id,on_date,student_name,nis,class_name,status,source,actor_id) VALUES($1,$2,$3,$4,$5::date,'Dashboard support','', '7D','hadir','manual',$6)`, studentID, placementID, classID, semesterID, today, admin.ID); err != nil {
		t.Fatal(err)
	}

	bkStats, err := store.CounselorDashboard(ctx, bk.ID)
	if err != nil || bkStats.Cases != 2 || bkStats.OpenCases != 1 || bkStats.CompletedCases != 1 {
		t.Fatalf("BK harus mendapat agregat kasus miliknya pada tahap BK: %+v err=%v", bkStats, err)
	}
	otherStats, err := store.CounselorDashboard(ctx, otherBK.ID)
	if err != nil || otherStats != (academic.CaseDashboardStats{}) {
		t.Fatalf("BK lain tidak boleh mendapat caseload historis/aktif rekan: %+v err=%v", otherStats, err)
	}
	kesStats, err := store.StudentAffairsDashboard(ctx, kes.ID)
	if err != nil || !kesStats.HasActiveYear || kesStats.ActiveStudents != 1 || kesStats.Recorded != 1 || kesStats.Present != 1 || kesStats.OpenCases != 1 {
		t.Fatalf("Kesiswaan harus mengikuti roster, absensi hari ini, dan kasus tahapnya: %+v err=%v", kesStats, err)
	}
	headStats, err := store.HeadmasterDashboard(ctx, head.ID)
	if err != nil || !headStats.HasActiveYear || headStats.ActiveStudents != 1 || headStats.Recorded != 1 || headStats.OpenCases != baselineOpenCases+2 {
		t.Fatalf("Kepala Sekolah mendapat ringkasan agregat seluruh sekolah: %+v err=%v", headStats, err)
	}
	if _, err = store.CounselorDashboard(ctx, admin.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("Admin tanpa role BK tidak boleh membaca statistik BK: %v", err)
	}
	if _, err = store.StudentAffairsDashboard(ctx, bk.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("BK tanpa role Kesiswaan tidak boleh membaca statistik Kesiswaan: %v", err)
	}
	if _, err = store.HeadmasterDashboard(ctx, kes.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("Kesiswaan tanpa role Kepala Sekolah tidak boleh membaca statistik Kepala Sekolah: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.school_years SET active=false WHERE id=$1`, yearID); err != nil {
		t.Fatal(err)
	}
	empty, err := store.HeadmasterDashboard(ctx, head.ID)
	if err != nil || empty.HasActiveYear || empty.ActiveStudents != 0 || empty.Recorded != 0 || empty.OpenCases != baselineOpenCases+2 {
		t.Fatalf("tanpa tahun aktif, roster/absensi harus belum tersedia tetapi ringkasan kasus tetap berlaku: %+v err=%v", empty, err)
	}
}
