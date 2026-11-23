//go:build integration

package academic

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestDirectTPFinalLegacyEvidenceCompatibilityIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	fixture := testutil.FixtureDatabase(t)
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, fixture, "r3legacy"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	fixtureStore := &Store{DB: fixture}
	if _, err := fixture.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id) VALUES($1,0,40,60,1,$2)`, f.Semester, f.Teacher); err != nil {
		t.Fatal(err)
	}
	if err := fixtureStore.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"}}); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := fixture.QueryRow(semesterFinalEvidenceQuery, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var evidence map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
		t.Fatal(err)
	}
	delete(evidence, "direct_tp_scores")
	evidenceBytes, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	snapshotBytes, err := json.Marshal(semesterFinalSnapshot{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, Student: SemesterRecommendationStudent{ID: fmtID(f.StudentA), Status: "Siap Final", Recommendation: "82"}, Evidence: evidenceBytes})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(snapshotBytes)
	_, err = fixture.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) SELECT school_year_id,$1,$2,$3,$4,$5,$6,1,82,82,$7::jsonb,$8,gen_random_uuid(),decode(repeat('cd',32),'hex') FROM app.semesters WHERE id=$1`, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher, string(snapshotBytes), hash[:])
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Finalized || preview.NeedsReview {
		t.Fatalf("format lama tidak boleh memicu tinjauan: %+v", preview)
	}
	recommendations, err := s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range recommendations.Students {
		if item.ID == fmtID(f.StudentA) {
			found = true
			if item.NeedsReview || item.Status != "Final" {
				t.Fatalf("daftar Final lama: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("Final lama tidak tampil")
	}
	// Format tanpa salah satu kelompok bukti harus terlihat sebagai belum
	// terverifikasi dan tidak boleh dipakai untuk menerbitkan keputusan baru.
	if err = fixture.QueryRow(semesterFinalEvidenceQuery, f.Semester, f.Class, f.Subject, f.StudentB).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(raw), &evidence); err != nil {
		t.Fatal(err)
	}
	delete(evidence, "direct_tp_scores")
	delete(evidence, "activities")
	partialEvidence, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	partialSnapshot, err := json.Marshal(semesterFinalSnapshot{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, Student: SemesterRecommendationStudent{ID: fmtID(f.StudentB)}, Evidence: partialEvidence})
	if err != nil {
		t.Fatal(err)
	}
	partialHash := sha256.Sum256(partialSnapshot)
	_, err = fixture.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) SELECT school_year_id,$1,$2,$3,$4,$5,$6,1,55,55,$7::jsonb,$8,gen_random_uuid(),decode(repeat('cd',32),'hex') FROM app.semesters WHERE id=$1`, f.Semester, f.Class, f.Subject, f.StudentB, f.TeacherID, f.Teacher, string(partialSnapshot), partialHash[:])
	if err != nil {
		t.Fatal(err)
	}
	uncertain, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB)
	if err != nil || !uncertain.Finalized || !uncertain.ReviewUncertain || uncertain.NeedsReview {
		t.Fatalf("bukti tidak lengkap: %+v %v", uncertain, err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 1, 55, "Retensi uji", uncertain.SnapshotToken, "00000000-0000-4000-8000-000000000099", "retain"); err == nil {
		t.Fatal("review bukti tak terverifikasi harus ditolak")
	}
	if _, err = fixture.Exec(`UPDATE app.semester_calendars SET ends_on='2020-12-30' WHERE semester_id=$1`, f.Semester); err != nil {
		t.Fatal(err)
	}
	preview, err = s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.NeedsReview {
		t.Fatal("perubahan kalender relevan harus terdeteksi")
	}
	recommendations, err = s.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range recommendations.Students {
		if item.ID == fmtID(f.StudentA) && !item.NeedsReview {
			t.Fatal("daftar harus menandai perubahan bukti")
		}
	}
	var current []byte
	if err = fixture.QueryRow(`SELECT snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&current); err != nil || !equalHash(current, hash[:]) {
		t.Fatalf("hash histori berubah: %v", err)
	}
}

func TestDirectTPNHAndFinalIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	fixture := testutil.FixtureDatabase(t)
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, fixture, "rd02"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	fixtureStore := &Store{DB: fixture}
	if _, err := fixture.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id) VALUES($1,60,20,20,1,$2)`, f.Semester, f.Teacher); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TP dasar satu", "TP dasar dua"} {
		if err := fixtureStore.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, name); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := fixture.Query(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 ORDER BY id`, f.Semester)
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
	rows.Close()
	if len(ids) != 2 {
		t.Fatalf("TP: %v", ids)
	}
	key := func() string {
		x := testutil.Random(t)
		return fmt.Sprintf("%s-%s-4%s-8%s-%s", x[:8], x[8:12], x[12:15], x[15:18], x[18:30])
	}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, key(), []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: ids[0], ExpectedVersion: -1, Value: "0"}, {StudentID: f.StudentB, ObjectiveID: ids[0], ExpectedVersion: -1, Value: "75"}}); err != nil {
		t.Fatal(err)
	}
	before, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil {
		t.Fatal(err)
	}
	if before.Student.Status != "Belum Lengkap" || !before.CanFinalizeException || before.Student.NH != "" {
		t.Fatalf("nilai nol harus menjadi bukti parsial: %+v", before)
	}
	if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, key(), []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: ids[1], ExpectedVersion: -1, Value: "100"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 70, "Alasan uji", before.SnapshotToken, key()); !errors.Is(err, ErrConflict) {
		t.Fatalf("snapshot basi: %v", err)
	}
	if err = fixtureStore.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "70"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "90"}}); err != nil {
		t.Fatal(err)
	}
	var batchStudents []int64
	for i := 0; i < 2; i++ {
		var id int64
		if err = fixture.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, fmt.Sprintf("RD02 batch %d %s", i, testutil.Random(t)[:8])).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) SELECT school_year_id,$2,$3,$4,'2020-01-01' FROM app.semesters WHERE id=$1`, f.Semester, id, f.Class, f.Teacher); err != nil {
			t.Fatal(err)
		}
		if err = s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, key(), []DirectTPScoreInput{{StudentID: id, ObjectiveID: ids[0], ExpectedVersion: -1, Value: "80"}, {StudentID: id, ObjectiveID: ids[1], ExpectedVersion: -1, Value: "80"}}); err != nil {
			t.Fatal(err)
		}
		if err = fixtureStore.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: id, Component: "asts", Date: "2020-01-10", Value: "80"}, {StudentID: id, Component: "asas", Date: "2020-01-10", Value: "80"}}); err != nil {
			t.Fatal(err)
		}
		batchStudents = append(batchStudents, id)
	}
	preview, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Student.NH != "50,00" || preview.Student.Recommendation != "62" || !preview.CanFinalize {
		t.Fatalf("rekomendasi: %+v", preview)
	}
	k := key()
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 62, "", preview.SnapshotToken, k); err != nil {
		t.Fatal(err)
	}
	var originalSnapshot, originalHash string
	if err = fixture.QueryRow(`SELECT snapshot::text,encode(snapshot_sha256,'hex') FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 AND version=1`, f.Semester, f.StudentA).Scan(&originalSnapshot, &originalHash); err != nil {
		t.Fatal(err)
	}
	latest, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil || !latest.Finalized || latest.NeedsReview {
		t.Fatalf("Final baru harus stabil: %+v %v", latest, err)
	}
	retentionKey := key()
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 62, "Retensi uji", latest.SnapshotToken, retentionKey, "retain"); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 63, "Revisi uji", latest.SnapshotToken, key(), "revision"); err != nil {
		t.Fatal(err)
	}
	var retainedSnapshot, retainedHash string
	if err = fixture.QueryRow(`SELECT snapshot::text,encode(snapshot_sha256,'hex') FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2 AND version=1`, f.Semester, f.StudentA).Scan(&retainedSnapshot, &retainedHash); err != nil || retainedSnapshot != originalSnapshot || retainedHash != originalHash {
		t.Fatalf("snapshot Final versi lama berubah: %v", err)
	}
	if err = s.ReviewSemesterFinal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 1, 62, "Retensi uji", latest.SnapshotToken, retentionKey, "retain"); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA, 62, "", preview.SnapshotToken, k); err != nil {
		t.Fatal(err)
	}
	var snapshot string
	var count int
	if err = fixture.QueryRow(`SELECT count(*),max(snapshot::text) FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=$2`, f.Semester, f.StudentA).Scan(&count, &snapshot); err != nil || count != 2 || !strings.Contains(snapshot, "direct_tp_scores") {
		t.Fatalf("snapshot %d %s %v", count, snapshot, err)
	}
	b, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB)
	if err != nil {
		t.Fatal(err)
	}
	if !b.CanFinalizeException {
		t.Fatalf("bukti TP parsial B: %+v", b)
	}
	if err = s.FinalizeSemesterException(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentB, 75, "Bukti nilai langsung", b.SnapshotToken, key()); err != nil {
		t.Fatal(err)
	}
	items := []SemesterFinalBatchItem{{StudentID: batchStudents[0]}, {StudentID: batchStudents[1]}}
	batch, err := s.SemesterFinalBatchPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, items)
	if err != nil {
		t.Fatal(err)
	}
	staleBatch := append([]SemesterFinalBatchItem(nil), batch.Items...)
	staleBatch[0].SnapshotToken = strings.Repeat("0", 64)
	if err = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, staleBatch, key()); !errors.Is(err, ErrConflict) {
		t.Fatalf("batch basi harus ditolak: %v", err)
	}
	var partial int
	if err = fixture.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND student_id=ANY($2)`, f.Semester, batchStudents).Scan(&partial); err != nil || partial != 0 {
		t.Fatalf("batch basi menulis sebagian: %d %v", partial, err)
	}
	batchKey := key()
	if err = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, batch.Items, batchKey); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSemesterBatch(ctx, f.Teacher, f.Semester, f.Class, f.Subject, batch.Items, batchKey); err != nil {
		t.Fatal(err)
	}
}

func TestDirectTPNHZeroWeightIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	fixture := testutil.FixtureDatabase(t)
	f := testutil.ExamFixtureFor(t, fixture, "nhzero"+testutil.Random(t)[:8])
	ctx := context.Background()
	setup := &Store{DB: fixture}
	runtime := &Store{DB: db}
	if _, err := fixture.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id) VALUES($1,0,40,60,1,$2)`, f.Semester, f.Teacher); err != nil {
		t.Fatal(err)
	}
	if err := setup.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP kosong"); err != nil {
		t.Fatal(err)
	}
	if err := setup.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, []ExamInitialInput{{StudentID: f.StudentA, Component: "asts", Date: "2020-01-10", Value: "0"}, {StudentID: f.StudentA, Component: "asas", Date: "2020-01-10", Value: "100"}}); err != nil {
		t.Fatal(err)
	}
	p, err := runtime.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, f.StudentA)
	if err != nil {
		t.Fatal(err)
	}
	if p.Student.NH != "" || p.Student.Recommendation != "60" || !p.CanFinalize {
		t.Fatalf("NH nol persen tidak boleh menahan Final: %+v", p)
	}
}
