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

func TestHistoricalAssessmentLinkServiceIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "cp1021"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester)
	exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,actor_id) VALUES($1,0,40,60,$2)`, f.Semester, f.Teacher)
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP awal"); err != nil {
		t.Fatal(err)
	}
	var initial int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1`, f.Semester).Scan(&initial); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif lama", "sumatif", "2020-01-11", []int64{initial}); err != nil {
		t.Fatal(err)
	}
	var activity int64
	if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1`, f.Semester).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: initial, Value: "0"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"}, {StudentID: f.StudentB, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentB, Component: "asas", Date: "2020-01-10", Value: "90"}}); err != nil {
		t.Fatal(err)
	}
	final, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !final.CanFinalize {
		t.Fatalf("final: %+v %v", final, err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 82, "", final.SnapshotToken, "00000000-0000-4000-8000-000000002111"); err != nil {
		t.Fatal(err)
	}
	var beforeSnapshot string
	var beforeHash []byte
	if err = db.QueryRow(`SELECT snapshot::text,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&beforeSnapshot, &beforeHash); err != nil {
		t.Fatal(err)
	}
	input := HistoricalTPPreviewInput{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, Definition: "TP historis", Reason: "terlupa", AssessmentDate: "2020-01-10", DateSource: "arsip", Learning: HistoricalTPReference{Kind: "manual", Document: "jurnal", Archive: "lemari", Teacher: "guru", Date: "2020-01-09"}, Assessment: HistoricalTPReference{Kind: "manual", Document: "sumatif", Archive: "lemari", Teacher: "guru", ResultSource: "hasil"}}
	p, err := s.PreviewHistoricalTP(ctx, f.Teacher, input, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	tp, err := s.ConfirmHistoricalLearningObjective(ctx, f.Teacher, HistoricalTPConfirmation{Preview: input, Verification: HistoricalTPVerification{LearningSection: "satu", AssessmentSection: "dua", Method: "periksa", ResultSource: "hasil", Acknowledged: true}, SnapshotToken: p.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002112"})
	if err != nil {
		t.Fatal(err)
	}
	link := HistoricalAssessmentLinkInput{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, ObjectiveID: tp, ActivityID: activity, Reason: "koreksi", Document: "sumatif", Archive: "lemari", Section: "butir satu", Method: "periksa", ResultSource: "lembar hasil", Acknowledged: true}
	if _, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, ""); err == nil {
		t.Fatal("tanggal berbeda tanpa penjelasan diterima")
	}
	link.DateExplanation, link.DateEvidence = "Penilaian berlangsung sehari setelah sumber", "tanggal pada lembar penilaian"
	var year int64
	if err = db.QueryRow(`SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&year); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name  string
		start any
		moved string
	}{
		{"pindah sebelum", "2020-01-01", "2020-01-05"},
		{"pindah sesudah", "2020-01-01", "2020-01-20"},
		{"belum pasti", nil, ""},
	} {
		var st, placement int64
		if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, scenario.name).Scan(&st); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5) RETURNING id`, year, st, f.Class, f.Teacher, scenario.start).Scan(&placement); err != nil {
			t.Fatal(err)
		}
		if scenario.moved != "" {
			exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,$4::date,'uji',$5)`, placement, f.Class, f.OtherClass, scenario.moved, f.Teacher)
		}
	}
	if _, err = s.PreviewHistoricalAssessmentLink(ctx, f.Other, link, 1, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru asing: %v", err)
	}
	bad := link
	bad.Section = ""
	if _, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, bad, 1, ""); err == nil {
		t.Fatal("bukti kosong diterima")
	}
	bad = link
	bad.ObjectiveID = initial
	if _, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, bad, 1, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("TP tanpa provenance: %v", err)
	}
	preview, err := s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	foundPlacements := map[string]bool{}
	for _, st := range preview.Students {
		switch st.Current.Name {
		case "pindah sebelum":
			foundPlacements[st.Current.Name] = true
			if st.Current.Relevance != "Bukan anggota kelas pada tanggal penilaian" {
				t.Fatalf("pindah sebelum: %+v", st)
			}
		case "pindah sesudah":
			foundPlacements[st.Current.Name] = true
			if st.Current.Relevance != "Kandidat anggota kelas" {
				t.Fatalf("pindah sesudah: %+v", st)
			}
		case "belum pasti":
			foundPlacements[st.Current.Name] = true
			if st.Current.Relevance != "Belum dapat diverifikasi" {
				t.Fatalf("penempatan ambigu: %+v", st)
			}
		}
	}
	if len(foundPlacements) != 3 {
		t.Fatalf("siswa penempatan tidak lengkap: %v", foundPlacements)
	}
	exec(`UPDATE app.teaching_assignments SET effective_until=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject)
	if _, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("assignment berakhir: %v", err)
	}
	exec(`UPDATE app.teaching_assignments SET effective_until=NULL WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject)
	exec(`UPDATE app.users SET active=false WHERE id=$1`, f.Teacher)
	if _, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("akun tidak aktif: %v", err)
	}
	exec(`UPDATE app.users SET active=true WHERE id=$1`, f.Teacher)
	exec(`UPDATE app.teachers SET active=false WHERE id=$1`, f.TeacherID)
	if _, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru tidak aktif: %v", err)
	}
	exec(`UPDATE app.teachers SET active=true WHERE id=$1`, f.TeacherID)
	preview, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE app.teachers SET name=name||' perubahan' WHERE id=$1`, f.TeacherID)
	stale := HistoricalAssessmentLinkConfirmation{Input: link, SnapshotToken: preview.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002115"}
	if _, err = s.ConfirmHistoricalAssessmentLink(ctx, f.Teacher, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("sumber berubah: %v", err)
	}
	preview, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	// FK tertunda wajib membatalkan audit yang tidak diikuti penyimpanan relasi.
	orphan, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = orphan.Exec(`INSERT INTO app.historical_assessment_objective_links(activity_id,learning_objective_id,provenance_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,historical_date,reason,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::date,$11::date,'uji','{}','{}','{}',decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'00000000-0000-4000-8000-000000002114')`, activity, tp, preview.ProvenanceID, preview.YearID, f.Semester, f.Class, f.Subject, preview.TeacherID, f.Teacher, preview.ActivityDate, preview.HistoricalDate)
	if err != nil {
		orphan.Rollback()
		t.Fatal(err)
	}
	if err = orphan.Commit(); err == nil {
		t.Fatal("audit tanpa relasi berhasil commit")
	}
	var orphanCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.historical_assessment_objective_links WHERE submission_key='00000000-0000-4000-8000-000000002114'`).Scan(&orphanCount); err != nil || orphanCount != 0 {
		t.Fatalf("audit parsial: %d %v", orphanCount, err)
	}
	req := HistoricalAssessmentLinkConfirmation{Input: link, SnapshotToken: preview.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002113"}
	exec(`UPDATE app.semester_assessment_weights SET nh=60,asts=20,asas=20 WHERE semester_id=$1`, f.Semester)
	if _, err = s.ConfirmHistoricalAssessmentLink(ctx, f.Teacher, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("token basi: %v", err)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=0,asts=40,asas=60 WHERE semester_id=$1`, f.Semester)
	preview, err = s.PreviewHistoricalAssessmentLink(ctx, f.Teacher, link, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	req.SnapshotToken = preview.SnapshotToken
	var wg sync.WaitGroup
	ids := make([]int64, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ids[i], errs[i] = s.ConfirmHistoricalAssessmentLink(ctx, f.Teacher, req)
		}(i)
	}
	close(start)
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || ids[0] != ids[1] {
		t.Fatalf("konkuren %v %v", ids, errs)
	}
	changed := req
	changed.Input.Method = "berbeda"
	if _, err = s.ConfirmHistoricalAssessmentLink(ctx, f.Teacher, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("replay berbeda: %v", err)
	}
	var links, audits, scores, finals int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.assessment_activity_objectives WHERE activity_id=$1 AND learning_objective_id=$2),(SELECT count(*) FROM app.historical_assessment_objective_links WHERE activity_id=$1 AND learning_objective_id=$2),(SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND learning_objective_id=$2),(SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$3)`, activity, tp, f.Semester).Scan(&links, &audits, &scores, &finals); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(links, audits, scores, finals) != "1 1 0 1" {
		t.Fatalf("integritas %d %d %d %d", links, audits, scores, finals)
	}
	if _, err = db.Exec(`UPDATE app.historical_assessment_objective_links SET reason='ubah' WHERE id=$1`, ids[0]); err == nil {
		t.Fatal("audit dapat diubah")
	}
	if _, err = db.Exec(`DELETE FROM app.historical_assessment_objective_links WHERE id=$1`, ids[0]); err == nil {
		t.Fatal("audit dapat dihapus")
	}
	var afterSnapshot string
	var afterHash []byte
	var oldScore string
	if err = db.QueryRow(`SELECT snapshot::text,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1`, f.Semester).Scan(&afterSnapshot, &afterHash); err != nil {
		t.Fatal(err)
	}
	if beforeSnapshot != afterSnapshot || !equalHash(beforeHash, afterHash) {
		t.Fatal("Final atau snapshot berubah")
	}
	if err = db.QueryRow(`SELECT score::text FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3`, activity, initial, f.StudentA).Scan(&oldScore); err != nil || oldScore != "0.00" {
		t.Fatalf("nilai TP awal: %q %v", oldScore, err)
	}
	review, err := s.SemesterFinalHistoricalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !review.NeedsReview {
		t.Fatalf("Final perlu ditinjau: %+v %v", review, err)
	}
	checkStatus := func(want string) {
		t.Helper()
		rec, e := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
		if e != nil {
			t.Fatal(e)
		}
		for _, st := range rec.Students {
			if st.ID == fmt.Sprint(f.StudentB) {
				if st.Status != want {
					t.Fatalf("status siswa B: %s, ingin %s", st.Status, want)
				}
				return
			}
		}
		t.Fatal("siswa B tidak ditemukan")
	}
	checkStatus("Siap Final")
	exec(`UPDATE app.semester_assessment_weights SET nh=60,asts=20,asas=20 WHERE semester_id=$1`, f.Semester)
	checkStatus("Belum Lengkap")
}
