package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const maxAssessmentActivityName = 200

// AssessmentActivity adalah kegiatan Formatif/Sumatif beserta TP yang dinilainya.
type AssessmentActivity struct {
	ID, SemesterID, ClassID, SubjectID, TeacherID, CreatedBy int64
	Name, Type, Date, CreatedAt                              string
	SemesterName, ClassName, SubjectName                     string
	Objectives                                               []LearningObjective
}

// AssessmentActivities membaca daftar kegiatan pada konteks penugasan guru.
func (s *Store) AssessmentActivities(ctx context.Context, actor, semesterID, classID, subjectID int64) ([]AssessmentActivity, error) {
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
		SELECT a.id,a.semester_id,a.class_id,a.subject_id,a.teacher_id,a.name,a.activity_type,
		       a.activity_date::text,a.created_by,a.created_at AT TIME ZONE 'Asia/Jakarta',
		       lo.id,lo.definition
		FROM app.assessment_activities a
		JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id
		JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id
		WHERE a.semester_id=$1 AND a.class_id=$2 AND a.subject_id=$3
		ORDER BY a.activity_date DESC,a.id DESC,lo.id`, semesterID, classID, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activities := make([]AssessmentActivity, 0)
	byID := make(map[int64]int)
	for rows.Next() {
		var item AssessmentActivity
		var created time.Time
		var objective LearningObjective
		if err = rows.Scan(&item.ID, &item.SemesterID, &item.ClassID, &item.SubjectID, &item.TeacherID,
			&item.Name, &item.Type, &item.Date, &item.CreatedBy, &created, &objective.ID, &objective.Definition); err != nil {
			return nil, err
		}
		if index, ok := byID[item.ID]; ok {
			activities[index].Objectives = append(activities[index].Objectives, objective)
			continue
		}
		item.CreatedAt = created.Format("02-01-2006 15:04")
		item.Objectives = []LearningObjective{objective}
		byID[item.ID] = len(activities)
		activities = append(activities, item)
	}
	return activities, rows.Err()
}

// CreateAssessmentActivity menyimpan kegiatan dan seluruh tautan TP dalam satu transaksi.
func (s *Store) CreateAssessmentActivity(ctx context.Context, actor, semesterID, classID, subjectID int64, name, activityType, activityDate string, objectiveIDs []int64) error {
	name = strings.TrimSpace(name)
	if !validAssessmentActivityName(name) {
		return ValidationError("Nama atau bentuk kegiatan wajib diisi dan maksimal 200 karakter.")
	}
	if activityType != "formatif" && activityType != "sumatif" {
		return ValidationError("Pilih jenis kegiatan Formatif atau Sumatif.")
	}
	parsedDate, err := time.Parse("2006-01-02", activityDate)
	if err != nil || parsedDate.Format("2006-01-02") != activityDate {
		return ValidationError("Tanggal kegiatan tidak valid.")
	}
	if semesterID < 1 || classID < 1 || subjectID < 1 {
		return ValidationError("Pilih konteks semester, kelas, dan mata pelajaran.")
	}
	if len(objectiveIDs) == 0 {
		return ValidationError("Pilih sedikitnya satu Tujuan Pembelajaran.")
	}
	seen := make(map[int64]struct{}, len(objectiveIDs))
	for _, id := range objectiveIDs {
		if id < 1 {
			return ValidationError("Pilihan Tujuan Pembelajaran tidak valid.")
		}
		if _, exists := seen[id]; exists {
			return ValidationError("Tujuan Pembelajaran yang sama tidak boleh dipilih lebih dari sekali.")
		}
		seen[id] = struct{}{}
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
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, semesterID, classID, subjectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	var yearID int64
	err = tx.QueryRowContext(ctx, `SELECT school_year_id FROM app.semesters WHERE id=$1`, semesterID).Scan(&yearID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM app.learning_objectives
		WHERE id=ANY($1::bigint[]) AND school_year_id=$2 AND semester_id=$3 AND class_id=$4 AND subject_id=$5
		ORDER BY id FOR SHARE`, objectiveIDs, yearID, semesterID, classID, subjectID)
	if err != nil {
		return err
	}
	matched := 0
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		matched++
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if matched != len(objectiveIDs) {
		return ErrForbidden
	}
	var activityID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app.assessment_activities
		(school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::date,$9) RETURNING id`,
		yearID, semesterID, classID, subjectID, teacherID, name, activityType, activityDate, actor).Scan(&activityID)
	if err != nil {
		return dbError(err)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO app.assessment_activity_objectives
		(activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id)
		SELECT $1,lo.id,lo.school_year_id,lo.semester_id,lo.class_id,lo.subject_id
		FROM app.learning_objectives lo
		WHERE lo.id=ANY($2::bigint[]) AND lo.school_year_id=$3 AND lo.semester_id=$4 AND lo.class_id=$5 AND lo.subject_id=$6`,
		activityID, objectiveIDs, yearID, semesterID, classID, subjectID)
	if err != nil {
		return dbError(err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted != int64(len(objectiveIDs)) {
		return errors.New("jumlah tautan TP berubah selama penyimpanan")
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func validAssessmentActivityName(name string) bool {
	return utf8.ValidString(name) && !strings.ContainsRune(name, 0) && utf8.RuneCountInString(name) >= 1 && utf8.RuneCountInString(name) <= maxAssessmentActivityName
}
