//go:build integration

package academic

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestExamInitialScoresIntegrityAndAuthorizationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "academic")
	s := &Store{DB: db}
	ctx := context.Background()
	save := func(actor, class int64, in ...ExamInitialInput) error {
		return s.SaveExamInitialScores(ctx, actor, f.Semester, class, f.Subject, in)
	}
	asts := ExamInitialInput{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "82,50"}
	asas := ExamInitialInput{StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"}
	zero := ExamInitialInput{StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "0"}
	for _, invalid := range []ExamInitialInput{{f.StudentB, "asas", "2020-01-10", "-1"}, {f.StudentB, "asas", "2020-01-10", "100,01"}, {f.StudentB, "asas", "2020-01-10", "82,567"}, {f.StudentB, "asas", "2020-02-30", "50"}, {f.StudentB, "asas", "2019-12-31", "50"}, {f.StudentB, "asas", time.Now().In(time.FixedZone("WIB", 7*3600)).AddDate(0, 0, 1).Format("2006-01-02"), "50"}, {f.StudentB, "bad", "2020-01-10", "50"}, {999999999, "asts", "2020-01-10", "50"}} {
		if err := save(f.Teacher, f.Class, invalid); err == nil {
			t.Fatalf("input tidak sah diterima: %+v", invalid)
		}
	}
	if err := save(f.Other, f.Class, asts); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru lain: %v", err)
	}
	if err := save(f.Teacher, 999999999, asts); !errors.Is(err, ErrForbidden) {
		t.Fatalf("kelas lain: %v", err)
	}
	var wg sync.WaitGroup
	ch := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); ch <- save(f.Teacher, f.Class, asts) }()
	}
	wg.Wait()
	close(ch)
	for err := range ch {
		if err != nil {
			t.Fatalf("replay konkuren: %v", err)
		}
	}
	if err := save(f.Teacher, f.Class, asts, asas, zero); err != nil {
		t.Fatal(err)
	}
	// Kesalahan penempatan pada baris kedua harus menggulung insert baris pertama.
	batchInvalid := ExamInitialInput{StudentID: f.MovedStudent, Component: "asas", Date: "2019-12-31", Value: "30"}
	pending := ExamInitialInput{StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "55"}
	if err := save(f.Teacher, f.Class, pending, batchInvalid); err == nil {
		t.Fatal("batch dengan penempatan salah diterima")
	}
	var pendingCount int
	if err := db.QueryRow(`SELECT count(*) FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2 AND student_id=$3 AND component='asas'`, f.Semester, f.Class, f.StudentB).Scan(&pendingCount); err != nil || pendingCount != 0 {
		t.Fatalf("rollback batch gagal: %d %v", pendingCount, err)
	}
	differentDate := asts
	differentDate.Date = "2020-01-11"
	if err := save(f.Teacher, f.Class, differentDate); !errors.Is(err, ErrConflict) {
		t.Fatalf("tanggal berubah harus konflik: %v", err)
	}
	if err := save(f.Teacher, f.OtherClass, ExamInitialInput{StudentID: f.StudentB, Component: "asas", Date: "2020-02-10", Value: "44"}); err == nil {
		t.Fatal("siswa dari kelas lain diterima")
	}
	different := asts
	different.Value = "83"
	blank := ExamInitialInput{StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "55"}
	if err := save(f.Teacher, f.Class, different, blank); !errors.Is(err, ErrConflict) {
		t.Fatalf("konflik batch: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id IN ($4,$5)`, f.Semester, f.Class, f.Subject, f.StudentA, f.StudentB).Scan(&count); err != nil || count != 3 {
		t.Fatalf("atomik/replay: %d %v", count, err)
	}
	sheet, err := s.ExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "2020-01-10")
	if err != nil {
		t.Fatal(err)
	}
	foundA, foundB := false, false
	for _, p := range sheet.Students {
		if p.ID == f.StudentA {
			foundA = true
		}
		if p.ID == f.StudentB {
			foundB = true
		}
		if p.ID == f.StudentA && (!p.ASTS.Recorded || p.ASTS.Value != "82,50" || !p.ASAS.Recorded || p.ASAS.Value != "90,00" || p.ASTS.TeacherName == "" || p.ASTS.RecordedAt == "") {
			t.Fatalf("sumber: %+v", p)
		}
		if p.ID == f.StudentB && (!p.ASTS.Recorded || p.ASTS.Value != "0,00" || p.ASAS.Recorded) {
			t.Fatalf("nol/kosong: %+v", p)
		}
	}
	if !foundA || !foundB {
		t.Fatal("roster siswa tidak lengkap")
	}
	for _, x := range []struct {
		class int64
		date  string
	}{{f.Class, "2020-01-10"}, {f.OtherClass, "2020-02-10"}} {
		if err = save(f.Teacher, x.class, ExamInitialInput{StudentID: f.MovedStudent, Component: "asts", Date: x.date, Value: "70"}); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.exam_initial_scores WHERE semester_id=$1 AND subject_id=$2 AND student_id=$3 AND component='asts'`, f.Semester, f.Subject, f.MovedStudent).Scan(&count); err != nil || count != 2 {
		t.Fatalf("bukti lintas kelas: %d %v", count, err)
	}
	// Pemeriksaan langsung memastikan batas tidak hanya bergantung pada service.
	var pgErr *pgconn.PgError
	_, err = db.Exec(`INSERT INTO app.exam_initial_scores(school_year_id,semester_id,class_id,subject_id,student_id,placement_id,teacher_id,component,exam_date,score,recorded_by) SELECT school_year_id,semester_id,class_id,subject_id,student_id,placement_id,teacher_id,component,exam_date,score,recorded_by FROM app.exam_initial_scores WHERE semester_id=$1 AND student_id=$2 AND component='asts'`, f.Semester, f.StudentA)
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("unik database: %v", err)
	}
	_, err = db.Exec(`INSERT INTO app.exam_initial_scores(school_year_id,semester_id,class_id,subject_id,student_id,placement_id,teacher_id,component,exam_date,score,recorded_by) SELECT school_year_id,semester_id,class_id,subject_id,$3,placement_id,teacher_id,'asas',exam_date,score,recorded_by FROM app.exam_initial_scores WHERE semester_id=$1 AND student_id=$2 AND component='asts'`, f.Semester, f.StudentA, f.StudentB)
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("FK penempatan database: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.exam_initial_scores SET score=5 WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA); err == nil {
		t.Fatal("histori bisa ditimpa")
	}
	for _, change := range []struct {
		query, restore string
		id             int64
	}{{`UPDATE app.users SET active=false WHERE id=$1`, `UPDATE app.users SET active=true WHERE id=$1`, f.Teacher}, {`UPDATE app.teachers SET active=false WHERE id=$1`, `UPDATE app.teachers SET active=true WHERE id=$1`, f.TeacherID}, {`UPDATE app.teaching_assignments SET effective_from=current_date+1 WHERE semester_id=$1 AND class_id=$2`, `UPDATE app.teaching_assignments SET effective_from='2020-01-01' WHERE semester_id=$1 AND class_id=$2`, 0}, {`UPDATE app.teaching_assignments SET effective_until=current_date WHERE semester_id=$1 AND class_id=$2`, `UPDATE app.teaching_assignments SET effective_until=NULL WHERE semester_id=$1 AND class_id=$2`, 0}} {
		args := []any{change.id}
		if change.id == 0 {
			args = []any{f.Semester, f.Class}
		}
		if _, err = db.Exec(change.query, args...); err != nil {
			t.Fatal(err)
		}
		err = save(f.Teacher, f.Class, asts)
		_, readErr := s.ExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "2020-01-10")
		_, restoreErr := db.Exec(change.restore, args...)
		if restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if !errors.Is(err, ErrForbidden) || !errors.Is(readErr, ErrForbidden) {
			t.Fatalf("hak tidak aktif: tulis %v baca %v", err, readErr)
		}
	}
	// Guru pengampu baru dapat membaca bukti, tetapi tidak dapat menimpa milik guru lama.
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET teacher_id=$1 WHERE semester_id=$2 AND class_id=$3`, f.OtherID, f.Semester, f.Class); err != nil {
		t.Fatal(err)
	}
	_, readErr := s.ExamInitialScores(ctx, f.Other, f.Semester, f.Class, f.Subject, "2020-01-10")
	writeErr := save(f.Other, f.Class, asts)
	_, restoreErr := db.Exec(`UPDATE app.teaching_assignments SET teacher_id=$1 WHERE semester_id=$2 AND class_id=$3`, f.TeacherID, f.Semester, f.Class)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if readErr != nil || !errors.Is(writeErr, ErrConflict) {
		t.Fatalf("pengampu baru: baca %v tulis %v", readErr, writeErr)
	}
}
