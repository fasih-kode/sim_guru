//go:build integration

package master_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestMasterDataIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := master.Store{DB: db}
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := testutil.Random(t)[:12]
	student, err := s.Save(ctx, admin.ID, master.Students, 0, master.Input{Record: master.Record{Name: "Siswa " + suffix, BirthDate: "2013-06-10", Gender: "P", Phone: "PRIVATE_" + suffix, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Save(ctx, admin.ID, master.Students, 0, master.Input{Record: master.Record{Name: "Siswa kedua " + suffix, BirthDate: "2013-06-11", Gender: "L", Active: true}})
	if err != nil || second.ID == student.ID {
		t.Fatal("optional identifiers must allow multiple students with distinct permanent IDs")
	}
	originalID := student.ID
	student.NIS = "NIS_" + suffix
	student.NISN = "NISN_" + suffix
	student, err = s.Save(ctx, admin.ID, master.Students, student.ID, master.Input{Record: student, Reason: "Melengkapi nomor induk"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ nis, nisn string }{{student.NIS, ""}, {"", student.NISN}} {
		_, err = s.Save(ctx, admin.ID, master.Students, 0, master.Input{Record: master.Record{Name: "Duplikat", BirthDate: "2013-01-01", Gender: "L", NIS: tc.nis, NISN: tc.nisn, Active: true}})
		if !errors.Is(err, master.ErrConflict) {
			t.Fatal("duplicate student identifier accepted")
		}
	}
	denied := student
	denied.Name = "Unauthorized change"
	for _, actor := range []int64{curriculum.ID, teacher.ID, 0} {
		if _, err = s.Save(ctx, actor, master.Students, student.ID, master.Input{Record: denied, Reason: "Unauthorized"}); !errors.Is(err, master.ErrForbidden) {
			t.Fatal("non-Admin changed student identity")
		}
	}
	readOnly, err := s.Get(ctx, curriculum.ID, master.Students, student.ID)
	if err != nil || readOnly.Phone != "" {
		t.Fatal("read-only view leaked contact data")
	}
	if _, err = s.List(ctx, teacher.ID, master.Students, master.Filter{}); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("unscoped teacher read all students")
	}
	if _, err = s.History(ctx, curriculum.ID, master.Students, student.ID); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("read-only role accessed sensitive correction history")
	}
	student.Name = "Nama dibetulkan " + suffix
	student.Active = false
	oldVersion := student.Version
	student, err = s.Save(ctx, admin.ID, master.Students, student.ID, master.Input{Record: student, Reason: "Koreksi nama dan nonaktif"})
	if err != nil || student.ID != originalID {
		t.Fatal("correction changed permanent ID")
	}
	current, err := s.Get(ctx, curriculum.ID, master.Students, originalID)
	if err != nil || current.Name != student.Name || current.Active {
		t.Fatal("readers did not observe correction through same ID")
	}
	stale := student
	stale.Version = oldVersion
	stale.Name = "Stale update"
	if _, err = s.Save(ctx, admin.ID, master.Students, student.ID, master.Input{Record: stale, Reason: "Stale"}); !errors.Is(err, master.ErrConflict) {
		t.Fatal("stale edit accepted")
	}
	histories, err := s.History(ctx, admin.ID, master.Students, student.ID)
	if err != nil || len(histories) != 3 || !strings.Contains(histories[0].Before, "Siswa "+suffix) || !strings.Contains(histories[0].After, student.Name) {
		t.Fatal("identity/status history missing")
	}
	// A conflicting update must roll back the row, version, and history together.
	collision := second
	collision.NIS = student.NIS
	if _, err = s.Save(ctx, admin.ID, master.Students, second.ID, master.Input{Record: collision, Reason: "Conflict"}); !errors.Is(err, master.ErrConflict) {
		t.Fatal("inactive student number reused")
	}
	unchanged, err := s.Get(ctx, admin.ID, master.Students, second.ID)
	if err != nil || unchanged.Version != second.Version || unchanged.NIS != "" {
		t.Fatal("failed update left partial changes")
	}
	filtered, err := s.List(ctx, admin.ID, master.Students, master.Filter{Search: suffix, Status: "inactive"})
	if err != nil || len(filtered) != 1 || filtered[0].ID != student.ID {
		t.Fatal("student filtering failed")
	}
	// Teacher identity, account status, and roles are independent.
	guru, err := s.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru " + suffix, UserID: teacher.ID, Email: "guru@example.test", Phone: "PRIVATE_" + suffix, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru duplikat", UserID: teacher.ID, Active: true}}); !errors.Is(err, master.ErrConflict) {
		t.Fatal("account linked to multiple teachers")
	}
	guru.Name = "Guru dibetulkan " + suffix
	guru.Active = false
	guru, err = s.Save(ctx, admin.ID, master.Teachers, guru.ID, master.Input{Record: guru, Reason: "Perubahan status guru"})
	if err != nil {
		t.Fatal(err)
	}
	var active bool
	if err = db.QueryRowContext(ctx, `SELECT active FROM app.users WHERE id=$1`, teacher.ID).Scan(&active); err != nil || !active {
		t.Fatal("teacher status changed account status")
	}
	reader, err := s.Get(ctx, curriculum.ID, master.Teachers, guru.ID)
	if err != nil || reader.Name != guru.Name || reader.Phone != "" || reader.Email != "" || reader.UserID != 0 {
		t.Fatal("teacher reader identity/contact policy broken")
	}
	if _, err = s.Save(ctx, curriculum.ID, master.Teachers, guru.ID, master.Input{Record: guru, Reason: "Unauthorized"}); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("curriculum changed teacher identity")
	}
	// Subject codes remain reserved, including historical codes.
	subject, err := s.Save(ctx, curriculum.ID, master.Subjects, 0, master.Input{Record: master.Record{Name: "Mapel " + suffix, Code: " old_" + suffix + " ", Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	oldCode := subject.Code
	subject.Code = "NEW_" + suffix
	subject.Name = "Mapel berganti nama"
	subject.Active = false
	subject, err = s.Save(ctx, curriculum.ID, master.Subjects, subject.ID, master.Input{Record: subject, Reason: "Perubahan kode resmi"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(ctx, curriculum.ID, master.Subjects, 0, master.Input{Record: master.Record{Name: "Reused", Code: strings.ToLower(oldCode), Active: true}}); !errors.Is(err, master.ErrConflict) {
		t.Fatal("historical code reassigned")
	}
	another, err := s.Save(ctx, curriculum.ID, master.Subjects, 0, master.Input{Record: master.Record{Name: "Mapel lain", Code: "OTHER_" + suffix, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	otherBefore := another
	another.Code = oldCode
	if _, err = s.Save(ctx, curriculum.ID, master.Subjects, another.ID, master.Input{Record: another, Reason: "Attempt historical reuse"}); !errors.Is(err, master.ErrConflict) {
		t.Fatal("historical collision update accepted")
	}
	another, err = s.Get(ctx, curriculum.ID, master.Subjects, another.ID)
	if err != nil || another.Code != otherBefore.Code || another.Version != otherBefore.Version {
		t.Fatal("code reservation failure did not roll back update")
	}
	if _, err = s.Save(ctx, admin.ID, master.Subjects, 0, master.Input{Record: master.Record{Name: "Admin mapel", Code: "DENIED_" + suffix, Active: true}}); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("Admin acquired academic write access")
	}
	if _, err = s.List(ctx, admin.ID, master.Subjects, master.Filter{}); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("Admin automatically acquired academic read access")
	}
	history, err := s.History(ctx, curriculum.ID, master.Subjects, subject.ID)
	if err != nil || len(history) != 2 || !strings.Contains(history[0].Before, oldCode) {
		t.Fatal("subject history lost on rename")
	}
	// Concurrent forms with the same version cannot overwrite each other silently.
	outcomes := make(chan error, 2)
	for _, name := range []string{"Koreksi A", "Koreksi B"} {
		go func(name string) {
			r := second
			r.Name = name
			_, err := s.Save(ctx, admin.ID, master.Students, r.ID, master.Input{Record: r, Reason: name})
			outcomes <- err
		}(name)
	}
	wins, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-outcomes
		if err == nil {
			wins++
		} else if errors.Is(err, master.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal("concurrent master edits lost optimistic locking")
	}
}
