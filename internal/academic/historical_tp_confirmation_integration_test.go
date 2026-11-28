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

func TestHistoricalTPConfirmationConcurrencyAndIntegrityIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "cp1020"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester)
	exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,actor_id) VALUES($1,0,40,60,$2)`, f.Semester, f.Teacher)
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP awal"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{
		{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"},
		{StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "90"},
	}); err != nil {
		t.Fatal(err)
	}
	finalPreview, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !finalPreview.CanFinalize {
		t.Fatalf("pratinjau Final: %+v %v", finalPreview, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 82, "", finalPreview.SnapshotToken, "00000000-0000-4000-8000-000000002051"); err != nil {
		t.Fatal(err)
	}
	var originalSnapshot string
	var originalHash []byte
	if err = db.QueryRow(`SELECT snapshot::text,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&originalSnapshot, &originalHash); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "rutin ditolak"); !errors.Is(err, ErrConflict) {
		t.Fatalf("guard rutin: %v", err)
	}
	input := HistoricalTPPreviewInput{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, Definition: "TP historis", Reason: "terlupa", AssessmentDate: "2020-01-10", DateSource: "arsip", Learning: HistoricalTPReference{Kind: "manual", Document: "jurnal 1", Archive: "lemari 1", Teacher: "guru", Date: "2020-01-09"}, Assessment: HistoricalTPReference{Kind: "manual", Document: "sumatif 1", Archive: "lemari 2", Teacher: "guru", ResultSource: "lembar hasil"}}
	preview, err := s.PreviewHistoricalTP(ctx, f.Teacher, input, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	req := HistoricalTPConfirmation{Preview: input, Verification: HistoricalTPVerification{LearningSection: "halaman 1", AssessmentSection: "halaman 2", Method: "diperiksa", ResultSource: "lembar hasil", Acknowledged: true}, SnapshotToken: preview.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002052"}
	if _, denied := s.ConfirmHistoricalLearningObjective(ctx, f.Other, req); !errors.Is(denied, ErrForbidden) {
		t.Fatalf("guru lain: %v", denied)
	}
	foreign := req
	foreign.Preview.Assessment = HistoricalTPReference{Kind: "app", SourceID: "999999999"}
	if _, denied := s.ConfirmHistoricalLearningObjective(ctx, f.Teacher, foreign); !errors.Is(denied, ErrForbidden) {
		t.Fatalf("sumber asing: %v", denied)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=60,asts=20,asas=20 WHERE semester_id=$1`, f.Semester)
	if _, err = s.ConfirmHistoricalLearningObjective(ctx, f.Teacher, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("snapshot basi: %v", err)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=0,asts=40,asas=60 WHERE semester_id=$1`, f.Semester)
	preview, err = s.PreviewHistoricalTP(ctx, f.Teacher, input, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	req.SnapshotToken = preview.SnapshotToken
	var wg sync.WaitGroup
	ids := make([]int64, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ids[i], errs[i] = s.ConfirmHistoricalLearningObjective(ctx, f.Teacher, req)
		}(i)
	}
	close(start)
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || ids[0] != ids[1] {
		t.Fatalf("konkuren: %v %v %v", ids, errs[0], errs[1])
	}
	changed := req
	changed.Verification.Method = "berbeda"
	if _, err = s.ConfirmHistoricalLearningObjective(ctx, f.Teacher, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("replay berbeda: %v", err)
	}
	checkStatus := func(want string) {
		t.Helper()
		recommendations, e := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
		if e != nil {
			t.Fatal(e)
		}
		for _, st := range recommendations.Students {
			if st.ID == fmt.Sprint(f.StudentB) {
				if st.Status != want {
					t.Fatalf("status B: %s ingin %s", st.Status, want)
				}
				return
			}
		}
		t.Fatal("siswa B tidak ditemukan")
	}
	review, err := s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !review.NeedsReview || review.FinalValue != "82" {
		t.Fatalf("Final harus perlu ditinjau: %+v %v", review, err)
	}
	checkStatus("Siap Final") // NH nol tidak menghalangi.
	exec(`UPDATE app.semester_assessment_weights SET nh=60,asts=20,asas=20 WHERE semester_id=$1`, f.Semester)
	checkStatus("Belum Lengkap") // NH positif dengan TP tanpa nilai.
	var objectives, audits, scores, links int
	var finalSnapshot string
	var finalHash []byte
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.learning_objectives WHERE semester_id=$1),(SELECT count(*) FROM app.historical_learning_objective_insertions WHERE semester_id=$1),(SELECT count(*) FROM app.assessment_activity_initial_scores WHERE semester_id=$1),(SELECT count(*) FROM app.assessment_activity_objectives WHERE semester_id=$1),(SELECT snapshot::text FROM app.semester_final_versions WHERE semester_id=$1),(SELECT snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1)`, f.Semester).Scan(&objectives, &audits, &scores, &links, &finalSnapshot, &finalHash); err != nil {
		t.Fatal(err)
	}
	if objectives != 2 || audits != 1 || scores != 0 || links != 0 || finalSnapshot != originalSnapshot || !equalHash(finalHash, originalHash) {
		t.Fatalf("integritas: %d %d %d %d %s", objectives, audits, scores, links, finalSnapshot)
	}
	var ordinal int
	if err = db.QueryRow(`SELECT ordinal FROM app.direct_tp_codes WHERE objective_id=$1`, ids[0]).Scan(&ordinal); err != nil || ordinal != 2 {
		t.Fatalf("kode TP historis: %d %v", ordinal, err)
	}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "00000000-0000-4000-8000-000000002053", []DirectTPScoreInput{{StudentID: f.StudentB, ObjectiveID: ids[0], ExpectedVersion: -1, Value: "75"}}); err != nil {
		t.Fatalf("nilai langsung TP historis pada penempatan sah: %v", err)
	}
}
