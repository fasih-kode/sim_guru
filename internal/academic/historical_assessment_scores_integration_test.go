//go:build integration

package academic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

type historicalScoreDBFixture struct {
	f                                                                                             testutil.ExamFixture
	year, routineTP, historicalTP, activity21, activity22, placementA, placementB, placementMoved int64
	snapshot                                                                                      string
	hash                                                                                          []byte
}

func newHistoricalScoreDBFixture(t *testing.T, db *sql.DB, suffix string) historicalScoreDBFixture {
	t.Helper()
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, suffix+testutil.Random(t)[:8])
	s := &Store{DB: db}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester)
	mustExec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,actor_id) VALUES($1,0,40,60,$2)`, f.Semester, f.Teacher)
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP rutin CP10-23"); err != nil {
		t.Fatal(err)
	}
	var x historicalScoreDBFixture
	x.f = f
	if err := db.QueryRowContext(ctx, `SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&x.routineTP); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif lama CP10-23", "sumatif", "2020-01-11", []int64{x.routineTP}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&x.activity21); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, x.activity21, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: x.routineTP, Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !p.CanFinalize {
		t.Fatalf("Final fixture: %+v %v", p, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 82, "", p.SnapshotToken, "00000000-0000-4000-8000-000000002301"); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT snapshot::text,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&x.snapshot, &x.hash); err != nil {
		t.Fatal(err)
	}
	var teacherName string
	if err = db.QueryRowContext(ctx, `SELECT name FROM app.teachers WHERE id=$1`, f.TeacherID).Scan(&teacherName); err != nil {
		t.Fatal(err)
	}
	tpInput := HistoricalTPPreviewInput{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, Definition: "TP historis CP10-23", Reason: "TP terlupa dicatat", AssessmentDate: "2020-01-10", DateSource: "arsip", Learning: HistoricalTPReference{Kind: "manual", Document: "Jurnal CP10-23", Archive: "arsip uji", Teacher: teacherName, Date: "2020-01-10"}, Assessment: HistoricalTPReference{Kind: "manual", Document: "Sumatif CP10-23", Archive: "arsip uji", Teacher: teacherName, ResultSource: "lembar nilai"}}
	tpPreview, err := s.PreviewHistoricalTP(ctx, f.Teacher, tpInput, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	x.historicalTP, err = s.ConfirmHistoricalLearningObjective(ctx, f.Teacher, HistoricalTPConfirmation{Preview: tpInput, Verification: HistoricalTPVerification{LearningSection: "halaman 1", AssessmentSection: "butir TP", Method: "cocokkan arsip", ResultSource: "lembar nilai", Acknowledged: true}, SnapshotToken: tpPreview.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002302"})
	if err != nil {
		t.Fatal(err)
	}
	link := HistoricalAssessmentLinkInput{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, ObjectiveID: x.historicalTP, ActivityID: x.activity21, Reason: "TP tertinggal pada relasi", Document: "Sumatif CP10-23", Archive: "arsip uji", Section: "butir TP", Method: "pencocokan", ResultSource: "lembar nilai", Acknowledged: true, DateExplanation: "Tanggal bukti TP berbeda sehari dari kegiatan", DateEvidence: "tanggal sumber"}
	lp, err := s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmHistoricalAssessmentLink(ctx, f.Teacher, HistoricalAssessmentLinkConfirmation{Input: link, SnapshotToken: lp.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002303"}); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "90"}}); err != nil {
		t.Fatal(err)
	}
	activityInput := HistoricalAssessmentActivityInput{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, ObjectiveIDs: []int64{x.routineTP, x.historicalTP}, Name: "Sumatif arsip CP10-23", ActivityDate: "2020-01-12", Reason: "Kegiatan arsip tertinggal", SourceIdentity: "cp1023-" + testutil.Random(t)[:12], SourceDocument: "Daftar Sumatif", SourceArchive: "arsip uji", SourceTeacher: teacherName, SourceSection: "lembar soal", ResultSource: "lembar nilai", Method: "cocokkan dokumen", Acknowledged: true, DuplicateChecked: true}
	ap, err := s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, activityInput, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	_, x.activity22, err = s.ConfirmHistoricalAssessmentActivity(ctx, f.Teacher, HistoricalAssessmentActivityConfirmation{Input: activityInput, SnapshotToken: ap.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002304"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&x.year); err != nil {
		t.Fatal(err)
	}
	for student, dest := range map[int64]*int64{f.StudentA: &x.placementA, f.StudentB: &x.placementB, f.MovedStudent: &x.placementMoved} {
		if err = db.QueryRowContext(ctx, `SELECT id FROM app.student_placements WHERE student_id=$1 AND school_year_id=$2 ORDER BY id DESC LIMIT 1`, student, x.year).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	return x
}

func insertHistoricalScoreBatch(ctx context.Context, db *sql.DB, x historicalScoreDBFixture, activity, objective, student, placement int64, score string, key string, auditedScore string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var date string
	if err = tx.QueryRowContext(ctx, `SELECT activity_date::text FROM app.assessment_activities WHERE id=$1`, activity).Scan(&date); err != nil {
		return err
	}
	var batch int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.historical_assessment_score_batches(assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'Pencatatan arsip tertinggal','Dokumen dicocokkan per siswa dan TP','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),$9) RETURNING id`, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, key).Scan(&batch); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.historical_assessment_initial_score_audits(batch_id,assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,learning_objective_id,student_id,placement_id,score,source_kind,source_identity,source_locator,source_score,source_result,verification) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'arsip_manual','Dokumen sintetis CP10-23',$14,$13,'hasil siswa pada TP','nilai dan identitas dicocokkan')`, batch, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, objective, student, placement, auditedScore, fmt.Sprintf("halaman 2, siswa %d, TP %d", student, objective)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.assessment_activity_initial_scores(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, activity, objective, x.year, x.f.Semester, x.f.Class, x.f.Subject, student, placement, date, score, x.f.Teacher); err != nil {
		return err
	}
	return tx.Commit()
}

type historicalScoreSlot struct {
	objective, student, placement int64
	score                         string
}

func insertHistoricalScoreMultiBatch(ctx context.Context, db *sql.DB, x historicalScoreDBFixture, activity int64, key string, slots []historicalScoreSlot) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var date string
	if err = tx.QueryRowContext(ctx, `SELECT activity_date::text FROM app.assessment_activities WHERE id=$1`, activity).Scan(&date); err != nil {
		return err
	}
	var batch int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.historical_assessment_score_batches(assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'Pencatatan multi-TP','Dokumen dicocokkan per siswa dan TP','{}',decode(repeat('33',32),'hex'),decode(repeat('44',32),'hex'),$9) RETURNING id`, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, key).Scan(&batch); err != nil {
		return err
	}
	for _, slot := range slots {
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.historical_assessment_initial_score_audits(batch_id,assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,learning_objective_id,student_id,placement_id,score,source_kind,source_identity,source_locator,source_score,source_result,verification) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'arsip_manual','Dokumen sintetis CP10-23',$14,$13,'hasil per siswa dan TP','nilai dan identitas dicocokkan')`, batch, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, slot.objective, slot.student, slot.placement, slot.score, fmt.Sprintf("halaman 3, siswa %d, TP %d", slot.student, slot.objective)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.assessment_activity_initial_scores(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, activity, slot.objective, x.year, x.f.Semester, x.f.Class, x.f.Subject, slot.student, slot.placement, date, slot.score, x.f.Teacher); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func TestHistoricalAssessmentScoreAuditDatabaseGuardsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	x := newHistoricalScoreDBFixture(t, db, "cp1023db")
	ctx := context.Background()
	if err := insertHistoricalScoreEvidenceAttempt(ctx, db, x, x.activity22, x.routineTP, x.f.StudentA, x.placementA, "00000000-0000-4000-8000-000000002319", "aplikasi", "75", "75"); err == nil {
		t.Fatal("SQL langsung menerima sumber aplikasi yang belum dapat diverifikasi")
	}
	if err := insertHistoricalScoreEvidenceAttempt(ctx, db, x, x.activity22, x.routineTP, x.f.StudentA, x.placementA, "00000000-0000-4000-8000-000000002320", "arsip_manual", "75", "74.99"); err == nil {
		t.Fatal("SQL langsung menerima nilai sumber yang berbeda dari transkripsi")
	}
	// Pasangan CP10-21 dan seluruh TP kegiatan CP10-22 tidak menerima SQL rutin.
	for _, target := range []struct{ activity, objective int64 }{{x.activity21, x.historicalTP}, {x.activity22, x.routineTP}, {x.activity22, x.historicalTP}} {
		_, err := db.ExecContext(ctx, `INSERT INTO app.assessment_activity_initial_scores(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by) SELECT $1,$2,$3,$4,$5,$6,$7,$8,a.activity_date,75,$9 FROM app.assessment_activities a WHERE a.id=$1`, target.activity, target.objective, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.StudentA, x.placementA, x.f.Teacher)
		if err == nil {
			t.Fatalf("SQL tanpa audit diterima untuk activity=%d objective=%d", target.activity, target.objective)
		}
	}
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity21, x.routineTP, x.f.StudentA, x.placementA, "70", "00000000-0000-4000-8000-000000002317", "70"); err == nil {
		t.Fatal("batch audit menerima pasangan TP rutin yang tidak ditautkan secara historis")
	}
	// Pasangan rutin di kegiatan CP10-21 tetap berjalan.
	s := &Store{DB: db}
	if err := s.SaveAssessmentActivityInitialScores(ctx, x.f.Teacher, x.activity21, []AssessmentScoreInput{{StudentID: x.f.StudentB, ObjectiveID: x.routineTP, Value: "0"}}); err != nil {
		t.Fatalf("nilai rutin pasangan biasa ditolak: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO app.assessment_activity_initial_scores(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by) SELECT $1,$2,$3,$4,$5,$6,$7,$8,a.activity_date,77,$9 FROM app.assessment_activities a WHERE a.id=$1`, x.activity21, x.routineTP, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.MovedStudent, x.placementMoved, x.f.Teacher); err != nil {
		t.Fatalf("SQL rutin pada pasangan biasa ditolak: %v", err)
	}
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity21, x.historicalTP, x.f.StudentB, x.placementB, "100", "00000000-0000-4000-8000-000000002313", "100"); err != nil {
		t.Fatalf("pasangan CP10-21 dengan audit yang cocok ditolak: %v", err)
	}
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity21, x.routineTP, x.f.StudentA, x.placementA, "70", "00000000-0000-4000-8000-000000002314", "70"); err == nil {
		t.Fatal("audit nilai tidak membuka pasangan rutin pada kegiatan lama")
	}
	key1, key2 := "00000000-0000-4000-8000-000000002305", "00000000-0000-4000-8000-000000002306"
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity22, x.routineTP, x.f.StudentA, x.placementA, "0", key1, "0"); err != nil {
		t.Fatalf("nilai nol sah: %v", err)
	}
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity22, x.historicalTP, x.f.StudentB, x.placementB, "82.50", key2, "82.50"); err != nil {
		t.Fatalf("batch multi-TP sah: %v", err)
	}
	if err := insertHistoricalScoreMultiBatch(ctx, db, x, x.activity22, "00000000-0000-4000-8000-000000002315", []historicalScoreSlot{{objective: x.routineTP, student: x.f.MovedStudent, placement: x.placementMoved, score: "99.99"}, {objective: x.historicalTP, student: x.f.MovedStudent, placement: x.placementMoved, score: "100"}}); err != nil {
		t.Fatalf("dua TP dalam satu batch atomik: %v", err)
	}
	if err := insertHistoricalScoreMultiBatch(ctx, db, x, x.activity22, "00000000-0000-4000-8000-000000002316", []historicalScoreSlot{{objective: x.routineTP, student: x.f.MovedStudent, placement: x.placementMoved, score: "88"}, {objective: x.historicalTP, student: x.f.StudentA, placement: x.placementB, score: "77"}}); err == nil {
		t.Fatal("batch dengan placement siswa kedua yang salah berhasil")
	}
	// Audit/nilai tidak cocok, slot duplikat, dan placement siswa lain harus rollback penuh.
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity22, x.routineTP, x.f.StudentB, x.placementB, "75", "00000000-0000-4000-8000-000000002307", "74"); err == nil {
		t.Fatal("nilai tidak sama dengan audit diterima")
	}
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity22, x.routineTP, x.f.StudentA, x.placementA, "75", "00000000-0000-4000-8000-000000002308", "75"); err == nil {
		t.Fatal("slot yang sudah dicatat menerima submission UUID berbeda")
	}
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity22, x.historicalTP, x.f.StudentA, x.placementB, "75", "00000000-0000-4000-8000-000000002309", "75"); err == nil {
		t.Fatal("placement milik siswa lain diterima")
	}
	if err := insertHistoricalScoreAuditOnly(ctx, db, x, x.activity22, x.routineTP, x.f.StudentA, x.placementA, "00000000-0000-4000-8000-000000002310"); err == nil {
		t.Fatal("audit tanpa nilai dapat commit")
	}
	if err := insertHistoricalScoreBatchOnly(ctx, db, x, x.activity22, "00000000-0000-4000-8000-000000002318"); err == nil {
		t.Fatal("batch tanpa audit detail dapat commit")
	}
	// Duplikasi submission UUID ditolak, meski detailnya berbeda.
	if err := insertHistoricalScoreBatch(ctx, db, x, x.activity22, x.historicalTP, x.f.StudentA, x.placementA, "70", key1, "70"); err == nil {
		t.Fatal("submission UUID yang sama diterima ulang")
	}
	if _, err := db.ExecContext(ctx, `UPDATE app.assessment_activity_initial_scores SET score=1 WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3`, x.activity22, x.routineTP, x.f.StudentA); err == nil {
		t.Fatal("UPDATE nilai berprovenance diterima")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3`, x.activity22, x.routineTP, x.f.StudentA); err == nil {
		t.Fatal("DELETE nilai berprovenance diterima")
	}
	var batches, details int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM app.historical_assessment_score_batches WHERE assessment_activity_id=$1`, x.activity22).Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1`, x.activity22).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if batches != 3 || details != 4 {
		t.Fatalf("rollback parsial: batches=%d details=%d", batches, details)
	}
	if _, err := db.ExecContext(ctx, `UPDATE app.historical_assessment_score_batches SET reason='diubah' WHERE assessment_activity_id=$1`, x.activity22); err == nil {
		t.Fatal("UPDATE audit batch diterima")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1`, x.activity22); err == nil {
		t.Fatal("DELETE audit detail diterima")
	}
	var finalAfter string
	var hashAfter []byte
	if err := db.QueryRowContext(ctx, `SELECT snapshot::text,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, x.f.Semester, x.f.StudentA).Scan(&finalAfter, &hashAfter); err != nil {
		t.Fatal(err)
	}
	if finalAfter != x.snapshot || string(hashAfter) != string(x.hash) {
		t.Fatal("Final/snapshot historis berubah")
	}
}

func insertHistoricalScoreEvidenceAttempt(ctx context.Context, db *sql.DB, x historicalScoreDBFixture, activity, objective, student, placement int64, key, sourceKind, score, sourceScore string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var date string
	if err = tx.QueryRowContext(ctx, `SELECT activity_date::text FROM app.assessment_activities WHERE id=$1`, activity).Scan(&date); err != nil {
		return err
	}
	var batch int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.historical_assessment_score_batches(assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'uji bukti','guru mencocokkan','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),$9) RETURNING id`, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, key).Scan(&batch); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.historical_assessment_initial_score_audits(batch_id,assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,learning_objective_id,student_id,placement_id,score,source_kind,source_identity,source_locator,source_score,source_result,verification) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'Dokumen CP10-23','halaman 8, siswa A, TP 1',$15,'hasil dicocokkan','guru memeriksa')`, batch, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, objective, student, placement, score, sourceKind, sourceScore)
	return err
}

func insertHistoricalScoreAuditOnly(ctx context.Context, db *sql.DB, x historicalScoreDBFixture, activity, objective, student, placement int64, key string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var date string
	if err = tx.QueryRowContext(ctx, `SELECT activity_date::text FROM app.assessment_activities WHERE id=$1`, activity).Scan(&date); err != nil {
		return err
	}
	var batch int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.historical_assessment_score_batches(assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'Alasan audit test','Pemeriksaan sumber','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),$9) RETURNING id`, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, key).Scan(&batch); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.historical_assessment_initial_score_audits(batch_id,assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,learning_objective_id,student_id,placement_id,score,source_kind,source_identity,source_locator,source_score,source_result,verification) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,75,'arsip_manual','arsip test','halaman 2, baris 3',75,'nilai per TP','dicocokkan')`, batch, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, objective, student, placement); err != nil {
		return err
	}
	return tx.Commit()
}

func insertHistoricalScoreBatchOnly(ctx context.Context, db *sql.DB, x historicalScoreDBFixture, activity int64, key string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var date string
	if err = tx.QueryRowContext(ctx, `SELECT activity_date::text FROM app.assessment_activities WHERE id=$1`, activity).Scan(&date); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.historical_assessment_score_batches(assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'Batch kosong tidak sah','Dokumen diperiksa','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),$9)`, activity, x.year, x.f.Semester, x.f.Class, x.f.Subject, x.f.TeacherID, x.f.Teacher, date, key)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func TestHistoricalAssessmentScoreConcurrentDuplicateIsAtomicIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	x := newHistoricalScoreDBFixture(t, db, "cp1023race")
	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			key := []string{"00000000-0000-4000-8000-000000002311", "00000000-0000-4000-8000-000000002312"}[i]
			errs[i] = insertHistoricalScoreBatch(ctx, db, x, x.activity22, x.routineTP, x.f.StudentA, x.placementA, "60", key, "60")
		}(i)
	}
	close(start)
	wg.Wait()
	success := 0
	for _, err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("konfirmasi bersamaan: berhasil=%d errors=%v", success, errs)
	}
	var scores, audits int
	if err := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3)`, x.activity22, x.routineTP, x.f.StudentA).Scan(&scores, &audits); err != nil || scores != 1 || audits != 1 {
		t.Fatalf("race meninggalkan duplikasi: score=%d audit=%d err=%v", scores, audits, err)
	}
}

func TestHistoricalAssessmentScoreServiceIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	x := newHistoricalScoreDBFixture(t, db, "cp1023svc")
	ctx := context.Background()
	s := &Store{DB: db}
	// K23-01: pemilik asli yang masih aktif tetap dapat menangani nilai
	// terlewat walau assignment kini berakhir, bila assignment tanggal acara sah.
	if _, err := db.ExecContext(ctx, `UPDATE app.teaching_assignments SET effective_until='2021-01-01' WHERE teacher_id=$1 AND semester_id=$2 AND class_id=$3 AND subject_id=$4`, x.f.TeacherID, x.f.Semester, x.f.Class, x.f.Subject); err != nil {
		t.Fatal(err)
	}
	var currentAssignment bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_assignments WHERE teacher_id=$1 AND semester_id=$2 AND class_id=$3 AND subject_id=$4 AND effective_from<=CURRENT_DATE AND (effective_until IS NULL OR effective_until>CURRENT_DATE))`, x.f.TeacherID, x.f.Semester, x.f.Class, x.f.Subject).Scan(&currentAssignment); err != nil || currentAssignment {
		t.Fatalf("fixture seharusnya tidak memiliki assignment saat ini: exists=%v err=%v", currentAssignment, err)
	}
	choices, err := s.HistoricalAssessmentScoreChoices(ctx, x.f.Teacher)
	if err != nil {
		t.Fatal(err)
	}
	var choice *HistoricalAssessmentScoreActivityChoice
	for i := range choices {
		if choices[i].ID == x.activity22 {
			choice = &choices[i]
		}
	}
	if choice == nil || len(choice.Objectives) != 2 {
		t.Fatalf("kegiatan CP22 multi-TP tidak tersedia: %+v", choice)
	}
	input := HistoricalAssessmentScoreInput{
		ActivityID: x.activity22, Reason: "Nilai historis pada lembar arsip",
		Verification: "Nama, kegiatan, tanggal, dan baris nilai dicocokkan dengan sumber sintetis",
		Acknowledged: true,
		Entries: []HistoricalAssessmentScoreEntry{
			{StudentID: x.f.StudentA, ObjectiveID: x.routineTP, Value: "0", SourceKind: "arsip_manual", SourceIdentity: "Arsip Sumatif uji CP10-23", SourceLocator: "halaman 2, siswa A, butir TP rutin", SourceScore: "0", SourceResult: "0 pada kolom TP rutin", Verification: "identitas siswa dan TP cocok"},
			{StudentID: x.f.StudentB, ObjectiveID: x.historicalTP, Value: "82.50", SourceKind: "arsip_manual", SourceIdentity: "Arsip Sumatif uji CP10-23", SourceLocator: "halaman 2, siswa B, butir TP historis", SourceScore: "82,50", SourceResult: "82,50 pada kolom TP historis", Verification: "identitas siswa dan TP cocok"},
		},
	}
	preview, err := s.PreviewHistoricalAssessmentScores(ctx, x.f.Teacher, input, 1, "")
	if err != nil {
		t.Fatalf("pratinjau: %+v cause=%#v", err, errors.Unwrap(err))
	}
	if preview.SelectedCount != 2 || preview.StudentCount < 2 || preview.FinalCount != 1 {
		t.Fatalf("ringkasan pratinjau: %+v", preview)
	}
	var beforeStudent *HistoricalAssessmentScoreStudent
	for i := range preview.Students {
		if preview.Students[i].ID == x.f.StudentA {
			beforeStudent = &preview.Students[i]
		}
	}
	if beforeStudent == nil || beforeStudent.AfterStatus != "Siap Final" || beforeStudent.Cells[0].Value != "0" || !beforeStudent.AfterNeedsReview {
		t.Fatalf("NH 0%% atau nilai nol tidak terproyeksi konsisten: %+v", beforeStudent)
	}
	// Perubahan kalender yang masih menempatkan kegiatan di dalam rentang
	// membuat snapshot lama tidak berlaku, tanpa meninggalkan penulisan parsial.
	if _, err = db.ExecContext(ctx, `UPDATE app.semester_calendars SET starts_on='2020-01-02' WHERE semester_id=$1`, x.f.Semester); err != nil {
		t.Fatal(err)
	}
	confirmation := HistoricalAssessmentScoreConfirmation{Input: input, SnapshotToken: preview.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002330"}
	if _, err = s.ConfirmHistoricalAssessmentScores(ctx, x.f.Teacher, confirmation); !errors.Is(err, ErrConflict) {
		t.Fatalf("token kalender kedaluwarsa: %v", err)
	}
	var scores, batches, details int
	if err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_score_batches WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1)`, x.activity22).Scan(&scores, &batches, &details); err != nil {
		t.Fatal(err)
	}
	if scores != 0 || batches != 0 || details != 0 {
		t.Fatalf("konflik snapshot meninggalkan data parsial: score=%d batch=%d detail=%d", scores, batches, details)
	}
	preview, err = s.PreviewHistoricalAssessmentScores(ctx, x.f.Teacher, input, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	confirmation.SnapshotToken = preview.SnapshotToken
	batchID, err := s.ConfirmHistoricalAssessmentScores(ctx, x.f.Teacher, confirmation)
	if err != nil {
		t.Fatalf("konfirmasi sah: %v", err)
	}
	if got, e := s.ConfirmHistoricalAssessmentScores(ctx, x.f.Teacher, confirmation); e != nil || got != batchID {
		t.Fatalf("replay identik tidak idempoten: id=%d err=%v", got, e)
	}
	changed := confirmation
	changed.Input.Entries = append([]HistoricalAssessmentScoreEntry(nil), input.Entries...)
	changed.Input.Entries[0].Value = "1"
	changed.Input.Entries[0].SourceScore = "1"
	if _, err = s.ConfirmHistoricalAssessmentScores(ctx, x.f.Teacher, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload berbeda pada submission UUID sama: %v", err)
	}
	if _, err = s.HistoricalAssessmentScoreResult(ctx, x.f.Other, batchID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pengguna lain membaca hasil: %v", err)
	}
	result, err := s.HistoricalAssessmentScoreResult(ctx, x.f.Teacher, batchID)
	if err != nil || len(result.Entries) != 2 {
		t.Fatalf("hasil batch: entries=%d err=%v", len(result.Entries), err)
	}
	if result.Entries[0].SourceScore == "" || result.Entries[0].SourceScore != result.Entries[0].Value {
		t.Fatalf("nilai pada arsip tidak terbaca atau berbeda dari nilai tersimpan: %+v", result.Entries[0])
	}
	if err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_score_batches WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1)`, x.activity22).Scan(&scores, &batches, &details); err != nil {
		t.Fatal(err)
	}
	if scores != 2 || batches != 1 || details != 2 {
		t.Fatalf("hasil tidak tepat satu batch atomik: score=%d batch=%d detail=%d", scores, batches, details)
	}
	var finalSnapshot string
	var finalHash []byte
	if err = db.QueryRowContext(ctx, `SELECT snapshot::text,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, x.f.Semester, x.f.StudentA).Scan(&finalSnapshot, &finalHash); err != nil {
		t.Fatal(err)
	}
	if finalSnapshot != x.snapshot || string(finalHash) != string(x.hash) {
		t.Fatal("penyimpanan nilai historis mengubah Final atau snapshot lama")
	}
	if _, err = s.PreviewHistoricalAssessmentScores(ctx, x.f.Other, input, 1, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru bukan pemilik dapat pratinjau: %v", err)
	}
}

func TestHistoricalAssessmentScoreProjectionPositiveNHIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	x := newHistoricalScoreDBFixture(t, db, "cp1023nhpos")
	ctx := context.Background()
	s := &Store{DB: db}
	if _, err := db.ExecContext(ctx, `UPDATE app.semester_assessment_weights SET nh=100,asts=0,asas=0 WHERE semester_id=$1`, x.f.Semester); err != nil {
		t.Fatal(err)
	}
	in := HistoricalAssessmentScoreInput{
		ActivityID: x.activity22, Reason: "Bukti Sumatif sintetis diperiksa", Verification: "Nilai dicocokkan per siswa dan TP", Acknowledged: true,
		Entries: []HistoricalAssessmentScoreEntry{
			{StudentID: x.f.StudentA, ObjectiveID: x.routineTP, Value: "0", SourceKind: "arsip_manual", SourceIdentity: "Arsip CP10-23 NH positif", SourceLocator: "halaman 3, siswa A, TP rutin", SourceScore: "0", SourceResult: "0 untuk TP rutin", Verification: "identitas dan hasil cocok"},
			{StudentID: x.f.StudentA, ObjectiveID: x.historicalTP, Value: "85", SourceKind: "arsip_manual", SourceIdentity: "Arsip CP10-23 NH positif", SourceLocator: "halaman 3, siswa A, TP historis", SourceScore: "85", SourceResult: "85 untuk TP historis", Verification: "identitas dan hasil cocok"},
		},
	}
	p, err := s.PreviewHistoricalAssessmentScores(ctx, x.f.Teacher, in, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	var student *HistoricalAssessmentScoreStudent
	for i := range p.Students {
		if p.Students[i].ID == x.f.StudentA {
			student = &p.Students[i]
		}
	}
	if student == nil || p.Weights.NH != 100 || student.AfterStatus != "Belum Lengkap" || student.AfterRecommendation != "" {
		t.Fatalf("NH positif harus tetap belum lengkap saat bukti TP lain kosong: preview=%+v student=%+v", p, student)
	}
	var scores, audits int
	if err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1)`, x.activity22).Scan(&scores, &audits); err != nil || scores != 0 || audits != 0 {
		t.Fatalf("pratinjau menulis nilai/audit: scores=%d audits=%d err=%v", scores, audits, err)
	}
}

func TestHistoricalAssessmentScoreConcurrentServiceReplayIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	x := newHistoricalScoreDBFixture(t, db, "cp1023svc-race")
	ctx := context.Background()
	s := &Store{DB: db}
	in := HistoricalAssessmentScoreInput{ActivityID: x.activity22, Reason: "Arsip Sumatif diperiksa", Verification: "Identitas siswa/TP diverifikasi", Acknowledged: true,
		Entries: []HistoricalAssessmentScoreEntry{{StudentID: x.f.StudentB, ObjectiveID: x.routineTP, Value: "75", SourceKind: "arsip_manual", SourceIdentity: "Arsip CP10-23 race", SourceLocator: "halaman 4, siswa B, TP rutin", SourceScore: "75", SourceResult: "75 pada kolom TP rutin", Verification: "nama dan hasil cocok"}}}
	p, err := s.PreviewHistoricalAssessmentScores(ctx, x.f.Teacher, in, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	req := HistoricalAssessmentScoreConfirmation{Input: in, SnapshotToken: p.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002331"}
	start := make(chan struct{})
	var wg sync.WaitGroup
	ids := make([]int64, 2)
	errs := make([]error, 2)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ids[i], errs[i] = s.ConfirmHistoricalAssessmentScores(ctx, x.f.Teacher, req)
		}(i)
	}
	close(start)
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || ids[0] != ids[1] {
		t.Fatalf("replay bersamaan tidak idempoten: ids=%v errors=%v", ids, errs)
	}
	var scores, batches, details int
	if err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_score_batches WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1)`, x.activity22).Scan(&scores, &batches, &details); err != nil || scores != 1 || batches != 1 || details != 1 {
		t.Fatalf("replay serentak menggandakan data: score=%d batch=%d detail=%d err=%v", scores, batches, details, err)
	}
}

func TestHistoricalAssessmentScoreProjectionLeavesUnselectedBelumDinilaiUntouchedIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	x := newHistoricalScoreDBFixture(t, db, "cp1023-unselected")
	ctx := context.Background()
	s := &Store{DB: db}
	if _, err := db.ExecContext(ctx, `UPDATE app.semester_assessment_weights SET nh=100,asts=0,asas=0 WHERE semester_id=$1`, x.f.Semester); err != nil {
		t.Fatal(err)
	}
	input := HistoricalAssessmentScoreInput{ActivityID: x.activity22, Reason: "Satu nilai terpilih", Verification: "Sumber manual dicocokkan", Acknowledged: true,
		Entries: []HistoricalAssessmentScoreEntry{{StudentID: x.f.StudentA, ObjectiveID: x.routineTP, Value: "0", SourceKind: "arsip_manual", SourceIdentity: "Arsip proyeksi terpilih", SourceLocator: "halaman 2, siswa A, TP rutin", SourceScore: "0", SourceResult: "0 pada sel TP", Verification: "identitas dan angka dicocokkan"}}}
	preview, err := s.PreviewHistoricalAssessmentScores(ctx, x.f.Teacher, input, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	var unselected *HistoricalAssessmentScoreStudent
	for i := range preview.Students {
		if preview.Students[i].ID == x.f.StudentB {
			unselected = &preview.Students[i]
		}
	}
	if unselected == nil || unselected.BeforeStatus != "Belum Dinilai" {
		t.Fatalf("fixture siswa tidak terpilih harus semula Belum Dinilai: %+v", unselected)
	}
	if unselected.AfterNH != unselected.BeforeNH || unselected.AfterStatus != unselected.BeforeStatus || unselected.AfterRecommendation != unselected.BeforeRecommendation || unselected.AfterNeedsReview != unselected.BeforeNeedsReview {
		t.Fatalf("proyeksi mengubah siswa yang tidak dipilih: %+v", unselected)
	}
	var scoreCount, auditCount int
	if err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1)`, x.activity22).Scan(&scoreCount, &auditCount); err != nil || scoreCount != 0 || auditCount != 0 {
		t.Fatalf("pratinjau membuat mutasi: scores=%d audits=%d err=%v", scoreCount, auditCount, err)
	}
}

func TestHistoricalAssessmentScoreSourceLocatorUniqueAcrossBatchesIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	x := newHistoricalScoreDBFixture(t, db, "cp23dupsrc")
	ctx := context.Background()
	s := &Store{DB: db}
	first := HistoricalAssessmentScoreInput{ActivityID: x.activity22, Reason: "Pencatatan nilai A", Verification: "Sumber dicocokkan", Acknowledged: true,
		Entries: []HistoricalAssessmentScoreEntry{{StudentID: x.f.StudentA, ObjectiveID: x.routineTP, Value: "82,50", SourceKind: "arsip_manual", SourceIdentity: "Arsip Nilai CP23", SourceLocator: "halaman 4, siswa A, TP rutin", SourceScore: "82.50", SourceResult: "82,50 pada sel", Verification: "identitas dicocokkan"}}}
	p1, err := s.PreviewHistoricalAssessmentScores(ctx, x.f.Teacher, first, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfirmHistoricalAssessmentScores(ctx, x.f.Teacher, HistoricalAssessmentScoreConfirmation{Input: first, SnapshotToken: p1.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002341"})
	if err != nil {
		t.Fatalf("batch pertama: %v", err)
	}
	second := HistoricalAssessmentScoreInput{ActivityID: x.activity22, Reason: "Pencatatan nilai B", Verification: "Sumber dicocokkan", Acknowledged: true,
		Entries: []HistoricalAssessmentScoreEntry{{StudentID: x.f.StudentB, ObjectiveID: x.routineTP, Value: "75", SourceKind: "arsip_manual", SourceIdentity: "  arsip   nilai cp23 ", SourceLocator: " HALAMAN 4, siswa A, TP rutin ", SourceScore: "75", SourceResult: "75 pada sel", Verification: "identitas dicocokkan"}}}
	if _, err = s.PreviewHistoricalAssessmentScores(ctx, x.f.Teacher, second, 1, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("lokator sumber ternormalisasi dipakai ulang lintas batch: %v", err)
	}
	var batches, scores, audits int
	if err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM app.historical_assessment_score_batches WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1)`, x.activity22).Scan(&batches, &scores, &audits); err != nil || batches != 1 || scores != 1 || audits != 1 {
		t.Fatalf("penolakan duplikasi meninggalkan batch parsial: batches=%d scores=%d audits=%d err=%v", batches, scores, audits, err)
	}
}

func TestHistoricalAssessmentScoreConcurrentSourceLocatorReuseRejectedIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	x := newHistoricalScoreDBFixture(t, db, "cp23racesrc")
	ctx := context.Background()
	s := &Store{DB: db}
	makeInput := func(student int64, locator string) HistoricalAssessmentScoreInput {
		return HistoricalAssessmentScoreInput{ActivityID: x.activity22, Reason: "Pencatatan bukti", Verification: "Sumber dicocokkan", Acknowledged: true,
			Entries: []HistoricalAssessmentScoreEntry{{StudentID: student, ObjectiveID: x.routineTP, Value: "75", SourceKind: "arsip_manual", SourceIdentity: "Arsip bersama", SourceLocator: locator, SourceScore: "75", SourceResult: "75 pada kolom TP", Verification: "nama siswa diverifikasi"}}}
	}
	inputs := []HistoricalAssessmentScoreInput{makeInput(x.f.StudentA, "halaman 5, siswa A, TP rutin"), makeInput(x.f.StudentB, " HALAMAN 5,   SISWA A, TP RUTIN ")}
	requests := make([]HistoricalAssessmentScoreConfirmation, 2)
	for i, input := range inputs {
		p, err := s.PreviewHistoricalAssessmentScores(ctx, x.f.Teacher, input, 1, "")
		if err != nil {
			t.Fatal(err)
		}
		requests[i] = HistoricalAssessmentScoreConfirmation{Input: input, SnapshotToken: p.SnapshotToken, SubmissionKey: []string{"00000000-0000-4000-8000-000000002342", "00000000-0000-4000-8000-000000002343"}[i]}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range requests {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = s.ConfirmHistoricalAssessmentScores(ctx, x.f.Teacher, requests[i])
		}(i)
	}
	close(start)
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatalf("konfirmasi bersamaan gagal dengan error tak terduga: %v", err)
		}
	}
	var batches, scores, audits int
	if err := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM app.historical_assessment_score_batches WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1)`, x.activity22).Scan(&batches, &scores, &audits); err != nil || successes != 1 || batches != 1 || scores != 1 || audits != 1 {
		t.Fatalf("replay sumber serentak tidak atomik: success=%d batches=%d scores=%d audits=%d err=%v", successes, batches, scores, audits, err)
	}
}

var _ = errors.Is
