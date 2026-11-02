-- +goose Up
CREATE TABLE app.student_daily_attendance (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 student_id bigint NOT NULL REFERENCES app.students(id),
 placement_id bigint NOT NULL REFERENCES app.student_placements(id),
 class_id bigint NOT NULL REFERENCES app.classes(id),
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 on_date date NOT NULL,
 student_name text NOT NULL, nis text NOT NULL, class_name text NOT NULL,
 status text NOT NULL CHECK(status IN('hadir','sakit','izin','alpa')),
 arrived_at timestamptz,
 source text NOT NULL CHECK(source IN('qr','manual','correction')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(student_id,on_date),
 CHECK(status='hadir' OR arrived_at IS NULL),
 CHECK(arrived_at IS NULL OR (arrived_at AT TIME ZONE 'Asia/Jakarta')::date=on_date)
);
CREATE INDEX student_daily_class_date_idx ON app.student_daily_attendance(class_id,on_date);
CREATE TABLE app.student_daily_attendance_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 attendance_id bigint NOT NULL REFERENCES app.student_daily_attendance(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb, after_state jsonb NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX student_daily_history_idx ON app.student_daily_attendance_history(attendance_id,id DESC);
CREATE TABLE app.student_qr_cards (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 student_id bigint NOT NULL REFERENCES app.students(id),
 token text NOT NULL UNIQUE CHECK(length(token)=70),
 issued_by bigint NOT NULL REFERENCES app.users(id),
 issued_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 revoked_at timestamptz
);
CREATE UNIQUE INDEX student_qr_active_idx ON app.student_qr_cards(student_id) WHERE revoked_at IS NULL;
CREATE TABLE app.student_qr_card_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 student_id bigint NOT NULL REFERENCES app.students(id),
 card_id bigint NOT NULL REFERENCES app.student_qr_cards(id),
 previous_card_id bigint REFERENCES app.student_qr_cards(id),
 action text NOT NULL CHECK(action IN('issue','replace','reprint')),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX student_qr_history_idx ON app.student_qr_card_history(student_id,id DESC);
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Attendance and QR history requires an explicit recovery strategy'; END $$;
