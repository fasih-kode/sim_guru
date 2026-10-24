-- +goose Up
CREATE TABLE app.student_year_transitions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 source_year_id bigint NOT NULL REFERENCES app.school_years(id),
 target_year_id bigint NOT NULL REFERENCES app.school_years(id),
 student_id bigint NOT NULL REFERENCES app.students(id),
 source_placement_id bigint NOT NULL REFERENCES app.student_placements(id),
 outcome text NOT NULL CHECK(outcome IN ('promote','repeat','graduate','leave')),
 target_class_id bigint,
 target_placement_id bigint REFERENCES app.student_placements(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(source_year_id<>target_year_id),
 CHECK((outcome IN ('promote','repeat') AND target_class_id IS NOT NULL AND target_placement_id IS NOT NULL) OR (outcome IN ('graduate','leave') AND target_class_id IS NULL AND target_placement_id IS NULL)),
 FOREIGN KEY(target_year_id,target_class_id) REFERENCES app.classes(school_year_id,id),
 UNIQUE(source_year_id,student_id)
);
CREATE INDEX student_year_transitions_target_year_idx ON app.student_year_transitions(target_year_id,created_at DESC);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Student transition history requires an explicit recovery strategy'; END $$;
