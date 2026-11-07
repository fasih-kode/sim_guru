package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const assessmentScoreHistoryPageSize = 10

type AssessmentOwnedScoreFilter struct {
	Search string
	Page   int
}

type AssessmentOwnedScore struct {
	ID, ActivityID, StudentID, ObjectiveID                int64
	SemesterID, ClassID, SubjectID, TeacherID, CreatedBy  int64
	InitialValue, CorrectedValue, EffectiveValue          string
	Version                                               int64
	StudentName, ActivityName, ActivityType               string
	Date, ClassName, SubjectName, SemesterName, Objective string
}

type AssessmentOwnedScorePage struct {
	Items      []AssessmentOwnedScore
	Page       int
	TotalPages int
	Search     string
}

type AssessmentScoreCorrection struct {
	Version               int64
	Before, After, Reason string
	Actor                 string
	At                    string
}

type AssessmentScoreCorrectionDetail struct {
	Score          AssessmentOwnedScore
	History        []AssessmentScoreCorrection
	CurrentVersion int64
}

// OwnedAssessmentInitialScores memberi pemilik akses historis terbatas ke nilai
// awal yang ia catat sendiri, tanpa mensyaratkan penugasan kelas masih aktif.
func (s *Store) OwnedAssessmentInitialScores(ctx context.Context, actor int64, filter AssessmentOwnedScoreFilter) (AssessmentOwnedScorePage, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if utf8.RuneCountInString(filter.Search) > 120 {
		return AssessmentOwnedScorePage{}, ValidationError("Kata pencarian maksimal 120 karakter.")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return AssessmentOwnedScorePage{}, err
	}
	defer tx.Rollback()
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return AssessmentOwnedScorePage{}, err
	}
	query := assessmentOwnedScoreQuery
	pattern := "%" + filter.Search + "%"
	var total int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.assessment_activity_initial_scores sc
		JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
		JOIN app.learning_objectives lo ON lo.id=sc.learning_objective_id
		JOIN app.students st ON st.id=sc.student_id
		JOIN app.classes cl ON cl.id=sc.class_id JOIN app.subjects su ON su.id=sc.subject_id
		WHERE sc.recorded_by=$1 AND a.teacher_id=$2 AND ($3='' OR
			st.name ILIKE $4 OR a.name ILIKE $4 OR cl.grade::text||cl.name ILIKE $4 OR
			su.name ILIKE $4 OR lo.definition ILIKE $4)`, actor, teacherID, filter.Search, pattern).Scan(&total); err != nil {
		return AssessmentOwnedScorePage{}, err
	}
	page := AssessmentOwnedScorePage{Page: filter.Page, Search: filter.Search, Items: make([]AssessmentOwnedScore, 0)}
	if total == 0 {
		return page, nil
	}
	page.TotalPages = (total + assessmentScoreHistoryPageSize - 1) / assessmentScoreHistoryPageSize
	if filter.Page > page.TotalPages {
		page.Page = page.TotalPages
	}
	rows, err := tx.QueryContext(ctx, query, actor, teacherID, filter.Search, pattern, assessmentScoreHistoryPageSize, (page.Page-1)*assessmentScoreHistoryPageSize)
	if err != nil {
		return AssessmentOwnedScorePage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AssessmentOwnedScore
		if err = rows.Scan(&item.ID, &item.ActivityID, &item.StudentID, &item.ObjectiveID, &item.InitialValue,
			&item.CorrectedValue, &item.EffectiveValue, &item.Version, &item.StudentName, &item.ActivityName, &item.ActivityType,
			&item.Date, &item.ClassName, &item.SubjectName, &item.Objective, &item.SemesterName); err != nil {
			return AssessmentOwnedScorePage{}, err
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

const assessmentOwnedScoreQuery = `SELECT sc.id,a.id,sc.student_id,sc.learning_objective_id,sc.score::text,
	coalesce(latest.after_score,sc.score)::text,
	CASE WHEN a.activity_type='sumatif' THEN greatest(coalesce(latest.after_score,sc.score),coalesce(remedial.best_score,sc.score))
		ELSE coalesce(latest.after_score,sc.score) END::text,
	coalesce(latest.version,0),st.name,a.name,a.activity_type,
	a.activity_date::text,cl.grade::text||cl.name,su.name,lo.definition,y.name||' · Semester '||sm.number::text
	FROM app.assessment_activity_initial_scores sc
	JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
	JOIN app.learning_objectives lo ON lo.id=sc.learning_objective_id
	JOIN app.students st ON st.id=sc.student_id
	JOIN app.classes cl ON cl.id=sc.class_id JOIN app.subjects su ON su.id=sc.subject_id
	JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.school_years y ON y.id=sm.school_year_id
	LEFT JOIN LATERAL (SELECT c.after_score,c.version FROM app.assessment_activity_score_corrections c
		WHERE c.initial_score_id=sc.id ORDER BY c.version DESC LIMIT 1) latest ON true
	LEFT JOIN LATERAL (SELECT max(r.score) AS best_score FROM app.assessment_activity_score_remedials r
		WHERE r.initial_score_id=sc.id) remedial ON true
	WHERE sc.recorded_by=$1 AND a.teacher_id=$2 AND ($3='' OR st.name ILIKE $4 OR a.name ILIKE $4 OR
		cl.grade::text||cl.name ILIKE $4 OR su.name ILIKE $4 OR lo.definition ILIKE $4)
	ORDER BY a.activity_date DESC,a.id DESC,st.name,lo.id,sc.id
	LIMIT $5 OFFSET $6`

// AssessmentScoreCorrectionDetail hanya memuat nilai milik asli aktor.
func (s *Store) AssessmentScoreCorrectionDetail(ctx context.Context, actor, scoreID int64) (AssessmentScoreCorrectionDetail, error) {
	if scoreID < 1 {
		return AssessmentScoreCorrectionDetail{}, ValidationError("Catatan nilai tidak valid.")
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return AssessmentScoreCorrectionDetail{}, err
	}
	defer tx.Rollback()
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return AssessmentScoreCorrectionDetail{}, err
	}
	var out AssessmentScoreCorrectionDetail
	err = tx.QueryRowContext(ctx, `SELECT sc.id,a.id,sc.student_id,sc.learning_objective_id,sc.score::text,
		coalesce(latest.after_score,sc.score)::text,
		CASE WHEN a.activity_type='sumatif' THEN greatest(coalesce(latest.after_score,sc.score),coalesce(remedial.best_score,sc.score))
			ELSE coalesce(latest.after_score,sc.score) END::text,
		coalesce(latest.version,0),st.name,a.name,a.activity_type,
		a.activity_date::text,cl.grade::text||cl.name,su.name,lo.definition,
		a.semester_id,a.class_id,a.subject_id,a.teacher_id,a.created_by,
		y.name||' · Semester '||sm.number::text
		FROM app.assessment_activity_initial_scores sc
		JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
		JOIN app.learning_objectives lo ON lo.id=sc.learning_objective_id
		JOIN app.students st ON st.id=sc.student_id
		JOIN app.classes cl ON cl.id=sc.class_id JOIN app.subjects su ON su.id=sc.subject_id
		JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.school_years y ON y.id=sm.school_year_id
		LEFT JOIN LATERAL (SELECT c.after_score,c.version FROM app.assessment_activity_score_corrections c
			WHERE c.initial_score_id=sc.id ORDER BY c.version DESC LIMIT 1) latest ON true
		LEFT JOIN LATERAL (SELECT max(r.score) AS best_score FROM app.assessment_activity_score_remedials r
			WHERE r.initial_score_id=sc.id) remedial ON true
		WHERE sc.id=$1 AND sc.recorded_by=$2 AND a.teacher_id=$3`, scoreID, actor, teacherID).Scan(
		&out.Score.ID, &out.Score.ActivityID, &out.Score.StudentID, &out.Score.ObjectiveID, &out.Score.InitialValue,
		&out.Score.CorrectedValue, &out.Score.EffectiveValue, &out.CurrentVersion, &out.Score.StudentName, &out.Score.ActivityName,
		&out.Score.ActivityType, &out.Score.Date, &out.Score.ClassName, &out.Score.SubjectName,
		&out.Score.Objective, &out.Score.SemesterID, &out.Score.ClassID, &out.Score.SubjectID,
		&out.Score.TeacherID, &out.Score.CreatedBy, &out.Score.SemesterName)
	if errors.Is(err, sql.ErrNoRows) {
		return AssessmentScoreCorrectionDetail{}, ErrNotFound
	}
	if err != nil {
		return AssessmentScoreCorrectionDetail{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.version,c.before_score::text,c.after_score::text,c.reason,u.display_name,
		c.corrected_at AT TIME ZONE 'Asia/Jakarta'
		FROM app.assessment_activity_score_corrections c JOIN app.users u ON u.id=c.corrected_by
		WHERE c.initial_score_id=$1 ORDER BY c.version`, scoreID)
	if err != nil {
		return AssessmentScoreCorrectionDetail{}, err
	}
	defer rows.Close()
	out.History = make([]AssessmentScoreCorrection, 0)
	for rows.Next() {
		var item AssessmentScoreCorrection
		var at time.Time
		if err = rows.Scan(&item.Version, &item.Before, &item.After, &item.Reason, &item.Actor, &at); err != nil {
			return AssessmentScoreCorrectionDetail{}, err
		}
		item.At = at.Format("02-01-2006 15:04")
		out.History = append(out.History, item)
	}
	return out, rows.Err()
}

// CorrectAssessmentInitialScore menambahkan versi koreksi tanpa mengubah nilai awal.
func (s *Store) CorrectAssessmentInitialScore(ctx context.Context, actor, scoreID, expectedVersion int64, newValue, reason string) error {
	if scoreID < 1 || expectedVersion < 0 {
		return ValidationError("Versi atau catatan nilai tidak valid. Muat ulang halaman.")
	}
	parsed, present, err := parseInitialScore(newValue)
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
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	var initialCents int64
	err = tx.QueryRowContext(ctx, `SELECT (sc.score*100)::bigint
		FROM app.assessment_activity_initial_scores sc
		JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
		WHERE sc.id=$1 AND sc.recorded_by=$2 AND a.teacher_id=$3
		FOR UPDATE OF sc`, scoreID, actor, teacherID).Scan(&initialCents)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	var currentCents int64
	var currentVersion int64
	err = tx.QueryRowContext(ctx, `SELECT (c.after_score*100)::bigint,c.version
		FROM app.assessment_activity_score_corrections c WHERE c.initial_score_id=$1
		ORDER BY c.version DESC LIMIT 1`, scoreID).Scan(&currentCents, &currentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		currentCents, currentVersion = initialCents, 0
	} else if err != nil {
		return err
	}
	if currentVersion != expectedVersion {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.assessment_activity_score_corrections
		(initial_score_id,version,before_score,after_score,reason,corrected_by)
		VALUES($1,$2,$3::numeric,$4::numeric,$5,$6)`, scoreID, currentVersion+1,
		formatScore(currentCents), formatScore(parsed), reason, actor)
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}
