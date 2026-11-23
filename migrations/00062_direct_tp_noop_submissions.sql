-- Replay permintaan tanpa perubahan tetap memiliki identitas batch audit.
-- +goose Up
ALTER TABLE app.direct_tp_submissions DROP CONSTRAINT direct_tp_submissions_expected_count_check;
ALTER TABLE app.direct_tp_submissions ADD CONSTRAINT direct_tp_submissions_expected_count_check CHECK(expected_count BETWEEN 0 AND 5000);
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Audit batch nilai TP memerlukan strategi pemulihan eksplisit'; END $$;
