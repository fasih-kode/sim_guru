//go:build integration

package academic_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentMovementsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := academic.Store{DB: db}
	masters := master.Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	other, _ := testutil.User(t, db, "guru_mapel")
	y, err := s.CreateYear(ctx, cur.ID, "Movement "+testutil.Random(t)[:10])
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateYear(ctx, cur.ID, y.ID); err != nil {
		t.Fatal(err)
	}
	a, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "B", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 8, "C", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	makeStudent := func() (int64, int64) {
		t.Helper()
		var id int64
		if e := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, "Movement "+testutil.Random(t)[:10]).Scan(&id); e != nil {
			t.Fatal(e)
		}
		p, e := s.CreatePlacement(ctx, admin.ID, id, a.ID)
		if e != nil {
			t.Fatal(e)
		}
		return id, p
	}
	wali, _ := testutil.User(t, db, "wali_kelas")
	var teacherID int64
	if err = db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Wali Uji',$1) RETURNING id`, wali.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err = s.AssignHomeroom(ctx, cur.ID, a.ID, teacherID); err != nil {
		t.Fatal(err)
	}
	if err = s.AssignHomeroom(ctx, cur.ID, b.ID, teacherID); err != nil {
		t.Fatal(err)
	}
	student, p := makeStudent()
	second, p2 := makeStudent()
	today := time.Now().In(time.FixedZone("WIB", 7*3600))
	date := today.Format("2006-01-02")
	yesterday := today.AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := today.AddDate(0, 0, 1).Format("2006-01-02")
	if err = s.MoveStudentClass(ctx, admin.ID, p, a.ID, b.ID, yesterday, "Pindah"); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Admin can move", err)
	}
	if err = s.MoveStudentClass(ctx, cur.ID, p, a.ID, c.ID, yesterday, "Pindah"); err == nil {
		t.Fatal("wrong grade allowed")
	}
	if err = s.MoveStudentClass(ctx, cur.ID, p, a.ID, b.ID, yesterday, "Pindah"); err != nil {
		t.Fatal(err)
	}
	current, err := s.MovementStudent(ctx, cur.ID, student, 0)
	if err != nil || current.ClassID != b.ID {
		t.Fatal("move not effective", current, err)
	}
	roster, err := s.HomeroomRoster(ctx, wali.ID, b.ID)
	if err != nil || len(roster) != 1 {
		t.Fatal("wali roster did not move", roster, err)
	}
	var original int64
	if err = db.QueryRow(`SELECT class_id FROM app.student_placements WHERE id=$1`, p).Scan(&original); err != nil || original != a.ID {
		t.Fatal("original placement rewritten", err)
	}
	if err = s.MoveStudentClass(ctx, cur.ID, p, a.ID, b.ID, date, "stale"); !errors.Is(err, academic.ErrConflict) {
		t.Fatal("stale source accepted", err)
	}
	if err = s.ExitStudentSchool(ctx, cur.ID, student, current.Version, date, "Pindah sekolah", ""); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("curriculum can exit", err)
	}
	if err = s.ExitStudentSchool(ctx, admin.ID, student, current.Version, date, "Pindah sekolah", "SMP Tujuan"); err != nil {
		t.Fatal(err)
	}
	rec, err := masters.Get(ctx, admin.ID, master.Students, student)
	if err != nil || !rec.Transferred || rec.Active {
		t.Fatal("exit status wrong", rec, err)
	}
	if _, err = masters.Save(ctx, admin.ID, master.Students, student, master.Input{Record: master.Record{ID: student, Name: rec.Name, BirthDate: rec.BirthDate, Gender: rec.Gender, Active: true, Version: rec.Version}, Reason: "Reactivate"}); err == nil {
		t.Fatal("master edit bypassed exit")
	}
	placements, err := s.Placements(ctx, cur.ID, y.ID, b.ID, "", 1)
	if err != nil || len(placements) != 0 {
		t.Fatal("departed student still enrolled", placements, err)
	}
	roster, err = s.HomeroomRoster(ctx, wali.ID, b.ID)
	if err != nil || len(roster) != 0 {
		t.Fatal("departed still visible to wali", roster, err)
	}
	counts, err := s.PlacementCounts(ctx, admin.ID)
	if err != nil || counts[b.ID] != 0 {
		t.Fatal("wrong counts", counts, err)
	}
	next, err := s.CreateYear(ctx, cur.ID, "Movement next "+testutil.Random(t)[:10])
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.TransitionPreview(ctx, cur.ID, y.ID, next.ID)
	if err != nil || len(preview.Students) != 1 || preview.Students[0].StudentID != second {
		t.Fatal("departed included in transition", preview, err)
	}
	if err = s.MoveStudentClass(ctx, cur.ID, p2, a.ID, b.ID, tomorrow, "Besok"); err != nil {
		t.Fatal(err)
	}
	current, err = s.MovementStudent(ctx, cur.ID, second, 0)
	if err != nil || current.ClassID != a.ID {
		t.Fatal("future move applied early", current, err)
	}
	if _, err = s.TransitionPreview(ctx, cur.ID, y.ID, next.ID); err == nil {
		t.Fatal("pending move did not block transition")
	}
	if err = s.ExitStudentSchool(ctx, admin.ID, second, current.Version, today.AddDate(0, 0, 2).Format("2006-01-02"), "Terjadwal", ""); err != nil {
		t.Fatal(err)
	}
	rec, err = masters.Get(ctx, admin.ID, master.Students, second)
	if err != nil || rec.Transferred || !rec.Active {
		t.Fatal("future exit applied early", rec, err)
	}
	history, err := s.StudentMovements(ctx, cur.ID, student)
	if err != nil || len(history) != 2 || history[0].FromClass != "7B" {
		t.Fatal("history wrong", history, err)
	}
	if _, err = s.StudentMovements(ctx, other.ID, student); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("history leaked", err)
	}
	if err = s.ExitStudentSchool(ctx, admin.ID, student, 1, date, "duplicate", ""); err == nil {
		t.Fatal("duplicate exit accepted")
	}
}
