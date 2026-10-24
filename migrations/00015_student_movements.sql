-- +goose Up
CREATE TABLE app.student_class_moves (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 placement_id bigint NOT NULL REFERENCES app.student_placements(id),
 from_class_id bigint NOT NULL REFERENCES app.classes(id),
 to_class_id bigint NOT NULL REFERENCES app.classes(id),
 effective_date date NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(from_class_id<>to_class_id),
 UNIQUE(placement_id,effective_date)
);
CREATE TABLE app.student_school_exits (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 student_id bigint NOT NULL UNIQUE REFERENCES app.students(id),
 from_class_id bigint REFERENCES app.classes(id),
 effective_date date NOT NULL,
 destination_school text NOT NULL DEFAULT '' CHECK(length(destination_school)<=200),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE VIEW app.current_student_placements AS
SELECT p.id,p.school_year_id,p.student_id,coalesce(m.to_class_id,p.class_id) AS class_id,p.actor_id,p.created_at
FROM app.student_placements p
LEFT JOIN LATERAL (SELECT to_class_id FROM app.student_class_moves WHERE placement_id=p.id AND effective_date <= (now() AT TIME ZONE 'Asia/Jakarta')::date ORDER BY effective_date DESC,id DESC LIMIT 1) m ON true
WHERE NOT EXISTS(SELECT 1 FROM app.student_school_exits e WHERE e.student_id=p.student_id AND e.effective_date <= (now() AT TIME ZONE 'Asia/Jakarta')::date);
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Student movement history requires an explicit recovery strategy'; END $$;
