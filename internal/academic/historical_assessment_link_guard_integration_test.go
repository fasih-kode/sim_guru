//go:build integration

package academic

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
	"github.com/fasih-kode/sim_guru/migrations"
)

// Seluruh perubahan fixture dibatalkan agar histori TEST tidak terhapus.
func TestHistoricalAssessmentLinkGuardIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	provider, err := migrations.Provider(db)
	if err != nil {
		t.Fatal(err)
	}
	version, err := provider.GetDBVersion(t.Context())
	if err != nil || version != 60 {
		t.Fatalf("TEST harus berada pada migration 60/60, aktual=%d err=%v", version, err)
	}
	tx, err := db.BeginTx(t.Context(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var year, semester, class, subject, actor, teacher, student, tp, tp2, tp3, tp4, sumatif, formatif int64
	id := func(dest *int64, q string, args ...any) {
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
	suffix := testutil.Random(t)[:10]
	id(&year, `INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "CP21-"+suffix)
	id(&semester, `INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, year)
	id(&class, `INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, year, "CP21-"+suffix)
	id(&subject, `INSERT INTO app.subjects(code,name) VALUES($1,$1) RETURNING id`, "CP21-"+suffix)
	id(&actor, `INSERT INTO app.users(username,display_name,password_hash) VALUES($1,$1,'fixture') RETURNING id`, "cp21_"+suffix)
	id(&teacher, `INSERT INTO app.teachers(name,user_id) VALUES('Guru uji',$1) RETURNING id`, actor)
	id(&student, `INSERT INTO app.students(name,birth_date,gender) VALUES('Siswa uji','2010-01-01','L') RETURNING id`)
	id(&tp, `INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by) VALUES($1,$2,$3,$4,'TP awal',$5) RETURNING id`, year, semester, class, subject, actor)
	id(&tp2, `INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by) VALUES($1,$2,$3,$4,'TP kedua',$5) RETURNING id`, year, semester, class, subject, actor)
	id(&tp3, `INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by) VALUES($1,$2,$3,$4,'TP ketiga',$5) RETURNING id`, year, semester, class, subject, actor)
	id(&tp4, `INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by) VALUES($1,$2,$3,$4,'TP keempat',$5) RETURNING id`, year, semester, class, subject, actor)
	id(&sumatif, `INSERT INTO app.assessment_activities(school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) VALUES($1,$2,$3,$4,$5,'Sumatif','sumatif','2020-01-10',$6) RETURNING id`, year, semester, class, subject, teacher, actor)
	exec(`INSERT INTO app.assessment_activity_objectives(activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id) VALUES($1,$2,$3,$4,$5,$6)`, sumatif, tp, year, semester, class, subject)
	hash := sha256.Sum256([]byte("fixture"))
	exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',$8,'00000000-0000-4000-8000-000000002101',$8)`, year, semester, class, subject, student, teacher, actor, hash[:])
	id(&formatif, `INSERT INTO app.assessment_activities(school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) VALUES($1,$2,$3,$4,$5,'Formatif','formatif','2020-01-10',$6) RETURNING id`, year, semester, class, subject, teacher, actor)
	exec(`INSERT INTO app.assessment_activity_objectives(activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id) VALUES($1,$2,$3,$4,$5,$6)`, formatif, tp, year, semester, class, subject)
	exec(`SAVEPOINT guard_check`)
	if _, err = tx.Exec(`INSERT INTO app.assessment_activity_objectives(activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id) VALUES($1,$2,$3,$4,$5,$6)`, sumatif, tp2, year, semester, class, subject); err == nil {
		t.Fatal("relasi Sumatif setelah Final dapat ditambah tanpa audit")
	}
	exec(`ROLLBACK TO SAVEPOINT guard_check`)
	if _, err = tx.Exec(`DELETE FROM app.assessment_activity_objectives WHERE activity_id=$1 AND learning_objective_id=$2`, sumatif, tp); err == nil {
		t.Fatal("relasi lama setelah Final dapat dihapus")
	}
	exec(`ROLLBACK TO SAVEPOINT guard_check`)

	// Migration 00055/00057: kegiatan pasca-Final hanya dapat disimpan bersama
	// audit dan himpunan TP yang persis sama dalam satu transaksi.
	newActivity := func(source string, submission string, withSecond bool) int64 {
		t.Helper()
		if _, e := tx.Exec(`SET CONSTRAINTS ALL DEFERRED`); e != nil {
			t.Fatal(e)
		}
		var activityID, auditID int64
		if e := tx.QueryRow(`SELECT nextval(pg_get_serial_sequence('app.assessment_activities','id'))`).Scan(&activityID); e != nil {
			t.Fatal(e)
		}
		if e := tx.QueryRow(`INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,'2020-01-10','uji', $8,'{}','{}','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),$9::uuid) RETURNING id`, activityID, year, semester, class, subject, teacher, actor, source, submission).Scan(&auditID); e != nil {
			t.Fatal(e)
		}
		if _, e := tx.Exec(`INSERT INTO app.assessment_activities(id,school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,'Sumatif historis uji','sumatif','2020-01-10',$7)`, activityID, year, semester, class, subject, teacher, actor); e != nil {
			t.Fatal(e)
		}
		if _, e := tx.Exec(`INSERT INTO app.historical_assessment_activity_objectives VALUES($1,$2),($1,$3)`, auditID, tp, tp2); e != nil {
			t.Fatal(e)
		}
		if withSecond {
			if _, e := tx.Exec(`INSERT INTO app.historical_assessment_activity_objectives VALUES($1,$2)`, auditID, tp3); e != nil {
				t.Fatal(e)
			}
		}
		var detailCount int
		if e := tx.QueryRow(`SELECT count(*) FROM app.historical_assessment_activity_insertions h JOIN app.historical_assessment_activity_objectives d ON d.audit_id=h.id WHERE h.activity_id=$1 AND h.transaction_id=txid_current() AND d.learning_objective_id=$2`, activityID, tp).Scan(&detailCount); e != nil || detailCount != 1 {
			t.Fatalf("audit detail sebelum relasi: count=%d err=%v", detailCount, e)
		}
		if _, e := tx.Exec(`INSERT INTO app.assessment_activity_objectives VALUES($1,$2,$3,$4,$5,$6),($1,$7,$3,$4,$5,$6)`, activityID, tp, year, semester, class, subject, tp2); e != nil {
			t.Fatal(e)
		}
		if withSecond {
			if _, e := tx.Exec(`INSERT INTO app.assessment_activity_objectives VALUES($1,$2,$3,$4,$5,$6)`, activityID, tp3, year, semester, class, subject); e != nil {
				t.Fatal(e)
			}
		}
		return activityID
	}

	txSavepoint := func(name string) {
		t.Helper()
		if _, e := tx.Exec("SAVEPOINT " + name); e != nil {
			t.Fatal(e)
		}
	}
	txRollbackTo := func(name string) {
		t.Helper()
		if _, e := tx.Exec("ROLLBACK TO SAVEPOINT " + name); e != nil {
			t.Fatal(e)
		}
	}

	// INSERT kegiatan langsung setelah Final tanpa audit ditolak.
	txSavepoint("no_audit")
	if _, e := tx.Exec(`INSERT INTO app.assessment_activities(school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) VALUES($1,$2,$3,$4,$5,'Tanpa audit','sumatif','2020-01-10',$6)`, year, semester, class, subject, teacher, actor); e == nil {
		t.Fatal("kegiatan historis tanpa audit diterima")
	}
	txRollbackTo("no_audit")
	var noAuditRows int
	if e := tx.QueryRow(`SELECT count(*) FROM app.assessment_activities WHERE school_year_id=$1 AND semester_id=$2 AND class_id=$3 AND subject_id=$4 AND name='Tanpa audit'`, year, semester, class, subject).Scan(&noAuditRows); e != nil || noAuditRows != 0 {
		t.Fatalf("penolakan kegiatan meninggalkan baris: %d %v", noAuditRows, e)
	}

	// Audit tanpa kegiatan gagal pada constraint FK/kelengkapan saat dipaksa.
	txSavepoint("orphan_audit")
	var orphanActivity int64
	if e := tx.QueryRow(`SELECT nextval(pg_get_serial_sequence('app.assessment_activities','id'))`).Scan(&orphanActivity); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(`INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,'2020-01-10','uji','orphan','{}','{}','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),gen_random_uuid())`, orphanActivity, year, semester, class, subject, teacher, actor); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`); e == nil {
		t.Fatal("audit tanpa kegiatan lolos constraint")
	}
	txRollbackTo("orphan_audit")
	var orphanRows int
	if e := tx.QueryRow(`SELECT (SELECT count(*) FROM app.historical_assessment_activity_insertions WHERE activity_id=$1) + (SELECT count(*) FROM app.assessment_activities WHERE id=$1)`, orphanActivity).Scan(&orphanRows); e != nil || orphanRows != 0 {
		t.Fatalf("rollback audit orphan meninggalkan data: %d %v", orphanRows, e)
	}

	// Satu relasi dari dua TP tidak dapat commit.
	txSavepoint("partial_set")
	var partialID, partialAudit int64
	if e := tx.QueryRow(`SELECT nextval(pg_get_serial_sequence('app.assessment_activities','id'))`).Scan(&partialID); e != nil {
		t.Fatal(e)
	}
	if e := tx.QueryRow(`INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,'2020-01-10','uji','partial','{}','{}','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),gen_random_uuid()) RETURNING id`, partialID, year, semester, class, subject, teacher, actor).Scan(&partialAudit); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(`INSERT INTO app.assessment_activities(id,school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,'Partial','sumatif','2020-01-10',$7)`, partialID, year, semester, class, subject, teacher, actor); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(`INSERT INTO app.historical_assessment_activity_objectives VALUES($1,$2),($1,$3)`, partialAudit, tp, tp2); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(`INSERT INTO app.assessment_activity_objectives VALUES($1,$2,$3,$4,$5,$6)`, partialID, tp, year, semester, class, subject); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`); e == nil {
		t.Fatal("himpunan relasi parsial diterima")
	}
	txRollbackTo("partial_set")
	var partialRows int
	if e := tx.QueryRow(`SELECT (SELECT count(*) FROM app.historical_assessment_activity_insertions WHERE activity_id=$1) + (SELECT count(*) FROM app.assessment_activities WHERE id=$1) + (SELECT count(*) FROM app.assessment_activity_objectives WHERE activity_id=$1)`, partialID).Scan(&partialRows); e != nil || partialRows != 0 {
		t.Fatalf("rollback parsial meninggalkan data: %d %v", partialRows, e)
	}

	// Himpunan awal yang lengkap diterima, tetapi TP tambahan dan metadata tidak boleh diubah.
	txSavepoint("complete_set")
	historicalID := newActivity("same source one", "00000000-0000-4000-8000-000000002201", true)
	if _, e := tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`); e != nil {
		t.Fatal("himpunan TP lengkap ditolak:", e)
	}
	txSavepoint("extra_objective")
	if _, e := tx.Exec(`INSERT INTO app.assessment_activity_objectives VALUES($1,$2,$3,$4,$5,$6)`, historicalID, tp4, year, semester, class, subject); e == nil {
		t.Fatal("TP tambahan di luar manifest audit diterima")
	}
	txRollbackTo("extra_objective")
	txSavepoint("metadata_update")
	if _, e := tx.Exec(`UPDATE app.assessment_activities SET name='diubah' WHERE id=$1`, historicalID); e == nil {
		t.Fatal("metadata kegiatan historis dapat diubah")
	}
	txRollbackTo("metadata_update")
	for _, column := range []string{"activity_type='formatif'", "activity_date='2020-01-11'", "teacher_id=teacher_id", "class_id=class_id", "subject_id=subject_id", "semester_id=semester_id", "school_year_id=school_year_id", "created_by=created_by"} {
		txSavepoint("metadata_field")
		if _, e := tx.Exec(`UPDATE app.assessment_activities SET `+column+` WHERE id=$1`, historicalID); e == nil {
			t.Fatalf("metadata kegiatan historis %s dapat diubah", column)
		}
		txRollbackTo("metadata_field")
	}
	txSavepoint("metadata_delete")
	if _, e := tx.Exec(`DELETE FROM app.assessment_activities WHERE id=$1`, historicalID); e == nil {
		t.Fatal("kegiatan historis dapat dihapus")
	}
	txRollbackTo("metadata_delete")
	txSavepoint("audit_update")
	if _, e := tx.Exec(`UPDATE app.historical_assessment_activity_insertions SET reason='ubah' WHERE activity_id=$1`, historicalID); e == nil {
		t.Fatal("audit kegiatan historis dapat diubah")
	}
	txRollbackTo("audit_update")
	txSavepoint("detail_delete")
	if _, e := tx.Exec(`DELETE FROM app.historical_assessment_activity_objectives WHERE audit_id=(SELECT id FROM app.historical_assessment_activity_insertions WHERE activity_id=$1)`, historicalID); e == nil {
		t.Fatal("manifest TP audit kegiatan dapat dihapus")
	}
	txRollbackTo("detail_delete")

	// Nama/tanggal yang sama bukan duplikasi sumber jika identitas peristiwanya berbeda.
	txSavepoint("distinct_event")
	_ = newActivity("same-source-two", "00000000-0000-4000-8000-000000002202", true)
	if _, e := tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`); e != nil {
		t.Fatal("peristiwa berbeda dengan nama/tanggal sama ditolak:", e)
	}
	txRollbackTo("distinct_event")

	// UUID dan identitas sumber yang sama ditolak oleh unique constraint.
	txSavepoint("duplicate_submission_identical")
	if _, e := tx.Exec(`INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,'2020-01-10','uji','third-event','{}','{}','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),'00000000-0000-4000-8000-000000002201')`, orphanActivity, year, semester, class, subject, teacher, actor); e == nil {
		t.Fatal("submission identik diterima sebagai duplikat kedua")
	}
	txRollbackTo("duplicate_submission_identical")
	txSavepoint("duplicate_submission")
	if _, e := tx.Exec(`INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,'2020-01-10','payload berbeda','third-event','{}','{}','{}',decode(repeat('33',32),'hex'),decode(repeat('44',32),'hex'),'00000000-0000-4000-8000-000000002201')`, orphanActivity, year, semester, class, subject, teacher, actor); e == nil {
		t.Fatal("submission UUID sama dengan payload berbeda diterima")
	}
	txRollbackTo("duplicate_submission")
	txSavepoint("duplicate_source")
	if _, e := tx.Exec(`INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,'2020-01-10','uji','  SAME   SOURCE ONE  ','{}','{}','{}',decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),gen_random_uuid())`, orphanActivity, year, semester, class, subject, teacher, actor); e == nil {
		t.Fatal("identitas sumber yang sama dengan variasi kapital/spasi diterima")
	}
	txRollbackTo("duplicate_source")
}

// Konkurensi penulisan langsung menguji bahwa kunci transaksi dan UNIQUE
// provenance mencegah dua konfirmasi untuk peristiwa sumber yang sama.
func TestHistoricalAssessmentActivityConcurrentSourceGuardIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := t.Context()
	f := testutil.ExamFixtureFor(t, db, "cp1022race"+testutil.Random(t)[:8])
	var year int64
	if err := db.QueryRow(`SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&year); err != nil {
		t.Fatal(err)
	}
	var tp int64
	if err := db.QueryRow(`INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by) VALUES($1,$2,$3,$4,'TP concurrency',$5) RETURNING id`, year, f.Semester, f.Class, f.Subject, f.Teacher).Scan(&tp); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("final baseline"))
	if _, err := db.Exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',$8,$9::uuid,$8)`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher, hash[:], "00000000-0000-4000-8000-000000002301"); err != nil {
		t.Fatal(err)
	}
	source := "manual-source-" + testutil.Random(t)[:12]
	keys := []string{"00000000-0000-4000-8000-000000002302", "00000000-0000-4000-8000-000000002303"}
	ready := make(chan error, 1)
	release := make(chan struct{})
	var wg sync.WaitGroup
	var errs [2]error
	write := func(i int) {
		defer wg.Done()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			errs[i] = err
			if i == 0 {
				ready <- err
			}
			return
		}
		defer tx.Rollback()
		var activityID, auditID int64
		if err = tx.QueryRow(`SELECT nextval(pg_get_serial_sequence('app.assessment_activities','id'))`).Scan(&activityID); err == nil {
			err = tx.QueryRow(`INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,'2020-01-10','uji konkurensi',$8,'{}','{}','{}',$9,$9,$10::uuid) RETURNING id`, activityID, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher, source, hash[:], keys[i]).Scan(&auditID)
		}
		if err == nil {
			_, err = tx.Exec(`INSERT INTO app.assessment_activities(id,school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,'Sumatif sama','sumatif','2020-01-10',$7)`, activityID, year, f.Semester, f.Class, f.Subject, f.TeacherID, f.Teacher)
		}
		if err == nil {
			_, err = tx.Exec(`INSERT INTO app.historical_assessment_activity_objectives VALUES($1,$2)`, auditID, tp)
		}
		if err == nil {
			_, err = tx.Exec(`INSERT INTO app.assessment_activity_objectives VALUES($1,$2,$3,$4,$5,$6)`, activityID, tp, year, f.Semester, f.Class, f.Subject)
		}
		if i == 0 {
			ready <- err
			if err == nil {
				<-release
			}
		}
		if err != nil {
			errs[i] = err
			return
		}
		errs[i] = tx.Commit()
	}
	wg.Add(1)
	go write(0)
	if err := <-ready; err != nil {
		t.Fatal("penulis pertama:", err)
	}
	wg.Add(1)
	go write(1)
	// Beri transaksi kedua kesempatan memasuki trigger yang terserialisasi.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("konfirmasi sumber bersamaan menghasilkan %d keberhasilan: %v", successes, errs)
	}
	var activities, audits, links int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM app.historical_assessment_activity_insertions WHERE school_year_id=$1 AND semester_id=$2 AND class_id=$3 AND subject_id=$4 AND source_identity=$5),(SELECT count(*) FROM app.assessment_activities WHERE school_year_id=$1 AND semester_id=$2 AND class_id=$3 AND subject_id=$4 AND name='Sumatif sama'),(SELECT count(*) FROM app.assessment_activity_objectives ao JOIN app.assessment_activities a ON a.id=ao.activity_id WHERE a.school_year_id=$1 AND a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4 AND a.name='Sumatif sama')`, year, f.Semester, f.Class, f.Subject, source).Scan(&audits, &activities, &links); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(activities, audits, links) != "1 1 1" {
		t.Fatalf("baris duplikat/parsial setelah race: activities=%d audits=%d links=%d", activities, audits, links)
	}
}
