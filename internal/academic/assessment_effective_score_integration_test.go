//go:build integration

package academic

import (
	"context"
	"fmt"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAssessmentEffectiveScoreDecreasingCorrectionAcrossViewsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	s := &Store{DB: db}
	f := testutil.ExamFixtureFor(t, db, "daily"+testutil.Random(t)[:8])
	_, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP uji efektif"); err != nil {
		t.Fatal(err)
	}
	var objective, year int64
	if err = db.QueryRow(`SELECT id,school_year_id FROM app.learning_objectives WHERE class_id=$1`, f.Class).Scan(&objective, &year); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"sumatif", "formatif"} {
		if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Kegiatan "+kind, kind, "2020-01-10", []int64{objective}); err != nil {
			t.Fatal(err)
		}
	}
	var sumatif, formatif int64
	for _, v := range []struct {
		k string
		p *int64
	}{{"sumatif", &sumatif}, {"formatif", &formatif}} {
		if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE class_id=$1 AND activity_type=$2`, f.Class, v.k).Scan(v.p); err != nil {
			t.Fatal(err)
		}
	}
	var blank int64
	if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES('Fixture nilai kosong','2010-01-01','L') RETURNING id`).Scan(&blank); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01')`, year, blank, f.Class, f.Teacher); err != nil {
		t.Fatal(err)
	}
	inputs := []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, Value: "80"}, {StudentID: f.StudentB, ObjectiveID: objective, Value: "80"}, {StudentID: f.MovedStudent, ObjectiveID: objective, Value: "0"}}
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, sumatif, inputs); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, formatif, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	var a, b, form int64
	for _, x := range []struct {
		activity, student int64
		target            *int64
	}{{sumatif, f.StudentA, &a}, {sumatif, f.StudentB, &b}, {formatif, f.StudentA, &form}} {
		if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2`, x.activity, x.student).Scan(x.target); err != nil {
			t.Fatal(err)
		}
		if err = s.CorrectAssessmentInitialScore(ctx, f.Teacher, *x.target, 0, "20", "Koreksi fixture"); err != nil {
			t.Fatal(err)
		}
	}
	check := func(id, student int64, want string) {
		t.Helper()
		d, e := s.AssessmentScoreCorrectionDetail(ctx, f.Teacher, id)
		if e != nil || d.Score.InitialValue != "80.00" || d.Score.CorrectedValue != "20.00" || d.Score.EffectiveValue != want {
			t.Fatalf("detail: %+v %v", d, e)
		}
		r, e := s.AssessmentScoreRemedialDetail(ctx, f.Teacher, id)
		if e != nil || r.Effective != want {
			t.Fatalf("remedial: %+v %v", r, e)
		}
		list, e := s.OwnedAssessmentInitialScores(ctx, f.Teacher, AssessmentOwnedScoreFilter{Page: 1})
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, x := range list.Items {
			if x.ID == id {
				found = true
				if x.EffectiveValue != want {
					t.Fatalf("daftar: %+v", x)
				}
			}
		}
		if !found {
			t.Fatal("nilai hilang dari daftar")
		}
		sheet, e := s.AssessmentActivityScoreSheet(ctx, f.Teacher, sumatif)
		if e != nil {
			t.Fatal(e)
		}
		for _, x := range sheet.Students {
			if x.ID == student {
				if x.Scores[0].Value != want {
					t.Fatalf("lembar: %+v", x)
				}
			}
			if x.ID == blank && x.Scores[0].Recorded {
				t.Fatal("kosong dianggap nilai")
			}
			if x.ID == f.MovedStudent && (!x.Scores[0].Recorded || x.Scores[0].Value != "0.00") {
				t.Fatalf("nol tidak sah: %+v", x)
			}
		}
		recap, e := s.AssessmentTPRecap(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
		if e != nil {
			t.Fatal(e)
		}
		for _, x := range recap.Students {
			if x.ID == fmt.Sprint(student) {
				formatted := want[:len(want)-3] + "," + want[len(want)-2:]
				if x.NH != formatted || !x.NHComplete {
					t.Fatalf("NH: %+v", x)
				}
			}
			if x.ID == fmt.Sprint(blank) && x.NHComplete {
				t.Fatal("nilai kosong dianggap lengkap")
			}
		}
	}
	check(a, f.StudentA, "20.00")
	check(b, f.StudentB, "20.00")
	if err = s.RecordAssessmentScoreRemedial(ctx, f.Teacher, b, "11111111-1111-4111-8111-111111111111", "10", "2020-01-10"); err != nil {
		t.Fatal(err)
	}
	check(b, f.StudentB, "20.00")
	if err = s.RecordAssessmentScoreRemedial(ctx, f.Teacher, a, "22222222-2222-4222-8222-222222222222", "30", "2020-01-10"); err != nil {
		t.Fatal(err)
	}
	check(a, f.StudentA, "30.00")
	formSheet, err := s.AssessmentActivityScoreSheet(ctx, f.Teacher, formatif)
	if err != nil {
		t.Fatal(err)
	}
	for _, student := range formSheet.Students {
		if student.ID == f.StudentA && student.Scores[0].Value != "20.00" {
			t.Fatalf("lembar Formatif berubah: %+v", student)
		}
	}
	fd, err := s.AssessmentScoreCorrectionDetail(ctx, f.Teacher, form)
	if err != nil || fd.Score.EffectiveValue != "20.00" {
		t.Fatalf("formatif: %+v %v", fd, err)
	}
}
