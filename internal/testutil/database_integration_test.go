//go:build integration

package testutil

import (
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

// TestDatabaseUsesPreMigratedRuntime verifies test connections never need DDL.
func TestDatabaseUsesPreMigratedRuntime(t *testing.T) {
	db := Database(t, "../../.env")
	var actual string
	var canCreate, ownsSchema bool
	if err := db.QueryRow(`SELECT current_database(), has_schema_privilege(current_user, 'app', 'CREATE'),
		(SELECT pg_get_userbyid(nspowner) = current_user FROM pg_namespace WHERE nspname = 'app')`).Scan(&actual, &canCreate, &ownsSchema); err != nil {
		t.Fatal(err)
	}
	if canCreate || ownsSchema {
		t.Fatalf("test runtime owns or can create in application schema: %s", actual)
	}
}

func TestDirectTPRuntimeCannotDisableGuardIntegration(t *testing.T) {
	db := Database(t, "../../.env")
	var directWrite, helperExec, owner, enabled, functionOwner bool
	err := db.QueryRow(`SELECT has_table_privilege(current_user,'app.direct_tp_guard','INSERT,UPDATE,DELETE'),
	 has_function_privilege(current_user,'app.direct_tp_lock_guard(bigint,bigint,bigint,bigint)','EXECUTE'),
	 (SELECT pg_get_userbyid(relowner)=current_user FROM pg_class WHERE oid='app.semester_final_versions'::regclass),
	 (SELECT tgenabled='O' FROM pg_trigger WHERE tgrelid='app.semester_final_versions'::regclass AND tgname='direct_tp_final_guard'),
	 (SELECT pg_get_userbyid(proowner)=current_user FROM pg_proc WHERE oid='app.direct_tp_score_guard()'::regprocedure)`).Scan(&directWrite, &helperExec, &owner, &enabled, &functionOwner)
	if err != nil || directWrite || helperExec || owner || functionOwner || !enabled {
		t.Fatalf("hak guard tidak aman: %v %v %v %v %v %v", directWrite, helperExec, owner, enabled, functionOwner, err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = tx.Exec(`ALTER TABLE app.semester_final_versions DISABLE TRIGGER direct_tp_final_guard`)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "42501" {
		t.Fatalf("menonaktifkan trigger: %v", err)
	}
}
