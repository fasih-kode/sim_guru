-- Riwayat koreksi uraian TP mempertahankan identitas TP dan seluruh relasi nilai.
-- +goose Up
CREATE TABLE app.learning_objective_definition_revisions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    learning_objective_id bigint NOT NULL REFERENCES app.learning_objectives(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version > 0),
    definition_before text NOT NULL CHECK (length(btrim(definition_before)) > 0),
    definition_after text NOT NULL CHECK (length(btrim(definition_after)) > 0),
    change_type text NOT NULL CHECK (change_type IN ('rutin','redaksi','substansi')),
    reason text NOT NULL DEFAULT '',
    actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    impact_snapshot jsonb NOT NULL CHECK (jsonb_typeof(impact_snapshot) = 'object'),
    preview_sha256 bytea NOT NULL CHECK (octet_length(preview_sha256) = 32),
    submission_key uuid NOT NULL,
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256) = 32),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT learning_objective_definition_revisions_objective_version_unique
        UNIQUE (learning_objective_id, version),
    CONSTRAINT learning_objective_definition_revisions_actor_submission_unique
        UNIQUE (actor_id, submission_key),
    CONSTRAINT learning_objective_definition_revisions_reason_check CHECK (
        (change_type = 'rutin' AND reason = '') OR
        (change_type IN ('redaksi','substansi') AND length(btrim(reason)) BETWEEN 1 AND 500)
    )
);

CREATE INDEX learning_objective_definition_revisions_history_idx
    ON app.learning_objective_definition_revisions (learning_objective_id, version DESC);

-- Riwayat TP adalah bukti koreksi dan tidak boleh ditimpa atau dihapus.
-- +goose StatementBegin
CREATE FUNCTION app.reject_learning_objective_revision_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Riwayat koreksi TP bersifat append-only' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER learning_objective_definition_revisions_append_only
    BEFORE UPDATE OR DELETE ON app.learning_objective_definition_revisions
    FOR EACH ROW EXECUTE FUNCTION app.reject_learning_objective_revision_mutation();

-- +goose Down
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM app.learning_objective_definition_revisions) THEN
        RAISE EXCEPTION 'Riwayat koreksi TP berisi data; pemulihan eksplisit diperlukan sebelum migration Down';
    END IF;
END $$;
DROP TRIGGER learning_objective_definition_revisions_append_only ON app.learning_objective_definition_revisions;
DROP FUNCTION app.reject_learning_objective_revision_mutation();
DROP TABLE app.learning_objective_definition_revisions;
