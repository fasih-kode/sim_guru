-- +goose Up
-- +goose StatementBegin
-- Helper privat untuk memeriksa kembali guru pada entry point yang memakai hak owner.
CREATE FUNCTION academic_write.legacy_teacher(p_actor bigint) RETURNS bigint
LANGUAGE plpgsql STABLE SET search_path=pg_catalog,pg_temp AS $$
DECLARE teacher bigint;
BEGIN
  PERFORM academic_write.require_actor(p_actor,'guru_mapel');
  SELECT t.id INTO teacher FROM app.teachers t WHERE t.user_id=p_actor AND t.active;
  IF NOT FOUND THEN RAISE EXCEPTION 'Guru tidak berwenang' USING ERRCODE='42501'; END IF;
  RETURN teacher;
END $$;

CREATE FUNCTION academic_write.legacy_assignment(
  p_teacher bigint,p_semester bigint,p_class bigint,p_subject bigint) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
  SELECT EXISTS(
    SELECT 1 FROM app.teaching_assignments a
    JOIN app.semesters sm ON sm.id=a.semester_id
    JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id
    JOIN app.subjects s ON s.id=a.subject_id AND s.active
    WHERE (a.teacher_id,a.semester_id,a.class_id,a.subject_id)=(p_teacher,p_semester,p_class,p_subject)
      AND a.effective_from<=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date
      AND (a.effective_until IS NULL OR a.effective_until>(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date)
  )
$$;

CREATE FUNCTION academic_write.save_legacy_initial_scores(
  p_actor bigint,p_activity bigint,p_students bigint[],p_objectives bigint[],p_values text[]) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  teacher bigint;
  activity app.assessment_activities%ROWTYPE;
  n_cells integer := coalesce(cardinality(p_students),0);
  matched integer;
  roster jsonb := '{}'::jsonb;
  placement record;
  cell record;
  value numeric;
  previous numeric;
BEGIN
  PERFORM academic_write.lock_write();
  teacher := academic_write.legacy_teacher(p_actor);
  IF n_cells=0 OR cardinality(p_objectives) IS DISTINCT FROM n_cells OR cardinality(p_values) IS DISTINCT FROM n_cells
     OR EXISTS (SELECT 1 FROM unnest(p_students,p_objectives,p_values) c(student,objective,raw)
                WHERE c.student IS NULL OR c.student<1 OR c.objective IS NULL OR c.objective<1 OR c.raw IS NULL)
     OR n_cells<>(SELECT count(DISTINCT (c.student,c.objective)) FROM unnest(p_students,p_objectives) c(student,objective)) THEN
    RAISE EXCEPTION 'Daftar nilai awal tidak valid' USING ERRCODE='22023';
  END IF;

  SELECT a.* INTO activity FROM app.assessment_activities a
  WHERE a.id=p_activity AND a.teacher_id=teacher FOR UPDATE;
  IF NOT FOUND OR NOT academic_write.legacy_assignment(teacher,activity.semester_id,activity.class_id,activity.subject_id) THEN
    RAISE EXCEPTION 'Kegiatan atau penugasan tidak berwenang' USING ERRCODE='42501';
  END IF;
  PERFORM ao.learning_objective_id FROM app.assessment_activity_objectives ao
    WHERE ao.activity_id=p_activity AND ao.learning_objective_id=ANY(p_objectives)
    ORDER BY ao.learning_objective_id FOR SHARE;
  GET DIAGNOSTICS matched=ROW_COUNT;
  IF matched<>(SELECT count(DISTINCT o) FROM unnest(p_objectives) o) THEN
    RAISE EXCEPTION 'TP tidak termasuk kegiatan' USING ERRCODE='42501';
  END IF;
  IF EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=p_activity)
     OR EXISTS (SELECT 1 FROM app.historical_assessment_objective_links h
                WHERE h.activity_id=p_activity AND h.learning_objective_id=ANY(p_objectives)) THEN
    RAISE EXCEPTION 'Nilai untuk relasi TP pada kegiatan historis memerlukan proses pencatatan historis tersendiri.' USING ERRCODE='22023';
  END IF;

  FOR placement IN SELECT p.student_id,p.id FROM app.student_placements p
    WHERE p.school_year_id=activity.school_year_id AND p.student_id=ANY(p_students)
      AND p.starts_on IS NOT NULL AND p.starts_on<=activity.activity_date
      AND app.student_placement_class_at(p.id,activity.activity_date)=activity.class_id
    ORDER BY p.student_id,p.id FOR SHARE
  LOOP
    IF roster ? placement.student_id::text THEN
      RAISE EXCEPTION 'Riwayat penempatan siswa tidak tunggal pada tanggal kegiatan.' USING ERRCODE='22023';
    END IF;
    roster := roster || jsonb_build_object(placement.student_id::text,placement.id);
  END LOOP;
  IF EXISTS (SELECT 1 FROM unnest(p_students) student WHERE NOT (roster ? student::text)) THEN
    RAISE EXCEPTION 'Penempatan siswa tidak sesuai kegiatan' USING ERRCODE='42501';
  END IF;

  FOR cell IN SELECT c.student,c.objective,c.raw FROM unnest(p_students,p_objectives,p_values) c(student,objective,raw)
                ORDER BY c.student,c.objective
  LOOP
    value := app.ux02e_parse_tp_score(cell.raw);
    INSERT INTO app.assessment_activity_initial_scores
      (assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by)
    VALUES(p_activity,cell.objective,activity.school_year_id,activity.semester_id,activity.class_id,activity.subject_id,
      cell.student,(roster->>cell.student::text)::bigint,activity.activity_date,value,p_actor)
    ON CONFLICT (assessment_activity_id,learning_objective_id,student_id) DO NOTHING;
    IF NOT FOUND THEN
      SELECT sc.score INTO previous FROM app.assessment_activity_initial_scores sc
      WHERE (sc.assessment_activity_id,sc.learning_objective_id,sc.student_id)=(p_activity,cell.objective,cell.student)
      FOR SHARE;
      IF NOT FOUND OR previous IS DISTINCT FROM value THEN
        RAISE EXCEPTION 'Nilai awal sudah berbeda' USING ERRCODE='40001';
      END IF;
    END IF;
  END LOOP;
END $$;

CREATE FUNCTION academic_write.correct_legacy_initial_score(
  p_actor bigint,p_score bigint,p_version bigint,p_value text,p_reason text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  teacher bigint;
  initial numeric;
  previous numeric;
  version integer;
BEGIN
  PERFORM academic_write.lock_write();
  teacher := academic_write.legacy_teacher(p_actor);
  PERFORM academic_write.validate_reason(p_reason);
  SELECT sc.score INTO initial FROM app.assessment_activity_initial_scores sc
    JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
    WHERE sc.id=p_score AND sc.recorded_by=p_actor AND a.teacher_id=teacher FOR UPDATE OF sc;
  IF NOT FOUND THEN RAISE EXCEPTION 'Nilai bukan milik guru' USING ERRCODE='42501'; END IF;

  SELECT c.after_score,c.version INTO previous,version FROM app.assessment_activity_score_corrections c
    WHERE c.initial_score_id=p_score ORDER BY c.version DESC LIMIT 1;
  IF NOT FOUND THEN previous:=initial; version:=0; END IF;
  IF version IS DISTINCT FROM p_version THEN
    RAISE EXCEPTION 'Versi koreksi telah berubah' USING ERRCODE='40001';
  END IF;
  INSERT INTO app.assessment_activity_score_corrections(initial_score_id,version,before_score,after_score,reason,corrected_by)
  VALUES(p_score,version+1,previous,app.ux02e_parse_tp_score(p_value),p_reason,p_actor);
END $$;

CREATE FUNCTION academic_write.record_legacy_score_remedial(
  p_actor bigint,p_score bigint,p_key uuid,p_value text,p_date date) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  teacher bigint;
  activity app.assessment_activities%ROWTYPE;
  value numeric;
  previous app.assessment_activity_score_remedials%ROWTYPE;
BEGIN
  PERFORM academic_write.lock_write();
  teacher := academic_write.legacy_teacher(p_actor);
  SELECT a.* INTO activity FROM app.assessment_activity_initial_scores sc
    JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
    WHERE sc.id=p_score AND sc.recorded_by=p_actor AND a.teacher_id=teacher AND a.activity_type='sumatif'
    FOR UPDATE OF sc,a;
  IF NOT FOUND OR NOT academic_write.legacy_assignment(teacher,activity.semester_id,activity.class_id,activity.subject_id) THEN
    RAISE EXCEPTION 'Remedial bukan kewenangan guru' USING ERRCODE='42501';
  END IF;
  IF p_key IS NULL OR p_date IS NULL OR p_date<activity.activity_date OR p_date>(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date THEN
    RAISE EXCEPTION 'Tanggal atau kunci remedial tidak valid' USING ERRCODE='22023';
  END IF;

  value := app.ux02e_parse_tp_score(p_value);
  INSERT INTO app.assessment_activity_score_remedials(initial_score_id,score,remedial_date,recorded_by,submission_key)
  VALUES(p_score,value,p_date,p_actor,p_key)
  ON CONFLICT (recorded_by,submission_key) DO NOTHING;
  IF NOT FOUND THEN
    SELECT m.* INTO previous FROM app.assessment_activity_score_remedials m
      WHERE m.recorded_by=p_actor AND m.submission_key=p_key;
    IF NOT FOUND OR (previous.initial_score_id,previous.score,previous.remedial_date) IS DISTINCT FROM (p_score,value,p_date) THEN
      RAISE EXCEPTION 'Kunci remedial sudah dipakai untuk permintaan lain' USING ERRCODE='40001';
    END IF;
  END IF;
END $$;

CREATE FUNCTION academic_write.lookup_historical_tp_replay(p_actor bigint,p_key uuid)
RETURNS TABLE(learning_objective_id bigint,request_sha256 bytea)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE teacher bigint;
BEGIN
  teacher := academic_write.legacy_teacher(p_actor);
  RETURN QUERY
    SELECT h.learning_objective_id,h.request_sha256 FROM app.historical_learning_objective_insertions h
    WHERE h.actor_id=p_actor AND h.teacher_id=teacher AND h.submission_key=p_key;
END $$;

CREATE FUNCTION academic_write.lock_final_assignment(
  p_actor bigint,p_semester bigint,p_class bigint,p_subject bigint) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE teacher bigint;
BEGIN
  PERFORM academic_write.lock_write();
  teacher := academic_write.legacy_teacher(p_actor);
  PERFORM a.id FROM app.teaching_assignments a
    JOIN app.semesters sm ON sm.id=a.semester_id
    JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id
    JOIN app.subjects s ON s.id=a.subject_id AND s.active
    WHERE (a.teacher_id,a.semester_id,a.class_id,a.subject_id)=(teacher,p_semester,p_class,p_subject)
      AND a.effective_from<=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date
      AND (a.effective_until IS NULL OR a.effective_until>(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date)
    ORDER BY a.id DESC LIMIT 1 FOR SHARE OF a;
  IF NOT FOUND THEN RAISE EXCEPTION 'Penugasan Final tidak aktif' USING ERRCODE='42501'; END IF;
END $$;
-- Snapshot privat untuk token. Hanya identitas dan versi baris yang masuk
-- fingerprint; nama siswa, isi jurnal, dan snapshot Final tidak keluar.
CREATE FUNCTION academic_write.historical_tp_facts(p_semester bigint,p_class bigint,p_subject bigint)
RETURNS jsonb LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
 WITH term AS (SELECT id,school_year_id FROM app.semesters WHERE id=p_semester),
 members AS (
   SELECT DISTINCT p.student_id FROM app.student_placements p JOIN term sm ON sm.school_year_id=p.school_year_id
     WHERE academic_write.class_relevant(p.id,p_class)
   UNION SELECT f.student_id FROM app.semester_final_versions f
     WHERE (f.semester_id,f.class_id,f.subject_id)=(p_semester,p_class,p_subject)
 ),
 placements AS (SELECT p.* FROM app.student_placements p JOIN term sm ON sm.school_year_id=p.school_year_id
                  JOIN members m ON m.student_id=p.student_id),
 versions AS (
   SELECT 'semester' k,s.id::text,s.xmin::text v FROM app.semesters s WHERE s.id=p_semester
   UNION ALL SELECT 'school_year',y.id::text,y.xmin::text FROM app.school_years y JOIN term ON term.school_year_id=y.id
   UNION ALL SELECT 'class',c.id::text,c.xmin::text FROM app.classes c WHERE c.id=p_class
   UNION ALL SELECT 'subject',s.id::text,s.xmin::text FROM app.subjects s WHERE s.id=p_subject
   UNION ALL SELECT 'calendar',c.semester_id::text,c.xmin::text FROM app.semester_calendars c WHERE c.semester_id=p_semester
   UNION ALL SELECT 'weight',w.semester_id::text,w.xmin::text FROM app.semester_assessment_weights w WHERE w.semester_id=p_semester
   UNION ALL SELECT 'assignment',a.id::text,a.xmin::text FROM app.teaching_assignments a
     WHERE (a.semester_id,a.class_id,a.subject_id)=(p_semester,p_class,p_subject)
   UNION ALL SELECT 'student',s.id::text,s.xmin::text FROM app.students s JOIN members m ON m.student_id=s.id
   UNION ALL SELECT 'placement',p.id::text,p.xmin::text FROM app.student_placements p JOIN placements ps ON ps.id=p.id
   UNION ALL SELECT 'move',m.id::text,m.xmin::text FROM app.student_class_moves m JOIN placements p ON p.id=m.placement_id
   UNION ALL SELECT 'exit',e.id::text,e.xmin::text FROM app.student_school_exits e JOIN members m ON m.student_id=e.student_id
   UNION ALL SELECT 'return',r.id::text,r.xmin::text FROM app.student_school_returns r JOIN placements p ON p.id=r.placement_id
   UNION ALL SELECT 'final',f.id::text,f.xmin::text FROM app.semester_final_versions f
     WHERE (f.semester_id,f.class_id,f.subject_id)=(p_semester,p_class,p_subject)
   UNION ALL SELECT 'objective',o.id::text,o.xmin::text FROM app.learning_objectives o
     WHERE (o.semester_id,o.class_id,o.subject_id)=(p_semester,p_class,p_subject)
   UNION ALL SELECT 'provenance',h.id::text,h.xmin::text FROM app.historical_learning_objective_insertions h
     WHERE (h.semester_id,h.class_id,h.subject_id)=(p_semester,p_class,p_subject)
   UNION ALL SELECT 'criterion',c.id::text,c.xmin::text FROM app.tp_achievement_criteria c
     WHERE (c.semester_id,c.class_id,c.subject_id)=(p_semester,p_class,p_subject)
   UNION ALL SELECT 'applicability',h.id::text,h.xmin::text FROM app.direct_tp_applicability_history h
     WHERE (h.semester_id,h.class_id,h.subject_id)=(p_semester,p_class,p_subject)
   UNION ALL SELECT 'tp_score',s.id::text,s.xmin::text FROM app.direct_tp_initial_scores s
     WHERE (s.semester_id,s.class_id,s.subject_id)=(p_semester,p_class,p_subject)
   UNION ALL SELECT 'tp_correction',c.id::text,c.xmin::text FROM app.direct_tp_score_corrections c
     JOIN app.direct_tp_initial_scores s ON s.id=c.initial_score_id
     WHERE (s.semester_id,s.class_id,s.subject_id)=(p_semester,p_class,p_subject)
   UNION ALL SELECT 'exam',s.id::text,s.xmin::text FROM app.exam_initial_scores s JOIN members m ON m.student_id=s.student_id
     WHERE (s.semester_id,s.subject_id)=(p_semester,p_subject)
   UNION ALL SELECT 'exam_correction',c.id::text,c.xmin::text FROM app.exam_score_corrections c
     JOIN app.exam_initial_scores s ON s.id=c.initial_score_id JOIN members m ON m.student_id=s.student_id
     WHERE (s.semester_id,s.subject_id)=(p_semester,p_subject)
   UNION ALL SELECT 'exam_remedial',r.id::text,r.xmin::text FROM app.exam_score_remedials r
     JOIN app.exam_initial_scores s ON s.id=r.initial_score_id JOIN members m ON m.student_id=s.student_id
     WHERE (s.semester_id,s.subject_id)=(p_semester,p_subject)
 )
 SELECT coalesce(jsonb_agg(jsonb_build_array(k,id,v) ORDER BY k COLLATE "C",id),'[]'::jsonb) FROM versions
$$;

-- Metadata otoritatif dari sumber aplikasi. JSON hanya dipakai secara internal.
CREATE FUNCTION academic_write.historical_tp_source(
  p_actor bigint,p_teacher bigint,p_input jsonb,p_learning boolean)
RETURNS jsonb LANGUAGE plpgsql STABLE SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  reference jsonb := p_input->CASE WHEN p_learning THEN 'Learning' ELSE 'Assessment' END;
  source_id text := academic_write.trim_evidence(reference->>'SourceID');
  sem bigint := (p_input->>'SemesterID')::bigint;
  cls bigint := (p_input->>'ClassID')::bigint;
  subject bigint := (p_input->>'SubjectID')::bigint;
  evidence jsonb;
BEGIN
  IF reference->>'Kind' IS DISTINCT FROM 'app' OR source_id IS NULL OR source_id !~ '^[+]?[0-9]+$'
     OR length(source_id)>500 THEN
    RETURN NULL;
  END IF;
  IF source_id::numeric NOT BETWEEN 1 AND 9223372036854775807 THEN RETURN NULL; END IF;
  IF p_learning THEN
    SELECT jsonb_build_object('id',j.id,'date',j.meeting_date,'teacher',j.teacher_snapshot,
        'version',v.version_no,'held',v.state='final' AND v.outcome='held','row',j.xmin::text,'revision',v.xmin::text)
    INTO evidence FROM app.teaching_journals j
    JOIN app.teaching_journal_versions v ON (v.journal_id,v.version_no)=(j.id,j.current_version_no)
    WHERE j.id=source_id::bigint AND j.owner_user_id=p_actor
      AND (j.teacher_id,j.semester_id,j.class_id,j.subject_id)=(p_teacher,sem,cls,subject);
  ELSE
    SELECT jsonb_build_object('id',a.id,'date',a.activity_date,'teacher',t.name,'row',a.xmin::text,'teacher_row',t.xmin::text)
    INTO evidence FROM app.assessment_activities a JOIN app.teachers t ON t.id=a.teacher_id
    WHERE a.id=source_id::bigint AND a.activity_type='sumatif'
      AND (a.teacher_id,a.semester_id,a.class_id,a.subject_id)=(p_teacher,sem,cls,subject);
  END IF;
  RETURN evidence;
END $$;

CREATE FUNCTION academic_write.historical_tp_proof(p_actor bigint,p_input jsonb,p_display_hash bytea)
RETURNS bytea LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  teacher bigint := academic_write.legacy_teacher(p_actor);
  sem bigint := (p_input->>'SemesterID')::bigint;
  cls bigint := (p_input->>'ClassID')::bigint;
  subject bigint := (p_input->>'SubjectID')::bigint;
BEGIN
  IF NOT academic_write.legacy_assignment(teacher,sem,cls,subject)
     OR NOT EXISTS (SELECT 1 FROM app.semester_final_versions f
                    WHERE (f.semester_id,f.class_id,f.subject_id)=(sem,cls,subject)) THEN
    RAISE EXCEPTION 'Konteks TP historis tidak berwenang' USING ERRCODE='42501';
  END IF;
  IF octet_length(p_display_hash) IS DISTINCT FROM 32 OR octet_length(p_input::text)>90000 THEN
    RAISE EXCEPTION 'Pratinjau TP historis tidak valid' USING ERRCODE='22023';
  END IF;
  RETURN academic_write_crypto.hmac(convert_to(jsonb_build_array(
      'historical-tp-proof-v1',p_actor,teacher,(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date,
      p_input,encode(p_display_hash,'hex'),academic_write.historical_tp_facts(sem,cls,subject),
      academic_write.historical_tp_source(p_actor,teacher,p_input,true),
      academic_write.historical_tp_source(p_actor,teacher,p_input,false)
    )::text,'UTF8'),(SELECT k.secret FROM academic_write.placement_digest_key k WHERE k.singleton),'sha256');
END $$;

CREATE FUNCTION academic_write.historical_tp_metadata(p_actor bigint,p_input jsonb,p_learning boolean)
RETURNS TABLE(event_date text,teacher_name text,fingerprint text)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  teacher bigint := academic_write.legacy_teacher(p_actor);
  meta jsonb;
BEGIN
  IF NOT academic_write.legacy_assignment(teacher,(p_input->>'SemesterID')::bigint,
      (p_input->>'ClassID')::bigint,(p_input->>'SubjectID')::bigint) THEN
    RAISE EXCEPTION 'Konteks sumber TP tidak berwenang' USING ERRCODE='42501';
  END IF;
  meta := academic_write.historical_tp_source(p_actor,teacher,p_input,p_learning);
  IF meta IS NOT NULL THEN
    RETURN QUERY SELECT meta->>'date',meta->>'teacher',encode(sha256(convert_to(meta::text,'UTF8')),'hex');
  END IF;
END $$;

CREATE FUNCTION academic_write.confirm_historical_tp(p_actor bigint,p_payload bytea,p_display_hash bytea)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  request jsonb;
  input jsonb;
  verification jsonb;
  teacher bigint;
  sem bigint;
  cls bigint;
  subject bigint;
  school_year bigint;
  objective bigint;
  submission uuid;
  request_hash bytea;
  old_hash bytea;
  proof bytea;
  evidence_day date;
  today date := (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date;
  ref jsonb;
  metadata jsonb;
  learning_metadata jsonb;
  assessment_metadata jsonb;
  field text;
  is_learning boolean;
BEGIN
  PERFORM academic_write.lock_write();
  teacher := academic_write.legacy_teacher(p_actor);
  IF p_payload IS NULL OR octet_length(p_payload)>110000 THEN
    RAISE EXCEPTION 'Permintaan TP historis tidak valid' USING ERRCODE='22023';
  END IF;
  request := convert_from(p_payload,'UTF8')::jsonb;
  input := request->'Preview';
  verification := request->'Verification';
  sem := (input->>'SemesterID')::bigint;
  cls := (input->>'ClassID')::bigint;
  subject := (input->>'SubjectID')::bigint;
  submission := (request->>'SubmissionKey')::uuid;
  proof := decode(request->>'SnapshotToken','hex');
  IF submission IS NULL OR octet_length(proof) IS DISTINCT FROM 32
     OR NOT academic_write.legacy_assignment(teacher,sem,cls,subject) THEN
    RAISE EXCEPTION 'Permintaan atau penugasan TP historis tidak valid' USING ERRCODE='42501';
  END IF;

  request_hash := sha256(p_payload);
  SELECT h.learning_objective_id,h.request_sha256 INTO objective,old_hash
    FROM app.historical_learning_objective_insertions h
    WHERE h.actor_id=p_actor AND h.submission_key=submission;
  IF FOUND THEN
    IF old_hash IS DISTINCT FROM request_hash THEN
      RAISE EXCEPTION 'Permintaan TP historis sudah berbeda' USING ERRCODE='40001';
    END IF;
    RETURN objective;
  END IF;
  IF academic_write.historical_tp_proof(p_actor,input,p_display_hash) IS DISTINCT FROM proof THEN
    RAISE EXCEPTION 'Pratinjau TP historis telah berubah' USING ERRCODE='40001';
  END IF;

  PERFORM academic_write.validate_reason(input->>'Reason');
  PERFORM academic_write.validate_reason(input->>'DateSource');
  IF jsonb_typeof(input->'Definition') IS DISTINCT FROM 'string'
     OR jsonb_typeof(input->'Reason') IS DISTINCT FROM 'string'
     OR jsonb_typeof(input->'DateSource') IS DISTINCT FROM 'string'
     OR coalesce(char_length(academic_write.trim_evidence(input->>'Definition')),0) NOT BETWEEN 1 AND 4000
     OR verification->'Acknowledged' IS DISTINCT FROM 'true'::jsonb
     OR verification - ARRAY['LearningSection','AssessmentSection','Method','ResultSource','Acknowledged'] IS DISTINCT FROM '{}'::jsonb THEN
    RAISE EXCEPTION 'Uraian atau verifikasi TP historis tidak lengkap' USING ERRCODE='22023';
  END IF;
  FOREACH field IN ARRAY ARRAY['LearningSection','AssessmentSection','Method','ResultSource'] LOOP
    IF jsonb_typeof(verification->field) IS DISTINCT FROM 'string' THEN
      RAISE EXCEPTION 'Verifikasi TP historis tidak lengkap' USING ERRCODE='22023';
    END IF;
    PERFORM academic_write.validate_reason(verification->>field);
  END LOOP;
  IF (input->>'AssessmentDate') IS NULL OR (input->>'AssessmentDate') !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'
     OR (input->>'AssessmentDate')::date>today THEN
    RAISE EXCEPTION 'Tanggal penilaian historis tidak valid' USING ERRCODE='22023';
  END IF;

  FOREACH is_learning IN ARRAY ARRAY[true,false] LOOP
    ref := CASE WHEN is_learning THEN input->'Learning' ELSE input->'Assessment' END;
    IF jsonb_typeof(ref) IS DISTINCT FROM 'object'
       OR ref - ARRAY['Kind','SourceID','Document','Archive','Teacher','ResultSource','Date'] IS DISTINCT FROM '{}'::jsonb THEN
      RAISE EXCEPTION 'Rujukan TP historis tidak valid' USING ERRCODE='22023';
    END IF;
    FOREACH field IN ARRAY ARRAY['Kind','SourceID','Document','Archive','Teacher','ResultSource','Date'] LOOP
      IF jsonb_typeof(ref->field) IS DISTINCT FROM 'string' OR char_length(ref->>field)>500 THEN
        RAISE EXCEPTION 'Rujukan TP historis tidak valid' USING ERRCODE='22023';
      END IF;
    END LOOP;
    IF ref->>'Date'<>'' AND ((ref->>'Date') !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR (ref->>'Date')::date>today) THEN
      RAISE EXCEPTION 'Tanggal bukti historis tidak valid' USING ERRCODE='22023';
    END IF;

    IF ref->>'Kind'='manual' THEN
      PERFORM academic_write.validate_reason(ref->>'Document');
      PERFORM academic_write.validate_reason(ref->>'Archive');
      PERFORM academic_write.validate_reason(ref->>'Teacher');
      IF is_learning AND ref->>'Date'='' THEN
        RAISE EXCEPTION 'Tanggal bukti pembelajaran wajib tersedia' USING ERRCODE='22023';
      ELSIF NOT is_learning THEN
        PERFORM academic_write.validate_reason(ref->>'ResultSource');
      END IF;
      metadata := jsonb_build_object('verification','declared_manual');
    ELSIF ref->>'Kind'='app' THEN
      metadata := academic_write.historical_tp_source(p_actor,teacher,input,is_learning);
      IF metadata IS NULL THEN
        RAISE EXCEPTION 'Bukti aplikasi berada di luar konteks' USING ERRCODE='42501';
      END IF;
      evidence_day := (metadata->>'date')::date;
      IF evidence_day>today OR (is_learning AND metadata->'held' IS DISTINCT FROM 'true'::jsonb)
         OR (is_learning AND ref->>'Date'<>'' AND (ref->>'Date')::date<>evidence_day)
         OR (NOT is_learning AND (input->>'AssessmentDate')::date<>evidence_day)
         OR (academic_write.trim_evidence(ref->>'Teacher')<>'' AND
             lower(academic_write.trim_evidence(ref->>'Teacher'))<>lower(academic_write.trim_evidence(metadata->>'teacher'))) THEN
        RAISE EXCEPTION 'Metadata bukti aplikasi tidak sesuai' USING ERRCODE='22023';
      END IF;
      metadata := metadata - 'row' - 'teacher_row' - 'revision';
    ELSE
      RAISE EXCEPTION 'Jenis bukti TP historis tidak valid' USING ERRCODE='22023';
    END IF;
    IF is_learning THEN
      learning_metadata := jsonb_build_object('reference',ref,'metadata',metadata);
    ELSE
      assessment_metadata := jsonb_build_object('reference',ref,'metadata',metadata,
        'date_source',academic_write.trim_evidence(input->>'DateSource'));
    END IF;
  END LOOP;

  SELECT sm.school_year_id INTO STRICT school_year FROM app.semesters sm WHERE sm.id=sem;
  objective := nextval('app.learning_objectives_id_seq'::regclass);
  INSERT INTO app.historical_learning_objective_insertions
    (learning_objective_id,semester_id,class_id,subject_id,actor_id,teacher_id,assessment_date,reason,
     learning_evidence,assessment_evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key)
    VALUES(objective,sem,cls,subject,p_actor,teacher,(input->>'AssessmentDate')::date,
      academic_write.trim_evidence(input->>'Reason'),learning_metadata,assessment_metadata,verification,
      jsonb_build_object('format','authoritative-context-v1','semester_id',sem,'class_id',cls,'subject_id',subject,
        'baseline_final_versions',(SELECT coalesce(jsonb_agg(jsonb_build_array(f.id,f.version) ORDER BY f.id),'[]'::jsonb)
          FROM app.semester_final_versions f WHERE (f.semester_id,f.class_id,f.subject_id)=(sem,cls,subject))),
      proof,request_hash,submission);
  INSERT INTO app.learning_objectives(id,school_year_id,semester_id,class_id,subject_id,definition,created_by)
    OVERRIDING SYSTEM VALUE VALUES(objective,school_year,sem,cls,subject,academic_write.trim_evidence(input->>'Definition'),p_actor);
  RETURN objective;
END $$;

CREATE FUNCTION academic_write.historical_tp_result(p_actor bigint,p_objective bigint)
RETURNS TABLE(definition text,audit_id bigint)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE teacher bigint := academic_write.legacy_teacher(p_actor);
BEGIN
  RETURN QUERY SELECT o.definition::text,h.id FROM app.historical_learning_objective_insertions h
    JOIN app.learning_objectives o ON o.id=h.learning_objective_id
    WHERE h.learning_objective_id=p_objective AND h.actor_id=p_actor AND h.teacher_id=teacher
      AND academic_write.legacy_assignment(teacher,h.semester_id,h.class_id,h.subject_id);
END $$;
-- Go menetapkan keputusan akademik. SQL memeriksa identitas, integritas
-- snapshot dan invariansi penyimpanan dalam transaksi yang sama.
CREATE FUNCTION academic_write.checked_final_snapshot(
  p_year bigint,p_semester bigint,p_class bigint,p_subject bigint,p_student bigint,
  p_snapshot bytea,p_hash bytea) RETURNS jsonb
LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $$
DECLARE snapshot jsonb;
BEGIN
  -- Hash dibuat terhadap byte JSON dari Go, sebelum konversi ke JSONB.
  IF p_snapshot IS NULL OR octet_length(p_hash) IS DISTINCT FROM 32 OR sha256(p_snapshot) IS DISTINCT FROM p_hash THEN
    RAISE EXCEPTION 'Byte atau hash snapshot Final tidak cocok' USING ERRCODE='22023';
  END IF;
  snapshot := convert_from(p_snapshot,'UTF8')::jsonb;
  IF NOT snapshot @> jsonb_build_object(
    'SemesterID',p_semester,'ClassID',p_class,'SubjectID',p_subject,
    'Student',jsonb_build_object('ID',p_student::text),
    'Evidence',jsonb_build_object(
      'semester',jsonb_build_object('id',p_semester,'school_year_id',p_year),
      'class',jsonb_build_object('id',p_class,'school_year_id',p_year),
      'subject',jsonb_build_object('id',p_subject),
      'student',jsonb_build_object('id',p_student))) THEN
    RAISE EXCEPTION 'Konteks snapshot Final tidak cocok' USING ERRCODE='22023';
  END IF;
  RETURN snapshot;
END $$;

CREATE FUNCTION academic_write.append_final_version(
  p_actor bigint,p_semester bigint,p_class bigint,p_subject bigint,p_student bigint,
  p_expected bigint,p_type text,p_recommendation integer,p_score integer,p_reason text,
  p_snapshot bytea,p_hash bytea,p_key uuid,p_request_hash bytea) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  teacher bigint;
  school_year bigint;
  snapshot jsonb;
  latest app.semester_final_versions%ROWTYPE;
  previous app.semester_final_versions%ROWTYPE;
  result_id bigint;
BEGIN
  PERFORM academic_write.lock_write();
  teacher := academic_write.legacy_teacher(p_actor);
  IF p_expected IS NULL OR p_expected NOT BETWEEN 0 AND 2147483646 OR p_key IS NULL
     OR octet_length(p_request_hash) IS DISTINCT FROM 32
     OR p_type IS NULL OR p_type NOT IN ('normal','exception')
     OR p_score IS NULL OR p_score NOT BETWEEN 0 AND 100 OR p_reason IS NULL OR char_length(p_reason)>500
     OR (p_type='normal' AND (p_recommendation IS NULL OR p_recommendation NOT BETWEEN 0 AND 100))
     OR (p_type='exception' AND p_recommendation IS NOT NULL) THEN
    RAISE EXCEPTION 'Parameter penulisan Final tidak valid' USING ERRCODE='22023';
  END IF;
  IF p_expected>0 OR p_type='exception' OR p_score<>p_recommendation THEN
    PERFORM academic_write.validate_reason(p_reason);
  END IF;
  SELECT sm.school_year_id INTO school_year
    FROM app.semesters sm
    JOIN app.classes c ON c.id=p_class AND c.school_year_id=sm.school_year_id
    JOIN app.subjects s ON s.id=p_subject
    JOIN app.students st ON st.id=p_student
    WHERE sm.id=p_semester;
  IF NOT FOUND THEN RAISE EXCEPTION 'Konteks Final tidak berwenang' USING ERRCODE='42501'; END IF;

  snapshot := academic_write.checked_final_snapshot(school_year,p_semester,p_class,p_subject,p_student,p_snapshot,p_hash);
  IF (p_type='normal' AND (snapshot#>>'{Student,Status}' IS DISTINCT FROM 'Siap Final'
       OR snapshot#>>'{Student,Recommendation}' IS DISTINCT FROM p_recommendation::text))
     OR (p_type='exception' AND (snapshot#>>'{Student,Status}' IS DISTINCT FROM 'Belum Lengkap'
       OR snapshot#>>'{Student,Recommendation}' IS DISTINCT FROM '')) THEN
    RAISE EXCEPTION 'Jenis Final tidak sesuai keputusan pada snapshot' USING ERRCODE='22023';
  END IF;

  SELECT f.* INTO latest FROM app.semester_final_versions f
    WHERE (f.semester_id,f.class_id,f.subject_id,f.student_id)=(p_semester,p_class,p_subject,p_student)
    ORDER BY f.version DESC LIMIT 1 FOR UPDATE;
  IF p_expected=0 THEN
    PERFORM academic_write.lock_final_assignment(p_actor,p_semester,p_class,p_subject);
    IF NOT EXISTS (
      SELECT 1 FROM app.student_placements p
      JOIN app.semester_calendars cal ON cal.semester_id=p_semester
      WHERE p.school_year_id=school_year AND p.student_id=p_student
        AND p.starts_on<=cal.ends_on AND (
          app.student_placement_class_at(p.id,greatest(p.starts_on,cal.starts_on))=p_class
          OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id
                    AND m.to_class_id=p_class AND m.effective_date BETWEEN cal.starts_on AND cal.ends_on)
          OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id
                    AND r.target_class_id=p_class AND r.effective_date BETWEEN cal.starts_on AND cal.ends_on))) THEN
      RAISE EXCEPTION 'Penempatan siswa tidak sesuai konteks Final' USING ERRCODE='42501';
    END IF;
  ELSIF latest.id IS NULL OR latest.teacher_id<>teacher OR latest.school_year_id<>school_year THEN
    -- Peninjauan versi lama mengikuti hak pemilik historis yang masih aktif.
    RAISE EXCEPTION 'Final historis bukan milik guru' USING ERRCODE='42501';
  END IF;

  SELECT f.* INTO previous FROM app.semester_final_versions f WHERE f.recorded_by=p_actor AND f.submission_key=p_key;
  IF FOUND THEN
    IF (previous.semester_id,previous.class_id,previous.subject_id,previous.student_id,previous.teacher_id,previous.version,
        previous.final_type,previous.recommendation_score,previous.final_score,previous.reason,previous.snapshot_sha256,previous.request_sha256)
        IS DISTINCT FROM (p_semester,p_class,p_subject,p_student,teacher,p_expected+1,
          p_type,p_recommendation,p_score,p_reason,p_hash,p_request_hash) THEN
      RAISE EXCEPTION 'Kunci Final telah dipakai untuk keputusan lain' USING ERRCODE='40001';
    END IF;
    RETURN previous.id;
  END IF;
  IF coalesce(latest.version,0)<>p_expected
     OR EXISTS(SELECT 1 FROM app.semester_final_retentions r WHERE r.recorded_by=p_actor AND r.submission_key=p_key)
     OR EXISTS(SELECT 1 FROM app.semester_final_batches b WHERE b.recorded_by=p_actor AND b.submission_key=p_key) THEN
    RAISE EXCEPTION 'Versi atau kunci Final telah berubah' USING ERRCODE='40001';
  END IF;

  INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,
    version,final_type,recommendation_score,final_score,reason,snapshot,snapshot_sha256,submission_key,request_sha256)
    VALUES(school_year,p_semester,p_class,p_subject,p_student,teacher,p_actor,p_expected+1,
      p_type,p_recommendation,p_score,p_reason,snapshot,p_hash,p_key,p_request_hash)
    RETURNING id INTO result_id;
  RETURN result_id;
END $$;

CREATE FUNCTION academic_write.append_final_retention(
  p_actor bigint,p_semester bigint,p_class bigint,p_subject bigint,p_student bigint,p_final bigint,
  p_expected bigint,p_score integer,p_reason text,p_snapshot bytea,p_hash bytea,p_key uuid,p_request_hash bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  teacher bigint;
  school_year bigint;
  latest app.semester_final_versions%ROWTYPE;
  target app.semester_final_versions%ROWTYPE;
  previous app.semester_final_retentions%ROWTYPE;
  snapshot jsonb;
BEGIN
  PERFORM academic_write.lock_write();
  teacher := academic_write.legacy_teacher(p_actor);
  PERFORM academic_write.validate_reason(p_reason);
  IF p_expected IS NULL OR p_expected<1 OR p_key IS NULL OR p_score IS NULL OR p_score NOT BETWEEN 0 AND 100
     OR octet_length(p_request_hash) IS DISTINCT FROM 32 THEN
    RAISE EXCEPTION 'Parameter retensi Final tidak valid' USING ERRCODE='22023';
  END IF;
  SELECT f.* INTO latest FROM app.semester_final_versions f
    WHERE (f.semester_id,f.class_id,f.subject_id,f.student_id)=(p_semester,p_class,p_subject,p_student)
    ORDER BY f.version DESC LIMIT 1 FOR UPDATE;
  SELECT sm.school_year_id INTO school_year FROM app.semesters sm
    JOIN app.classes c ON c.id=p_class AND c.school_year_id=sm.school_year_id WHERE sm.id=p_semester;
  IF school_year IS NULL OR latest.id IS NULL OR latest.teacher_id<>teacher OR latest.school_year_id<>school_year THEN
    RAISE EXCEPTION 'Final historis bukan milik guru pada konteks ini' USING ERRCODE='42501';
  END IF;
  SELECT f.* INTO target FROM app.semester_final_versions f WHERE f.id=p_final
    AND (f.school_year_id,f.semester_id,f.class_id,f.subject_id,f.student_id,f.teacher_id)=(school_year,p_semester,p_class,p_subject,p_student,teacher);
  IF NOT FOUND THEN RAISE EXCEPTION 'Versi Final retensi tidak sesuai konteks' USING ERRCODE='42501'; END IF;
  IF (target.version,target.final_score) IS DISTINCT FROM (p_expected,p_score) THEN
    RAISE EXCEPTION 'Versi atau nilai yang akan dipertahankan tidak sesuai' USING ERRCODE='40001';
  END IF;

  snapshot := academic_write.checked_final_snapshot(school_year,p_semester,p_class,p_subject,p_student,p_snapshot,p_hash);
  SELECT r.* INTO previous FROM app.semester_final_retentions r WHERE r.recorded_by=p_actor AND r.submission_key=p_key;
  IF FOUND THEN
    IF (previous.final_version_id,previous.reason,previous.evidence_sha256,previous.request_sha256)
       IS DISTINCT FROM (p_final,p_reason,p_hash,p_request_hash) THEN
      RAISE EXCEPTION 'Kunci retensi telah dipakai untuk keputusan lain' USING ERRCODE='40001';
    END IF;
    RETURN;
  END IF;
  IF latest.id<>p_final
     OR EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.recorded_by=p_actor AND f.submission_key=p_key)
     OR EXISTS(SELECT 1 FROM app.semester_final_batches b WHERE b.recorded_by=p_actor AND b.submission_key=p_key) THEN
    RAISE EXCEPTION 'Versi atau kunci retensi telah berubah' USING ERRCODE='40001';
  END IF;

  INSERT INTO app.semester_final_retentions(final_version_id,recorded_by,reason,evidence_snapshot,evidence_sha256,submission_key,request_sha256)
    VALUES(p_final,p_actor,p_reason,snapshot,p_hash,p_key,p_request_hash);
END $$;

CREATE FUNCTION academic_write.final_batch_actor(p_actor bigint,p_key uuid)
RETURNS TABLE(teacher_name text,request_sha256 bytea)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE teacher bigint := academic_write.legacy_teacher(p_actor);
BEGIN
  RETURN QUERY SELECT t.name::text,b.request_sha256 FROM app.teachers t
    LEFT JOIN app.semester_final_batches b ON b.recorded_by=p_actor AND b.submission_key=p_key
    WHERE t.id=teacher;
END $$;

CREATE FUNCTION academic_write.append_final_batch(
  p_actor bigint,p_semester bigint,p_class bigint,p_subject bigint,p_key uuid,p_request_hash bytea,p_items jsonb)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
  teacher bigint;
  batch bigint;
  previous app.semester_final_batches%ROWTYPE;
  item record;
  final_id bigint;
BEGIN
  PERFORM academic_write.lock_write();
  teacher := academic_write.legacy_teacher(p_actor);
  PERFORM academic_write.lock_final_assignment(p_actor,p_semester,p_class,p_subject);
  IF p_key IS NULL OR octet_length(p_request_hash) IS DISTINCT FROM 32
     OR jsonb_typeof(p_items) IS DISTINCT FROM 'array' THEN
    RAISE EXCEPTION 'Parameter batch Final tidak valid' USING ERRCODE='22023';
  END IF;
  IF jsonb_array_length(p_items) NOT BETWEEN 1 AND 500 OR jsonb_array_length(p_items)<>
   (SELECT count(DISTINCT x.student) FROM jsonb_to_recordset(p_items) x(student bigint)) THEN
    RAISE EXCEPTION 'Daftar siswa batch Final tidak valid' USING ERRCODE='22023';
  END IF;
  SELECT b.* INTO previous FROM app.semester_final_batches b WHERE b.recorded_by=p_actor AND b.submission_key=p_key;
  IF FOUND THEN
    IF (previous.teacher_id,previous.semester_id,previous.class_id,previous.subject_id,previous.request_sha256)
      IS DISTINCT FROM (teacher,p_semester,p_class,p_subject,p_request_hash)
      OR (SELECT count(*) FROM app.semester_final_batch_items i WHERE i.batch_id=previous.id)<>jsonb_array_length(p_items)
      OR EXISTS (
        SELECT 1 FROM jsonb_to_recordset(p_items)
          x(student bigint,score integer,snapshot text,snapshot_hash text,submission uuid,request_hash text)
        WHERE NOT EXISTS (
          SELECT 1 FROM app.semester_final_batch_items i JOIN app.semester_final_versions f ON f.id=i.final_version_id
          WHERE i.batch_id=previous.id AND i.student_id=x.student AND f.final_score=x.score
            AND f.submission_key=x.submission AND f.request_sha256=decode(x.request_hash,'base64')
            AND f.snapshot_sha256=decode(x.snapshot_hash,'base64')
            AND f.snapshot_sha256=sha256(decode(x.snapshot,'base64')))) THEN
      RAISE EXCEPTION 'Kunci batch Final telah dipakai untuk keputusan lain' USING ERRCODE='40001';
    END IF;
    RETURN;
  END IF;
  IF EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.recorded_by=p_actor AND f.submission_key=p_key)
     OR EXISTS(SELECT 1 FROM app.semester_final_retentions r WHERE r.recorded_by=p_actor AND r.submission_key=p_key) THEN
    RAISE EXCEPTION 'Kunci batch Final telah dipakai untuk keputusan lain' USING ERRCODE='40001';
  END IF;

  INSERT INTO app.semester_final_batches(semester_id,class_id,subject_id,teacher_id,recorded_by,submission_key,request_sha256)
    VALUES(p_semester,p_class,p_subject,teacher,p_actor,p_key,p_request_hash) RETURNING id INTO batch;
  FOR item IN SELECT x.* FROM jsonb_to_recordset(p_items)
    x(student bigint,score integer,snapshot text,snapshot_hash text,submission uuid,request_hash text)
    ORDER BY x.student
  LOOP
    IF EXISTS(SELECT 1 FROM app.semester_final_versions f
      WHERE (f.semester_id,f.class_id,f.subject_id,f.student_id)=(p_semester,p_class,p_subject,item.student)) THEN
      RAISE EXCEPTION 'Siswa dalam batch sudah memiliki Final' USING ERRCODE='40001';
    END IF;
    final_id := academic_write.append_final_version(p_actor,p_semester,p_class,p_subject,item.student,0,'normal',item.score,
      item.score,'',decode(item.snapshot,'base64'),decode(item.snapshot_hash,'base64'),item.submission,decode(item.request_hash,'base64'));
    INSERT INTO app.semester_final_batch_items(batch_id,student_id,final_version_id) VALUES(batch,item.student,final_id);
  END LOOP;
END $$;
-- +goose StatementEnd

REVOKE EXECUTE ON FUNCTION academic_write.legacy_teacher(bigint),
  academic_write.legacy_assignment(bigint,bigint,bigint,bigint),
  academic_write.save_legacy_initial_scores(bigint,bigint,bigint[],bigint[],text[]),
  academic_write.correct_legacy_initial_score(bigint,bigint,bigint,text,text),
  academic_write.record_legacy_score_remedial(bigint,bigint,uuid,text,date),
  academic_write.lookup_historical_tp_replay(bigint,uuid),
  academic_write.lock_final_assignment(bigint,bigint,bigint,bigint),
  academic_write.historical_tp_facts(bigint,bigint,bigint),
  academic_write.historical_tp_source(bigint,bigint,jsonb,boolean),
  academic_write.historical_tp_proof(bigint,jsonb,bytea),
  academic_write.historical_tp_metadata(bigint,jsonb,boolean),
  academic_write.confirm_historical_tp(bigint,bytea,bytea),
  academic_write.historical_tp_result(bigint,bigint),
  academic_write.checked_final_snapshot(bigint,bigint,bigint,bigint,bigint,bytea,bytea),
  academic_write.append_final_version(bigint,bigint,bigint,bigint,bigint,bigint,text,integer,integer,text,bytea,bytea,uuid,bytea),
  academic_write.append_final_retention(bigint,bigint,bigint,bigint,bigint,bigint,bigint,integer,text,bytea,bytea,uuid,bytea),
  academic_write.final_batch_actor(bigint,uuid),
  academic_write.append_final_batch(bigint,bigint,bigint,bigint,uuid,bytea,jsonb) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION academic_write.save_legacy_initial_scores(bigint,bigint,bigint[],bigint[],text[]),
  academic_write.correct_legacy_initial_score(bigint,bigint,bigint,text,text),
  academic_write.record_legacy_score_remedial(bigint,bigint,uuid,text,date),
  academic_write.lookup_historical_tp_replay(bigint,uuid),
  academic_write.lock_final_assignment(bigint,bigint,bigint,bigint),
  academic_write.historical_tp_proof(bigint,jsonb,bytea),
  academic_write.historical_tp_metadata(bigint,jsonb,boolean),
  academic_write.confirm_historical_tp(bigint,bytea,bytea),
  academic_write.historical_tp_result(bigint,bigint),
  academic_write.append_final_version(bigint,bigint,bigint,bigint,bigint,bigint,text,integer,integer,text,bytea,bytea,uuid,bytea),
  academic_write.append_final_retention(bigint,bigint,bigint,bigint,bigint,bigint,bigint,integer,text,bytea,bytea,uuid,bytea),
  academic_write.final_batch_actor(bigint,uuid),
  academic_write.append_final_batch(bigint,bigint,bigint,bigint,uuid,bytea,jsonb) TO sim_guru_app;

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Perubahan ACL penulis akademik memerlukan pemulihan eksplisit'; END $$;
