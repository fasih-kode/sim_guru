//go:build integration

package academic

import (
	"context"
	"fmt"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestExamEffectiveScoreAfterDecreasingCorrectionIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	store := &Store{DB: db}
	cases := []struct {
		initial, correction string
		remedials           []string
		want                string
	}{
		{"65", "0", nil, "0.00"}, {"72,50", "0", nil, "0.00"}, {"80", "20", []string{"30"}, "30.00"}, {"80", "20", []string{"10"}, "20.00"}, {"65", "70", []string{"75", "85", "60"}, "85.00"}, {"80", "", []string{"70"}, "80.00"}, {"80", "90", []string{"85"}, "90.00"},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			f := testutil.ExamFixtureFor(t, db, "effective"+testutil.Random(t)[:8])
			for _, component := range []string{"asts", "asas"} {
				in := ExamInitialInput{StudentID: f.StudentA, Component: component, Date: "2020-01-10", Value: tc.initial}
				if err := store.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{in}); err != nil {
					t.Fatal(err)
				}
				var id int64
				if err := db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE class_id=$1 AND student_id=$2 AND component=$3`, f.Class, f.StudentA, component).Scan(&id); err != nil {
					t.Fatal(err)
				}
				if tc.correction != "" {
					if err := store.CorrectExamInitialScore(ctx, f.Teacher, id, 0, tc.correction, "Koreksi uji"); err != nil {
						t.Fatal(err)
					}
				}
				for j, v := range tc.remedials {
					key := fmt.Sprintf("10000000-0000-4000-8000-%012d", j+1)
					if component == "asas" {
						key = fmt.Sprintf("20000000-0000-4000-8000-%012d", j+1)
					}
					if err := store.RecordExamScoreRemedial(ctx, f.Teacher, id, key, v, "2020-01-10"); err != nil {
						t.Fatal(err)
					}
				}
				detail, err := store.ExamScoreRemedialDetail(ctx, f.Teacher, id)
				if err != nil || detail.Effective != tc.want || len(detail.History) != len(tc.remedials) {
					t.Fatalf("%s efektif=%s histori=%d ingin=%s error=%v", component, detail.Effective, len(detail.History), tc.want, err)
				}
			}
		})
	}
}
