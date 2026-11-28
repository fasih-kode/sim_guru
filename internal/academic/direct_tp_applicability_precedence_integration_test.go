//go:build integration

package academic

import (
	"context"
	"fmt"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestDirectTPApplicabilityDecisionPrecedenceIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	fixture := testutil.FixtureDatabase(t)
	f := testutil.ExamFixtureFor(t, fixture, "precedence"+testutil.Random(t)[:8])
	ctx := context.Background()
	runtime := &Store{DB: db}
	setup := &Store{DB: fixture}
	if _, err := fixture.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id) VALUES($1,100,0,0,1,$2)`, f.Semester, f.Teacher); err != nil {
		t.Fatal(err)
	}
	for _, class := range []int64{f.Class, f.OtherClass} {
		if err := setup.CreateLearningObjective(ctx, f.Teacher, f.Semester, class, f.Subject, "TP sintetis"); err != nil {
			t.Fatal(err)
		}
	}
	var a, b int64
	if err := fixture.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2`, f.Semester, f.Class).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := fixture.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2`, f.Semester, f.OtherClass).Scan(&b); err != nil {
		t.Fatal(err)
	}
	key := func() string {
		x := testutil.Random(t)
		return fmt.Sprintf("%s-%s-4%s-8%s-%s", x[:8], x[8:12], x[12:15], x[15:18], x[18:30])
	}
	set := func(student, class, objective int64, date string, applies bool) {
		t.Helper()
		err := runtime.SaveDirectTPApplicability(ctx, f.Teacher, DirectTPApplicabilityInput{SemesterID: f.Semester, ClassID: class, SubjectID: f.Subject, StudentID: student, ObjectiveID: objective, EffectiveDate: date, Applicable: applies, EvidenceVerified: true, Reason: "Bukti sintetis", SubmissionKey: key()})
		if err != nil {
			t.Fatal(err)
		}
	}
	check := func(student, class int64, want string) {
		t.Helper()
		book, err := runtime.DirectTPGradebook(ctx, f.Teacher, f.Semester, class, f.Subject)
		if err != nil {
			t.Fatal(err)
		}
		for _, st := range book.Students {
			if st.ID == student {
				if len(st.Cells) != 1 || st.Cells[0].Note != want {
					t.Fatalf("keberlakuan: %+v", st)
				}
				return
			}
		}
		t.Fatal("siswa tidak ada")
	}
	set(f.StudentB, f.Class, a, "2020-01-10", false)
	check(f.StudentB, f.Class, "TP tidak berlaku")
	set(f.StudentA, f.Class, a, "2020-01-10", true)
	check(f.StudentA, f.Class, "")
	set(f.MovedStudent, f.OtherClass, b, "2020-02-10", false)
	check(f.MovedStudent, f.OtherClass, "TP tidak berlaku")
	if _, err := fixture.Exec(`UPDATE app.student_class_moves SET effective_date='2019-12-31' WHERE placement_id=(SELECT id FROM app.student_placements WHERE student_id=$1)`, f.MovedStudent); err != nil {
		t.Fatal(err)
	}
	check(f.MovedStudent, f.OtherClass, "TP tidak berlaku")
	normalized, err := runtime.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range normalized.Students {
		if st.ID == fmtID(f.MovedStudent) && (len(st.Objectives) != 1 || st.Objectives[0].HistoricalApplicability != "tidak berlaku" || st.NH != "") {
			t.Fatalf("NH sesudah penempatan dinormalkan: %+v", st)
		}
	}
	if _, err := fixture.Exec(`UPDATE app.student_placements SET starts_on='2020-03-01' WHERE student_id=$1`, f.MovedStudent); err != nil {
		t.Fatal(err)
	}
	check(f.MovedStudent, f.OtherClass, "Penempatan perlu diverifikasi")
	rec, err := runtime.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range rec.Students {
		if st.ID == fmtID(f.MovedStudent) {
			if st.NH != "" || st.Recommendation != "" || st.Status == "Siap Final" {
				t.Fatalf("NH tidak pasti: %+v", st)
			}
			return
		}
	}
	t.Fatal("siswa tidak ada dalam rekap")
}
