-- Remedial merupakan hasil baru yang ditautkan ke nilai awal Sumatif.
-- +goose Up
CREATE TABLE app.assessment_activity_score_remedials (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    initial_score_id bigint NOT NULL REFERENCES app.assessment_activity_initial_scores(id) ON DELETE RESTRICT,
    score numeric(5,2) NOT NULL CHECK (score >= 0 AND score <= 100),
    remedial_date date NOT NULL,
    recorded_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX assessment_score_remedials_history_idx
    ON app.assessment_activity_score_remedials (initial_score_id, remedial_date, id);

-- Menjaga jenis bukti tetap Sumatif walaupun ada penulis database selain service.
-- +goose StatementBegin
CREATE FUNCTION app.guard_assessment_score_remedial_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM app.assessment_activity_initial_scores sc
        JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
        WHERE sc.id=NEW.initial_score_id AND a.activity_type='sumatif'
    ) THEN
        RAISE EXCEPTION 'Remedial hanya berlaku untuk nilai awal Sumatif' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app.reject_assessment_score_remedial_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Riwayat remedial nilai bersifat append-only' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd

CREATE TRIGGER assessment_score_remedials_sumatif_guard
    BEFORE INSERT ON app.assessment_activity_score_remedials
    FOR EACH ROW EXECUTE FUNCTION app.guard_assessment_score_remedial_insert();

CREATE TRIGGER assessment_score_remedials_append_only
    BEFORE UPDATE OR DELETE ON app.assessment_activity_score_remedials
    FOR EACH ROW EXECUTE FUNCTION app.reject_assessment_score_remedial_mutation();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Histori remedial nilai memerlukan strategi pemulihan eksplisit'; END $$;
