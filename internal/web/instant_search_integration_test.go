//go:build integration

package web

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestInstantSearchUnplacedBeyondFirstPageIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	actor, _ := testutil.User(t, db, "admin")
	prefix := "UX01_" + testutil.Random(t)[:10]
	var year int64
	if e := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, prefix).Scan(&year); e != nil {
		t.Fatal(e)
	}
	var students []int64
	t.Cleanup(func() {
		for _, id := range students {
			if _, e := db.Exec(`DELETE FROM app.students WHERE id=$1`, id); e != nil {
				t.Error(e)
			}
		}
		for _, q := range []string{`DELETE FROM app.school_years WHERE id=$1`} {
			if _, e := db.Exec(q, year); e != nil {
				t.Error(e)
			}
		}
		if _, e := db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1`, actor.ID); e != nil {
			t.Error(e)
		}
		if _, e := db.Exec(`DELETE FROM app.users WHERE id=$1`, actor.ID); e != nil {
			t.Error(e)
		}
	})
	for i := 1; i <= 12; i++ {
		var id int64
		if e := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, fmt.Sprintf("%s_%02d", prefix, i)).Scan(&id); e != nil {
			t.Fatal(e)
		}
		students = append(students, id)
	}
	store := &academic.Store{DB: db}
	first, e := store.UnplacedStudents(context.Background(), actor.ID, year, prefix, 1)
	if e != nil || len(first) != 11 {
		t.Fatal("fixture halaman awal", len(first), e)
	}
	found, e := store.UnplacedStudents(context.Background(), actor.ID, year, strings.ToLower(prefix)+"_12", 1)
	if e != nil || len(found) != 1 || found[0].ID != students[11] {
		t.Fatal("pencarian hanya menjangkau halaman pertama", found, e)
	}
}
