package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type ExamOwnedScore struct {
	ID                                                                 int64
	StudentName, Component, Date, ClassName, SubjectName, SemesterName string
	InitialValue, CorrectedValue                                       string
}
type ExamOwnedScorePage struct {
	Items            []ExamOwnedScore
	Page, TotalPages int
	Search           string
}
type ExamScoreCorrectionDetail struct {
	Score          ExamOwnedScore
	CurrentVersion int64
	History        []AssessmentScoreCorrection
}

// OwnedExamInitialScores hanya membuka bukti yang benar-benar dicatat oleh pemiliknya.
func (s *Store) OwnedExamInitialScores(ctx context.Context, actor int64, filter AssessmentOwnedScoreFilter) (ExamOwnedScorePage, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if utf8.RuneCountInString(filter.Search) > 120 {
		return ExamOwnedScorePage{}, ValidationError("Kata pencarian maksimal 120 karakter.")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	tx, err := s.examHistoryReader(ctx, actor)
	if err != nil {
		return ExamOwnedScorePage{}, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return ExamOwnedScorePage{}, err
	}
	where := ` FROM app.exam_initial_scores e JOIN app.students st ON st.id=e.student_id JOIN app.classes cl ON cl.id=e.class_id JOIN app.subjects su ON su.id=e.subject_id JOIN app.semesters sm ON sm.id=e.semester_id JOIN app.school_years y ON y.id=e.school_year_id WHERE e.recorded_by=$1 AND e.teacher_id=$2 AND ($3='' OR st.name ILIKE $4 OR cl.grade::text||cl.name ILIKE $4 OR su.name ILIKE $4 OR e.component ILIKE $4)`
	pattern := "%" + filter.Search + "%"
	var total int
	if err = tx.QueryRowContext(ctx, `SELECT count(*)`+where, actor, teacher, filter.Search, pattern).Scan(&total); err != nil {
		return ExamOwnedScorePage{}, err
	}
	out := ExamOwnedScorePage{Page: filter.Page, Search: filter.Search, Items: []ExamOwnedScore{}}
	if total == 0 {
		return out, nil
	}
	out.TotalPages = (total + assessmentScoreHistoryPageSize - 1) / assessmentScoreHistoryPageSize
	if out.Page > out.TotalPages {
		out.Page = out.TotalPages
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.id,st.name,e.component,e.exam_date::text,cl.grade::text||cl.name,su.name,y.name||' · Semester '||sm.number::text,e.score::text,coalesce((SELECT c.after_score FROM app.exam_score_corrections c WHERE c.initial_score_id=e.id ORDER BY c.version DESC LIMIT 1),e.score)::text`+where+` ORDER BY e.exam_date DESC,e.id DESC LIMIT $5 OFFSET $6`, actor, teacher, filter.Search, pattern, assessmentScoreHistoryPageSize, (out.Page-1)*assessmentScoreHistoryPageSize)
	if err != nil {
		return ExamOwnedScorePage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ExamOwnedScore
		if err = rows.Scan(&item.ID, &item.StudentName, &item.Component, &item.Date, &item.ClassName, &item.SubjectName, &item.SemesterName, &item.InitialValue, &item.CorrectedValue); err != nil {
			return ExamOwnedScorePage{}, err
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}

func (s *Store) ExamScoreCorrectionDetail(ctx context.Context, actor, scoreID int64) (ExamScoreCorrectionDetail, error) {
	var out ExamScoreCorrectionDetail
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
	err = tx.QueryRowContext(ctx, `SELECT e.id,st.name,e.component,e.exam_date::text,cl.grade::text||cl.name,su.name,y.name||' · Semester '||sm.number::text,e.score::text,coalesce(latest.after_score,e.score)::text,coalesce(latest.version,0)
 FROM app.exam_initial_scores e JOIN app.students st ON st.id=e.student_id JOIN app.classes cl ON cl.id=e.class_id JOIN app.subjects su ON su.id=e.subject_id JOIN app.semesters sm ON sm.id=e.semester_id JOIN app.school_years y ON y.id=e.school_year_id LEFT JOIN LATERAL (SELECT c.after_score,c.version FROM app.exam_score_corrections c WHERE c.initial_score_id=e.id ORDER BY c.version DESC LIMIT 1) latest ON true WHERE e.id=$1 AND e.recorded_by=$2 AND e.teacher_id=$3`, scoreID, actor, teacher).Scan(&out.Score.ID, &out.Score.StudentName, &out.Score.Component, &out.Score.Date, &out.Score.ClassName, &out.Score.SubjectName, &out.Score.SemesterName, &out.Score.InitialValue, &out.Score.CorrectedValue, &out.CurrentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.version,c.before_score::text,c.after_score::text,c.reason,u.display_name,c.corrected_at AT TIME ZONE 'Asia/Jakarta' FROM app.exam_score_corrections c JOIN app.users u ON u.id=c.corrected_by WHERE c.initial_score_id=$1 ORDER BY c.version`, scoreID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.History = []AssessmentScoreCorrection{}
	for rows.Next() {
		var item AssessmentScoreCorrection
		var at time.Time
		if err = rows.Scan(&item.Version, &item.Before, &item.After, &item.Reason, &item.Actor, &at); err != nil {
			return out, err
		}
		item.At = at.Format("02-01-2006 15:04")
		out.History = append(out.History, item)
	}
	return out, rows.Err()
}

// CorrectExamInitialScore mengunci sumber dan menambah versi tanpa mengubah bukti awal.
func (s *Store) CorrectExamInitialScore(ctx context.Context, actor, scoreID, expected int64, newValue, reason string) error {
	if scoreID < 1 || expected < 0 {
		return ValidationError("Versi atau catatan nilai tidak valid. Muat ulang halaman.")
	}
	value, present, err := parseInitialScore(newValue)
	if err != nil {
		return err
	}
	if !present {
		return ValidationError("Nilai koreksi wajib diisi.")
	}
	reason = strings.TrimSpace(reason)
	if !utf8.ValidString(reason) || strings.ContainsRune(reason, 0) || utf8.RuneCountInString(reason) < 1 || utf8.RuneCountInString(reason) > 500 {
		return ValidationError("Alasan koreksi wajib diisi, maksimal 500 karakter.")
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
	var original int64
	err = tx.QueryRowContext(ctx, `SELECT (score*100)::bigint FROM app.exam_initial_scores WHERE id=$1 AND recorded_by=$2 AND teacher_id=$3 FOR UPDATE`, scoreID, actor, teacher).Scan(&original)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	var current, version int64
	err = tx.QueryRowContext(ctx, `SELECT (after_score*100)::bigint,version FROM app.exam_score_corrections WHERE initial_score_id=$1 ORDER BY version DESC LIMIT 1`, scoreID).Scan(&current, &version)
	if errors.Is(err, sql.ErrNoRows) {
		current = original
		version = 0
	} else if err != nil {
		return err
	}
	if version != expected {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.exam_score_corrections(initial_score_id,version,before_score,after_score,reason,corrected_by) VALUES($1,$2,$3::numeric,$4::numeric,$5,$6)`, scoreID, version+1, formatScore(current), formatScore(value), reason, actor)
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}

// Pembacaan versi dan histori menggunakan satu snapshot agar tidak berbeda saat koreksi bersamaan.
func (s *Store) examHistoryReader(ctx context.Context, actor int64) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}
