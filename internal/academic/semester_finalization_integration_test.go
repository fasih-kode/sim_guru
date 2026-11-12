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

func TestSemesterFinalNormalSnapshotIdempotencyLockAndReviewIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "final"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	var err error
	if _, err = db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 60, ASTS: 20, ASAS: 20}, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP Final CP10-14"); err != nil {
		t.Fatal(err)
	}
	var objective int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&objective); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif Final CP10-14", "sumatif", "2020-01-10", []int64{objective}); err != nil {
		t.Fatal(err)
	}
	var activity int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	inputs := []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, Value: "80"}, {StudentID: f.StudentB, ObjectiveID: objective, Value: "80"}}
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, inputs); err != nil {
		t.Fatal(err)
	}
	exams := []ExamInitialInput{
		{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"},
		{StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "90"},
	}
	if err = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, exams); err != nil {
		t.Fatal(err)
	}
	preview, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !preview.CanFinalize || preview.Student.Recommendation != "80" {
		t.Fatalf("pratinjau siap: %+v %v", preview, err)
	}
	// Nilai/status yang dikirim lama ditolak setelah bukti input berubah.
	if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif tambahan sebelum Final", "sumatif", "2020-02-10", []int64{objective}); err != nil {
		t.Fatal(err)
	}
	var added int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND name='Sumatif tambahan sebelum Final'`, f.Semester, f.Class).Scan(&added); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, added, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, Value: "80"}, {StudentID: f.StudentB, ObjectiveID: objective, Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", preview.SnapshotToken, "00000000-0000-4000-8000-000000000001"); !errors.Is(err, ErrConflict) {
		t.Fatalf("snapshot basi harus konflik: %v", err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 0 {
		t.Fatalf("tidak boleh ada Final parsial: %d %v", count, err)
	}
	preview, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !preview.CanFinalize || preview.Student.Recommendation != "80" {
		t.Fatalf("pratinjau baru: %+v %v", preview, err)
	}

	type result struct {
		key string
		err error
	}
	keys := []string{"00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003"}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			<-start
			results <- result{key, s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", preview.SnapshotToken, key)}
		}(key)
	}
	close(start)
	wg.Wait()
	close(results)
	var success string
	successes, conflicts := 0, 0
	for r := range results {
		if r.err == nil {
			successes++
			success = r.key
		} else if errors.Is(r.err, ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("final konkuren: %v", r.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("hasil konkurensi success=%d conflict=%d", successes, conflicts)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", preview.SnapshotToken, success); err != nil {
		t.Fatalf("replay identik harus idempoten: %v", err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 81, "Nilai berbeda", preview.SnapshotToken, success); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload berbeda pada kunci sama harus konflik: %v", err)
	}
	var recommendation, final int
	var snapshot []byte
	var hash []byte
	if err = db.QueryRow(`SELECT recommendation_score,final_score,snapshot,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4`, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&recommendation, &final, &snapshot, &hash); err != nil {
		t.Fatal(err)
	}
	if recommendation != 80 || final != 80 || len(snapshot) == 0 || len(hash) != 32 {
		t.Fatalf("snapshot Final tidak lengkap: recommendation=%d final=%d", recommendation, final)
	}
	preview, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !preview.Finalized || preview.NeedsReview || preview.FinalValue != "80" {
		t.Fatalf("Final stabil: %+v %v", preview, err)
	}
	list, err := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, st := range list.Students {
		if st.ID == fmt.Sprint(f.StudentA) {
			found = st.Status == "Final" && st.Finalized && st.FinalValue == "80" && !st.NeedsReview
		}
	}
	if !found {
		t.Fatal("daftar rekomendasi tidak menampilkan Final dengan snapshot mutakhir")
	}
	// Struktur rutin terkunci hanya pada konteks yang sudah mempunyai Final.
	if err = s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP setelah Final"); !errors.Is(err, ErrConflict) {
		t.Fatalf("TP rutin setelah Final: %v", err)
	}
	if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif setelah Final", "sumatif", "2020-03-10", []int64{objective}); !errors.Is(err, ErrConflict) {
		t.Fatalf("Sumatif rutin setelah Final: %v", err)
	}
	if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Formatif setelah Final", "formatif", "2020-03-10", []int64{objective}); err != nil {
		t.Fatalf("Formatif tidak termasuk penguncian PN-05: %v", err)
	}
	if err = s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, "TP konteks lain tetap terbuka"); err != nil {
		t.Fatalf("konteks lain tidak boleh terkunci: %v", err)
	}
	previewB, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB)
	if err != nil || !previewB.CanFinalize || previewB.Student.Recommendation != "80" {
		t.Fatalf("pratinjau siswa B: %+v %v", previewB, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 81, "", previewB.SnapshotToken, "00000000-0000-4000-8000-000000000004"); err == nil {
		t.Fatal("Final berbeda dari rekomendasi harus meminta alasan")
	} else {
		var validation ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("jenis penolakan alasan: %v", err)
		}
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 81, "Penyesuaian hasil akhir", previewB.SnapshotToken, "00000000-0000-4000-8000-000000000005"); err != nil {
		t.Fatalf("Final berbeda dengan alasan: %v", err)
	}

	// Perubahan bobot setelah Final tidak mengubah snapshot lama; perubahan dikenali.
	if err = s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 50, ASTS: 20, ASAS: 30}, 1); err != nil {
		t.Fatal(err)
	}
	preview, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !preview.Finalized || !preview.NeedsReview || preview.FinalValue != "80" {
		t.Fatalf("perubahan bobot harus menandai Final lama: %+v %v", preview, err)
	}
	var unchanged int
	if err = db.QueryRow(`SELECT final_score FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&unchanged); err != nil || unchanged != 80 {
		t.Fatalf("nilai Final historis tidak boleh berubah: %d %v", unchanged, err)
	}
}

func TestSemesterFinalNormalZeroWeightAndAuthorizationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "finalzero"+testutil.Random(t)[:7])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	var err error
	if _, err = db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "0"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "100"}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !p.CanFinalize || p.Student.Recommendation != "60" {
		t.Fatalf("bobot NH nol dan nilai 0 sah: %+v %v", p, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Other, f.Semester, f.Class, f.Subject, f.StudentA, 60, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000011"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru tanpa penugasan harus ditolak: %v", err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 60, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000012"); err != nil {
		t.Fatal(err)
	}
	// Siswa tanpa bukti dan siswa dengan komponen berbobot positif yang belum
	// lengkap tidak boleh difinalkan, sekalipun request membawa angka valid.
	previewB, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB)
	if err != nil || previewB.CanFinalize || previewB.Student.Status != "Belum Dinilai" {
		t.Fatalf("siswa tanpa bukti harus Belum Dinilai: %+v %v", previewB, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 60, "", previewB.SnapshotToken, "00000000-0000-4000-8000-000000000013"); err == nil {
		t.Fatal("siswa Belum Dinilai tidak boleh difinalkan")
	}
	var finalCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentB).Scan(&finalCount); err != nil || finalCount != 0 {
		t.Fatalf("penolakan Belum Dinilai tidak boleh menyimpan Final: %d %v", finalCount, err)
	}
	if err = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "0"}}); err != nil {
		t.Fatal(err)
	}
	previewB, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB)
	if err != nil || previewB.CanFinalize || previewB.Student.Status != "Belum Lengkap" {
		t.Fatalf("ASTS 0 sah tetapi ASAS berbobot masih kosong: %+v %v", previewB, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 60, "", previewB.SnapshotToken, "00000000-0000-4000-8000-000000000014"); err == nil {
		t.Fatal("siswa Belum Lengkap tidak boleh difinalkan")
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentB).Scan(&finalCount); err != nil || finalCount != 0 {
		t.Fatalf("penolakan Belum Lengkap tidak boleh menyimpan Final: %d %v", finalCount, err)
	}
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, f.Teacher); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 60, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000016"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("akun guru nonaktif tidak boleh melakukan Final: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.users SET active=true WHERE id=$1`, f.Teacher); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.subjects SET active=false WHERE id=$1`, f.Subject); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB); !errors.Is(err, ErrForbidden) {
		t.Fatalf("mapel nonaktif harus ditolak: %v", err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 60, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000015"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("mapel nonaktif tidak boleh menerima Final: %v", err)
	}
}

func TestSemesterFinalizationSerializesAgainstRoutineStructureIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "finalrace"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 60, ASTS: 20, ASAS: 20}, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP awal CP10-14 race"); err != nil {
		t.Fatal(err)
	}
	var objective, activity int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&objective); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif awal CP10-14 race", "sumatif", "2020-01-10", []int64{objective}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, Value: "80"}, {StudentID: f.StudentB, ObjectiveID: objective, Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"}}); err != nil {
		t.Fatal(err)
	}
	preview, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !preview.CanFinalize {
		t.Fatalf("pratinjau Final race tidak siap: %+v %v", preview, err)
	}
	start := make(chan struct{})
	finalResult := make(chan error, 1)
	structureResult := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		finalResult <- s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", preview.SnapshotToken, "00000000-0000-4000-8000-000000000021")
	}()
	go func() {
		defer wg.Done()
		<-start
		structureResult <- s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP baru bersamaan dengan Final")
	}()
	close(start)
	wg.Wait()
	finalErr, structureErr := <-finalResult, <-structureResult
	finalSucceeded := finalErr == nil
	structureSucceeded := structureErr == nil
	if finalSucceeded == structureSucceeded {
		t.Fatalf("Final dan penambahan struktur harus terserialisasi: final=%v structure=%v", finalErr, structureErr)
	}
	if finalErr != nil && !errors.Is(finalErr, ErrConflict) {
		t.Fatalf("pratinjau yang kalah race harus konflik: %v", finalErr)
	}
	if structureErr != nil && !errors.Is(structureErr, ErrConflict) {
		t.Fatalf("struktur yang kalah race harus konflik: %v", structureErr)
	}
	var finalCount, objectiveCount int
	if err := db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&finalCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&objectiveCount); err != nil {
		t.Fatal(err)
	}
	if finalCount != boolInt(finalSucceeded) || objectiveCount != 1+boolInt(structureSucceeded) {
		t.Fatalf("hasil race meninggalkan data tidak konsisten: final=%d objective=%d", finalCount, objectiveCount)
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
