//go:build integration

package academic

import (
	"crypto/sha256"
	"database/sql"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

// Seluruh perubahan fixture dibatalkan agar histori TEST tidak terhapus.
func TestHistoricalAssessmentLinkGuardIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	tx, err := db.BeginTx(t.Context(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var year, semester, class, subject, actor, teacher, student, tp, tp2, sumatif, formatif int64
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
}
