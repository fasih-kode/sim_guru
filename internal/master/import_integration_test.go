//go:build integration

package master_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestImportTransactionsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := master.Store{DB: db}
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	other, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	suffix := testutil.Random(t)[:12]
	student := func(name, nis string) master.ImportRow {
		return master.ImportRow{Row: 2, Input: master.Input{Record: master.Record{Name: name + suffix, BirthDate: "2013-01-02", Gender: "P", NIS: nis, Active: true}}}
	}
	rows := []master.ImportRow{student("Import A ", "00"+suffix), student("Import B ", "01"+suffix)}
	rows[1].Row = 3
	report, err := s.PreviewImport(ctx, admin.ID, master.Students, rows)
	if err != nil || report.Token == "" || report.ErrorCount != 0 {
		t.Fatal("valid preview rejected")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app.students WHERE name LIKE $1`, "Import %"+suffix).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("preview wrote master data")
	}
	if _, err = s.ConfirmImport(ctx, other.ID, master.Students, report.Token); err == nil {
		t.Fatal("another Admin confirmed someone else's preview")
	}
	if _, err = s.ConfirmImport(ctx, curriculum.ID, master.Students, report.Token); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("curriculum imported identities")
	}
	result, err := s.ConfirmImport(ctx, admin.ID, master.Students, report.Token)
	if err != nil || result.Count != 2 || result.AlreadyComplete {
		t.Fatal("confirmation failed")
	}
	result, err = s.ConfirmImport(ctx, admin.ID, master.Students, report.Token)
	if err != nil || !result.AlreadyComplete {
		t.Fatal("confirmation replay not idempotent")
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.students WHERE name LIKE $1`, "Import %"+suffix).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("confirmation replay duplicated records")
	}
	var payload string
	if err := db.QueryRow(`SELECT payload::text FROM app.master_imports WHERE token_hash=$1`, auth.TokenHash(report.Token)).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload != "[]" {
		t.Fatal("completed preview retained personal data")
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.student_history h JOIN app.students s ON s.id=h.entity_id WHERE s.name LIKE $1 AND h.reason='Impor Excel'`, "Import %"+suffix).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("import history missing")
	}
	duplicate, err := s.PreviewImport(ctx, admin.ID, master.Students, rows)
	if err != nil || duplicate.ErrorCount != 2 || duplicate.Token != "" {
		t.Fatal("existing identities not flagged")
	}
	duplicatedRows := []master.ImportRow{student("Duplicate ", "05"+suffix), student("Duplicate ", "05"+suffix)}
	duplicatedRows[1].Row = 3
	duplicate, err = s.PreviewImport(ctx, admin.ID, master.Students, duplicatedRows)
	if err != nil || duplicate.ErrorCount != 2 {
		t.Fatal("within-file duplicate not flagged on both rows")
	}
	staleRows := []master.ImportRow{student("Stale A ", "06"+suffix), student("Stale B ", "07"+suffix)}
	staleRows[1].Row = 3
	stale, err := s.PreviewImport(ctx, admin.ID, master.Students, staleRows)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(ctx, admin.ID, master.Students, 0, staleRows[1].Input); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmImport(ctx, admin.ID, master.Students, stale.Token); err == nil {
		t.Fatal("stale preview ignored newly created duplicate")
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.students WHERE name=$1`, staleRows[0].Input.Name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("conflicting batch left partial records")
	}
	expired, err := s.PreviewImport(ctx, admin.ID, master.Students, []master.ImportRow{student("Expired ", "08"+suffix)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app.master_imports SET expires_at=now()-interval '1 minute' WHERE token_hash=$1`, auth.TokenHash(expired.Token)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmImport(ctx, admin.ID, master.Students, expired.Token); err == nil {
		t.Fatal("expired preview confirmed")
	}
	if err := db.QueryRow(`SELECT payload::text FROM app.master_imports WHERE token_hash=$1`, auth.TokenHash(expired.Token)).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload != "[]" {
		t.Fatal("expired preview not scrubbed")
	}
	// Concurrent double-clicks serialize to a single commit and a receipt replay.
	concurrent, err := s.PreviewImport(ctx, admin.ID, master.Students, []master.ImportRow{student("Concurrent ", "09"+suffix)})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan master.ImportResult, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			r, err := s.ConfirmImport(ctx, admin.ID, master.Students, concurrent.Token)
			outcomes <- r
			failures <- err
		}()
	}
	fresh, replays := 0, 0
	for i := 0; i < 2; i++ {
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
		r := <-outcomes
		if r.AlreadyComplete {
			replays++
		} else {
			fresh++
		}
	}
	if fresh != 1 || replays != 1 {
		t.Fatal("concurrent confirmations not idempotent")
	}
	teacherRow := master.ImportRow{Row: 2, Input: master.Input{Record: master.Record{Name: "Guru Import " + suffix, NIP: "000" + suffix, Email: "guru@example.test", Phone: "0812", UserID: curriculum.ID, Active: true}}}
	teacherPreview, err := s.PreviewImport(ctx, admin.ID, master.Teachers, []master.ImportRow{teacherRow})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmImport(ctx, admin.ID, master.Teachers, teacherPreview.Token); err != nil {
		t.Fatal(err)
	}
	var accountID int64
	if err := db.QueryRow(`SELECT COALESCE(user_id,0) FROM app.teachers WHERE name=$1`, teacherRow.Input.Name).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if accountID != 0 {
		t.Fatal("import assigned an account")
	}
	teacherRow.Input.Name = "Different name " + suffix
	potential, err := s.PreviewImport(ctx, admin.ID, master.Teachers, []master.ImportRow{teacherRow})
	if err != nil || potential.ErrorCount != 1 || !strings.Contains(potential.Rows[0].Errors[0], "Potensi duplikat") {
		t.Fatal("teacher identifier collision not flagged for review")
	}
	revoked, err := s.PreviewImport(ctx, admin.ID, master.Students, []master.ImportRow{student("Revoked ", "10"+suffix)})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = accounts.Update(ctx, other.ID, admin.ID, auth.Input{Username: admin.Username, DisplayName: admin.DisplayName, Roles: []string{"kurikulum"}, Active: true, Version: admin.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmImport(ctx, admin.ID, master.Students, revoked.Token); !errors.Is(err, master.ErrForbidden) {
		t.Fatal("revoked Admin confirmed import")
	}
}
