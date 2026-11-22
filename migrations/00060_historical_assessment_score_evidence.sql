-- Nilai sumber terstruktur harus sama dengan nilai yang ditranskripsi.
-- Sumber aplikasi dinonaktifkan sampai tersedia referensi aplikasi yang dapat diverifikasi.
-- +goose Up
ALTER TABLE app.historical_assessment_initial_score_audits
    ADD COLUMN source_score numeric(5,2),
    ADD CONSTRAINT historical_score_source_score_range_check
        CHECK (source_score IS NULL OR (source_score >= 0 AND source_score <= 100));

-- Satu lokator arsip tidak boleh dipakai untuk pasangan siswa/TP yang berbeda
-- pada kegiatan yang sama, termasuk lewat batch terpisah atau permintaan serentak.
CREATE INDEX historical_score_source_locator_lookup_idx
    ON app.historical_assessment_initial_score_audits (
        assessment_activity_id,
        lower(regexp_replace(btrim(source_identity), '[[:space:]]+', ' ', 'g')),
        lower(regexp_replace(btrim(source_locator), '[[:space:]]+', ' ', 'g'))
    );

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.guard_historical_score_detail_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE activity_kind text;
BEGIN
    PERFORM pg_advisory_xact_lock(742009);
    NEW.transaction_id := txid_current();
    IF NEW.source_kind <> 'arsip_manual' OR NEW.source_score IS NULL OR NEW.source_score <> NEW.score THEN
        RAISE EXCEPTION 'Bukti arsip manual harus mencantumkan nilai sumber yang sama dengan nilai yang dicatat' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM app.historical_assessment_initial_score_audits h
        WHERE h.assessment_activity_id=NEW.assessment_activity_id
          AND lower(regexp_replace(btrim(h.source_identity), '[[:space:]]+', ' ', 'g'))=lower(regexp_replace(btrim(NEW.source_identity), '[[:space:]]+', ' ', 'g'))
          AND lower(regexp_replace(btrim(h.source_locator), '[[:space:]]+', ' ', 'g'))=lower(regexp_replace(btrim(NEW.source_locator), '[[:space:]]+', ' ', 'g'))
    ) THEN
        RAISE EXCEPTION 'Identitas dan lokator bukti sudah digunakan untuk nilai pada kegiatan ini' USING ERRCODE='23505';
    END IF;
    SELECT a.activity_type INTO activity_kind FROM app.assessment_activities a WHERE a.id=NEW.assessment_activity_id;
    IF activity_kind <> 'sumatif' OR NOT EXISTS (
        SELECT 1 FROM app.historical_assessment_score_batches b
        WHERE b.id=NEW.batch_id AND b.assessment_activity_id=NEW.assessment_activity_id
          AND b.school_year_id=NEW.school_year_id AND b.semester_id=NEW.semester_id
          AND b.class_id=NEW.class_id AND b.subject_id=NEW.subject_id
          AND b.teacher_id=NEW.teacher_id AND b.actor_id=NEW.actor_id
          AND b.activity_date=NEW.activity_date AND b.transaction_id=txid_current()
    ) OR NOT (
        EXISTS (SELECT 1 FROM app.historical_assessment_objective_links l
            WHERE l.activity_id=NEW.assessment_activity_id AND l.learning_objective_id=NEW.learning_objective_id
              AND l.teacher_id=NEW.teacher_id)
        OR EXISTS (SELECT 1 FROM app.historical_assessment_activity_insertions h
            JOIN app.historical_assessment_activity_objectives d ON d.audit_id=h.id
            WHERE h.activity_id=NEW.assessment_activity_id AND d.learning_objective_id=NEW.learning_objective_id
              AND h.teacher_id=NEW.teacher_id)
    ) OR EXISTS (
        SELECT 1 FROM app.assessment_activity_initial_scores s
        WHERE s.assessment_activity_id=NEW.assessment_activity_id
          AND s.learning_objective_id=NEW.learning_objective_id AND s.student_id=NEW.student_id
    ) OR NOT EXISTS (
        SELECT 1 FROM app.assessment_activities a
        JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id AND ao.learning_objective_id=NEW.learning_objective_id
        JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id
          AND lo.school_year_id=a.school_year_id AND lo.semester_id=a.semester_id
          AND lo.class_id=a.class_id AND lo.subject_id=a.subject_id
        JOIN app.student_placements p ON p.id=NEW.placement_id
          AND p.student_id=NEW.student_id AND p.school_year_id=a.school_year_id
        WHERE a.id=NEW.assessment_activity_id AND a.school_year_id=NEW.school_year_id
          AND a.semester_id=NEW.semester_id AND a.class_id=NEW.class_id AND a.subject_id=NEW.subject_id
          AND app.student_placement_class_at(p.id,a.activity_date)=a.class_id
          AND p.starts_on IS NOT NULL AND p.starts_on<=a.activity_date
    ) THEN
        RAISE EXCEPTION 'Rincian audit nilai tidak cocok dengan pasangan historis atau penempatan pada tanggal kegiatan' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM app.historical_assessment_initial_score_audits WHERE source_score IS NOT NULL) THEN
        RAISE EXCEPTION 'Bukti nilai CP10-23 sudah tersimpan; pemulihan eksplisit diperlukan';
    END IF;
    RAISE EXCEPTION 'Migration memperketat provenance nilai historis; Down memerlukan strategi eksplisit';
END $$;
