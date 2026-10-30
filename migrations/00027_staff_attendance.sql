-- +goose Up
CREATE TABLE app.staff_attendance_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 staff_member_id bigint NOT NULL REFERENCES app.staff_members(id),
 staff_name text NOT NULL CHECK(length(trim(staff_name)) BETWEEN 1 AND 200),
 staff_user_id bigint NOT NULL REFERENCES app.users(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 on_date date NOT NULL,
 action text NOT NULL CHECK(action IN('check_in','check_out')),
 source text NOT NULL DEFAULT 'gps_selfie' CHECK(source='gps_selfie'),
 occurred_at timestamptz NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 distance_m double precision NOT NULL CHECK(distance_m>=0 AND distance_m<'Infinity'::double precision),
 accuracy_m double precision NOT NULL CHECK(accuracy_m BETWEEN 0 AND 50),
 school_latitude double precision NOT NULL CHECK(school_latitude BETWEEN -90 AND 90),
 school_longitude double precision NOT NULL CHECK(school_longitude BETWEEN -180 AND 180),
 geofence_radius_m integer NOT NULL CHECK(geofence_radius_m BETWEEN 25 AND 500),
 location_version bigint NOT NULL CHECK(location_version>0),
 photo_metadata_id bigint NOT NULL,
 idempotency_key_hash char(64) NOT NULL CHECK(idempotency_key_hash ~ '^[0-9a-f]{64}$'),
 input_fingerprint char(64) NOT NULL CHECK(input_fingerprint ~ '^[0-9a-f]{64}$'),
 device_location_proof char(64) NOT NULL CHECK(device_location_proof ~ '^[0-9a-f]{64}$'),
 UNIQUE(staff_member_id,on_date,action),
 UNIQUE(staff_member_id,staff_user_id,action,idempotency_key_hash),
 UNIQUE(id,photo_metadata_id),
 CHECK((occurred_at AT TIME ZONE 'Asia/Jakarta')::date=on_date)
);
CREATE INDEX staff_attendance_events_staff_date_idx ON app.staff_attendance_events(staff_member_id,on_date DESC,id DESC);

CREATE TABLE app.staff_attendance_photos (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL UNIQUE REFERENCES app.staff_attendance_events(id),
 storage_key text NOT NULL UNIQUE CHECK(length(storage_key) BETWEEN 32 AND 200),
 mime_type text NOT NULL CHECK(mime_type='image/jpeg'),
 size_bytes integer NOT NULL CHECK(size_bytes BETWEEN 1 AND 102400),
 checksum_sha256 char(64) NOT NULL CHECK(checksum_sha256 ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 retain_until timestamptz NOT NULL,
 deleted_at timestamptz,
 cleanup_attempts integer NOT NULL DEFAULT 0 CHECK(cleanup_attempts>=0),
 last_cleanup_error text,
 UNIQUE(event_id,id),
 CHECK(retain_until>=created_at)
);
ALTER TABLE app.staff_attendance_events
 ADD CONSTRAINT staff_attendance_event_photo_fk
 FOREIGN KEY(id,photo_metadata_id) REFERENCES app.staff_attendance_photos(event_id,id)
 DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE app.staff_attendance_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL REFERENCES app.staff_attendance_events(id),
 action text NOT NULL CHECK(action='recorded'),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb CHECK(before_state IS NULL OR jsonb_typeof(before_state)='object'),
 after_state jsonb NOT NULL CHECK(jsonb_typeof(after_state)='object'),
 reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(before_state IS NULL)
);
CREATE INDEX staff_attendance_history_event_idx ON app.staff_attendance_history(event_id,id DESC);

-- Constraint tertunda memastikan snapshot event dan metadata foto tersimpan atomik.
-- +goose StatementBegin
CREATE FUNCTION app.require_staff_attendance_photo() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (
   SELECT 1 FROM app.staff_attendance_photos p
   WHERE p.event_id=NEW.id AND p.id=NEW.photo_metadata_id
 ) THEN
   RAISE EXCEPTION 'Staff attendance requires matching photo metadata' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER staff_attendance_photo_required
 AFTER INSERT OR UPDATE OF photo_metadata_id ON app.staff_attendance_events
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.require_staff_attendance_photo();
-- +goose StatementBegin
CREATE FUNCTION app.require_matching_staff_attendance_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (
   SELECT 1 FROM app.staff_attendance_events e
   WHERE e.id=NEW.event_id AND e.photo_metadata_id=NEW.id
 ) THEN
   RAISE EXCEPTION 'Staff photo metadata must match its attendance event' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER staff_attendance_photo_event_match
 AFTER INSERT OR UPDATE OF event_id ON app.staff_attendance_photos
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.require_matching_staff_attendance_event();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Staff attendance evidence requires an explicit recovery strategy'; END $$;
