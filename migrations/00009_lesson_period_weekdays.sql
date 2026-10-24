-- +goose Up
ALTER TABLE app.lesson_periods
 ADD COLUMN weekday smallint NOT NULL DEFAULT 1 CHECK(weekday BETWEEN 1 AND 7);
ALTER TABLE app.lesson_periods
 DROP CONSTRAINT lesson_periods_semester_id_period_no_key;
-- Before this change, period hours applied to every weekday. Duplicate those
-- definitions to preserve the existing timetable behavior without data loss.
INSERT INTO app.lesson_periods(semester_id,weekday,period_no,starts_at,ends_at,updated_by,updated_at)
SELECT p.semester_id,days.weekday,p.period_no,p.starts_at,p.ends_at,p.updated_by,p.updated_at
FROM app.lesson_periods p CROSS JOIN generate_series(2,7) AS days(weekday);
ALTER TABLE app.lesson_periods
 ADD CONSTRAINT lesson_periods_semester_weekday_period_key UNIQUE(semester_id,weekday,period_no);
DROP INDEX app.lesson_periods_semester_time_idx;
CREATE INDEX lesson_periods_semester_time_idx ON app.lesson_periods(semester_id,weekday,starts_at,ends_at);

ALTER TABLE app.weekly_timetable_periods
 ADD COLUMN weekday smallint NOT NULL DEFAULT 1 CHECK(weekday BETWEEN 1 AND 7);
ALTER TABLE app.weekly_timetable_periods DROP CONSTRAINT weekly_timetable_periods_pkey;
-- Existing published timetable revisions used one shared bell schedule.
-- Snapshot it for each weekday so all existing entries keep their old times.
INSERT INTO app.weekly_timetable_periods(revision_id,weekday,period_no,starts_at,ends_at)
SELECT p.revision_id,days.weekday,p.period_no,p.starts_at,p.ends_at
FROM app.weekly_timetable_periods p CROSS JOIN generate_series(2,7) AS days(weekday);
ALTER TABLE app.weekly_timetable_periods
 ADD CONSTRAINT weekly_timetable_periods_pkey PRIMARY KEY(revision_id,weekday,period_no);

-- +goose Down
DO $$
BEGIN
 IF EXISTS (
  SELECT 1 FROM app.lesson_periods p
  WHERE p.weekday<>1 AND EXISTS (
   SELECT 1 FROM app.lesson_period_history h WHERE h.period_id=p.id
  )
 ) OR EXISTS (
  SELECT 1 FROM app.lesson_periods
  GROUP BY semester_id,period_no
  HAVING count(DISTINCT (starts_at,ends_at))>1
 ) OR EXISTS (
  SELECT 1 FROM app.weekly_timetable_periods
  GROUP BY revision_id,period_no
  HAVING count(DISTINCT (starts_at,ends_at))>1
 ) THEN
  RAISE EXCEPTION 'cannot downgrade weekday-specific lesson periods without losing data';
 END IF;
END $$;
DELETE FROM app.lesson_periods WHERE weekday<>1;
DELETE FROM app.weekly_timetable_periods WHERE weekday<>1;
ALTER TABLE app.lesson_periods DROP CONSTRAINT lesson_periods_semester_weekday_period_key;
DROP INDEX app.lesson_periods_semester_time_idx;
ALTER TABLE app.lesson_periods DROP COLUMN weekday;
ALTER TABLE app.lesson_periods ADD CONSTRAINT lesson_periods_semester_id_period_no_key UNIQUE(semester_id,period_no);
CREATE INDEX lesson_periods_semester_time_idx ON app.lesson_periods(semester_id,starts_at,ends_at);
ALTER TABLE app.weekly_timetable_periods DROP CONSTRAINT weekly_timetable_periods_pkey;
ALTER TABLE app.weekly_timetable_periods DROP COLUMN weekday;
ALTER TABLE app.weekly_timetable_periods ADD CONSTRAINT weekly_timetable_periods_pkey PRIMARY KEY(revision_id,period_no);
