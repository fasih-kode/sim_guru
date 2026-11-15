-- Batch mengikat satu permintaan kepada hasil Final individual yang immutable.
-- +goose Up
CREATE TABLE app.semester_final_batches (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    semester_id bigint NOT NULL REFERENCES app.semesters(id) ON DELETE RESTRICT,
    class_id bigint NOT NULL REFERENCES app.classes(id) ON DELETE RESTRICT,
    subject_id bigint NOT NULL REFERENCES app.subjects(id) ON DELETE RESTRICT,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    recorded_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    submission_key uuid NOT NULL,
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256)=32),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (recorded_by, submission_key)
);
CREATE TABLE app.semester_final_batch_items (
    batch_id bigint NOT NULL REFERENCES app.semester_final_batches(id) ON DELETE RESTRICT,
    student_id bigint NOT NULL REFERENCES app.students(id) ON DELETE RESTRICT,
    final_version_id bigint NOT NULL UNIQUE REFERENCES app.semester_final_versions(id) ON DELETE RESTRICT,
    PRIMARY KEY (batch_id, student_id)
);
CREATE TRIGGER semester_final_batches_append_only BEFORE UPDATE OR DELETE ON app.semester_final_batches
    FOR EACH ROW EXECUTE FUNCTION app.reject_semester_final_mutation();
CREATE TRIGGER semester_final_batch_items_append_only BEFORE UPDATE OR DELETE ON app.semester_final_batch_items
    FOR EACH ROW EXECUTE FUNCTION app.reject_semester_final_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM app.semester_final_batches) THEN
        RAISE EXCEPTION 'Histori batch Final berisi data; pemulihan eksplisit diperlukan sebelum migration Down';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE app.semester_final_batch_items;
DROP TABLE app.semester_final_batches;
