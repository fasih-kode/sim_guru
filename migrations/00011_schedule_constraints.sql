-- +goose Up
CREATE TABLE app.schedule_constraints (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 teacher_id bigint REFERENCES app.teachers(id),
 subject_id bigint REFERENCES app.subjects(id),
 weekdays smallint[] NOT NULL CHECK(cardinality(weekdays) BETWEEN 1 AND 7 AND weekdays <@ ARRAY[1,2,3,4,5,6,7]::smallint[]),
 severity text NOT NULL CHECK(severity IN ('hard','soft')),
 all_day boolean NOT NULL,
 starts_at time,
 ends_at time,
 version bigint NOT NULL DEFAULT 1,
 updated_by bigint NOT NULL REFERENCES app.users(id),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK((teacher_id IS NULL) <> (subject_id IS NULL)),
 CHECK((all_day AND starts_at IS NULL AND ends_at IS NULL) OR
       (NOT all_day AND starts_at IS NOT NULL AND ends_at IS NOT NULL AND starts_at<ends_at))
);
CREATE INDEX schedule_constraints_semester ON app.schedule_constraints(semester_id);
CREATE TABLE app.schedule_constraint_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 constraint_id bigint NOT NULL,
 before_state jsonb,
 after_state jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DO $$ BEGIN
 RAISE EXCEPTION 'Rollback requires an explicit recovery strategy; constraint history must be preserved';
END $$;
