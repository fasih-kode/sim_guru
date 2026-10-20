-- +goose Up
CREATE TABLE app.users (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 username text NOT NULL UNIQUE CHECK (username ~ '^[a-z0-9][a-z0-9._-]{2,63}$'),
 display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 120),
 password_hash text NOT NULL,
 active boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE app.roles (code text PRIMARY KEY, name text NOT NULL);
INSERT INTO app.roles(code,name) VALUES
 ('admin','Admin'),('kurikulum','Kurikulum'),('guru_mapel','Guru Mapel'),('wali_kelas','Wali Kelas'),
 ('kepala_sekolah','Kepala Sekolah'),('bk','BK'),('kesiswaan','Kesiswaan'),('petugas_absensi','Petugas Absensi');
CREATE TABLE app.user_roles (
 user_id bigint NOT NULL REFERENCES app.users(id),
 role_code text NOT NULL REFERENCES app.roles(code),
 PRIMARY KEY(user_id,role_code)
);
CREATE TABLE app.sessions (
 token_hash text PRIMARY KEY CHECK (length(token_hash)=64),
 user_id bigint NOT NULL REFERENCES app.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 revoked_at timestamptz
);
CREATE INDEX sessions_user_idx ON app.sessions(user_id);
CREATE INDEX sessions_expiry_idx ON app.sessions(expires_at);
CREATE TABLE app.access_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor_id bigint REFERENCES app.users(id),
 target_id bigint NOT NULL REFERENCES app.users(id),
 action text NOT NULL,
 before_state jsonb,
 after_state jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX access_audit_time_idx ON app.access_audit(id DESC);

-- +goose Down
DROP TABLE app.access_audit;
DROP TABLE app.sessions;
DROP TABLE app.user_roles;
DROP TABLE app.roles;
DROP TABLE app.users;
