-- +goose Up
-- Catatan materi untuk pertemuan yang tidak terlaksana hanya konteks opsional,
-- bukan bukti materi diajarkan. Alasan wajib dan waktu aktual tetap dilarang.
ALTER TABLE app.teaching_journal_versions
  DROP CONSTRAINT teaching_journal_versions_check1;

ALTER TABLE app.teaching_journal_versions
  ADD CONSTRAINT teaching_journal_versions_not_held_context_check
  CHECK (
    outcome <> 'not_held'
    OR (
      length(btrim(not_held_reason)) BETWEEN 1 AND 500
      AND actual_starts_at IS NULL
      AND actual_ends_at IS NULL
    )
  );

-- +goose Down
-- Tolak rollback bila sudah ada konteks materi yang hanya didukung schema baru.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (
    SELECT 1 FROM app.teaching_journal_versions
    WHERE outcome = 'not_held' AND material <> ''
  ) THEN
    RAISE EXCEPTION 'Non-held journal context exists; explicit recovery is required before rollback';
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE app.teaching_journal_versions
  DROP CONSTRAINT teaching_journal_versions_not_held_context_check;

ALTER TABLE app.teaching_journal_versions
  ADD CONSTRAINT teaching_journal_versions_check1
  CHECK (
    outcome = 'held'
    OR (
      length(btrim(not_held_reason)) BETWEEN 1 AND 500
      AND material = ''
      AND actual_starts_at IS NULL
      AND actual_ends_at IS NULL
    )
  );
