//go:build integration

package academic

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAssessmentActivitiesAreScopedAndAtomicIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	otherTeacher, _ := testutil.User(t, db, "guru_mapel")
	suffix := testutil.Random(t)[:10]
	var yearID, semesterID, classID, otherClassID, subjectID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "Kegiatan CP10-04 "+suffix).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	for _, target := range []*int64{&classID, &otherClassID} {
		if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,8,$2) RETURNING id`, yearID, testutil.Random(t)[:8]).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "A04-"+suffix, "Mapel kegiatan "+suffix).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	masters := &master.Store{DB: db}
	teacherIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru CP10-04 " + suffix, UserID: teacher.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	otherIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru lain CP10-04 " + suffix, UserID: otherTeacher.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{DB: db}
	var studentID, studentBID, futureStudentID, unknownStudentID, foreignStudentID, placementID, placementBID int64
	activityDate := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	from := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -10)
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, teacherIdentity.ID, from); err != nil {
		t.Fatal("penugasan utama", err)
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, otherClassID, subjectID, otherIdentity.ID, from); err != nil {
		t.Fatal("penugasan pembanding", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_initial_scores WHERE assessment_activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_objectives WHERE activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activities WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE school_year_id=$1`, yearID)
		for _, placement := range []int64{placementID, placementBID} {
			if placement > 0 {
				_, _ = db.Exec(`DELETE FROM app.student_class_moves WHERE placement_id=$1`, placement)
				_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, placement)
			}
		}
		for _, id := range []int64{studentID, studentBID, futureStudentID, unknownStudentID, foreignStudentID} {
			if id > 0 {
				_, _ = db.Exec(`DELETE FROM app.student_placements WHERE student_id=$1`, id)
				_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, id)
			}
		}
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id=$1)`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id=$1`, semesterID)
		for _, id := range []int64{teacherIdentity.ID, otherIdentity.ID} {
			_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, id)
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, id)
		}
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code=$1`, "A04-"+suffix)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id=$1`, subjectID)
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.classes WHERE id IN ($1,$2)`, classID, otherClassID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		for _, id := range []int64{admin.ID, curriculum.ID, teacher.ID, otherTeacher.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, cleanupErr := db.Exec(q, id); cleanupErr != nil {
					t.Errorf("cleanup akun sintetis: %v", cleanupErr)
					return
				}
			}
		}
	})
	if err = store.CreateLearningObjective(ctx, teacher.ID, semesterID, classID, subjectID, "TP pertama "+suffix); err != nil {
		t.Fatal("buat TP 1", err)
	}
	if err = store.CreateLearningObjective(ctx, teacher.ID, semesterID, classID, subjectID, "TP kedua "+suffix); err != nil {
		t.Fatal("buat TP 2", err)
	}
	if err = store.CreateLearningObjective(ctx, otherTeacher.ID, semesterID, otherClassID, subjectID, "TP kelas lain "+suffix); err != nil {
		t.Fatal("buat TP beda konteks", err)
	}
	objectives, err := store.LearningObjectives(ctx, teacher.ID, semesterID, classID, subjectID)
	if err != nil || len(objectives) != 2 {
		t.Fatalf("baca TP pada konteks utama: count=%d err=%v", len(objectives), err)
	}
	var foreignObjectiveID int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semesterID, otherClassID, subjectID).Scan(&foreignObjectiveID); err != nil {
		t.Fatal("baca TP konteks guru lain", err)
	}
	if err = store.CreateAssessmentActivity(ctx, teacher.ID, semesterID, classID, subjectID, "Tes formatif — sesi 1", "formatif", activityDate, []int64{objectives[0].ID, objectives[1].ID}); err != nil {
		t.Fatalf("buat kegiatan formatif multi-TP: %v", err)
	}
	if err = store.CreateAssessmentActivity(ctx, teacher.ID, semesterID, classID, subjectID, "Proyek sumatif", "sumatif", "2026-11-03", []int64{objectives[0].ID}); err != nil {
		t.Fatalf("buat kegiatan sumatif: %v", err)
	}
	var activityID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND activity_type='formatif'`, yearID).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	createStudent := func(name string) int64 {
		t.Helper()
		var id int64
		if e := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','L') RETURNING id`, name+suffix).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	studentID, studentBID, futureStudentID, unknownStudentID, foreignStudentID = createStudent("Nilai eligible "), createStudent("Nilai eligible B "), createStudent("Nilai future "), createStudent("Nilai tanggal tidak jelas "), createStudent("Nilai kelas lain ")
	if err = db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date-10) RETURNING id`, yearID, studentID, classID, admin.ID, activityDate).Scan(&placementID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date-10) RETURNING id`, yearID, studentBID, classID, admin.ID, activityDate).Scan(&placementBID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,$4::date+1,'Perpindahan setelah kegiatan',$5)`, placementID, classID, otherClassID, activityDate, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date+1)`, yearID, futureStudentID, classID, admin.ID, activityDate); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,NULL)`, yearID, unknownStudentID, classID, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date-10)`, yearID, foreignStudentID, otherClassID, admin.ID, activityDate); err != nil {
		t.Fatal(err)
	}
	sheet, err := store.AssessmentActivityScoreSheet(ctx, teacher.ID, activityID)
	if err != nil || len(sheet.Students) != 2 || sheet.UnknownPlacementCount != 1 {
		t.Fatalf("roster historische tidak sesuai: %+v err=%v", sheet, err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentID, ObjectiveID: objectives[0].ID, Value: "0"}, {StudentID: studentID, ObjectiveID: objectives[1].ID, Value: "82,50"}}); err != nil {
		t.Fatal("simpan nilai multi-TP", err)
	}
	var scoreCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, activityID).Scan(&scoreCount); err != nil || scoreCount != 2 {
		t.Fatalf("nilai tersimpan=%d err=%v", scoreCount, err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentID, ObjectiveID: objectives[0].ID, Value: "0.00"}, {StudentID: studentID, ObjectiveID: objectives[1].ID, Value: "82.5"}}); err != nil {
		t.Fatalf("pengiriman identik harus idempoten: %v", err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentBID, ObjectiveID: objectives[0].ID, Value: "20"}, {StudentID: studentID, ObjectiveID: objectives[0].ID, Value: "1"}}); err == nil {
		t.Fatal("nilai awal berbeda atau siswa future harus ditolak")
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentID, ObjectiveID: foreignObjectiveID, Value: "1"}}); err == nil {
		t.Fatal("TP yang tidak terkait kegiatan harus ditolak")
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: foreignStudentID, ObjectiveID: objectives[0].ID, Value: "1"}}); err == nil {
		t.Fatal("siswa kelas lain harus ditolak")
	}
	if _, err = store.AssessmentActivityScoreSheet(ctx, otherTeacher.ID, activityID); err == nil {
		t.Fatal("guru tanpa penugasan tidak boleh membaca lembar nilai")
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, otherTeacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentID, ObjectiveID: objectives[0].ID, Value: "1"}}); err == nil {
		t.Fatal("guru tanpa penugasan tidak boleh menyimpan nilai")
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, activityID).Scan(&scoreCount); err != nil || scoreCount != 2 {
		t.Fatalf("request ditolak meninggalkan nilai parsial: %d err=%v", scoreCount, err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: futureStudentID, ObjectiveID: objectives[0].ID, Value: "20"}}); err == nil {
		t.Fatal("siswa future harus ditolak")
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentBID, ObjectiveID: objectives[1].ID, Value: ""}}); err == nil {
		t.Fatal("nilai kosong tidak boleh menghasilkan record")
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentBID, ObjectiveID: objectives[1].ID, Value: "70"}})
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			t.Errorf("retry konkuren identik: %v", e)
		}
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, activityID).Scan(&scoreCount); err != nil || scoreCount != 3 {
		t.Fatalf("retry konkuren menghasilkan %d nilai, err=%v", scoreCount, err)
	}
	var activityCount, relationCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activities WHERE school_year_id=$1`, yearID).Scan(&activityCount); err != nil || activityCount != 2 {
		t.Fatalf("jumlah kegiatan=%d err=%v", activityCount, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_objectives ao JOIN app.assessment_activities a ON a.id=ao.activity_id WHERE a.school_year_id=$1`, yearID).Scan(&relationCount); err != nil || relationCount != 3 {
		t.Fatalf("jumlah tautan TP=%d err=%v", relationCount, err)
	}
	activities, err := store.AssessmentActivities(ctx, teacher.ID, semesterID, classID, subjectID)
	if err != nil || len(activities) != 2 {
		t.Fatalf("daftar kegiatan: count=%d err=%v", len(activities), err)
	}
	foundMulti := false
	for _, activity := range activities {
		if activity.Type == "formatif" && activity.Name == "Tes formatif — sesi 1" && len(activity.Objectives) == 2 {
			foundMulti = true
		}
	}
	if !foundMulti {
		t.Fatal("daftar tidak mengembalikan kegiatan Formatif dengan kedua TP")
	}
	if err = store.CreateAssessmentActivity(ctx, teacher.ID, semesterID, classID, subjectID, "Campuran tidak sah", "sumatif", "2026-11-04", []int64{objectives[0].ID, foreignObjectiveID}); err == nil {
		t.Fatal("TP dari konteks kelas lain harus ditolak")
	}
	if err = store.CreateAssessmentActivity(ctx, otherTeacher.ID, semesterID, classID, subjectID, "Akses lintas guru", "sumatif", "2026-11-04", []int64{objectives[0].ID}); err == nil {
		t.Fatal("guru lain tanpa penugasan tidak boleh membuat kegiatan")
	}
	if err = store.CreateAssessmentActivity(ctx, admin.ID, semesterID, classID, subjectID, "Role admin", "sumatif", "2026-11-04", []int64{objectives[0].ID}); err == nil {
		t.Fatal("Admin tidak otomatis boleh membuat kegiatan Guru Mapel")
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activities WHERE school_year_id=$1`, yearID).Scan(&activityCount); err != nil || activityCount != 2 {
		t.Fatalf("request ditolak meninggalkan kegiatan parsial: count=%d err=%v", activityCount, err)
	}
	if _, err = db.Exec(`INSERT INTO app.assessment_activity_objectives(activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id)
		SELECT a.id,lo.id,a.school_year_id,a.semester_id,lo.class_id,lo.subject_id FROM app.assessment_activities a CROSS JOIN app.learning_objectives lo WHERE a.school_year_id=$1 AND lo.id=$2 LIMIT 1`, yearID, foreignObjectiveID); err == nil {
		t.Fatal("composite FK database harus menolak TP beda konteks")
	}
}
