//go:build integration

package academic_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestCorrectTeachingAssignmentIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := academic.Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	otherTeacher, _ := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	year, err := s.CreateYear(ctx, curriculum.ID, "Koreksi "+testutil.Random(t)[:10])
	must(err)
	var semester int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester))
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "A", "", 0)
	must(err)
	var teacherID, otherTeacherID, subjectID int64
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Koreksi',$1) RETURNING id`, teacher.ID).Scan(&teacherID))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Guru Koreksi Lain',$1) RETURNING id`, otherTeacher.ID).Scan(&otherTeacherID))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$1) RETURNING id`, "Mapel Koreksi "+testutil.Random(t)[:10]).Scan(&subjectID))
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	must(s.AssignTeacher(ctx, curriculum.ID, semester, class.ID, subjectID, teacherID, start))
	var assignmentID int64
	must(db.QueryRow(`SELECT id FROM app.teaching_assignments WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semester, class.ID, subjectID).Scan(&assignmentID))
	if err = s.CorrectTeachingAssignment(ctx, admin.ID, assignmentID, semester, class.ID, subjectID, otherTeacherID, start, nil); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("Admin correction error = %v", err)
	}
	correctedStart := start.AddDate(0, 0, 1)
	must(s.CorrectTeachingAssignment(ctx, curriculum.ID, assignmentID, semester, class.ID, subjectID, otherTeacherID, correctedStart, nil))
	var storedTeacher int64
	var storedStart time.Time
	must(db.QueryRow(`SELECT teacher_id,effective_from FROM app.teaching_assignments WHERE id=$1`, assignmentID).Scan(&storedTeacher, &storedStart))
	if storedTeacher != otherTeacherID || !storedStart.Equal(correctedStart) {
		t.Fatalf("corrected assignment = teacher %d, start %s", storedTeacher, storedStart)
	}
	var historyCount int
	var before, after string
	must(db.QueryRow(`SELECT count(*), (array_agg(before_state ORDER BY id DESC))[1]::text, (array_agg(after_state ORDER BY id DESC))[1]::text FROM app.teaching_assignment_history WHERE assignment_id=$1`, assignmentID).Scan(&historyCount, &before, &after))
	if historyCount != 2 || before == "" || after == "" {
		t.Fatalf("history count/before/after = %d/%q/%q", historyCount, before, after)
	}
	conflictingStart := correctedStart.AddDate(0, 0, 2)
	conflictingEnd := conflictingStart.AddDate(0, 0, 2)
	must(s.AssignTeacher(ctx, curriculum.ID, semester, class.ID, subjectID, teacherID, conflictingEnd))
	var conflictingID int64
	must(db.QueryRow(`SELECT id FROM app.teaching_assignments WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND teacher_id=$4`, semester, class.ID, subjectID, teacherID).Scan(&conflictingID))
	if err = s.CorrectTeachingAssignment(ctx, curriculum.ID, conflictingID, semester, class.ID, subjectID, otherTeacherID, conflictingStart, nil); err == nil {
		t.Fatal("overlapping correction accepted")
	}
}
