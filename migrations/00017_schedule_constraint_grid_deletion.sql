-- +goose Up
-- Keep grid rows as audit anchors after deletion and allow a new active grid
-- for the same target in the same semester.
ALTER TABLE app.schedule_constraint_grids ADD COLUMN deleted_at timestamptz;
DROP INDEX app.constraint_grid_teacher;
DROP INDEX app.constraint_grid_subject;
CREATE UNIQUE INDEX constraint_grid_teacher ON app.schedule_constraint_grids(semester_id,teacher_id) WHERE teacher_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX constraint_grid_subject ON app.schedule_constraint_grids(semester_id,subject_id) WHERE subject_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DO $$ BEGIN
 RAISE EXCEPTION 'Deleted grid audit records require an explicit recovery strategy';
END $$;
