-- +goose Up
CREATE TABLE app.timetable_xml_imports (
 token_hash text PRIMARY KEY CHECK(length(token_hash)=64),
 owner_id bigint NOT NULL REFERENCES app.users(id),
 semester_id bigint NOT NULL REFERENCES app.semesters(id),
 file_name text NOT NULL,
 file_hash text NOT NULL CHECK(length(file_hash)=64),
 payload jsonb NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','complete','expired')),
 revision_id bigint REFERENCES app.weekly_timetable_revisions(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '30 minutes',
 completed_at timestamptz
);
CREATE INDEX timetable_xml_imports_owner ON app.timetable_xml_imports(owner_id,state);
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Timetable import provenance requires an explicit recovery strategy'; END $$;
