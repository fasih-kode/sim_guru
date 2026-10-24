-- +goose Up
ALTER TABLE app.student_school_exits DROP CONSTRAINT student_school_exits_student_id_key;
CREATE UNIQUE INDEX student_school_exits_student_date_idx ON app.student_school_exits(student_id,effective_date);
CREATE TABLE app.student_school_returns (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 exit_id bigint NOT NULL UNIQUE REFERENCES app.student_school_exits(id),
 placement_id bigint NOT NULL REFERENCES app.student_placements(id),
 target_class_id bigint NOT NULL REFERENCES app.classes(id),
 effective_date date NOT NULL,
 source_school text NOT NULL DEFAULT '' CHECK(length(source_school)<=200),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 created_placement boolean NOT NULL DEFAULT false,
 actor_id bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX student_school_returns_placement_date_idx ON app.student_school_returns(placement_id,effective_date);
CREATE VIEW app.outstanding_student_school_exits AS
SELECT e.*,r.effective_date AS return_date FROM app.student_school_exits e
LEFT JOIN app.student_school_returns r ON r.exit_id=e.id
WHERE r.id IS NULL OR r.effective_date > (now() AT TIME ZONE 'Asia/Jakarta')::date;

-- A NULL class means the student is outside school on that date. Original
-- placements stay intact; both returns and class moves contribute dated classes.
-- +goose StatementBegin
CREATE FUNCTION app.student_placement_class_at(placement bigint, as_of date) RETURNS bigint
LANGUAGE sql STABLE AS $$
 SELECT CASE WHEN last_school_event.kind='exit' OR EXISTS(
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
CREATE OR REPLACE VIEW app.current_student_placements AS
SELECT p.id,p.school_year_id,p.student_id,app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AS class_id,p.actor_id,p.created_at
FROM app.student_placements p
WHERE app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL;

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Readmission history requires an explicit recovery strategy'; END $$;
