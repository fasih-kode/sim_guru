package academic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// AssessmentScoreInput adalah satu nilai awal yang dikirim guru untuk satu siswa dan TP.
type AssessmentScoreInput struct {
	StudentID, ObjectiveID int64
	Value                  string
}

type AssessmentScoreCell struct {
	ObjectiveID      int64
	InitialScoreID   int64
	Value            string
	Recorded         bool
	CanCorrect       bool
	CanRemediate     bool
	HistoricalLocked bool
}

type AssessmentScoreStudent struct {
	ID        int64
	Name, NIS string
	Scores    []AssessmentScoreCell
}

type AssessmentScoreSheet struct {
	Activity              AssessmentActivity
	Students              []AssessmentScoreStudent
	UnknownPlacementCount int
	CanRecord             bool
	HasBlankScores        bool
}

// AssessmentActivityScoreSheet menampilkan roster menurut penempatan pada tanggal kegiatan.
func (s *Store) AssessmentActivityScoreSheet(ctx context.Context, actor, activityID int64) (AssessmentScoreSheet, error) {
	if activityID < 1 {
		return AssessmentScoreSheet{}, ValidationError("Kegiatan penilaian tidak valid.")
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return AssessmentScoreSheet{}, err
	}
	defer tx.Rollback()
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return AssessmentScoreSheet{}, err
	}
	var out AssessmentScoreSheet
	var yearID int64
	err = tx.QueryRowContext(ctx, `SELECT a.id,a.semester_id,a.class_id,a.subject_id,a.teacher_id,a.name,a.activity_type,
		a.activity_date::text,a.created_by,a.school_year_id
		FROM app.assessment_activities a WHERE a.id=$1`, activityID).Scan(
		&out.Activity.ID, &out.Activity.SemesterID, &out.Activity.ClassID, &out.Activity.SubjectID, &out.Activity.TeacherID,
		&out.Activity.Name, &out.Activity.Type, &out.Activity.Date, &out.Activity.CreatedBy, &yearID)
	if errors.Is(err, sql.ErrNoRows) {
		return AssessmentScoreSheet{}, ErrNotFound
	}
	if err != nil {
		return AssessmentScoreSheet{}, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, out.Activity.SemesterID, out.Activity.ClassID, out.Activity.SubjectID)
	if err != nil {
		return AssessmentScoreSheet{}, err
	}
	if !allowed {
		return AssessmentScoreSheet{}, ErrForbidden
	}
	out.CanRecord = teacherID == out.Activity.TeacherID
	if err = tx.QueryRowContext(ctx, `SELECT y.name || ' · Semester ' || sm.number::text,
		c.grade::text || c.name,s.name FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id
		JOIN app.classes c ON c.id=$2 AND c.school_year_id=sm.school_year_id JOIN app.subjects s ON s.id=$3
		WHERE sm.id=$1`, out.Activity.SemesterID, out.Activity.ClassID, out.Activity.SubjectID).Scan(&out.Activity.SemesterName, &out.Activity.ClassName, &out.Activity.SubjectName); err != nil {
		return AssessmentScoreSheet{}, err
	}
	out.Activity.Objectives = make([]LearningObjective, 0)
	rows, err := tx.QueryContext(ctx, `SELECT lo.id,lo.definition FROM app.assessment_activity_objectives ao
		JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id WHERE ao.activity_id=$1 ORDER BY lo.id`, activityID)
	if err != nil {
		return AssessmentScoreSheet{}, err
	}
	for rows.Next() {
		var objective LearningObjective
		if err = rows.Scan(&objective.ID, &objective.Definition); err != nil {
			rows.Close()
			return AssessmentScoreSheet{}, err
		}
		objective.SemesterID, objective.ClassID, objective.SubjectID = out.Activity.SemesterID, out.Activity.ClassID, out.Activity.SubjectID
		out.Activity.Objectives = append(out.Activity.Objectives, objective)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return AssessmentScoreSheet{}, err
	}
	if err = rows.Close(); err != nil {
		return AssessmentScoreSheet{}, err
	}
	if len(out.Activity.Objectives) == 0 {
		return AssessmentScoreSheet{}, ErrNotFound
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.student_placements p
		WHERE p.school_year_id=$1 AND p.starts_on IS NULL
		AND app.student_placement_class_at(p.id,$3::date)=$2`, yearID, out.Activity.ClassID, out.Activity.Date).Scan(&out.UnknownPlacementCount); err != nil {
		return AssessmentScoreSheet{}, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT p.student_id,s.name,coalesce(s.nis,''),lo.id,sc.id,
		CASE WHEN sc.id IS NULL THEN NULL ELSE greatest(coalesce(latest.after_score,sc.score),coalesce(remedial.best_score,coalesce(latest.after_score,sc.score)))::text END,sc.recorded_by,
		(EXISTS (SELECT 1 FROM app.historical_assessment_objective_links h WHERE h.activity_id=ao.activity_id AND h.learning_objective_id=ao.learning_objective_id)
		 OR EXISTS (SELECT 1 FROM app.historical_assessment_activity_objectives hdo JOIN app.historical_assessment_activity_insertions ha ON ha.id=hdo.audit_id WHERE ha.activity_id=ao.activity_id AND hdo.learning_objective_id=ao.learning_objective_id))
		FROM app.student_placements p JOIN app.students s ON s.id=p.student_id
		CROSS JOIN app.assessment_activity_objectives ao
		JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id
		LEFT JOIN app.assessment_activity_initial_scores sc ON sc.assessment_activity_id=ao.activity_id
			AND sc.learning_objective_id=ao.learning_objective_id AND sc.student_id=p.student_id
		LEFT JOIN LATERAL (SELECT c.after_score FROM app.assessment_activity_score_corrections c
			WHERE c.initial_score_id=sc.id ORDER BY c.version DESC LIMIT 1) latest ON true
		LEFT JOIN LATERAL (SELECT max(r.score) AS best_score FROM app.assessment_activity_score_remedials r
			WHERE r.initial_score_id=sc.id) remedial ON true
		WHERE ao.activity_id=$1 AND p.school_year_id=$2 AND p.starts_on IS NOT NULL AND p.starts_on<=$4::date
		AND app.student_placement_class_at(p.id,$4::date)=$3
		ORDER BY s.name,p.student_id,lo.id`, activityID, yearID, out.Activity.ClassID, out.Activity.Date)
	if err != nil {
		return AssessmentScoreSheet{}, err
	}
	defer rows.Close()
	byStudent := make(map[int64]int)
	for rows.Next() {
		var studentID, objectiveID int64
		var name, nis string
		var scoreID, recordedBy sql.NullInt64
		var score sql.NullString
		var historicalLocked bool
		if err = rows.Scan(&studentID, &name, &nis, &objectiveID, &scoreID, &score, &recordedBy, &historicalLocked); err != nil {
			return AssessmentScoreSheet{}, err
		}
		idx, ok := byStudent[studentID]
		if !ok {
			idx = len(out.Students)
			byStudent[studentID] = idx
			out.Students = append(out.Students, AssessmentScoreStudent{ID: studentID, Name: name, NIS: nis})
		}
		cell := AssessmentScoreCell{ObjectiveID: objectiveID, HistoricalLocked: historicalLocked}
		if scoreID.Valid {
			cell.InitialScoreID = scoreID.Int64
			cell.CanCorrect = out.Activity.TeacherID == teacherID && recordedBy.Valid && recordedBy.Int64 == actor
			cell.CanRemediate = out.Activity.Type == "sumatif" && out.Activity.TeacherID == teacherID && recordedBy.Valid && recordedBy.Int64 == actor
		}
		if score.Valid {
			cell.Value, cell.Recorded = score.String, true
		} else if out.CanRecord && !historicalLocked {
			out.HasBlankScores = true
		}
		out.Students[idx].Scores = append(out.Students[idx].Scores, cell)
	}
	return out, rows.Err()
}

// SaveAssessmentActivityInitialScores menyimpan semua nilai nonkosong tanpa menimpa nilai awal.
func (s *Store) SaveAssessmentActivityInitialScores(ctx context.Context, actor, activityID int64, inputs []AssessmentScoreInput) error {
	if activityID < 1 {
		return ValidationError("Kegiatan penilaian tidak valid.")
	}
	values := make([]string, 0, len(inputs))
	students := make([]int64, 0, len(inputs))
	objectives := make([]int64, 0, len(inputs))
	seen := make(map[[2]int64]bool)
	for _, input := range inputs {
		if input.StudentID < 1 || input.ObjectiveID < 1 {
			return ValidationError("Siswa atau Tujuan Pembelajaran tidak valid.")
		}
		key := [2]int64{input.StudentID, input.ObjectiveID}
		if seen[key] {
			return ValidationError("Pasangan siswa dan TP dikirim lebih dari sekali.")
		}
		seen[key] = true
		parsed, present, err := parseInitialScore(input.Value)
		if err != nil {
			return err
		}
		if present {
			students = append(students, input.StudentID)
			objectives = append(objectives, input.ObjectiveID)
			values = append(values, formatScore(parsed))
		}
	}
	if len(values) == 0 {
		return ValidationError("Isi sedikitnya satu nilai sebelum menyimpan.")
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
	var activity AssessmentActivity
	err = tx.QueryRowContext(ctx, `SELECT semester_id,class_id,subject_id,teacher_id
		FROM app.assessment_activities WHERE id=$1`, activityID).Scan(
		&activity.SemesterID, &activity.ClassID, &activity.SubjectID, &activity.TeacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, activity.SemesterID, activity.ClassID, activity.SubjectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	if teacherID != activity.TeacherID {
		return ErrForbidden
	}
	// Lock konteks dan penulisan ada dalam satu operasi terbatas pada transaksi ini.
	if _, err = tx.ExecContext(ctx, `SELECT academic_write.save_legacy_initial_scores($1,$2,$3,$4,$5)`,
		actor, activityID, students, objectives, values); err != nil {
		return academicWriterError(dbError(err))
	}
	return tx.Commit()
}

type parsedInitialScore struct {
	studentID, objectiveID int64
	cents                  int64
}

func parseInitialScore(raw string) (int64, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	for _, r := range raw {
		if unicode.IsSpace(r) {
			return 0, false, ValidationError("Nilai harus berupa angka 0–100 dengan maksimal dua angka desimal.")
		}
	}
	if strings.Count(raw, ",") > 1 || strings.Count(raw, ".") > 1 || (strings.Contains(raw, ",") && strings.Contains(raw, ".")) {
		return 0, false, ValidationError("Nilai harus berupa angka 0–100 dengan maksimal dua angka desimal.")
	}
	canonical := strings.ReplaceAll(raw, ",", ".")
	parts := strings.Split(canonical, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && (parts[1] == "" || len(parts[1]) > 2)) {
		return 0, false, ValidationError("Nilai harus berupa angka 0–100 dengan maksimal dua angka desimal.")
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return 0, false, ValidationError("Nilai harus berupa angka 0–100 dengan maksimal dua angka desimal.")
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		for _, r := range fraction {
			if r < '0' || r > '9' {
				return 0, false, ValidationError("Nilai harus berupa angka 0–100 dengan maksimal dua angka desimal.")
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > 100 {
		return 0, false, ValidationError("Nilai harus berada pada rentang 0–100.")
	}
	if len(fraction) == 1 {
		fraction += "0"
	}
	if fraction == "" {
		fraction = "00"
	}
	decimal, _ := strconv.ParseInt(fraction, 10, 64)
	cents := whole*100 + decimal
	if cents > 10000 {
		return 0, false, ValidationError("Nilai harus berada pada rentang 0–100.")
	}
	return cents, true, nil
}

func formatScore(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }
