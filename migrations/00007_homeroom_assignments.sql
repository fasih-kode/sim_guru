-- +goose Up
CREATE TABLE app.homeroom_assignments (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 class_id bigint NOT NULL UNIQUE REFERENCES app.classes(id),
 teacher_id bigint NOT NULL REFERENCES app.teachers(id),
 assigned_by bigint NOT NULL REFERENCES app.users(id),
 assigned_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX homeroom_assignments_teacher_idx ON app.homeroom_assignments(teacher_id,class_id);

CREATE TABLE app.homeroom_assignment_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 assignment_id bigint NOT NULL REFERENCES app.homeroom_assignments(id),
 class_id bigint NOT NULL REFERENCES app.classes(id),
 before_teacher_id bigint REFERENCES app.teachers(id),
 after_teacher_id bigint NOT NULL REFERENCES app.teachers(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX homeroom_assignment_history_class_idx ON app.homeroom_assignment_history(class_id,id DESC);

-- +goose Down
DROP TABLE app.homeroom_assignment_history;
DROP TABLE app.homeroom_assignments;
