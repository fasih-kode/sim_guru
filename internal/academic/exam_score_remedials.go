package academic

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type ExamScoreRemedialDetail struct {
	Score     ExamOwnedScore
	Effective string
	CanRecord bool
	History   []AssessmentScoreRemedial
}

// ExamScoreRemedialDetail mempertahankan akses baca pemilik historis tanpa memberikan hak mutasi.
func (s *Store) ExamScoreRemedialDetail(ctx context.Context, actor, scoreID int64) (ExamScoreRemedialDetail, error) {
	var out ExamScoreRemedialDetail
	if scoreID < 1 {
		return out, ValidationError("Catatan nilai tidak valid.")
	}
	tx, err := s.examHistoryReader(ctx, actor)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	var semester, class, subject int64
	err = tx.QueryRowContext(ctx, `SELECT e.id,st.name,e.component,e.exam_date::text,cl.grade::text||cl.name,su.name,y.name||' · Semester '||sm.number::text,e.score::text,coalesce(c.after_score,e.score)::text,greatest(coalesce(c.after_score,e.score),coalesce(r.best,e.score))::text,e.semester_id,e.class_id,e.subject_id
 FROM app.exam_initial_scores e JOIN app.students st ON st.id=e.student_id JOIN app.classes cl ON cl.id=e.class_id JOIN app.subjects su ON su.id=e.subject_id JOIN app.semesters sm ON sm.id=e.semester_id JOIN app.school_years y ON y.id=e.school_year_id
 LEFT JOIN LATERAL (SELECT after_score FROM app.exam_score_corrections WHERE initial_score_id=e.id ORDER BY version DESC LIMIT 1) c ON true
 LEFT JOIN LATERAL (SELECT max(score) best FROM app.exam_score_remedials WHERE initial_score_id=e.id) r ON true
 WHERE e.id=$1 AND e.recorded_by=$2 AND e.teacher_id=$3`, scoreID, actor, teacher).Scan(&out.Score.ID, &out.Score.StudentName, &out.Score.Component, &out.Score.Date, &out.Score.ClassName, &out.Score.SubjectName, &out.Score.SemesterName, &out.Score.InitialValue, &out.Score.CorrectedValue, &out.Effective, &semester, &class, &subject)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	out.CanRecord, err = learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.score::text,r.remedial_date::text,u.display_name,r.recorded_at AT TIME ZONE 'Asia/Jakarta' FROM app.exam_score_remedials r JOIN app.users u ON u.id=r.recorded_by WHERE r.initial_score_id=$1 ORDER BY r.remedial_date,r.id`, scoreID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.History = []AssessmentScoreRemedial{}
	for rows.Next() {
		var item AssessmentScoreRemedial
		var at time.Time
		if err = rows.Scan(&item.ID, &item.Value, &item.Date, &item.Actor, &at); err != nil {
			return out, err
		}
		item.Recorded = at.Format("02-01-2006 15:04")
		out.History = append(out.History, item)
	}
	return out, rows.Err()
}

// RecordExamScoreRemedial mengunci sumber, memeriksa hak dan penempatan, kemudian mencatat sekali per submission.
func (s *Store) RecordExamScoreRemedial(ctx context.Context, actor, scoreID int64, key, value, date string) error {
	if scoreID < 1 {
		return ValidationError("Catatan nilai tidak valid.")
	}
	key, err := normalizeRemedialSubmissionKey(key)
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
	d, err := time.Parse("2006-01-02", date)
	if err != nil || d.Format("2006-01-02") != date {
		return ValidationError("Tanggal remedial tidak valid.")
	}
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	var semester, class, subject, placement int64
	var examDate, today string
	err = tx.QueryRowContext(ctx, `SELECT semester_id,class_id,subject_id,placement_id,exam_date::text,(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text FROM app.exam_initial_scores WHERE id=$1 AND recorded_by=$2 AND teacher_id=$3 FOR UPDATE`, scoreID, actor, teacher).Scan(&semester, &class, &subject, &placement, &examDate, &today)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	ok, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if date < examDate {
		return ValidationError("Tanggal remedial tidak boleh sebelum tanggal ujian awal.")
	}
	if date > today {
		return ValidationError("Tanggal remedial tidak boleh setelah hari ini.")
	}
	var placed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_placements p WHERE p.id=$1 AND p.starts_on IS NOT NULL AND p.starts_on<=$2::date AND app.student_placement_class_at(p.id,$2::date)=$3)`, placement, date, class).Scan(&placed)
	if err != nil {
		return err
	}
	if !placed {
		return ValidationError("Penempatan siswa pada tanggal remedial tidak sesuai kelas sumber.")
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app.exam_score_remedials(initial_score_id,score,remedial_date,recorded_by,submission_key) VALUES($1,$2::numeric,$3::date,$4,$5::uuid) ON CONFLICT(recorded_by,submission_key) DO NOTHING RETURNING id`, scoreID, formatScore(cents), date, actor, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var oldID int64
		var oldValue, oldDate string
		if err = tx.QueryRowContext(ctx, `SELECT initial_score_id,score::text,remedial_date::text FROM app.exam_score_remedials WHERE recorded_by=$1 AND submission_key=$2::uuid`, actor, key).Scan(&oldID, &oldValue, &oldDate); err != nil {
			return err
		}
		if oldID != scoreID || oldValue != formatScore(cents) || oldDate != date {
			return ErrConflict
		}
		return tx.Commit()
	}
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}
