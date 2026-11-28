-- Final normal disimpan sebagai bukti resmi berversi, tidak mengganti sumber nilai.
-- +goose Up
CREATE TABLE app.semester_final_versions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_year_id bigint NOT NULL REFERENCES app.school_years(id) ON DELETE RESTRICT,
    semester_id bigint NOT NULL REFERENCES app.semesters(id) ON DELETE RESTRICT,
    class_id bigint NOT NULL REFERENCES app.classes(id) ON DELETE RESTRICT,
    subject_id bigint NOT NULL REFERENCES app.subjects(id) ON DELETE RESTRICT,
    student_id bigint NOT NULL REFERENCES app.students(id) ON DELETE RESTRICT,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    recorded_by bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version > 0),
    recommendation_score smallint NOT NULL CHECK (recommendation_score BETWEEN 0 AND 100),
    final_score smallint NOT NULL CHECK (final_score BETWEEN 0 AND 100),
    reason text NOT NULL DEFAULT '' CHECK (length(btrim(reason)) <= 500),
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    snapshot_sha256 bytea NOT NULL CHECK (octet_length(snapshot_sha256) = 32),
    submission_key uuid NOT NULL,
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256) = 32),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (final_score = recommendation_score OR length(btrim(reason)) BETWEEN 1 AND 500),
    CONSTRAINT semester_final_versions_unique UNIQUE (semester_id, class_id, subject_id, student_id, version),
    CONSTRAINT semester_final_versions_submission_unique UNIQUE (recorded_by, submission_key)
);

CREATE INDEX semester_final_versions_context_student_idx
    ON app.semester_final_versions(semester_id, class_id, subject_id, student_id, version DESC);

-- Histori Final menjadi bukti audit dan tidak boleh ditimpa atau dihapus.
-- +goose StatementBegin
CREATE FUNCTION app.reject_semester_final_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Histori Nilai Final bersifat append-only' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER semester_final_versions_append_only
    BEFORE UPDATE OR DELETE ON app.semester_final_versions
    FOR EACH ROW EXECUTE FUNCTION app.reject_semester_final_mutation();

-- Struktur rutin dikunci di database juga, sehingga dua request yang berjalan
-- bersamaan dengan Final pertama tetap terserialisasi oleh kunci mutasi aplikasi.
-- +goose StatementBegin
CREATE FUNCTION app.reject_routine_structure_after_final() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target_semester bigint;
    target_class bigint;
    target_subject bigint;
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    IF TG_TABLE_NAME = 'assessment_activities' THEN
        IF NEW.activity_type <> 'sumatif' THEN
            RETURN NEW;
        END IF;
        target_semester := NEW.semester_id;
        target_class := NEW.class_id;
        target_subject := NEW.subject_id;
    ELSE
        target_semester := NEW.semester_id;
        target_class := NEW.class_id;
        target_subject := NEW.subject_id;
    END IF;

    IF EXISTS (
        SELECT 1 FROM app.semester_final_versions f
        WHERE f.semester_id=target_semester AND f.class_id=target_class AND f.subject_id=target_subject
    ) THEN
        IF TG_TABLE_NAME = 'assessment_activities' THEN
            RAISE EXCEPTION 'Penambahan kegiatan Sumatif rutin dikunci setelah Final pertama' USING ERRCODE='55000';
        END IF;
        RAISE EXCEPTION 'Penambahan TP rutin dikunci setelah Final pertama' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER semester_final_lock_new_objectives
    BEFORE INSERT ON app.learning_objectives
    FOR EACH ROW EXECUTE FUNCTION app.reject_routine_structure_after_final();
CREATE TRIGGER semester_final_lock_new_sumatif
    BEFORE INSERT ON app.assessment_activities
    FOR EACH ROW EXECUTE FUNCTION app.reject_routine_structure_after_final();

-- +goose Down
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM app.semester_final_versions) THEN
        RAISE EXCEPTION 'Histori Final berisi data; pemulihan eksplisit diperlukan sebelum migration Down';
    END IF;
END $$;
DROP TRIGGER semester_final_lock_new_sumatif ON app.assessment_activities;
DROP TRIGGER semester_final_lock_new_objectives ON app.learning_objectives;
DROP FUNCTION app.reject_routine_structure_after_final();
DROP TRIGGER semester_final_versions_append_only ON app.semester_final_versions;
DROP FUNCTION app.reject_semester_final_mutation();
DROP TABLE app.semester_final_versions;
