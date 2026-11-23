-- S-06: satu baris append-only per versi keputusan untuk satu siswa dan TP.
-- +goose Up
CREATE TABLE app.direct_tp_applicability_history (
    id uuid PRIMARY KEY DEFAULT pg_catalog.gen_random_uuid(),
    school_year_id bigint NOT NULL,
    semester_id bigint NOT NULL,
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    objective_id bigint NOT NULL,
    student_id bigint NOT NULL,
    placement_id bigint NOT NULL,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id) ON DELETE RESTRICT,
    actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    effective_date date NOT NULL,
    applicable boolean NOT NULL,
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    evidence_verified boolean NOT NULL CHECK (evidence_verified),
    version bigint NOT NULL CHECK (version > 0),
    submission_key uuid NOT NULL,
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256)=32),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (objective_id,school_year_id,semester_id,class_id,subject_id)
      REFERENCES app.learning_objectives(id,school_year_id,semester_id,class_id,subject_id) ON DELETE RESTRICT,
    FOREIGN KEY (placement_id,student_id,school_year_id)
      REFERENCES app.student_placements(id,student_id,school_year_id) ON DELETE RESTRICT,
    UNIQUE (semester_id,class_id,subject_id,student_id,objective_id,version),
    UNIQUE (actor_id,submission_key)
);
CREATE TRIGGER direct_tp_applicability_append_only
    BEFORE UPDATE OR DELETE ON app.direct_tp_applicability_history
    FOR EACH ROW EXECUTE FUNCTION app.direct_tp_reject_mutation();
GRANT SELECT, INSERT ON TABLE app.direct_tp_applicability_history TO sim_guru_app;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Histori penetapan TP memerlukan strategi pemulihan eksplisit';
END $$;
-- +goose StatementEnd
