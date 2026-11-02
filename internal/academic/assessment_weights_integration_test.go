//go:build integration

package academic

import (
	"context"
	"errors"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAssessmentWeightsAuthorizationValidationAndAuditIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	var yearID, semesterID, otherSemesterID int64
	name := "Bobot test " + testutil.Random(t)[:12]
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, name).Scan(&yearID); err != nil {
		t.Fatal("gagal membuat tahun ajaran sintetis", err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal("gagal membuat semester sintetis", err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,2) RETURNING id`, yearID).Scan(&otherSemesterID); err != nil {
		t.Fatal("gagal membuat semester pembanding", err)
	}
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM app.semester_assessment_weight_history WHERE semester_id=$1`, `DELETE FROM app.semester_assessment_weights WHERE semester_id=$1`} {
			if _, err := db.Exec(q, semesterID); err != nil {
				t.Errorf("cleanup data bobot: %v", err)
				return
			}
		}
		if _, err := db.Exec(`DELETE FROM app.semesters WHERE school_year_id=$1`, yearID); err != nil {
			t.Errorf("cleanup semester uji: %v", err)
			return
		}
		if _, err := db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID); err != nil {
			t.Errorf("cleanup tahun ajaran: %v", err)
		}
		for _, userID := range []int64{curriculum.ID, teacher.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, err := db.Exec(q, userID); err != nil {
					t.Errorf("cleanup akun tes: %v", err)
					return
				}
			}
		}
	})
	store := &Store{DB: db}
	initial, err := store.AssessmentWeight(ctx, curriculum.ID, semesterID)
	if err != nil || initial.Configured {
		t.Fatalf("konfigurasi awal harus belum diatur: %#v %v", initial, err)
	}
	valid := AssessmentWeights{SemesterID: semesterID, NH: 60, ASTS: 20, ASAS: 20}
	if err = store.SaveAssessmentWeight(ctx, teacher.ID, valid, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru tidak boleh mengubah bobot: %v", err)
	}
	if err = store.SaveAssessmentWeight(ctx, curriculum.ID, AssessmentWeights{SemesterID: semesterID, NH: 60, ASTS: 30, ASAS: 20}, 0); err == nil {
		t.Fatal("total 110 diterima")
	}
	assertCounts := func(wantCurrent, wantHistory int) {
		t.Helper()
		var current, history int
		if e := db.QueryRow(`SELECT count(*) FROM app.semester_assessment_weights WHERE semester_id=$1`, semesterID).Scan(&current); e != nil {
			t.Fatal(e)
		}
		if e := db.QueryRow(`SELECT count(*) FROM app.semester_assessment_weight_history WHERE semester_id=$1`, semesterID).Scan(&history); e != nil {
			t.Fatal(e)
		}
		if current != wantCurrent || history != wantHistory {
			t.Fatalf("baris konfigurasi/histori=%d/%d, ingin %d/%d", current, history, wantCurrent, wantHistory)
		}
	}
	assertCounts(0, 0)
	if _, err = db.Exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,actor_id) VALUES($1,60,30,20,$2)`, semesterID, curriculum.ID); err == nil {
		t.Fatal("constraint database harus menolak total bobot 110")
	}
	if err = store.SaveAssessmentWeight(ctx, curriculum.ID, valid, 0); err != nil {
		t.Fatal("konfigurasi 100% sah gagal disimpan", err)
	}
	got, err := store.AssessmentWeight(ctx, curriculum.ID, semesterID)
	if err != nil || !got.Configured || got.NH != 60 || got.ASTS != 20 || got.ASAS != 20 || got.Version != 1 {
		t.Fatalf("konfigurasi tersimpan tidak cocok: %#v %v", got, err)
	}
	if err = store.SaveAssessmentWeight(ctx, curriculum.ID, AssessmentWeights{SemesterID: semesterID, NH: 100, ASTS: 10}, got.Version); err == nil {
		t.Fatal("perubahan tidak valid 110 diterima")
	}
	assertCounts(1, 1)
	if err = store.SaveAssessmentWeight(ctx, curriculum.ID, AssessmentWeights{SemesterID: semesterID, NH: 50, ASTS: 25, ASAS: 25}, got.Version); err != nil {
		t.Fatal("pembaruan valid gagal", err)
	}
	other, err := store.AssessmentWeight(ctx, curriculum.ID, otherSemesterID)
	if err != nil || other.Configured {
		t.Fatalf("perubahan semester pertama tidak boleh mengubah semester kedua: %#v %v", other, err)
	}
	if err = store.SaveAssessmentWeight(ctx, curriculum.ID, AssessmentWeights{SemesterID: semesterID, NH: 60, ASTS: 20, ASAS: 20}, got.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("versi basi harus ditolak: %v", err)
	}
	assertCounts(1, 2)
	var previousNH, previousASTS, previousASAS int
	if err = db.QueryRow(`SELECT previous_nh,previous_asts,previous_asas FROM app.semester_assessment_weight_history WHERE semester_id=$1 ORDER BY id DESC LIMIT 1`, semesterID).Scan(&previousNH, &previousASTS, &previousASAS); err != nil {
		t.Fatal(err)
	}
	if previousNH != 60 || previousASTS != 20 || previousASAS != 20 {
		t.Fatalf("snapshot audit sebelumnya salah: %d/%d/%d", previousNH, previousASTS, previousASAS)
	}
	var invalidAuditRows int
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_assessment_weight_history WHERE semester_id=$1 AND (actor_id<>$2 OR changed_at IS NULL)`, semesterID, curriculum.ID).Scan(&invalidAuditRows); err != nil || invalidAuditRows != 0 {
		t.Fatalf("audit harus mencatat actor Kurikulum dan waktu setiap perubahan: invalid=%d err=%v", invalidAuditRows, err)
	}
}
