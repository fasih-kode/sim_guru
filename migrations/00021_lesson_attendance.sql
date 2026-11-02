-- +goose Up
ALTER TABLE app.weekly_timetable_entries
 ADD CONSTRAINT weekly_timetable_entries_id_revision_unique UNIQUE(id,revision_id);

CREATE TABLE app.student_lesson_attendance (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 timetable_entry_id bigint NOT NULL,
 revision_id bigint NOT NULL,
 assignment_id bigint NOT NULL REFERENCES app.teaching_assignments(id),
 semester_id bigint NOT NULL,
 meeting_date date NOT NULL,
 student_id bigint NOT NULL REFERENCES app.students(id),
 placement_id bigint NOT NULL REFERENCES app.student_placements(id),
 class_id bigint NOT NULL REFERENCES app.classes(id),
 subject_id bigint NOT NULL REFERENCES app.subjects(id),
 teacher_id bigint NOT NULL REFERENCES app.teachers(id),
 student_name text NOT NULL,
 nis text NOT NULL,
 class_name text NOT NULL,
 subject_name text NOT NULL,
 teacher_name text NOT NULL,
 period_start smallint NOT NULL CHECK(period_start BETWEEN 1 AND 30),
 period_end smallint NOT NULL CHECK(period_end BETWEEN period_start AND 30),
 starts_at time NOT NULL,
 ends_at time NOT NULL,
 time_segments text NOT NULL,
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
 status text NOT NULL CHECK(status IN('hadir','sakit','izin','alpa')),
 recorded_by bigint NOT NULL REFERENCES app.users(id),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 UNIQUE(timetable_entry_id,meeting_date,student_id),
 FOREIGN KEY(timetable_entry_id,revision_id) REFERENCES app.weekly_timetable_entries(id,revision_id),
 FOREIGN KEY(revision_id,semester_id) REFERENCES app.weekly_timetable_revisions(id,semester_id),
 CHECK(starts_at<ends_at)
);
CREATE INDEX student_lesson_attendance_class_date_idx ON app.student_lesson_attendance(class_id,meeting_date);
CREATE INDEX student_lesson_attendance_teacher_date_idx ON app.student_lesson_attendance(teacher_id,meeting_date);
CREATE INDEX student_lesson_attendance_student_date_idx ON app.student_lesson_attendance(student_id,meeting_date DESC);

CREATE TABLE app.student_lesson_attendance_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 attendance_id bigint NOT NULL REFERENCES app.student_lesson_attendance(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 action text NOT NULL CHECK(action IN('recorded','corrected')),
 before_state jsonb,
 after_state jsonb NOT NULL CHECK(jsonb_typeof(after_state)='object'),
 reason text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((action='recorded' AND before_state IS NULL AND reason IS NULL) OR
       (action='corrected' AND before_state IS NOT NULL AND reason IS NOT NULL AND length(trim(reason)) BETWEEN 1 AND 500))
);
CREATE INDEX student_lesson_attendance_history_idx ON app.student_lesson_attendance_history(attendance_id,id DESC);

-- Kehadiran dan histori adalah bukti akademik; rollback harus memakai prosedur pemulihan eksplisit.
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Lesson attendance evidence requires an explicit recovery strategy';
END $$;
-- +goose StatementEnd
