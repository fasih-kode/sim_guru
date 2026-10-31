-- +goose Up
-- Penguatan invariant pada fondasi 00031 setelah uji PostgreSQL pertama.
CREATE UNIQUE INDEX teaching_journal_single_cutover_uq ON app.teaching_journal_cutover_batches((true));

ALTER TABLE app.teaching_journal_lineage_decisions ADD COLUMN semester_id bigint;
UPDATE app.teaching_journal_lineage_decisions d SET semester_id=t.semester_id FROM app.teaching_journal_lineage_transitions t WHERE t.id=d.transition_id;
ALTER TABLE app.teaching_journal_lineage_decisions ALTER COLUMN semester_id SET NOT NULL;
ALTER TABLE app.teaching_journal_lineage_decisions ADD CONSTRAINT teaching_journal_lineage_decision_lesson_semester_fk FOREIGN KEY(logical_lesson_id,semester_id) REFERENCES app.logical_lessons(id,semester_id) ON DELETE RESTRICT;

DROP TRIGGER teaching_journal_lineage_complete ON app.teaching_journal_lineage_decisions;
-- Pemeriksaan dijalankan sekali saat COMMIT setelah semua keputusan dimasukkan.
-- +goose StatementBegin
CREATE FUNCTION app.validate_teaching_journal_lineage_header() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE expected_count bigint; actual_count bigint;
BEGIN
 IF NEW.transition_kind='cutover' THEN
   SELECT count(*) INTO expected_count FROM app.weekly_timetable_entries WHERE revision_id=NEW.to_revision_id;
   SELECT count(*) INTO actual_count FROM app.teaching_journal_lineage_decisions WHERE transition_id=NEW.id AND target_entry_id IS NOT NULL;
   IF expected_count=0 OR actual_count<>expected_count OR EXISTS(SELECT 1 FROM app.weekly_timetable_entries e WHERE e.revision_id=NEW.to_revision_id AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions d WHERE d.transition_id=NEW.id AND d.target_entry_id=e.id AND d.decision='cutover')) THEN
     RAISE EXCEPTION 'cutover lineage must cover every target entry exactly once' USING ERRCODE='23514';
   END IF;
 ELSE
   SELECT count(*) INTO expected_count FROM app.weekly_timetable_entries WHERE revision_id=NEW.from_revision_id;
   SELECT count(*) INTO actual_count FROM app.teaching_journal_lineage_decisions WHERE transition_id=NEW.id AND source_entry_id IS NOT NULL;
   IF expected_count=0 OR actual_count<>expected_count OR EXISTS(SELECT 1 FROM app.weekly_timetable_entries e WHERE e.revision_id=NEW.from_revision_id AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions d WHERE d.transition_id=NEW.id AND d.source_entry_id=e.id)) THEN
     RAISE EXCEPTION 'lineage transition must cover every source entry exactly once' USING ERRCODE='23514';
   END IF;
   SELECT count(*) INTO expected_count FROM app.weekly_timetable_entries WHERE revision_id=NEW.to_revision_id;
   SELECT count(*) INTO actual_count FROM app.teaching_journal_lineage_decisions WHERE transition_id=NEW.id AND target_entry_id IS NOT NULL;
   IF expected_count=0 OR actual_count<>expected_count OR EXISTS(SELECT 1 FROM app.weekly_timetable_entries e WHERE e.revision_id=NEW.to_revision_id AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions d WHERE d.transition_id=NEW.id AND d.target_entry_id=e.id)) THEN
     RAISE EXCEPTION 'lineage transition must cover every target entry exactly once' USING ERRCODE='23514';
   END IF;
   IF EXISTS(SELECT 1 FROM app.weekly_timetable_entries e WHERE e.revision_id=NEW.from_revision_id AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions p WHERE p.to_revision_id=NEW.from_revision_id AND p.target_entry_id=e.id)) THEN
     RAISE EXCEPTION 'lineage source revision has unresolved or pre-cutover entries' USING ERRCODE='23514';
   END IF;
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER teaching_journal_lineage_complete
 AFTER INSERT ON app.teaching_journal_lineage_transitions DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION app.validate_teaching_journal_lineage_header();

-- +goose StatementBegin
CREATE FUNCTION app.reject_logical_lesson_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'logical lesson identity is permanent' USING ERRCODE='55000'; END $$;
-- +goose StatementEnd
CREATE TRIGGER logical_lessons_permanent BEFORE UPDATE OR DELETE ON app.logical_lessons FOR EACH ROW EXECUTE FUNCTION app.reject_logical_lesson_mutation();

-- +goose Down
-- Constraint strengthening protects permanent journal lineage and cannot be rolled back safely.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Teaching journal lineage constraints require an explicit recovery strategy'; END $$;
-- +goose StatementEnd
