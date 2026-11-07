//go:build integration

package academic

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAssessmentTPRecapTwoLevelCompletenessPlacementAndAuthorizationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	otherTeacher, _ := testutil.User(t, db, "guru_mapel")
	suffix := testutil.Random(t)[:10]
	keySuffix := testutil.Random(t)[:10]
	submissionKey := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%s%02d", keySuffix, n) }
	today := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	todayTime, err := time.Parse("2006-01-02", today)
	if err != nil {
		t.Fatal(err)
	}
	tomorrow := todayTime.AddDate(0, 0, 1).Format("2006-01-02")
	calendarStart := todayTime.AddDate(0, 0, -30).Format("2006-01-02")
	calendarEnd := todayTime.AddDate(0, 0, 30).Format("2006-01-02")
	var yearID, semesterID, classID, otherClassID, subjectID int64
	if err = db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "Rekap CP10-08 "+suffix).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	for _, target := range []*int64{&classID, &otherClassID} {
		if err = db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, testutil.Random(t)[:7]).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,$2::date,$3::date,'[1,2,3,4,5]'::jsonb)`, semesterID, calendarStart, calendarEnd); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "R08-"+suffix, "Mapel rekap "+suffix).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	masters := &master.Store{DB: db}
	ownerIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru rekap " + suffix, UserID: teacher.ID, Active: true}})
	if err != nil {
		t.Fatal("identitas guru pemilik", err)
	}
	otherIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru rekap lain " + suffix, UserID: otherTeacher.ID, Active: true}})
	if err != nil {
		t.Fatal("identitas guru pembanding", err)
	}
	store := &Store{DB: db}
	effectiveFrom := todayTime.AddDate(0, 0, -15)
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, ownerIdentity.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan pemilik", err)
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, otherClassID, subjectID, otherIdentity.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan pembanding", err)
	}
	keepFixture := false
	var studentA, studentB int64
	t.Cleanup(func() {
		if keepFixture {
			t.Log("fixture rekap dipertahankan karena memuat histori koreksi/remedial append-only")
			return
		}
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_initial_scores WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_objectives WHERE activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activities WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE school_year_id=$1`, yearID)
		for _, id := range []int64{studentA, studentB} {
			if id > 0 {
				_, _ = db.Exec(`DELETE FROM app.student_placements WHERE student_id=$1`, id)
				_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, id)
			}
		}
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id=$1)`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id=$1`, semesterID)
		for _, id := range []int64{ownerIdentity.ID, otherIdentity.ID} {
			_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, id)
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, id)
		}
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code=$1`, "R08-"+suffix)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id=$1`, subjectID)
		_, _ = db.Exec(`DELETE FROM app.semester_calendars WHERE semester_id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.classes WHERE id IN ($1,$2)`, classID, otherClassID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		for _, id := range []int64{admin.ID, curriculum.ID, teacher.ID, otherTeacher.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, cleanupErr := db.Exec(q, id); cleanupErr != nil {
					t.Errorf("cleanup akun fixture: %v", cleanupErr)
					return
				}
			}
		}
	})

	var objective1, objective2, objective3 int64
	for _, item := range []struct {
		name string
		id   *int64
	}{{"TP presisi satu", &objective1}, {"TP presisi dua", &objective2}} {
		if err = store.CreateLearningObjective(ctx, teacher.ID, semesterID, classID, subjectID, item.name+" "+suffix); err != nil {
			t.Fatal("buat TP", err)
		}
		if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND definition=$4`, semesterID, classID, subjectID, item.name+" "+suffix).Scan(item.id); err != nil {
			t.Fatal("baca TP", err)
		}
	}
	createActivity := func(name, kind, date string, objectives ...int64) int64 {
		t.Helper()
		if e := store.CreateAssessmentActivity(ctx, teacher.ID, semesterID, classID, subjectID, name+" "+suffix, kind, date, objectives); e != nil {
			t.Fatal("buat kegiatan", e)
		}
		var id int64
		if e := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND name=$2`, yearID, name+" "+suffix).Scan(&id); e != nil {
			t.Fatal("baca kegiatan", e)
		}
		return id
	}
	activity60 := createActivity("Sumatif TP1 60", "sumatif", today, objective1)
	activity80 := createActivity("Sumatif TP1 80", "sumatif", today, objective1)
	activityMulti := createActivity("Sumatif multi-TP", "sumatif", today, objective1, objective2)
	formatif := createActivity("Formatif tidak dihitung", "formatif", today, objective1)

	createStudent := func(name, start string) (int64, int64) {
		t.Helper()
		var studentID, placementID int64
		if e := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','L') RETURNING id`, name+suffix).Scan(&studentID); e != nil {
			t.Fatal("buat siswa", e)
		}
		if e := db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date) RETURNING id`, yearID, studentID, classID, admin.ID, start).Scan(&placementID); e != nil {
			t.Fatal("buat penempatan", e)
		}
		return studentID, placementID
	}
	studentA, _ = createStudent("Siswa rekap aktif ", calendarStart)
	studentB, _ = createStudent("Siswa pindah masuk ", tomorrow)
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activity60, []AssessmentScoreInput{{StudentID: studentA, ObjectiveID: objective1, Value: "60"}}); err != nil {
		t.Fatal("nilai 60", err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activity80, []AssessmentScoreInput{{StudentID: studentA, ObjectiveID: objective1, Value: "80"}}); err != nil {
		t.Fatal("nilai 80", err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityMulti, []AssessmentScoreInput{
		{StudentID: studentA, ObjectiveID: objective1, Value: "100"},
		{StudentID: studentA, ObjectiveID: objective2, Value: "90"},
	}); err != nil {
		t.Fatal("nilai multi-TP", err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, formatif, []AssessmentScoreInput{{StudentID: studentA, ObjectiveID: objective1, Value: "100"}}); err != nil {
		t.Fatal("nilai Formatif", err)
	}
	var correctionScoreID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2 AND learning_objective_id=$3`, activity60, studentA, objective1).Scan(&correctionScoreID); err != nil {
		t.Fatal(err)
	}
	// Baseline: tiga kegiatan TP1 bernilai 60, 80, 100; TP2 bernilai 90.
	// Formatif tidak ikut dan multi-TP dihitung
	// satu kali secara terpisah pada tiap TP.
	recap, err := store.AssessmentTPRecap(ctx, teacher.ID, semesterID, classID, subjectID)
	if err != nil {
		t.Fatal("baca rekap", err)
	}
	var foundA *AssessmentTPRecapStudent
	for i := range recap.Students {
		if recap.Students[i].ID == fmt.Sprint(studentA) {
			foundA = &recap.Students[i]
		}
	}
	if foundA == nil || !foundA.NHComplete || foundA.NH != "85,00" || len(foundA.Objectives) != 2 {
		t.Fatalf("NH/TP roster tidak sesuai: %+v", recap.Students)
	}
	if foundA.Objectives[0].ScoredCount != 3 || foundA.Objectives[0].RequiredCount != 3 || foundA.Objectives[0].Average != "80,00" {
		t.Fatalf("rata-rata tiga kegiatan TP1 harus 80,00 dan Formatif diabaikan: %+v", foundA.Objectives[0])
	}
	if foundA.Objectives[1].ScoredCount != 1 || foundA.Objectives[1].RequiredCount != 1 || foundA.Objectives[1].Average != "90,00" {
		t.Fatalf("nilai TP2 dari kegiatan multi-TP salah: %+v", foundA.Objectives[1])
	}
	if err = store.CorrectAssessmentInitialScore(ctx, teacher.ID, correctionScoreID, 0, "70", "Koreksi uji rekap"); err != nil {
		t.Fatal("koreksi nilai uji", err)
	}
	// Setelah histori append-only dibuat, fixture harus dipertahankan agar
	// cleanup tidak mencoba menghapus bukti yang memang tidak dapat dihapus.
	keepFixture = true
	if err = store.RecordAssessmentScoreRemedial(ctx, teacher.ID, correctionScoreID, submissionKey(1), "85", today); err != nil {
		t.Fatal("remedial di atas koreksi", err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, teacher.ID, correctionScoreID, submissionKey(2), "65", today); err != nil {
		t.Fatal("remedial di bawah koreksi", err)
	}
	effectiveRecap, err := store.AssessmentTPRecap(ctx, teacher.ID, semesterID, classID, subjectID)
	if err != nil {
		t.Fatal("baca rekap setelah koreksi dan remedial", err)
	}
	for _, student := range effectiveRecap.Students {
		if student.ID == fmt.Sprint(studentA) && (student.Objectives[0].Average != "88,33" || student.NH != "89,17" || !student.NHComplete) {
			t.Fatalf("nilai efektif Sumatif tidak memakai koreksi/remedial satu kali: %+v", student)
		}
	}
	if _, err = store.AssessmentTPRecap(ctx, otherTeacher.ID, semesterID, classID, subjectID); err != ErrForbidden {
		t.Fatalf("guru tanpa penugasan harus ditolak: %v", err)
	}
	var beforeScores, beforeCorrections, beforeRemedials int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE school_year_id=$1`, yearID).Scan(&beforeScores); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_corrections c JOIN app.assessment_activity_initial_scores sc ON sc.id=c.initial_score_id WHERE sc.school_year_id=$1`, yearID).Scan(&beforeCorrections); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_remedials r JOIN app.assessment_activity_initial_scores sc ON sc.id=r.initial_score_id WHERE sc.school_year_id=$1`, yearID).Scan(&beforeRemedials); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AssessmentTPRecap(ctx, teacher.ID, semesterID, classID, subjectID); err != nil {
		t.Fatal(err)
	}
	var afterScores, afterCorrections, afterRemedials int
	_ = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE school_year_id=$1`, yearID).Scan(&afterScores)
	_ = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_corrections c JOIN app.assessment_activity_initial_scores sc ON sc.id=c.initial_score_id WHERE sc.school_year_id=$1`, yearID).Scan(&afterCorrections)
	_ = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_remedials r JOIN app.assessment_activity_initial_scores sc ON sc.id=r.initial_score_id WHERE sc.school_year_id=$1`, yearID).Scan(&afterRemedials)
	if beforeScores != afterScores || beforeCorrections != afterCorrections || beforeRemedials != afterRemedials {
		t.Fatalf("read service mengubah histori: scores %d/%d corrections %d/%d remedials %d/%d", beforeScores, afterScores, beforeCorrections, afterCorrections, beforeRemedials, afterRemedials)
	}
	if err = store.CreateLearningObjective(ctx, teacher.ID, semesterID, classID, subjectID, "TP tanpa Sumatif "+suffix); err != nil {
		t.Fatal("buat TP tanpa bukti", err)
	}
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND definition=$4`, semesterID, classID, subjectID, "TP tanpa Sumatif "+suffix).Scan(&objective3); err != nil {
		t.Fatal("baca TP tanpa bukti", err)
	}
	noEvidence, err := store.AssessmentTPRecap(ctx, teacher.ID, semesterID, classID, subjectID)
	if err != nil {
		t.Fatal("baca rekap TP tanpa bukti", err)
	}
	for _, student := range noEvidence.Students {
		if student.ID == fmt.Sprint(studentA) && (student.NHComplete || student.Objectives[2].Complete || student.Objectives[2].RequiredCount != 0) {
			t.Fatalf("TP tanpa Sumatif tidak boleh menampilkan NH lengkap: %+v", student)
		}
	}
	activityPending := createActivity("Sumatif setelah penempatan", "sumatif", tomorrow, objective1)
	pending, err := store.AssessmentTPRecap(ctx, teacher.ID, semesterID, classID, subjectID)
	if err != nil {
		t.Fatal("baca rekap kewajiban setelah tanggal penempatan", err)
	}
	for _, student := range pending.Students {
		if student.ID == fmt.Sprint(studentB) && (student.Objectives[0].RequiredCount != 1 || student.Objectives[0].ScoredCount != 0 || student.NHComplete) {
			t.Fatalf("kegiatan setelah tanggal penempatan harus menjadi kewajiban kosong siswa pindahan: %+v", student)
		}
		if student.ID == fmt.Sprint(studentA) && (student.Objectives[0].RequiredCount != 4 || student.Objectives[0].ScoredCount != 3 || student.NHComplete) {
			t.Fatalf("kegiatan wajib tanpa nilai harus tetap kosong dan mencegah NH lengkap: %+v", student)
		}
	}
	if activityPending == 0 {
		t.Fatal("fixture kegiatan tertunda tidak dibuat")
	}
	if err = store.CreateLearningObjective(ctx, teacher.ID, semesterID, classID, subjectID, "TP nilai nol "+suffix); err != nil {
		t.Fatal("buat TP nilai nol", err)
	}
	var objectiveZero int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND definition=$4`, semesterID, classID, subjectID, "TP nilai nol "+suffix).Scan(&objectiveZero); err != nil {
		t.Fatal("baca TP nilai nol", err)
	}
	activityZero := createActivity("Sumatif nilai nol", "sumatif", today, objectiveZero)
	if err = store.SaveAssessmentActivityInitialScores(ctx, teacher.ID, activityZero, []AssessmentScoreInput{{StudentID: studentA, ObjectiveID: objectiveZero, Value: "0"}}); err != nil {
		t.Fatal("simpan nilai nol", err)
	}
	zeroRecap, err := store.AssessmentTPRecap(ctx, teacher.ID, semesterID, classID, subjectID)
	if err != nil {
		t.Fatal("baca rekap nilai nol", err)
	}
	for _, student := range zeroRecap.Students {
		if student.ID != fmt.Sprint(studentA) {
			continue
		}
		foundZero := false
		for _, objective := range student.Objectives {
			if objective.ID == fmt.Sprint(objectiveZero) {
				foundZero = objective.Complete && objective.RequiredCount == 1 && objective.ScoredCount == 1 && objective.Average == "0,00"
			}
		}
		if !foundZero {
			t.Fatalf("nilai nol yang tercatat harus sah dan dihitung: %+v", student)
		}
	}
}
