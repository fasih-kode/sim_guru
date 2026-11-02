-- +goose Up
CREATE TABLE app.student_arrival_rules (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 class_id bigint REFERENCES app.classes(id),
 weekday smallint NOT NULL CHECK(weekday BETWEEN 0 AND 7),
 on_date date,
 effective_from date NOT NULL,
 deadline time NOT NULL,
 tolerance_minutes integer NOT NULL CHECK(tolerance_minutes BETWEEN 0 AND 1439),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((on_date IS NULL AND weekday BETWEEN 1 AND 7) OR (on_date IS NOT NULL AND weekday=0 AND effective_from=on_date)),
 CHECK(extract(second FROM deadline)=0),
 CHECK(extract(hour FROM deadline)*60+extract(minute FROM deadline)+tolerance_minutes<1440)
);
CREATE INDEX student_arrival_rules_lookup_idx ON app.student_arrival_rules(semester_id,effective_from DESC,id DESC);
ALTER TABLE app.student_daily_attendance ADD COLUMN arrival_rule_id bigint REFERENCES app.student_arrival_rules(id);
ALTER TABLE app.student_daily_attendance ADD COLUMN arrival_policy jsonb;
ALTER TABLE app.student_daily_attendance ADD COLUMN late_minutes integer CHECK(late_minutes>=0);
-- Existing unknown times and previously unassessed evidence stay unchanged.
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Arrival rule snapshots require an explicit recovery strategy'; END $$;
