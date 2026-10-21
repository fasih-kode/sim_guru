-- +goose Up
CREATE TABLE app.students (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
 birth_date date NOT NULL,
 gender text NOT NULL CHECK(gender IN ('L','P')),
 nis text UNIQUE CHECK(nis IS NULL OR length(nis) BETWEEN 1 AND 64),
 nisn text UNIQUE CHECK(nisn IS NULL OR length(nisn) BETWEEN 1 AND 64),
 phone text NOT NULL DEFAULT '' CHECK(length(phone)<=40),
 active boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE app.teachers (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
 nip text NOT NULL DEFAULT '' CHECK(length(nip)<=64),
 nuptk text NOT NULL DEFAULT '' CHECK(length(nuptk)<=64),
 email text NOT NULL DEFAULT '' CHECK(length(email)<=254),
 phone text NOT NULL DEFAULT '' CHECK(length(phone)<=40),
 user_id bigint UNIQUE REFERENCES app.users(id),
 active boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE app.subjects (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 code text NOT NULL UNIQUE CHECK(length(code) BETWEEN 1 AND 64),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
 active boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
-- A code remains reserved to its subject even after rename/deactivation.
CREATE TABLE app.subject_codes (code text PRIMARY KEY, subject_id bigint NOT NULL REFERENCES app.subjects(id));
CREATE TABLE app.student_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 entity_id bigint NOT NULL REFERENCES app.students(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb, after_state jsonb NOT NULL, reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE app.teacher_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 entity_id bigint NOT NULL REFERENCES app.teachers(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb, after_state jsonb NOT NULL, reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE app.subject_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 entity_id bigint NOT NULL REFERENCES app.subjects(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb, after_state jsonb NOT NULL, reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX student_history_entity_idx ON app.student_history(entity_id,id DESC);
CREATE INDEX teacher_history_entity_idx ON app.teacher_history(entity_id,id DESC);
CREATE INDEX subject_history_entity_idx ON app.subject_history(entity_id,id DESC);

-- +goose Down
DROP TABLE app.subject_history;
DROP TABLE app.teacher_history;
DROP TABLE app.student_history;
DROP TABLE app.subject_codes;
DROP TABLE app.subjects;
DROP TABLE app.teachers;
DROP TABLE app.students;
