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

// Jalankan hanya setelah migration 00066 dan privilege S-06 diotorisasi PO.
func TestDirectTPApplicabilityMovedStudentIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	fixture := testutil.FixtureDatabase(t)
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, fixture, "s06"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	fixtureStore := &Store{DB: fixture}
	if _, err := fixture.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id) VALUES($1,100,0,0,1,$2)`, f.Semester, f.Teacher); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TP berlaku", "TP dikecualikan"} {
		if err := fixtureStore.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, name); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := fixture.Query(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 ORDER BY id`, f.Semester, f.OtherClass)
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
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(ids) != 2 {
		t.Fatal(ids)
	}
	if err = fixtureStore.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP siswa normal"); err != nil {
		t.Fatal(err)
	}
	var normalObjective int64
	if err = fixture.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2`, f.Semester, f.Class).Scan(&normalObjective); err != nil {
		t.Fatal(err)
	}
	key := func() string {
		x := testutil.Random(t)
		return fmt.Sprintf("%s-%s-4%s-8%s-%s", x[:8], x[8:12], x[12:15], x[15:18], x[18:30])
	}
	base := DirectTPApplicabilityInput{SemesterID: f.Semester, ClassID: f.OtherClass, SubjectID: f.Subject, StudentID: f.MovedStudent, ObjectiveID: ids[0], EffectiveDate: "2020-02-10", Applicable: true, EvidenceVerified: true, Reason: "Dasar pembelajaran fixture", SubmissionKey: key()}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, key(), []DirectTPScoreInput{{StudentID: f.StudentB, ObjectiveID: normalObjective, ExpectedVersion: -1, Value: "80"}}); err != nil {
		t.Fatalf("siswa normal tanpa penetapan: %v", err)
	}
	if _, err = fixture.Exec(`UPDATE app.student_placements SET starts_on=NULL WHERE student_id=$1`, f.StudentA); err != nil {
		t.Fatal(err)
	}
	unknown := base
	unknown.ClassID = f.Class
	unknown.StudentID = f.StudentA
	unknown.ObjectiveID = normalObjective
	unknown.SubmissionKey = key()
	if err = s.SaveDirectTPApplicability(ctx, f.Teacher, unknown); !errors.Is(err, ErrForbidden) {
		t.Fatalf("tanggal penempatan tidak pasti: %v", err)
	}
	unknownPreview, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || unknownPreview.CanFinalize || unknownPreview.CanFinalizeException {
		t.Fatalf("Final pada penempatan tidak pasti: %+v %v", unknownPreview, err)
	}
	if err = s.SaveDirectTPApplicability(ctx, f.Other, base); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru asing: %v", err)
	}
	invalid := base
	invalid.EffectiveDate = "2020-01-10"
	invalid.SubmissionKey = key()
	if err = s.SaveDirectTPApplicability(ctx, f.Teacher, invalid); !errors.Is(err, ErrForbidden) {
		t.Fatalf("tanggal tidak sah: %v", err)
	}
	if err = s.SaveDirectTPApplicability(ctx, f.Teacher, base); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDirectTPApplicability(ctx, f.Teacher, base); err != nil {
		t.Fatalf("replay identik: %v", err)
	}
	changed := base
	changed.Reason = "Payload berbeda"
	if err = s.SaveDirectTPApplicability(ctx, f.Teacher, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("replay berbeda: %v", err)
	}
	excluded := base
	excluded.ObjectiveID = ids[1]
	excluded.Applicable = false
	excluded.SubmissionKey = key()
	if err = s.SaveDirectTPApplicability(ctx, f.Teacher, excluded); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, key(), []DirectTPScoreInput{{StudentID: f.MovedStudent, ObjectiveID: ids[0], ExpectedVersion: -1, Value: "0"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, key(), []DirectTPScoreInput{{StudentID: f.MovedStudent, ObjectiveID: ids[1], ExpectedVersion: -1, Value: "70"}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("TP tidak berlaku: %v", err)
	}
	result, err := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, st := range result.Students {
		if st.ID == fmtID(f.MovedStudent) {
			found = true
			if st.NH != "0,00" || st.Recommendation != "0" || st.Status != "Siap Final" {
				t.Fatalf("NH pindahan: %+v", st)
			}
		}
	}
	if !found {
		t.Fatal("siswa tidak ditemukan")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		next := base
		next.ExpectedVersion = 1
		next.SubmissionKey = key()
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.SaveDirectTPApplicability(ctx, f.Teacher, next) }()
	}
	wg.Wait()
	close(errs)
	ok, conflict := 0, 0
	for e := range errs {
		if e == nil {
			ok++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("konkurensi: %d %d", ok, conflict)
	}
	var count int
	if err = fixture.QueryRow(`SELECT count(*) FROM app.direct_tp_applicability_history WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.MovedStudent).Scan(&count); err != nil || count != 3 {
		t.Fatalf("histori: %d %v", count, err)
	}
	preview, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, f.MovedStudent)
	if err != nil || !preview.CanFinalize {
		t.Fatalf("pratinjau Final: %+v %v", preview, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.OtherClass, f.Subject, f.MovedStudent, 0, "", preview.SnapshotToken, key()); err != nil {
		t.Fatal(err)
	}
	afterFinal := base
	afterFinal.ExpectedVersion = 2
	afterFinal.SubmissionKey = key()
	if err = s.SaveDirectTPApplicability(ctx, f.Teacher, afterFinal); !errors.Is(err, ErrConflict) {
		t.Fatalf("penetapan setelah Final: %v", err)
	}
}
