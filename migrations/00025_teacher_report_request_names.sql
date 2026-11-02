-- +goose Up
-- Snapshot permintaan harus menerima seluruh nama yang sah pada master guru.
ALTER TABLE app.teacher_attendance_correction_requests
 DROP CONSTRAINT teacher_attendance_correction_requests_teacher_name_check,
 ADD CONSTRAINT teacher_attendance_correction_requests_teacher_name_check
 CHECK(length(trim(teacher_name)) BETWEEN 1 AND 200);

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Batas nama snapshot tidak boleh dipersempit tanpa pemeriksaan data'; END $$;
