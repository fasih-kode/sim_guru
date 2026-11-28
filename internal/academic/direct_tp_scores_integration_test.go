//go:build integration

package academic

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDirectTPScoreFoundationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "direct"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP pertama"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP kedua"); err != nil {
		t.Fatal(err)
	}
	var objective int64
	var second int64
	var ordinal int
	if err := db.QueryRow(`SELECT o.id,c.ordinal FROM app.learning_objectives o JOIN app.direct_tp_codes c ON c.objective_id=o.id WHERE (o.semester_id,o.class_id,o.subject_id)=($1,$2,$3) ORDER BY c.ordinal LIMIT 1`, f.Semester, f.Class, f.Subject).Scan(&objective, &ordinal); err != nil || ordinal != 1 {
		t.Fatalf("kode TP: %d %v", ordinal, err)
	}
	if err := db.QueryRow(`SELECT objective_id FROM app.direct_tp_codes WHERE (semester_id,class_id,subject_id,ordinal)=($1,$2,$3,2)`, f.Semester, f.Class, f.Subject).Scan(&second); err != nil {
		t.Fatal(err)
	}
	copied, err := s.CopyLearningObjective(ctx, f.Teacher, objective, f.Semester, f.Class, f.Subject, f.Semester, f.OtherClass, f.Subject)
	if err != nil {
		t.Fatalf("salin TP: %v", err)
	}
	if err = db.QueryRow(`SELECT ordinal FROM app.direct_tp_codes WHERE objective_id=$1`, copied).Scan(&ordinal); err != nil || ordinal != 1 {
		t.Fatalf("kode TP salinan: %d %v", ordinal, err)
	}
	if _, err = db.Exec(`UPDATE app.learning_objectives SET definition='TP pertama diperbaiki' WHERE id=$1`, objective); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT ordinal FROM app.direct_tp_codes WHERE objective_id=$1`, objective).Scan(&ordinal); err != nil || ordinal != 1 {
		t.Fatalf("kode setelah koreksi: %d %v", ordinal, err)
	}
	key := "61000000-0000-4000-8000-000000000001"
	input := []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, ExpectedVersion: -1, Value: "0"}, {StudentID: f.StudentB, ObjectiveID: objective, ExpectedVersion: -1, Value: "82,50"}}
	if err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, key, input); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, key, input); err != nil {
		t.Fatalf("replay: %v", err)
	}
	altered := append([]DirectTPScoreInput(nil), input...)
	altered[0].Value = "1"
	if err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, key, altered); !errors.Is(err, ErrConflict) {
		t.Fatalf("konflik replay: %v", err)
	}
	change := []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, ExpectedVersion: 0, Value: "100"}}
	if err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000002", change); err != nil {
		t.Fatal(err)
	}
	var original, updated string
	var version int
	if err := db.QueryRow(`SELECT s.score::text,c.after_score::text,c.version FROM app.direct_tp_initial_scores s JOIN app.direct_tp_score_corrections c ON c.initial_score_id=s.id WHERE s.student_id=$1`, f.StudentA).Scan(&original, &updated, &version); err != nil || original != "0.00" || updated != "100.00" || version != 1 {
		t.Fatalf("nilai/koreksi: %s %s %d %v", original, updated, version, err)
	}
	if err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000003", change); !errors.Is(err, ErrConflict) {
		t.Fatalf("versi basi: %v", err)
	}
	noOp := []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, ExpectedVersion: 1, Value: "100,00"}}
	if err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000005", noOp); err != nil {
		t.Fatalf("no-op: %v", err)
	}
	if err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000005", noOp); err != nil {
		t.Fatalf("replay no-op: %v", err)
	}
	values, err := s.DirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil || len(values) != 2 {
		t.Fatalf("baca nilai: %v %v", values, err)
	}
	if err := s.SaveDirectTPScores(ctx, f.Other, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000004", input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru asing: %v", err)
	}
	for _, invalid := range []string{"-1", "100,01", "1,001", "abc"} {
		err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000010", []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: second, ExpectedVersion: -1, Value: invalid}})
		if err == nil {
			t.Fatalf("nilai tidak valid diterima: %q", invalid)
		}
	}
	err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000011", []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: second, ExpectedVersion: -1, Value: ""}})
	if err == nil {
		t.Fatal("batch sel kosong saja diterima")
	}
	err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000012", []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: second, ExpectedVersion: -1, Value: "1"}, {StudentID: f.MovedStudent, ObjectiveID: second, ExpectedVersion: -1, Value: "2"}})
	if err == nil {
		t.Fatal("batch dengan penempatan pindah diterima")
	}
	err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000013", []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: copied, ExpectedVersion: -1, Value: "50"}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("TP konteks lain: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET effective_until='2020-02-01' WHERE (semester_id,class_id,subject_id,teacher_id)=($1,$2,$3,$4)`, f.Semester, f.Class, f.Subject, f.TeacherID); err != nil {
		t.Fatal(err)
	}
	err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000014", []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: second, ExpectedVersion: -1, Value: "50"}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("penugasan berakhir: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET effective_until=NULL WHERE (semester_id,class_id,subject_id,teacher_id)=($1,$2,$3,$4)`, f.Semester, f.Class, f.Subject, f.TeacherID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, f.Teacher); err != nil {
		t.Fatal(err)
	}
	err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000015", []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: second, ExpectedVersion: -1, Value: "50"}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("akun tidak aktif: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.users SET active=true WHERE id=$1`, f.Teacher); err != nil {
		t.Fatal(err)
	}
	var year, placeA, placeB int64
	if err = db.QueryRow(`SELECT sm.school_year_id,pa.id,pb.id FROM app.semesters sm JOIN app.student_placements pa ON pa.school_year_id=sm.school_year_id AND pa.student_id=$2 JOIN app.student_placements pb ON pb.school_year_id=sm.school_year_id AND pb.student_id=$3 WHERE sm.id=$1`, f.Semester, f.StudentA, f.StudentB).Scan(&year, &placeA, &placeB); err != nil {
		t.Fatal(err)
	}
	var wrongYear int64
	if err = db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "fk-final-"+testutil.Random(t)[:8]).Scan(&wrongYear); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',decode(repeat('ab',32),'hex'),gen_random_uuid(),decode(repeat('cd',32),'hex'))`, wrongYear, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher)
	var finalFK *pgconn.PgError
	if !errors.As(err, &finalFK) || finalFK.Code != "23503" {
		t.Fatalf("konteks tahun Final: %v", err)
	}
	for _, tc := range []struct{ objective, placement int64 }{{copied, placeA}, {second, placeB}} {
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		var bid int64
		e = tx.QueryRow(`INSERT INTO app.direct_tp_submissions(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count) VALUES($1,$2,$3,$4,$5,$6,gen_random_uuid(),decode(repeat('ab',32),'hex'),1) RETURNING id`, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher).Scan(&bid)
		if e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
		_, e = tx.Exec(`INSERT INTO app.direct_tp_initial_scores(school_year_id,semester_id,class_id,subject_id,objective_id,student_id,placement_id,owner_teacher_id,recorded_by,score,submission_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,50,$10)`, year, f.Semester, f.Class, f.Subject, tc.objective, f.StudentA, tc.placement, f.TeacherID, f.Teacher, bid)
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || pg.Code != "23503" {
			tx.Rollback()
			t.Fatalf("FK konteks atau penempatan: %v", e)
		}
		if e = tx.Rollback(); e != nil {
			t.Fatal(e)
		}
	}
	txAudit, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = txAudit.Exec(`INSERT INTO app.direct_tp_submissions(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count) VALUES($1,$2,$3,$4,$5,$6,gen_random_uuid(),decode(repeat('ab',32),'hex'),1)`, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher)
	if err != nil {
		txAudit.Rollback()
		t.Fatal(err)
	}
	err = txAudit.Commit()
	var auditErr *pgconn.PgError
	if !errors.As(err, &auditErr) || auditErr.Code != "23514" {
		t.Fatalf("audit tanpa nilai dapat commit: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.direct_tp_initial_scores SET id=id WHERE semester_id=$1`, f.Semester); err == nil {
		t.Fatal("nilai awal dapat diubah")
	}
	if _, err = db.Exec(`DELETE FROM app.direct_tp_score_corrections WHERE initial_score_id IN (SELECT id FROM app.direct_tp_initial_scores WHERE semester_id=$1)`, f.Semester); err == nil {
		t.Fatal("histori koreksi dapat dihapus")
	}
	var initial, corrections, batches int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM app.direct_tp_initial_scores WHERE semester_id=$1),(SELECT count(*) FROM app.direct_tp_score_corrections c JOIN app.direct_tp_initial_scores s ON s.id=c.initial_score_id WHERE s.semester_id=$1),(SELECT count(*) FROM app.direct_tp_submissions WHERE semester_id=$1)`, f.Semester).Scan(&initial, &corrections, &batches); err != nil || initial != 2 || corrections != 1 || batches != 3 {
		t.Fatalf("jumlah %d %d %d: %v", initial, corrections, batches, err)
	}
	// Final siswa B mengunci nilai B, tetapi tidak mengunci sel siswa A.
	_, err = db.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256)
	 SELECT school_year_id,$1,$2,$3,$4,$5,$6,1,80,80,'{}'::jsonb,decode(repeat('ab',32),'hex'),'61000000-0000-4000-8000-000000000006',decode(repeat('cd',32),'hex') FROM app.semesters WHERE id=$1`, f.Semester, f.Class, f.Subject, f.StudentB, f.TeacherID, f.Teacher)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000007", []DirectTPScoreInput{{StudentID: f.StudentB, ObjectiveID: second, ExpectedVersion: -1, Value: "50"}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("sel kosong setelah Final: %v", err)
	}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000008", []DirectTPScoreInput{{StudentID: f.StudentB, ObjectiveID: objective, ExpectedVersion: 0, Value: "50"}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("koreksi setelah Final: %v", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var bid, sourceID int64
	if err = tx.QueryRow(`SELECT id FROM app.direct_tp_initial_scores WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentB).Scan(&sourceID); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.QueryRow(`INSERT INTO app.direct_tp_submissions(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count) VALUES($1,$2,$3,$4,$5,$6,gen_random_uuid(),decode(repeat('ab',32),'hex'),1) RETURNING id`, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher).Scan(&bid); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	_, err = tx.Exec(`INSERT INTO app.direct_tp_score_corrections(initial_score_id,version,before_score,after_score,corrected_by,submission_id) VALUES($1,1,82.50,50,$2,$3)`, sourceID, f.Teacher, bid)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23514" {
		tx.Rollback()
		t.Fatalf("SQL koreksi setelah Final: %v", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "61000000-0000-4000-8000-000000000009", []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: second, ExpectedVersion: -1, Value: "50"}}); err != nil {
		t.Fatalf("siswa lain belum Final: %v", err)
	}
}

func TestDirectTPConcurrentFinalGuardIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "directrace"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP untuk uji konkurensi"); err != nil {
		t.Fatal(err)
	}
	var objective, year, placement int64
	if err := db.QueryRow(`SELECT o.id,o.school_year_id,p.id FROM app.learning_objectives o JOIN app.student_placements p ON p.student_id=$4 AND p.school_year_id=o.school_year_id WHERE (o.semester_id,o.class_id,o.subject_id)=($1,$2,$3)`, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&objective, &year, &placement); err != nil {
		t.Fatal(err)
	}
	finalTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer finalTx.Rollback()
	_, err = finalTx.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',decode(repeat('ab',32),'hex'),'62000000-0000-4000-8000-000000000001',decode(repeat('cd',32),'hex'))`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher)
	if err != nil {
		t.Fatal(err)
	}
	scoreTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer scoreTx.Rollback()
	var batch int64
	err = scoreTx.QueryRow(`INSERT INTO app.direct_tp_submissions(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count) VALUES($1,$2,$3,$4,$5,$6,'62000000-0000-4000-8000-000000000002',decode(repeat('cd',32),'hex'),1) RETURNING id`, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher).Scan(&batch)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, e := scoreTx.Exec(`INSERT INTO app.direct_tp_initial_scores(school_year_id,semester_id,class_id,subject_id,objective_id,student_id,placement_id,owner_teacher_id,recorded_by,score,submission_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,50,$10)`, year, f.Semester, f.Class, f.Subject, objective, f.StudentA, placement, f.TeacherID, f.Teacher, batch)
		result <- e
	}()
	select {
	case e := <-result:
		t.Fatalf("nilai tidak menunggu transaksi Final: %v", e)
	case <-time.After(200 * time.Millisecond):
	}
	if err = finalTx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("nilai tetap tertahan setelah Final commit")
	}
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23514" {
		t.Fatalf("nilai setelah Final: %v", err)
	}
	if err = scoreTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.direct_tp_initial_scores WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 0 {
		t.Fatalf("nilai parsial: %d %v", count, err)
	}
}

func TestDirectTPStaleSnapshotGuardIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "directstale"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP snapshot lama"); err != nil {
		t.Fatal(err)
	}
	var objective, year, placement int64
	if err := db.QueryRow(`SELECT o.id,o.school_year_id,p.id FROM app.learning_objectives o JOIN app.student_placements p ON p.student_id=$4 AND p.school_year_id=o.school_year_id WHERE (o.semester_id,o.class_id,o.subject_id)=($1,$2,$3)`, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&objective, &year, &placement); err != nil {
		t.Fatal(err)
	}
	scoreTx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer scoreTx.Rollback()
	var count int
	if err = scoreTx.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 0 {
		t.Fatalf("snapshot awal: %d %v", count, err)
	}
	_, err = db.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',decode(repeat('ab',32),'hex'),'63000000-0000-4000-8000-000000000001',decode(repeat('cd',32),'hex'))`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher)
	if err != nil {
		t.Fatal(err)
	}
	var batch int64
	err = scoreTx.QueryRow(`INSERT INTO app.direct_tp_submissions(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count) VALUES($1,$2,$3,$4,$5,$6,'63000000-0000-4000-8000-000000000002',decode(repeat('cd',32),'hex'),1) RETURNING id`, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher).Scan(&batch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = scoreTx.Exec(`INSERT INTO app.direct_tp_initial_scores(school_year_id,semester_id,class_id,subject_id,objective_id,student_id,placement_id,owner_teacher_id,recorded_by,score,submission_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,50,$10)`, year, f.Semester, f.Class, f.Subject, objective, f.StudentA, placement, f.TeacherID, f.Teacher, batch)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "40001" {
		t.Fatalf("snapshot basi: %v", err)
	}
	if err = scoreTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.direct_tp_initial_scores WHERE semester_id=$1)+(SELECT count(*) FROM app.direct_tp_submissions WHERE semester_id=$1)`, f.Semester).Scan(&count); err != nil || count != 0 {
		t.Fatalf("sisa rollback: %d %v", count, err)
	}
}

func TestDirectTPScoreBeforeConcurrentFinalIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "directfirst"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP nilai dahulu"); err != nil {
		t.Fatal(err)
	}
	var objective, year, placement int64
	if err := db.QueryRow(`SELECT o.id,o.school_year_id,p.id FROM app.learning_objectives o JOIN app.student_placements p ON p.student_id=$4 AND p.school_year_id=o.school_year_id WHERE (o.semester_id,o.class_id,o.subject_id)=($1,$2,$3)`, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&objective, &year, &placement); err != nil {
		t.Fatal(err)
	}
	scoreTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer scoreTx.Rollback()
	var batch int64
	err = scoreTx.QueryRow(`INSERT INTO app.direct_tp_submissions(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count) VALUES($1,$2,$3,$4,$5,$6,'64000000-0000-4000-8000-000000000001',decode(repeat('cd',32),'hex'),1) RETURNING id`, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher).Scan(&batch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = scoreTx.Exec(`INSERT INTO app.direct_tp_initial_scores(school_year_id,semester_id,class_id,subject_id,objective_id,student_id,placement_id,owner_teacher_id,recorded_by,score,submission_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,50,$10)`, year, f.Semester, f.Class, f.Subject, objective, f.StudentA, placement, f.TeacherID, f.Teacher, batch)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, e := db.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',decode(repeat('ab',32),'hex'),'64000000-0000-4000-8000-000000000002',decode(repeat('cd',32),'hex'))`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher)
		result <- e
	}()
	select {
	case e := <-result:
		t.Fatalf("Final tidak menunggu transaksi nilai: %v", e)
	case <-time.After(200 * time.Millisecond):
	}
	if err = scoreTx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Final tetap tertahan setelah nilai commit")
	}
	var scores, finals int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.direct_tp_initial_scores WHERE semester_id=$1),(SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1)`, f.Semester).Scan(&scores, &finals); err != nil || scores != 1 || finals != 1 {
		t.Fatalf("hasil serial: %d %d %v", scores, finals, err)
	}
}

func TestDirectTPDeadlockRollbackIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f := testutil.ExamFixtureFor(t, db, "directdeadlock"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP deadlock"); err != nil {
		t.Fatal(err)
	}
	var objective, year, placeA, placeB int64
	if err := db.QueryRow(`SELECT o.id,o.school_year_id,pa.id,pb.id FROM app.learning_objectives o JOIN app.student_placements pa ON pa.student_id=$4 AND pa.school_year_id=o.school_year_id JOIN app.student_placements pb ON pb.student_id=$5 AND pb.school_year_id=o.school_year_id WHERE (o.semester_id,o.class_id,o.subject_id)=($1,$2,$3)`, f.Semester, f.Class, f.Subject, f.StudentA, f.StudentB).Scan(&objective, &year, &placeA, &placeB); err != nil {
		t.Fatal(err)
	}
	txs := make([]*sql.Tx, 2)
	batches := make([]int64, 2)
	for i := range txs {
		var err error
		txs[i], err = db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer txs[i].Rollback()
		key := []string{"65000000-0000-4000-8000-000000000001", "65000000-0000-4000-8000-000000000002"}[i]
		err = txs[i].QueryRowContext(ctx, `INSERT INTO app.direct_tp_submissions(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count) VALUES($1,$2,$3,$4,$5,$6,$7,decode(repeat('cd',32),'hex'),2) RETURNING id`, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher, key).Scan(&batches[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	insert := func(tx *sql.Tx, batch, student, placement int64) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO app.direct_tp_initial_scores(school_year_id,semester_id,class_id,subject_id,objective_id,student_id,placement_id,owner_teacher_id,recorded_by,score,submission_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,50,$10)`, year, f.Semester, f.Class, f.Subject, objective, student, placement, f.TeacherID, f.Teacher, batch)
		return err
	}
	if err := insert(txs[0], batches[0], f.StudentA, placeA); err != nil {
		t.Fatal(err)
	}
	if err := insert(txs[1], batches[1], f.StudentB, placeB); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		i   int
		err error
	}
	results := make(chan outcome, 2)
	go func() { results <- outcome{0, insert(txs[0], batches[0], f.StudentB, placeB)} }()
	go func() { results <- outcome{1, insert(txs[1], batches[1], f.StudentA, placeA)} }()
	first := <-results
	var pg *pgconn.PgError
	if !errors.As(first.err, &pg) || pg.Code != "40P01" {
		t.Fatalf("tidak mendapat deadlock yang aman: %v", first.err)
	}
	if err := txs[first.i].Rollback(); err != nil {
		t.Fatal(err)
	}
	second := <-results
	if second.err != nil {
		t.Fatalf("transaksi yang bertahan: %v", second.err)
	}
	if err := txs[second.i].Commit(); err != nil {
		t.Fatal(err)
	}
	var scores, batchCount int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM app.direct_tp_initial_scores WHERE semester_id=$1),(SELECT count(*) FROM app.direct_tp_submissions WHERE semester_id=$1)`, f.Semester).Scan(&scores, &batchCount); err != nil || scores != 2 || batchCount != 1 {
		t.Fatalf("hasil deadlock: %d %d %v", scores, batchCount, err)
	}
	keys := []string{"65000000-0000-4000-8000-000000000001", "65000000-0000-4000-8000-000000000002"}
	err := s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, keys[first.i], []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, ExpectedVersion: -1, Value: "50"}, {StudentID: f.StudentB, ObjectiveID: objective, ExpectedVersion: -1, Value: "50"}})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("retry harus menolak versi yang telah berubah: %v", err)
	}
}

func TestDirectTPConcurrentReplayIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "directreplay"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP replay"); err != nil {
		t.Fatal(err)
	}
	var objective int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1`, f.Semester).Scan(&objective); err != nil {
		t.Fatal(err)
	}
	input := []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, ExpectedVersion: -1, Value: "0"}}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "66000000-0000-4000-8000-000000000001", input)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("replay bersamaan: %v", err)
		}
	}
	var scores, batches int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM app.direct_tp_initial_scores WHERE semester_id=$1),(SELECT count(*) FROM app.direct_tp_submissions WHERE semester_id=$1)`, f.Semester).Scan(&scores, &batches); err != nil || scores != 1 || batches != 1 {
		t.Fatalf("replay duplikat: %d %d %v", scores, batches, err)
	}
}
