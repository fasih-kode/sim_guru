-- +goose Up
CREATE TABLE app.school_years (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 name text NOT NULL UNIQUE CHECK(length(name) BETWEEN 1 AND 40),
 active boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX school_years_one_active_idx ON app.school_years(active) WHERE active;

CREATE TABLE app.semesters (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 school_year_id bigint NOT NULL REFERENCES app.school_years(id),
 number smallint NOT NULL CHECK(number IN (1,2)),
 active boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(school_year_id,number)
);
CREATE UNIQUE INDEX semesters_one_active_idx ON app.semesters(active) WHERE active;

CREATE TABLE app.classes (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 school_year_id bigint NOT NULL REFERENCES app.school_years(id),
 grade smallint NOT NULL CHECK(grade BETWEEN 7 AND 9),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 40),
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX classes_year_grade_name_idx ON app.classes(school_year_id,grade,lower(name));
CREATE INDEX classes_year_idx ON app.classes(school_year_id,grade,name);

CREATE TABLE app.academic_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 entity_type text NOT NULL CHECK(entity_type IN ('school_year','semester','class')),
 entity_id bigint NOT NULL,
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb,
 after_state jsonb NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX academic_history_entity_idx ON app.academic_history(entity_type,entity_id,id DESC);

-- +goose Down
DROP TABLE app.academic_history;
DROP TABLE app.classes;
DROP TABLE app.semesters;
DROP TABLE app.school_years;
