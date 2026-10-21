-- +goose Up
CREATE TABLE app.master_imports (
 token_hash text PRIMARY KEY CHECK(length(token_hash)=64),
 owner_id bigint NOT NULL REFERENCES app.users(id),
 kind text NOT NULL CHECK(kind IN ('students','teachers')),
 payload jsonb NOT NULL,
 row_count integer NOT NULL CHECK(row_count BETWEEN 1 AND 500),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','complete','expired')),
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '15 minutes',
 completed_at timestamptz
);
CREATE INDEX master_imports_owner_idx ON app.master_imports(owner_id,state);

-- +goose Down
DROP TABLE app.master_imports;
