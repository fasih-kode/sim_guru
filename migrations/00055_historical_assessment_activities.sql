-- Kegiatan Sumatif historis dan daftar TP disimpan bersama audit dalam satu transaksi.
-- +goose Up
CREATE TABLE app.historical_assessment_activity_insertions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    activity_id bigint NOT NULL UNIQUE,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    activity_date date NOT NULL,
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    source_identity text NOT NULL CHECK (length(btrim(source_identity)) BETWEEN 1 AND 500),
    evidence jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    verification jsonb NOT NULL CHECK (jsonb_typeof(verification) = 'object'),
    impact_snapshot jsonb NOT NULL CHECK (jsonb_typeof(impact_snapshot) = 'object'),
    preview_sha256 bytea NOT NULL CHECK (octet_length(preview_sha256) = 32),
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256) = 32),
    submission_key uuid NOT NULL,
    transaction_id bigint NOT NULL DEFAULT txid_current(),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (actor_id, submission_key),
    UNIQUE (school_year_id, semester_id, class_id, subject_id, source_identity),
    FOREIGN KEY (activity_id, school_year_id, semester_id, class_id, subject_id)
        REFERENCES app.assessment_activities(id, school_year_id, semester_id, class_id, subject_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE app.historical_assessment_activity_objectives (
    audit_id bigint NOT NULL REFERENCES app.historical_assessment_activity_insertions(id) ON DELETE RESTRICT,
    learning_objective_id bigint NOT NULL REFERENCES app.learning_objectives(id) ON DELETE RESTRICT,
    PRIMARY KEY (audit_id, learning_objective_id)
);

-- +goose StatementBegin
CREATE FUNCTION app.guard_historical_activity_audit_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    NEW.transaction_id := txid_current();
    IF EXISTS (SELECT 1 FROM app.assessment_activities WHERE id=NEW.activity_id) OR NOT EXISTS (
        SELECT 1 FROM app.semester_final_versions f
        WHERE f.semester_id=NEW.semester_id AND f.class_id=NEW.class_id AND f.subject_id=NEW.subject_id
    ) THEN
        RAISE EXCEPTION 'Kegiatan historis memerlukan konteks ber-Final dan ID baru' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_activity_audit_insert BEFORE INSERT ON app.historical_assessment_activity_insertions
    FOR EACH ROW EXECUTE FUNCTION app.guard_historical_activity_audit_insert();

-- +goose StatementBegin
CREATE FUNCTION app.guard_historical_activity_detail_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF NOT EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h
        JOIN app.learning_objectives lo ON lo.id=NEW.learning_objective_id
         AND lo.school_year_id=h.school_year_id AND lo.semester_id=h.semester_id
         AND lo.class_id=h.class_id AND lo.subject_id=h.subject_id
        WHERE h.id=NEW.audit_id AND h.transaction_id=txid_current()) THEN
        RAISE EXCEPTION 'Rincian TP historis tidak sesuai transaksi atau konteks' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_activity_detail_insert BEFORE INSERT ON app.historical_assessment_activity_objectives
    FOR EACH ROW EXECUTE FUNCTION app.guard_historical_activity_detail_insert();

CREATE TRIGGER historical_activity_audit_append_only BEFORE UPDATE OR DELETE ON app.historical_assessment_activity_insertions
    FOR EACH ROW EXECUTE FUNCTION app.reject_historical_assessment_link_audit_mutation();
CREATE TRIGGER historical_activity_detail_append_only BEFORE UPDATE OR DELETE ON app.historical_assessment_activity_objectives
    FOR EACH ROW EXECUTE FUNCTION app.reject_historical_assessment_link_audit_mutation();

-- Pemeriksaan ditunda sampai commit agar seluruh himpunan TP dapat disisipkan atomik.
-- +goose StatementBegin
CREATE FUNCTION app.check_historical_activity_complete() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE aid bigint;
BEGIN
    IF TG_TABLE_NAME='historical_assessment_activity_insertions' THEN aid := NEW.id;
    ELSE aid := NEW.audit_id; END IF;
    IF NOT EXISTS (
        SELECT 1 FROM app.historical_assessment_activity_insertions h
        JOIN app.assessment_activities a ON a.id=h.activity_id
        WHERE h.id=aid AND a.activity_type='sumatif' AND a.teacher_id=h.teacher_id
          AND a.created_by=h.actor_id AND a.activity_date=h.activity_date
    ) OR NOT EXISTS (SELECT 1 FROM app.historical_assessment_activity_objectives d WHERE d.audit_id=aid)
    OR EXISTS (
        (SELECT d.learning_objective_id FROM app.historical_assessment_activity_objectives d WHERE d.audit_id=aid
         EXCEPT SELECT ao.learning_objective_id FROM app.assessment_activity_objectives ao JOIN app.historical_assessment_activity_insertions h ON h.activity_id=ao.activity_id WHERE h.id=aid)
        UNION ALL
        (SELECT ao.learning_objective_id FROM app.assessment_activity_objectives ao JOIN app.historical_assessment_activity_insertions h ON h.activity_id=ao.activity_id WHERE h.id=aid
         EXCEPT SELECT d.learning_objective_id FROM app.historical_assessment_activity_objectives d WHERE d.audit_id=aid)
    ) THEN
        RAISE EXCEPTION 'Kegiatan historis dan himpunan TP tidak lengkap' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER historical_activity_complete AFTER INSERT ON app.historical_assessment_activity_insertions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.check_historical_activity_complete();
CREATE CONSTRAINT TRIGGER historical_activity_detail_complete AFTER INSERT ON app.historical_assessment_activity_objectives
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.check_historical_activity_complete();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.reject_routine_structure_after_final() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF TG_TABLE_NAME='assessment_activities' AND NEW.activity_type <> 'sumatif' THEN RETURN NEW; END IF;
    IF EXISTS (SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=NEW.semester_id AND f.class_id=NEW.class_id AND f.subject_id=NEW.subject_id) THEN
        IF TG_TABLE_NAME='assessment_activities' THEN
            IF NOT EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h
                WHERE h.activity_id=NEW.id AND h.school_year_id=NEW.school_year_id AND h.semester_id=NEW.semester_id
                  AND h.class_id=NEW.class_id AND h.subject_id=NEW.subject_id AND h.teacher_id=NEW.teacher_id
                  AND h.actor_id=NEW.created_by AND h.activity_date=NEW.activity_date AND h.transaction_id=txid_current()) THEN
                RAISE EXCEPTION 'Penambahan Sumatif setelah Final memerlukan audit historis' USING ERRCODE='55000';
            END IF;
        ELSIF NOT EXISTS (SELECT 1 FROM app.historical_learning_objective_insertions h
            WHERE h.learning_objective_id=NEW.id AND h.semester_id=NEW.semester_id AND h.class_id=NEW.class_id
              AND h.subject_id=NEW.subject_id AND h.actor_id=NEW.created_by AND h.transaction_id=txid_current()) THEN
            RAISE EXCEPTION 'Penambahan TP rutin dikunci setelah Final pertama' USING ERRCODE='55000';
        END IF;
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.guard_assessment_objective_link_after_final() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE activity_kind text;
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF TG_OP <> 'INSERT' THEN
        IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'Relasi kegiatan tidak dapat diubah' USING ERRCODE='55000'; END IF;
        IF EXISTS (SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=OLD.semester_id AND f.class_id=OLD.class_id AND f.subject_id=OLD.subject_id) THEN
            RAISE EXCEPTION 'Relasi kegiatan terkunci setelah Final' USING ERRCODE='55000';
        END IF;
        RETURN OLD;
    END IF;
    SELECT activity_type INTO activity_kind FROM app.assessment_activities WHERE id=NEW.activity_id;
    IF activity_kind='sumatif' AND EXISTS (SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=NEW.semester_id AND f.class_id=NEW.class_id AND f.subject_id=NEW.subject_id)
       AND NOT EXISTS (SELECT 1 FROM app.historical_assessment_objective_links h WHERE h.activity_id=NEW.activity_id AND h.learning_objective_id=NEW.learning_objective_id
           AND h.school_year_id=NEW.school_year_id AND h.semester_id=NEW.semester_id AND h.class_id=NEW.class_id AND h.subject_id=NEW.subject_id AND h.transaction_id=txid_current())
       AND NOT EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h JOIN app.historical_assessment_activity_objectives d ON d.audit_id=h.id
           WHERE h.activity_id=NEW.activity_id AND d.learning_objective_id=NEW.learning_objective_id AND h.school_year_id=NEW.school_year_id
             AND h.semester_id=NEW.semester_id AND h.class_id=NEW.class_id AND h.subject_id=NEW.subject_id AND h.transaction_id=txid_current()) THEN
        RAISE EXCEPTION 'Relasi Sumatif setelah Final memerlukan koreksi historis' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app.guard_historical_initial_score() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=NEW.assessment_activity_id)
       OR EXISTS (SELECT 1 FROM app.historical_assessment_objective_links h WHERE h.activity_id=NEW.assessment_activity_id AND h.learning_objective_id=NEW.learning_objective_id) THEN
        RAISE EXCEPTION 'Nilai historis memerlukan proses pencatatan tersendiri' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_initial_score_guard BEFORE INSERT ON app.assessment_activity_initial_scores
    FOR EACH ROW EXECUTE FUNCTION app.guard_historical_initial_score();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Migration ini melindungi histori nilai; pemulihan eksplisit diperlukan'; END $$;
