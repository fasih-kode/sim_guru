//go:build integration

package academic

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: tp, Value: "75"}}); err == nil {
		t.Fatal("nilai rutin untuk relasi historis diterima")
	}
	sheet, err := s.AssessmentActivityScoreSheet(ctx, f.Teacher, activity)
	if err != nil {
		t.Fatal(err)
	}
	foundLocked := false
	for _, student := range sheet.Students {
		for _, cell := range student.Scores {
			if cell.ObjectiveID == tp && cell.HistoricalLocked {
				foundLocked = true
			}
		}
	}
	if !foundLocked {
		t.Fatal("relasi historis tidak ditandai terkunci pada lembar nilai")
	}
	var placement int64
	if err = db.QueryRow(`SELECT id FROM app.student_placements WHERE student_id=$1 AND school_year_id=$2`, f.StudentA, year).Scan(&placement); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.assessment_activity_initial_scores(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'2020-01-11',75,$9)`, activity, tp, year, f.Semester, f.Class, f.Subject, f.StudentA, placement, f.Teacher); err == nil {
		t.Fatal("SQL langsung memasukkan nilai ke relasi historis")
	}
	// Migration 00055: uji himpunan multi-TP pada transaksi yang selalu dibatalkan.
	for _, incomplete := range []bool{false, true} {
		tx, beginErr := db.BeginTx(ctx, nil)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		var newActivity, auditID int64
		if err = tx.QueryRow(`SELECT nextval(pg_get_serial_sequence('app.assessment_activities','id'))`).Scan(&newActivity); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.QueryRow(`INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,'2020-01-11','uji',$8,'{}','{}','{}',decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),gen_random_uuid()) RETURNING id`, newActivity, year, f.Semester, f.Class, f.Subject, preview.TeacherID, f.Teacher, fmt.Sprint("uji-", newActivity)).Scan(&auditID); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO app.historical_assessment_activity_objectives VALUES($1,$2),($1,$3)`, auditID, initial, tp); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO app.assessment_activities(id,school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,'Uji historis','sumatif','2020-01-11',$7)`, newActivity, year, f.Semester, f.Class, f.Subject, preview.TeacherID, f.Teacher); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO app.assessment_activity_objectives VALUES($1,$2,$3,$4,$5,$6)`, newActivity, initial, year, f.Semester, f.Class, f.Subject); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if !incomplete {
			if _, err = tx.Exec(`INSERT INTO app.assessment_activity_objectives VALUES($1,$2,$3,$4,$5,$6)`, newActivity, tp, year, f.Semester, f.Class, f.Subject); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
		}
		_, err = tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`)
		if incomplete && err == nil {
			tx.Rollback()
			t.Fatal("himpunan TP tidak lengkap diterima")
		}
		if !incomplete && err != nil {
			tx.Rollback()
			t.Fatalf("himpunan TP lengkap ditolak: %v", err)
		}
		if !incomplete {
			if _, err = tx.Exec(`INSERT INTO app.assessment_activity_initial_scores(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'2020-01-11',75,$9)`, newActivity, initial, year, f.Semester, f.Class, f.Subject, f.StudentA, placement, f.Teacher); err == nil {
				tx.Rollback()
				t.Fatal("SQL langsung memasukkan nilai kegiatan historis")
			}
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			t.Fatal(rollbackErr)
		}
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

	// CP10-22 service: kegiatan historis dapat disimpan tanpa membuat nilai,
	// dengan semua TP dipilih dan duplikat nama/tanggal ditinjau secara eksplisit.
	activityInput := HistoricalAssessmentActivityInput{
		SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject,
		ObjectiveIDs: []int64{initial, tp}, Name: "Sumatif lama", ActivityDate: "2020-01-11",
		Reason: "Kegiatan arsip terlewat saat pencatatan", SourceIdentity: "arsip-sumatif-cp22-" + testutil.Random(t)[:8],
		SourceDocument: "Daftar Sumatif semester 1", SourceArchive: "Arsip akademik lemari A",
		SourceTeacher: "Guru pemilik kegiatan", SourceSection: "Halaman dan butir penilaian TP",
		ResultSource: "Lembar hasil bertanda", Method: "Mencocokkan dokumen sumber",
		DuplicateChecked: true, DuplicateExplanation: "Nama dan tanggal sama; identitas dokumen sumber berbeda.", Acknowledged: true,
	}
	// Lebih dari satu halaman memastikan ringkasan memakai keseluruhan roster.
	for i := 0; i < 11; i++ {
		var student int64
		if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, fmt.Sprintf("CP22 pagination %d %s", i, testutil.Random(t)[:6])).Scan(&student); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01')`, year, student, f.Class, f.Teacher)
	}
	if _, duplicateErr := s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, func() HistoricalAssessmentActivityInput { x := activityInput; x.DuplicateChecked = false; return x }(), 1, ""); duplicateErr == nil {
		t.Fatal("kandidat nama/tanggal mirip diterima tanpa pemeriksaan sumber")
	}
	activityPreview, activityErr := s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, activityInput, 1, "")
	if activityErr != nil {
		t.Fatal("pratinjau kegiatan historis:", activityErr)
	}
	if activityPreview.FinalCount != 1 || len(activityPreview.Objectives) != 2 || activityPreview.NumericImpactKnown || activityPreview.StudentCount < 12 || activityPreview.PageCount < 2 || len(activityPreview.Students) != 10 {
		t.Fatalf("ringkasan pratinjau: %+v", activityPreview)
	}
	secondPage, pageErr := s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, activityInput, 2, activityPreview.SnapshotToken)
	if pageErr != nil || secondPage.StudentCount != activityPreview.StudentCount || secondPage.MemberCount != activityPreview.MemberCount || secondPage.SnapshotToken != activityPreview.SnapshotToken || len(secondPage.Students) == 0 {
		t.Fatalf("pagination mengubah ringkasan/fingerprint: page=%+v err=%v", secondPage, pageErr)
	}
	if activityPreview.ImpactNotice == "" {
		t.Fatal("dampak numerik yang belum diketahui harus dijelaskan")
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=0,asts=40,asas=60 WHERE semester_id=$1`, f.Semester)
	zeroWeightPreview, zeroWeightErr := s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, activityInput, 1, "")
	if zeroWeightErr != nil || zeroWeightPreview.Weights.NH != 0 || !strings.Contains(zeroWeightPreview.ImpactNotice, "0%") {
		t.Fatalf("NH 0%% pada pratinjau CP10-22: %+v %v", zeroWeightPreview, zeroWeightErr)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=60,asts=20,asas=20 WHERE semester_id=$1`, f.Semester)
	// Tanggal luar kalender hanya diterima dengan alasan khusus dan tetap
	// mensyaratkan assignment yang berlaku pada tanggal kegiatan sebenarnya.
	exec(`UPDATE app.teaching_assignments SET effective_from='2019-01-01' WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject)
	outsideInput := activityInput
	outsideInput.Name, outsideInput.ActivityDate, outsideInput.SourceIdentity = "Sumatif arsip tanggal lama", "2019-12-31", activityInput.SourceIdentity+"-outside"
	outsideInput.DuplicateChecked, outsideInput.DuplicateExplanation = false, ""
	outsideInput.OutsideCalendarReason = "Tanggal pada arsip sah berada di luar kalender yang dicatat."
	outsidePreview, outsideErr := s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, outsideInput, 1, "")
	if outsideErr != nil || !outsidePreview.OutsideCalendar {
		t.Fatalf("kegiatan di luar kalender beralasan: %+v %v", outsidePreview, outsideErr)
	}
	futureInput := activityInput
	futureInput.ActivityDate = "2030-01-01"
	if _, futureErr := s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, futureInput, 1, ""); futureErr == nil {
		t.Fatal("tanggal masa depan diterima")
	}
	activityReq := HistoricalAssessmentActivityConfirmation{Input: activityInput, SnapshotToken: activityPreview.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002401"}
	// Perubahan bobot setelah pratinjau membuat token basi.
	exec(`UPDATE app.semester_assessment_weights SET nh=50,asts=25,asas=25 WHERE semester_id=$1`, f.Semester)
	if _, _, staleErr := s.ConfirmHistoricalAssessmentActivity(ctx, f.Teacher, activityReq); !errors.Is(staleErr, ErrConflict) {
		t.Fatalf("snapshot kegiatan basi: %v", staleErr)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=60,asts=20,asas=20 WHERE semester_id=$1`, f.Semester)
	activityPreview, activityErr = s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, activityInput, 1, "")
	if activityErr != nil {
		t.Fatal("pratinjau ulang kegiatan historis:", activityErr)
	}
	activityReq.SnapshotToken = activityPreview.SnapshotToken
	var confirmWG sync.WaitGroup
	var auditIDs, activityIDs [2]int64
	var confirmErrs [2]error
	confirmStart := make(chan struct{})
	for i := range auditIDs {
		confirmWG.Add(1)
		go func(i int) {
			defer confirmWG.Done()
			<-confirmStart
			auditIDs[i], activityIDs[i], confirmErrs[i] = s.ConfirmHistoricalAssessmentActivity(ctx, f.Teacher, activityReq)
		}(i)
	}
	close(confirmStart)
	confirmWG.Wait()
	if confirmErrs[0] != nil || confirmErrs[1] != nil || auditIDs[0] != auditIDs[1] || activityIDs[0] != activityIDs[1] {
		t.Fatalf("konfirmasi bersamaan CP10-22: audit=%v activity=%v errors=%v", auditIDs, activityIDs, confirmErrs)
	}
	auditID, activityID := auditIDs[0], activityIDs[0]
	replayAudit, replayActivity, replayErr := s.ConfirmHistoricalAssessmentActivity(ctx, f.Teacher, activityReq)
	if replayErr != nil || replayAudit != auditID || replayActivity != activityID {
		t.Fatalf("replay CP10-22: audit=%d activity=%d err=%v", replayAudit, replayActivity, replayErr)
	}
	storedResult, resultErr := s.HistoricalAssessmentActivityResult(ctx, f.Teacher, auditID)
	if resultErr != nil || storedResult.ActivityID != activityID || storedResult.AuditID != auditID || len(storedResult.Preview.Students) != activityPreview.StudentCount {
		t.Fatalf("hasil audit pembuat: %+v %v", storedResult, resultErr)
	}
	if _, resultErr = s.HistoricalAssessmentActivityResult(ctx, f.Other, auditID); !errors.Is(resultErr, ErrForbidden) {
		t.Fatalf("hasil audit dibaca guru lain: %v", resultErr)
	}
	changedActivityReq := activityReq
	changedActivityReq.Input.Reason = "Payload berbeda"
	if _, _, changedErr := s.ConfirmHistoricalAssessmentActivity(ctx, f.Teacher, changedActivityReq); !errors.Is(changedErr, ErrConflict) {
		t.Fatalf("replay CP10-22 dengan payload berbeda: %v", changedErr)
	}
	var activityAudits, activityRelations, activityScores int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.historical_assessment_activity_insertions WHERE id=$1),(SELECT count(*) FROM app.assessment_activity_objectives WHERE activity_id=$2),(SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$2)`, auditID, activityID).Scan(&activityAudits, &activityRelations, &activityScores); err != nil {
		t.Fatal(err)
	}
	if activityAudits != 1 || activityRelations != 2 || activityScores != 0 {
		t.Fatalf("integritas CP10-22 audit=%d relasi=%d nilai=%d", activityAudits, activityRelations, activityScores)
	}
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activityID, []AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: tp, Value: "75"}}); err == nil {
		t.Fatal("service nilai rutin menerima pasangan kegiatan CP10-22")
	}
	if _, err = db.Exec(`INSERT INTO app.assessment_activity_initial_scores(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'2020-01-11',75,$9)`, activityID, tp, year, f.Semester, f.Class, f.Subject, f.StudentA, placement, f.Teacher); err == nil {
		t.Fatal("SQL langsung menerima nilai pada kegiatan CP10-22")
	}
	if _, duplicateErr := s.PreviewHistoricalAssessmentActivity(ctx, f.Teacher, activityInput, 1, ""); !errors.Is(duplicateErr, ErrConflict) {
		t.Fatalf("identitas sumber yang sudah terbukti ganda: %v", duplicateErr)
	}
	var finalSnapshot string
	var finalHash []byte
	if err = db.QueryRow(`SELECT snapshot::text,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 ORDER BY version DESC LIMIT 1`, f.Semester, f.StudentA).Scan(&finalSnapshot, &finalHash); err != nil {
		t.Fatal(err)
	}
	if finalSnapshot != beforeSnapshot || !equalHash(finalHash, beforeHash) {
		t.Fatal("Final atau snapshot berubah saat menyimpan kegiatan historis")
	}
}
