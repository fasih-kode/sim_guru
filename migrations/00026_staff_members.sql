-- +goose Up
CREATE TABLE app.staff_members (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 employee_number text NOT NULL DEFAULT '' CHECK(length(employee_number)<=64),
 email text NOT NULL DEFAULT '' CHECK(length(email)<=254),
 phone text NOT NULL DEFAULT '' CHECK(length(phone)<=40),
 user_id bigint UNIQUE REFERENCES app.users(id),
 active boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX staff_members_employee_number_unique ON app.staff_members(employee_number) WHERE employee_number<>'';
CREATE TABLE app.staff_member_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 entity_id bigint NOT NULL REFERENCES app.staff_members(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb CHECK(before_state IS NULL OR jsonb_typeof(before_state)='object'),
 after_state jsonb NOT NULL CHECK(jsonb_typeof(after_state)='object'),
 reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX staff_member_history_entity_idx ON app.staff_member_history(entity_id,id DESC);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Staff identity history requires an explicit recovery strategy'; END $$;
