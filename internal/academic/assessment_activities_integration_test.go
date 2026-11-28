//go:build integration

package academic

import (
	"context"
	"errors"
	"fmt"
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
		var immutableCorrections int
		if cleanupErr := db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_corrections c
			JOIN app.assessment_activity_initial_scores sc ON sc.id=c.initial_score_id
			JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id WHERE a.school_year_id=$1`, yearID).Scan(&immutableCorrections); cleanupErr != nil {
			t.Errorf("periksa histori koreksi fixture: %v", cleanupErr)
			return
		}
		if immutableCorrections > 0 {
			t.Log("fixture dipertahankan karena histori koreksi append-only")
			return
		}
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_score_corrections WHERE initial_score_id IN (SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1))`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_initial_scores WHERE assessment_activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_objectives WHERE activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activities WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.student_class_moves WHERE placement_id IN (SELECT id FROM app.student_placements WHERE student_id=ANY($1::bigint[]))`, []int64{studentID, studentBID, futureStudentID, unknownStudentID, foreignStudentID})
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
	if err = store.CreateAssessmentActivity(ctx, teacher.ID, semesterID, classID, subjectID, "Proyek sumatif", "sumatif", activityDate, []int64{objectives[0].ID}); err != nil {
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
	var sumativeActivityID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND name='Proyek sumatif'`, yearID).Scan(&sumativeActivityID); err != nil {
		t.Fatal("baca kegiatan Sumatif", err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, sumativeActivityID, []AssessmentScoreInput{{StudentID: studentID, ObjectiveID: objectives[0].ID, Value: "75.00"}}); err != nil {
		t.Fatal("simpan nilai awal Sumatif", err)
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
	// Pengampu baru boleh membaca kegiatan lama, tetapi tidak boleh menambah
	// nilai awal yang menjadi milik guru sebelumnya.
	reassignmentDate := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -1)
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, otherIdentity.ID, reassignmentDate); err != nil {
		t.Fatal("ganti pengampu resmi", err)
	}
	formerActivity, err := store.AssessmentActivityScoreSheet(ctx, otherTeacher.ID, activityID)
	if err != nil || formerActivity.CanRecord || formerActivity.Activity.TeacherID != teacherIdentity.ID {
		t.Fatalf("pengampu baru harus mendapat baca-saja pada kegiatan lama: canRecord=%v owner=%d err=%v", formerActivity.CanRecord, formerActivity.Activity.TeacherID, err)
	}
	if len(formerActivity.Students) != 2 {
		t.Fatalf("roster kegiatan lama tidak dapat dibaca: %d siswa", len(formerActivity.Students))
	}
	oldValues := make(map[int64]map[int64]string)
	for _, student := range formerActivity.Students {
		oldValues[student.ID] = make(map[int64]string)
		for _, score := range student.Scores {
			if score.Recorded {
				oldValues[student.ID][score.ObjectiveID] = score.Value
			}
		}
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, otherTeacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentBID, ObjectiveID: objectives[0].ID, Value: "25"}}); err != ErrForbidden {
		t.Fatalf("pengampu baru tidak boleh menambah nilai kegiatan lama: %v", err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityID, []AssessmentScoreInput{{StudentID: studentBID, ObjectiveID: objectives[0].ID, Value: "25"}}); err != ErrForbidden {
		t.Fatalf("pemilik lama tanpa penugasan aktif tidak boleh menambah nilai: %v", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, activityID).Scan(&scoreCount); err != nil || scoreCount != 3 {
		t.Fatalf("penolakan mutasi kegiatan lama mengubah jumlah nilai=%d err=%v", scoreCount, err)
	}
	for studentID, scores := range oldValues {
		for objectiveID, expected := range scores {
			var actual string
			if err = db.QueryRow(`SELECT score::text FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2 AND learning_objective_id=$3`, activityID, studentID, objectiveID).Scan(&actual); err != nil || actual != expected {
				t.Fatalf("nilai lama berubah: siswa=%d TP=%d nilai=%q ingin=%q err=%v", studentID, objectiveID, actual, expected, err)
			}
		}
	}
	var sumativeScoreID, formatifScoreID, concurrentScoreID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2 AND learning_objective_id=$3`, sumativeActivityID, studentID, objectives[0].ID).Scan(&sumativeScoreID); err != nil {
		t.Fatal("baca nilai awal Sumatif", err)
	}
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2 AND learning_objective_id=$3`, activityID, studentID, objectives[1].ID).Scan(&formatifScoreID); err != nil {
		t.Fatal("baca nilai awal Formatif", err)
	}
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2 AND learning_objective_id=$3`, activityID, studentBID, objectives[1].ID).Scan(&concurrentScoreID); err != nil {
		t.Fatal("baca nilai pembanding untuk uji konkurensi", err)
	}
	if err = store.CorrectAssessmentInitialScore(ctx, teacher.ID, sumativeScoreID, 0, "86,00", "Kesalahan input Sumatif"); err != nil {
		t.Fatalf("pemilik historis aktif harus dapat mengoreksi nilai Sumatif: %v", err)
	}
	if err = store.CorrectAssessmentInitialScore(ctx, teacher.ID, sumativeScoreID, 1, "88.25", "Koreksi kedua"); err != nil {
		t.Fatalf("koreksi berulang harus menambahkan versi: %v", err)
	}
	for _, invalid := range []struct {
		version       int64
		value, reason string
	}{{2, "91.234", "Alasan"}, {2, "101", "Alasan"}, {2, "92", "  "}} {
		if err = store.CorrectAssessmentInitialScore(ctx, teacher.ID, sumativeScoreID, invalid.version, invalid.value, invalid.reason); err == nil {
			t.Fatalf("nilai/alasan tidak valid harus ditolak: %+v", invalid)
		}
	}
	if err = store.CorrectAssessmentInitialScore(ctx, teacher.ID, sumativeScoreID, 1, "92", "Versi kedaluwarsa"); err != ErrConflict {
		t.Fatalf("versi lama harus ditolak sebagai konflik, got %v", err)
	}
	if err = store.CorrectAssessmentInitialScore(ctx, teacher.ID, formatifScoreID, 0, "90", "Koreksi Formatif"); err != nil {
		t.Fatalf("koreksi Formatif harus bekerja: %v", err)
	}
	var sumativeInitial, formatifInitial string
	if err = db.QueryRow(`SELECT score::text FROM app.assessment_activity_initial_scores WHERE id=$1`, sumativeScoreID).Scan(&sumativeInitial); err != nil || sumativeInitial != "75.00" {
		t.Fatalf("nilai awal tidak boleh ditimpa: %q err=%v", sumativeInitial, err)
	}
	if err = db.QueryRow(`SELECT score::text FROM app.assessment_activity_initial_scores WHERE id=$1`, formatifScoreID).Scan(&formatifInitial); err != nil || formatifInitial != "82.50" {
		t.Fatalf("nilai Formatif asli berubah: %q err=%v", formatifInitial, err)
	}
	var correctionCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_corrections WHERE initial_score_id=$1`, sumativeScoreID).Scan(&correctionCount); err != nil || correctionCount != 2 {
		t.Fatalf("koreksi Sumatif harus append-only dua versi: %d err=%v", correctionCount, err)
	}
	var latest string
	if err = db.QueryRow(`SELECT after_score::text FROM app.assessment_activity_score_corrections WHERE initial_score_id=$1 ORDER BY version DESC LIMIT 1`, sumativeScoreID).Scan(&latest); err != nil || latest != "88.25" {
		t.Fatalf("nilai berlaku terbaru salah: %q err=%v", latest, err)
	}
	if err = store.CorrectAssessmentInitialScore(ctx, otherTeacher.ID, sumativeScoreID, 2, "89", "Guru baru"); err != ErrForbidden {
		t.Fatalf("guru pengampu baru tidak boleh mengoreksi nilai lama, got %v", err)
	}
	if _, err = store.AssessmentScoreCorrectionDetail(ctx, otherTeacher.ID, sumativeScoreID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("guru baru tidak boleh menemukan form koreksi lama: %v", err)
	}
	if _, err = store.OwnedAssessmentInitialScores(ctx, teacher.ID, AssessmentOwnedScoreFilter{Search: "Nilai eligible", Page: 1}); err != nil {
		t.Fatalf("pemilik historis harus dapat menemukan nilai sendiri: %v", err)
	}
	var wgCorrections sync.WaitGroup
	correctionErrors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wgCorrections.Add(1)
		go func(value int) {
			defer wgCorrections.Done()
			correctionErrors <- store.CorrectAssessmentInitialScore(ctx, teacher.ID, concurrentScoreID, 0, fmt.Sprintf("%d", 71+value), "Koreksi konkuren")
		}(i)
	}
	wgCorrections.Wait()
	close(correctionErrors)
	correctionSuccess, staleCount := 0, 0
	for correctionErr := range correctionErrors {
		switch {
		case correctionErr == nil:
			correctionSuccess++
		case errors.Is(correctionErr, ErrConflict):
			staleCount++
		default:
			t.Errorf("hasil koreksi konkuren tidak terduga: %v", correctionErr)
		}
	}
	if correctionSuccess != 1 || staleCount != 7 {
		t.Fatalf("koreksi dengan versi sama harus menghasilkan 1 sukses/7 konflik; %d/%d", correctionSuccess, staleCount)
	}
	if _, err = db.Exec(`UPDATE app.assessment_activity_score_corrections SET reason='diubah' WHERE initial_score_id=$1`, sumativeScoreID); err == nil {
		t.Fatal("database harus menolak perubahan histori koreksi")
	}
	if _, err = db.Exec(`DELETE FROM app.assessment_activity_score_corrections WHERE initial_score_id=$1`, sumativeScoreID); err == nil {
		t.Fatal("database harus menolak penghapusan histori koreksi")
	}
	if _, err = db.Exec(`UPDATE app.teachers SET active=false WHERE id=$1`, teacherIdentity.ID); err != nil {
		t.Fatal("nonaktifkan identitas untuk tes penolakan", err)
	}
	if err = store.CorrectAssessmentInitialScore(ctx, teacher.ID, sumativeScoreID, 2, "89", "Identitas nonaktif"); err != ErrForbidden {
		t.Fatalf("identitas guru nonaktif harus ditolak: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.teachers SET active=true WHERE id=$1`, teacherIdentity.ID); err != nil {
		t.Fatal("pulihkan identitas fixture", err)
	}
	correctedSheet, err := store.AssessmentActivityScoreSheet(ctx, otherTeacher.ID, activityID)
	if err != nil || correctedSheet.CanRecord {
		t.Fatalf("pengampu baru tetap hanya baca: canRecord=%v err=%v", correctedSheet.CanRecord, err)
	}
	for _, student := range correctedSheet.Students {
		for _, cell := range student.Scores {
			if student.ID == studentID && cell.ObjectiveID == objectives[1].ID && cell.Value != "90.00" {
				t.Fatalf("lembar kegiatan harus menampilkan nilai berlaku hasil koreksi, got %s", cell.Value)
			}
		}
	}
	if err = store.CreateAssessmentActivity(ctx, otherTeacher.ID, semesterID, classID, subjectID, "Kegiatan milik guru B", "sumatif", activityDate, []int64{objectives[0].ID}); err != nil {
		t.Fatalf("guru pengampu baru membuat kegiatan sendiri: %v", err)
	}
	var teacherBActivityID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND name='Kegiatan milik guru B'`, yearID).Scan(&teacherBActivityID); err != nil {
		t.Fatal("baca kegiatan guru B", err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, otherTeacher.ID, teacherBActivityID, []AssessmentScoreInput{{StudentID: studentBID, ObjectiveID: objectives[0].ID, Value: "75"}}); err != nil {
		t.Fatalf("guru pengampu baru mencatat nilai kegiatannya sendiri: %v", err)
	}
	newOwnerSheet, err := store.AssessmentActivityScoreSheet(ctx, otherTeacher.ID, teacherBActivityID)
	if err != nil || !newOwnerSheet.CanRecord {
		t.Fatalf("kegiatan milik pengampu baru tidak dapat dicatat: canRecord=%v err=%v", newOwnerSheet.CanRecord, err)
	}
}
