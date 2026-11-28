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

func TestSemesterFinalBatchAtomicReplayAndConcurrencyIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "batch"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); err != nil {
		t.Fatal(err)
	}
	inputs := []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "0"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "100"}, {StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "0"}, {StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "100"}}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, inputs); err != nil {
		t.Fatal(err)
	}
	ids := []SemesterFinalBatchItem{{StudentID: f.StudentA}, {StudentID: f.StudentB}}
	if _, err := s.SemesterFinalBatchPreview(ctx, f.Other, f.Semester, f.Class, f.Subject, ids); !errors.Is(err, ErrForbidden) {
		t.Fatalf("akses guru lain: %v", err)
	}
	if _, err := s.SemesterFinalBatchPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []SemesterFinalBatchItem{{StudentID: f.StudentA}, {StudentID: f.StudentA}}); err == nil {
		t.Fatal("siswa duplikat diterima")
	}
	p, err := s.SemesterFinalBatchPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range p.Items {
		if v.Student.Recommendation != "60" {
			t.Fatalf("rekomendasi bobot nol: %+v", v.Student)
		}
	}
	key := "00000000-0000-4000-8000-000000000481"
	stale := append([]SemesterFinalBatchItem(nil), p.Items...)
	stale[1].SnapshotToken = "0000000000000000000000000000000000000000000000000000000000000000"
	if err = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, stale, key); !errors.Is(err, ErrConflict) {
		t.Fatalf("snapshot basi: %v", err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Final parsial: %d %v", count, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, p.Items, key)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err = range errs {
		if err != nil {
			t.Fatalf("replay serentak: %v", err)
		}
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND final_score=60`, f.Semester).Scan(&count); err != nil || count != 2 {
		t.Fatalf("jumlah Final: %d %v", count, err)
	}
	if err = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, p.Items[:1], key); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload berubah: %v", err)
	}
	if _, err = s.SemesterFinalBatchPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, ids); err == nil {
		t.Fatal("siswa Final dapat dipilih")
	}
	// Pengulangan bukan jalan pintas untuk melewati penugasan yang berakhir.
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET effective_until='2020-02-01' WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND teacher_id=$4`, f.Semester, f.Class, f.Subject, f.TeacherID); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, p.Items, key); !errors.Is(err, ErrForbidden) {
		t.Fatalf("penugasan kedaluwarsa saat replay: %v", err)
	}
}

func TestSemesterFinalBatchPartialClassIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "batch32"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); err != nil {
		t.Fatal(err)
	}
	var year int64
	if err := db.QueryRow(`SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&year); err != nil {
		t.Fatal(err)
	}
	students := []int64{f.StudentA, f.StudentB}
	for i := 0; i < 30; i++ {
		var id int64
		if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, fmt.Sprintf("batch32-%d-%d", f.Semester, i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01')`, year, id, f.Class, f.Teacher); err != nil {
			t.Fatal(err)
		}
		students = append(students, id)
	}
	var scores []ExamInitialInput
	for i, id := range students {
		scores = append(scores, ExamInitialInput{StudentID: id, Component: "asts", Date: "2020-01-10", Value: "0"})
		if i < 29 {
			scores = append(scores, ExamInitialInput{StudentID: id, Component: "asas", Date: "2020-01-10", Value: "100"})
		}
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, scores); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		p, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, students[i])
		if err != nil {
			t.Fatal(err)
		}
		key := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+100)
		if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, students[i], 60, "", p.SnapshotToken, key); err != nil {
			t.Fatal(err)
		}
	}
	before, err := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	var ready, incomplete, final int
	for _, st := range before.Students {
		switch st.Status {
		case "Siap Final":
			ready++
		case "Belum Lengkap":
			incomplete++
		case "Final":
			final++
		}
	}
	if ready != 27 || incomplete != 3 || final != 2 {
		t.Fatalf("kondisi awal siap=%d belum=%d final=%d", ready, incomplete, final)
	}
	var historicBefore string
	if err = db.QueryRow(`SELECT jsonb_agg(to_jsonb(v) ORDER BY v.id)::text FROM app.semester_final_versions v WHERE semester_id=$1`, f.Semester).Scan(&historicBefore); err != nil {
		t.Fatal(err)
	}
	// Token yang sah sekalipun tidak boleh membuat siswa belum lengkap atau
	// siswa yang sudah Final ikut diproses dalam batch.
	for i, id := range []int64{students[29], students[0]} {
		invalidPreview, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, id)
		if e != nil {
			t.Fatal(e)
		}
		validPreview, e := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, students[2])
		if e != nil {
			t.Fatal(e)
		}
		items := []SemesterFinalBatchItem{{StudentID: id, SnapshotToken: invalidPreview.SnapshotToken}, {StudentID: students[2], SnapshotToken: validPreview.SnapshotToken}}
		key := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+104)
		if e = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, items, key); e == nil {
			t.Fatal("siswa belum lengkap atau sudah Final diterima")
		}
		var count int
		if e = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&count); e != nil || count != 2 {
			t.Fatalf("batch tidak sah menghasilkan Final parsial: %d %v", count, e)
		}
	}
	var ids []SemesterFinalBatchItem
	for _, id := range students[2:22] {
		ids = append(ids, SemesterFinalBatchItem{StudentID: id})
	}
	p, err := s.SemesterFinalBatchPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, ids)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, p.Items, "00000000-0000-4000-8000-000000000103"); err != nil {
		t.Fatal(err)
	}
	var historicAfter string
	if err = db.QueryRow(`SELECT jsonb_agg(to_jsonb(v) ORDER BY v.id)::text FROM app.semester_final_versions v WHERE semester_id=$1 AND student_id IN ($2,$3)`, f.Semester, students[0], students[1]).Scan(&historicAfter); err != nil || historicBefore != historicAfter {
		t.Fatalf("histori Final lama berubah: %v", err)
	}
	after, err := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	ready, incomplete, final = 0, 0, 0
	for _, st := range after.Students {
		switch st.Status {
		case "Siap Final":
			ready++
		case "Belum Lengkap":
			incomplete++
		case "Final":
			final++
		}
	}
	if ready != 7 || incomplete != 3 || final != 22 {
		t.Fatalf("hasil batch siap=%d belum=%d final=%d", ready, incomplete, final)
	}
}

func TestSemesterFinalBatchOverlapAndIndividualRaceIntegration(t *testing.T) {
	for _, individual := range []bool{false, true} {
		t.Run(fmt.Sprintf("individual_%v", individual), func(t *testing.T) {
			db := testutil.Database(t, "../../.env")
			ctx := context.Background()
			f := testutil.ExamFixtureFor(t, db, "batchrace"+testutil.Random(t)[:8])
			s := &Store{DB: db}
			cur, _ := testutil.User(t, db, "kurikulum")
			if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
				t.Fatal(err)
			}
			if err := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 0, ASTS: 40, ASAS: 60}, 0); err != nil {
				t.Fatal(err)
			}
			var scores []ExamInitialInput
			for _, id := range []int64{f.StudentA, f.StudentB} {
				scores = append(scores, ExamInitialInput{StudentID: id, Component: "asts", Date: "2020-01-10", Value: "0"}, ExamInitialInput{StudentID: id, Component: "asas", Date: "2020-01-10", Value: "100"})
			}
			if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, scores); err != nil {
				t.Fatal(err)
			}
			all, err := s.SemesterFinalBatchPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []SemesterFinalBatchItem{{StudentID: f.StudentA}, {StudentID: f.StudentB}})
			if err != nil {
				t.Fatal(err)
			}
			one, err := s.SemesterFinalBatchPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []SemesterFinalBatchItem{{StudentID: f.StudentB}})
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			errs := make(chan error, 2)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				errs <- s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, all.Items, "00000000-0000-4000-8000-000000000111")
			}()
			go func() {
				defer wg.Done()
				<-start
				if individual {
					errs <- s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 60, "", one.Items[0].SnapshotToken, "00000000-0000-4000-8000-000000000112")
				} else {
					errs <- s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, one.Items, "00000000-0000-4000-8000-000000000113")
				}
			}()
			close(start)
			wg.Wait()
			close(errs)
			success, conflict := 0, 0
			for err := range errs {
				if err == nil {
					success++
				} else if errors.Is(err, ErrConflict) {
					conflict++
				} else {
					t.Fatalf("hasil konkurensi: %v", err)
				}
			}
			if success != 1 || conflict != 1 {
				t.Fatalf("success=%d conflict=%d", success, conflict)
			}
			var duplicates int
			if err = db.QueryRow(`SELECT count(*) FROM (SELECT student_id FROM app.semester_final_versions WHERE semester_id=$1 GROUP BY student_id HAVING count(*)>1) d`, f.Semester).Scan(&duplicates); err != nil || duplicates != 0 {
				t.Fatalf("duplikat=%d %v", duplicates, err)
			}
		})
	}
}

func TestSemesterFinalBatchMultiTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "batchtp"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	cur, _ := testutil.User(t, db, "kurikulum")
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentWeight(ctx, cur.ID, AssessmentWeights{SemesterID: f.Semester, NH: 100}, 0); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TP batch satu", "TP batch dua"} {
		if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, name); err != nil {
			t.Fatal(err)
		}
	}
	var tp1, tp2 int64
	if err := db.QueryRow(`SELECT min(id),max(id) FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&tp1, &tp2); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Multi TP batch", "sumatif", "2020-01-10", []int64{tp1, tp2}); err != nil {
		t.Fatal(err)
	}
	var activity int64
	if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND name='Multi TP batch'`, f.Semester).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.StudentA, f.StudentB} {
		if err := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, []AssessmentScoreInput{{StudentID: id, ObjectiveID: tp1, Value: "70"}, {StudentID: id, ObjectiveID: tp2, Value: "90"}}); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.SemesterFinalBatchPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []SemesterFinalBatchItem{{StudentID: f.StudentA}, {StudentID: f.StudentB}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, p.Items, "00000000-0000-4000-8000-000000000121"); err != nil {
		t.Fatal(err)
	}
	var count int
	err = db.QueryRow(`SELECT count(*) FROM app.semester_final_versions v CROSS JOIN LATERAL jsonb_array_elements(v.snapshot->'Evidence'->'activities') a CROSS JOIN LATERAL jsonb_array_elements(a->'scores') sc WHERE v.semester_id=$1 AND (sc->'initial'->>'assessment_activity_id')::bigint=$2`, f.Semester, activity).Scan(&count)
	if err != nil || count != 4 {
		t.Fatalf("empat sumber dalam snapshot: %d %v", count, err)
	}
}
