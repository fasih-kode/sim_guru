//go:build integration

package academic

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

// Perubahan metadata sumber dilakukan lewat service master dan commit pada
// transaksi terpisah sebelum permintaan konfirmasi memakai token lama.
func TestHistoricalTPSourceChangeBetweenTransactionsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "cp20source"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester)
	exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,actor_id) VALUES($1,0,40,60,$2)`, f.Semester, f.Teacher)
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP sebelum Final"); err != nil {
		t.Fatal(err)
	}
	var year, objective int64
	if err := db.QueryRow(`SELECT o.school_year_id,o.id FROM app.learning_objectives o WHERE o.semester_id=$1 AND o.class_id=$2 AND o.subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&year, &objective); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif sumber", "sumatif", "2020-01-10", []int64{objective}); err != nil {
		t.Fatal(err)
	}
	var activity int64
	if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND name='Sumatif sumber'`, f.Semester, f.Class).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("fixture sumber"))
	exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',$8,'00000000-0000-4000-8000-000000002061',$8)`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher, digest[:])
	in := HistoricalTPPreviewInput{SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject, Definition: "TP sumber", Reason: "perbaikan", AssessmentDate: "2020-01-10", DateSource: "kegiatan aplikasi", Learning: HistoricalTPReference{Kind: "manual", Document: "jurnal", Archive: "lemari", Teacher: "guru", Date: "2020-01-09"}, Assessment: HistoricalTPReference{Kind: "app", SourceID: fmt.Sprint(activity)}}
	preview, err := s.PreviewHistoricalTP(ctx, f.Teacher, in, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	req := HistoricalTPConfirmation{Preview: in, Verification: HistoricalTPVerification{LearningSection: "halaman 1", AssessmentSection: "halaman 2", Method: "diperiksa", ResultSource: "lembar hasil", Acknowledged: true}, SnapshotToken: preview.SnapshotToken, SubmissionKey: "00000000-0000-4000-8000-000000002062"}
	admin, _ := testutil.User(t, db, "admin")
	var version int64
	if err = db.QueryRow(`SELECT version FROM app.teachers WHERE id=$1`, f.TeacherID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	_, err = (&master.Store{DB: db}).Save(ctx, admin.ID, master.Teachers, f.TeacherID, master.Input{Record: master.Record{Name: "Guru sumber berubah", UserID: f.Teacher, Active: true, Version: version}, Reason: "Koreksi identitas fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmHistoricalLearningObjective(ctx, f.Teacher, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("metadata sumber berubah harus konflik: %v", err)
	}
	var objectives, audits int
	var snapshot string
	var storedHash []byte
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.learning_objectives WHERE semester_id=$1),(SELECT count(*) FROM app.historical_learning_objective_insertions WHERE semester_id=$1),(SELECT snapshot::text FROM app.semester_final_versions WHERE semester_id=$1),(SELECT snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1)`, f.Semester).Scan(&objectives, &audits, &snapshot, &storedHash); err != nil {
		t.Fatal(err)
	}
	if objectives != 1 || audits != 0 || snapshot != "{}" || !equalHash(storedHash, digest[:]) {
		t.Fatalf("perubahan parsial: objectives=%d audits=%d snapshot=%s", objectives, audits, snapshot)
	}
}
