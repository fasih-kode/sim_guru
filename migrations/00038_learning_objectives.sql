-- Definisi TP hanya disimpan dalam konteks kelas, mapel, dan semester.
-- Identitas tahun dipakai oleh FK majemuk agar kelas dan semester tidak tertukar.
-- +goose Up
ALTER TABLE app.semesters
    ADD CONSTRAINT semesters_id_school_year_unique UNIQUE (id, school_year_id);
ALTER TABLE app.classes
    ADD CONSTRAINT classes_id_school_year_unique UNIQUE (id, school_year_id);

CREATE TABLE app.learning_objectives (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL REFERENCES app.subjects(id) ON DELETE RESTRICT,
    definition text NOT NULL CHECK (length(btrim(definition)) > 0),
    created_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (semester_id, school_year_id)
        REFERENCES app.semesters(id, school_year_id) ON DELETE RESTRICT,
    FOREIGN KEY (class_id, school_year_id)
        REFERENCES app.classes(id, school_year_id) ON DELETE RESTRICT
);

CREATE INDEX learning_objectives_context_idx
    ON app.learning_objectives (semester_id, class_id, subject_id, id);

-- +goose Down
DROP TABLE app.learning_objectives;
ALTER TABLE app.classes DROP CONSTRAINT classes_id_school_year_unique;
ALTER TABLE app.semesters DROP CONSTRAINT semesters_id_school_year_unique;
