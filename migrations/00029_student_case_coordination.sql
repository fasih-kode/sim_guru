-- +goose Up
CREATE TABLE app.student_cases (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 student_id bigint NOT NULL REFERENCES app.students(id),
 student_name_at_open text NOT NULL CHECK(length(trim(student_name_at_open)) BETWEEN 1 AND 200),
 class_id_at_open bigint NOT NULL REFERENCES app.classes(id),
 class_name_at_open text NOT NULL CHECK(length(trim(class_name_at_open)) BETWEEN 1 AND 80),
 stage text NOT NULL CHECK(stage IN ('wali_kelas','guru_bk','kesiswaan')),
 responsible_user_id bigint NOT NULL REFERENCES app.users(id),
 status text NOT NULL DEFAULT 'perlu_tindak_lanjut' CHECK(status IN ('perlu_tindak_lanjut','selesai')),
 created_by bigint NOT NULL REFERENCES app.users(id),
 version bigint NOT NULL DEFAULT 1 CHECK(version > 0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX student_cases_student_idx ON app.student_cases(student_id,created_at DESC,id DESC);
CREATE INDEX student_cases_owner_idx ON app.student_cases(stage,responsible_user_id,status,updated_at DESC,id DESC);
CREATE INDEX student_cases_status_idx ON app.student_cases(status,updated_at DESC,id DESC);

CREATE TABLE app.student_case_entries (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 case_id bigint NOT NULL REFERENCES app.student_cases(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 entry_type text NOT NULL CHECK(entry_type IN ('wali_pembinaan','tindak_lanjut','bk_ringkasan','kesiswaan_keputusan')),
 content text NOT NULL CHECK(length(trim(content)) BETWEEN 1 AND 4000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX student_case_entries_case_idx ON app.student_case_entries(case_id,id);

-- Isi konseling pribadi terpisah dari ringkasan koordinasi dan tidak masuk
-- query daftar/pencarian kasus.
CREATE TABLE app.student_case_counseling_notes (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 case_id bigint NOT NULL REFERENCES app.student_cases(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 content text NOT NULL CHECK(length(trim(content)) BETWEEN 1 AND 8000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX student_case_counseling_case_idx ON app.student_case_counseling_notes(case_id,id DESC);

CREATE TABLE app.student_case_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 case_id bigint NOT NULL REFERENCES app.student_cases(id),
 actor_id bigint NOT NULL REFERENCES app.users(id),
 event_type text NOT NULL CHECK(event_type IN ('created','entry_added','counseling_note_added','referred_to_bk','referred_to_kesiswaan','completed','reopened')),
 from_stage text CHECK(from_stage IS NULL OR from_stage IN ('wali_kelas','guru_bk','kesiswaan')),
 to_stage text CHECK(to_stage IS NULL OR to_stage IN ('wali_kelas','guru_bk','kesiswaan')),
 from_responsible_user_id bigint REFERENCES app.users(id),
 to_responsible_user_id bigint REFERENCES app.users(id),
 reason text NOT NULL DEFAULT '' CHECK(length(reason) <= 1000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX student_case_history_case_idx ON app.student_case_history(case_id,id DESC);

ALTER TABLE app.student_guidance_records
 ADD COLUMN case_id bigint REFERENCES app.student_cases(id);
CREATE INDEX student_guidance_records_case_idx ON app.student_guidance_records(case_id) WHERE case_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION app.student_case_check_owner_stage() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE expected_role text;
BEGIN
 IF TG_OP = 'INSERT' THEN
  IF NEW.stage <> 'wali_kelas' OR NEW.created_by <> NEW.responsible_user_id THEN
   RAISE EXCEPTION 'kasus formal harus dimulai Wali Kelas';
  END IF;
 ELSE
  IF NEW.stage IS DISTINCT FROM OLD.stage THEN
   IF NOT ((OLD.stage='wali_kelas' AND NEW.stage='guru_bk') OR (OLD.stage='guru_bk' AND NEW.stage='kesiswaan')) THEN
    RAISE EXCEPTION 'tahap kasus hanya dapat maju sesuai alur';
   END IF;
   IF NEW.responsible_user_id IS NOT DISTINCT FROM OLD.responsible_user_id THEN
    RAISE EXCEPTION 'rujukan harus menetapkan penanggung jawab tahap tujuan';
   END IF;
  ELSIF NEW.responsible_user_id IS DISTINCT FROM OLD.responsible_user_id THEN
   RAISE EXCEPTION 'penanggung jawab tidak dapat diganti pada tahap yang sama';
  END IF;
 END IF;

 expected_role := CASE NEW.stage WHEN 'wali_kelas' THEN 'wali_kelas' WHEN 'guru_bk' THEN 'bk' ELSE 'kesiswaan' END;
 IF NOT EXISTS(SELECT 1 FROM app.users u JOIN app.user_roles r ON r.user_id=u.id
   WHERE u.id=NEW.responsible_user_id AND u.active AND r.role_code=expected_role) THEN
  RAISE EXCEPTION 'penanggung jawab harus memiliki akun aktif dan role tahap';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER student_cases_owner_stage_guard
 BEFORE INSERT OR UPDATE OF stage,responsible_user_id ON app.student_cases
 FOR EACH ROW EXECUTE FUNCTION app.student_case_check_owner_stage();

-- +goose StatementBegin
CREATE FUNCTION app.student_case_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'histori dan catatan kasus bersifat append-only';
END $$;
-- +goose StatementEnd
CREATE TRIGGER student_case_history_append_only
 BEFORE UPDATE OR DELETE ON app.student_case_history
 FOR EACH ROW EXECUTE FUNCTION app.student_case_append_only();
CREATE TRIGGER student_case_entries_append_only
 BEFORE UPDATE OR DELETE ON app.student_case_entries
 FOR EACH ROW EXECUTE FUNCTION app.student_case_append_only();
CREATE TRIGGER student_case_counseling_append_only
 BEFORE UPDATE OR DELETE ON app.student_case_counseling_notes
 FOR EACH ROW EXECUTE FUNCTION app.student_case_append_only();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Kasus dan catatan konseling memerlukan strategi pemulihan eksplisit'; END $$;
