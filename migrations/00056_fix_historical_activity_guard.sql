-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.reject_routine_structure_after_final() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF TG_TABLE_NAME='assessment_activities' THEN
        IF NEW.activity_type <> 'sumatif' THEN RETURN NEW; END IF;
    END IF;
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

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Pemulihan guard memerlukan strategi eksplisit'; END $$;
