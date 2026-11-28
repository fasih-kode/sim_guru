-- Identitas sumber disimpan dalam bentuk kanonis agar variasi kapital/spasi
-- tidak melewati constraint duplikasi peristiwa.
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.guard_historical_activity_audit_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    NEW.transaction_id := txid_current();
    NEW.source_identity := lower(regexp_replace(btrim(NEW.source_identity), '\s+', ' ', 'g'));
    IF EXISTS (SELECT 1 FROM app.assessment_activities WHERE id=NEW.activity_id) OR NOT EXISTS (
        SELECT 1 FROM app.semester_final_versions f
        WHERE f.semester_id=NEW.semester_id AND f.class_id=NEW.class_id AND f.subject_id=NEW.subject_id
    ) THEN
        RAISE EXCEPTION 'Kegiatan historis memerlukan konteks ber-Final dan ID baru' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Pemulihan normalisasi identitas sumber memerlukan strategi eksplisit'; END $$;
