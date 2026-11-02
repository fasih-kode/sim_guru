-- +goose Up
-- Fondasi Jurnal Mengajar. Tabel timetable/attendance lama tidak diubah.
CREATE TABLE app.teaching_journal_cutover_batches (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 cutover_date date NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 UNIQUE(cutover_date)
);

CREATE TABLE app.logical_lessons (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id) ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 UNIQUE(id,semester_id)
);

CREATE TABLE app.teaching_journal_lineage_transitions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 cutover_batch_id bigint REFERENCES app.teaching_journal_cutover_batches(id) ON DELETE RESTRICT,
 semester_id bigint NOT NULL REFERENCES app.semesters(id) ON DELETE RESTRICT,
 from_revision_id bigint,
 to_revision_id bigint NOT NULL,
 effective_from date NOT NULL,
 transition_kind text NOT NULL CHECK(transition_kind IN ('cutover','revision')),
 actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(id,from_revision_id,to_revision_id),
 FOREIGN KEY(from_revision_id,semester_id) REFERENCES app.weekly_timetable_revisions(id,semester_id) ON DELETE RESTRICT,
 FOREIGN KEY(to_revision_id,semester_id) REFERENCES app.weekly_timetable_revisions(id,semester_id) ON DELETE RESTRICT,
 CHECK((transition_kind='cutover' AND from_revision_id IS NULL AND cutover_batch_id IS NOT NULL) OR (transition_kind='revision' AND from_revision_id IS NOT NULL AND cutover_batch_id IS NULL))
);
CREATE UNIQUE INDEX teaching_journal_lineage_cutover_revision_uq ON app.teaching_journal_lineage_transitions(to_revision_id) WHERE transition_kind='cutover';
CREATE UNIQUE INDEX teaching_journal_lineage_pair_uq ON app.teaching_journal_lineage_transitions(from_revision_id,to_revision_id) WHERE transition_kind='revision';

CREATE TABLE app.teaching_journal_lineage_decisions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 transition_id bigint NOT NULL,
 from_revision_id bigint,
 to_revision_id bigint NOT NULL,
 source_entry_id bigint,
 target_entry_id bigint,
 logical_lesson_id bigint NOT NULL REFERENCES app.logical_lessons(id) ON DELETE RESTRICT,
 decision text NOT NULL CHECK(decision IN ('cutover','continue','new','stop')),
 actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(transition_id,from_revision_id,to_revision_id) REFERENCES app.teaching_journal_lineage_transitions(id,from_revision_id,to_revision_id) ON DELETE RESTRICT,
 FOREIGN KEY(source_entry_id,from_revision_id) REFERENCES app.weekly_timetable_entries(id,revision_id) ON DELETE RESTRICT,
 FOREIGN KEY(target_entry_id,to_revision_id) REFERENCES app.weekly_timetable_entries(id,revision_id) ON DELETE RESTRICT,
 CHECK((decision IN ('cutover','new') AND source_entry_id IS NULL AND target_entry_id IS NOT NULL) OR
       (decision='continue' AND source_entry_id IS NOT NULL AND target_entry_id IS NOT NULL) OR
       (decision='stop' AND source_entry_id IS NOT NULL AND target_entry_id IS NULL))
);
CREATE UNIQUE INDEX teaching_journal_lineage_source_uq ON app.teaching_journal_lineage_decisions(transition_id,source_entry_id) WHERE source_entry_id IS NOT NULL;
CREATE UNIQUE INDEX teaching_journal_lineage_target_uq ON app.teaching_journal_lineage_decisions(transition_id,target_entry_id) WHERE target_entry_id IS NOT NULL;
CREATE UNIQUE INDEX teaching_journal_lineage_target_logical_uq ON app.teaching_journal_lineage_decisions(to_revision_id,logical_lesson_id) WHERE target_entry_id IS NOT NULL;
CREATE INDEX teaching_journal_lineage_logical_idx ON app.teaching_journal_lineage_decisions(logical_lesson_id,recorded_at);

CREATE TABLE app.teaching_journals (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 logical_lesson_id bigint NOT NULL REFERENCES app.logical_lessons(id) ON DELETE RESTRICT,
 meeting_date date NOT NULL,
 semester_id bigint NOT NULL,
 revision_id bigint NOT NULL,
 timetable_entry_id bigint NOT NULL,
 assignment_id bigint NOT NULL REFERENCES app.teaching_assignments(id) ON DELETE RESTRICT,
 class_id bigint NOT NULL REFERENCES app.classes(id) ON DELETE RESTRICT,
 subject_id bigint NOT NULL REFERENCES app.subjects(id) ON DELETE RESTRICT,
 teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
 owner_user_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 class_snapshot text NOT NULL,
 subject_snapshot text NOT NULL,
 teacher_snapshot text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 current_version_no integer NOT NULL DEFAULT 1 CHECK(current_version_no > 0),
 UNIQUE(logical_lesson_id,meeting_date),
 UNIQUE(id,current_version_no),
 FOREIGN KEY(revision_id,semester_id) REFERENCES app.weekly_timetable_revisions(id,semester_id) ON DELETE RESTRICT,
 FOREIGN KEY(timetable_entry_id,revision_id) REFERENCES app.weekly_timetable_entries(id,revision_id) ON DELETE RESTRICT,
 FOREIGN KEY(logical_lesson_id,semester_id) REFERENCES app.logical_lessons(id,semester_id) ON DELETE RESTRICT
);

CREATE TABLE app.teaching_journal_versions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 journal_id bigint NOT NULL REFERENCES app.teaching_journals(id) ON DELETE RESTRICT,
 version_no integer NOT NULL CHECK(version_no > 0),
 state text NOT NULL CHECK(state IN ('draft','final')),
 outcome text NOT NULL CHECK(outcome IN ('held','not_held')),
 material text NOT NULL DEFAULT '' CHECK(char_length(material) <= 500),
 activity_notes text NOT NULL DEFAULT '' CHECK(char_length(activity_notes) <= 2000),
 not_held_reason text NOT NULL DEFAULT '',
 actual_starts_at time,
 actual_ends_at time,
 actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 correction_reason text,
 request_key uuid NOT NULL,
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(journal_id,version_no),
 UNIQUE(journal_id,request_key),
 UNIQUE(id,journal_id,version_no),
 CHECK((actual_starts_at IS NULL AND actual_ends_at IS NULL) OR (actual_starts_at IS NOT NULL AND actual_ends_at IS NOT NULL AND actual_starts_at < actual_ends_at)),
 CHECK(outcome='held' OR (length(btrim(not_held_reason)) BETWEEN 1 AND 500 AND material='' AND actual_starts_at IS NULL AND actual_ends_at IS NULL)),
 CHECK(state='draft' OR outcome='not_held' OR length(btrim(material)) > 0),
 CHECK(correction_reason IS NULL OR length(btrim(correction_reason)) BETWEEN 1 AND 500)
);
ALTER TABLE app.teaching_journals ADD CONSTRAINT teaching_journal_current_version_fk
 FOREIGN KEY(id,current_version_no) REFERENCES app.teaching_journal_versions(journal_id,version_no) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE app.teaching_journal_version_periods (
 version_id bigint NOT NULL REFERENCES app.teaching_journal_versions(id) ON DELETE RESTRICT,
 ordinal smallint NOT NULL CHECK(ordinal > 0),
 period_no smallint NOT NULL CHECK(period_no BETWEEN 1 AND 30),
 starts_at time NOT NULL,
 ends_at time NOT NULL,
 PRIMARY KEY(version_id,ordinal),
 UNIQUE(version_id,period_no),
 CHECK(starts_at < ends_at)
);

CREATE TABLE app.teaching_journal_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 journal_id bigint NOT NULL REFERENCES app.teaching_journals(id) ON DELETE RESTRICT,
 version_id bigint NOT NULL,
 version_no integer NOT NULL,
 event text NOT NULL CHECK(event IN ('created','saved_draft','finalized','corrected_final')),
 actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 reason text,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(version_id,journal_id,version_no) REFERENCES app.teaching_journal_versions(id,journal_id,version_no) ON DELETE RESTRICT,
 CHECK((event='corrected_final' AND reason IS NOT NULL AND length(btrim(reason)) BETWEEN 1 AND 500) OR (event<>'corrected_final' AND reason IS NULL))
);
CREATE INDEX teaching_journal_history_journal_idx ON app.teaching_journal_history(journal_id,id DESC);

-- +goose StatementBegin
CREATE FUNCTION app.reject_teaching_journal_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'teaching journal evidence is append-only' USING ERRCODE='55000'; END $$;
-- +goose StatementEnd
CREATE TRIGGER teaching_journal_versions_append_only BEFORE UPDATE OR DELETE ON app.teaching_journal_versions FOR EACH ROW EXECUTE FUNCTION app.reject_teaching_journal_evidence_mutation();
CREATE TRIGGER teaching_journal_periods_append_only BEFORE UPDATE OR DELETE ON app.teaching_journal_version_periods FOR EACH ROW EXECUTE FUNCTION app.reject_teaching_journal_evidence_mutation();
CREATE TRIGGER teaching_journal_history_append_only BEFORE UPDATE OR DELETE ON app.teaching_journal_history FOR EACH ROW EXECUTE FUNCTION app.reject_teaching_journal_evidence_mutation();
CREATE TRIGGER teaching_journal_lineage_decisions_append_only BEFORE UPDATE OR DELETE ON app.teaching_journal_lineage_decisions FOR EACH ROW EXECUTE FUNCTION app.reject_teaching_journal_evidence_mutation();
CREATE TRIGGER teaching_journal_lineage_transitions_append_only BEFORE UPDATE OR DELETE ON app.teaching_journal_lineage_transitions FOR EACH ROW EXECUTE FUNCTION app.reject_teaching_journal_evidence_mutation();

-- +goose StatementBegin
CREATE FUNCTION app.validate_teaching_journal_lineage_transition() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE transition_id bigint; transition_row app.teaching_journal_lineage_transitions%ROWTYPE; expected_count bigint; actual_count bigint;
BEGIN
 transition_id := COALESCE(NEW.transition_id, OLD.transition_id);
 SELECT * INTO transition_row FROM app.teaching_journal_lineage_transitions WHERE id=transition_id;
 IF NOT FOUND THEN RETURN NULL; END IF;
 SELECT count(*) INTO actual_count FROM app.teaching_journal_lineage_decisions WHERE teaching_journal_lineage_decisions.transition_id=transition_id;
 IF transition_row.transition_kind='cutover' THEN
   SELECT count(*) INTO expected_count FROM app.weekly_timetable_entries WHERE revision_id=transition_row.to_revision_id;
   IF actual_count<>expected_count OR EXISTS(SELECT 1 FROM app.weekly_timetable_entries e WHERE e.revision_id=transition_row.to_revision_id AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions d WHERE d.transition_id=transition_id AND d.target_entry_id=e.id AND d.decision='cutover')) THEN
     RAISE EXCEPTION 'cutover lineage must cover every target entry exactly once' USING ERRCODE='23514';
   END IF;
 ELSE
   SELECT count(*) INTO expected_count FROM app.weekly_timetable_entries WHERE revision_id=transition_row.from_revision_id;
   IF (SELECT count(*) FROM app.teaching_journal_lineage_decisions WHERE teaching_journal_lineage_decisions.transition_id=transition_id AND source_entry_id IS NOT NULL)<>expected_count OR EXISTS(SELECT 1 FROM app.weekly_timetable_entries e WHERE e.revision_id=transition_row.from_revision_id AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions d WHERE d.transition_id=transition_id AND d.source_entry_id=e.id)) THEN
     RAISE EXCEPTION 'lineage transition must cover every source entry exactly once' USING ERRCODE='23514';
   END IF;
   SELECT count(*) INTO expected_count FROM app.weekly_timetable_entries WHERE revision_id=transition_row.to_revision_id;
   IF (SELECT count(*) FROM app.teaching_journal_lineage_decisions WHERE teaching_journal_lineage_decisions.transition_id=transition_id AND target_entry_id IS NOT NULL)<>expected_count OR EXISTS(SELECT 1 FROM app.weekly_timetable_entries e WHERE e.revision_id=transition_row.to_revision_id AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions d WHERE d.transition_id=transition_id AND d.target_entry_id=e.id)) THEN
     RAISE EXCEPTION 'lineage transition must cover every target entry exactly once' USING ERRCODE='23514';
   END IF;
   IF EXISTS(SELECT 1 FROM app.weekly_timetable_entries e WHERE e.revision_id=transition_row.from_revision_id AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions p WHERE p.to_revision_id=transition_row.from_revision_id AND p.target_entry_id=e.id)) THEN
     RAISE EXCEPTION 'lineage source revision has unresolved or pre-cutover entries' USING ERRCODE='23514';
   END IF;
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER teaching_journal_lineage_complete
 AFTER INSERT ON app.teaching_journal_lineage_decisions DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION app.validate_teaching_journal_lineage_transition();

-- +goose StatementBegin
CREATE FUNCTION app.validate_teaching_journal_lineage_identity() RETURNS trigger LANGUAGE plpgsql AS $$
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
 IF NEW.decision='new' AND EXISTS(SELECT 1 FROM app.teaching_journal_lineage_decisions WHERE logical_lesson_id=NEW.logical_lesson_id) THEN
   RAISE EXCEPTION 'new logical lesson identity has already been used' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER teaching_journal_lineage_identity_guard BEFORE INSERT ON app.teaching_journal_lineage_decisions FOR EACH ROW EXECUTE FUNCTION app.validate_teaching_journal_lineage_identity();

-- +goose StatementBegin
CREATE FUNCTION app.guard_teaching_journal_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.id<>OLD.id OR NEW.logical_lesson_id<>OLD.logical_lesson_id OR NEW.meeting_date<>OLD.meeting_date OR NEW.semester_id<>OLD.semester_id OR NEW.revision_id<>OLD.revision_id OR NEW.timetable_entry_id<>OLD.timetable_entry_id OR NEW.assignment_id<>OLD.assignment_id OR NEW.class_id<>OLD.class_id OR NEW.subject_id<>OLD.subject_id OR NEW.teacher_id<>OLD.teacher_id OR NEW.owner_user_id<>OLD.owner_user_id OR NEW.class_snapshot<>OLD.class_snapshot OR NEW.subject_snapshot<>OLD.subject_snapshot OR NEW.teacher_snapshot<>OLD.teacher_snapshot OR NEW.created_at<>OLD.created_at OR NEW.current_version_no<>OLD.current_version_no+1 THEN
   RAISE EXCEPTION 'journal snapshot is immutable and version must advance by one' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER teaching_journals_snapshot_guard BEFORE UPDATE ON app.teaching_journals FOR EACH ROW EXECUTE FUNCTION app.guard_teaching_journal_snapshot();

-- Current lineage status is derived from immutable decisions; pending revisions do not mutate timetable.
CREATE VIEW app.teaching_journal_lineage_status AS
 SELECT r.id AS revision_id,r.semester_id,r.effective_from,
        CASE WHEN EXISTS(SELECT 1 FROM app.teaching_journal_lineage_transitions x WHERE x.to_revision_id=r.id)
             THEN 'resolved' ELSE 'pending' END AS lineage_status
 FROM app.weekly_timetable_revisions r
 WHERE r.status='published'
   AND EXISTS(SELECT 1 FROM app.teaching_journal_cutover_batches b WHERE r.effective_from >= b.cutover_date)
   AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_transitions c WHERE c.to_revision_id=r.id AND c.transition_kind='cutover');

-- +goose Down
-- Bukti jurnal dan keputusan lineage tidak aman untuk dihapus otomatis.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Teaching journal evidence requires an explicit recovery strategy'; END $$;
-- +goose StatementEnd
