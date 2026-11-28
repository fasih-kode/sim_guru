-- Keputusan mempertahankan Final tidak menciptakan versi Final atau menghapus penanda peninjauan.
-- +goose Up
CREATE TABLE app.semester_final_retentions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    final_version_id bigint NOT NULL REFERENCES app.semester_final_versions(id) ON DELETE RESTRICT,
    recorded_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    evidence_snapshot jsonb NOT NULL CHECK (jsonb_typeof(evidence_snapshot) = 'object'),
    evidence_sha256 bytea NOT NULL CHECK (octet_length(evidence_sha256) = 32),
    submission_key uuid NOT NULL,
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256) = 32),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(recorded_by, submission_key)
);
CREATE TRIGGER semester_final_retentions_append_only BEFORE UPDATE OR DELETE
    ON app.semester_final_retentions FOR EACH ROW EXECUTE FUNCTION app.reject_semester_final_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM app.semester_final_retentions) THEN
        RAISE EXCEPTION 'Histori keputusan mempertahankan Final tidak dapat dihapus';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE app.semester_final_retentions;
