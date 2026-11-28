-- Fondasi nilai TP langsung. Migration dijalankan oleh pemilik DDL terpisah.
-- +goose Up
CREATE TABLE app.direct_tp_code_counters (
    semester_id bigint NOT NULL, class_id bigint NOT NULL, subject_id bigint NOT NULL,
    last_ordinal bigint NOT NULL CHECK (last_ordinal > 0),
    PRIMARY KEY (semester_id, class_id, subject_id)
);
CREATE TABLE app.direct_tp_codes (
    objective_id bigint PRIMARY KEY,
    school_year_id bigint NOT NULL, semester_id bigint NOT NULL, class_id bigint NOT NULL, subject_id bigint NOT NULL,
    ordinal bigint NOT NULL CHECK (ordinal > 0), assigned_by bigint NOT NULL REFERENCES app.users(id),
    assigned_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (objective_id,school_year_id,semester_id,class_id,subject_id)
      REFERENCES app.learning_objectives(id,school_year_id,semester_id,class_id,subject_id) ON DELETE RESTRICT,
    UNIQUE (semester_id,class_id,subject_id,ordinal)
);
-- +goose StatementBegin
CREATE FUNCTION app.direct_tp_reject_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Riwayat nilai TP langsung bersifat tetap' USING ERRCODE='55000'; END $$;
-- +goose StatementEnd
CREATE TRIGGER direct_tp_codes_immutable BEFORE UPDATE OR DELETE ON app.direct_tp_codes FOR EACH ROW EXECUTE FUNCTION app.direct_tp_reject_mutation();
-- +goose StatementBegin
CREATE FUNCTION app.direct_tp_assign_code() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,app,pg_temp AS $$
DECLARE next_ordinal bigint;
BEGIN
    INSERT INTO app.direct_tp_code_counters(semester_id,class_id,subject_id,last_ordinal)
      VALUES(NEW.semester_id,NEW.class_id,NEW.subject_id,1)
      ON CONFLICT(semester_id,class_id,subject_id) DO UPDATE
      SET last_ordinal=app.direct_tp_code_counters.last_ordinal+1 RETURNING last_ordinal INTO next_ordinal;
    INSERT INTO app.direct_tp_codes(objective_id,school_year_id,semester_id,class_id,subject_id,ordinal,assigned_by)
      VALUES(NEW.id,NEW.school_year_id,NEW.semester_id,NEW.class_id,NEW.subject_id,next_ordinal,NEW.created_by);
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER direct_tp_assign_code AFTER INSERT ON app.learning_objectives FOR EACH ROW EXECUTE FUNCTION app.direct_tp_assign_code();

CREATE TABLE app.direct_tp_guard (
    semester_id bigint NOT NULL, class_id bigint NOT NULL, subject_id bigint NOT NULL, student_id bigint NOT NULL,
    generation bigint NOT NULL DEFAULT 1 CHECK(generation>0),
    PRIMARY KEY(semester_id,class_id,subject_id,student_id)
);
-- +goose StatementBegin
CREATE FUNCTION app.direct_tp_lock_guard(p_semester bigint,p_class bigint,p_subject bigint,p_student bigint) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog,app,pg_temp AS $$
BEGIN
    INSERT INTO app.direct_tp_guard(semester_id,class_id,subject_id,student_id)
      VALUES(p_semester,p_class,p_subject,p_student)
      ON CONFLICT(semester_id,class_id,subject_id,student_id) DO UPDATE SET generation=app.direct_tp_guard.generation+1;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION app.direct_tp_final_guard() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,app,pg_temp AS $$
BEGIN PERFORM app.direct_tp_lock_guard(NEW.semester_id,NEW.class_id,NEW.subject_id,NEW.student_id); RETURN NEW; END $$;
-- +goose StatementEnd
CREATE TRIGGER direct_tp_final_guard BEFORE INSERT ON app.semester_final_versions FOR EACH ROW EXECUTE FUNCTION app.direct_tp_final_guard();

CREATE TABLE app.direct_tp_submissions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_year_id bigint NOT NULL, semester_id bigint NOT NULL, class_id bigint NOT NULL, subject_id bigint NOT NULL,
    teacher_id bigint NOT NULL REFERENCES app.teachers(id), actor_id bigint NOT NULL REFERENCES app.users(id),
    submission_key uuid NOT NULL, request_sha256 bytea NOT NULL CHECK(octet_length(request_sha256)=32),
    expected_count integer NOT NULL CHECK(expected_count BETWEEN 1 AND 5000),
    transaction_id bigint NOT NULL DEFAULT txid_current(), recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(actor_id,submission_key),
    UNIQUE(id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id)
);
CREATE TABLE app.direct_tp_initial_scores (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_year_id bigint NOT NULL, semester_id bigint NOT NULL, class_id bigint NOT NULL, subject_id bigint NOT NULL,
    objective_id bigint NOT NULL, student_id bigint NOT NULL, placement_id bigint NOT NULL,
    owner_teacher_id bigint NOT NULL, recorded_by bigint NOT NULL,
    score numeric(5,2) NOT NULL CHECK(score BETWEEN 0 AND 100),
    submission_id bigint NOT NULL, transaction_id bigint NOT NULL DEFAULT txid_current(),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY(objective_id,school_year_id,semester_id,class_id,subject_id)
      REFERENCES app.learning_objectives(id,school_year_id,semester_id,class_id,subject_id) ON DELETE RESTRICT,
    FOREIGN KEY(placement_id,student_id,school_year_id) REFERENCES app.student_placements(id,student_id,school_year_id) ON DELETE RESTRICT,
    FOREIGN KEY(submission_id,school_year_id,semester_id,class_id,subject_id,owner_teacher_id,recorded_by)
      REFERENCES app.direct_tp_submissions(id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id) ON DELETE RESTRICT,
    UNIQUE(semester_id,class_id,subject_id,objective_id,student_id)
);
CREATE TABLE app.direct_tp_score_corrections (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    initial_score_id bigint NOT NULL REFERENCES app.direct_tp_initial_scores(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK(version>0), before_score numeric(5,2) NOT NULL CHECK(before_score BETWEEN 0 AND 100),
    after_score numeric(5,2) NOT NULL CHECK(after_score BETWEEN 0 AND 100),
    corrected_by bigint NOT NULL REFERENCES app.users(id), submission_id bigint NOT NULL REFERENCES app.direct_tp_submissions(id),
    transaction_id bigint NOT NULL DEFAULT txid_current(), corrected_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK(before_score<>after_score), UNIQUE(initial_score_id,version), UNIQUE(submission_id,initial_score_id)
);
CREATE TRIGGER direct_tp_initial_immutable BEFORE UPDATE OR DELETE ON app.direct_tp_initial_scores FOR EACH ROW EXECUTE FUNCTION app.direct_tp_reject_mutation();
CREATE TRIGGER direct_tp_correction_immutable BEFORE UPDATE OR DELETE ON app.direct_tp_score_corrections FOR EACH ROW EXECUTE FUNCTION app.direct_tp_reject_mutation();
CREATE TRIGGER direct_tp_submissions_immutable BEFORE UPDATE OR DELETE ON app.direct_tp_submissions FOR EACH ROW EXECUTE FUNCTION app.direct_tp_reject_mutation();
-- +goose StatementBegin
CREATE FUNCTION app.direct_tp_score_guard() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,app,pg_temp AS $$
DECLARE sem bigint; cls bigint; subj bigint; stu bigint; old_score numeric; old_version integer; owner_id bigint; submit_actor bigint; submit_teacher bigint;
BEGIN
    IF TG_TABLE_NAME='direct_tp_initial_scores' THEN
        sem:=NEW.semester_id; cls:=NEW.class_id; subj:=NEW.subject_id; stu:=NEW.student_id;
        NEW.transaction_id:=txid_current();
    ELSE
        SELECT s.semester_id,s.class_id,s.subject_id,s.student_id,s.owner_teacher_id INTO sem,cls,subj,stu,owner_id
          FROM app.direct_tp_initial_scores s WHERE s.id=NEW.initial_score_id FOR UPDATE;
        IF NOT FOUND THEN RAISE EXCEPTION 'Nilai awal tidak ditemukan' USING ERRCODE='23503'; END IF;
        SELECT b.actor_id,b.teacher_id INTO submit_actor,submit_teacher FROM app.direct_tp_submissions b
          WHERE b.id=NEW.submission_id AND b.transaction_id=txid_current() AND (b.semester_id,b.class_id,b.subject_id)=(sem,cls,subj);
        IF NOT FOUND OR NEW.corrected_by<>submit_actor OR owner_id<>submit_teacher THEN RAISE EXCEPTION 'Audit koreksi tidak cocok' USING ERRCODE='23514'; END IF;
        SELECT coalesce(c.after_score,s.score),coalesce(c.version,0) INTO old_score,old_version FROM app.direct_tp_initial_scores s
          LEFT JOIN LATERAL (SELECT after_score,version FROM app.direct_tp_score_corrections WHERE initial_score_id=s.id ORDER BY version DESC LIMIT 1) c ON true WHERE s.id=NEW.initial_score_id;
        IF NEW.version<>old_version+1 OR NEW.before_score<>old_score THEN RAISE EXCEPTION 'Versi nilai sudah berubah' USING ERRCODE='40001'; END IF;
        NEW.transaction_id:=txid_current();
    END IF;
    PERFORM app.direct_tp_lock_guard(sem,cls,subj,stu);
    IF EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE (f.semester_id,f.class_id,f.subject_id,f.student_id)=(sem,cls,subj,stu)) THEN
        RAISE EXCEPTION 'Nilai TP siswa Final terkunci' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER direct_tp_initial_guard BEFORE INSERT ON app.direct_tp_initial_scores FOR EACH ROW EXECUTE FUNCTION app.direct_tp_score_guard();
CREATE TRIGGER direct_tp_correction_guard BEFORE INSERT ON app.direct_tp_score_corrections FOR EACH ROW EXECUTE FUNCTION app.direct_tp_score_guard();
-- +goose StatementBegin
CREATE FUNCTION app.direct_tp_submission_transaction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.transaction_id:=txid_current(); RETURN NEW; END $$;
-- +goose StatementEnd
CREATE TRIGGER direct_tp_submission_transaction BEFORE INSERT ON app.direct_tp_submissions FOR EACH ROW EXECUTE FUNCTION app.direct_tp_submission_transaction();
-- +goose StatementBegin
CREATE FUNCTION app.direct_tp_check_batch() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,app,pg_temp AS $$
DECLARE bid bigint; expected integer; actual integer; batch_tx bigint;
BEGIN
    IF TG_TABLE_NAME='direct_tp_submissions' THEN bid:=NEW.id; ELSE bid:=NEW.submission_id; END IF;
    SELECT expected_count,transaction_id INTO expected,batch_tx FROM app.direct_tp_submissions WHERE id=bid;
    IF batch_tx IS DISTINCT FROM txid_current() THEN RAISE EXCEPTION 'Transaksi audit tidak cocok' USING ERRCODE='23514'; END IF;
    SELECT (SELECT count(*) FROM app.direct_tp_initial_scores WHERE submission_id=bid AND transaction_id=batch_tx)+
           (SELECT count(*) FROM app.direct_tp_score_corrections WHERE submission_id=bid AND transaction_id=batch_tx) INTO actual;
    IF actual<>expected THEN RAISE EXCEPTION 'Jumlah audit batch tidak cocok' USING ERRCODE='23514'; END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER direct_tp_batch_complete AFTER INSERT ON app.direct_tp_submissions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.direct_tp_check_batch();
CREATE CONSTRAINT TRIGGER direct_tp_initial_batch_complete AFTER INSERT ON app.direct_tp_initial_scores DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.direct_tp_check_batch();
CREATE CONSTRAINT TRIGGER direct_tp_correction_batch_complete AFTER INSERT ON app.direct_tp_score_corrections DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION app.direct_tp_check_batch();
REVOKE ALL ON app.direct_tp_code_counters,app.direct_tp_guard FROM PUBLIC,sim_guru_app;
REVOKE ALL ON FUNCTION app.direct_tp_assign_code(),app.direct_tp_lock_guard(bigint,bigint,bigint,bigint),app.direct_tp_final_guard(),app.direct_tp_score_guard(),app.direct_tp_check_batch() FROM PUBLIC,sim_guru_app;
GRANT SELECT ON app.direct_tp_codes TO sim_guru_app;
GRANT SELECT,INSERT ON app.direct_tp_submissions,app.direct_tp_initial_scores,app.direct_tp_score_corrections TO sim_guru_app;
GRANT USAGE,SELECT ON SEQUENCE app.direct_tp_submissions_id_seq,app.direct_tp_initial_scores_id_seq,app.direct_tp_score_corrections_id_seq TO sim_guru_app;
GRANT UPDATE(id) ON app.direct_tp_initial_scores TO sim_guru_app;
GRANT UPDATE(id) ON app.semester_final_versions TO sim_guru_app;
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Nilai TP langsung memerlukan strategi pemulihan eksplisit'; END $$;
