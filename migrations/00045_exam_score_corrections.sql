-- Nilai awal ASTS/ASAS tidak berubah; koreksi dicatat sebagai histori append-only.
-- +goose Up
CREATE TABLE app.exam_score_corrections (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    initial_score_id bigint NOT NULL REFERENCES app.exam_initial_scores(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version > 0),
    before_score numeric(5,2) NOT NULL CHECK (before_score BETWEEN 0 AND 100),
    after_score numeric(5,2) NOT NULL CHECK (after_score BETWEEN 0 AND 100),
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    corrected_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    corrected_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT exam_score_corrections_version_unique UNIQUE (initial_score_id, version)
);
CREATE INDEX exam_score_corrections_latest_idx ON app.exam_score_corrections(initial_score_id, version DESC);
-- +goose StatementBegin
CREATE FUNCTION app.reject_exam_score_correction_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Histori koreksi nilai ujian bersifat append-only' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER exam_score_corrections_append_only BEFORE UPDATE OR DELETE ON app.exam_score_corrections
    FOR EACH ROW EXECUTE FUNCTION app.reject_exam_score_correction_mutation();
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Histori koreksi nilai ujian memerlukan strategi pemulihan eksplisit'; END $$;
