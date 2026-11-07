-- Nilai awal ujian adalah bukti milik guru dan kelas asal yang tidak dapat ditimpa.
-- +goose Up
CREATE TABLE app.exam_initial_scores (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL REFERENCES app.subjects(id) ON DELETE RESTRICT,
    student_id bigint NOT NULL REFERENCES app.students(id) ON DELETE RESTRICT,
    placement_id bigint NOT NULL,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    component text NOT NULL CHECK (component IN ('asts','asas')),
    exam_date date NOT NULL,
    score numeric(5,2) NOT NULL CHECK (score BETWEEN 0 AND 100),
    recorded_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (semester_id,school_year_id) REFERENCES app.semesters(id,school_year_id) ON DELETE RESTRICT,
    FOREIGN KEY (class_id,school_year_id) REFERENCES app.classes(id,school_year_id) ON DELETE RESTRICT,
    FOREIGN KEY (placement_id,student_id,school_year_id) REFERENCES app.student_placements(id,student_id,school_year_id) ON DELETE RESTRICT,
    UNIQUE (semester_id,class_id,subject_id,student_id,component)
);
CREATE INDEX exam_initial_scores_context_idx ON app.exam_initial_scores(semester_id,class_id,subject_id,student_id);
-- +goose StatementBegin
CREATE FUNCTION app.reject_exam_initial_score_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Bukti nilai awal ujian bersifat tetap' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER exam_initial_scores_immutable BEFORE UPDATE OR DELETE ON app.exam_initial_scores
    FOR EACH ROW EXECUTE FUNCTION app.reject_exam_initial_score_mutation();
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Bukti nilai ujian memerlukan strategi pemulihan historis eksplisit'; END $$;
