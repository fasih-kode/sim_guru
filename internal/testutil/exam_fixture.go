//go:build integration

package testutil

import (
	"database/sql"
	"fmt"
	"testing"
)

// ExamFixture tetap memakai satu fixture sintetis per paket agar histori immutable tidak terus bertambah.
type ExamFixture struct{ Teacher, Other, TeacherID, OtherID, Semester, Class, OtherClass, Subject, StudentA, StudentB, MovedStudent int64 }

func ExamFixtureFor(t *testing.T, db *sql.DB, key string) ExamFixture {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(9101009)`); err != nil {
		t.Fatal(err)
	}
	name := "fixture-cp1009-" + key
	var year int64
	err = tx.QueryRow(`SELECT id FROM app.school_years WHERE name=$1`, name).Scan(&year)
	if err == sql.ErrNoRows {
		if err = tx.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, name).Scan(&year); err != nil {
			t.Fatal(err)
		}
		var semester, subject, c1, c2, teacher, other, teacherID, otherID int64
		if err = tx.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, year).Scan(&semester); err != nil {
			t.Fatal(err)
		}
		if err = tx.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$1) RETURNING id`, "CP1009-"+key).Scan(&subject); err != nil {
			t.Fatal(err)
		}
		for i, p := range []*int64{&c1, &c2} {
			if err = tx.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, year, fmt.Sprintf("CP1009-%s-%d", key, i)).Scan(p); err != nil {
				t.Fatal(err)
			}
		}
		for i, p := range []*int64{&teacher, &other} {
			if err = tx.QueryRow(`INSERT INTO app.users(username,display_name,password_hash) VALUES($1,$1,'fixture-login-disabled') RETURNING id`, fmt.Sprintf("fixture_cp1009_%s_%d", key, i)).Scan(p); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(`INSERT INTO app.user_roles(user_id,role_code) VALUES($1,'guru_mapel')`, *p); err != nil {
				t.Fatal(err)
			}
		}
		if err = tx.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru fixture 1',$1) RETURNING id`, teacher).Scan(&teacherID); err != nil {
			t.Fatal(err)
		}
		if err = tx.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru fixture 2',$1) RETURNING id`, other).Scan(&otherID); err != nil {
			t.Fatal(err)
		}
		for _, class := range []int64{c1, c2} {
			if _, err = tx.Exec(`INSERT INTO app.teaching_assignments(semester_id,class_id,subject_id,teacher_id,effective_from,assigned_by) VALUES($1,$2,$3,$4,'2020-01-01',$5)`, semester, class, subject, teacherID, teacher); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 3; i++ {
			var student, placement int64
			if err = tx.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, fmt.Sprintf("fixture cp1009 %s siswa %d", key, i)).Scan(&student); err != nil {
				t.Fatal(err)
			}
			if err = tx.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01') RETURNING id`, year, student, c1, teacher).Scan(&placement); err != nil {
				t.Fatal(err)
			}
			if i == 2 {
				if _, err = tx.Exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,'2020-02-01','Fixture uji',$4)`, placement, c1, c2, teacher); err != nil {
					t.Fatal(err)
				}
			}
		}
	} else if err != nil {
		t.Fatal(err)
	}
	var f ExamFixture
	queries := []struct {
		q    string
		dest *int64
		arg  any
	}{
		{`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, &f.Semester, year},
		{`SELECT id FROM app.subjects WHERE code=$1`, &f.Subject, "CP1009-" + key},
		{`SELECT id FROM app.classes WHERE school_year_id=$1 AND name=$2`, &f.Class, fmt.Sprintf("CP1009-%s-0", key)},
		{`SELECT id FROM app.classes WHERE school_year_id=$1 AND name=$2`, &f.OtherClass, fmt.Sprintf("CP1009-%s-1", key)},
	}
	for _, x := range queries {
		if x.dest == &f.Class || x.dest == &f.OtherClass {
			err = tx.QueryRow(x.q, year, x.arg).Scan(x.dest)
		} else {
			err = tx.QueryRow(x.q, x.arg).Scan(x.dest)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, p := range []*int64{&f.Teacher, &f.Other} {
		if err = tx.QueryRow(`SELECT id FROM app.users WHERE username=$1`, fmt.Sprintf("fixture_cp1009_%s_%d", key, i)).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.QueryRow(`SELECT id FROM app.teachers WHERE user_id=$1`, f.Teacher).Scan(&f.TeacherID); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(`SELECT id FROM app.teachers WHERE user_id=$1`, f.Other).Scan(&f.OtherID); err != nil {
		t.Fatal(err)
	}
	for i, p := range []*int64{&f.StudentA, &f.StudentB, &f.MovedStudent} {
		if err = tx.QueryRow(`SELECT id FROM app.students WHERE name=$1`, fmt.Sprintf("fixture cp1009 %s siswa %d", key, i)).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return f
}
