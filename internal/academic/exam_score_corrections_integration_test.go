//go:build integration

package academic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestExamScoreCorrectionsOwnershipConcurrencyAndHistoryIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "examcorrection")
	s := &Store{DB: db}
	ctx := context.Background()
	save := func(class, student int64, component, date, value string) {
		t.Helper()
		if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, class, f.Subject, []ExamInitialInput{{StudentID: student, Component: component, Date: date, Value: value}}); err != nil {
			t.Fatal(err)
		}
	}
	save(f.Class, f.StudentA, "asts", "2020-01-10", "72,50")
	save(f.Class, f.StudentA, "asas", "2020-01-10", "65")
	save(f.Class, f.MovedStudent, "asts", "2020-01-10", "60")
	save(f.OtherClass, f.MovedStudent, "asts", "2020-02-10", "75")
	getID := func(class, student int64, component string) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(`SELECT id FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4 AND component=$5`, f.Semester, class, f.Subject, student, component).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	asts := getID(f.Class, f.StudentA, "asts")
	asas := getID(f.Class, f.StudentA, "asas")
	otherClass := getID(f.OtherClass, f.MovedStudent, "asts")
	firstClass := getID(f.Class, f.MovedStudent, "asts")
	detail, err := s.ExamScoreCorrectionDetail(ctx, f.Teacher, asts)
	if err != nil {
		t.Fatal(err)
	}
	base := detail.CurrentVersion
	for _, v := range []string{"-1", "100,01", "82,567", ""} {
		if err = s.CorrectExamInitialScore(ctx, f.Teacher, asts, base, v, "valid"); err == nil {
			t.Fatalf("nilai invalid diterima %q", v)
		}
	}
	for _, reason := range []string{" ", strings.Repeat("a", 501)} {
		if err = s.CorrectExamInitialScore(ctx, f.Teacher, asts, base, "80", reason); err == nil {
			t.Fatal("alasan invalid diterima")
		}
	}
	if err = s.CorrectExamInitialScore(ctx, f.Other, asts, base, "80", "bukan pemilik"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guru lain: %v", err)
	}
	if _, err = s.ExamScoreCorrectionDetail(ctx, f.Other, asts); !errors.Is(err, ErrNotFound) {
		t.Fatalf("baca guru lain: %v", err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, 999999999, 0, "80", "salah id"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("id asing: %v", err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, asts, base, "82,50", "Koreksi pertama"); err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, asts, base+1, "0", "Koreksi kedua"); err != nil {
		t.Fatal(err)
	}
	d, err := s.ExamScoreCorrectionDetail(ctx, f.Teacher, asts)
	if err != nil || d.Score.InitialValue != "72.50" || d.Score.CorrectedValue != "0.00" || d.CurrentVersion != base+2 || len(d.History) != int(base+2) {
		t.Fatalf("histori ASTS: %+v %v", d, err)
	}
	if d.History[base].After != "82.50" || d.History[base+1].Before != "82.50" || d.History[base+1].After != "0.00" {
		t.Fatal("urutan koreksi tidak sesuai")
	}
	ad, err := s.ExamScoreCorrectionDetail(ctx, f.Teacher, asas)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, asas, ad.CurrentVersion, "100", "Koreksi ASAS"); err != nil {
		t.Fatal(err)
	}
	d, err = s.ExamScoreCorrectionDetail(ctx, f.Teacher, asts)
	if err != nil || d.CurrentVersion != base+2 {
		t.Fatal("koreksi ASAS memengaruhi ASTS")
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, asts, base, "50", "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("versi basi: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.CorrectExamInitialScore(ctx, f.Teacher, asts, base+2, "50", "Serentak")
		}()
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for e := range results {
		if e == nil {
			ok++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("konkurensi: sukses %d konflik %d", ok, conflict)
	}
	var orig string
	if err = db.QueryRow(`SELECT score::text FROM app.exam_initial_scores WHERE id=$1`, asts).Scan(&orig); err != nil || orig != "72.50" {
		t.Fatalf("nilai asli berubah %s %v", orig, err)
	}
	if _, err = db.Exec(`UPDATE app.exam_score_corrections SET reason='ubah' WHERE initial_score_id=$1`, asts); err == nil {
		t.Fatal("UPDATE histori diterima")
	}
	if _, err = db.Exec(`DELETE FROM app.exam_score_corrections WHERE initial_score_id=$1`, asts); err == nil {
		t.Fatal("DELETE histori diterima")
	}
	firstDetail, err := s.ExamScoreCorrectionDetail(ctx, f.Teacher, firstClass)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, firstClass, firstDetail.CurrentVersion, "61", "Koreksi kelas asal"); err != nil {
		t.Fatal(err)
	}
	od, err := s.ExamScoreCorrectionDetail(ctx, f.Teacher, otherClass)
	if err != nil || od.Score.InitialValue != "75.00" || od.CurrentVersion != 0 {
		t.Fatalf("sumber kelas lain berubah %+v %v", od, err)
	}
	// Pengampu baru tetap tidak berhak mengakses atau mengubah bukti milik guru lama.
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET teacher_id=$1 WHERE semester_id=$2 AND class_id=$3`, f.OtherID, f.Semester, f.Class); err != nil {
		t.Fatal(err)
	}
	_, newReadErr := s.ExamScoreCorrectionDetail(ctx, f.Other, asts)
	newWriteErr := s.CorrectExamInitialScore(ctx, f.Other, asts, base+3, "70", "Bukan pemilik")
	_, oldReadErr := s.ExamScoreCorrectionDetail(ctx, f.Teacher, asts)
	_, restoreOwnerErr := db.Exec(`UPDATE app.teaching_assignments SET teacher_id=$1 WHERE semester_id=$2 AND class_id=$3`, f.TeacherID, f.Semester, f.Class)
	if restoreOwnerErr != nil {
		t.Fatal(restoreOwnerErr)
	}
	if !errors.Is(newReadErr, ErrNotFound) || !errors.Is(newWriteErr, ErrForbidden) || oldReadErr != nil {
		t.Fatalf("pergantian pengampu: %v %v %v", newReadErr, newWriteErr, oldReadErr)
	}
	// Hak koreksi historis tidak bergantung pada penugasan yang sudah berakhir.
	if _, err = db.Exec(`UPDATE app.teaching_assignments SET effective_until=current_date WHERE semester_id=$1 AND class_id=$2`, f.Semester, f.Class); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`UPDATE app.teaching_assignments SET effective_until=NULL WHERE semester_id=$1 AND class_id=$2`, f.Semester, f.Class)
	d, err = s.ExamScoreCorrectionDetail(ctx, f.Teacher, asts)
	if err != nil {
		t.Fatal("pemilik historis baca", err)
	}
	if err = s.CorrectExamInitialScore(ctx, f.Teacher, asts, d.CurrentVersion, "80", "Koreksi historis"); err != nil {
		t.Fatal("pemilik historis tulis", err)
	}
	for _, x := range []struct {
		off, on string
		id      int64
	}{{`UPDATE app.users SET active=false WHERE id=$1`, `UPDATE app.users SET active=true WHERE id=$1`, f.Teacher}, {`UPDATE app.teachers SET active=false WHERE id=$1`, `UPDATE app.teachers SET active=true WHERE id=$1`, f.TeacherID}} {
		if _, err = db.Exec(x.off, x.id); err != nil {
			t.Fatal(err)
		}
		err = s.CorrectExamInitialScore(ctx, f.Teacher, asts, base+4, "25", "nonaktif")
		_, inactiveReadErr := s.ExamScoreCorrectionDetail(ctx, f.Teacher, asts)
		_, restore := db.Exec(x.on, x.id)
		if restore != nil {
			t.Fatal(restore)
		}
		if !errors.Is(err, ErrForbidden) || !errors.Is(inactiveReadErr, ErrForbidden) {
			t.Fatalf("nonaktif: tulis %v baca %v", err, inactiveReadErr)
		}
	}
	list, err := s.OwnedExamInitialScores(ctx, f.Teacher, AssessmentOwnedScoreFilter{Search: "fixture cp1009", Page: 1})
	if err != nil || len(list.Items) == 0 {
		t.Fatalf("daftar historis %v", err)
	}
}
