//go:build integration

package academic_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

type transitionFixture struct {
	store       academic.Store
	db          *sql.DB
	ctx         context.Context
	actor       int64
	sourceYear  int64
	targetYear  int64
	sourceClass map[int]int64
	targetClass map[int]int64
}

func newTransitionFixture(t *testing.T, countByGrade map[int]int) (*transitionFixture, map[int][]int64) {
	t.Helper()
	db := testutil.Database(t, "../../.env")
	s := academic.Store{DB: db}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	cur, _ := testutil.User(t, db, "kurikulum")
	admin, _ := testutil.User(t, db, "admin")
	suffix := testutil.Random(t)[:12]
	source, err := s.CreateYear(ctx, cur.ID, "Transisi sumber "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateYear(ctx, cur.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	target, err := s.CreateYear(ctx, cur.ID, "Transisi tujuan "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	f := &transitionFixture{store: s, db: db, ctx: ctx, actor: cur.ID, sourceYear: source.ID, targetYear: target.ID, sourceClass: map[int]int64{}, targetClass: map[int]int64{}}
	for _, grade := range []int{7, 8, 9} {
		name := string(rune('A' + grade - 7))
		from, saveErr := s.SaveClass(ctx, admin.ID, 0, source.ID, grade, name, "", 0)
		if saveErr != nil {
			t.Fatal(saveErr)
		}
		to, saveErr := s.SaveClass(ctx, admin.ID, 0, target.ID, grade, name, "", 0)
		if saveErr != nil {
			t.Fatal(saveErr)
		}
		f.sourceClass[grade], f.targetClass[grade] = from.ID, to.ID
	}
	students := map[int][]int64{}
	for grade, count := range countByGrade {
		for i := 0; i < count; i++ {
			var studentID int64
			if err = db.QueryRowContext(ctx, `INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','L') RETURNING id`, fmt.Sprintf("Transisi %s %d %d", suffix, grade, i)).Scan(&studentID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.CreatePlacement(ctx, admin.ID, studentID, f.sourceClass[grade]); err != nil {
				t.Fatal(err)
			}
			students[grade] = append(students[grade], studentID)
		}
	}
	return f, students
}

func TestStudentYearTransitionBatchIntegration(t *testing.T) {
	f, students := newTransitionFixture(t, map[int]int{7: 1, 8: 1, 9: 2})
	decisions := []academic.TransitionDecision{
		{StudentID: students[7][0], Outcome: "promote", TargetClassID: f.targetClass[8]},
		{StudentID: students[8][0], Outcome: "repeat", TargetClassID: f.targetClass[8]},
		{StudentID: students[9][0], Outcome: "graduate"},
		{StudentID: students[9][1], Outcome: "leave"},
	}
	if err := f.store.ExecuteStudentTransition(f.ctx, f.actor, f.sourceYear, f.targetYear, decisions); err != nil {
		t.Fatal("execute batch:", err)
	}
	var transitions, targetPlacements, histories int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM app.student_year_transitions WHERE source_year_id=$1`, f.sourceYear).Scan(&transitions); err != nil || transitions != len(decisions) {
		t.Fatalf("transition history count=%d err=%v", transitions, err)
	}
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM app.student_placements WHERE school_year_id=$1`, f.targetYear).Scan(&targetPlacements); err != nil || targetPlacements != 2 {
		t.Fatalf("target placement count=%d err=%v", targetPlacements, err)
	}
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM app.student_placement_history h JOIN app.student_placements p ON p.id=h.placement_id WHERE p.school_year_id=$1`, f.targetYear).Scan(&histories); err != nil || histories != 2 {
		t.Fatalf("target history count=%d err=%v", histories, err)
	}
	var promoteReasons, repeatReasons, noTargetOutcomes int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FILTER (WHERE reason='Transisi tahun ajaran: naik kelas'), count(*) FILTER (WHERE reason='Transisi tahun ajaran: tidak naik kelas') FROM app.student_placement_history h JOIN app.student_placements p ON p.id=h.placement_id WHERE p.school_year_id=$1`, f.targetYear).Scan(&promoteReasons, &repeatReasons); err != nil || promoteReasons != 1 || repeatReasons != 1 {
		t.Fatalf("transition reasons promote=%d repeat=%d err=%v", promoteReasons, repeatReasons, err)
	}
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM app.student_year_transitions WHERE source_year_id=$1 AND outcome IN ('graduate','leave') AND target_class_id IS NULL AND target_placement_id IS NULL`, f.sourceYear).Scan(&noTargetOutcomes); err != nil || noTargetOutcomes != 2 {
		t.Fatalf("graduate/leave outcomes with no target=%d err=%v", noTargetOutcomes, err)
	}
	var originalClass, transitionedClass int64
	if err := f.db.QueryRowContext(f.ctx, `SELECT class_id FROM app.student_placements WHERE student_id=$1 AND school_year_id=$2`, students[7][0], f.sourceYear).Scan(&originalClass); err != nil || originalClass != f.sourceClass[7] {
		t.Fatalf("source placement changed: class=%d err=%v", originalClass, err)
	}
	if err := f.db.QueryRowContext(f.ctx, `SELECT target_class_id FROM app.student_year_transitions WHERE student_id=$1`, students[7][0]).Scan(&transitionedClass); err != nil || transitionedClass != f.targetClass[8] {
		t.Fatalf("promoted target class=%d err=%v", transitionedClass, err)
	}
	if err := f.store.ExecuteStudentTransition(f.ctx, f.actor, f.sourceYear, f.targetYear, decisions); err == nil || !strings.Contains(err.Error(), "sudah pernah dijalankan") {
		t.Fatalf("duplicate batch should be rejected, got %v", err)
	}
	var afterDuplicate int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM app.student_year_transitions WHERE source_year_id=$1`, f.sourceYear).Scan(&afterDuplicate); err != nil || afterDuplicate != len(decisions) {
		t.Fatalf("duplicate batch changed history: count=%d err=%v", afterDuplicate, err)
	}
}

func TestStudentYearTransitionRollbackIntegration(t *testing.T) {
	f, students := newTransitionFixture(t, map[int]int{7: 2})
	first, second := students[7][0], students[7][1]
	trigger := "transition_fail_" + testutil.Random(t)[:12]
	function := "app." + trigger
	_, err := f.db.ExecContext(f.ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$ BEGIN IF NEW.student_id = %d THEN RAISE EXCEPTION 'intentional integration failure'; END IF; RETURN NEW; END $body$`, function, second))
	if err != nil {
		t.Fatal("create failure trigger function:", err)
	}
	_, err = f.db.ExecContext(f.ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON app.student_year_transitions FOR EACH ROW EXECUTE FUNCTION %s()`, trigger, function))
	if err != nil {
		_, _ = f.db.ExecContext(f.ctx, fmt.Sprintf(`DROP FUNCTION %s()`, function))
		t.Fatal("create failure trigger:", err)
	}
	t.Cleanup(func() {
		_, _ = f.db.ExecContext(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app.student_year_transitions`, trigger))
		_, _ = f.db.ExecContext(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, function))
	})
	decisions := []academic.TransitionDecision{
		{StudentID: first, Outcome: "promote", TargetClassID: f.targetClass[8]},
		{StudentID: second, Outcome: "promote", TargetClassID: f.targetClass[8]},
	}
	if err = f.store.ExecuteStudentTransition(f.ctx, f.actor, f.sourceYear, f.targetYear, decisions); err == nil {
		t.Fatal("expected injected failure")
	}
	for _, q := range []struct {
		name  string
		query string
	}{
		{"transition rows", `SELECT count(*) FROM app.student_year_transitions WHERE source_year_id=$1`},
		{"target placements", `SELECT count(*) FROM app.student_placements WHERE school_year_id=$1`},
		{"target placement histories", `SELECT count(*) FROM app.student_placement_history h JOIN app.student_placements p ON p.id=h.placement_id WHERE p.school_year_id=$1`},
	} {
		var count int
		if scanErr := f.db.QueryRowContext(f.ctx, q.query, f.targetYear).Scan(&count); scanErr != nil || count != 0 {
			t.Fatalf("%s leaked from rolled back batch: count=%d err=%v", q.name, count, scanErr)
		}
	}
	if _, err = f.db.ExecContext(f.ctx, fmt.Sprintf(`DROP TRIGGER %s ON app.student_year_transitions`, trigger)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.ExecContext(f.ctx, fmt.Sprintf(`DROP FUNCTION %s()`, function)); err != nil {
		t.Fatal(err)
	}
	if err = f.store.ExecuteStudentTransition(f.ctx, f.actor, f.sourceYear, f.targetYear, decisions); err != nil {
		t.Fatal("batch could not retry after rollback:", err)
	}
}

func TestStudentYearTransitionConcurrentIntegration(t *testing.T) {
	f, students := newTransitionFixture(t, map[int]int{7: 2})
	decisions := []academic.TransitionDecision{
		{StudentID: students[7][0], Outcome: "promote", TargetClassID: f.targetClass[8]},
		{StudentID: students[7][1], Outcome: "repeat", TargetClassID: f.targetClass[7]},
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- f.store.ExecuteStudentTransition(f.ctx, f.actor, f.sourceYear, f.targetYear, decisions)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var successes, failures int
	for err := range results {
		if err == nil {
			successes++
		} else if strings.Contains(err.Error(), "sudah pernah dijalankan") {
			failures++
		} else {
			t.Fatalf("unexpected concurrent batch error: %v", err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent batch results: success=%d failure=%d", successes, failures)
	}
	var transitions, placements int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM app.student_year_transitions WHERE source_year_id=$1`, f.sourceYear).Scan(&transitions); err != nil || transitions != 2 {
		t.Fatalf("concurrent transition count=%d err=%v", transitions, err)
	}
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM app.student_placements WHERE school_year_id=$1`, f.targetYear).Scan(&placements); err != nil || placements != 2 {
		t.Fatalf("concurrent target placement count=%d err=%v", placements, err)
	}
}
