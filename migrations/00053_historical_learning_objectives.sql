-- Provenance wajib hadir dalam transaksi yang sama sebelum TP historis disisipkan.
-- +goose Up
CREATE TABLE app.historical_learning_objective_insertions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    learning_objective_id bigint NOT NULL UNIQUE REFERENCES app.learning_objectives(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    semester_id bigint NOT NULL REFERENCES app.semesters(id) ON DELETE RESTRICT,
    class_id bigint NOT NULL REFERENCES app.classes(id) ON DELETE RESTRICT,
    subject_id bigint NOT NULL REFERENCES app.subjects(id) ON DELETE RESTRICT,
    actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    assessment_date date NOT NULL,
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    learning_evidence jsonb NOT NULL CHECK (jsonb_typeof(learning_evidence) = 'object'),
    assessment_evidence jsonb NOT NULL CHECK (jsonb_typeof(assessment_evidence) = 'object'),
    verification jsonb NOT NULL CHECK (jsonb_typeof(verification) = 'object'),
    impact_snapshot jsonb NOT NULL CHECK (jsonb_typeof(impact_snapshot) = 'object'),
    preview_sha256 bytea NOT NULL CHECK (octet_length(preview_sha256) = 32),
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256) = 32),
    submission_key uuid NOT NULL,
    transaction_id bigint NOT NULL DEFAULT txid_current(),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (actor_id, submission_key)
);
CREATE INDEX historical_learning_objective_insertions_context_idx
    ON app.historical_learning_objective_insertions(semester_id,class_id,subject_id,id);

-- +goose StatementBegin
CREATE FUNCTION app.set_historical_learning_objective_transaction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.transaction_id := txid_current();
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_learning_objective_insertions_transaction
    BEFORE INSERT ON app.historical_learning_objective_insertions
    FOR EACH ROW EXECUTE FUNCTION app.set_historical_learning_objective_transaction();

-- +goose StatementBegin
CREATE FUNCTION app.reject_historical_learning_objective_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Histori TP historis bersifat append-only' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_learning_objective_insertions_append_only
    BEFORE UPDATE OR DELETE ON app.historical_learning_objective_insertions
    FOR EACH ROW EXECUTE FUNCTION app.reject_historical_learning_objective_audit_mutation();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.reject_routine_structure_after_final() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF TG_TABLE_NAME = 'assessment_activities' THEN
        IF NEW.activity_type <> 'sumatif' THEN RETURN NEW; END IF;
    END IF;
    IF EXISTS (SELECT 1 FROM app.semester_final_versions f
               WHERE f.semester_id=NEW.semester_id AND f.class_id=NEW.class_id AND f.subject_id=NEW.subject_id) THEN
        IF TG_TABLE_NAME = 'assessment_activities' THEN
            RAISE EXCEPTION 'Penambahan kegiatan Sumatif rutin dikunci setelah Final pertama' USING ERRCODE='55000';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM app.historical_learning_objective_insertions h
                       WHERE h.learning_objective_id=NEW.id AND h.semester_id=NEW.semester_id
                         AND h.class_id=NEW.class_id AND h.subject_id=NEW.subject_id
                         AND h.actor_id=NEW.created_by AND h.transaction_id=txid_current()) THEN
            RAISE EXCEPTION 'Penambahan TP rutin dikunci setelah Final pertama' USING ERRCODE='55000';
        END IF;
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM app.historical_learning_objective_insertions) THEN
        RAISE EXCEPTION 'Histori TP historis berisi data; pemulihan eksplisit diperlukan';
    END IF;
END $$;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.reject_routine_structure_after_final() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF TG_TABLE_NAME = 'assessment_activities' THEN
        IF NEW.activity_type <> 'sumatif' THEN RETURN NEW; END IF;
    END IF;
    IF EXISTS (SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=NEW.semester_id AND f.class_id=NEW.class_id AND f.subject_id=NEW.subject_id) THEN
        IF TG_TABLE_NAME = 'assessment_activities' THEN
            RAISE EXCEPTION 'Penambahan kegiatan Sumatif rutin dikunci setelah Final pertama' USING ERRCODE='55000';
        END IF;
        RAISE EXCEPTION 'Penambahan TP rutin dikunci setelah Final pertama' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
DROP TRIGGER historical_learning_objective_insertions_append_only ON app.historical_learning_objective_insertions;
DROP FUNCTION app.reject_historical_learning_objective_audit_mutation();
DROP TRIGGER historical_learning_objective_insertions_transaction ON app.historical_learning_objective_insertions;
DROP FUNCTION app.set_historical_learning_objective_transaction();
DROP TABLE app.historical_learning_objective_insertions;
