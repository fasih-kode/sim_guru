-- +goose Up
INSERT INTO app.roles(code,name) VALUES
 ('pegawai_tu','Pegawai TU'),
 ('pengelola_kehadiran_guru','TU Pengelola Kehadiran Guru');

CREATE TABLE app.teacher_attendance_correction_requests (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 teacher_id bigint NOT NULL REFERENCES app.teachers(id),
 teacher_name text NOT NULL CHECK(length(trim(teacher_name)) BETWEEN 1 AND 120),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 on_date date NOT NULL,
 attendance_action text NOT NULL CHECK(attendance_action IN('check_in','check_out')),
 request_kind text NOT NULL CHECK(request_kind IN('completion','correction')),
 request_text text NOT NULL CHECK(length(trim(request_text)) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX teacher_attendance_correction_requests_created_idx
 ON app.teacher_attendance_correction_requests(created_at DESC,id DESC);
CREATE INDEX teacher_attendance_correction_requests_teacher_date_idx
 ON app.teacher_attendance_correction_requests(teacher_id,on_date DESC,id DESC);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Kehadiran dan permintaan koreksinya memerlukan prosedur pemulihan eksplisit'; END $$;
