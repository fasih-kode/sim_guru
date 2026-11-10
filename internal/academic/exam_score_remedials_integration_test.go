//go:build integration

package academic

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestExamRemedialsIntegrityIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "rem"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	ctx := context.Background()
	inputs := []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "65"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "80"}, {StudentID: f.MovedStudent, Component: "asts", Date: "2020-01-10", Value: "50"}}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, inputs); err != nil {
		t.Fatal(err)
	}
	var asts, asas, moved int64
	for _, x := range []struct {
		student   int64
		component string
		dest      *int64
	}{{f.StudentA, "asts", &asts}, {f.StudentA, "asas", &asas}, {f.MovedStudent, "asts", &moved}} {
		if err := db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE class_id=$1 AND student_id=$2 AND component=$3`, f.Class, x.student, x.component).Scan(x.dest); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CorrectExamInitialScore(ctx, f.Teacher, asts, 0, "70", "koreksi"); err != nil {
		t.Fatal(err)
	}
	key := func(i int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", i) }
	record := func(id int64, k int, v, date string) error {
		return s.RecordExamScoreRemedial(ctx, f.Teacher, id, key(k), v, date)
	}
	for _, bad := range []struct{ v, d string }{{"", "2020-01-10"}, {"-1", "2020-01-10"}, {"100,01", "2020-01-10"}, {"82,567", "2020-01-10"}, {"75", "2019-12-31"}, {"75", "2999-01-01"}, {"75", "2020-02-30"}} {
		if err := record(asts, 1, bad.v, bad.d); err == nil {
			t.Fatalf("input invalid diterima: %+v", bad)
		}
	}
	for i, v := range []string{"75", "85", "60"} {
		if err := record(asts, i+1, v, "2020-01-10"); err != nil {
			t.Fatal(err)
		}
	}
	if err := record(asts, 1, "75", "2020-01-10"); err != nil {
		t.Fatal(err)
	}
	if err := record(asts, 16, "75", "2020-01-10"); err != nil {
		t.Fatal("dua tindakan berbeda harus diterima:", err)
	}
	if err := record(999999999, 17, "75", "2020-01-10"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("sumber tidak sah: %v", err)
	}
	if !errors.Is(record(asts, 1, "76", "2020-01-10"), ErrConflict) {
		t.Fatal("konflik kunci tidak ditolak")
	}
	d, err := s.ExamScoreRemedialDetail(ctx, f.Teacher, asts)
	if err != nil || d.Effective != "85.00" || d.Score.InitialValue != "65.00" || d.Score.CorrectedValue != "70.00" || len(d.History) != 4 {
		t.Fatalf("detail: %+v %v", d, err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, asts, 1, "90", "koreksi lagi"); err != nil {
		t.Fatal(err)
	}
	d, err = s.ExamScoreRemedialDetail(ctx, f.Teacher, asts)
	if err != nil || d.Effective != "90.00" {
		t.Fatalf("setelah koreksi: %+v %v", d, err)
	}
	if err = record(asas, 4, "70", "2020-01-10"); err != nil {
		t.Fatal(err)
	}
	d, err = s.ExamScoreRemedialDetail(ctx, f.Teacher, asas)
	if err != nil || d.Effective != "80.00" {
		t.Fatalf("ASAS: %+v %v", d, err)
	}
	for i, v := range []string{"0", "100"} {
		if err = record(asas, 5+i, v, "2020-01-10"); err != nil {
			t.Fatal(err)
		}
	}
	if err = record(moved, 8, "75", "2020-02-02"); err == nil {
		t.Fatal("siswa pindah diterima")
	}
	if err = s.RecordExamScoreRemedial(ctx, f.Other, asts, key(9), "70", "2020-01-10"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru lain: %v", err)
	}
	if _, err = s.ExamScoreRemedialDetail(ctx, f.Other, asts); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lintas pengguna: %v", err)
	}
	if err = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, []ExamInitialInput{{StudentID: f.MovedStudent, Component: "asts", Date: "2020-02-10", Value: "40"}}); err != nil {
		t.Fatal(err)
	}
	var otherSource int64
	if err = db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE class_id=$1 AND student_id=$2 AND component='asts'`, f.OtherClass, f.MovedStudent).Scan(&otherSource); err != nil {
		t.Fatal(err)
	}
	if err = record(otherSource, 12, "55", "2020-02-10"); err != nil {
		t.Fatal(err)
	}
	old, err := s.ExamScoreRemedialDetail(ctx, f.Teacher, moved)
	if err != nil || old.Effective != "50.00" || len(old.History) != 0 {
		t.Fatalf("sumber lintas kelas berubah: %+v %v", old, err)
	}
	for _, table := range []string{"app.users", "app.teachers"} {
		var q string
		var id int64
		if table == "app.users" {
			q = "UPDATE app.users SET active=$2 WHERE id=$1"
			id = f.Teacher
		} else {
			q = "UPDATE app.teachers SET active=$2 WHERE id=$1"
			id = f.TeacherID
		}
		if _, err = db.Exec(q, id, false); err != nil {
			t.Fatal(err)
		}
		if err = record(asts, 13, "70", "2020-01-10"); !errors.Is(err, ErrForbidden) {
			t.Fatalf("identitas nonaktif %s: %v", table, err)
		}
		if _, err = db.Exec(q, id, true); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- record(asts, 10, "88", "2020-01-10") }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.exam_score_remedials WHERE initial_score_id=$1`, asts).Scan(&count); err != nil || count != 5 {
		t.Fatalf("duplikasi %d %v", count, err)
	}
	if _, err = db.Exec(`UPDATE app.exam_score_remedials SET score=1 WHERE initial_score_id=$1`, asts); err == nil {
		t.Fatal("update histori diterima")
	}
	if _, err = db.Exec(`DELETE FROM app.exam_score_remedials WHERE initial_score_id=$1`, asts); err == nil {
		t.Fatal("hapus histori diterima")
	}

	// Pembatalan transaksi tidak menyisakan bukti parsial.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO app.exam_score_remedials(initial_score_id,score,remedial_date,recorded_by,submission_key) VALUES($1,40,'2020-01-10',$2,$3::uuid)`, asts, f.Teacher, key(14)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.exam_score_remedials WHERE initial_score_id=$1`, asts).Scan(&count); err != nil || count != 5 {
		t.Fatalf("rollback parsial %d %v", count, err)
	}
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET effective_until=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date WHERE semester_id=$1 AND class_id=$2 AND teacher_id=$3`, f.Semester, f.Class, f.TeacherID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.teaching_assignments(semester_id,class_id,subject_id,teacher_id,effective_from,assigned_by) VALUES($1,$2,$3,$4,(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date,$5)`, f.Semester, f.Class, f.Subject, f.OtherID, f.Other); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordExamScoreRemedial(ctx, f.Other, asts, key(15), "70", "2020-01-10"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pengampu baru diterima: %v", err)
	}

	d, err = s.ExamScoreRemedialDetail(ctx, f.Teacher, asts)
	if err != nil || d.CanRecord {
		t.Fatalf("pemilik historis %+v %v", d, err)
	}
	if err = record(asts, 11, "70", "2020-01-10"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("penugasan berakhir: %v", err)
	}
}
