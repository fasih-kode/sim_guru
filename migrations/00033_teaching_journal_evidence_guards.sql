-- +goose Up
-- Identitas baru tidak dapat menggunakan ulang identity yang sudah muncul di lineage.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.validate_teaching_journal_lineage_identity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source_logical bigint; stopped boolean;
BEGIN
 IF NEW.decision IN ('continue','stop') THEN
   SELECT logical_lesson_id INTO source_logical FROM app.teaching_journal_lineage_decisions WHERE to_revision_id=NEW.from_revision_id AND target_entry_id=NEW.source_entry_id ORDER BY id DESC LIMIT 1;
   IF source_logical IS NULL OR source_logical<>NEW.logical_lesson_id THEN
     RAISE EXCEPTION 'lineage decision does not preserve source logical identity' USING ERRCODE='23514';
   END IF;
   SELECT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions d JOIN app.teaching_journal_lineage_transitions tr ON tr.id=d.transition_id WHERE d.logical_lesson_id=NEW.logical_lesson_id AND d.decision='stop' AND tr.effective_from<= (SELECT effective_from FROM app.weekly_timetable_revisions WHERE id=NEW.to_revision_id)) INTO stopped;
   IF stopped THEN RAISE EXCEPTION 'stopped logical lesson cannot be reused' USING ERRCODE='23514'; END IF;
 END IF;
 IF NEW.decision IN ('new','cutover') AND EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions WHERE logical_lesson_id=NEW.logical_lesson_id) THEN
   RAISE EXCEPTION 'new logical lesson identity has already been used' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd

-- Pastikan setiap versi dan pemiliknya memiliki histori serta snapshot JP atomik.
-- +goose StatementBegin
CREATE FUNCTION app.validate_teaching_journal_version_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owner_id bigint;
BEGIN
 SELECT owner_user_id INTO owner_id FROM app.teaching_journals WHERE id=NEW.journal_id;
 IF owner_id IS NULL OR owner_id<>NEW.actor_id THEN
   RAISE EXCEPTION 'journal version actor must be the recorded owner' USING ERRCODE='23514';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM app.teaching_journal_history h WHERE h.version_id=NEW.id AND h.journal_id=NEW.journal_id AND h.version_no=NEW.version_no) THEN
   RAISE EXCEPTION 'journal version requires a history event' USING ERRCODE='23514';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM app.teaching_journal_version_periods p WHERE p.version_id=NEW.id) THEN
   RAISE EXCEPTION 'journal version requires period snapshots' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER teaching_journal_version_evidence_complete
 AFTER INSERT ON app.teaching_journal_versions DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION app.validate_teaching_journal_version_evidence();

-- +goose Down
-- Guard evidence append-only tidak dapat dilepas otomatis tanpa strategi pemulihan.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Teaching journal evidence guards require an explicit recovery strategy'; END $$;
-- +goose StatementEnd
