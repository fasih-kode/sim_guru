-- +goose Up
-- Batch cutover dicatat per snapshot scope aktif; revision yang sama tetap hanya boleh dipotong sekali.
DROP INDEX app.teaching_journal_single_cutover_uq;
ALTER TABLE app.teaching_journal_cutover_batches DROP CONSTRAINT teaching_journal_cutover_batches_cutover_date_key;
CREATE UNIQUE INDEX teaching_journal_cutover_batch_semester_uq ON app.teaching_journal_lineage_transitions(cutover_batch_id,semester_id) WHERE transition_kind='cutover';

-- +goose Down
-- Beberapa snapshot cutover pada satu tanggal tidak dapat diringkas aman menjadi satu batch.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Cutover scope history requires an explicit recovery strategy'; END $$;
-- +goose StatementEnd
