//go:build integration

package academic

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestSemesterFinalMultiTPIndicatorSnapshotIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "finalmulti"+testutil.Random(t)[:7])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 100, ASTS: 0, ASAS: 0}, 0); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TP presisi", "TP kedua"} {
		if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, name); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.Query(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 ORDER BY id`, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Close(); err != nil || len(ids) != 2 {
		t.Fatalf("TP: %v %v", ids, err)
	}
	if err = s.SaveTPCriterion(ctx, f.Teacher, f.Semester, f.Class, f.Subject, 0, 0, "75", "Batas uji", false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Multi TP", "Sumatif kedua"} {
		if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, name, "sumatif", "2020-01-10", ids); err != nil {
			t.Fatal(err)
		}
	}
	rows, err = db.Query(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 ORDER BY id`, f.Semester, f.Class)
	if err != nil {
		t.Fatal(err)
	}
	var activities []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		activities = append(activities, id)
	}
	if err = rows.Close(); err != nil || len(activities) != 2 {
		t.Fatalf("kegiatan: %v %v", activities, err)
	}
	for i, values := range [][]string{{"74.99", "80"}, {"75.00", "90"}} {
		if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activities[i], []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: ids[0], Value: values[0]}, {StudentID: f.StudentA, ObjectiveID: ids[1], Value: values[1]}}); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !p.CanFinalize || len(p.Student.Objectives) != 2 {
		t.Fatalf("pratinjau: %+v %v", p, err)
	}
	o := p.Student.Objectives[0]
	if o.Average != "75,00" || o.AverageExact != "14999/200" || o.Threshold != "75.00" || o.Achievement != "Belum Mencapai" {
		t.Fatalf("presisi indikator: %+v", o)
	}
	var scoreID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3`, activities[0], ids[1], f.StudentA).Scan(&scoreID); err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectAssessmentInitialScore(ctx, f.Teacher, scoreID, 0, "81", "Koreksi uji"); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000051"); !errors.Is(err, ErrConflict) {
		t.Fatalf("pratinjau basi: %v", err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Final parsial: %d %v", count, err)
	}
	p, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !p.CanFinalize {
		t.Fatalf("pratinjau baru: %+v %v", p, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 80, "", p.SnapshotToken, "00000000-0000-4000-8000-000000000052"); err != nil {
		t.Fatal(err)
	}
	var snapshot string
	if err = db.QueryRow(`SELECT snapshot::text FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"74.99", "81.00", "14999/200", "Belum Mencapai"} {
		if !strings.Contains(snapshot, text) {
			t.Fatalf("snapshot tidak memuat %q: %s", text, snapshot)
		}
	}
	var sources int
	if err = db.QueryRow(`SELECT jsonb_array_length(snapshot->'Evidence'->'activities'->0->'scores') FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&sources); err != nil || sources != 2 {
		t.Fatalf("jumlah sumber multi TP: %d %v", sources, err)
	}
	if err = s.SaveTPCriterion(ctx, f.Teacher, f.Semester, f.Class, f.Subject, 0, 1, "70", "Perubahan batas", false); err != nil {
		t.Fatal(err)
	}
	p, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !p.Finalized || !p.NeedsReview || p.FinalValue != "80" {
		t.Fatalf("Final lama: %+v %v", p, err)
	}
	var after string
	if err = db.QueryRow(`SELECT snapshot::text FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&after); err != nil || after != snapshot {
		t.Fatalf("snapshot historis berubah: %v", err)
	}
}
