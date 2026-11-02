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

func TestPlacementStartBatchIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	y, e := s.CreateYear(ctx, cur.ID, "Batch "+testutil.Random(t)[:10])
	must(e)
	c, e := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	must(e)
	var sem int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, y.ID).Scan(&sem))
	suggestion, e := s.SuggestedPlacementStart(ctx, admin.ID, y.ID)
	must(e)
	if suggestion != "" {
		t.Fatal("guessed date")
	}
	must(s.SaveSemesterCalendar(ctx, cur.ID, SemesterCalendar{SemesterID: sem, StartsOn: "2020-07-13", EndsOn: "2020-12-31", SchoolDays: []int{1, 2, 3, 4, 5}}, "Kalender"))
	suggestion, e = s.SuggestedPlacementStart(ctx, admin.ID, y.ID)
	must(e)
	if suggestion != "2020-07-13" {
		t.Fatal(suggestion)
	}
	var ids []int64
	for i := 0; i < 14; i++ {
		var student int64
		must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, fmt.Sprintf("Batch %02d", i)).Scan(&student))
		p, e := s.CreatePlacement(ctx, admin.ID, student, c.ID)
		must(e)
		ids = append(ids, p)
	}
	must(s.SavePlacementStart(ctx, admin.ID, ids[13], 1, "2020-07-14", "Tanggal khusus"))
	b := PlacementStartBatch{YearID: y.ID, ClassID: c.ID, Date: "2020-07-13", Reason: "Awal tahun", IDs: ids[:12]}
	p, e := s.PreviewPlacementStartBatch(ctx, admin.ID, b)
	must(e)
	if !p.Valid || len(p.Rows) != 12 {
		t.Fatal(p)
	}
	var n int
	must(db.QueryRow(`SELECT count(*) FROM app.student_placements WHERE school_year_id=$1 AND starts_on IS NOT NULL`, y.ID).Scan(&n))
	if n != 1 {
		t.Fatal("preview wrote", n)
	}
	if _, e = s.PreviewPlacementStartBatch(ctx, cur.ID, b); !errors.Is(e, ErrForbidden) {
		t.Fatal("role", e)
	}
	invalid := b
	invalid.IDs = []int64{ids[13]}
	p2, e := s.PreviewPlacementStartBatch(ctx, admin.ID, invalid)
	must(e)
	if p2.Valid || p2.Rows[0].Issue == "" {
		t.Fatal("overwrite accepted")
	}
	invalid = b
	invalid.Date = "2020-07-01"
	p2, e = s.PreviewPlacementStartBatch(ctx, admin.ID, invalid)
	must(e)
	if p2.Valid {
		t.Fatal("outside calendar")
	}
	// A change that remains valid still invalidates the confirmation fingerprint.
	must(s.SaveSchoolDateException(ctx, cur.ID, sem, 1, SchoolDateException{Date: "2020-07-20", Label: "Libur"}, false, "Perubahan kalender"))
	if _, e = s.ApplyPlacementStartBatch(ctx, admin.ID, b, p.Digest); !errors.Is(e, ErrConflict) {
		t.Fatal("stale calendar", e)
	}
	p, e = s.PreviewPlacementStartBatch(ctx, admin.ID, b)
	must(e)
	// Inject a failure on a later row; the first writes and histories must roll back.
	_, e = db.Exec(fmt.Sprintf(`CREATE FUNCTION app.fail_batch_date() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id=%d THEN RAISE EXCEPTION 'batch test failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_batch_date BEFORE UPDATE ON app.student_placements FOR EACH ROW EXECUTE FUNCTION app.fail_batch_date()`, ids[1]))
	must(e)
	if _, e = s.ApplyPlacementStartBatch(ctx, admin.ID, b, p.Digest); e == nil {
		t.Fatal("failure not injected")
	}
	_, e = db.Exec(`DROP TRIGGER fail_batch_date ON app.student_placements; DROP FUNCTION app.fail_batch_date()`)
	must(e)
	must(db.QueryRow(`SELECT count(*) FROM app.student_placements WHERE school_year_id=$1 AND starts_on IS NOT NULL`, y.ID).Scan(&n))
	if n != 1 {
		t.Fatal("partial batch", n)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.ApplyPlacementStartBatch(ctx, admin.ID, b, p.Digest); results <- e }()
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal(success, conflicts)
	}
	var untouched string
	must(db.QueryRow(`SELECT coalesce(starts_on::text,'') FROM app.student_placements WHERE id=$1`, ids[12]).Scan(&untouched))
	if untouched != "" {
		t.Fatal("excluded student changed")
	}
	must(db.QueryRow(`SELECT starts_on::text FROM app.student_placements WHERE id=$1`, ids[13]).Scan(&untouched))
	if untouched != "2020-07-14" {
		t.Fatal("existing overwritten")
	}
	must(db.QueryRow(`SELECT count(*) FROM app.attendance_foundation_history WHERE entity_type='placement_date' AND entity_id=ANY($1::bigint[])`, ids[:12]).Scan(&n))
	if n != 12 {
		t.Fatal("histories", n)
	}
}
