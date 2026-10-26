-- +goose Up
CREATE TABLE app.teacher_attendance_location (
 id smallint PRIMARY KEY DEFAULT 1 CHECK(id=1),
 latitude double precision NOT NULL CHECK(latitude BETWEEN -90 AND 90),
 longitude double precision NOT NULL CHECK(longitude BETWEEN -180 AND 180),
 radius_m integer NOT NULL DEFAULT 100 CHECK(radius_m BETWEEN 25 AND 500),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 updated_by bigint NOT NULL REFERENCES app.users(id),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE app.teacher_attendance_location_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 location_id smallint NOT NULL DEFAULT 1 REFERENCES app.teacher_attendance_location(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb CHECK(before_state IS NULL OR jsonb_typeof(before_state)='object'),
 after_state jsonb NOT NULL CHECK(jsonb_typeof(after_state)='object'),
 reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX teacher_attendance_location_history_idx ON app.teacher_attendance_location_history(id DESC);

CREATE TABLE app.teacher_attendance_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 teacher_id bigint NOT NULL REFERENCES app.teachers(id),
 teacher_name text NOT NULL CHECK(length(trim(teacher_name))>0),
 on_date date NOT NULL,
 action text NOT NULL CHECK(action IN('check_in','check_out')),
 source text NOT NULL CHECK(source IN('gps_selfie','manual')),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 occurred_at timestamptz NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 distance_m double precision,
 accuracy_m double precision,
 school_latitude double precision,
 school_longitude double precision,
 geofence_radius_m integer,
 photo_metadata_id bigint,
 manual_reason text,
 UNIQUE(teacher_id,on_date,action),
 UNIQUE(id,photo_metadata_id),
 CHECK((occurred_at AT TIME ZONE 'Asia/Jakarta')::date=on_date),
 CHECK(distance_m IS NULL OR distance_m>=0 AND distance_m<'Infinity'::double precision),
 CHECK(accuracy_m IS NULL OR accuracy_m BETWEEN 0 AND 50),
 CHECK(school_latitude IS NULL OR school_latitude BETWEEN -90 AND 90),
 CHECK(school_longitude IS NULL OR school_longitude BETWEEN -180 AND 180),
 CHECK(geofence_radius_m IS NULL OR geofence_radius_m BETWEEN 25 AND 500),
 CHECK((source='gps_selfie' AND distance_m IS NOT NULL AND accuracy_m IS NOT NULL AND school_latitude IS NOT NULL AND school_longitude IS NOT NULL AND geofence_radius_m IS NOT NULL AND photo_metadata_id IS NOT NULL AND manual_reason IS NULL)
    OR (source='manual' AND distance_m IS NULL AND accuracy_m IS NULL AND school_latitude IS NULL AND school_longitude IS NULL AND geofence_radius_m IS NULL AND photo_metadata_id IS NULL AND length(trim(coalesce(manual_reason,''))) BETWEEN 1 AND 500))
);
CREATE INDEX teacher_attendance_events_teacher_date_idx ON app.teacher_attendance_events(teacher_id,on_date DESC);

CREATE TABLE app.teacher_attendance_photos (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL UNIQUE REFERENCES app.teacher_attendance_events(id),
 storage_key text NOT NULL UNIQUE CHECK(length(storage_key) BETWEEN 32 AND 200),
 mime_type text NOT NULL CHECK(mime_type IN('image/jpeg','image/png','image/webp')),
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
ALTER TABLE app.teacher_attendance_events
 ADD CONSTRAINT teacher_attendance_event_photo_fk
 FOREIGN KEY(id,photo_metadata_id) REFERENCES app.teacher_attendance_photos(event_id,id)
 DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE app.teacher_attendance_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL REFERENCES app.teacher_attendance_events(id),
 action text NOT NULL CHECK(action IN('recorded','corrected')),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 before_state jsonb CHECK(before_state IS NULL OR jsonb_typeof(before_state)='object'),
 after_state jsonb NOT NULL CHECK(jsonb_typeof(after_state)='object'),
 reason text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((action='recorded' AND before_state IS NULL AND reason IS NULL) OR
       (action='corrected' AND before_state IS NOT NULL AND length(trim(coalesce(reason,''))) BETWEEN 1 AND 500))
);
CREATE INDEX teacher_attendance_history_event_idx ON app.teacher_attendance_history(event_id,id DESC);

-- Constraint tertunda menjaga event GPS/selfie dan metadata fotonya atomik.
-- +goose StatementBegin
CREATE FUNCTION app.require_teacher_attendance_photo() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.source='gps_selfie' AND NOT EXISTS (
   SELECT 1 FROM app.teacher_attendance_photos p WHERE p.event_id=NEW.id AND p.id=NEW.photo_metadata_id
 ) THEN
   RAISE EXCEPTION 'GPS/selfie attendance requires photo metadata' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER teacher_attendance_photo_required
 AFTER INSERT OR UPDATE OF source,photo_metadata_id ON app.teacher_attendance_events
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.require_teacher_attendance_photo();
-- +goose StatementBegin
CREATE FUNCTION app.require_matching_teacher_attendance_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (
   SELECT 1 FROM app.teacher_attendance_events e
   WHERE e.id=NEW.event_id AND e.source='gps_selfie' AND e.photo_metadata_id=NEW.id
 ) THEN
   RAISE EXCEPTION 'Photo metadata must belong to a GPS/selfie attendance event' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER teacher_attendance_photo_event_match
 AFTER INSERT OR UPDATE OF event_id ON app.teacher_attendance_photos
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.require_matching_teacher_attendance_event();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Teacher attendance evidence requires an explicit recovery strategy'; END $$;
