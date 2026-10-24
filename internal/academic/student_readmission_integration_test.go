//go:build integration

package academic_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentReadmissionIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := academic.Store{DB: db}
	ms := master.Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	cur, _ := testutil.User(t, db, "kurikulum")
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	y, err := s.CreateYear(ctx, cur.ID, "Return "+testutil.Random(t)[:10])
	must(err)
	must(s.ActivateYear(ctx, cur.ID, y.ID))
	a, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	must(err)
	b, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "B", "", 0)
	must(err)
	wrong, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 8, "C", "", 0)
	must(err)
	makeStudent := func() (int64, int64) {
		t.Helper()
		var id int64
		must(db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, "Return "+testutil.Random(t)[:10]).Scan(&id))
		p, e := s.CreatePlacement(ctx, admin.ID, id, a.ID)
		must(e)
		return id, p
	}
	student, p := makeStudent()
	second, p2 := makeStudent()
	today := time.Now().In(time.FixedZone("WIB", 7*3600))
	date := func(n int) string { return today.AddDate(0, 0, n).Format("2006-01-02") }
	info := func(id int64) academic.StudentMovement {
		t.Helper()
		v, e := s.MovementStudent(ctx, admin.ID, id, 0)
		must(e)
		return v
	}
	v := info(student)
	must(s.ExitStudentSchool(ctx, admin.ID, student, v.Version, date(-8), "Keluar pertama", "Sekolah lain"))
	v = info(student)
	if err = s.ReadmitStudent(ctx, cur.ID, student, v.Version, v.ExitID, b.ID, date(-6), "Kembali", ""); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("curriculum can readmit", err)
	}
	if err = s.ReadmitStudent(ctx, admin.ID, student, v.Version, v.ExitID, wrong.ID, date(-6), "Kembali", ""); err == nil {
		t.Fatal("same year wrong grade accepted")
	}
	if err = s.ReadmitStudent(ctx, admin.ID, student, v.Version, v.ExitID, b.ID, date(-9), "Kembali", ""); err == nil {
		t.Fatal("return before exit accepted")
	}
	must(s.ReadmitStudent(ctx, admin.ID, student, v.Version, v.ExitID, b.ID, date(-6), "Kembali pertama", "Sekolah asal"))
	if err = s.ReadmitStudent(ctx, admin.ID, student, v.Version, v.ExitID, b.ID, date(-6), "duplicate", ""); err == nil {
		t.Fatal("duplicate accepted")
	}
	for _, tc := range []struct {
		offset int
		class  int64
		valid  bool
	}{{-9, a.ID, true}, {-7, 0, false}, {-6, b.ID, true}} {
		var got sql.NullInt64
		must(db.QueryRow(`SELECT app.student_placement_class_at($1,$2::date)`, p, date(tc.offset)).Scan(&got))
		if got.Valid != tc.valid || got.Int64 != tc.class {
			t.Fatal("gap or historical class wrong", tc, got)
		}
	}
	v = info(student)
	if v.ClassID != b.ID {
		t.Fatal("return did not change effective class")
	}
	must(s.ExitStudentSchool(ctx, admin.ID, student, v.Version, date(-4), "Keluar kedua", ""))
	v = info(student)
	must(s.ReadmitStudent(ctx, admin.ID, student, v.Version, v.ExitID, a.ID, date(-2), "Kembali kedua", ""))
	rec, err := ms.Get(ctx, admin.ID, master.Students, student)
	must(err)
	if !rec.Active || rec.Transferred || rec.ExitDate != "" {
		t.Fatal("old exit still blocks returned student", rec)
	}
	placements, err := s.Placements(ctx, cur.ID, y.ID, a.ID, "", 1)
	must(err)
	if len(placements) != 2 {
		t.Fatal("returned missing from roster", placements)
	}
	var count int
	var original int64
	must(db.QueryRow(`SELECT count(*),min(class_id) FROM app.student_placements WHERE student_id=$1`, student).Scan(&count, &original))
	if count != 1 || original != a.ID {
		t.Fatal("original placement duplicated/rewritten")
	}
	history, err := s.StudentMovements(ctx, cur.ID, student)
	must(err)
	if len(history) != 4 || !strings.Contains(history[0].ToClass, "Terima kembali") || history[1].FromClass != "7B" {
		t.Fatal("history lost", history)
	}
	next, err := s.CreateYear(ctx, cur.ID, "Return next "+testutil.Random(t)[:10])
	must(err)
	nextClass, err := s.SaveClass(ctx, admin.ID, 0, next.ID, 8, "A", "", 0)
	must(err)
	preview, err := s.TransitionPreview(ctx, cur.ID, y.ID, next.ID)
	must(err)
	if len(preview.Students) != 2 {
		t.Fatal("returned excluded from transition")
	}
	v = info(student)
	must(s.ExitStudentSchool(ctx, admin.ID, student, v.Version, date(-1), "Keluar ketiga", ""))
	v2 := info(second)
	must(s.ExitStudentSchool(ctx, admin.ID, second, v2.Version, date(-1), "Keluar lintas tahun", ""))
	must(s.ActivateYear(ctx, cur.ID, next.ID))
	v = info(student)
	must(s.ReadmitStudent(ctx, admin.ID, student, v.Version, v.ExitID, nextClass.ID, date(1), "Kembali besok", ""))
	rec, err = ms.Get(ctx, admin.ID, master.Students, student)
	must(err)
	if rec.Active || !rec.Transferred || rec.ReturnDate != date(1) {
		t.Fatal("future return activated early", rec)
	}
	_, err = ms.Save(ctx, admin.ID, master.Students, student, master.Input{Record: rec, Reason: "Koreksi identitas saat menunggu kembali"})
	must(err)
	var underlyingActive bool
	must(db.QueryRow(`SELECT active FROM app.students WHERE id=$1`, student).Scan(&underlyingActive))
	if !underlyingActive {
		t.Fatal("identity edit canceled scheduled reactivation")
	}
	var nextPlacement int64
	must(db.QueryRow(`SELECT id FROM app.student_placements WHERE student_id=$1 AND school_year_id=$2`, student, next.ID).Scan(&nextPlacement))
	for _, tc := range []struct {
		d     int
		valid bool
	}{{0, false}, {1, true}} {
		var got sql.NullInt64
		must(db.QueryRow(`SELECT app.student_placement_class_at($1,$2::date)`, nextPlacement, date(tc.d)).Scan(&got))
		if got.Valid != tc.valid {
			t.Fatal("new placement visible before return", got)
		}
	}
	third, err := s.CreateYear(ctx, cur.ID, "Return future "+testutil.Random(t)[:10])
	must(err)
	if _, err = s.TransitionPreview(ctx, cur.ID, next.ID, third.ID); err == nil {
		t.Fatal("pending return did not block transition")
	}
	v2 = info(second)
	if err = s.ReadmitStudent(ctx, admin.ID, second, v2.Version, v2.ExitID, a.ID, date(0), "Kembali", ""); err == nil {
		t.Fatal("inactive year accepted")
	}
	must(s.ReadmitStudent(ctx, admin.ID, second, v2.Version, v2.ExitID, nextClass.ID, date(0), "Kembali lintas tahun", ""))
	current := info(second)
	if current.ClassID != nextClass.ID {
		t.Fatal("next year return not assigned")
	}
	var old int64
	must(db.QueryRow(`SELECT class_id FROM app.student_placements WHERE id=$1`, p2).Scan(&old))
	if old != a.ID {
		t.Fatal("old year placement rewritten")
	}
	counts, err := s.PlacementCounts(ctx, admin.ID)
	must(err)
	if counts[nextClass.ID] != 1 {
		t.Fatal("future return counted early", counts)
	}
}
