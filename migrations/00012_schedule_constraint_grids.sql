-- +goose Up
CREATE TABLE app.schedule_constraint_grids (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 teacher_id bigint REFERENCES app.teachers(id),
 subject_id bigint REFERENCES app.subjects(id),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
 version bigint NOT NULL DEFAULT 1,
 updated_by bigint NOT NULL REFERENCES app.users(id),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK((teacher_id IS NULL) <> (subject_id IS NULL))
);
CREATE UNIQUE INDEX constraint_grid_teacher ON app.schedule_constraint_grids(semester_id,teacher_id) WHERE teacher_id IS NOT NULL;
CREATE UNIQUE INDEX constraint_grid_subject ON app.schedule_constraint_grids(semester_id,subject_id) WHERE subject_id IS NOT NULL;
CREATE TABLE app.schedule_constraint_grid_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 grid_id bigint NOT NULL REFERENCES app.schedule_constraint_grids(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb,
 after_state jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
-- Legacy time constraints, revision snapshots and their history remain intact.
-- +goose Down
DO $$ BEGIN
 RAISE EXCEPTION 'Constraint grid history requires an explicit recovery strategy';
END $$;
