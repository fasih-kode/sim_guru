-- +goose Up
CREATE TABLE app.teacher_attendance_reports (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 teacher_id bigint NOT NULL REFERENCES app.teachers(id),
 teacher_name text NOT NULL CHECK(length(trim(teacher_name)) BETWEEN 1 AND 200),
 kind text NOT NULL CHECK(kind IN ('izin','sakit','dinas_luar')),
 starts_on date NOT NULL,
 ends_on date NOT NULL CHECK(ends_on>=starts_on),
 description text NOT NULL CHECK(length(trim(description)) BETWEEN 1 AND 500),
 active boolean NOT NULL DEFAULT true,
 recorded_by bigint NOT NULL REFERENCES app.users(id),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 request_key_hash char(64) NOT NULL UNIQUE CHECK(request_key_hash ~ '^[0-9a-f]{64}$'),
 creation_fingerprint char(64) NOT NULL CHECK(creation_fingerprint ~ '^[0-9a-f]{64}$'),
 -- Range bawaan PostgreSQL menjaga overlap tanpa extension tambahan.
 CONSTRAINT teacher_attendance_reports_no_overlap EXCLUDE USING gist
 (int8range(teacher_id,teacher_id,'[]') WITH &&, daterange(starts_on,ends_on,'[]') WITH &&) WHERE(active)
);
CREATE INDEX teacher_attendance_reports_teacher_idx ON app.teacher_attendance_reports(teacher_id,id DESC);
CREATE TABLE app.teacher_attendance_report_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 report_id bigint NOT NULL REFERENCES app.teacher_attendance_reports(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 action text NOT NULL CHECK(action IN ('recorded','corrected','cancelled')),
 reason text,
 before_state jsonb,
 after_state jsonb NOT NULL CHECK(jsonb_typeof(after_state)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((action='recorded' AND before_state IS NULL AND reason IS NULL) OR
       (action IN ('corrected','cancelled') AND jsonb_typeof(before_state)='object' AND length(trim(coalesce(reason,''))) BETWEEN 1 AND 500))
);
CREATE INDEX teacher_attendance_report_history_report_idx ON app.teacher_attendance_report_history(report_id,id DESC);
ALTER TABLE app.teacher_attendance_correction_requests
 ADD COLUMN report_id bigint REFERENCES app.teacher_attendance_reports(id),
 ALTER COLUMN attendance_action DROP NOT NULL,
 ADD CONSTRAINT teacher_attendance_request_target CHECK(
  (report_id IS NULL AND attendance_action IS NOT NULL) OR
  (report_id IS NOT NULL AND attendance_action IS NULL AND request_kind='correction'));
CREATE INDEX teacher_attendance_correction_requests_report_idx ON app.teacher_attendance_correction_requests(report_id) WHERE report_id IS NOT NULL;

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Laporan dan histori kehadiran memerlukan prosedur pemulihan eksplisit'; END $$;
