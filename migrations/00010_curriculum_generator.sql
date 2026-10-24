-- +goose Up
CREATE TABLE app.curriculum_rules (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 grade smallint NOT NULL CHECK(grade BETWEEN 7 AND 9),
 class_id bigint REFERENCES app.classes(id),
 subject_id bigint NOT NULL REFERENCES app.subjects(id),
 weekly_jp smallint NOT NULL CHECK(weekly_jp BETWEEN 0 AND 210),
 meetings jsonb NOT NULL CHECK(jsonb_typeof(meetings)='array'),
 different_days boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1,
 updated_by bigint NOT NULL REFERENCES app.users(id),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX curriculum_grade_rule ON app.curriculum_rules(semester_id,grade,subject_id) WHERE class_id IS NULL;
CREATE UNIQUE INDEX curriculum_class_rule ON app.curriculum_rules(semester_id,class_id,subject_id) WHERE class_id IS NOT NULL;
CREATE TABLE app.school_time_blocks (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 weekday smallint NOT NULL CHECK(weekday BETWEEN 1 AND 7),
 kind text NOT NULL CHECK(kind IN ('break','activity')),
 label text NOT NULL CHECK(length(label) BETWEEN 1 AND 100),
 period_no smallint CHECK(period_no BETWEEN 1 AND 30),
 starts_at time NOT NULL,
 ends_at time NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 updated_by bigint NOT NULL REFERENCES app.users(id),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(starts_at<ends_at),
 CHECK((kind='break' AND period_no IS NULL) OR (kind='activity' AND period_no IS NOT NULL))
);
CREATE TABLE app.schedule_configuration_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 entity text NOT NULL CHECK(entity IN ('rule','block')),
 entity_id bigint NOT NULL,
 before_state jsonb,
 after_state jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE app.weekly_timetable_revisions ADD COLUMN plan_snapshot jsonb;
-- Existing published revisions intentionally retain their original snapshots/data.

-- +goose Down
DO $$ BEGIN
 RAISE EXCEPTION 'Rollback requires an explicit data recovery strategy; curriculum history must be preserved';
END $$;
