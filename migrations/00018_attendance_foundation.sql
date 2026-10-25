-- +goose Up
CREATE TABLE app.semester_calendars (
 semester_id bigint PRIMARY KEY REFERENCES app.semesters(id),
 starts_on date NOT NULL, ends_on date NOT NULL,
 school_days jsonb NOT NULL CHECK(jsonb_typeof(school_days)='array' AND jsonb_array_length(school_days) BETWEEN 1 AND 7),
 version bigint NOT NULL DEFAULT 1,
 CHECK(starts_on<=ends_on)
);
CREATE TABLE app.school_date_exceptions (
 semester_id bigint NOT NULL REFERENCES app.semester_calendars(semester_id),
 on_date date NOT NULL, school_day boolean NOT NULL,
 label text NOT NULL CHECK(length(label) BETWEEN 1 AND 200),
 PRIMARY KEY(semester_id,on_date)
);
ALTER TABLE app.student_placements ADD COLUMN starts_on date;
ALTER TABLE app.student_placements ADD COLUMN date_version bigint NOT NULL DEFAULT 1;
CREATE TABLE app.attendance_foundation_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 entity_type text NOT NULL CHECK(entity_type IN ('calendar','placement_date')),
 entity_id bigint NOT NULL, actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb, after_state jsonb NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attendance_foundation_history_entity_idx ON app.attendance_foundation_history(entity_type,entity_id,id DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.student_placement_class_at(placement bigint, as_of date) RETURNS bigint
LANGUAGE sql STABLE AS $$
 SELECT CASE WHEN (p.starts_on IS NOT NULL AND as_of<p.starts_on) OR last_school_event.kind='exit' OR EXISTS(
  SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.created_placement AND r.effective_date>as_of
 ) THEN NULL::bigint ELSE coalesce((
  SELECT ev.class_id FROM (
   SELECT m.to_class_id AS class_id,m.effective_date,m.id,1 AS priority FROM app.student_class_moves m WHERE m.placement_id=p.id AND m.effective_date<=as_of
   UNION ALL
   SELECT r.target_class_id,r.effective_date,r.id,2 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.effective_date<=as_of
  ) ev ORDER BY ev.effective_date DESC,ev.priority DESC,ev.id DESC LIMIT 1
 ),p.class_id) END
 FROM app.student_placements p
 LEFT JOIN LATERAL (
  SELECT ev.kind FROM (
   SELECT 'exit' AS kind,e.effective_date,e.id,1 AS priority FROM app.student_school_exits e WHERE e.student_id=p.student_id AND e.effective_date<=as_of
   UNION ALL
   SELECT 'return',r.effective_date,r.id,2 FROM app.student_school_returns r JOIN app.student_school_exits e ON e.id=r.exit_id WHERE e.student_id=p.student_id AND r.effective_date<=as_of
  ) ev ORDER BY ev.effective_date DESC,ev.priority DESC,ev.id DESC LIMIT 1
 ) last_school_event ON true
 WHERE p.id=placement
$$;
-- +goose StatementEnd

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Calendar and placement date history requires an explicit recovery strategy'; END $$;
