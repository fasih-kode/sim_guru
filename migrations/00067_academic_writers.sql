-- +goose Up
-- Role login disediakan melalui provisioning terpisah. Migration berjalan
-- sebagai owner yang memiliki objek, tanpa membuat role atau mengubah HBA.
-- +goose StatementBegin
DO $$
BEGIN
 IF EXISTS (
  SELECT 1 FROM pg_roles r
  WHERE r.rolname IN ('sim_guru_app','ux02e_placement_writer','ux02e_calendar_writer','ux02e_tp_score_writer','ux02e_s06_writer')
    AND (r.rolsuper OR r.rolcreaterole OR pg_has_role(r.oid,current_user,'SET'))
 ) OR EXISTS (
  SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname='app' AND c.relname IN
        ('student_placements','semester_calendars','school_date_exceptions','attendance_foundation_history',
         'direct_tp_applicability_history','direct_tp_initial_scores','direct_tp_score_corrections','direct_tp_submissions')
     AND pg_get_userbyid(c.relowner)<>current_user
 ) THEN
  RAISE EXCEPTION 'Pemisahan owner dan role runtime diperlukan sebelum migration penulis akademik';
 END IF;
END $$;

CREATE SCHEMA academic_write;
REVOKE ALL ON SCHEMA academic_write FROM PUBLIC;
GRANT USAGE ON SCHEMA academic_write TO sim_guru_app,ux02e_placement_writer,ux02e_calendar_writer;

CREATE FUNCTION academic_write.require_actor(p_actor bigint,p_role text) RETURNS void
LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM app.users u JOIN app.user_roles r ON r.user_id=u.id
                WHERE u.id=p_actor AND u.active AND r.role_code=p_role) THEN
  RAISE EXCEPTION 'Aktor tidak berwenang' USING ERRCODE='42501';
 END IF;
END $$;

CREATE FUNCTION academic_write.lock_write() RETURNS void
LANGUAGE plpgsql VOLATILE SET search_path=pg_catalog,pg_temp AS $$
BEGIN
 IF current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION 'Penulis fakta wajib READ COMMITTED' USING ERRCODE='25001';
 END IF;
 PERFORM pg_advisory_xact_lock(742009::bigint);
END $$;

CREATE FUNCTION academic_write.trim_evidence(p_value text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT SET search_path=pg_catalog,pg_temp AS $$
 SELECT btrim(p_value, U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000')
$$;

CREATE FUNCTION academic_write.validate_reason(p_reason text) RETURNS void
LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $$
BEGIN
 IF p_reason IS NULL OR academic_write.trim_evidence(p_reason)='' OR char_length(p_reason)>500 THEN
  RAISE EXCEPTION 'Alasan tidak valid' USING ERRCODE='22023';
 END IF;
END $$;

CREATE FUNCTION academic_write.calendar_state(p_semester bigint) RETURNS jsonb
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
 SELECT jsonb_build_object('SemesterID',s.id,'Version',coalesce(c.version,0),'StartsOn',coalesce(c.starts_on::text,''),
  'EndsOn',coalesce(c.ends_on::text,''),'SchoolDays',c.school_days,'Exceptions',(
   SELECT jsonb_agg(jsonb_build_object('Date',x.on_date,'SchoolDay',x.school_day,'Label',x.label) ORDER BY x.on_date)
    FROM app.school_date_exceptions x WHERE x.semester_id=s.id))
 FROM app.semesters s LEFT JOIN app.semester_calendars c ON c.semester_id=s.id WHERE s.id=p_semester
$$;


-- Digest v2 sengaja tidak memakai serialisasi json.Marshal milik kontrak lama.
-- Kunci hanya dibaca helper owner-only. Role writer tidak bisa menerbitkan digest.
CREATE SCHEMA academic_write_crypto;
REVOKE ALL ON SCHEMA academic_write_crypto FROM PUBLIC;
CREATE EXTENSION pgcrypto WITH SCHEMA academic_write_crypto;
CREATE TABLE academic_write.placement_digest_key (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    secret bytea NOT NULL CHECK (octet_length(secret) = 32)
);
REVOKE ALL ON academic_write.placement_digest_key FROM PUBLIC;
INSERT INTO academic_write.placement_digest_key(secret) VALUES (academic_write_crypto.gen_random_bytes(32));

CREATE FUNCTION academic_write.placement_state(p_id bigint, p_year bigint, p_class bigint, p_date date)
RETURNS TABLE (
    id bigint, year_id bigint, date_version bigint, student_name text,
    class_name text, starts_on text, date_valid boolean, facts jsonb
)
LANGUAGE sql STABLE
SET search_path = pg_catalog, pg_temp
AS $$
 SELECT p.id, p.school_year_id, p.date_version, s.name, c.grade::text || c.name, coalesce(to_char(p.starts_on,'YYYY-MM-DD'),''),
   p_date IS NOT NULL AND isfinite(p_date) AND NOT (
    EXISTS (SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND m.effective_date<p_date)
    OR EXISTS (SELECT 1 FROM app.student_school_exits x JOIN app.classes origin ON origin.id=x.from_class_id
       WHERE x.student_id=p.student_id AND origin.school_year_id=p.school_year_id AND x.effective_date<p_date)
    OR EXISTS (SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id
       AND (r.effective_date<p_date OR (r.created_placement AND r.effective_date<>p_date)))
    OR EXISTS (SELECT 1 FROM app.semester_calendars cal JOIN app.semesters sm ON sm.id=cal.semester_id
       WHERE sm.school_year_id=p.school_year_id AND ((sm.number=1 AND p_date<cal.starts_on) OR (sm.number=2 AND p_date>cal.ends_on)))),
   jsonb_build_array(
      jsonb_build_array(p.id,p.school_year_id,p.student_id,p.class_id,p.actor_id,to_char(p.starts_on,'YYYY-MM-DD'),p.date_version),
      jsonb_build_array(s.id,s.name,s.active,s.version),
      jsonb_build_array(c.id,c.school_year_id,c.grade,c.name,c.version),
      (SELECT jsonb_agg(jsonb_build_array(m.id,m.from_class_id,m.to_class_id,to_char(m.effective_date,'YYYY-MM-DD'),m.actor_id,m.reason) ORDER BY m.id)
       FROM app.student_class_moves m WHERE m.placement_id=p.id),
      (SELECT jsonb_agg(jsonb_build_array(x.id,x.from_class_id,origin.school_year_id,to_char(x.effective_date,'YYYY-MM-DD'),x.actor_id,x.reason) ORDER BY x.id)
       FROM app.student_school_exits x JOIN app.classes origin ON origin.id=x.from_class_id WHERE x.student_id=p.student_id),
      (SELECT jsonb_agg(jsonb_build_array(r.id,r.exit_id,r.target_class_id,to_char(r.effective_date,'YYYY-MM-DD'),r.created_placement,r.actor_id,r.reason) ORDER BY r.id)
       FROM app.student_school_returns r WHERE r.placement_id=p.id),
      (SELECT jsonb_agg(jsonb_build_array(sm.id,sm.number,to_char(cal.starts_on,'YYYY-MM-DD'),to_char(cal.ends_on,'YYYY-MM-DD'),cal.school_days,cal.version) ORDER BY sm.id)
       FROM app.semester_calendars cal JOIN app.semesters sm ON sm.id=cal.semester_id WHERE sm.school_year_id=p.school_year_id))
 FROM app.student_placements p JOIN app.students s ON s.id=p.student_id JOIN app.classes c ON c.id=p.class_id
 WHERE p.id=p_id AND p.school_year_id=p_year AND (p_class=0 OR p.class_id=p_class)
$$;

CREATE FUNCTION academic_write.validate_placement_batch(p_year bigint, p_class bigint, p_ids bigint[], p_date date, p_reason text)
RETURNS void LANGUAGE plpgsql IMMUTABLE SET search_path = pg_catalog, pg_temp AS $$
BEGIN
 PERFORM academic_write.validate_reason(p_reason);
 IF p_year IS NULL OR p_year<1 OR p_class IS NULL OR p_class<0
    OR coalesce(cardinality(p_ids),0) NOT BETWEEN 1 AND 5000 OR array_ndims(p_ids) IS DISTINCT FROM 1
    OR EXISTS (SELECT 1 FROM unnest(p_ids) id WHERE id IS NULL OR id<1)
    OR cardinality(p_ids)<>(SELECT count(DISTINCT id) FROM unnest(p_ids) id)
    OR p_date IS NULL OR p_date NOT BETWEEN date '1900-01-01' AND date '2200-12-31' THEN
  RAISE EXCEPTION 'Konteks atau daftar penempatan tidak valid' USING ERRCODE='22023';
 END IF;
END $$;

-- Fungsi privat yang sama menghasilkan digest untuk preview dan konfirmasi.
CREATE FUNCTION academic_write.placement_batch_state(
    p_actor bigint, p_year bigint, p_class bigint, p_ids bigint[], p_date date, p_reason text, p_batch boolean
)
RETURNS TABLE (
    id bigint, year_id bigint, date_version bigint, student_name text,
    class_name text, starts_on text, date_valid boolean, digest text
)
LANGUAGE sql STABLE SET search_path = pg_catalog, pg_temp AS $$
 WITH rows AS MATERIALIZED (
   SELECT st.* FROM unnest(p_ids) AS requested(id)
   CROSS JOIN LATERAL academic_write.placement_state(requested.id,p_year,p_class,p_date) st
 ), fingerprint AS (
   SELECT 'v2:' || encode(academic_write_crypto.hmac(
     convert_to(jsonb_build_array(
       'placement-start-batch-v2',p_batch,p_actor,p_year,p_class,to_char(p_date,'YYYY-MM-DD'),
       academic_write.trim_evidence(p_reason),jsonb_agg(r.facts ORDER BY r.id)
     )::text,'UTF8'),
     (SELECT k.secret FROM academic_write.placement_digest_key k WHERE k.singleton),'sha256'),'hex') AS digest
    FROM rows r HAVING count(*)=cardinality(p_ids)
 )
 SELECT r.id,r.year_id,r.date_version,r.student_name,r.class_name,r.starts_on,r.date_valid,f.digest
 FROM rows r CROSS JOIN fingerprint f ORDER BY r.id
$$;

CREATE FUNCTION academic_write.preview_placement_batch(
    p_actor bigint, p_year bigint, p_class bigint, p_ids bigint[], p_date date, p_reason text, p_batch boolean
)
RETURNS TABLE (
    id bigint, year_id bigint, date_version bigint, student_name text,
    class_name text, starts_on text, date_valid boolean, digest text
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
BEGIN
 PERFORM academic_write.require_actor(p_actor,'admin');
 PERFORM academic_write.validate_placement_batch(p_year,p_class,p_ids,p_date,p_reason);
 RETURN QUERY SELECT b.id,b.year_id,b.date_version,b.student_name,b.class_name,b.starts_on,b.date_valid,b.digest
 FROM academic_write.placement_batch_state(p_actor,p_year,p_class,p_ids,p_date,p_reason,p_batch) b;
END $$;

-- Fungsi privat hanya dipanggil oleh guard definer yang sudah terpasang.
CREATE FUNCTION academic_write.class_relevant(p_id bigint,p_class bigint)
RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM app.student_placements p WHERE p.id=p_id AND (
   p.class_id=p_class OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id
       AND (m.from_class_id=p_class OR m.to_class_id=p_class))
   OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=p_class)))
$$;

CREATE FUNCTION academic_write.member_on(p_id bigint,p_class bigint,p_evidence_date date,p_effective_start date)
RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
 -- Riwayat class_at dievaluasi dengan tanggal mulai kandidat. Tanggal NULL tetap tidak diketahui.
 SELECT CASE
   WHEN p_effective_start IS NULL THEN NULL::boolean
   WHEN p_evidence_date<p_effective_start THEN false
   WHEN p.starts_on IS NULL OR p.starts_on<=p_evidence_date
        THEN app.student_placement_class_at(p.id,p_evidence_date) IS NOT DISTINCT FROM p_class
   -- class_at lama menutup tanggal ini karena starts_on tersimpan lebih baru.
   -- Evaluasi ulang move/exit/return tanpa menulis tanggal kandidat.
   WHEN last_event.kind='exit' OR EXISTS(SELECT 1 FROM app.student_school_returns r
        WHERE r.placement_id=p.id AND r.created_placement AND r.effective_date>p_evidence_date) THEN false
   ELSE coalesce(last_class.class_id,p.class_id)=p_class
 END
 FROM app.student_placements p
 LEFT JOIN LATERAL (
  SELECT v.kind FROM (
    SELECT 'exit' AS kind,e.effective_date,e.id,1 AS priority FROM app.student_school_exits e
       WHERE e.student_id=p.student_id AND e.effective_date<=p_evidence_date
    UNION ALL
    SELECT 'return',r.effective_date,r.id,2 FROM app.student_school_returns r
       JOIN app.student_school_exits e ON e.id=r.exit_id WHERE e.student_id=p.student_id AND r.effective_date<=p_evidence_date
  ) v ORDER BY v.effective_date DESC,v.priority DESC,v.id DESC LIMIT 1
 ) last_event ON true
 LEFT JOIN LATERAL (
  SELECT v.class_id FROM (
    SELECT m.to_class_id AS class_id,m.effective_date,m.id,1 AS priority FROM app.student_class_moves m
      WHERE m.placement_id=p.id AND m.effective_date<=p_evidence_date
    UNION ALL
    SELECT r.target_class_id,r.effective_date,r.id,2 FROM app.student_school_returns r
      WHERE r.placement_id=p.id AND r.effective_date<=p_evidence_date
  ) v ORDER BY v.effective_date DESC,v.priority DESC,v.id DESC LIMIT 1
 ) last_class ON true
 WHERE p.id=p_id
$$;

CREATE FUNCTION academic_write.academic_evidence(p_year bigint,p_semester bigint,p_placement bigint)
RETURNS TABLE(semester_id bigint,class_id bigint,subject_id bigint,evidence_date date,linked_placement bigint,manual_review boolean)
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
 WITH activities AS (
   SELECT a.id,a.semester_id,a.class_id,a.subject_id,a.activity_date
   FROM app.assessment_activities a
   WHERE a.school_year_id=p_year AND a.activity_type='sumatif' AND (p_semester IS NULL OR a.semester_id=p_semester)
 ), scores AS (
   SELECT s.id,s.semester_id,s.class_id,s.subject_id,s.activity_date_snapshot,s.placement_id,a.activity_date
   FROM app.assessment_activity_initial_scores s JOIN activities a ON a.id=s.assessment_activity_id
   WHERE p_placement IS NULL OR s.placement_id=p_placement
 )
 SELECT a.semester_id,a.class_id,a.subject_id,a.activity_date,NULL::bigint,
   NOT EXISTS(SELECT 1 FROM app.assessment_activity_objectives b WHERE b.activity_id=a.id)
 FROM activities a
 UNION ALL
 -- Koreksi tetap terikat ke bukti asal ini. corrected_at hanya waktu pencatatan.
 SELECT s.semester_id,s.class_id,s.subject_id,s.activity_date_snapshot,s.placement_id,
   s.activity_date_snapshot IS DISTINCT FROM s.activity_date
 FROM scores s
 UNION ALL
 SELECT s.semester_id,s.class_id,s.subject_id,r.remedial_date,s.placement_id,false
 FROM scores s JOIN app.assessment_activity_score_remedials r ON r.initial_score_id=s.id
 UNION ALL
 SELECT o.semester_id,o.class_id,o.subject_id,h.assessment_date,NULL::bigint,
   NOT coalesce((h.semester_id,h.class_id,h.subject_id)=(o.semester_id,o.class_id,o.subject_id)
     AND h.verification->'Acknowledged'='true'::jsonb
     AND nullif(btrim(h.verification->>'LearningSection'),'') IS NOT NULL
     AND nullif(btrim(h.verification->>'AssessmentSection'),'') IS NOT NULL
     AND nullif(btrim(h.verification->>'Method'),'') IS NOT NULL
     AND nullif(btrim(h.verification->>'ResultSource'),'') IS NOT NULL
     AND CASE h.learning_evidence->>'Kind'
          WHEN 'app' THEN nullif(btrim(h.learning_evidence->>'SourceID'),'') IS NOT NULL
          WHEN 'manual' THEN nullif(btrim(h.learning_evidence->>'Document'),'') IS NOT NULL
                         AND nullif(btrim(h.learning_evidence->>'Archive'),'') IS NOT NULL
          ELSE false END
     AND CASE h.assessment_evidence->>'Kind'
          WHEN 'app' THEN nullif(btrim(h.assessment_evidence->>'SourceID'),'') IS NOT NULL
          WHEN 'manual' THEN nullif(btrim(h.assessment_evidence->>'Document'),'') IS NOT NULL
                         AND nullif(btrim(h.assessment_evidence->>'Archive'),'') IS NOT NULL
          ELSE false END,false)
 FROM app.historical_learning_objective_insertions h JOIN app.learning_objectives o ON o.id=h.learning_objective_id
 WHERE o.school_year_id=p_year AND (p_semester IS NULL OR o.semester_id=p_semester)
$$;

CREATE FUNCTION academic_write.check_calendar_evidence(
  p_year bigint,p_semester bigint,p_old_start date,p_old_end date,p_new_start date,p_new_end date)
RETURNS void LANGUAGE plpgsql STABLE SET search_path=pg_catalog,pg_temp AS $$
DECLARE uncertain boolean;
BEGIN
 SELECT bool_or(r.manual_review OR (p.id IS NOT NULL AND member.possible IS NULL))
 INTO uncertain
 FROM academic_write.academic_evidence(p_year,p_semester,NULL) r
 LEFT JOIN app.student_placements p ON r.linked_placement IS NULL AND p.school_year_id=p_year
      AND academic_write.class_relevant(p.id,r.class_id)
 LEFT JOIN LATERAL (
   SELECT academic_write.member_on(p.id,r.class_id,r.evidence_date,p.starts_on) AS possible
 ) member ON p.id IS NOT NULL
 WHERE (r.evidence_date BETWEEN p_old_start AND p_old_end) IS DISTINCT FROM (r.evidence_date BETWEEN p_new_start AND p_new_end)
   AND (r.linked_placement IS NOT NULL OR (p.id IS NOT NULL AND member.possible IS DISTINCT FROM false));
 IF uncertain IS NOT NULL THEN
  RAISE EXCEPTION '%',CASE WHEN uncertain THEN 'Bukti akademik terkait perlu verifikasi manual'
    ELSE 'Perubahan rentang memengaruhi bukti akademik bertanggal' END USING ERRCODE='23514';
 END IF;
END $$;

CREATE FUNCTION academic_write.check_placement_evidence(p_year bigint,p_placement bigint,p_old_start date,p_new_start date)
RETURNS void LANGUAGE plpgsql STABLE SET search_path=pg_catalog,pg_temp AS $$
DECLARE uncertain boolean;
BEGIN
 SELECT bool_or(r.manual_review OR member.old_value IS NULL OR member.new_value IS NULL)
 INTO uncertain
 FROM academic_write.academic_evidence(p_year,NULL,p_placement) r
 CROSS JOIN LATERAL (
   SELECT academic_write.member_on(p_placement,r.class_id,r.evidence_date,p_old_start) AS old_value,
          academic_write.member_on(p_placement,r.class_id,r.evidence_date,p_new_start) AS new_value
 ) member
 WHERE (r.linked_placement=p_placement OR (r.linked_placement IS NULL AND academic_write.class_relevant(p_placement,r.class_id)))
   AND member.old_value IS DISTINCT FROM member.new_value;
 IF uncertain IS NOT NULL THEN
  RAISE EXCEPTION '%',CASE WHEN uncertain THEN 'Bukti akademik terkait perlu verifikasi manual'
    ELSE 'Perubahan tanggal memengaruhi bukti akademik bertanggal' END USING ERRCODE='23514';
 END IF;
END $$;

REVOKE EXECUTE ON FUNCTION
 academic_write.class_relevant(bigint,bigint),academic_write.member_on(bigint,bigint,date,date),
 academic_write.academic_evidence(bigint,bigint,bigint),
 academic_write.check_calendar_evidence(bigint,bigint,date,date,date,date),
 academic_write.check_placement_evidence(bigint,bigint,date,date) FROM PUBLIC;

CREATE FUNCTION academic_write.set_calendar(p_actor bigint, p_semester bigint, p_version bigint, p_start date, p_end date, p_days jsonb, p_reason text)
 RETURNS void
 LANGUAGE plpgsql
 SECURITY DEFINER
 SET search_path TO 'pg_catalog', 'pg_temp'
AS $function$
DECLARE
 old_state jsonb;
 year_id bigint;
 semester_number integer;
 days jsonb;
BEGIN
 PERFORM academic_write.lock_write();
 PERFORM academic_write.require_actor(p_actor,'kurikulum');
 PERFORM academic_write.validate_reason(p_reason);
 IF p_start IS NULL OR p_end IS NULL OR NOT isfinite(p_start) OR NOT isfinite(p_end) OR p_start>p_end
    OR p_start<date '1900-01-01' OR p_end>date '2200-12-31' OR p_end-p_start>366
    OR p_days IS NULL OR jsonb_typeof(p_days)<>'array' THEN
  RAISE EXCEPTION 'Rentang atau hari sekolah tidak valid' USING ERRCODE='22023';
 END IF;
 SELECT jsonb_agg(v ORDER BY v) INTO days FROM (SELECT DISTINCT v FROM jsonb_array_elements(p_days) AS v) d;
 IF jsonb_array_length(p_days) NOT BETWEEN 1 AND 7 OR jsonb_array_length(days)<>jsonb_array_length(p_days)
    OR EXISTS(SELECT 1 FROM jsonb_array_elements(p_days) AS v WHERE (v::text COLLATE "C") !~ '^[1-7]$') THEN
  RAISE EXCEPTION 'Hari sekolah tidak valid' USING ERRCODE='22023';
 END IF;
 SELECT school_year_id,number INTO year_id,semester_number FROM app.semesters WHERE id=p_semester;
 old_state:=academic_write.calendar_state(p_semester);
 IF old_state IS NULL OR (old_state->>'Version')::bigint IS DISTINCT FROM p_version THEN
  RAISE EXCEPTION 'Versi kalender berubah' USING ERRCODE='40001';
 END IF;
 IF EXISTS(SELECT 1 FROM app.semester_calendars c JOIN app.semesters s ON s.id=c.semester_id
   WHERE s.school_year_id=year_id AND s.id<>p_semester
     AND ((s.number<semester_number AND c.ends_on>=p_start) OR (s.number>semester_number AND c.starts_on<=p_end)))
    OR EXISTS(SELECT 1 FROM app.school_date_exceptions x WHERE x.semester_id=p_semester
      AND x.on_date NOT BETWEEN p_start AND p_end) THEN
  RAISE EXCEPTION 'Rentang kalender bertentangan dengan fakta yang tercatat' USING ERRCODE='22023';
 END IF;
 IF ((old_state->>'StartsOn') IS DISTINCT FROM p_start::text OR (old_state->>'EndsOn') IS DISTINCT FROM p_end::text)
    AND (
      EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=p_semester)
      OR EXISTS(SELECT 1 FROM (
          SELECT DISTINCT ON(h.class_id,h.subject_id,h.student_id,h.objective_id) h.effective_date
          FROM app.direct_tp_applicability_history h WHERE h.semester_id=p_semester
          ORDER BY h.class_id,h.subject_id,h.student_id,h.objective_id,h.version DESC) d
        WHERE d.effective_date NOT BETWEEN p_start AND p_end)
      OR EXISTS(SELECT 1 FROM app.direct_tp_initial_scores s WHERE s.semester_id=p_semester)
    ) THEN
  RAISE EXCEPTION 'Review bukti nilai, keputusan atau Final sebelum mengubah rentang' USING ERRCODE='23514';
 END IF;
 IF ((old_state->>'StartsOn') IS DISTINCT FROM p_start::text OR (old_state->>'EndsOn') IS DISTINCT FROM p_end::text) THEN
  PERFORM academic_write.check_calendar_evidence(year_id,p_semester,
    nullif(old_state->>'StartsOn','')::date,nullif(old_state->>'EndsOn','')::date,p_start,p_end);
 END IF;
 INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES(p_semester,p_start,p_end,days)
  ON CONFLICT(semester_id) DO UPDATE SET starts_on=excluded.starts_on,ends_on=excluded.ends_on,
    school_days=excluded.school_days,version=app.semester_calendars.version+1;
 INSERT INTO app.attendance_foundation_history(entity_type,entity_id,actor_id,before_state,after_state,reason)
  VALUES('calendar',p_semester,p_actor,old_state,academic_write.calendar_state(p_semester),academic_write.trim_evidence(p_reason));
END $function$;

CREATE FUNCTION academic_write.set_calendar_exception(
 p_actor bigint,p_semester bigint,p_version bigint,p_date date,p_school boolean,p_label text,p_reset boolean,p_reason text)
RETURNS void LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE old_state jsonb;
BEGIN
 PERFORM academic_write.lock_write();
 PERFORM academic_write.require_actor(p_actor,'kurikulum');
 PERFORM academic_write.validate_reason(p_reason);
 old_state:=academic_write.calendar_state(p_semester);
 IF old_state IS NULL OR (old_state->>'Version')::bigint IS DISTINCT FROM p_version THEN
  RAISE EXCEPTION 'Versi kalender berubah' USING ERRCODE='40001';
 END IF;
 IF p_version<1 OR p_date IS NULL OR NOT isfinite(p_date) OR p_date NOT BETWEEN (old_state->>'StartsOn')::date AND (old_state->>'EndsOn')::date
    OR p_reset IS NULL OR (NOT p_reset AND (p_label IS NULL OR academic_write.trim_evidence(p_label)='' OR char_length(p_label)>200 OR p_school IS NULL)) THEN
  RAISE EXCEPTION 'Tanggal atau keterangan pengecualian tidak valid' USING ERRCODE='22023';
 END IF;
 IF p_reset THEN
  DELETE FROM app.school_date_exceptions WHERE semester_id=p_semester AND on_date=p_date;
 ELSE
  INSERT INTO app.school_date_exceptions(semester_id,on_date,school_day,label) VALUES(p_semester,p_date,p_school,academic_write.trim_evidence(p_label))
    ON CONFLICT(semester_id,on_date) DO UPDATE SET school_day=excluded.school_day,label=excluded.label;
 END IF;
 UPDATE app.semester_calendars SET version=version+1 WHERE semester_id=p_semester;
 INSERT INTO app.attendance_foundation_history(entity_type,entity_id,actor_id,before_state,after_state,reason)
   VALUES('calendar',p_semester,p_actor,old_state,academic_write.calendar_state(p_semester),academic_write.trim_evidence(p_reason));
END $$;

CREATE FUNCTION academic_write.set_placement_dates(p_actor bigint, p_year bigint, p_class bigint, p_ids bigint[], p_digest text, p_date date, p_reason text, p_batch boolean)
 RETURNS integer
 LANGUAGE plpgsql
 SECURITY DEFINER
 SET search_path TO 'pg_catalog', 'pg_temp'
AS $function$
DECLARE
 st record;
 current_digest text;
 applied integer := 0;
 st_student bigint;
 old_state jsonb;
 n int;
 is_bulk boolean:=p_batch;
 declared_year bigint;
BEGIN
 PERFORM academic_write.lock_write();
 PERFORM academic_write.require_actor(p_actor,'admin');
 PERFORM academic_write.validate_placement_batch(p_year,p_class,p_ids,p_date,p_reason);
 IF p_batch IS NULL OR (NOT p_batch AND cardinality(p_ids)<>1) THEN
  RAISE EXCEPTION 'Pilihan penempatan tidak valid' USING ERRCODE='22023';
 END IF;
 IF p_digest IS NULL OR (p_digest COLLATE "C") !~ '^v2:[a-f0-9]{64}$' THEN
  RAISE EXCEPTION 'Pratinjau penempatan berubah' USING ERRCODE='40001';
 END IF;
 SELECT b.digest INTO current_digest
 FROM academic_write.placement_batch_state(p_actor,p_year,p_class,p_ids,p_date,p_reason,p_batch) b LIMIT 1;
 IF current_digest IS DISTINCT FROM p_digest THEN
  RAISE EXCEPTION 'Pratinjau penempatan berubah' USING ERRCODE='40001';
 END IF;
 PERFORM p.id FROM app.student_placements p
 WHERE p.id=ANY(p_ids) AND p.school_year_id=p_year AND (p_class=0 OR p.class_id=p_class)
 ORDER BY p.id FOR UPDATE;
 FOR st IN SELECT b.* FROM academic_write.placement_batch_state(p_actor,p_year,p_class,p_ids,p_date,p_reason,p_batch) b LOOP
  IF st.digest IS DISTINCT FROM p_digest OR (is_bulk AND st.starts_on<>'') THEN
   RAISE EXCEPTION 'Pratinjau penempatan berubah' USING ERRCODE='40001';
  END IF;
  SELECT p.school_year_id,p.student_id INTO STRICT declared_year,st_student FROM app.student_placements p WHERE p.id=st.id;
  IF NOT st.date_valid THEN RAISE EXCEPTION 'Tanggal tidak sesuai fakta akademik' USING ERRCODE='22023'; END IF;
  IF st.starts_on IS DISTINCT FROM p_date::text AND (
      EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.school_year_id=declared_year AND f.student_id=st_student)
      OR EXISTS(SELECT 1 FROM app.direct_tp_initial_scores s WHERE s.placement_id=st.id)
      OR EXISTS(SELECT 1 FROM (
         SELECT DISTINCT ON(h.semester_id,h.class_id,h.subject_id,h.objective_id) h.objective_id,h.effective_date
         FROM app.direct_tp_applicability_history h WHERE h.placement_id=st.id
         ORDER BY h.semester_id,h.class_id,h.subject_id,h.objective_id,h.version DESC) d
          LEFT JOIN app.historical_learning_objective_insertions h0 ON h0.learning_objective_id=d.objective_id
          WHERE p_date>d.effective_date OR p_date>h0.assessment_date)
  ) THEN
   RAISE EXCEPTION 'Review bukti nilai, keputusan atau Final sebelum mengubah tanggal' USING ERRCODE='23514';
  END IF;
  IF st.starts_on IS DISTINCT FROM p_date::text THEN
   PERFORM academic_write.check_placement_evidence(declared_year,st.id,nullif(st.starts_on,'')::date,p_date);
  END IF;
  SELECT jsonb_build_object('ID',st.id,'YearID',declared_year,'ClassID',0,
    'Version',st.date_version,'StudentName',CASE WHEN is_bulk THEN st.student_name ELSE '' END,
    'ClassName',CASE WHEN is_bulk THEN st.class_name ELSE '' END,'StartsOn',st.starts_on) INTO old_state;
  UPDATE app.student_placements SET starts_on=p_date,date_version=date_version+1
   WHERE id=st.id AND date_version=st.date_version AND (NOT is_bulk OR starts_on IS NULL);
  GET DIAGNOSTICS n=ROW_COUNT;
  IF n<>1 THEN RAISE EXCEPTION 'Versi penempatan berubah' USING ERRCODE='40001'; END IF;
  INSERT INTO app.attendance_foundation_history(entity_type,entity_id,actor_id,before_state,after_state,reason)
   VALUES('placement_date',st.id,p_actor,old_state,old_state||jsonb_build_object('Version',st.date_version+1,'StartsOn',p_date::text),academic_write.trim_evidence(p_reason));
  applied := applied + 1;
 END LOOP;
 IF applied<>cardinality(p_ids) THEN
  RAISE EXCEPTION 'Pratinjau penempatan berubah' USING ERRCODE='40001';
 END IF;
 RETURN applied;
END $function$;

CREATE FUNCTION app.ux02e_s06_lock_placement(p_placement bigint) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
  PERFORM 1 FROM app.student_placements p WHERE p.id=p_placement FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Penempatan tidak tersedia' USING ERRCODE='22023';
  END IF;
END $$;

CREATE FUNCTION app.ux02e_academic_insert_coordinate() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
BEGIN
  IF current_setting('transaction_isolation') <> 'read committed' THEN
    RAISE EXCEPTION 'Mutasi akademik memerlukan READ COMMITTED' USING ERRCODE='25000';
  END IF;
  PERFORM pg_advisory_xact_lock(742009::bigint);
  RETURN NULL;
END $$;
REVOKE EXECUTE ON FUNCTION app.ux02e_academic_insert_coordinate() FROM PUBLIC;
CREATE TRIGGER ux02e_g_before_insert BEFORE INSERT ON app.direct_tp_applicability_history
FOR EACH STATEMENT EXECUTE FUNCTION app.ux02e_academic_insert_coordinate();
CREATE TRIGGER ux02e_g_before_insert BEFORE INSERT ON app.direct_tp_submissions
FOR EACH STATEMENT EXECUTE FUNCTION app.ux02e_academic_insert_coordinate();
CREATE TRIGGER ux02e_g_before_insert BEFORE INSERT ON app.direct_tp_initial_scores
FOR EACH STATEMENT EXECUTE FUNCTION app.ux02e_academic_insert_coordinate();
CREATE TRIGGER ux02e_g_before_insert BEFORE INSERT ON app.direct_tp_score_corrections
FOR EACH STATEMENT EXECUTE FUNCTION app.ux02e_academic_insert_coordinate();

CREATE FUNCTION app.ux02e_tp_score_batch_assignment() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE today date := (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM app.teaching_assignments a
    JOIN app.semesters sm ON sm.id=a.semester_id
    JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id
    JOIN app.subjects sub ON sub.id=a.subject_id AND sub.active
    JOIN app.teachers t ON t.id=a.teacher_id AND t.active
    JOIN app.users u ON u.id=t.user_id AND u.active
    JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel'
    WHERE (sm.school_year_id,a.semester_id,a.class_id,a.subject_id,a.teacher_id,u.id)=
          (NEW.school_year_id,NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.teacher_id,NEW.actor_id)
      AND a.effective_from<=today AND (a.effective_until IS NULL OR a.effective_until>today)
  ) THEN
    RAISE EXCEPTION 'Penugasan skor TP tidak cocok' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
REVOKE EXECUTE ON FUNCTION app.ux02e_tp_score_batch_assignment() FROM PUBLIC;
CREATE TRIGGER aa_ux02e_tp_score_batch_assignment BEFORE INSERT ON app.direct_tp_submissions
FOR EACH ROW EXECUTE FUNCTION app.ux02e_tp_score_batch_assignment();


CREATE FUNCTION app.ux02e_s06_check_insert()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER
 SET search_path TO 'pg_catalog', 'pg_temp'
AS $function$
DECLARE
  today date;
  decision_hash bytea;
  json_reason text;
  current_version bigint;
BEGIN
  IF current_setting('transaction_isolation') <> 'read committed' THEN
    RAISE EXCEPTION 'Keputusan S-06 memerlukan READ COMMITTED' USING ERRCODE='25000';
  END IF;
  IF NEW.version IS NULL OR NEW.version < 1 OR NEW.submission_key IS NULL OR
     NEW.request_sha256 IS NULL OR octet_length(NEW.request_sha256) <> 32 OR
     NEW.evidence_verified IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'Bentuk audit S-06 tidak valid' USING ERRCODE='23514';
  END IF;
  PERFORM pg_advisory_xact_lock(742009::bigint);
  PERFORM app.ux02e_s06_lock_placement(NEW.placement_id);
  PERFORM app.direct_tp_lock_guard(NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.student_id);
  today := (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date;

  IF NOT EXISTS(
    SELECT 1
    FROM app.learning_objectives o
    JOIN app.semesters sm ON sm.id=o.semester_id AND sm.school_year_id=o.school_year_id
    JOIN app.classes c ON c.id=o.class_id AND c.school_year_id=sm.school_year_id
    JOIN app.subjects sub ON sub.id=o.subject_id AND sub.active
    JOIN app.semester_calendars cal ON cal.semester_id=sm.id
    JOIN app.student_placements p ON p.id=NEW.placement_id AND p.school_year_id=sm.school_year_id
    JOIN app.students st ON st.id=p.student_id AND st.active
    WHERE (o.id,o.school_year_id,o.semester_id,o.class_id,o.subject_id,p.student_id)=
          (NEW.objective_id,NEW.school_year_id,NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.student_id)
      AND p.starts_on <= NEW.effective_date
      AND NEW.effective_date BETWEEN cal.starts_on AND cal.ends_on
      AND NEW.effective_date <= today
      AND app.student_placement_class_at(p.id,NEW.effective_date)=NEW.class_id
      AND NOT EXISTS (
        SELECT 1 FROM app.historical_learning_objective_insertions h
        WHERE h.learning_objective_id=o.id
          AND (p.starts_on IS NULL OR p.starts_on>h.assessment_date
               OR app.student_placement_class_at(p.id,h.assessment_date) IS DISTINCT FROM NEW.class_id)
      )
  ) THEN
    RAISE EXCEPTION 'Konteks S-06 atau penempatan tidak valid' USING ERRCODE='23514';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM app.teaching_assignments a
    JOIN app.teachers t ON t.id=a.teacher_id AND t.active
    JOIN app.users u ON u.id=t.user_id AND u.active
    JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel'
    WHERE (a.semester_id,a.class_id,a.subject_id,a.teacher_id,u.id)=
          (NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.teacher_id,NEW.actor_id)
      AND a.effective_from<=today AND (a.effective_until IS NULL OR a.effective_until>today)
  ) THEN
    RAISE EXCEPTION 'Penugasan S-06 tidak valid' USING ERRCODE='23514';
  END IF;
  IF EXISTS(
    SELECT 1 FROM app.semester_final_versions f
    WHERE (f.semester_id,f.class_id,f.subject_id,f.student_id)=
          (NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.student_id)
  ) OR EXISTS(
    SELECT 1 FROM app.direct_tp_initial_scores s
    WHERE (s.semester_id,s.class_id,s.subject_id,s.student_id,s.objective_id)=
          (NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.student_id,NEW.objective_id)
      AND s.owner_teacher_id<>NEW.teacher_id
  ) THEN
    RAISE EXCEPTION 'Final atau kepemilikan nilai mengunci S-06' USING ERRCODE='23514';
  END IF;

  -- Bentuk JSON sama dengan input Go yang sudah menormalisasi UUID ke huruf kecil.
  json_reason := to_json(NEW.reason)::text;
  json_reason := replace(replace(replace(json_reason,'&',E'\\u0026'),'<',E'\\u003c'),'>',E'\\u003e');
  json_reason := replace(replace(json_reason,chr(8232),E'\\u2028'),chr(8233),E'\\u2029');
  decision_hash := sha256(convert_to(format(
    '{"SemesterID":%s,"ClassID":%s,"SubjectID":%s,"StudentID":%s,"ObjectiveID":%s,"ExpectedVersion":%s,"EffectiveDate":"%s","Applicable":%s,"EvidenceVerified":%s,"Reason":%s,"SubmissionKey":"%s"}',
    NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.student_id,NEW.objective_id,NEW.version-1,
    to_char(NEW.effective_date,'YYYY-MM-DD'),NEW.applicable::text,NEW.evidence_verified::text,json_reason,NEW.submission_key
  ),'UTF8'));
  IF NEW.request_sha256 IS DISTINCT FROM decision_hash THEN
    RAISE EXCEPTION 'Hash permintaan S-06 tidak cocok' USING ERRCODE='23514';
  END IF;
  IF EXISTS(
    SELECT 1 FROM app.direct_tp_applicability_history h
    WHERE (h.actor_id,h.submission_key)=(NEW.actor_id,NEW.submission_key)
  ) THEN
    RAISE EXCEPTION 'Submission S-06 sudah tercatat' USING ERRCODE='23505';
  END IF;
  SELECT coalesce(max(h.version),0) INTO current_version FROM app.direct_tp_applicability_history h
  WHERE (h.semester_id,h.class_id,h.subject_id,h.student_id,h.objective_id)=
        (NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.student_id,NEW.objective_id);
  IF NEW.version-1 <> current_version THEN
    RAISE EXCEPTION 'Versi S-06 sudah berubah' USING ERRCODE='40001';
  END IF;
  RETURN NEW;
END $function$;
CREATE FUNCTION app.ux02e_tp_score_evidence()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER
 SET search_path TO 'pg_catalog', 'pg_temp'
AS $function$
DECLARE
  context_year bigint;
  context_semester bigint;
  context_class bigint;
  context_subject bigint;
  objective bigint;
  student bigint;
  placement bigint;
  today date;
  applies boolean;
BEGIN
  IF TG_TABLE_NAME='direct_tp_initial_scores' THEN
    context_year := NEW.school_year_id;
    context_semester := NEW.semester_id;
    context_class := NEW.class_id;
    context_subject := NEW.subject_id;
    objective := NEW.objective_id;
    student := NEW.student_id;
    placement := NEW.placement_id;
  ELSE
    SELECT s.school_year_id,s.semester_id,s.class_id,s.subject_id,s.objective_id,s.student_id,s.placement_id
    INTO STRICT context_year,context_semester,context_class,context_subject,objective,student,placement
    FROM app.direct_tp_initial_scores s WHERE s.id=NEW.initial_score_id;
  END IF;
  PERFORM app.ux02e_s06_lock_placement(placement);
  today := (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date;
  SELECT CASE
    WHEN d.version IS NOT NULL THEN
      d.applicable AND d.placement_id=p.id
      AND p.starts_on<=d.effective_date AND d.effective_date BETWEEN cal.starts_on AND cal.ends_on
      AND d.effective_date<=today
      AND app.student_placement_class_at(p.id,d.effective_date)=context_class
    ELSE
      p.starts_on<=cal.starts_on
      AND app.student_placement_class_at(p.id,cal.starts_on)=context_class
      AND app.student_placement_class_at(p.id,least(cal.ends_on,today))=context_class
      AND NOT EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND m.effective_date BETWEEN cal.starts_on AND cal.ends_on)
      AND NOT EXISTS(SELECT 1 FROM app.student_school_exits x WHERE x.student_id=p.student_id AND x.effective_date BETWEEN cal.starts_on AND cal.ends_on)
      AND NOT EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.effective_date BETWEEN cal.starts_on AND cal.ends_on)

    END
    INTO applies
  FROM app.student_placements p
  JOIN app.students st ON st.id=p.student_id AND st.active
  JOIN app.semesters sm ON sm.school_year_id=p.school_year_id
  JOIN app.semester_calendars cal ON cal.semester_id=sm.id
  JOIN app.learning_objectives o ON (o.school_year_id,o.semester_id)=(p.school_year_id,sm.id)
  JOIN app.direct_tp_codes code ON code.objective_id=o.id
  LEFT JOIN LATERAL (
    SELECT v.placement_id,v.applicable,v.effective_date,v.version FROM app.direct_tp_applicability_history v
    WHERE (v.semester_id,v.class_id,v.subject_id,v.student_id,v.objective_id)=
          (context_semester,context_class,context_subject,student,objective)
    ORDER BY v.version DESC LIMIT 1
  ) d ON true
  WHERE (p.id,p.student_id,p.school_year_id,sm.id,o.class_id,o.subject_id,o.id)=
        (placement,student,context_year,context_semester,context_class,context_subject,objective)
    AND NOT EXISTS (
        SELECT 1 FROM app.historical_learning_objective_insertions h
        WHERE h.learning_objective_id=o.id
          AND (p.starts_on IS NULL OR p.starts_on>h.assessment_date
               OR app.student_placement_class_at(p.id,h.assessment_date) IS DISTINCT FROM context_class)
      );
  IF applies IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'TP atau penempatan belum berlaku untuk skor' USING ERRCODE='23514';
  END IF;
  -- Trigger asli menjalankan pemeriksaan Final, versi koreksi, dan F setelah P.
  RETURN NEW;
END $function$;

CREATE FUNCTION app.ux02e_parse_tp_score(raw text) RETURNS numeric
LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  token text;
  whole text;
  fraction text;
  score numeric;
BEGIN
  -- Unicode White_Space yang juga dipakai strings.TrimSpace di Go.
  token := btrim(raw, U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000');
  IF token IS NULL OR (token COLLATE "C") !~ '^[0-9]+([.,][0-9]{1,2})?$' THEN
    RAISE EXCEPTION 'Format nilai TP tidak sah' USING ERRCODE='22023';
  END IF;
  token := replace(token, ',', '.');
  whole := ltrim(split_part(token, '.', 1), '0');
  fraction := split_part(token, '.', 2);
  IF length(whole)>3 THEN
    RAISE EXCEPTION 'Di luar rentang nilai TP' USING ERRCODE='22023';
  END IF;
  score := (coalesce(nullif(whole,''),'0') || '.' || coalesce(nullif(fraction,''),'0'))::numeric;
  IF score>100 THEN
    RAISE EXCEPTION 'Di luar rentang nilai TP' USING ERRCODE='22023';
  END IF;
  RETURN score;
END $$;

CREATE FUNCTION app.ux02e_write_initial_tp(
    p_year bigint,p_semester bigint,p_class bigint,p_subject bigint,p_objective bigint,
    p_student bigint,p_placement bigint,p_teacher bigint,p_actor bigint,p_raw text,p_batch bigint)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE valid_score numeric := app.ux02e_parse_tp_score(p_raw);
BEGIN
  PERFORM academic_write.lock_write();
  INSERT INTO app.direct_tp_initial_scores(
    school_year_id,semester_id,class_id,subject_id,objective_id,student_id,
    placement_id,owner_teacher_id,recorded_by,score,submission_id)
  VALUES(p_year,p_semester,p_class,p_subject,p_objective,p_student,
         p_placement,p_teacher,p_actor,valid_score,p_batch);
END $$;

CREATE FUNCTION app.ux02e_write_tp_correction(
    p_initial bigint,p_version integer,p_before text,p_after text,p_actor bigint,p_batch bigint)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  valid_before numeric := app.ux02e_parse_tp_score(p_before);
  valid_after numeric := app.ux02e_parse_tp_score(p_after);
BEGIN
  PERFORM academic_write.lock_write();
  INSERT INTO app.direct_tp_score_corrections(
    initial_score_id,version,before_score,after_score,corrected_by,submission_id)
  VALUES(p_initial,p_version,valid_before,valid_after,p_actor,p_batch);
END $$;


-- Guard dan parser tidak dapat dieksekusi langsung oleh runtime atau writer.
REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA academic_write FROM PUBLIC;
GRANT EXECUTE ON FUNCTION academic_write.preview_placement_batch(bigint,bigint,bigint,bigint[],date,text,boolean) TO sim_guru_app;
GRANT EXECUTE ON FUNCTION academic_write.set_placement_dates(bigint,bigint,bigint,bigint[],text,date,text,boolean) TO ux02e_placement_writer;
GRANT EXECUTE ON FUNCTION academic_write.set_calendar(bigint,bigint,bigint,date,date,jsonb,text),
    academic_write.set_calendar_exception(bigint,bigint,bigint,date,boolean,text,boolean,text) TO ux02e_calendar_writer;

-- Hanya role skor/S-06 khusus memerlukan USAGE app dan SELECT kolom berikut
-- untuk pemeriksaan Go yang menyertai transaksi bisnis.
GRANT USAGE ON SCHEMA app TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(id,active) ON app.users,app.students,app.subjects TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(user_id,role_code) ON app.user_roles TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(id,user_id,active) ON app.teachers TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(teacher_id,semester_id,class_id,subject_id,effective_from,effective_until) ON app.teaching_assignments TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(id,school_year_id) ON app.semesters,app.classes TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(semester_id,starts_on,ends_on) ON app.semester_calendars TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(id,school_year_id,student_id,class_id,starts_on) ON app.student_placements TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(id,school_year_id,semester_id,class_id,subject_id) ON app.learning_objectives TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(placement_id,to_class_id,effective_date,id) ON app.student_class_moves TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(from_class_id) ON app.student_class_moves TO ux02e_tp_score_writer;
GRANT SELECT(id,student_id,effective_date) ON app.student_school_exits TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(placement_id,exit_id,target_class_id,effective_date,id,created_placement) ON app.student_school_returns TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(learning_objective_id,assessment_date) ON app.historical_learning_objective_insertions TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(semester_id,class_id,subject_id,student_id) ON app.semester_final_versions TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(semester_id,class_id,subject_id,student_id,objective_id,version)
  ON app.direct_tp_applicability_history TO ux02e_s06_writer,ux02e_tp_score_writer;
GRANT SELECT(applicable,effective_date,placement_id) ON app.direct_tp_applicability_history TO ux02e_tp_score_writer;
GRANT SELECT(actor_id,submission_key,request_sha256) ON app.direct_tp_applicability_history TO ux02e_s06_writer;
GRANT SELECT(objective_id) ON app.direct_tp_codes TO ux02e_tp_score_writer;
GRANT SELECT(id,owner_teacher_id,score,semester_id,class_id,subject_id,student_id,objective_id) ON app.direct_tp_initial_scores TO ux02e_tp_score_writer;
GRANT SELECT(owner_teacher_id,semester_id,class_id,subject_id,student_id,objective_id) ON app.direct_tp_initial_scores TO ux02e_s06_writer;
GRANT SELECT(initial_score_id,after_score,version) ON app.direct_tp_score_corrections TO ux02e_tp_score_writer;
GRANT SELECT(id,actor_id,submission_key,request_sha256) ON app.direct_tp_submissions TO ux02e_tp_score_writer;
GRANT INSERT(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count)
  ON app.direct_tp_submissions TO ux02e_tp_score_writer;
GRANT USAGE ON SEQUENCE app.direct_tp_submissions_id_seq TO ux02e_tp_score_writer;
GRANT INSERT(school_year_id,semester_id,class_id,subject_id,objective_id,student_id,placement_id,teacher_id,actor_id,
    effective_date,applicable,reason,evidence_verified,version,submission_key,request_sha256)
  ON app.direct_tp_applicability_history TO ux02e_s06_writer;

REVOKE EXECUTE ON FUNCTION app.ux02e_s06_lock_placement(bigint),app.ux02e_s06_check_insert(),
  app.ux02e_tp_score_evidence(),app.ux02e_parse_tp_score(text),
  app.ux02e_write_initial_tp(bigint,bigint,bigint,bigint,bigint,bigint,bigint,bigint,bigint,text,bigint),
  app.ux02e_write_tp_correction(bigint,integer,text,text,bigint,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
  app.ux02e_write_initial_tp(bigint,bigint,bigint,bigint,bigint,bigint,bigint,bigint,bigint,text,bigint),
  app.ux02e_write_tp_correction(bigint,integer,text,text,bigint,bigint) TO ux02e_tp_score_writer;

CREATE TRIGGER ux02e_s06_check_insert BEFORE INSERT ON app.direct_tp_applicability_history
 FOR EACH ROW EXECUTE FUNCTION app.ux02e_s06_check_insert();
CREATE TRIGGER aa_ux02e_tp_score_evidence BEFORE INSERT ON app.direct_tp_initial_scores
 FOR EACH ROW EXECUTE FUNCTION app.ux02e_tp_score_evidence();
CREATE TRIGGER aa_ux02e_tp_score_evidence BEFORE INSERT ON app.direct_tp_score_corrections
 FOR EACH ROW EXECUTE FUNCTION app.ux02e_tp_score_evidence();

REVOKE INSERT ON TABLE app.direct_tp_applicability_history,app.direct_tp_submissions,
  app.direct_tp_initial_scores,app.direct_tp_score_corrections FROM PUBLIC,sim_guru_app;
REVOKE UPDATE(id) ON app.direct_tp_initial_scores FROM sim_guru_app;
REVOKE INSERT,UPDATE,DELETE,TRUNCATE ON TABLE app.semester_calendars,app.school_date_exceptions,
  app.attendance_foundation_history FROM PUBLIC,sim_guru_app;
-- Penempatan awal dan readmission masih menggunakan INSERT existing.
REVOKE UPDATE,DELETE,TRUNCATE ON TABLE app.student_placements FROM PUBLIC,sim_guru_app;

CREATE TRIGGER attendance_foundation_append_only BEFORE UPDATE OR DELETE ON app.attendance_foundation_history
 FOR EACH ROW EXECUTE FUNCTION app.direct_tp_reject_mutation();

-- Pencatatan tanggal pada penempatan awal tetap satu transaksi dengan INSERT.
-- Entri ini hanya membawa nilai yang berasal dari baris yang baru dibuat.
CREATE FUNCTION academic_write.record_initial_placement_date(p_placement bigint)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
BEGIN
 INSERT INTO app.attendance_foundation_history(entity_type,entity_id,actor_id,before_state,after_state,reason)
 SELECT 'placement_date',p.id,p.actor_id,'null'::jsonb,
   jsonb_build_object('StartsOn',to_char(p.starts_on,'YYYY-MM-DD'),'Version',p.date_version),
   'Tanggal mulai penempatan awal oleh Admin'
 FROM app.student_placements p
 WHERE p.id=p_placement AND p.starts_on IS NOT NULL AND p.date_version=1
   AND p.xmin::text::bigint = mod(pg_current_xact_id()::text::numeric,4294967296)
   AND EXISTS (SELECT 1 FROM app.users u JOIN app.user_roles r ON r.user_id=u.id
               WHERE u.id=p.actor_id AND u.active AND r.role_code='admin')
   AND NOT EXISTS (SELECT 1 FROM app.attendance_foundation_history a
                   WHERE a.entity_type='placement_date' AND a.entity_id=p.id);
 IF NOT FOUND THEN
  RAISE EXCEPTION 'Audit penempatan awal tidak cocok dengan transaksi' USING ERRCODE='23514';
 END IF;
END $$;
REVOKE EXECUTE ON FUNCTION academic_write.record_initial_placement_date(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION academic_write.record_initial_placement_date(bigint) TO sim_guru_app;

DO $$
BEGIN
 IF EXISTS (
  SELECT 1 FROM unnest(ARRAY['direct_tp_applicability_history','direct_tp_submissions','direct_tp_initial_scores',
                              'direct_tp_score_corrections','semester_calendars','school_date_exceptions',
                              'attendance_foundation_history']) AS r(rel)
   WHERE has_any_column_privilege('sim_guru_app','app.'||r.rel,'INSERT')
 ) OR EXISTS (
  SELECT 1 FROM unnest(ARRAY['student_placements','semester_calendars','school_date_exceptions',
                              'attendance_foundation_history']) AS r(rel)
   WHERE has_any_column_privilege('sim_guru_app','app.'||r.rel,'UPDATE')
      OR has_table_privilege('sim_guru_app','app.'||r.rel,'DELETE,TRUNCATE')
 ) THEN
  RAISE EXCEPTION 'Privilege tulis lama di luar ACL penulis masih efektif';
 END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Kontrak penulis akademik memerlukan prosedur pemulihan eksplisit';
END $$;
-- +goose StatementEnd
