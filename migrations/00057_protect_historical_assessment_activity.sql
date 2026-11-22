-- Kegiatan historis CP10-22 dan himpunan TP yang diaudit tidak dapat diubah setelah konfirmasi.
-- +goose Up
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
    IF EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=NEW.activity_id)
       AND NOT EXISTS (
           SELECT 1 FROM app.historical_assessment_activity_insertions h
           JOIN app.historical_assessment_activity_objectives d ON d.audit_id=h.id
           WHERE h.activity_id=NEW.activity_id AND d.learning_objective_id=NEW.learning_objective_id
             AND h.transaction_id=txid_current()
       ) THEN
        RAISE EXCEPTION 'Himpunan TP kegiatan historis tidak dapat diperluas' USING ERRCODE='55000';
    END IF;
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
CREATE FUNCTION app.reject_historical_assessment_activity_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=OLD.id) THEN
        RAISE EXCEPTION 'Metadata kegiatan historis bersifat tetap; gunakan koreksi beralasan' USING ERRCODE='55000';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER historical_assessment_activity_immutable
    BEFORE UPDATE OR DELETE ON app.assessment_activities
    FOR EACH ROW EXECUTE FUNCTION app.reject_historical_assessment_activity_mutation();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Pemulihan perlindungan histori memerlukan strategi eksplisit'; END $$;
