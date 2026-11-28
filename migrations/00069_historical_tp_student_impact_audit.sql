-- Konfirmasi menyimpan dampak minimum dari seluruh siswa yang dievaluasi.
-- Identitas TP, tanggal, bukti, dan fingerprint tetap pada kolom audit yang ada.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION academic_write.confirm_historical_tp(p_actor bigint,p_payload bytea,p_display_hash bytea,p_impact bytea)
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
  impact jsonb;
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
  IF octet_length(p_display_hash) IS DISTINCT FROM 32
     OR p_impact IS NULL OR octet_length(p_impact)>32000 THEN
    RAISE EXCEPTION 'Data dampak TP historis tidak valid' USING ERRCODE='22023';
  END IF;
  IF academic_write.historical_tp_proof(p_actor,input,sha256(p_display_hash||sha256(p_impact))) IS DISTINCT FROM proof THEN
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
  -- Go menetapkan relevansi. PostgreSQL memeriksa bentuk, cakupan, dan versi Final.
  impact := convert_from(p_impact,'UTF8')::jsonb;
  IF jsonb_typeof(impact) IS DISTINCT FROM 'array' THEN
    RAISE EXCEPTION 'Daftar dampak TP historis tidak valid' USING ERRCODE='22023';
  END IF;
  IF jsonb_array_length(impact) NOT BETWEEN 1 AND 250 OR EXISTS (
    SELECT 1 FROM jsonb_array_elements(impact) e(item)
    WHERE jsonb_typeof(item) IS DISTINCT FROM 'object'
       OR item - ARRAY['student_id','relevance','final_id','final_version'] IS DISTINCT FROM '{}'::jsonb
       OR jsonb_typeof(item->'student_id') IS DISTINCT FROM 'number'
       OR coalesce(item->>'relevance','') NOT IN ('candidate_member','not_member','unverified')
       OR (item ? 'final_id') IS DISTINCT FROM (item ? 'final_version')
       OR (item ? 'final_id' AND (
            jsonb_typeof(item->'final_id') IS DISTINCT FROM 'number'
            OR jsonb_typeof(item->'final_version') IS DISTINCT FROM 'number'))
  ) THEN
    RAISE EXCEPTION 'Rincian dampak TP historis tidak valid' USING ERRCODE='22023';
  END IF;
  IF EXISTS (
    WITH candidates AS (
      SELECT p.student_id FROM app.student_placements p
      WHERE p.school_year_id=school_year AND academic_write.class_relevant(p.id,cls)
      UNION
      SELECT f.student_id FROM app.semester_final_versions f
      WHERE (f.semester_id,f.class_id,f.subject_id)=(sem,cls,subject)
    ), latest AS (
      SELECT DISTINCT ON (f.student_id) f.student_id,f.id,f.version
      FROM app.semester_final_versions f
      WHERE (f.semester_id,f.class_id,f.subject_id)=(sem,cls,subject)
      ORDER BY f.student_id,f.version DESC
    ), evaluated AS (
      SELECT e.student_id,e.final_id,e.final_version,count(*) OVER (PARTITION BY e.student_id) AS copies
      FROM jsonb_to_recordset(impact) e(student_id bigint,final_id bigint,final_version integer)
    )
    SELECT 1 FROM candidates c FULL JOIN evaluated e ON e.student_id=c.student_id
    LEFT JOIN latest f ON f.student_id=c.student_id
    WHERE c.student_id IS NULL OR e.student_id IS NULL OR e.copies<>1
       OR (e.final_id,e.final_version) IS DISTINCT FROM (f.id,f.version)
  ) THEN
    RAISE EXCEPTION 'Siswa atau versi Final pada dampak telah berubah' USING ERRCODE='40001';
  END IF;

  objective := nextval('app.learning_objectives_id_seq'::regclass);
  INSERT INTO app.historical_learning_objective_insertions
    (learning_objective_id,semester_id,class_id,subject_id,actor_id,teacher_id,assessment_date,reason,
     learning_evidence,assessment_evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key)
    VALUES(objective,sem,cls,subject,p_actor,teacher,(input->>'AssessmentDate')::date,
      academic_write.trim_evidence(input->>'Reason'),learning_metadata,assessment_metadata,verification,
      jsonb_build_object('format','student-impact-v2','semester_id',sem,'class_id',cls,'subject_id',subject,
        'students',impact),
      proof,request_hash,submission);
  INSERT INTO app.learning_objectives(id,school_year_id,semester_id,class_id,subject_id,definition,created_by)
    OVERRIDING SYSTEM VALUE VALUES(objective,school_year,sem,cls,subject,academic_write.trim_evidence(input->>'Definition'),p_actor);
  RETURN objective;
END $$;
-- +goose StatementEnd

REVOKE EXECUTE ON FUNCTION academic_write.confirm_historical_tp(bigint,bytea,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION academic_write.confirm_historical_tp(bigint,bytea,bytea,bytea) TO sim_guru_app;
DROP FUNCTION academic_write.confirm_historical_tp(bigint,bytea,bytea);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Perubahan format audit TP historis memerlukan pemulihan eksplisit'; END $$;
