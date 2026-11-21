//go:build integration

package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

// Semua fixture berada pada satu transaksi yang selalu dibatalkan. Fungsi Tx
// yang sama dipakai oleh service publik dengan transaksi read-only.
func TestHistoricalTPPreviewRollbackIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	suffix := testutil.Random(t)[:10]
	var year, sem, class, other, subject, user, teacher, activity int64
	insertID := func(dest *int64, q string, args ...any) {
		t.Helper()
		if e := tx.QueryRow(q, args...).Scan(dest); e != nil {
			t.Fatal(e)
		}
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := tx.Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	insertID(&year, `INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "CP1019-"+suffix)
	insertID(&sem, `INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, year)
	insertID(&class, `INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, year, "CP19A"+suffix)
	insertID(&other, `INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, year, "CP19B"+suffix)
	insertID(&subject, `INSERT INTO app.subjects(code,name) VALUES($1,$1) RETURNING id`, "CP19-"+suffix)
	insertID(&user, `INSERT INTO app.users(username,display_name,password_hash) VALUES($1,$1,'fixture') RETURNING id`, "cp19_"+suffix)
	exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'guru_mapel')`, user)
	insertID(&teacher, `INSERT INTO app.teachers(name,user_id) VALUES('Guru uji',$1) RETURNING id`, user)
	exec(`INSERT INTO app.teaching_assignments(semester_id,class_id,subject_id,teacher_id,effective_from,assigned_by) VALUES($1,$2,$3,$4,'2020-01-01',$5)`, sem, class, subject, teacher, user)
	exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, sem)
	exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,actor_id) VALUES($1,0,40,60,$2)`, sem, user)
	var objective int64
	insertID(&objective, `INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by) VALUES($1,$2,$3,$4,'TP awal',$5) RETURNING id`, year, sem, class, subject, user)
	insertID(&activity, `INSERT INTO app.assessment_activities(school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) VALUES($1,$2,$3,$4,$5,'Sumatif uji','sumatif','2020-02-01',$6) RETURNING id`, year, sem, class, subject, teacher, user)
	exec(`INSERT INTO app.assessment_activity_objectives(activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id) VALUES($1,$2,$3,$4,$5,$6)`, activity, objective, year, sem, class, subject)
	var assignment, revision, entry, lesson, journal int64
	if err = tx.QueryRow(`SELECT id FROM app.teaching_assignments WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, sem, class, subject).Scan(&assignment); err != nil {
		t.Fatal(err)
	}
	insertID(&revision, `INSERT INTO app.weekly_timetable_revisions(semester_id,revision_no,status,effective_from,created_by) VALUES($1,1,'draft','2020-01-01',$2) RETURNING id`, sem, user)
	insertID(&entry, `INSERT INTO app.weekly_timetable_entries(revision_id,weekday,class_id,subject_id,teacher_id,assignment_id,period_start,period_end) VALUES($1,1,$2,$3,$4,$5,1,1) RETURNING id`, revision, class, subject, teacher, assignment)
	insertID(&lesson, `INSERT INTO app.logical_lessons(semester_id,created_by) VALUES($1,$2) RETURNING id`, sem, user)
	// FK versi jurnal ditunda sampai commit; fixture selalu dibatalkan.
	insertID(&journal, `INSERT INTO app.teaching_journals(logical_lesson_id,meeting_date,semester_id,revision_id,timetable_entry_id,assignment_id,class_id,subject_id,teacher_id,owner_user_id,class_snapshot,subject_snapshot,teacher_snapshot) VALUES($1,'2020-01-10',$2,$3,$4,$5,$6,$7,$8,$9,'Kelas','Mapel','Guru uji') RETURNING id`, lesson, sem, revision, entry, assignment, class, subject, teacher, user)
	var first, movedOut int64
	for i := 0; i < 35; i++ {
		var student, placement int64
		insertID(&student, `INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, fmt.Sprintf("CP19 siswa %02d %s", i, suffix))
		if i == 0 {
			first = student
		}
		if i == 3 {
			movedOut = student
		}
		var starts any = "2020-01-01"
		if i == 1 || i == 24 {
			starts = "2020-03-01"
		}
		if i == 2 || i == 12 {
			starts = nil
		}
		initialClass := class
		if i == 4 {
			initialClass = other
		}
		insertID(&placement, `INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5) RETURNING id`, year, student, initialClass, user, starts)
		if i == 3 || i == 33 {
			exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,'2020-01-15','fixture',$4)`, placement, class, other, user)
		}
		if i == 4 {
			exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,'2020-03-01','fixture',$4)`, placement, other, class, user)
		}
	}
	hash := sha256.Sum256([]byte("fixture"))
	exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',$8,'00000000-0000-4000-8000-000000001919',$8)`, year, sem, class, subject, first, teacher, user, hash[:])
	exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',$8,'00000000-0000-4000-8000-000000001920',$8)`, year, sem, class, subject, movedOut, teacher, user, hash[:])
	in := HistoricalTPPreviewInput{SemesterID: sem, ClassID: class, SubjectID: subject, Definition: strings.Repeat("Tujuan pembelajaran é ", 30), Reason: "Koreksi historis", AssessmentDate: "2020-02-01", DateSource: "Arsip", Learning: HistoricalTPReference{Kind: "manual", Document: "Jurnal 1", Archive: "Lemari A", Teacher: "Guru", Date: "2020-01-10"}, Assessment: HistoricalTPReference{Kind: "app", SourceID: fmt.Sprint(activity)}}
	s := &Store{DB: db}
	fingerprint := func() string {
		t.Helper()
		var result string
		e := tx.QueryRow(`SELECT md5(jsonb_build_object(
		 'objectives',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM app.learning_objectives o WHERE o.semester_id=$1),
		 'activities',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM app.assessment_activities a WHERE a.semester_id=$1),
		 'scores',(SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM app.assessment_activity_initial_scores s WHERE s.semester_id=$1),
		 'finals',(SELECT jsonb_agg(to_jsonb(f) ORDER BY f.id) FROM app.semester_final_versions f WHERE f.semester_id=$1),
		 'placements',(SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id) FROM app.student_placements p WHERE p.school_year_id=$2))::text)`, sem, year).Scan(&result)
		if e != nil {
			t.Fatal(e)
		}
		return result
	}
	before := fingerprint()
	p, e := s.previewHistoricalTPTx(ctx, tx, user, in, 1, "")
	if e != nil {
		t.Fatal(e)
	}
	if p.FinalCount != 2 || p.PageCount != 4 || len(p.Students) != 10 || p.Weights.NH != 0 || p.LearningStatus != "Perlu Verifikasi Manual" || !strings.Contains(p.AssessmentStatus, "Metadata aplikasi cocok") {
		t.Fatalf("hasil: %+v", p)
	}
	if p.StudentCount != 35 || p.MemberCount != 28 || p.NonMemberCount != 5 || p.UnknownCount != 2 {
		t.Fatalf("jumlah seluruh halaman tidak cocok: total=%d anggota=%d bukan=%d belum=%d", p.StudentCount, p.MemberCount, p.NonMemberCount, p.UnknownCount)
	}
	appReference := in
	appReference.Learning = HistoricalTPReference{Kind: "app", SourceID: fmt.Sprint(journal)}
	appPreview, e := s.previewHistoricalTPTx(ctx, tx, user, appReference, 1, "")
	if e != nil || !strings.Contains(appPreview.LearningStatus, "Metadata aplikasi cocok") {
		t.Fatalf("jurnal aplikasi: %+v %v", appPreview, e)
	}
	for _, tc := range []struct {
		name     string
		input    HistoricalTPPreviewInput
		learning bool
	}{
		{"tanggal jurnal bertentangan", func() HistoricalTPPreviewInput { v := appReference; v.Learning.Date = "2020-01-11"; return v }(), true},
		{"guru jurnal bertentangan", func() HistoricalTPPreviewInput { v := appReference; v.Learning.Teacher = "Guru lain"; return v }(), true},
		{"guru sumatif bertentangan", func() HistoricalTPPreviewInput { v := in; v.Assessment.Teacher = "Guru lain"; return v }(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.previewHistoricalTPTx(ctx, tx, user, tc.input, 1, "")
			if err != nil {
				t.Fatal(err)
			}
			status := got.AssessmentStatus
			if tc.learning {
				status = got.LearningStatus
			}
			if status != "Referensi tidak memadai" {
				t.Fatalf("metadata bertentangan diterima: %q", status)
			}
		})
	}
	matching := appReference
	matching.Learning.Date, matching.Learning.Teacher, matching.Assessment.Teacher = "2020-01-10", " guru uji ", "GURU UJI"
	if got, err := s.previewHistoricalTPTx(ctx, tx, user, matching, 1, ""); err != nil || !strings.Contains(got.LearningStatus, "Metadata aplikasi cocok") || !strings.Contains(got.AssessmentStatus, "Metadata aplikasi cocok") {
		t.Fatalf("metadata cocok: %+v %v", got, err)
	}
	exec(`UPDATE app.teachers SET name='Guru baru' WHERE id=$1`, teacher)
	if _, err := s.previewHistoricalTPTx(ctx, tx, user, in, 2, p.SnapshotToken); !errors.Is(err, ErrConflict) {
		t.Fatalf("perubahan metadata guru harus membatalkan snapshot: %v", err)
	}
	exec(`UPDATE app.teachers SET name='Guru uji' WHERE id=$1`, teacher)
	mismatch := in
	mismatch.AssessmentDate = "2020-02-02"
	mismatchPreview, e := s.previewHistoricalTPTx(ctx, tx, user, mismatch, 1, "")
	if e != nil || mismatchPreview.AssessmentStatus != "Referensi tidak memadai" || mismatchPreview.Students[0].Relevance != "Belum dapat diverifikasi" {
		t.Fatalf("tanggal tidak cocok: %+v %v", mismatchPreview, e)
	}
	if p.Students[0].Relevance != "Kandidat anggota kelas" || p.Students[1].Relevance != "Bukan anggota kelas pada tanggal penilaian" || p.Students[2].Relevance != "Belum dapat diverifikasi" || p.Students[3].Relevance != "Bukan anggota kelas pada tanggal penilaian" || p.Students[3].FinalVersion != 1 || p.Students[4].Relevance != "Bukan anggota kelas pada tanggal penilaian" {
		t.Fatalf("klasifikasi: %+v", p.Students[:4])
	}
	if second, err := s.previewHistoricalTPTx(ctx, tx, user, in, 2, p.SnapshotToken); err != nil || second.StudentCount != 35 || second.MemberCount != 28 || second.NonMemberCount != 5 || second.UnknownCount != 2 {
		t.Fatalf("pagination dan jumlah seluruh siswa: %+v %v", second, err)
	}
	for page := 3; page <= 4; page++ {
		otherPage, err := s.previewHistoricalTPTx(ctx, tx, user, in, page, p.SnapshotToken)
		if err != nil || otherPage.StudentCount != 35 || otherPage.MemberCount != 28 || otherPage.NonMemberCount != 5 || otherPage.UnknownCount != 2 || otherPage.FinalCount != 2 {
			t.Fatalf("ringkasan halaman %d: %+v %v", page, otherPage, err)
		}
	}
	if _, e = s.previewHistoricalTPTx(ctx, tx, user, in, 1, p.SnapshotToken); e != nil {
		t.Fatalf("pagination kembali: %v", e)
	}
	if again, e := s.previewHistoricalTPTx(ctx, tx, user, in, 1, ""); e != nil || again.SnapshotToken != p.SnapshotToken {
		t.Fatalf("replay read-only berubah: %v", e)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=60,asts=20,asas=20 WHERE semester_id=$1`, sem)
	if _, e = s.previewHistoricalTPTx(ctx, tx, user, in, 2, p.SnapshotToken); !errors.Is(e, ErrConflict) {
		t.Fatalf("perubahan bobot harus membatalkan snapshot: %v", e)
	}
	positive, e := s.previewHistoricalTPTx(ctx, tx, user, in, 1, "")
	if e != nil || positive.Weights.NH != 60 {
		t.Fatalf("bobot positif: %+v %v", positive.Weights, e)
	}
	exec(`UPDATE app.semester_assessment_weights SET nh=0,asts=40,asas=60 WHERE semester_id=$1`, sem)
	missing := in
	missing.Assessment = HistoricalTPReference{Kind: "manual"}
	pMissing, e := s.previewHistoricalTPTx(ctx, tx, user, missing, 1, "")
	if e != nil || pMissing.AssessmentStatus != "Referensi tidak memadai" || pMissing.Students[0].Relevance != "Belum dapat diverifikasi" {
		t.Fatalf("referensi ambigu: %+v %v", pMissing, e)
	}
	if _, e = s.previewHistoricalTPTx(ctx, tx, user, in, 2, "stale"); !errors.Is(e, ErrConflict) {
		t.Fatalf("token basi: %v", e)
	}
	if _, e = s.previewHistoricalTPTx(ctx, tx, user+999999, in, 1, ""); !errors.Is(e, ErrForbidden) {
		t.Fatalf("aktor asing: %v", e)
	}
	in.Assessment.SourceID = "999999999"
	if _, e = s.previewHistoricalTPTx(ctx, tx, user, in, 1, ""); !errors.Is(e, ErrForbidden) {
		t.Fatalf("sumber asing: %v", e)
	}
	var count int
	if e = tx.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1`, sem).Scan(&count); e != nil || count != 2 {
		t.Fatalf("Final berubah: %d %v", count, e)
	}
	if after := fingerprint(); after != before {
		t.Fatal("pratinjau mengubah data akademik atau Final")
	}
}
