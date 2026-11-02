package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// LearningObjectiveContext adalah kombinasi yang benar-benar diampu guru.
type LearningObjectiveContext struct {
	SemesterID, ClassID, SubjectID       int64
	SemesterName, ClassName, SubjectName string
}

// LearningObjective menyimpan satu definisi TP beserta konteks dan pencatatnya.
type LearningObjective struct {
	ID, SemesterID, ClassID, SubjectID, CreatedBy int64
	Definition, CreatedAt                         string
}

// LearningObjectiveContexts hanya mengembalikan konteks penugasan aktif milik guru.
func (s *Store) LearningObjectiveContexts(ctx context.Context, actor int64) ([]LearningObjectiveContext, error) {
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT a.semester_id,a.class_id,a.subject_id,
		       y.name || ' · Semester ' || sm.number::text,
	       c.grade::text || c.name,s.name
		FROM app.teaching_assignments a
		JOIN app.semesters sm ON sm.id=a.semester_id
		JOIN app.school_years y ON y.id=sm.school_year_id
		JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id
		JOIN app.subjects s ON s.id=a.subject_id AND s.active
		WHERE a.teacher_id=$1
		  AND a.effective_from <= (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date
		  AND (a.effective_until IS NULL OR a.effective_until > (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date)
		ORDER BY 4,5,6`, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contexts []LearningObjectiveContext
	for rows.Next() {
		var item LearningObjectiveContext
		if err = rows.Scan(&item.SemesterID, &item.ClassID, &item.SubjectID, &item.SemesterName, &item.ClassName, &item.SubjectName); err != nil {
			return nil, err
		}
		contexts = append(contexts, item)
	}
	return contexts, rows.Err()
}

// LearningObjectives membaca definisi TP setelah memeriksa penugasan resmi.
func (s *Store) LearningObjectives(ctx context.Context, actor int64, semesterID, classID, subjectID int64) ([]LearningObjective, error) {
	if semesterID < 1 || classID < 1 || subjectID < 1 {
		return nil, ValidationError("Pilih konteks semester, kelas, dan mata pelajaran.")
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return nil, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, semesterID, classID, subjectID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id,semester_id,class_id,subject_id,definition,created_by,created_at AT TIME ZONE 'Asia/Jakarta'
		FROM app.learning_objectives
		WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3
		ORDER BY id`, semesterID, classID, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var objectives []LearningObjective
	for rows.Next() {
		var item LearningObjective
		var created time.Time
		if err = rows.Scan(&item.ID, &item.SemesterID, &item.ClassID, &item.SubjectID, &item.Definition, &item.CreatedBy, &created); err != nil {
			return nil, err
		}
		item.CreatedAt = created.Format("02-01-2006 15:04")
		objectives = append(objectives, item)
	}
	return objectives, rows.Err()
}

// CreateLearningObjective menulis TP hanya pada konteks penugasan guru yang masih berlaku.
func (s *Store) CreateLearningObjective(ctx context.Context, actor int64, semesterID, classID, subjectID int64, definition string) error {
	definition = strings.TrimSpace(definition)
	if semesterID < 1 || classID < 1 || subjectID < 1 {
		return ValidationError("Pilih konteks semester, kelas, dan mata pelajaran.")
	}
	if definition == "" {
		return ValidationError("Uraian Tujuan Pembelajaran wajib diisi.")
	}
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	var yearID int64
	err = tx.QueryRowContext(ctx, `SELECT sm.school_year_id FROM app.semesters sm WHERE sm.id=$1`, semesterID).Scan(&yearID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, semesterID, classID, subjectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by)
		VALUES($1,$2,$3,$4,$5,$6)`, yearID, semesterID, classID, subjectID, definition, actor)
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}

// CopyLearningObjective menyalin uraian TP ke kelas lain yang sedang diampu aktor.
// Konteks sumber dan tujuan diperiksa kembali dalam transaksi yang sama.
func (s *Store) CopyLearningObjective(ctx context.Context, actor, sourceID, targetSemesterID, targetClassID, targetSubjectID int64) (int64, error) {
	if sourceID < 1 || targetSemesterID < 1 || targetClassID < 1 || targetSubjectID < 1 {
		return 0, ValidationError("Pilih TP sumber dan konteks kelas tujuan yang valid.")
	}
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return 0, err
	}
	var sourceSemesterID, sourceClassID, sourceSubjectID int64
	var definition string
	err = tx.QueryRowContext(ctx, `
		SELECT semester_id,class_id,subject_id,definition
		FROM app.learning_objectives WHERE id=$1 FOR SHARE`, sourceID).
		Scan(&sourceSemesterID, &sourceClassID, &sourceSubjectID, &definition)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, sourceSemesterID, sourceClassID, sourceSubjectID)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, ErrForbidden
	}
	if sourceClassID == targetClassID {
		return 0, ValidationError("Kelas tujuan harus berbeda dari kelas sumber.")
	}
	allowed, err = learningObjectiveAssignment(ctx, tx, teacherID, targetSemesterID, targetClassID, targetSubjectID)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, ErrForbidden
	}
	var targetYearID int64
	err = tx.QueryRowContext(ctx, `SELECT school_year_id FROM app.semesters WHERE id=$1`, targetSemesterID).Scan(&targetYearID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	var copiedID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		targetYearID, targetSemesterID, targetClassID, targetSubjectID, definition, actor).Scan(&copiedID)
	if err != nil {
		return 0, dbError(err)
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return copiedID, nil
}

func learningObjectiveTeacherID(ctx context.Context, tx *sql.Tx, actor int64) (int64, error) {
	var teacherID int64
	err := tx.QueryRowContext(ctx, `
		SELECT t.id
		FROM app.teachers t
		JOIN app.users u ON u.id=t.user_id AND u.active
		JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel'
		WHERE u.id=$1 AND t.active`, actor).Scan(&teacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrForbidden
	}
	return teacherID, err
}

func learningObjectiveAssignment(ctx context.Context, tx *sql.Tx, teacherID, semesterID, classID, subjectID int64) (bool, error) {
	var allowed bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM app.teaching_assignments a
			JOIN app.semesters sm ON sm.id=a.semester_id
			JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id
			JOIN app.subjects s ON s.id=a.subject_id AND s.active
			WHERE a.teacher_id=$1 AND a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4
			  AND a.effective_from <= (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date
			  AND (a.effective_until IS NULL OR a.effective_until > (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date)
		)`, teacherID, semesterID, classID, subjectID).Scan(&allowed)
	return allowed, err
}
