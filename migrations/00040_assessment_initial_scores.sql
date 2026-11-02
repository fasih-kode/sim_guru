-- Nilai awal disimpan terpisah untuk setiap siswa dan TP dalam satu kegiatan.
-- Penempatan dan tanggal pelaksanaan disalin sebagai rujukan historis.
-- +goose Up
ALTER TABLE app.student_placements
    ADD CONSTRAINT student_placements_id_student_year_unique
    UNIQUE (id, student_id, school_year_id);

CREATE TABLE app.assessment_activity_initial_scores (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    assessment_activity_id bigint NOT NULL,
    learning_objective_id bigint NOT NULL,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    student_id bigint NOT NULL REFERENCES app.students(id) ON DELETE RESTRICT,
    placement_id bigint NOT NULL,
    activity_date_snapshot date NOT NULL,
    score numeric(5,2) NOT NULL CHECK (score >= 0 AND score <= 100),
    recorded_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT assessment_initial_scores_activity_context_fk
        FOREIGN KEY (assessment_activity_id, school_year_id, semester_id, class_id, subject_id)
        REFERENCES app.assessment_activities(id, school_year_id, semester_id, class_id, subject_id)
        ON DELETE RESTRICT,
    CONSTRAINT assessment_initial_scores_activity_objective_fk
        FOREIGN KEY (assessment_activity_id, learning_objective_id)
        REFERENCES app.assessment_activity_objectives(activity_id, learning_objective_id)
        ON DELETE RESTRICT,
    CONSTRAINT assessment_initial_scores_placement_context_fk
        FOREIGN KEY (placement_id, student_id, school_year_id)
        REFERENCES app.student_placements(id, student_id, school_year_id)
        ON DELETE RESTRICT,
    CONSTRAINT assessment_initial_scores_occurrence_student_unique
        UNIQUE (assessment_activity_id, learning_objective_id, student_id)
);

CREATE INDEX assessment_initial_scores_student_activity_idx
    ON app.assessment_activity_initial_scores (student_id, assessment_activity_id);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Nilai asesmen awal memerlukan strategi pemulihan historis eksplisit'; END $$;
