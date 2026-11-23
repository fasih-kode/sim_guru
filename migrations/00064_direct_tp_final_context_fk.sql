-- Konteks Final baru harus berasal dari semester dan kelas pada tahun yang sama.
-- NOT VALID mempertahankan baris historis untuk audit terpisah, tetapi PostgreSQL
-- tetap memeriksa setiap baris baru dalam transaksi yang sama dengan penjaga.
-- +goose Up
ALTER TABLE app.semester_final_versions
  ADD CONSTRAINT direct_tp_final_semester_year_fk
  FOREIGN KEY (semester_id,school_year_id) REFERENCES app.semesters(id,school_year_id) NOT VALID,
  ADD CONSTRAINT direct_tp_final_class_year_fk
  FOREIGN KEY (class_id,school_year_id) REFERENCES app.classes(id,school_year_id) NOT VALID;

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'Batas konteks Final memerlukan strategi pemulihan eksplisit'; END $$;
