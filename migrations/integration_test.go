//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"github.com/fasih-kode/sim_guru/internal/database"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestMigrationsIntegration(t *testing.T) {
	if err := godotenv.Load("../.env"); err != nil && !os.IsNotExist(err) {
		t.Fatal("cannot read local test configuration")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || dsn == "" || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("TEST_DATABASE_URL must explicitly target a database ending in _test")
	}
	if dsn == os.Getenv("DATABASE_URL") {
		t.Fatal("test and application database must differ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var actual, user string
	var superuser, createDB, createRole bool
	err = db.QueryRowContext(ctx, "SELECT current_database(), current_user, rolsuper, rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname=current_user").Scan(&actual, &user, &superuser, &createDB, &createRole)
	if err != nil {
		t.Fatal("cannot inspect test role")
	}
	if !strings.HasSuffix(actual, "_test") || superuser || createDB || createRole {
		t.Fatal("unsafe test database or privileges")
	}
	p, err := Provider(db)
	if err != nil {
		t.Fatal(err)
	}
	verifyTeachingJournalNotHeldContextUpgrade(t, db)
	if _, err = p.Up(ctx); err != nil {
		t.Fatalf("migration up failed: %v", err)
	}
	results, err := p.Up(ctx)
	if err != nil || len(results) != 0 {
		t.Fatal("migration up is not idempotent")
	}
	current, target, err := p.GetVersions(ctx)
	if err != nil || current != target || target < 51 {
		t.Fatal("unexpected migration version")
	}
	var retentionSchema bool
	if err = db.QueryRow(`SELECT to_regclass('app.semester_final_retentions') IS NOT NULL
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='app.semester_final_retentions'::regclass AND tgname='semester_final_retentions_append_only' AND tgenabled='O')
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='app.semester_final_retentions'::regclass AND contype='u')`).Scan(&retentionSchema); err != nil || !retentionSchema {
		t.Fatal("missing immutable Final retention audit")
	}
	var exceptionSchema bool
	if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='app' AND table_name='semester_final_versions' AND column_name='final_type' AND is_nullable='NO')
		AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='app' AND table_name='semester_final_versions' AND column_name='recommendation_score' AND is_nullable='YES')
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='semester_final_versions_type_check' AND convalidated)`).Scan(&exceptionSchema); err != nil || !exceptionSchema {
		t.Fatal("missing Final type or exception constraint")
	}
	var batchSchema bool
	if err = db.QueryRow(`SELECT to_regclass('app.semester_final_batches') IS NOT NULL
		AND to_regclass('app.semester_final_batch_items') IS NOT NULL
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='semester_final_batches_append_only' AND NOT tgisinternal)
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='semester_final_batch_items_append_only' AND NOT tgisinternal)
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='app.semester_final_batches'::regclass AND contype='u' AND pg_get_constraintdef(oid) LIKE 'UNIQUE (recorded_by, submission_key)%')`).Scan(&batchSchema); err != nil || !batchSchema {
		t.Fatal("missing immutable Final batch tables or submission uniqueness")
	}
	var semesterFinalSchema bool
	if err = db.QueryRow(`SELECT to_regclass('app.semester_final_versions') IS NOT NULL
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='semester_final_versions_append_only' AND NOT tgisinternal)
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='semester_final_lock_new_objectives' AND NOT tgisinternal)
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='semester_final_lock_new_sumatif' AND NOT tgisinternal)
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='semester_final_versions_recommendation_score_check' AND contype='c')
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='semester_final_versions_final_score_check' AND contype='c')
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='semester_final_versions_unique' AND contype='u')`).Scan(&semesterFinalSchema); err != nil || !semesterFinalSchema {
		t.Fatal("missing immutable semester final table, score constraints, or routine structure guards")
	}
	var exists bool
	if err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='app')").Scan(&exists); err != nil || !exists {
		t.Fatal("missing application schema")
	}
	var roleCount int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM app.roles WHERE code IN('pegawai_tu','pengelola_kehadiran_guru')`).Scan(&roleCount); err != nil || roleCount != 2 {
		t.Fatal("missing TU attendance roles")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.teacher_attendance_correction_requests') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing teacher attendance correction request table")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.teacher_attendance_reports') IS NOT NULL AND to_regclass('app.teacher_attendance_report_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing teacher attendance report tables")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.staff_members') IS NOT NULL AND to_regclass('app.staff_member_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing staff master tables")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.staff_attendance_events') IS NOT NULL AND to_regclass('app.staff_attendance_photos') IS NOT NULL AND to_regclass('app.staff_attendance_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing staff attendance tables")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.student_guidance_records') IS NOT NULL AND to_regclass('app.student_guidance_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing student guidance tables")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.student_cases') IS NOT NULL AND to_regclass('app.student_case_entries') IS NOT NULL AND to_regclass('app.student_case_counseling_notes') IS NOT NULL AND to_regclass('app.student_case_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing case coordination tables")
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.logical_lessons') IS NOT NULL AND to_regclass('app.teaching_journal_lineage_transitions') IS NOT NULL AND to_regclass('app.teaching_journal_lineage_decisions') IS NOT NULL AND to_regclass('app.teaching_journals') IS NOT NULL AND to_regclass('app.teaching_journal_versions') IS NOT NULL AND to_regclass('app.teaching_journal_version_periods') IS NOT NULL AND to_regclass('app.teaching_journal_history') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("missing teaching journal foundation tables")
	}
	var activitySchema bool
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.assessment_activities') IS NOT NULL AND to_regclass('app.assessment_activity_objectives') IS NOT NULL AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='learning_objectives_id_context_unique' AND contype='u') AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='assessment_activities_id_context_unique' AND contype='u') AND (SELECT count(*)=2 FROM pg_constraint WHERE conrelid='app.assessment_activity_objectives'::regclass AND contype='f')`).Scan(&activitySchema); err != nil || !activitySchema {
		t.Fatal("missing CP10-04 activity tables, context keys, or composite foreign keys")
	}
	var scoreSchema bool
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.assessment_activity_initial_scores') IS NOT NULL
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='assessment_initial_scores_occurrence_student_unique' AND contype='u')
		AND (SELECT count(*)=5 FROM pg_constraint WHERE conrelid='app.assessment_activity_initial_scores'::regclass AND contype='f')
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='student_placements_id_student_year_unique' AND contype='u')`).Scan(&scoreSchema); err != nil || !scoreSchema {
		t.Fatal("missing CP10-05 initial score table, uniqueness, or composite foreign keys")
	}
	var historicalScoreSchema bool
	if err = db.QueryRowContext(ctx, `SELECT
		to_regclass('app.historical_assessment_score_batches') IS NOT NULL
		AND to_regclass('app.historical_assessment_initial_score_audits') IS NOT NULL
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='app.historical_assessment_score_batches'::regclass AND contype='u' AND pg_get_constraintdef(oid) LIKE 'UNIQUE (actor_id, submission_key)%')
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='app.historical_assessment_initial_score_audits'::regclass AND contype='u' AND pg_get_constraintdef(oid) LIKE 'UNIQUE (assessment_activity_id, learning_objective_id, student_id)%')
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='app.historical_assessment_initial_score_audits'::regclass AND contype='f' AND condeferrable)
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='app.assessment_activity_initial_scores'::regclass AND tgname='historical_initial_score_guard' AND tgenabled='O')
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='app.assessment_activity_initial_scores'::regclass AND tgname='historical_initial_score_immutable' AND tgenabled='O')
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='app.historical_assessment_score_batches'::regclass AND tgname='historical_score_batch_append_only' AND tgenabled='O')
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='app.historical_assessment_initial_score_audits'::regclass AND tgname='historical_score_detail_append_only' AND tgenabled='O')`).Scan(&historicalScoreSchema); err != nil || !historicalScoreSchema {
		t.Fatal("missing CP10-23 append-only score provenance, deferred integrity, idempotency, or guards")
	}
	var scoreCorrections bool
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.assessment_activity_score_corrections') IS NOT NULL
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='assessment_score_corrections_version_unique' AND contype='u')
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='assessment_score_corrections_append_only' AND NOT tgisinternal)
		AND (SELECT count(*)=2 FROM pg_constraint WHERE conrelid='app.assessment_activity_score_corrections'::regclass AND contype='f')`).Scan(&scoreCorrections); err != nil || !scoreCorrections {
		t.Fatal("missing CP10-06 append-only corrections, version uniqueness, or foreign keys")
	}
	var scoreRemedials bool
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('app.assessment_activity_score_remedials') IS NOT NULL
		AND (SELECT count(*)=2 FROM pg_constraint WHERE conrelid='app.assessment_activity_score_remedials'::regclass AND contype='f')
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='assessment_score_remedials_append_only' AND NOT tgisinternal)
		AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='assessment_score_remedials_sumatif_guard' AND NOT tgisinternal)
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='assessment_activity_score_remedials_score_check' AND contype='c')
		AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='app' AND table_name='assessment_activity_score_remedials' AND column_name='submission_key' AND udt_name='uuid')
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='assessment_score_remedials_submission_key_required' AND contype='c' AND NOT convalidated)
		AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='assessment_score_remedials_submission_unique' AND contype='u' AND pg_get_constraintdef(oid) LIKE 'UNIQUE (recorded_by, submission_key)%')`).Scan(&scoreRemedials); err != nil || !scoreRemedials {
		t.Fatal("missing CP10-07 remedial table, constraints, or immutable/sumatif triggers")
	}
	var historicalMappingTable, historicalMappingUnique, historicalMappingAppendOnly bool
	if err = db.QueryRowContext(ctx, `SELECT
		to_regclass('app.teaching_journal_historical_mappings') IS NOT NULL,
		(SELECT count(*)=2 FROM pg_constraint WHERE conrelid='app.teaching_journal_historical_mappings'::regclass AND contype='u'),
		EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='teaching_journal_historical_mappings_append_only' AND NOT tgisinternal)`).Scan(&historicalMappingTable, &historicalMappingUnique, &historicalMappingAppendOnly); err != nil || !historicalMappingTable || !historicalMappingUnique || !historicalMappingAppendOnly {
		t.Fatal("missing version-36 historical mapping table, occurrence uniqueness, or append-only guard")
	}
	var notHeldContextConstraint bool
	if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='teaching_journal_versions_not_held_context_check' AND contype='c')`).Scan(&notHeldContextConstraint); err != nil || !notHeldContextConstraint {
		t.Fatal("missing constraint for optional non-held journal context")
	}
	var occurrenceUnique, currentVersionFK, entryRevisionFK, appendOnly, lineageCompleteness, journalCompleteness bool
	if err = db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM pg_constraint WHERE conname='teaching_journals_logical_lesson_id_meeting_date_key' AND contype='u'),
		EXISTS(SELECT 1 FROM pg_constraint WHERE conname='teaching_journal_current_version_fk' AND contype='f' AND condeferrable),
		EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid WHERE t.relname='teaching_journal_lineage_decisions' AND c.contype='f' AND pg_get_constraintdef(c.oid) LIKE '%weekly_timetable_entries%'),
		EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='teaching_journal_versions_append_only' AND NOT tgisinternal),
		EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='teaching_journal_lineage_complete' AND NOT tgisinternal),
		EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='teaching_journal_version_evidence_complete' AND NOT tgisinternal)`).Scan(&occurrenceUnique, &currentVersionFK, &entryRevisionFK, &appendOnly, &lineageCompleteness, &journalCompleteness); err != nil || !occurrenceUnique || !currentVersionFK || !entryRevisionFK || !appendOnly || !lineageCompleteness || !journalCompleteness {
		t.Fatal("missing teaching journal uniqueness, relationships, or immutable evidence guards")
	}
	var nullableCaseLink, ownerGuard, historyAppendOnly, handoffEvent bool
	if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='app' AND table_name='student_guidance_records' AND column_name='case_id' AND is_nullable='YES'),
		EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='student_cases_owner_stage_guard' AND NOT tgisinternal),
		EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='student_case_history_append_only' AND NOT tgisinternal),
		EXISTS(SELECT 1 FROM pg_constraint WHERE conname='student_case_history_event_type_check' AND pg_get_constraintdef(oid) LIKE '%wali_handoff%')`).Scan(&nullableCaseLink, &ownerGuard, &historyAppendOnly, &handoffEvent); err != nil || !nullableCaseLink || !ownerGuard || !historyAppendOnly || !handoffEvent {
		t.Fatal("case link must remain optional and case guard/history trigger must exist")
	}
	var guidanceRecordFK, guidanceStatus, guidanceHistoryFK bool
	if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='student_guidance_records_student_id_fkey' AND contype='f' AND confdeltype='a'),
		EXISTS(SELECT 1 FROM pg_constraint WHERE conname='student_guidance_records_status_check' AND contype='c'),
		EXISTS(SELECT 1 FROM pg_constraint WHERE conname='student_guidance_history_record_id_fkey' AND contype='f' AND confdeltype='a')`).Scan(&guidanceRecordFK, &guidanceStatus, &guidanceHistoryFK); err != nil || !guidanceRecordFK || !guidanceStatus || !guidanceHistoryFK {
		t.Fatal("cannot inspect guidance foreign keys")
	}
	var uniqueActions, photoPair, staffForeignKey bool
	if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='app' AND t.relname='staff_attendance_events' AND c.contype='u' AND pg_get_constraintdef(c.oid) LIKE 'UNIQUE (staff_member_id, on_date, action)%'), EXISTS(SELECT 1 FROM pg_constraint WHERE conname='staff_attendance_event_photo_fk' AND contype='f'), EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid WHERE t.relname='staff_attendance_events' AND c.contype='f' AND pg_get_constraintdef(c.oid) LIKE '%staff_member_id%staff_members%')`).Scan(&uniqueActions, &photoPair, &staffForeignKey); err != nil || !uniqueActions || !photoPair || !staffForeignKey {
		t.Fatalf("missing staff event uniqueness or relationships (unique=%t photo_fk=%t staff_fk=%t)", uniqueActions, photoPair, staffForeignKey)
	}
	var overlap bool
	if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='teacher_attendance_reports_no_overlap' AND contype='x')`).Scan(&overlap); err != nil || !overlap {
		t.Fatal("missing report overlap exclusion constraint")
	}
	// Verify transaction rollback using a deliberately failing migration. No existing data is dropped.
	broken := fstest.MapFS{"00002_probe.sql": {Data: []byte("-- +goose Up\nCREATE TABLE app.foundation_rollback_probe(id integer);\nSELECT 1/0;\n-- +goose Down\nSELECT 1;\n")}}
	probe, err := goose.NewProvider(goose.DialectPostgres, db, broken, goose.WithDisableVersioning(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = probe.Up(ctx); err == nil {
		t.Fatal("expected failing migration")
	}
	if err = db.QueryRowContext(ctx, "SELECT to_regclass('app.foundation_rollback_probe') IS NULL").Scan(&exists); err != nil || !exists {
		t.Fatal("failed migration left partial state")
	}
}

// verifyTeachingJournalNotHeldContextUpgrade applies only migration 00035 to
// a synthetic copy of the pre-upgrade constraint and one historical row. This
// runs only before the application schema exists in the newly created test DB.
func verifyTeachingJournalNotHeldContextUpgrade(t *testing.T, db *sql.DB) {
	t.Helper()
	var appSchemaExists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='app')`).Scan(&appSchemaExists); err != nil {
		t.Fatal("cannot inspect isolated upgrade-test schema")
	}
	if appSchemaExists {
		t.Log("upgrade fixture skipped: isolated database already contains the application schema")
		return
	}
	if _, err := db.Exec(`CREATE SCHEMA app;
		CREATE TABLE app.teaching_journal_versions (
			id bigint PRIMARY KEY,
			outcome text NOT NULL,
			material text NOT NULL,
			not_held_reason text NOT NULL,
			actual_starts_at timestamptz,
			actual_ends_at timestamptz,
			CONSTRAINT teaching_journal_versions_check1 CHECK (
				outcome='held' OR (
					length(btrim(not_held_reason)) BETWEEN 1 AND 500
					AND material=''
					AND actual_starts_at IS NULL
					AND actual_ends_at IS NULL
				)
			)
		);
		INSERT INTO app.teaching_journal_versions(id,outcome,material,not_held_reason)
		VALUES (1,'not_held','','Kegiatan dibatalkan sebelum pencatatan konteks baru');`); err != nil {
		t.Fatal("cannot prepare representative pre-00035 schema and historical row")
	}
	ownedSchema := true
	t.Cleanup(func() {
		if ownedSchema {
			_, _ = db.Exec(`DROP SCHEMA app CASCADE`)
		}
	})
	migrationSQL, err := Files.ReadFile("00035_teaching_journal_not_held_context.sql")
	if err != nil {
		t.Fatal("cannot read migration 00035")
	}
	upgradeFS := fstest.MapFS{"00035_teaching_journal_not_held_context.sql": &fstest.MapFile{Data: migrationSQL}}
	upgrade, err := goose.NewProvider(goose.DialectPostgres, db, upgradeFS, goose.WithDisableVersioning(true))
	if err != nil {
		t.Fatal("cannot initialize isolated 34-to-35 upgrade test")
	}
	if _, err = upgrade.Up(context.Background()); err != nil {
		t.Fatalf("migration 00035 failed against representative version-34 schema: %v", err)
	}
	var oldReason, oldMaterial string
	if err = db.QueryRow(`SELECT not_held_reason,material FROM app.teaching_journal_versions WHERE id=1`).Scan(&oldReason, &oldMaterial); err != nil || oldReason != "Kegiatan dibatalkan sebelum pencatatan konteks baru" || oldMaterial != "" {
		t.Fatalf("migration did not preserve historical row: reason=%q material=%q err=%v", oldReason, oldMaterial, err)
	}
	if _, err = db.Exec(`INSERT INTO app.teaching_journal_versions(id,outcome,material,not_held_reason)
		VALUES (2,'not_held','Materi sebagai konteks, bukan materi yang diajarkan','Jadwal dibatalkan')`); err != nil {
		t.Fatalf("upgraded constraint rejected optional not-held context: %v", err)
	}
	if _, err = db.Exec(`DROP SCHEMA app CASCADE`); err != nil {
		t.Fatal("cannot remove schema created solely for the isolated upgrade test")
	}
	ownedSchema = false
}
