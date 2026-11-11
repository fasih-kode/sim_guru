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
