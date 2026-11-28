//go:build integration

package testutil

import (
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

func TestValidateDatabaseTarget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		actual   string
		actualID int64
		wantErr  bool
	}{
		{name: "exact target", actual: "sim_guru_rd02_s06_20261123_test", actualID: 84604},
		{name: "wrong database", actual: "sim_guru_test", actualID: 84604, wantErr: true},
		{name: "wrong OID", actual: "sim_guru_rd02_s06_20261123_test", actualID: 84605, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDatabaseTarget(tc.actual, tc.actualID, "sim_guru_rd02_s06_20261123_test", 84604)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateDatabaseTarget() error = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}

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
