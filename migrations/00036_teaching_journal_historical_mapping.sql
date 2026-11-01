-- Pemetaan aditif untuk occurrence pra-cutover yang ditetapkan manual Kurikulum.
-- Record menyimpan bukti sumber dan tidak dapat diedit/dihapus.
-- +goose Up
CREATE TABLE app.teaching_journal_historical_mappings (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 cutover_batch_id bigint NOT NULL REFERENCES app.teaching_journal_cutover_batches(id) ON DELETE RESTRICT,
 logical_lesson_id bigint NOT NULL,
 semester_id bigint NOT NULL,
 revision_id bigint NOT NULL,
 timetable_entry_id bigint NOT NULL,
 meeting_date date NOT NULL,
 assignment_id bigint NOT NULL REFERENCES app.teaching_assignments(id) ON DELETE RESTRICT,
 class_id bigint NOT NULL REFERENCES app.classes(id) ON DELETE RESTRICT,
 subject_id bigint NOT NULL REFERENCES app.subjects(id) ON DELETE RESTRICT,
 teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
 owner_user_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 class_snapshot text NOT NULL,
 subject_snapshot text NOT NULL,
 teacher_snapshot text NOT NULL,
 periods_snapshot jsonb NOT NULL CHECK(jsonb_typeof(periods_snapshot)='array' AND jsonb_array_length(periods_snapshot) BETWEEN 1 AND 30),
 actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(revision_id,timetable_entry_id,meeting_date),
 UNIQUE(logical_lesson_id,meeting_date),
 FOREIGN KEY(revision_id,semester_id) REFERENCES app.weekly_timetable_revisions(id,semester_id) ON DELETE RESTRICT,
 FOREIGN KEY(timetable_entry_id,revision_id) REFERENCES app.weekly_timetable_entries(id,revision_id) ON DELETE RESTRICT,
 FOREIGN KEY(logical_lesson_id,semester_id) REFERENCES app.logical_lessons(id,semester_id) ON DELETE RESTRICT
);
CREATE INDEX teaching_journal_historical_owner_date_idx ON app.teaching_journal_historical_mappings(owner_user_id,meeting_date);
CREATE TRIGGER teaching_journal_historical_mappings_append_only BEFORE UPDATE OR DELETE ON app.teaching_journal_historical_mappings FOR EACH ROW EXECUTE FUNCTION app.reject_teaching_journal_evidence_mutation();

-- +goose Down
DROP TRIGGER teaching_journal_historical_mappings_append_only ON app.teaching_journal_historical_mappings;
DROP TABLE app.teaching_journal_historical_mappings;
