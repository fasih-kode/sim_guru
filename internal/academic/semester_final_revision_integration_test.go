//go:build integration

package academic

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestSemesterFinalRevisionHistoricalOwnerAndRetentionIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "rev"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "80"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000901"); err != nil {
		t.Fatal(err)
	}
	var original string
	if err = db.QueryRow(`SELECT to_jsonb(v)::text FROM app.semester_final_versions v WHERE semester_id=$1 AND student_id=$2 AND version=1`, f.Semester, f.StudentA).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET effective_until='2020-02-01' WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND teacher_id=$4`, f.Semester, f.Class, f.Subject, f.TeacherID); err != nil {
		t.Fatal(err)
	}
	if err = s.AssignTeacher(ctx, cur.ID, f.Semester, f.Class, f.Subject, f.OtherID, time.Now()); err != nil {
		t.Fatal(err)
	}
	p, err = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || p.FinalVersion != 1 {
		t.Fatalf("pemilik historis: %+v %v", p, err)
	}
	if _, err = s.SemesterFinalHistoricalPreview(ctx, f.Other, f.Semester, f.Class, f.Subject, f.StudentA); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru lain %v", err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Other, f.Semester, f.Class, f.Subject, f.StudentA, 1, 85, "Bukan pemilik", p.SnapshotToken, "00000000-0000-4000-8000-000000000908", "revision"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pengampu baru mengambil alih Final: %v", err)
	}
	if _, err = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB); !errors.Is(err, ErrForbidden) {
		t.Fatalf("siswa tanpa Final milik guru %v", err)
	}
	if _, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pemilik historis tidak berhak membuat Final lain %v", err)
	}
	key := "00000000-0000-4000-8000-000000000902"
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 85, "Perbaikan", p.SnapshotToken, key, "revision"); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 85, "Perbaikan", p.SnapshotToken, key, "revision"); err != nil {
		t.Fatalf("replay %v", err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 84, "Perbaikan", p.SnapshotToken, key, "revision"); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload berbeda %v", err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 85, "Lama", p.SnapshotToken, "00000000-0000-4000-8000-000000000903", "revision"); !errors.Is(err, ErrConflict) {
		t.Fatalf("versi basi %v", err)
	}
	p, err = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || p.FinalVersion != 2 || p.FinalValue != "85" || len(p.History) != 2 {
		t.Fatalf("versi dua %+v %v", p, err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 2, 85, "Konfirmasi", p.SnapshotToken, "00000000-0000-4000-8000-000000000904", "revision"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 100}, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 3, 0, "Basi", p.SnapshotToken, "00000000-0000-4000-8000-000000000905", "revision"); !errors.Is(err, ErrConflict) {
		t.Fatalf("snapshot basi %v", err)
	}
	p, err = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 3, 0, "Tidak cukup bukti", p.SnapshotToken, "00000000-0000-4000-8000-000000000906", "revision"); err == nil {
		t.Fatal("versi tanpa bukti diterima")
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 3, 85, "Pertahankan", p.SnapshotToken, "00000000-0000-4000-8000-000000000907", "retain"); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 3, 85, "Pertahankan", p.SnapshotToken, "00000000-0000-4000-8000-000000000907", "retain"); err != nil {
		t.Fatalf("replay mempertahankan: %v", err)
	}
	var retained, versions int
	var after string
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.semester_final_retentions WHERE final_version_id=(SELECT id FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 AND version=3)),(SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2),(SELECT to_jsonb(v)::text FROM app.semester_final_versions v WHERE semester_id=$1 AND student_id=$2 AND version=1)`, f.Semester, f.StudentA).Scan(&retained, &versions, &after); err != nil {
		t.Fatal(err)
	}
	if retained != 1 || versions != 3 || after != original {
		t.Fatalf("histori %d %d %t", retained, versions, after == original)
	}
	if _, err = db.Exec(`UPDATE app.semester_final_retentions SET reason='diubah' WHERE final_version_id=(SELECT id FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 AND version=3)`, f.Semester, f.StudentA); err == nil {
		t.Fatal("audit mempertahankan dapat diubah")
	}
	if _, err = db.Exec(`DELETE FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 AND version=1`, f.Semester, f.StudentA); err == nil {
		t.Fatal("histori Final dapat dihapus")
	}
	p, err = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !p.NeedsReview {
		t.Fatalf("penanda review: %+v %v", p, err)
	}
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, f.Teacher); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 3, 85, "Pertahankan", p.SnapshotToken, "00000000-0000-4000-8000-000000000907", "retain"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("akun nonaktif %v", err)
	}
}
func TestSemesterFinalRevisionConcurrentIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "revcon"+testutil.Random(t)[:6])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, e := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "80"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "80"}}); e != nil {
		t.Fatal(e)
	}
	p, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000911"); e != nil {
		t.Fatal(e)
	}
	p, e = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 100, "Koreksi", p.SnapshotToken, fmt.Sprintf("00000000-0000-4000-8000-%012d", 920+i), "revision")
		}(i)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("%d berhasil %d konflik", success, conflict)
	}
}

func TestSemesterFinalRevisionTypeTransitionsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "revtypes"+testutil.Random(t)[:6])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, e := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "0"}}); e != nil {
		t.Fatal(e)
	}
	p, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 0, "Final awal", p.SnapshotToken, "00000000-0000-4000-8000-000000000941"); e != nil {
		t.Fatal(e)
	}
	p, e = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "100"}}); e != nil {
		t.Fatal(e)
	}
	var initialID int64
	if e = db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE semester_id=$1 AND student_id=$2 AND component='asts'`, f.Semester, f.StudentA).Scan(&initialID); e != nil {
		t.Fatal(e)
	}
	if e = s.CorrectExamInitialScore(ctx, f.Teacher, initialID, 0, "20", "Koreksi sumber"); e != nil {
		t.Fatal(e)
	}
	if e = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 100, "Bukti lengkap", p.SnapshotToken, "00000000-0000-4000-8000-000000000942", "revision"); !errors.Is(e, ErrConflict) {
		t.Fatalf("bukti basi %v", e)
	}
	p, e = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 100, "Bukti lengkap", p.SnapshotToken, "00000000-0000-4000-8000-000000000943", "revision"); e != nil {
		t.Fatal(e)
	}
	var typ string
	var recommendation int
	if e = db.QueryRow(`SELECT final_type,recommendation_score FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 AND version=2`, f.Semester, f.StudentA).Scan(&typ, &recommendation); e != nil || typ != "normal" || recommendation != 68 {
		t.Fatalf("normal %s %d %v", typ, recommendation, e)
	}
	if e = s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 60, ASTS: 20, ASAS: 20}, 1); e != nil {
		t.Fatal(e)
	}
	p, e = s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 2, 0, "NH belum lengkap", p.SnapshotToken, "00000000-0000-4000-8000-000000000944", "revision"); e != nil {
		t.Fatal(e)
	}
	var nullRecommendation bool
	if e = db.QueryRow(`SELECT final_type,recommendation_score IS NULL FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 AND version=3`, f.Semester, f.StudentA).Scan(&typ, &nullRecommendation); e != nil || typ != "exception" || !nullRecommendation {
		t.Fatalf("exception %s %t %v", typ, nullRecommendation, e)
	}
}
