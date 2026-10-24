-- +goose Up
CREATE TABLE app.teaching_assignments (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 class_id bigint NOT NULL REFERENCES app.classes(id),
 subject_id bigint NOT NULL REFERENCES app.subjects(id),
 teacher_id bigint NOT NULL REFERENCES app.teachers(id),
 effective_from date NOT NULL,
 effective_until date,
 assigned_by bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(effective_until IS NULL OR effective_until > effective_from)
);
CREATE INDEX teaching_assignments_scope_idx ON app.teaching_assignments(semester_id,class_id,subject_id,effective_from DESC);
CREATE UNIQUE INDEX teaching_assignments_open_idx ON app.teaching_assignments(semester_id,class_id,subject_id) WHERE effective_until IS NULL;
CREATE INDEX teaching_assignments_teacher_idx ON app.teaching_assignments(teacher_id,semester_id,effective_from);

CREATE TABLE app.teaching_assignment_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 assignment_id bigint NOT NULL REFERENCES app.teaching_assignments(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb,
 after_state jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX teaching_assignment_history_idx ON app.teaching_assignment_history(assignment_id,id DESC);

CREATE TABLE app.lesson_periods (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 period_no smallint NOT NULL CHECK(period_no BETWEEN 1 AND 30),
 starts_at time NOT NULL,
 ends_at time NOT NULL,
 updated_by bigint NOT NULL REFERENCES app.users(id),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(starts_at < ends_at),
 UNIQUE(semester_id,period_no)
);
CREATE INDEX lesson_periods_semester_time_idx ON app.lesson_periods(semester_id,starts_at,ends_at);

CREATE TABLE app.lesson_period_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 period_id bigint NOT NULL REFERENCES app.lesson_periods(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb,
 after_state jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX lesson_period_history_idx ON app.lesson_period_history(period_id,id DESC);

CREATE TABLE app.weekly_timetable_revisions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 revision_no integer NOT NULL CHECK(revision_no > 0),
 status text NOT NULL CHECK(status IN ('draft','published')),
 effective_from date NOT NULL,
 created_by bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 published_by bigint REFERENCES app.users(id),
 published_at timestamptz,
 UNIQUE(semester_id,revision_no),
 UNIQUE(semester_id,effective_from),
 UNIQUE(id,semester_id),
 CHECK((status='draft' AND published_by IS NULL AND published_at IS NULL) OR (status='published' AND published_by IS NOT NULL AND published_at IS NOT NULL))
);
CREATE UNIQUE INDEX weekly_timetable_one_draft_idx ON app.weekly_timetable_revisions(semester_id) WHERE status='draft';
CREATE INDEX weekly_timetable_published_idx ON app.weekly_timetable_revisions(semester_id,effective_from DESC) WHERE status='published';

CREATE TABLE app.weekly_timetable_periods (
 revision_id bigint NOT NULL REFERENCES app.weekly_timetable_revisions(id),
 period_no smallint NOT NULL CHECK(period_no BETWEEN 1 AND 30),
 starts_at time NOT NULL,
 ends_at time NOT NULL,
 PRIMARY KEY(revision_id,period_no),
 CHECK(starts_at < ends_at)
);

CREATE TABLE app.weekly_timetable_entries (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 revision_id bigint NOT NULL REFERENCES app.weekly_timetable_revisions(id),
 weekday smallint NOT NULL CHECK(weekday BETWEEN 1 AND 7),
 class_id bigint NOT NULL REFERENCES app.classes(id),
 subject_id bigint NOT NULL REFERENCES app.subjects(id),
 teacher_id bigint NOT NULL REFERENCES app.teachers(id),
 assignment_id bigint NOT NULL REFERENCES app.teaching_assignments(id),
 period_start smallint NOT NULL,
 period_end smallint NOT NULL,
 CHECK(period_start > 0 AND period_end >= period_start)
);
CREATE INDEX weekly_timetable_entries_revision_idx ON app.weekly_timetable_entries(revision_id,weekday,period_start,period_end);
CREATE INDEX weekly_timetable_entries_teacher_idx ON app.weekly_timetable_entries(teacher_id,revision_id,weekday);
CREATE INDEX weekly_timetable_entries_class_idx ON app.weekly_timetable_entries(class_id,revision_id,weekday);

CREATE TABLE app.weekly_timetable_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 revision_id bigint NOT NULL REFERENCES app.weekly_timetable_revisions(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 action text NOT NULL CHECK(action IN ('draft_created','entry_added','entry_removed','published')),
 before_state jsonb,
 after_state jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX weekly_timetable_history_idx ON app.weekly_timetable_history(revision_id,id DESC);

-- +goose Down
DROP TABLE app.weekly_timetable_history;
DROP TABLE app.weekly_timetable_entries;
DROP TABLE app.weekly_timetable_periods;
DROP TABLE app.weekly_timetable_revisions;
DROP TABLE app.lesson_period_history;
DROP TABLE app.lesson_periods;
DROP TABLE app.teaching_assignment_history;
DROP TABLE app.teaching_assignments;
