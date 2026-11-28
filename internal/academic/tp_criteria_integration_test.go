//go:build integration

package academic

import (
	"context"
	"errors"
	"fmt"
	"github.com/fasih-kode/sim_guru/internal/testutil"
	"math/big"
	"reflect"
	"sync"
	"testing"
)

func TestTPAchievementExactPrecision(t *testing.T) {
	for _, tc := range []struct {
		value, threshold, want string
		complete               bool
	}{{"75", "75", "Mencapai", true}, {"74.995", "75", "Belum Mencapai", true}, {"0", "0", "Mencapai", true}, {"100", "100", "Mencapai", true}, {"75", "", "Belum Diatur", true}, {"0", "75", "Belum Lengkap", false}} {
		n, _ := new(big.Rat).SetString(tc.value)
		if got := tpAchievement(n, tc.complete, tc.threshold); got != tc.want {
			t.Errorf("%+v: %s", tc, got)
		}
	}
}
func TestTPCriteriaScopeVersionAndPrecisionIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "criteria"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		class      int64
		definition string
	}{{f.Class, "TP Sumber"}, {f.OtherClass, "TP Lain"}} {
		if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, v.class, f.Subject, v.definition); err != nil {
			t.Fatal(err)
		}
	}
	var tp, other int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE class_id=$1`, f.Class).Scan(&tp); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE class_id=$1`, f.OtherClass).Scan(&other); err != nil {
		t.Fatal(err)
	}
	save := func(actor, objective, version int64, value string, reset bool) error {
		return s.SaveTPCriterion(ctx, actor, f.Semester, f.Class, f.Subject, objective, version, value, "Penetapan uji", reset)
	}
	c, err := s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil || c.Default.Configured {
		t.Fatalf("tidak ada default: %+v %v", c, err)
	}
	for _, bad := range []string{"-1", "100,01", "75,001", ""} {
		if err = save(f.Teacher, 0, 0, bad, false); err == nil {
			t.Fatalf("nilai invalid %q", bad)
		}
	}
	if err = save(f.Other, 0, 0, "75", false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru lain: %v", err)
	}
	if err = save(f.Teacher, other, 0, "75", false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("TP konteks lain: %v", err)
	}
	if err = save(f.Teacher, 0, 0, "75", false); err != nil {
		t.Fatal(err)
	}
	if err = save(f.Teacher, tp, 0, "80", false); err != nil {
		t.Fatal(err)
	}
	c, err = s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil || c.Default.Threshold != "75.00" || c.Overrides[tp].Threshold != "80.00" || len(c.History) != 2 {
		t.Fatalf("kriteria: %+v %v", c, err)
	}
	if err = save(f.Teacher, tp, 1, "", true); err != nil {
		t.Fatal(err)
	}
	c, err = s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil || c.Overrides[tp].Configured || c.Overrides[tp].Version != 2 {
		t.Fatalf("reset: %+v %v", c, err)
	}
	var wg sync.WaitGroup
	result := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); result <- save(f.Teacher, 0, 1, fmt.Sprint(76+i), false) }(i)
	}
	wg.Wait()
	close(result)
	ok, conflict := 0, 0
	for e := range result {
		if e == nil {
			ok++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("race %d/%d", ok, conflict)
	}
	guidance, err := s.TPGuidance(ctx, cur.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveTPGuidance(ctx, cur.ID, guidance.Version, "Pedoman sekolah", "Penetapan"); err != nil {
		t.Fatal(err)
	}
	readGuidance := func(actor, class int64) (string, error) {
		return s.TPGuidanceForTeacher(ctx, actor, f.Semester, class, f.Subject)
	}
	if text, e := readGuidance(f.Teacher, f.Class); e != nil || text != "Pedoman sekolah" {
		t.Fatalf("pedoman guru: %q %v", text, e)
	}
	if _, e := readGuidance(f.Other, f.Class); !errors.Is(e, ErrForbidden) {
		t.Fatalf("penugasan lain baca pedoman: %v", e)
	}
	if _, e := readGuidance(cur.ID, f.Class); !errors.Is(e, ErrForbidden) {
		t.Fatalf("tanpa role guru: %v", e)
	}
	if e := s.SaveTPGuidance(ctx, cur.ID, guidance.Version+1, "Pedoman terbaru", "Pembaruan"); e != nil {
		t.Fatal(e)
	}
	if text, e := readGuidance(f.Teacher, f.Class); e != nil || text != "Pedoman terbaru" {
		t.Fatalf("pedoman terbaru: %q %v", text, e)
	}
	audit, e := s.TPGuidance(ctx, cur.ID)
	if e != nil || audit.Version != guidance.Version+2 || len(audit.History) < 2 || audit.History[0].After != "Pedoman terbaru" {
		t.Fatalf("histori pedoman: %+v %v", audit, e)
	}
	if err = s.SaveTPGuidance(ctx, cur.ID, guidance.Version, "Versi basi", "Uji"); !errors.Is(err, ErrConflict) {
		t.Fatalf("versi pedoman basi: %v", err)
	}
	if _, err = db.Exec(`UPDATE app.tp_achievement_guidance_history SET reason='rusak'`); err == nil {
		t.Fatal("histori pedoman dapat diubah")
	}
	if err = s.SaveTPGuidance(ctx, f.Teacher, guidance.Version+2, "Tidak sah", "uji"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("hak pedoman: %v", err)
	}
	for i, v := range []string{"74,99", "75,00"} {
		name := fmt.Sprintf("Sumatif %d", i)
		if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, name, "sumatif", "2020-01-10", []int64{tp}); err != nil {
			t.Fatal(err)
		}
		var activity int64
		if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE class_id=$1 AND name=$2`, f.Class, name).Scan(&activity); err != nil {
			t.Fatal(err)
		}
		if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: tp, Value: v}}); err != nil {
			t.Fatal(err)
		}
	}
	c, err = s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err = save(f.Teacher, tp, c.Overrides[tp].Version, "75", false); err != nil {
		t.Fatal(err)
	}
	recap, err := s.AssessmentTPRecap(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, student := range recap.Students {
		if student.ID == fmt.Sprint(f.StudentA) {
			found = true
			o := student.Objectives[0]
			if o.Average != "75,00" || o.Achievement != "Belum Mencapai" || student.NH != "75,00" {
				t.Fatalf("presisi: %+v %+v", o, student)
			}
		}
	}
	if !found {
		t.Fatal("siswa tidak ada")
	}
	// Tanpa pengecualian, perubahan bawaan langsung memperbarui indikator.
	c, err = s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err = save(f.Teacher, tp, c.Overrides[tp].Version, "", true); err != nil {
		t.Fatal(err)
	}
	if err = save(f.Teacher, 0, c.Default.Version, "74,99", false); err != nil {
		t.Fatal(err)
	}
	updated, err := s.AssessmentTPRecap(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range updated.Students {
		if st.ID == fmt.Sprint(f.StudentA) && st.Objectives[0].Achievement != "Mencapai" {
			t.Fatalf("indikator bawaan: %+v", st.Objectives[0])
		}
	}
	c, err = s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err = save(f.Teacher, tp, c.Overrides[tp].Version, "75", false); err != nil {
		t.Fatal(err)
	}
	// Salinan hanya membawa definisi dan mengikuti kriteria kelas tujuan.
	if err = s.SaveTPCriterion(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, 0, 0, "60", "Bawaan tujuan", false); err != nil {
		t.Fatal(err)
	}
	copied, err := s.CopyLearningObjective(ctx, f.Teacher, tp, f.Semester, f.Class, f.Subject, f.Semester, f.OtherClass, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject)
	if err != nil || destination.Default.Threshold != "60.00" || destination.Overrides[copied].Configured {
		t.Fatalf("salinan kriteria: %+v %v", destination, err)
	}
	if err = s.SaveTPCriterion(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, copied, 0, "0", "Batas nol", false); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveTPCriterion(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, copied, 1, "100", "Batas maksimum", false); err != nil {
		t.Fatal(err)
	}
	var score int64
	if err = db.QueryRow(`SELECT sc.id FROM app.assessment_activity_initial_scores sc JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id WHERE a.class_id=$1 AND a.name='Sumatif 0' AND sc.student_id=$2`, f.Class, f.StudentA).Scan(&score); err != nil {
		t.Fatal(err)
	}
	beforeGuide, e := s.AssessmentTPRecap(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if e != nil {
		t.Fatal(e)
	}
	latestGuide, e := s.TPGuidance(ctx, cur.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SaveTPGuidance(ctx, cur.ID, latestGuide.Version, "Teks tanpa batas baru", "Uji indikator"); e != nil {
		t.Fatal(e)
	}
	afterGuide, e := s.AssessmentTPRecap(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(beforeGuide, afterGuide) {
		t.Fatal("perubahan teks pedoman mengubah rekap atau indikator")
	}
	if err = s.CorrectAssessmentInitialScore(ctx, f.Teacher, score, 0, "75", "Koreksi uji"); err != nil {
		t.Fatal(err)
	}
	assertStatus := func(want string) {
		t.Helper()
		r, e := s.AssessmentTPRecap(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
		if e != nil {
			t.Fatal(e)
		}
		for _, st := range r.Students {
			if st.ID == fmt.Sprint(f.StudentA) {
				if st.Objectives[0].Achievement != want {
					t.Fatalf("indikator: %+v", st.Objectives[0])
				}
				return
			}
		}
		t.Fatal("siswa hilang")
	}
	assertStatus("Mencapai")
	if err = s.CorrectAssessmentInitialScore(ctx, f.Teacher, score, 1, "70", "Koreksi turun"); err != nil {
		t.Fatal(err)
	}
	assertStatus("Belum Mencapai")
	if err = s.RecordAssessmentScoreRemedial(ctx, f.Teacher, score, "44444444-4444-4444-8444-444444444444", "90", "2020-01-10"); err != nil {
		t.Fatal(err)
	}
	assertStatus("Mencapai")
	for _, student := range recap.Students {
		if student.ID == fmt.Sprint(f.StudentB) && student.Objectives[0].Achievement != "Belum Lengkap" {
			t.Fatalf("nilai kosong: %+v", student)
		}
	}
	// Penugasan yang belum aktif/kedaluwarsa, mapel atau identitas nonaktif tidak berhak.
	for _, q := range []string{`UPDATE app.teaching_assignments SET effective_from='2999-01-01' WHERE class_id=$1`, `UPDATE app.teaching_assignments SET effective_from='2020-01-01',effective_until='2020-02-01' WHERE class_id=$1`} {
		if _, err = db.Exec(q, f.Class); err != nil {
			t.Fatal(err)
		}
		if _, e := readGuidance(f.Teacher, f.Class); !errors.Is(e, ErrForbidden) {
			t.Fatalf("pedoman penugasan tidak berlaku: %v", e)
		}
		if _, err = s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.Class, f.Subject); !errors.Is(err, ErrForbidden) {
			t.Fatalf("penugasan tidak berlaku: %v", err)
		}
		if err = save(f.Teacher, 0, 2, "75", false); !errors.Is(err, ErrForbidden) {
			t.Fatalf("mutasi penugasan tidak berlaku: %v", err)
		}
	}
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET effective_from='2020-01-01',effective_until=NULL WHERE class_id=$1`, f.Class); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		table string
		id    int64
	}{{"app.subjects", f.Subject}, {"app.teachers", f.TeacherID}, {"app.users", f.Teacher}} {
		q := "UPDATE " + item.table + " SET active=$2 WHERE id=$1"
		if _, err = db.Exec(q, item.id, false); err != nil {
			t.Fatal(err)
		}
		if _, err = s.TPCriteriaForContext(ctx, f.Teacher, f.Semester, f.Class, f.Subject); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s nonaktif: %v", item.table, err)
		}
		if _, e := readGuidance(f.Teacher, f.Class); !errors.Is(e, ErrForbidden) {
			t.Fatalf("pedoman %s nonaktif: %v", item.table, e)
		}
		if _, err = db.Exec(q, item.id, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`UPDATE app.tp_achievement_criteria_history SET reason='rusak' WHERE criteria_id IN (SELECT id FROM app.tp_achievement_criteria WHERE class_id=$1)`, f.Class); err == nil {
		t.Fatal("histori dapat diubah")
	}
}
