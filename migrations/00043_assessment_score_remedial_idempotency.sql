-- Kunci unik mencegah retry satu submission menambah baris remedial kedua.
-- Baris CP10-07 lama tidak diubah; NULL hanya berlaku untuk histori sebelum
-- kunci submission diwajibkan oleh migration ini.
-- +goose Up
ALTER TABLE app.assessment_activity_score_remedials
    ADD COLUMN submission_key uuid;

ALTER TABLE app.assessment_activity_score_remedials
    ADD CONSTRAINT assessment_score_remedials_submission_key_required
        CHECK (submission_key IS NOT NULL) NOT VALID,
    ADD CONSTRAINT assessment_score_remedials_submission_unique
        UNIQUE (recorded_by, submission_key);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Kunci idempotensi remedial memerlukan strategi pemulihan eksplisit'; END $$;
