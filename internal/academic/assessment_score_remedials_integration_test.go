//go:build integration

package academic

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAssessmentScoreRemedialsAreAppendOnlyScopedAndUseMaximumIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	owner, _ := testutil.User(t, db, "guru_mapel")
	other, _ := testutil.User(t, db, "guru_mapel")
	suffix := testutil.Random(t)[:10]
	var yearID, semesterID, classID, subjectID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "Remedial CP10-07 "+suffix).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "R"+suffix[:6]).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "R07-"+suffix, "Mapel remedial "+suffix).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	masters := &master.Store{DB: db}
	ownerTeacher, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru remedial " + suffix, UserID: owner.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	otherTeacher, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru baru remedial " + suffix, UserID: other.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{DB: db}
	start := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -30)
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, ownerTeacher.ID, start); err != nil {
		t.Fatal("penugasan resmi", err)
	}
	if err = store.CreateLearningObjective(ctx, owner.ID, semesterID, classID, subjectID, "TP remedial "+suffix); err != nil {
		t.Fatal("buat TP", err)
	}
	var objectiveID int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semesterID, classID, subjectID).Scan(&objectiveID); err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	keyNumber := 0
	submissionKey := func() string { keyNumber++; return fmt.Sprintf("00000000-0000-4000-8000-%012x", keyNumber) }
	if err = store.CreateAssessmentActivity(ctx, owner.ID, semesterID, classID, subjectID, "Sumatif remedial "+suffix, "sumatif", date, []int64{objectiveID}); err != nil {
		t.Fatal("buat Sumatif", err)
	}
	if err = store.CreateAssessmentActivity(ctx, owner.ID, semesterID, classID, subjectID, "Formatif remedial "+suffix, "formatif", date, []int64{objectiveID}); err != nil {
		t.Fatal("buat Formatif", err)
	}
	var sumatifID, formatifID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND activity_type='sumatif'`, yearID).Scan(&sumatifID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND activity_type='formatif'`, yearID).Scan(&formatifID); err != nil {
		t.Fatal(err)
	}
	var studentA, studentB, placementA, placementB int64
	for i, target := range []*int64{&studentA, &studentB} {
		if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','L') RETURNING id`, fmt.Sprintf("Siswa remedial %d %s", i, suffix)).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	for i, pair := range [][2]*int64{{&placementA, &studentA}, {&placementB, &studentB}} {
		if err = db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date-30) RETURNING id`, yearID, *pair[1], classID, admin.ID, date).Scan(pair[0]); err != nil {
			t.Fatal(i, err)
		}
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, owner.ID, sumatifID, []AssessmentScoreInput{
		{StudentID: studentA, ObjectiveID: objectiveID, Value: "60"},
		{StudentID: studentB, ObjectiveID: objectiveID, Value: "0"},
	}); err != nil {
		t.Fatal("nilai awal Sumatif", err)
	}
	if err = store.SaveAssessmentActivityInitialScores(ctx, owner.ID, formatifID, []AssessmentScoreInput{{StudentID: studentA, ObjectiveID: objectiveID, Value: "70"}}); err != nil {
		t.Fatal("nilai awal Formatif", err)
	}
	var scoreA, scoreB, formatifScore int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2`, sumatifID, studentA).Scan(&scoreA); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2`, sumatifID, studentB).Scan(&scoreB); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2`, formatifID, studentA).Scan(&formatifScore); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var count int
		if cleanupErr := db.QueryRow(`SELECT (SELECT count(*) FROM app.assessment_activity_score_remedials r JOIN app.assessment_activity_initial_scores sc ON sc.id=r.initial_score_id JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id WHERE a.school_year_id=$1) + (SELECT count(*) FROM app.assessment_activity_score_corrections c JOIN app.assessment_activity_initial_scores sc ON sc.id=c.initial_score_id JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id WHERE a.school_year_id=$1)`, yearID).Scan(&count); cleanupErr != nil {
			t.Errorf("periksa remedial append-only: %v", cleanupErr)
			return
		}
		if count > 0 {
			t.Log("fixture dipertahankan karena histori remedial bersifat append-only")
			return
		}
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_score_corrections WHERE initial_score_id IN (SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1))`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_initial_scores WHERE assessment_activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_objectives WHERE activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activities WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.student_placements WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.students WHERE id=ANY($1::bigint[])`, []int64{studentA, studentB})
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id=$1)`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, ownerTeacher.ID)
		_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, ownerTeacher.ID)
		_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, otherTeacher.ID)
		_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, otherTeacher.ID)
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code=$1`, "R07-"+suffix)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id=$1`, subjectID)
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.classes WHERE id=$1`, classID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		for _, id := range []int64{admin.ID, curriculum.ID, owner.ID, other.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				_, _ = db.Exec(q, id)
			}
		}
	})

	// Koreksi baseline 60 menjadi 70; remedial tetap menjadi catatan baru.
	if err = store.CorrectAssessmentInitialScore(ctx, owner.ID, scoreA, 0, "70", "Koreksi fixture"); err != nil {
		t.Fatal("koreksi nilai awal fixture", err)
	}
	// Guru baru memiliki assignment aktif tetapi bukan pemilik kegiatan atau nilai awal.
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, otherTeacher.ID, time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -1)); err != nil {
		t.Fatal("pengalihan fixture ke guru baru", err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, other.ID, scoreA, submissionKey(), "90", date); err == nil {
		t.Error("guru baru dengan assignment aktif dapat memberi remedial atas bukti sebelumnya")
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, ownerTeacher.ID, time.Now().In(time.FixedZone("WIB", 7*60*60))); err != nil {
		t.Fatal("kembalikan assignment fixture ke pemilik", err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, other.ID, scoreA, submissionKey(), "90", date); err == nil {
		t.Error("guru dengan assignment kedaluwarsa dapat memberi remedial")
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, otherTeacher.ID, time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, 7)); err != nil {
		t.Fatal("buat assignment masa depan", err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, other.ID, scoreA, submissionKey(), "90", date); err == nil {
		t.Error("guru dengan assignment masa depan dapat memberi remedial")
	}
	for _, value := range []string{"75", "85", "65"} {
		if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, submissionKey(), value, date); err != nil {
			t.Fatalf("catat remedial %s: %v", value, err)
		}
	}
	// Nilai batas 0 dan 100 sah; nilai efektif menjadi maksimum 100.
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreB, submissionKey(), "0", date); err != nil {
		t.Fatal("nilai 0 harus sah", err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreB, submissionKey(), "100", date); err != nil {
		t.Fatal("nilai 100 harus sah", err)
	}
	for _, invalid := range []string{"-0.01", "100.01", "82.567", ""} {
		if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, submissionKey(), invalid, date); err == nil {
			t.Errorf("nilai tidak sah %q diterima", invalid)
		}
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, submissionKey(), "80", "2026-02-30"); err == nil {
		t.Error("tanggal remedial invalid diterima")
	}
	yesterday := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, 1).Format("2006-01-02")
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, submissionKey(), "80", yesterday); err == nil {
		t.Error("remedial sebelum tanggal kegiatan diterima")
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, submissionKey(), "80", tomorrow); err == nil {
		t.Error("remedial setelah hari ini WIB diterima")
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, formatifScore, submissionKey(), "90", date); err == nil {
		t.Error("remedial Formatif diterima")
	}
	if _, err = db.Exec(`INSERT INTO app.assessment_activity_score_remedials(initial_score_id,score,remedial_date,recorded_by) VALUES($1,90,$2::date,$3)`, formatifScore, date, owner.ID); err == nil {
		t.Error("guard database menerima remedial pada nilai awal Formatif")
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, 9223372036854770000, submissionKey(), "90", date); err == nil {
		t.Error("remedial tanpa nilai awal diterima")
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, other.ID, scoreA, submissionKey(), "90", date); err == nil {
		t.Error("guru lain tanpa assignment tidak berhak")
	}
	// Replay kunci dan isi yang sama tidak menggandakan histori. Kunci baru
	// tetap membuat catatan terpisah walau nilai dan tanggalnya sama.
	replayKey := submissionKey()
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, replayKey, "75", date); err != nil {
		t.Fatalf("submission remedial gagal: %v", err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, replayKey, "75.00", date); err != nil {
		t.Fatalf("replay identik harus berhasil: %v", err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, replayKey, "76", date); err == nil {
		t.Fatal("kunci sama dengan payload berbeda tidak menghasilkan konflik")
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, submissionKey(), "75", date); err != nil {
		t.Fatalf("submission terpisah dengan nilai/tanggal sama ditolak: %v", err)
	}
	var initial, corrected, maxRemedial, count string
	err = db.QueryRow(`SELECT sc.score::text,c.after_score::text,max(r.score)::text,count(r.id)::text
		FROM app.assessment_activity_initial_scores sc
		JOIN app.assessment_activity_score_corrections c ON c.initial_score_id=sc.id
		LEFT JOIN app.assessment_activity_score_remedials r ON r.initial_score_id=sc.id
		WHERE sc.id=$1 GROUP BY sc.id,c.after_score`, scoreA).Scan(&initial, &corrected, &maxRemedial, &count)
	if err != nil || initial != "60.00" || corrected != "70.00" || maxRemedial != "85.00" || count != "5" {
		t.Fatalf("snapshot nilai tidak sesuai initial=%s corrected=%s max=%s count=%s err=%v", initial, corrected, maxRemedial, count, err)
	}
	detail, err := store.AssessmentScoreRemedialDetail(ctx, owner.ID, scoreA)
	if err != nil || detail.CorrectedInitial != "70.00" || detail.Effective != "85.00" || len(detail.History) != 5 {
		t.Fatalf("detail remedial tidak memakai maksimum/history: %+v err=%v", detail, err)
	}
	ownedScores, err := store.OwnedAssessmentInitialScores(ctx, owner.ID, AssessmentOwnedScoreFilter{Page: 1})
	if err != nil {
		t.Fatalf("daftar nilai milik guru gagal: %v", err)
	}
	var listed *AssessmentOwnedScore
	for i := range ownedScores.Items {
		if ownedScores.Items[i].ID == scoreA {
			listed = &ownedScores.Items[i]
			break
		}
	}
	if listed == nil || listed.InitialValue != "60.00" || listed.CorrectedValue != "70.00" || listed.EffectiveValue != "85.00" {
		t.Fatalf("daftar mencampur nilai asli, terkoreksi, dan efektif: %+v", listed)
	}
	correctionDetail, err := store.AssessmentScoreCorrectionDetail(ctx, owner.ID, scoreA)
	if err != nil || correctionDetail.Score.InitialValue != "60.00" || correctionDetail.Score.CorrectedValue != "70.00" || correctionDetail.Score.EffectiveValue != "85.00" {
		t.Fatalf("detail koreksi tidak memisahkan nilai atau tidak menerapkan maksimum Sumatif: %+v err=%v", correctionDetail.Score, err)
	}
	sheetAt85, err := store.AssessmentActivityScoreSheet(ctx, owner.ID, sumatifID)
	if err != nil {
		t.Fatalf("baca lembar nilai setelah koreksi dan remedial: %v", err)
	}
	var effectiveAt85 string
	for _, student := range sheetAt85.Students {
		if student.ID != studentA {
			continue
		}
		for _, cell := range student.Scores {
			if cell.ObjectiveID == objectiveID {
				effectiveAt85 = cell.Value
			}
		}
	}
	if effectiveAt85 != "85.00" {
		t.Fatalf("lembar nilai harus menampilkan maksimum nilai terkoreksi 70 dan remedial 85, dapat %q", effectiveAt85)
	}
	if err = store.CorrectAssessmentInitialScore(ctx, owner.ID, scoreA, 1, "90", "Koreksi setelah remedial"); err != nil {
		t.Fatal("koreksi awal setelah remedial", err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, submissionKey(), "80", date); err != nil {
		t.Fatal("remedial di bawah nilai awal terkoreksi", err)
	}
	detail, err = store.AssessmentScoreRemedialDetail(ctx, owner.ID, scoreA)
	if err != nil || detail.CorrectedInitial != "90.00" || detail.Effective != "90.00" || len(detail.History) != 6 {
		t.Fatalf("remedial lebih rendah tidak mempertahankan nilai terkoreksi: %+v err=%v", detail, err)
	}
	var effective string
	if err = db.QueryRow(`SELECT greatest(coalesce(c.after_score,sc.score),coalesce(max(r.score),coalesce(c.after_score,sc.score)))::text FROM app.assessment_activity_initial_scores sc LEFT JOIN app.assessment_activity_score_corrections c ON c.initial_score_id=sc.id LEFT JOIN app.assessment_activity_score_remedials r ON r.initial_score_id=sc.id WHERE sc.id=$1 GROUP BY sc.id,c.after_score`, scoreB).Scan(&effective); err != nil || effective != "100.00" {
		t.Fatalf("nilai batas tidak menghasilkan efektif 100: %s err=%v", effective, err)
	}
	if _, err = store.AssessmentScoreRemedialDetail(ctx, other.ID, scoreA); err == nil {
		t.Error("guru lain dapat membaca detail remedial pemilik")
	}
	if _, err = db.Exec(`UPDATE app.assessment_activity_score_remedials SET score=1 WHERE initial_score_id=$1`, scoreA); err == nil {
		t.Error("baris remedial dapat diubah")
	}
	if _, err = db.Exec(`DELETE FROM app.assessment_activity_score_remedials WHERE initial_score_id=$1`, scoreA); err == nil {
		t.Error("baris remedial dapat dihapus")
	}

	// Dua request konkuren dengan kunci sama hanya boleh membuat satu baris.
	var wg sync.WaitGroup
	results := make(chan error, 4)
	concurrentKey := submissionKey()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, concurrentKey, "72", date)
		}()
	}
	// Dua kunci berbeda dengan payload sama tetap mewakili dua tindakan.
	for i := 0; i < 2; i++ {
		key := submissionKey()
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			results <- store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, key, "73", date)
		}(key)
	}
	wg.Wait()
	close(results)
	for err = range results {
		if err != nil {
			t.Errorf("remedial konkuren gagal: %v", err)
		}
	}
	if err = db.QueryRow(`SELECT count(*)::text FROM app.assessment_activity_score_remedials WHERE initial_score_id=$1`, scoreA).Scan(&count); err != nil || count != "9" {
		t.Fatalf("remedial konkuren tidak seluruhnya tercatat: count=%s err=%v", count, err)
	}
	var normalized sql.NullString
	if err = db.QueryRow(`SELECT max(score)::text FROM app.assessment_activity_score_remedials WHERE initial_score_id=$1`, scoreA).Scan(&normalized); err != nil || normalized.String != "85.00" {
		t.Fatalf("remedial rendah menurunkan maksimum: %v err=%v", normalized, err)
	}
	var formatifRemedials int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_remedials WHERE initial_score_id=$1`, formatifScore).Scan(&formatifRemedials); err != nil || formatifRemedials != 0 {
		t.Fatalf("remedial Formatif menciptakan histori: count=%d err=%v", formatifRemedials, err)
	}
	sheet, err := store.AssessmentActivityScoreSheet(ctx, owner.ID, sumatifID)
	if err != nil {
		t.Fatal("baca lembar nilai Sumatif", err)
	}
	var sumatifCell *AssessmentScoreCell
	for _, student := range sheet.Students {
		if student.ID == studentA {
			for i := range student.Scores {
				if student.Scores[i].ObjectiveID == objectiveID {
					sumatifCell = &student.Scores[i]
				}
			}
		}
	}
	if sumatifCell == nil || sumatifCell.Value != "90.00" || !sumatifCell.CanRemediate {
		t.Fatalf("lembar Sumatif tidak menampilkan nilai efektif dan aksi remedial: %+v", sumatifCell)
	}
	formatifSheet, err := store.AssessmentActivityScoreSheet(ctx, owner.ID, formatifID)
	if err != nil {
		t.Fatal("baca lembar nilai Formatif", err)
	}
	for _, student := range formatifSheet.Students {
		for _, cell := range student.Scores {
			if student.ID == studentA && cell.ObjectiveID == objectiveID && cell.CanRemediate {
				t.Fatal("lembar Formatif menampilkan aksi remedial")
			}
		}
	}
	if _, err = db.Exec(`UPDATE app.teachers SET active=false WHERE id=$1`, ownerTeacher.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordAssessmentScoreRemedial(ctx, owner.ID, scoreA, submissionKey(), "99", date); err == nil {
		t.Error("guru dengan identitas nonaktif dapat menambah remedial")
	}
}
