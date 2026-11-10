-- Hasil remedial ujian adalah bukti baru dan tidak mengubah nilai awal.
-- +goose Up
CREATE TABLE app.exam_score_remedials (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    initial_score_id bigint NOT NULL REFERENCES app.exam_initial_scores(id) ON DELETE RESTRICT,
    score numeric(5,2) NOT NULL CHECK (score BETWEEN 0 AND 100),
    remedial_date date NOT NULL,
    recorded_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    submission_key uuid NOT NULL,
    CONSTRAINT exam_score_remedials_submission_unique UNIQUE (recorded_by, submission_key)
);
CREATE INDEX exam_score_remedials_history_idx ON app.exam_score_remedials(initial_score_id, remedial_date, id);
-- +goose StatementBegin
CREATE FUNCTION app.reject_exam_score_remedial_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Histori remedial nilai ujian bersifat append-only' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER exam_score_remedials_append_only BEFORE UPDATE OR DELETE ON app.exam_score_remedials
    FOR EACH ROW EXECUTE FUNCTION app.reject_exam_score_remedial_mutation();
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Histori remedial ujian memerlukan strategi pemulihan eksplisit'; END $$;
