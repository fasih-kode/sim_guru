-- Kolom bertipe eksplisit memakai default metadata agar histori tidak ditulis ulang.
-- +goose Up
ALTER TABLE app.semester_final_versions ADD COLUMN final_type text NOT NULL DEFAULT 'normal';
ALTER TABLE app.semester_final_versions ALTER COLUMN recommendation_score DROP NOT NULL;
ALTER TABLE app.semester_final_versions DROP CONSTRAINT semester_final_versions_check;
ALTER TABLE app.semester_final_versions ADD CONSTRAINT semester_final_versions_type_check CHECK (
 (final_type='normal' AND recommendation_score IS NOT NULL AND (final_score=recommendation_score OR length(btrim(reason)) BETWEEN 1 AND 500)) OR
 (final_type='exception' AND recommendation_score IS NULL AND length(btrim(reason)) BETWEEN 1 AND 500)
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM app.semester_final_versions WHERE final_type='exception') THEN
  RAISE EXCEPTION 'Histori Final pengecualian tidak dapat dipertahankan oleh schema lama';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE app.semester_final_versions DROP CONSTRAINT semester_final_versions_type_check;
ALTER TABLE app.semester_final_versions ALTER COLUMN recommendation_score SET NOT NULL;
ALTER TABLE app.semester_final_versions ADD CONSTRAINT semester_final_versions_check CHECK (final_score=recommendation_score OR length(btrim(reason)) BETWEEN 1 AND 500);
ALTER TABLE app.semester_final_versions DROP COLUMN final_type;
