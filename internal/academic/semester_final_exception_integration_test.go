//go:build integration

package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestSemesterFinalExceptionEvidenceSnapshotAndReplayIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "except"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, e := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "0"}, {StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "80"}}); e != nil {
		t.Fatal(e)
	}
	// Nilai kelas asal yang belum diakui tidak menjadi bukti pada kelas tujuan.
	if e := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.MovedStudent, Component: "asts", Date: "2020-01-10", Value: "80"}}); e != nil {
		t.Fatal(e)
	}
	moved, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, f.MovedStudent)
	if e != nil || moved.CanFinalizeException || moved.Student.Status != "Belum Dinilai" {
		t.Fatalf("nilai kelas asal tidak diakui: %+v %v", moved, e)
	}
	for _, id := range []int64{f.StudentA, f.StudentB} {
		p, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, id)
		if e != nil || !p.CanFinalizeException || len(p.Student.ExceptionEvidence) != 1 || p.Student.Recommendation != "" {
			t.Fatalf("pratinjau pengecualian %+v %v", p, e)
		}
	}
	p, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	key := "00000000-0000-4000-8000-000000000501"
	for _, v := range []string{"", " ", strings.Repeat("a", 501)} {
		if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 0, v, p.SnapshotToken, key); e == nil {
			t.Fatal("alasan tidak sah diterima")
		}
	}
	if e = s.FinalizeSemesterException(ctx, f.Other, f.Semester, f.Class, f.Subject, f.StudentA, 0, "Alasan", p.SnapshotToken, key); !errors.Is(e, ErrForbidden) {
		t.Fatalf("akses lintas guru: %v", e)
	}
	if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, f.StudentA, 0, "Alasan", p.SnapshotToken, key); !errors.Is(e, ErrForbidden) {
		t.Fatalf("siswa di luar konteks: %v", e)
	}
	var source int64
	if e = db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE semester_id=$1 AND student_id=$2 AND component='asts'`, f.Semester, f.StudentA).Scan(&source); e != nil {
		t.Fatal(e)
	}
	if e = s.CorrectExamInitialScore(ctx, f.Teacher, source, 0, "10", "Koreksi uji"); e != nil {
		t.Fatal(e)
	}
	if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 0, "Alasan", p.SnapshotToken, key); !errors.Is(e, ErrConflict) {
		t.Fatalf("snapshot lama: %v", e)
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); e != nil || count != 0 {
		t.Fatalf("Final parsial: %d %v", count, e)
	}
	p, e = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 0, "Alasan uji", p.SnapshotToken, key)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for e = range results {
		if e != nil {
			t.Fatalf("replay serentak: %v", e)
		}
	}
	if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 100, "Alasan uji", p.SnapshotToken, key); !errors.Is(e, ErrConflict) {
		t.Fatalf("key dengan payload lain: %v", e)
	}
	var typ string
	var recommendation sql.NullInt64
	var score int
	var reason string
	if e = db.QueryRow(`SELECT final_type,recommendation_score,final_score,reason FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&typ, &recommendation, &score, &reason); e != nil || typ != "exception" || recommendation.Valid || score != 0 || reason != "Alasan uji" {
		t.Fatalf("Final tersimpan %s %+v %d %q %v", typ, recommendation, score, reason, e)
	}
	p, e = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil || !p.Finalized || p.FinalType != "exception" || p.NeedsReview {
		t.Fatalf("pembacaan Final %+v %v", p, e)
	}
	q, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 100, "Alasan valid", q.SnapshotToken, "00000000-0000-4000-8000-000000000502"); e != nil {
		t.Fatal(e)
	}
	if e = s.CorrectExamInitialScore(ctx, f.Teacher, source, 1, "20", "Koreksi setelah Final"); e != nil {
		t.Fatal(e)
	}
	p, e = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil || !p.NeedsReview || p.FinalType != "exception" || p.FinalValue != "0" || p.HistoricalStudent.ASTS != "10,00" {
		t.Fatalf("histori dan Perlu Ditinjau: %+v %v", p, e)
	}
	if _, e = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, f.Teacher); e != nil {
		t.Fatal(e)
	}
	if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 0, "Alasan uji", p.SnapshotToken, key); !errors.Is(e, ErrForbidden) {
		t.Fatalf("akun nonaktif: %v", e)
	}
	if _, e = db.Exec(`UPDATE app.users SET active=true WHERE id=$1`, f.Teacher); e != nil {
		t.Fatal(e)
	}
	// Replay tetap memerlukan penugasan yang berlaku.
	if _, e = db.Exec(`UPDATE app.teaching_assignments SET effective_until='2020-02-01' WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND teacher_id=$4`, f.Semester, f.Class, f.Subject, f.TeacherID); e != nil {
		t.Fatal(e)
	}
	if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 0, "Alasan uji", p.SnapshotToken, key); !errors.Is(e, ErrForbidden) {
		t.Fatalf("replay tanpa penugasan: %v", e)
	}
}

func TestSemesterFinalExceptionPartialNHAndInvalidEvidenceIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "exceptionnh"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, e := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 100}, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP NH belum lengkap"); e != nil {
		t.Fatal(e)
	}
	var objective int64
	if e := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&objective); e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct{ name, kind string }{{"Sumatif satu", "sumatif"}, {"Sumatif kosong", "sumatif"}, {"Formatif saja", "formatif"}} {
		if e := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, v.name, v.kind, "2020-01-10", []int64{objective}); e != nil {
			t.Fatal(e)
		}
	}
	var sum, formatif int64
	if e := db.QueryRow(`SELECT min(id) FILTER(WHERE name='Sumatif satu'),min(id) FILTER(WHERE name='Formatif saja') FROM app.assessment_activities WHERE class_id=$1`, f.Class).Scan(&sum, &formatif); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, sum, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, Value: "0"}}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, formatif, []AssessmentScoreInput{{StudentID: f.StudentB, ObjectiveID: objective, Value: "100"}}); e != nil {
		t.Fatal(e)
	}
	formatifOnly, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB)
	if e != nil || formatifOnly.CanFinalizeException || formatifOnly.Student.Status != "Belum Dinilai" {
		t.Fatalf("formatif saja: %+v %v", formatifOnly, e)
	}
	if e := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "100"}}); e != nil {
		t.Fatal(e)
	}
	a, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil || !a.CanFinalizeException || a.Student.NH != "" || a.Student.Recommendation != "" {
		t.Fatalf("NH sebagian %+v %v", a, e)
	}
	b, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB)
	if e != nil || b.CanFinalizeException || b.Student.Status != "Belum Dinilai" {
		t.Fatalf("formatif/bobot nol %+v %v", b, e)
	}
	if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 80, "Alasan", b.SnapshotToken, "00000000-0000-4000-8000-000000000503"); e == nil {
		t.Fatal("formatif dan komponen bobot nol diterima")
	}
	// Ketiga jalur memakai kunci mutasi yang sama; hanya pengecualian sah.
	start := make(chan struct{})
	results := make(chan error, 3)
	var wg sync.WaitGroup
	for _, action := range []func() error{
		func() error {
			return s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "Alasan sah", a.SnapshotToken, "00000000-0000-4000-8000-000000000504")
		},
		func() error {
			return s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", a.SnapshotToken, "00000000-0000-4000-8000-000000000505")
		},
		func() error {
			return s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []SemesterFinalBatchItem{{StudentID: f.StudentA, SnapshotToken: a.SnapshotToken}}, "00000000-0000-4000-8000-000000000506")
		},
	} {
		wg.Add(1)
		go func(fn func() error) { defer wg.Done(); <-start; results <- fn() }(action)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for result := range results {
		if result == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("Final konkuren berhasil=%d", success)
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 AND final_type='exception'`, f.Semester, f.StudentA).Scan(&count); e != nil || count != 1 {
		t.Fatalf("Final ganda atau salah jenis: %d %v", count, e)
	}
}
