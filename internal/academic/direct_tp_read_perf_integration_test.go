//go:build integration

package academic

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type directTPQueryCounter struct{ count atomic.Int64 }

func (c *directTPQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.count.Add(1)
	return ctx
}
func (*directTPQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestDirectTPReadMeasurementsIntegration(t *testing.T) {
	checked := testutil.Database(t, "../../.env")
	_ = checked
	fixture := testutil.FixtureDatabase(t)
	f := testutil.ExamFixtureFor(t, fixture, "readperf"+testutil.Random(t)[:8])
	ctx := context.Background()
	setup := &Store{DB: fixture}
	if _, err := fixture.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id) VALUES($1,100,0,0,1,$2)`, f.Semester, f.Teacher); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := setup.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, fmt.Sprintf("TP uji %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	var year int64
	if err := fixture.QueryRow(`SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&year); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 36; i++ {
		var id int64
		if err := fixture.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, fmt.Sprintf("fixture readperf %s %d", testutil.Random(t)[:8], i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01')`, year, id, f.Class, f.Teacher); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := pgx.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	counter := &directTPQueryCounter{}
	cfg.Tracer = counter
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	runtime := &Store{DB: db}
	measure := func(name string, fn func() error) {
		t.Helper()
		counter.count.Store(0)
		start := time.Now()
		if err := fn(); err != nil {
			t.Fatal(err)
		}
		if counter.count.Load() > 20 {
			t.Fatalf("%s menghasilkan %d query untuk 39 siswa dan 10 TP", name, counter.count.Load())
		}
		t.Logf("%s siswa=39 tp=10 queries=%d duration=%s", name, counter.count.Load(), time.Since(start))
	}
	measure("gradebook", func() error {
		b, e := runtime.DirectTPGradebook(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
		if e == nil && len(b.Students) != 39 {
			return fmt.Errorf("roster: %d", len(b.Students))
		}
		return e
	})
	measure("recap", func() error {
		_, e := runtime.AssessmentTPRecap(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
		return e
	})
	measure("recommendation", func() error {
		_, e := runtime.SemesterRecommendations(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
		return e
	})
}
