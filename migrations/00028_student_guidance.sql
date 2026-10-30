-- +goose Up
CREATE TABLE app.student_guidance_records (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 student_id bigint NOT NULL REFERENCES app.students(id),
 student_name_at_event text NOT NULL CHECK(length(trim(student_name_at_event)) BETWEEN 1 AND 200),
 class_id_at_event bigint NOT NULL REFERENCES app.classes(id),
 class_name_at_event text NOT NULL CHECK(length(trim(class_name_at_event)) BETWEEN 1 AND 80),
 occurred_on date NOT NULL,
 incident text NOT NULL CHECK(length(trim(incident)) BETWEEN 1 AND 2000),
 action text NOT NULL CHECK(length(trim(action)) BETWEEN 1 AND 2000),
 status text NOT NULL CHECK(status IN ('perlu_tindak_lanjut','selesai')),
 recorded_by bigint NOT NULL REFERENCES app.users(id),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX student_guidance_records_student_idx ON app.student_guidance_records(student_id,occurred_on DESC,id DESC);
CREATE INDEX student_guidance_records_date_status_idx ON app.student_guidance_records(occurred_on DESC,status,id DESC);

CREATE TABLE app.student_guidance_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 record_id bigint NOT NULL REFERENCES app.student_guidance_records(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 change_type text NOT NULL CHECK(change_type IN ('recorded','content_changed','status_changed')),
 before_state jsonb CHECK(before_state IS NULL OR jsonb_typeof(before_state)='object'),
 after_state jsonb NOT NULL CHECK(jsonb_typeof(after_state)='object'),
 reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((change_type='recorded' AND before_state IS NULL) OR (change_type<>'recorded' AND before_state IS NOT NULL))
);
CREATE INDEX student_guidance_history_record_idx ON app.student_guidance_history(record_id,id DESC);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Catatan pembinaan dan histori memerlukan strategi pemulihan eksplisit'; END $$;
