-- Berikan kode stabil kepada TP yang sudah ada tanpa mengubah definisinya.
-- +goose Up
INSERT INTO app.direct_tp_codes(objective_id,school_year_id,semester_id,class_id,subject_id,ordinal,assigned_by,assigned_at)
SELECT o.id,o.school_year_id,o.semester_id,o.class_id,o.subject_id,
       coalesce((SELECT max(c.ordinal) FROM app.direct_tp_codes c WHERE
         (c.semester_id,c.class_id,c.subject_id)=(o.semester_id,o.class_id,o.subject_id)),0) +
       row_number() OVER (PARTITION BY o.semester_id,o.class_id,o.subject_id ORDER BY o.created_at,o.id),
       o.created_by,o.created_at
FROM app.learning_objectives o
WHERE NOT EXISTS (SELECT 1 FROM app.direct_tp_codes c WHERE c.objective_id=o.id);

INSERT INTO app.direct_tp_code_counters(semester_id,class_id,subject_id,last_ordinal)
SELECT semester_id,class_id,subject_id,max(ordinal) FROM app.direct_tp_codes
GROUP BY semester_id,class_id,subject_id
ON CONFLICT(semester_id,class_id,subject_id) DO UPDATE
SET last_ordinal=greatest(app.direct_tp_code_counters.last_ordinal,excluded.last_ordinal);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Kode TP permanen tidak boleh dihapus secara otomatis'; END $$;
