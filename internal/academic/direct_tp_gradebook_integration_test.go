//go:build integration

package academic

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestDirectTPMutationUsesReadCommittedIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "bookisolation"+testutil.Random(t)[:8])
	if _, err := db.Exec(`SET default_transaction_isolation='repeatable read'`); err != nil {
		t.Fatal(err)
	}
	tx, err := (&Store{DB: db}).beginDirectTP(ctx, f.Teacher)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var actual string
	if err = tx.QueryRow(`SHOW transaction_isolation`).Scan(&actual); err != nil || actual != "read committed" {
		t.Fatalf("isolasi mutasi: %q %v", actual, err)
	}
}

func TestDirectTPGradebookConcurrentReadWriteIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "bookrace"+testutil.Random(t)[:8])
	s := &Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP persaingan"); err != nil {
		t.Fatal(err)
	}
	var objective int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1`, f.Semester).Scan(&objective); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	readErrors := make(chan error, 1)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			keys := []string{"d1000000-0000-4000-8000-000000000001", "d1000000-0000-4000-8000-000000000002"}
			values := []string{"10", "20"}
			results <- s.SaveDirectTPScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, keys[i], []DirectTPScoreInput{{StudentID: f.StudentA, ObjectiveID: objective, ExpectedVersion: -1, Value: values[i]}})
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 10; i++ {
			book, err := s.DirectTPGradebook(ctx, f.Teacher, f.Semester, f.Class, f.Subject)
			if err != nil {
				readErrors <- err
				return
			}
			for _, student := range book.Students {
				if student.ID == f.StudentA && len(student.Cells) > 0 {
					v := student.Cells[0].Value
					if v != "" && v != "10.00" && v != "20.00" {
						readErrors <- errors.New("pembacaan nilai tidak konsisten")
						return
					}
				}
			}
		}
		readErrors <- nil
	}()
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if err := <-readErrors; err != nil {
		t.Fatal(err)
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("hasil %d sukses %d konflik", success, conflict)
	}
	var scores, batches int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM app.direct_tp_initial_scores WHERE semester_id=$1),(SELECT count(*) FROM app.direct_tp_submissions WHERE semester_id=$1)`, f.Semester).Scan(&scores, &batches); err != nil || scores != 1 || batches != 1 {
		t.Fatalf("persistensi %d %d: %v", scores, batches, err)
	}
}
