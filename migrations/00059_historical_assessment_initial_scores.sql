-- Nilai Sumatif historis hanya dapat dicatat bersama bukti per siswa/TP.
-- Audit dan nilai awal harus hadir serta cocok dalam transaksi yang sama.
-- +goose Up
CREATE TABLE app.historical_assessment_score_batches (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    assessment_activity_id bigint NOT NULL,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    activity_date date NOT NULL,
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    verification text NOT NULL CHECK (length(btrim(verification)) BETWEEN 1 AND 1000),
    impact_snapshot jsonb NOT NULL CHECK (jsonb_typeof(impact_snapshot) = 'object'),
    preview_sha256 bytea NOT NULL CHECK (octet_length(preview_sha256) = 32),
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256) = 32),
    submission_key uuid NOT NULL,
    transaction_id bigint NOT NULL DEFAULT txid_current(),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (actor_id, submission_key),
    UNIQUE (id, assessment_activity_id, school_year_id, semester_id, class_id, subject_id, teacher_id, actor_id, activity_date),
    FOREIGN KEY (assessment_activity_id, school_year_id, semester_id, class_id, subject_id)
        REFERENCES app.assessment_activities(id, school_year_id, semester_id, class_id, subject_id)
        ON DELETE RESTRICT
);

CREATE TABLE app.historical_assessment_initial_score_audits (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id bigint NOT NULL,
    assessment_activity_id bigint NOT NULL,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    teacher_id bigint NOT NULL,
    actor_id bigint NOT NULL,
    activity_date date NOT NULL,
    learning_objective_id bigint NOT NULL,
    student_id bigint NOT NULL REFERENCES app.students(id) ON DELETE RESTRICT,
    placement_id bigint NOT NULL,
    score numeric(5,2) NOT NULL CHECK (score >= 0 AND score <= 100),
    source_kind text NOT NULL CHECK (source_kind IN ('aplikasi', 'arsip_manual')),
    source_identity text NOT NULL CHECK (length(btrim(source_identity)) BETWEEN 1 AND 500),
    source_locator text NOT NULL CHECK (length(btrim(source_locator)) BETWEEN 1 AND 1000),
    source_result text NOT NULL CHECK (length(btrim(source_result)) BETWEEN 1 AND 500),
    verification text NOT NULL CHECK (length(btrim(verification)) BETWEEN 1 AND 1000),
    transaction_id bigint NOT NULL DEFAULT txid_current(),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (assessment_activity_id, learning_objective_id, student_id),
    UNIQUE (batch_id, assessment_activity_id, learning_objective_id, student_id),
    FOREIGN KEY (batch_id, assessment_activity_id, school_year_id, semester_id, class_id, subject_id, teacher_id, actor_id, activity_date)
        REFERENCES app.historical_assessment_score_batches(id, assessment_activity_id, school_year_id, semester_id, class_id, subject_id, teacher_id, actor_id, activity_date)
        ON DELETE RESTRICT,
    FOREIGN KEY (placement_id, student_id, school_year_id)
        REFERENCES app.student_placements(id, student_id, school_year_id) ON DELETE RESTRICT,
    FOREIGN KEY (assessment_activity_id, learning_objective_id, student_id)
        REFERENCES app.assessment_activity_initial_scores(assessment_activity_id, learning_objective_id, student_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX historical_assessment_score_audits_batch_idx
    ON app.historical_assessment_initial_score_audits(batch_id, id);

-- Seluruh writer struktur/nilai sejarah memakai advisory lock yang sama.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.guard_historical_initial_score() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF EXISTS (
        SELECT 1 FROM app.historical_assessment_activity_insertions h
        WHERE h.activity_id=NEW.assessment_activity_id
    ) OR EXISTS (
        SELECT 1 FROM app.historical_assessment_objective_links h
        WHERE h.activity_id=NEW.assessment_activity_id AND h.learning_objective_id=NEW.learning_objective_id
    ) THEN
        IF NOT EXISTS (
            SELECT 1 FROM app.historical_assessment_initial_score_audits d
            JOIN app.historical_assessment_score_batches b ON b.id=d.batch_id
            WHERE d.assessment_activity_id=NEW.assessment_activity_id
              AND d.learning_objective_id=NEW.learning_objective_id
              AND d.student_id=NEW.student_id
              AND d.school_year_id=NEW.school_year_id AND d.semester_id=NEW.semester_id
              AND d.class_id=NEW.class_id AND d.subject_id=NEW.subject_id
              AND d.teacher_id=(SELECT a.teacher_id FROM app.assessment_activities a WHERE a.id=NEW.assessment_activity_id)
              AND d.actor_id=NEW.recorded_by AND d.placement_id=NEW.placement_id
              AND d.activity_date=NEW.activity_date_snapshot AND d.score=NEW.score
              AND d.transaction_id=txid_current() AND b.transaction_id=txid_current()
        ) THEN
            RAISE EXCEPTION 'Nilai pada pasangan historis memerlukan bukti per siswa dan TP dalam transaksi yang sama' USING ERRCODE='55000';
        END IF;
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app.guard_historical_score_batch_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    NEW.transaction_id := txid_current();
    IF NOT EXISTS (
        SELECT 1 FROM app.assessment_activities a
        JOIN app.teachers t ON t.id=a.teacher_id AND t.user_id=NEW.actor_id
        WHERE a.id=NEW.assessment_activity_id AND a.activity_type='sumatif'
          AND a.school_year_id=NEW.school_year_id AND a.semester_id=NEW.semester_id
          AND a.class_id=NEW.class_id AND a.subject_id=NEW.subject_id
          AND a.teacher_id=NEW.teacher_id AND a.activity_date=NEW.activity_date
          AND EXISTS (SELECT 1 FROM app.semester_final_versions f
              WHERE f.semester_id=a.semester_id AND f.class_id=a.class_id AND f.subject_id=a.subject_id)
          AND (EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=a.id)
            OR EXISTS (SELECT 1 FROM app.historical_assessment_objective_links h WHERE h.activity_id=a.id))
    ) THEN
        RAISE EXCEPTION 'Batch nilai historis tidak sesuai pemilik kegiatan atau konteks ber-Final' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_score_batch_insert
    BEFORE INSERT ON app.historical_assessment_score_batches
    FOR EACH ROW EXECUTE FUNCTION app.guard_historical_score_batch_insert();

-- +goose StatementBegin
CREATE FUNCTION app.guard_historical_score_detail_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE activity_kind text;
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    NEW.transaction_id := txid_current();
    SELECT a.activity_type INTO activity_kind FROM app.assessment_activities a WHERE a.id=NEW.assessment_activity_id;
    IF activity_kind <> 'sumatif' OR NOT EXISTS (
        SELECT 1 FROM app.historical_assessment_score_batches b
        WHERE b.id=NEW.batch_id AND b.assessment_activity_id=NEW.assessment_activity_id
          AND b.school_year_id=NEW.school_year_id AND b.semester_id=NEW.semester_id
          AND b.class_id=NEW.class_id AND b.subject_id=NEW.subject_id
          AND b.teacher_id=NEW.teacher_id AND b.actor_id=NEW.actor_id
          AND b.activity_date=NEW.activity_date AND b.transaction_id=txid_current()
    ) OR NOT (
        EXISTS (SELECT 1 FROM app.historical_assessment_objective_links l
            WHERE l.activity_id=NEW.assessment_activity_id AND l.learning_objective_id=NEW.learning_objective_id
              AND l.teacher_id=NEW.teacher_id)
        OR EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h
            JOIN app.historical_assessment_activity_objectives d ON d.audit_id=h.id
            WHERE h.activity_id=NEW.assessment_activity_id AND d.learning_objective_id=NEW.learning_objective_id
              AND h.teacher_id=NEW.teacher_id)
    ) OR EXISTS (
        SELECT 1 FROM app.assessment_activity_initial_scores s
        WHERE s.assessment_activity_id=NEW.assessment_activity_id
          AND s.learning_objective_id=NEW.learning_objective_id AND s.student_id=NEW.student_id
    ) OR NOT EXISTS (
        SELECT 1 FROM app.assessment_activities a
        JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id AND ao.learning_objective_id=NEW.learning_objective_id
        JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id
          AND lo.school_year_id=a.school_year_id AND lo.semester_id=a.semester_id
          AND lo.class_id=a.class_id AND lo.subject_id=a.subject_id
        JOIN app.student_placements p ON p.id=NEW.placement_id
          AND p.student_id=NEW.student_id AND p.school_year_id=a.school_year_id
        WHERE a.id=NEW.assessment_activity_id AND a.school_year_id=NEW.school_year_id
          AND a.semester_id=NEW.semester_id AND a.class_id=NEW.class_id AND a.subject_id=NEW.subject_id
          AND app.student_placement_class_at(p.id,a.activity_date)=a.class_id
          AND p.starts_on IS NOT NULL AND p.starts_on<=a.activity_date
    ) THEN
        RAISE EXCEPTION 'Rincian audit nilai tidak cocok dengan pasangan historis atau penempatan pada tanggal kegiatan' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_score_detail_insert
    BEFORE INSERT ON app.historical_assessment_initial_score_audits
    FOR EACH ROW EXECUTE FUNCTION app.guard_historical_score_detail_insert();

-- +goose StatementBegin
CREATE FUNCTION app.check_historical_score_audit_complete() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE bid bigint;
BEGIN
    IF TG_TABLE_NAME='historical_assessment_score_batches' THEN bid := NEW.id;
    ELSIF TG_TABLE_NAME='historical_assessment_initial_score_audits' THEN bid := NEW.batch_id;
    ELSE
        SELECT d.batch_id INTO bid FROM app.historical_assessment_initial_score_audits d
        WHERE d.assessment_activity_id=NEW.assessment_activity_id
          AND d.learning_objective_id=NEW.learning_objective_id AND d.student_id=NEW.student_id;
        IF bid IS NULL THEN RETURN NULL; END IF;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM app.historical_assessment_initial_score_audits d WHERE d.batch_id=bid)
       OR EXISTS (
          SELECT 1 FROM app.historical_assessment_initial_score_audits d
          LEFT JOIN app.assessment_activity_initial_scores s
            ON s.assessment_activity_id=d.assessment_activity_id
           AND s.learning_objective_id=d.learning_objective_id AND s.student_id=d.student_id
          WHERE d.batch_id=bid AND (s.id IS NULL OR s.score<>d.score OR s.recorded_by<>d.actor_id
            OR s.placement_id<>d.placement_id OR s.activity_date_snapshot<>d.activity_date)
       ) THEN
        RAISE EXCEPTION 'Batch nilai historis harus memiliki pasangan nilai dan audit yang identik' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER historical_score_batch_complete
    AFTER INSERT ON app.historical_assessment_score_batches
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.check_historical_score_audit_complete();
CREATE CONSTRAINT TRIGGER historical_score_detail_complete
    AFTER INSERT ON app.historical_assessment_initial_score_audits
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.check_historical_score_audit_complete();
CREATE CONSTRAINT TRIGGER historical_score_value_complete
    AFTER INSERT ON app.assessment_activity_initial_scores
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.check_historical_score_audit_complete();

-- Nilai awal hasil transkripsi historis tidak boleh diubah atau dihapus.
-- Perbaikan nilai yang telah tercatat mengikuti jalur koreksi tersendiri.
-- +goose StatementBegin
CREATE FUNCTION app.guard_historical_initial_score_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF EXISTS (SELECT 1 FROM app.historical_assessment_initial_score_audits d
        WHERE d.assessment_activity_id=OLD.assessment_activity_id
          AND d.learning_objective_id=OLD.learning_objective_id AND d.student_id=OLD.student_id) THEN
        RAISE EXCEPTION 'Nilai awal historis berprovenance bersifat immutable' USING ERRCODE='55000';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_initial_score_immutable
    BEFORE UPDATE OR DELETE ON app.assessment_activity_initial_scores
    FOR EACH ROW EXECUTE FUNCTION app.guard_historical_initial_score_mutation();

-- Audit batch dan detail hanya dapat ditambah.
CREATE TRIGGER historical_score_batch_append_only
    BEFORE UPDATE OR DELETE ON app.historical_assessment_score_batches
    FOR EACH ROW EXECUTE FUNCTION app.reject_historical_assessment_link_audit_mutation();
CREATE TRIGGER historical_score_detail_append_only
    BEFORE UPDATE OR DELETE ON app.historical_assessment_initial_score_audits
    FOR EACH ROW EXECUTE FUNCTION app.reject_historical_assessment_link_audit_mutation();

-- +goose Down
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM app.historical_assessment_score_batches)
       OR EXISTS (SELECT 1 FROM app.historical_assessment_initial_score_audits) THEN
        RAISE EXCEPTION 'Audit nilai historis berisi data; pemulihan eksplisit diperlukan';
    END IF;
    RAISE EXCEPTION 'Migration ini mengganti guard historis; pemulihan memerlukan strategi eksplisit';
END $$;
