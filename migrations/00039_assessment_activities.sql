-- Kegiatan asesmen menautkan satu kegiatan dengan satu atau beberapa TP pada konteks yang sama.
-- Relasi TP dipisahkan agar nilai kelak dapat direkam per siswa dan per TP.
-- +goose Up
ALTER TABLE app.learning_objectives
    ADD CONSTRAINT learning_objectives_id_context_unique
    UNIQUE (id, school_year_id, semester_id, class_id, subject_id);

CREATE TABLE app.assessment_activities (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL REFERENCES app.subjects(id) ON DELETE RESTRICT,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    name varchar(200) NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    activity_type text NOT NULL CHECK (activity_type IN ('formatif', 'sumatif')),
    activity_date date NOT NULL,
    created_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (semester_id, school_year_id)
        REFERENCES app.semesters(id, school_year_id) ON DELETE RESTRICT,
    FOREIGN KEY (class_id, school_year_id)
        REFERENCES app.classes(id, school_year_id) ON DELETE RESTRICT,
    CONSTRAINT assessment_activities_id_context_unique
        UNIQUE (id, school_year_id, semester_id, class_id, subject_id)
);

CREATE INDEX assessment_activities_context_date_idx
    ON app.assessment_activities (semester_id, class_id, subject_id, activity_date, id);

CREATE TABLE app.assessment_activity_objectives (
    activity_id bigint NOT NULL,
    learning_objective_id bigint NOT NULL,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    PRIMARY KEY (activity_id, learning_objective_id),
    FOREIGN KEY (activity_id, school_year_id, semester_id, class_id, subject_id)
        REFERENCES app.assessment_activities(id, school_year_id, semester_id, class_id, subject_id) ON DELETE RESTRICT,
    FOREIGN KEY (learning_objective_id, school_year_id, semester_id, class_id, subject_id)
        REFERENCES app.learning_objectives(id, school_year_id, semester_id, class_id, subject_id) ON DELETE RESTRICT
);

CREATE INDEX assessment_activity_objectives_learning_objective_idx
    ON app.assessment_activity_objectives (learning_objective_id, activity_id);

-- +goose Down
DROP TABLE app.assessment_activity_objectives;
DROP TABLE app.assessment_activities;
ALTER TABLE app.learning_objectives DROP CONSTRAINT learning_objectives_id_context_unique;
