-- Kriteria aktif tidak memiliki angka bawaan; perubahan disimpan sebagai histori.
-- +goose Up
CREATE TABLE app.tp_achievement_guidance (
    id smallint PRIMARY KEY CHECK (id=1),
    guidance text NOT NULL CHECK (length(btrim(guidance)) BETWEEN 1 AND 4000),
    version bigint NOT NULL CHECK (version>0),
    actor_id bigint NOT NULL REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE app.tp_achievement_guidance_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    previous_guidance text,
    guidance text NOT NULL CHECK (length(btrim(guidance)) BETWEEN 1 AND 4000),
    version bigint NOT NULL UNIQUE CHECK (version>0),
    actor_id bigint NOT NULL REFERENCES app.users(id),
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    changed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
ALTER TABLE app.learning_objectives ADD CONSTRAINT tp_achievement_objective_context_unique UNIQUE(id,semester_id,class_id,subject_id);
CREATE TABLE app.tp_achievement_criteria (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    semester_id bigint NOT NULL REFERENCES app.semesters(id),
    class_id bigint NOT NULL REFERENCES app.classes(id),
    subject_id bigint NOT NULL REFERENCES app.subjects(id),
    objective_id bigint REFERENCES app.learning_objectives(id),
    threshold numeric(5,2) CHECK (threshold BETWEEN 0 AND 100),
    version bigint NOT NULL CHECK (version>0),
    actor_id bigint NOT NULL REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (objective_id IS NOT NULL OR threshold IS NOT NULL),
    FOREIGN KEY (objective_id,semester_id,class_id,subject_id) REFERENCES app.learning_objectives(id,semester_id,class_id,subject_id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX tp_criteria_default_unique ON app.tp_achievement_criteria(semester_id,class_id,subject_id) WHERE objective_id IS NULL;
CREATE UNIQUE INDEX tp_criteria_override_unique ON app.tp_achievement_criteria(objective_id) WHERE objective_id IS NOT NULL;
CREATE TABLE app.tp_achievement_criteria_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    criteria_id bigint NOT NULL REFERENCES app.tp_achievement_criteria(id),
    previous_threshold numeric(5,2) CHECK (previous_threshold BETWEEN 0 AND 100),
    threshold numeric(5,2) CHECK (threshold BETWEEN 0 AND 100),
    version bigint NOT NULL CHECK (version>0),
    actor_id bigint NOT NULL REFERENCES app.users(id),
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    changed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(criteria_id,version)
);
-- +goose StatementBegin
CREATE FUNCTION app.reject_tp_achievement_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Histori kriteria bersifat append-only' USING ERRCODE='55000'; END $$;
-- +goose StatementEnd
CREATE TRIGGER tp_criteria_history_append_only BEFORE UPDATE OR DELETE ON app.tp_achievement_criteria_history FOR EACH ROW EXECUTE FUNCTION app.reject_tp_achievement_history_mutation();
CREATE TRIGGER tp_guidance_history_append_only BEFORE UPDATE OR DELETE ON app.tp_achievement_guidance_history FOR EACH ROW EXECUTE FUNCTION app.reject_tp_achievement_history_mutation();
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Histori kriteria memerlukan strategi pemulihan eksplisit'; END $$;
