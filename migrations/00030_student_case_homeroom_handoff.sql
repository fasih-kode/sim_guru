-- +goose Up
ALTER TABLE app.student_case_history
 DROP CONSTRAINT student_case_history_event_type_check;
ALTER TABLE app.student_case_history
 ADD CONSTRAINT student_case_history_event_type_check
 CHECK(event_type IN ('created','entry_added','counseling_note_added','referred_to_bk','referred_to_kesiswaan','wali_handoff','completed','reopened'));

-- Pemilik kasus dapat berubah pada tahap Wali hanya melalui handoff terkontrol.
-- Perpindahan tahap normal tetap mengikuti guard migration 00029.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.student_case_check_owner_stage() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 expected_role text;
 active_placement_count integer;
 active_wali_count integer;
 active_wali_user_id bigint;
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
   IF OLD.stage <> 'wali_kelas' OR NEW.stage <> 'wali_kelas'
      OR OLD.status <> 'perlu_tindak_lanjut' OR NEW.status <> OLD.status
      OR NEW.student_id <> OLD.student_id THEN
    RAISE EXCEPTION 'handoff hanya berlaku untuk kasus aktif tahap Wali Kelas';
   END IF;

   SELECT count(*) INTO active_placement_count
   FROM app.student_placements p
   JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
   WHERE p.student_id=NEW.student_id
     AND p.starts_on IS NOT NULL
     AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
     AND app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL;
   IF active_placement_count <> 1 THEN
    RAISE EXCEPTION 'handoff memerlukan satu penempatan siswa aktif yang jelas';
   END IF;

   SELECT count(DISTINCT u.id),min(u.id) INTO active_wali_count,active_wali_user_id
   FROM app.student_placements p
   JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
   JOIN app.classes c ON c.id=app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AND c.school_year_id=sy.id
   JOIN app.homeroom_assignments h ON h.class_id=c.id
   JOIN app.teachers t ON t.id=h.teacher_id AND t.active
   JOIN app.users u ON u.id=t.user_id AND u.active
   JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='wali_kelas'
   WHERE p.student_id=NEW.student_id
     AND p.starts_on IS NOT NULL
     AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
     AND app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL;
   IF active_wali_count <> 1 OR active_wali_user_id IS DISTINCT FROM NEW.responsible_user_id THEN
    RAISE EXCEPTION 'handoff harus menunjuk Wali Kelas aktif pada penempatan siswa';
   END IF;
   IF active_wali_user_id=OLD.responsible_user_id THEN
    RAISE EXCEPTION 'penanggung jawab lama masih menjadi Wali Kelas aktif siswa';
   END IF;
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

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Handoff kasus telah dicatat dalam histori; pemulihan memerlukan strategi data eksplisit'; END $$;
