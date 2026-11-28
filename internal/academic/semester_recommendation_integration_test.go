//go:build integration

package academic

import (
	"context"
	"fmt"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestSemesterRecommendationsZeroWeightAndSourceIsolationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, testutil.Random(t)[:8])
	s := &Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id) VALUES($1,0,40,60,1,$2)`, f.Semester, f.Teacher); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "0"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "100"}}); err != nil {
		t.Fatal(err)
	}
	result, err := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if result.OutOfCalendarActivityCount != 0 {
		t.Fatalf("konteks tanpa kegiatan luar kalender: %d", result.OutOfCalendarActivityCount)
	}
	var a, b *SemesterRecommendationStudent
	for i := range result.Students {
		p := &result.Students[i]
		if p.ID == fmt.Sprint(f.StudentA) {
			a = p
		}
		if p.ID == fmt.Sprint(f.StudentB) {
			b = p
		}
	}
	if a == nil || a.Status != "Siap Final" || a.Recommendation != "60" || a.ASTS != "0,00" {
		t.Fatalf("NH nol tanpa TP, ujian nol/seratus: %+v", a)
	}
	if b == nil || b.Status != "Belum Dinilai" || b.Recommendation != "" {
		t.Fatalf("sumber kosong: %+v", b)
	}
	if _, err = s.SemesterRecommendations(ctx, f.Other, f.Semester, f.Class, f.Subject); err != ErrForbidden {
		t.Fatalf("guru lain: %v", err)
	}
	var scoreID int64
	if err = db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2 AND student_id=$3 AND component='asas'`, f.Semester, f.Class, f.StudentA).Scan(&scoreID); err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, scoreID, 0, "0", "Koreksi uji"); err != nil {
		t.Fatal(err)
	}
	assertEffective := func(want string) {
		t.Helper()
		got, e := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range got.Students {
			if item.ID == fmt.Sprint(f.StudentA) {
				if item.Status != "Siap Final" || item.Recommendation != want {
					t.Fatalf("nilai efektif: %+v want %s", item, want)
				}
				return
			}
		}
		t.Fatal("siswa A tidak ditemukan")
	}
	assertEffective("0")
	keyRaw := testutil.Random(t)
	key := fmt.Sprintf("%s-%s-4%s-8%s-%s", keyRaw[:8], keyRaw[8:12], keyRaw[12:15], keyRaw[15:18], keyRaw[18:30])
	if err = s.RecordExamScoreRemedial(ctx, f.Teacher, scoreID, key, "30", "2020-01-11"); err != nil {
		t.Fatal(err)
	}
	assertEffective("18")
	// Dua sumber pada kelas berbeda tidak boleh otomatis dipilih atau diakui.
	for _, x := range []struct {
		class       int64
		date, value string
	}{{f.Class, "2020-01-10", "50"}, {f.OtherClass, "2020-02-10", "90"}} {
		if err = s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, x.class, f.Subject, []ExamInitialInput{{StudentID: f.MovedStudent, Component: "asts", Date: x.date, Value: x.value}}); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	foundMoved := false
	for _, v := range moved.Students {
		if v.ID == fmt.Sprint(f.MovedStudent) {
			if v.Recommendation != "" || v.ASTS != "Perlu pengakuan atau pemilihan sumber" {
				t.Fatalf("sumber lintas kelas: %+v", v)
			}
			foundMoved = true
		}
	}
	if !foundMoved {
		t.Fatal("siswa pindahan tidak ditemukan")
	}
	if _, err = db.Exec(`UPDATE app.subjects SET active=false WHERE id=$1`, f.Subject); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject); err != ErrForbidden {
		t.Fatalf("mapel nonaktif: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.subjects SET active=true WHERE id=$1`, f.Subject); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, f.Teacher); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject); err != ErrForbidden {
		t.Fatalf("akun nonaktif: %v", err)
	}
}

func TestSemesterRecommendationOutsideCalendarIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, testutil.Random(t)[:8])
	s := &Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id) VALUES($1,100,0,0,1,$2)`, f.Semester, f.Teacher); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP presisi luar kalender"); err != nil {
		t.Fatal(err)
	}
	var objective int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&objective); err != nil {
		t.Fatal(err)
	}
	for i, entry := range []struct{ date, value string }{{"2021-01-04", "82.49"}, {"2021-01-05", "82.50"}} {
		name := fmt.Sprintf("Luar kalender %d", i)
		if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, name, "sumatif", entry.date, []int64{objective}); err != nil {
			t.Fatal(err)
		}
		var activity int64
		if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND name=$2`, f.Semester, name).Scan(&activity); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, Value: entry.value}}); err != nil {
			t.Fatal(err)
		}
	}
	// Pembacaan harus murni: seluruh kolom nilai dan histori dibandingkan sebelum dan sesudah.
	fingerprint := func() string {
		t.Helper()
		var result string
		err := db.QueryRow(`SELECT md5(
			coalesce((SELECT string_agg(row_to_json(x)::text,'' ORDER BY x.id) FROM app.assessment_activity_initial_scores x WHERE x.semester_id=$1),'') ||
			coalesce((SELECT string_agg(row_to_json(c)::text,'' ORDER BY c.id) FROM app.assessment_activity_score_corrections c JOIN app.assessment_activity_initial_scores x ON x.id=c.initial_score_id WHERE x.semester_id=$1),'') ||
			coalesce((SELECT string_agg(row_to_json(r)::text,'' ORDER BY r.id) FROM app.assessment_activity_score_remedials r JOIN app.assessment_activity_initial_scores x ON x.id=r.initial_score_id WHERE x.semester_id=$1),'')
		)`, f.Semester).Scan(&result)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := fingerprint()
	result, err := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if result.OutOfCalendarActivityCount != 2 {
		t.Fatalf("jumlah luar kalender=%d", result.OutOfCalendarActivityCount)
	}
	found := false
	for _, student := range result.Students {
		if student.ID == fmt.Sprint(f.StudentA) {
			found = true
			if student.NH != "82,50" || student.Recommendation != "82" || student.Status != "Siap Final" {
				t.Fatalf("nilai eksak 82,495 harus dibulatkan menjadi 82: %+v", student)
			}
		}
	}
	if !found {
		t.Fatal("siswa A tidak ditemukan")
	}
	if before != fingerprint() {
		t.Fatal("pembacaan mengubah nilai historis")
	}
}
