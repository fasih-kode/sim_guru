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

func TestLearningObjectiveCorrectionRoutineImpactHistoryAndFinalReviewIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "cp1018"+testutil.Random(t)[:7])
	s := &Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP awal CP10-18"); err != nil {
		t.Fatal(err)
	}
	var objectiveID int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&objectiveID); err != nil {
		t.Fatal(err)
	}
	state, err := s.LearningObjectiveEdit(ctx, f.Teacher, objectiveID)
	if err != nil || !state.RoutineAllowed || state.Version != 0 {
		t.Fatalf("TP belum digunakan harus terbuka untuk koreksi rutin: %+v %v", state, err)
	}
	definition := "TP awal CP10-18 dengan redaksi yang diperbaiki"
	routineKey := "00000000-0000-4000-8000-000000001801"
	if err = s.CorrectLearningObjectiveRoutine(ctx, f.Teacher, objectiveID, 0, definition, state.RoutineToken, routineKey); err != nil {
		t.Fatalf("koreksi rutin: %v", err)
	}
	// Replay identik tetap berhasil meskipun versi aktif sudah bertambah.
	if err = s.CorrectLearningObjectiveRoutine(ctx, f.Teacher, objectiveID, 0, definition, state.RoutineToken, routineKey); err != nil {
		t.Fatalf("replay rutin identik: %v", err)
	}
	if err = s.CorrectLearningObjectiveRoutine(ctx, f.Teacher, objectiveID, 0, definition+" berbeda", state.RoutineToken, routineKey); !errors.Is(err, ErrConflict) {
		t.Fatalf("kunci sama dengan isi berbeda harus konflik: %v", err)
	}
	if _, err = s.LearningObjectiveEdit(ctx, f.Other, objectiveID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru lain tidak boleh membaca koreksi TP: %v", err)
	}
	var stableID int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE id=$1 AND definition=$2`, objectiveID, definition).Scan(&stableID); err != nil || stableID != objectiveID {
		t.Fatalf("identitas TP harus stabil setelah edit rutin: %d %v", stableID, err)
	}

	curUser, _ := testutil.User(t, db, "kurikulum")
	if err = s.SaveAssessmentWeight(ctx, curUser.ID, AssessmentWeights{SemesterID: f.Semester, NH: 60, ASTS: 20, ASAS: 20}, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif TP CP10-18", "sumatif", "2020-01-10", []int64{objectiveID}); err != nil {
		t.Fatal(err)
	}
	var activityID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activityID, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objectiveID, Value: "80"}, {StudentID: f.StudentB, ObjectiveID: objectiveID, Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{
		{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"},
		{StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "90"},
	}); err != nil {
		t.Fatal(err)
	}
	for i, student := range []int64{f.StudentA, f.StudentB} {
		p, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, student)
		if e != nil || !p.CanFinalize {
			t.Fatalf("siswa %d siap Final: %+v %v", i, p, e)
		}
		key := fmt.Sprintf("00000000-0000-4000-8000-%012d", 1810+i)
		if e = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, student, 80, "", p.SnapshotToken, key); e != nil {
			t.Fatalf("Final fixture %d: %v", i, e)
		}
	}
	var originalFinals []struct {
		Student        int64
		Snapshot, Hash []byte
	}
	rows, err := db.Query(`SELECT student_id,snapshot,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 ORDER BY student_id`, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var v struct {
			Student        int64
			Snapshot, Hash []byte
		}
		if err = rows.Scan(&v.Student, &v.Snapshot, &v.Hash); err != nil {
			t.Fatal(err)
		}
		originalFinals = append(originalFinals, v)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(originalFinals) != 2 {
		t.Fatalf("dua Final awal: %d", len(originalFinals))
	}
	editState, err := s.LearningObjectiveEdit(ctx, f.Teacher, objectiveID)
	if err != nil || editState.RoutineAllowed || !editState.HasFinals || editState.ScoredStudentCount != 2 || editState.FinalStudentCount != 2 {
		t.Fatalf("status dampak setelah Final: %+v %v", editState, err)
	}
	if _, err = s.PreviewLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, editState.Version, "Redaksi tanpa alasan", "redaksi", "", 1, "00000000-0000-4000-8000-000000001819"); err == nil {
		t.Fatal("redaksi TP yang sudah dinilai wajib memiliki alasan")
	}
	var relationCountBefore, scoreCountBefore int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_objectives WHERE learning_objective_id=$1`, objectiveID).Scan(&relationCountBefore); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE learning_objective_id=$1`, objectiveID).Scan(&scoreCountBefore); err != nil {
		t.Fatal(err)
	}
	p1, err := s.PreviewLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, editState.Version, "Definisi substansi CP10-18", "substansi", "Penyesuaian substansi sesuai kurikulum", 1, "00000000-0000-4000-8000-000000001820")
	if err != nil || p1.State.ScoredStudentCount != 2 || p1.State.FinalStudentCount != 2 || p1.State.ActivityCount != 1 || len(p1.ImpactedFinals) != 2 {
		t.Fatalf("pratinjau dampak: %+v %v", p1, err)
	}
	pStale, err := s.PreviewLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, editState.Version, "Definisi stale CP10-18", "redaksi", "Perubahan pratinjau kedua", 1, "00000000-0000-4000-8000-000000001821")
	if err != nil {
		t.Fatal(err)
	}
	// Dua konfirmasi bersamaan pada versi sama: satu tersimpan, satu ditolak.
	type result struct {
		preview LearningObjectiveCorrectionPreview
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, p := range []LearningObjectiveCorrectionPreview{p1, pStale} {
		wg.Add(1)
		go func(p LearningObjectiveCorrectionPreview) {
			defer wg.Done()
			<-start
			results <- result{p, s.ConfirmLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, p.ExpectedVersion, p.DefinitionAfter, p.ChangeType, p.Reason, p.SnapshotToken, p.SubmissionKey)}
		}(p)
	}
	close(start)
	wg.Wait()
	close(results)
	var winner LearningObjectiveCorrectionPreview
	var stalePreview LearningObjectiveCorrectionPreview
	successes, conflicts := 0, 0
	for result := range results {
		if result.err == nil {
			successes++
			winner = result.preview
		} else if errors.Is(result.err, ErrConflict) {
			conflicts++
			stalePreview = result.preview
		} else {
			t.Fatalf("konfirmasi bersamaan: %v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("satu koreksi harus menang: success=%d conflict=%d", successes, conflicts)
	}
	if err = s.ConfirmLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, winner.ExpectedVersion, winner.DefinitionAfter, winner.ChangeType, winner.Reason, winner.SnapshotToken, winner.SubmissionKey); err != nil {
		t.Fatalf("replay koreksi identik: %v", err)
	}
	if err = s.ConfirmLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, stalePreview.ExpectedVersion, stalePreview.DefinitionAfter, stalePreview.ChangeType, stalePreview.Reason, stalePreview.SnapshotToken, stalePreview.SubmissionKey); !errors.Is(err, ErrConflict) {
		t.Fatalf("pratinjau basi harus konflik: %v", err)
	}
	var revisionCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.learning_objective_definition_revisions WHERE learning_objective_id=$1`, objectiveID).Scan(&revisionCount); err != nil || revisionCount != 2 {
		t.Fatalf("histori tidak parsial/ganda: %d %v", revisionCount, err)
	}
	var updated string
	if err = db.QueryRow(`SELECT definition FROM app.learning_objectives WHERE id=$1`, objectiveID).Scan(&updated); err != nil || updated != winner.DefinitionAfter {
		t.Fatalf("uraian aktif: %q %v", updated, err)
	}
	var relationCountAfter, scoreCountAfter int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_objectives WHERE learning_objective_id=$1`, objectiveID).Scan(&relationCountAfter); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE learning_objective_id=$1`, objectiveID).Scan(&scoreCountAfter); err != nil {
		t.Fatal(err)
	}
	if relationCountAfter != relationCountBefore || scoreCountAfter != scoreCountBefore {
		t.Fatalf("koreksi uraian mengubah relasi/nilai: relation=%d/%d scores=%d/%d", relationCountBefore, relationCountAfter, scoreCountBefore, scoreCountAfter)
	}
	for _, old := range originalFinals {
		var snapshot, hash []byte
		if err = db.QueryRow(`SELECT snapshot,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4 AND version=1`, f.Semester, f.Class, f.Subject, old.Student).Scan(&snapshot, &hash); err != nil || string(snapshot) != string(old.Snapshot) || string(hash) != string(old.Hash) {
			t.Fatalf("snapshot Final lama berubah untuk siswa %d: %v", old.Student, err)
		}
	}
	for _, student := range []int64{f.StudentA, f.StudentB} {
		p, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, student)
		if e != nil || !p.NeedsReview || p.FinalValue != "80" {
			t.Fatalf("Final lama wajib ditandai Perlu Ditinjau: %+v %v", p, e)
		}
	}
	// Peninjauan ulang satu siswa lewat CP10-17 tidak mengubah versi siswa lain.
	previewFinal, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 80, "Tinjau ulang setelah koreksi TP", previewFinal.SnapshotToken, "00000000-0000-4000-8000-000000001830", "revision"); err != nil {
		t.Fatalf("review Final satu siswa: %v", err)
	}
	var versionA, versionB int
	if err = db.QueryRow(`SELECT version FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4 ORDER BY version DESC LIMIT 1`, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&versionA); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT max(version) FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4`, f.Semester, f.Class, f.Subject, f.StudentB).Scan(&versionB); err != nil || versionA != 2 || versionB != 1 {
		t.Fatalf("review harus individual: A=%d B=%d %v", versionA, versionB, err)
	}
	// Bukti koreksi TP tidak dapat ditimpa atau dihapus.
	var revisionID int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objective_definition_revisions WHERE learning_objective_id=$1 ORDER BY version DESC LIMIT 1`, objectiveID).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.learning_objective_definition_revisions SET reason='ubah' WHERE id=$1`, revisionID); err == nil {
		t.Fatal("trigger harus menolak UPDATE histori TP")
	}
	if _, err = db.Exec(`DELETE FROM app.learning_objective_definition_revisions WHERE id=$1`, revisionID); err == nil {
		t.Fatal("trigger harus menolak DELETE histori TP")
	}
	// Rutinitas sesudah Final tetap ditolak bahkan untuk TP yang belum dipakai.
	if err = s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP rutin terkunci"); !errors.Is(err, ErrConflict) {
		t.Fatalf("penguncian struktur tetap berlaku: %v", err)
	}
	currentState, err := s.LearningObjectiveEdit(ctx, f.Teacher, objectiveID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectLearningObjectiveRoutine(ctx, f.Teacher, objectiveID, currentState.Version, "Koreksi rutin sesudah Final", currentState.RoutineToken, "00000000-0000-4000-8000-000000001840"); !errors.Is(err, ErrConflict) {
		t.Fatalf("koreksi rutin setelah Final harus ditolak: %v", err)
	}
}
