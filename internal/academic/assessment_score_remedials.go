package academic

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type AssessmentScoreRemedial struct {
	ID       int64
	Value    string
	Date     string
	Actor    string
	Recorded string
}

type AssessmentScoreRemedialDetail struct {
	Score                       AssessmentOwnedScore
	CorrectedInitial, Effective string
	History                     []AssessmentScoreRemedial
}

// AssessmentScoreRemedialDetail memberi pemilik bukti akses ke konteks dan
// seluruh remedialnya, tanpa membuka catatan milik guru lain.
func (s *Store) AssessmentScoreRemedialDetail(ctx context.Context, actor, scoreID int64) (AssessmentScoreRemedialDetail, error) {
	if scoreID < 1 {
		return AssessmentScoreRemedialDetail{}, ValidationError("Catatan nilai tidak valid.")
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return AssessmentScoreRemedialDetail{}, err
	}
	defer tx.Rollback()
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return AssessmentScoreRemedialDetail{}, err
	}
	var out AssessmentScoreRemedialDetail
	err = tx.QueryRowContext(ctx, `SELECT sc.id,a.id,sc.student_id,sc.learning_objective_id,sc.score::text,
		coalesce(c.after_score,sc.score)::text,
		greatest(coalesce(c.after_score,sc.score),coalesce(r.best_score,coalesce(c.after_score,sc.score)))::text,
		coalesce(c.version,0),st.name,a.name,a.activity_type,a.activity_date::text,
		cl.grade::text||cl.name,su.name,lo.definition,a.semester_id,a.class_id,a.subject_id,a.teacher_id,a.created_by,
		y.name||' · Semester '||sm.number::text
		FROM app.assessment_activity_initial_scores sc
		JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
		JOIN app.learning_objectives lo ON lo.id=sc.learning_objective_id
		JOIN app.students st ON st.id=sc.student_id JOIN app.classes cl ON cl.id=sc.class_id
		JOIN app.subjects su ON su.id=sc.subject_id JOIN app.semesters sm ON sm.id=a.semester_id
		JOIN app.school_years y ON y.id=sm.school_year_id
		LEFT JOIN LATERAL (SELECT after_score,version FROM app.assessment_activity_score_corrections
			WHERE initial_score_id=sc.id ORDER BY version DESC LIMIT 1) c ON true
		LEFT JOIN LATERAL (SELECT max(score) AS best_score FROM app.assessment_activity_score_remedials
			WHERE initial_score_id=sc.id) r ON true
		WHERE sc.id=$1 AND sc.recorded_by=$2 AND a.teacher_id=$3 AND a.activity_type='sumatif'`, scoreID, actor, teacherID).Scan(
		&out.Score.ID, &out.Score.ActivityID, &out.Score.StudentID, &out.Score.ObjectiveID,
		&out.Score.InitialValue, &out.CorrectedInitial, &out.Effective, &out.Score.Version,
		&out.Score.StudentName, &out.Score.ActivityName, &out.Score.ActivityType, &out.Score.Date,
		&out.Score.ClassName, &out.Score.SubjectName, &out.Score.Objective,
		&out.Score.SemesterID, &out.Score.ClassID, &out.Score.SubjectID,
		&out.Score.TeacherID, &out.Score.CreatedBy, &out.Score.SemesterName)
	if errors.Is(err, sql.ErrNoRows) {
		return AssessmentScoreRemedialDetail{}, ErrNotFound
	}
	if err != nil {
		return AssessmentScoreRemedialDetail{}, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, out.Score.SemesterID, out.Score.ClassID, out.Score.SubjectID)
	if err != nil {
		return AssessmentScoreRemedialDetail{}, err
	}
	if !allowed {
		return AssessmentScoreRemedialDetail{}, ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.score::text,r.remedial_date::text,u.display_name,
		r.recorded_at AT TIME ZONE 'Asia/Jakarta'
		FROM app.assessment_activity_score_remedials r JOIN app.users u ON u.id=r.recorded_by
		WHERE r.initial_score_id=$1 ORDER BY r.remedial_date,r.id`, scoreID)
	if err != nil {
		return AssessmentScoreRemedialDetail{}, err
	}
	defer rows.Close()
	out.History = make([]AssessmentScoreRemedial, 0)
	for rows.Next() {
		var item AssessmentScoreRemedial
		var at time.Time
		if err = rows.Scan(&item.ID, &item.Value, &item.Date, &item.Actor, &at); err != nil {
			return AssessmentScoreRemedialDetail{}, err
		}
		item.Recorded = at.Format("02-01-2006 15:04")
		out.History = append(out.History, item)
	}
	return out, rows.Err()
}

// RecordAssessmentScoreRemedial menambahkan hasil baru tanpa mengubah nilai awal
// atau histori koreksi. Penguncian baris mencegah dua penulis membaca konteks
// otorisasi yang berbeda untuk nilai yang sama.
func (s *Store) RecordAssessmentScoreRemedial(ctx context.Context, actor, scoreID int64, submissionKey, value, date string) error {
	if scoreID < 1 {
		return ValidationError("Catatan nilai tidak valid.")
	}
	submissionKey, err := normalizeRemedialSubmissionKey(submissionKey)
	if err != nil {
		return err
	}
	cents, present, err := parseInitialScore(value)
	if err != nil {
		return err
	}
	if !present {
		return ValidationError("Nilai remedial wajib diisi.")
	}
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil || parsedDate.Format("2006-01-02") != date {
		return ValidationError("Tanggal remedial tidak valid.")
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
	var semesterID, classID, subjectID int64
	var activityDate, todayWIB string
	err = tx.QueryRowContext(ctx, `SELECT a.semester_id,a.class_id,a.subject_id,a.activity_date::text,
		(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text
		FROM app.assessment_activity_initial_scores sc
		JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
		WHERE sc.id=$1 AND sc.recorded_by=$2 AND a.teacher_id=$3 AND a.activity_type='sumatif'
		FOR UPDATE OF sc,a`, scoreID, actor, teacherID).Scan(&semesterID, &classID, &subjectID, &activityDate, &todayWIB)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
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
	if date < activityDate {
		return ValidationError("Tanggal remedial tidak boleh sebelum tanggal kegiatan Sumatif.")
	}
	if date > todayWIB {
		return ValidationError("Tanggal remedial tidak boleh setelah hari ini.")
	}
	var remedialID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app.assessment_activity_score_remedials
		(initial_score_id,score,remedial_date,recorded_by,submission_key)
		VALUES($1,$2::numeric,$3::date,$4,$5::uuid)
		ON CONFLICT (recorded_by,submission_key) DO NOTHING
		RETURNING id`, scoreID, formatScore(cents), date, actor, submissionKey).Scan(&remedialID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingScoreID int64
		var existingValue, existingDate string
		if err = tx.QueryRowContext(ctx, `SELECT initial_score_id,score::text,remedial_date::text
			FROM app.assessment_activity_score_remedials
			WHERE recorded_by=$1 AND submission_key=$2::uuid`, actor, submissionKey).
			Scan(&existingScoreID, &existingValue, &existingDate); err != nil {
			return err
		}
		if existingScoreID != scoreID || existingValue != formatScore(cents) || existingDate != date {
			return ErrConflict
		}
		return tx.Commit()
	}
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}

func normalizeRemedialSubmissionKey(raw string) (string, error) {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return "", ValidationError("Kunci pengiriman tidak valid. Muat ulang formulir.")
	}
	compact := strings.ReplaceAll(raw, "-", "")
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != 16 {
		return "", ValidationError("Kunci pengiriman tidak valid. Muat ulang formulir.")
	}
	compact = strings.ToLower(compact)
	return compact[:8] + "-" + compact[8:12] + "-" + compact[12:16] + "-" + compact[16:20] + "-" + compact[20:], nil
}
