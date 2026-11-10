//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAssessmentEffectiveAfterCorrectionHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "httpfix"+testutil.Random(t)[:8])
	ctx := context.Background()
	s := &academic.Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP HTTP koreksi menurun"); err != nil {
		t.Fatal(err)
	}
	var tp int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE class_id=$1`, f.Class).Scan(&tp); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif HTTP koreksi menurun", "sumatif", "2020-01-10", []int64{tp}); err != nil {
		t.Fatal(err)
	}
	var activity int64
	if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE class_id=$1`, f.Class).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activity, []academic.AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: tp, Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2`, activity, f.StudentA).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := s.CorrectAssessmentInitialScore(ctx, f.Teacher, id, 0, "20", "Koreksi uji"); err != nil {
		t.Fatal(err)
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &owner))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	check := func(want string) {
		t.Helper()
		for _, path := range []string{fmt.Sprintf("/guru/nilai-awal/%d/remedial", id), fmt.Sprintf("/guru/nilai-awal/%d/koreksi", id), "/guru/nilai-awal-saya", fmt.Sprintf("/guru/kegiatan-penilaian/%d/nilai", activity), fmt.Sprintf("/guru/rekap-nilai-tp?semester_id=%d&class_id=%d&subject_id=%d", f.Semester, f.Class, f.Subject)} {
			w := get(path)
			if w.Code != 200 || (!strings.Contains(w.Body.String(), want) && !strings.Contains(w.Body.String(), strings.ReplaceAll(want, ".", ","))) {
				t.Fatalf("%s HTTP %d tidak ada nilai %s", path, w.Code, want)
			}
		}
	}
	check("20.00")
	if err = s.RecordAssessmentScoreRemedial(ctx, f.Teacher, id, "33333333-3333-4333-8333-333333333333", "30", "2020-01-10"); err != nil {
		t.Fatal(err)
	}
	check("30.00")
}
