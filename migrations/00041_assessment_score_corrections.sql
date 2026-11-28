-- Nilai awal tetap utuh; koreksi disimpan sebagai rangkaian append-only.
-- +goose Up
CREATE TABLE app.assessment_activity_score_corrections (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    initial_score_id bigint NOT NULL REFERENCES app.assessment_activity_initial_scores(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version > 0),
    before_score numeric(5,2) NOT NULL CHECK (before_score >= 0 AND before_score <= 100),
    after_score numeric(5,2) NOT NULL CHECK (after_score >= 0 AND after_score <= 100),
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    corrected_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    corrected_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT assessment_score_corrections_version_unique UNIQUE (initial_score_id, version)
);

CREATE INDEX assessment_score_corrections_latest_idx
    ON app.assessment_activity_score_corrections (initial_score_id, version DESC);

-- +goose StatementBegin
CREATE FUNCTION app.reject_assessment_score_correction_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Histori koreksi nilai bersifat append-only' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd

CREATE TRIGGER assessment_score_corrections_append_only
    BEFORE UPDATE OR DELETE ON app.assessment_activity_score_corrections
    FOR EACH ROW EXECUTE FUNCTION app.reject_assessment_score_correction_mutation();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Histori koreksi nilai memerlukan strategi pemulihan eksplisit'; END $$;
