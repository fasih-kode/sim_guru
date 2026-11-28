-- Relasi Sumatif historis setelah Final memerlukan audit dalam transaksi yang sama.
-- +goose Up
CREATE TABLE app.historical_assessment_objective_links (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    activity_id bigint NOT NULL,
    learning_objective_id bigint NOT NULL,
    provenance_id bigint NOT NULL REFERENCES app.historical_learning_objective_insertions(id) ON DELETE RESTRICT,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    activity_date date NOT NULL,
    historical_date date NOT NULL,
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    evidence jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    verification jsonb NOT NULL CHECK (jsonb_typeof(verification) = 'object'),
    impact_snapshot jsonb NOT NULL CHECK (jsonb_typeof(impact_snapshot) = 'object'),
    preview_sha256 bytea NOT NULL CHECK (octet_length(preview_sha256) = 32),
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256) = 32),
    submission_key uuid NOT NULL,
    transaction_id bigint NOT NULL DEFAULT txid_current(),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (activity_id, learning_objective_id),
    UNIQUE (actor_id, submission_key),
    FOREIGN KEY (activity_id, learning_objective_id)
        REFERENCES app.assessment_activity_objectives(activity_id, learning_objective_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (activity_id, school_year_id, semester_id, class_id, subject_id)
        REFERENCES app.assessment_activities(id, school_year_id, semester_id, class_id, subject_id) ON DELETE RESTRICT,
    FOREIGN KEY (learning_objective_id, school_year_id, semester_id, class_id, subject_id)
        REFERENCES app.learning_objectives(id, school_year_id, semester_id, class_id, subject_id) ON DELETE RESTRICT
);

-- +goose StatementBegin
CREATE FUNCTION app.guard_historical_assessment_link_audit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    NEW.transaction_id := txid_current();
    IF EXISTS (SELECT 1 FROM app.assessment_activity_objectives ao WHERE ao.activity_id = NEW.activity_id AND ao.learning_objective_id = NEW.learning_objective_id) THEN
        RAISE EXCEPTION 'Relasi kegiatan dan TP sudah ada' USING ERRCODE = '23505';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM app.assessment_activities a
        JOIN app.historical_learning_objective_insertions h ON h.id = NEW.provenance_id
        WHERE a.id = NEW.activity_id AND a.activity_type = 'sumatif'
          AND a.teacher_id = NEW.teacher_id AND a.activity_date = NEW.activity_date
          AND h.learning_objective_id = NEW.learning_objective_id
          AND h.assessment_date = NEW.historical_date
          AND h.semester_id = NEW.semester_id AND h.class_id = NEW.class_id AND h.subject_id = NEW.subject_id
          AND EXISTS (SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id = NEW.semester_id AND f.class_id = NEW.class_id AND f.subject_id = NEW.subject_id)
    ) THEN
        RAISE EXCEPTION 'Sumber relasi historis tidak sesuai' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_assessment_link_audit_insert BEFORE INSERT ON app.historical_assessment_objective_links
    FOR EACH ROW EXECUTE FUNCTION app.guard_historical_assessment_link_audit();

-- +goose StatementBegin
CREATE FUNCTION app.reject_historical_assessment_link_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Histori koreksi relasi bersifat append-only' USING ERRCODE = '55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_assessment_link_audit_append_only BEFORE UPDATE OR DELETE ON app.historical_assessment_objective_links
    FOR EACH ROW EXECUTE FUNCTION app.reject_historical_assessment_link_audit_mutation();

-- +goose StatementBegin
CREATE FUNCTION app.guard_assessment_objective_link_after_final() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    activity_kind text;
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF TG_OP <> 'INSERT' THEN
        IF TG_OP = 'UPDATE' THEN
            RAISE EXCEPTION 'Relasi kegiatan tidak dapat diubah' USING ERRCODE = '55000';
        END IF;
        IF EXISTS (SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id = OLD.semester_id AND f.class_id = OLD.class_id AND f.subject_id = OLD.subject_id) THEN
            RAISE EXCEPTION 'Relasi kegiatan terkunci setelah Final' USING ERRCODE = '55000';
        END IF;
        RETURN OLD;
    END IF;
    SELECT activity_type INTO activity_kind FROM app.assessment_activities WHERE id = NEW.activity_id;
    IF activity_kind = 'sumatif' AND EXISTS (
        SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id = NEW.semester_id AND f.class_id = NEW.class_id AND f.subject_id = NEW.subject_id
    ) AND NOT EXISTS (
        SELECT 1 FROM app.historical_assessment_objective_links h
        WHERE h.activity_id = NEW.activity_id AND h.learning_objective_id = NEW.learning_objective_id
          AND h.school_year_id = NEW.school_year_id AND h.semester_id = NEW.semester_id
          AND h.class_id = NEW.class_id AND h.subject_id = NEW.subject_id AND h.transaction_id = txid_current()
    ) THEN
        RAISE EXCEPTION 'Relasi Sumatif setelah Final memerlukan koreksi historis' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER assessment_objective_link_final_guard BEFORE INSERT OR UPDATE OR DELETE ON app.assessment_activity_objectives
    FOR EACH ROW EXECUTE FUNCTION app.guard_assessment_objective_link_after_final();

-- +goose Down
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM app.historical_assessment_objective_links) THEN
        RAISE EXCEPTION 'Histori koreksi relasi berisi data; pemulihan eksplisit diperlukan';
    END IF;
END $$;
DROP TRIGGER assessment_objective_link_final_guard ON app.assessment_activity_objectives;
DROP FUNCTION app.guard_assessment_objective_link_after_final();
DROP TRIGGER historical_assessment_link_audit_append_only ON app.historical_assessment_objective_links;
DROP FUNCTION app.reject_historical_assessment_link_audit_mutation();
DROP TRIGGER historical_assessment_link_audit_insert ON app.historical_assessment_objective_links;
DROP FUNCTION app.guard_historical_assessment_link_audit();
DROP TABLE app.historical_assessment_objective_links;
