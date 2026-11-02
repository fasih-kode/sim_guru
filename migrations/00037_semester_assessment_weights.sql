-- Konfigurasi bobot NH, ASTS, dan ASAS berlaku seragam per semester.
-- Tidak ada nilai awal agar kebijakan tidak ditetapkan secara diam-diam.
-- +goose Up
CREATE TABLE app.semester_assessment_weights (
    semester_id bigint PRIMARY KEY REFERENCES app.semesters(id) ON DELETE RESTRICT,
    nh smallint NOT NULL CHECK (nh BETWEEN 0 AND 100),
    asts smallint NOT NULL CHECK (asts BETWEEN 0 AND 100),
    asas smallint NOT NULL CHECK (asas BETWEEN 0 AND 100),
    CHECK (nh + asts + asas = 100),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE app.semester_assessment_weight_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    semester_id bigint NOT NULL REFERENCES app.semesters(id) ON DELETE RESTRICT,
    previous_nh smallint,
    previous_asts smallint,
    previous_asas smallint,
    nh smallint NOT NULL CHECK (nh BETWEEN 0 AND 100),
    asts smallint NOT NULL CHECK (asts BETWEEN 0 AND 100),
    asas smallint NOT NULL CHECK (asas BETWEEN 0 AND 100),
    CHECK (nh + asts + asas = 100),
    CHECK (
        (previous_nh IS NULL AND previous_asts IS NULL AND previous_asas IS NULL)
        OR
        (previous_nh IS NOT NULL AND previous_asts IS NOT NULL AND previous_asas IS NOT NULL
         AND previous_nh BETWEEN 0 AND 100 AND previous_asts BETWEEN 0 AND 100
         AND previous_asas BETWEEN 0 AND 100
         AND previous_nh + previous_asts + previous_asas = 100)
    ),
    actor_id bigint NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    changed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX semester_assessment_weight_history_idx
    ON app.semester_assessment_weight_history(semester_id, id DESC);

-- +goose Down
DROP TABLE app.semester_assessment_weight_history;
DROP TABLE app.semester_assessment_weights;
