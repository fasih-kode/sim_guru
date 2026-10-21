-- +goose Up
CREATE UNIQUE INDEX classes_school_year_id_pair_idx ON app.classes(school_year_id,id);

CREATE TABLE app.student_placements (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 school_year_id bigint NOT NULL REFERENCES app.school_years(id),
 student_id bigint NOT NULL REFERENCES app.students(id),
 class_id bigint NOT NULL,
 actor_id bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(school_year_id,student_id),
 FOREIGN KEY(school_year_id,class_id) REFERENCES app.classes(school_year_id,id)
);
CREATE INDEX student_placements_class_idx ON app.student_placements(school_year_id,class_id,student_id);

CREATE TABLE app.student_placement_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 placement_id bigint NOT NULL REFERENCES app.student_placements(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 from_class_id bigint,
 to_class_id bigint NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(from_class_id) REFERENCES app.classes(id),
 FOREIGN KEY(to_class_id) REFERENCES app.classes(id)
);
CREATE INDEX student_placement_history_placement_idx ON app.student_placement_history(placement_id,id DESC);

-- +goose Down
DROP TABLE app.student_placement_history;
DROP TABLE app.student_placements;
DROP INDEX app.classes_school_year_id_pair_idx;
