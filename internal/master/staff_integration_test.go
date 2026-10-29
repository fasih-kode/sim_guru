//go:build integration

package master_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStaffMasterIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := master.Store{DB: db}
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	tu, _ := testutil.User(t, db, "pegawai_tu")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := testutil.Random(t)[:12]

	staff, err := s.Save(ctx, admin.ID, master.Staff, 0, master.Input{Record: master.Record{Name: "Pegawai Uji " + suffix, EmployeeNumber: "", Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	staff.EmployeeNumber = "PGW-" + suffix
	staff.UserID = tu.ID
	staff.Email = "pegawai@example.test"
	staff.Phone = "0812"
	staff, err = s.Save(ctx, admin.ID, master.Staff, staff.ID, master.Input{Record: staff, Reason: "Tautkan akun pegawai"})
	if err != nil {
		t.Fatal(err)
	}
	if staff.UserID != tu.ID {
		t.Fatal("account link was not persisted")
	}
	if _, err = s.Save(ctx, curriculum.ID, master.Staff, 0, master.Input{Record: master.Record{Name: "Ditolak", Active: true}}); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("curriculum created staff")
	}
	if _, err = s.List(ctx, tu.ID, master.Staff, master.Filter{}); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("TU read admin master")
	}
	if _, err = s.History(ctx, curriculum.ID, master.Staff, staff.ID); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("non-admin read sensitive history")
	}
	if _, err = s.Save(ctx, admin.ID, master.Staff, 0, master.Input{Record: master.Record{Name: "Duplikat nomor", EmployeeNumber: staff.EmployeeNumber, Active: true}}); !errors.Is(err, master.ErrConflict) {
		t.Fatal("duplicate employee number accepted")
	}
	if _, err = s.Save(ctx, admin.ID, master.Staff, 0, master.Input{Record: master.Record{Name: "Akun rangkap", EmployeeNumber: "PGW2-" + suffix, UserID: tu.ID, Active: true}}); !errors.Is(err, master.ErrConflict) {
		t.Fatal("account linked to two staff")
	}
	staff.Active = false
	staff, err = s.Save(ctx, admin.ID, master.Staff, staff.ID, master.Input{Record: staff, Reason: "Penonaktifan uji"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := s.History(ctx, admin.ID, master.Staff, staff.ID)
	if err != nil || len(history) != 3 {
		t.Fatalf("deactivation must preserve history: %v / %d", err, len(history))
	}
}
