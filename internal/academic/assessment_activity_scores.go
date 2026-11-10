package academic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
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
	ObjectiveID    int64
	InitialScoreID int64
	Value          string
	Recorded       bool
	CanCorrect     bool
	CanRemediate   bool
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
		CASE WHEN sc.id IS NULL THEN NULL ELSE greatest(coalesce(latest.after_score,sc.score),coalesce(remedial.best_score,coalesce(latest.after_score,sc.score)))::text END,sc.recorded_by
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
		if err = rows.Scan(&studentID, &name, &nis, &objectiveID, &scoreID, &score, &recordedBy); err != nil {
			return AssessmentScoreSheet{}, err
		}
		idx, ok := byStudent[studentID]
		if !ok {
			idx = len(out.Students)
			byStudent[studentID] = idx
			out.Students = append(out.Students, AssessmentScoreStudent{ID: studentID, Name: name, NIS: nis})
		}
		cell := AssessmentScoreCell{ObjectiveID: objectiveID}
		if scoreID.Valid {
			cell.InitialScoreID = scoreID.Int64
			cell.CanCorrect = out.Activity.TeacherID == teacherID && recordedBy.Valid && recordedBy.Int64 == actor
			cell.CanRemediate = out.Activity.Type == "sumatif" && out.Activity.TeacherID == teacherID && recordedBy.Valid && recordedBy.Int64 == actor
		}
		if score.Valid {
			cell.Value, cell.Recorded = score.String, true
		} else if out.CanRecord {
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
	values := make([]parsedInitialScore, 0, len(inputs))
	seen := make(map[[2]int64]bool)
	studentSet, objectiveSet := make(map[int64]bool), make(map[int64]bool)
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
		if !present {
			continue
		}
		values = append(values, parsedInitialScore{studentID: input.StudentID, objectiveID: input.ObjectiveID, cents: parsed})
		studentSet[input.StudentID], objectiveSet[input.ObjectiveID] = true, true
	}
	if len(values) == 0 {
		return ValidationError("Isi sedikitnya satu nilai sebelum menyimpan.")
	}
	students, objectives := sortedKeys(studentSet), sortedKeys(objectiveSet)
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
	var yearID int64
	err = tx.QueryRowContext(ctx, `SELECT id,school_year_id,semester_id,class_id,subject_id,teacher_id,activity_date::text
		FROM app.assessment_activities WHERE id=$1 FOR UPDATE`, activityID).Scan(
		&activity.ID, &yearID, &activity.SemesterID, &activity.ClassID, &activity.SubjectID, &activity.TeacherID, &activity.Date)
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
	rows, err := tx.QueryContext(ctx, `SELECT learning_objective_id FROM app.assessment_activity_objectives
		WHERE activity_id=$1 AND learning_objective_id=ANY($2::bigint[]) ORDER BY learning_objective_id FOR SHARE`, activityID, objectives)
	if err != nil {
		return err
	}
	matched := make(map[int64]bool, len(objectives))
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		matched[id] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(matched) != len(objectives) {
		return ErrForbidden
	}
	placements := make(map[int64]int64, len(students))
	rows, err = tx.QueryContext(ctx, `SELECT p.student_id,p.id
		FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=ANY($2::bigint[])
		AND p.starts_on IS NOT NULL AND p.starts_on<=$4::date
		AND app.student_placement_class_at(p.id,$4::date)=$3 ORDER BY p.student_id FOR SHARE`, yearID, students, activity.ClassID, activity.Date)
	if err != nil {
		return err
	}
	for rows.Next() {
		var studentID, placementID int64
		if err = rows.Scan(&studentID, &placementID); err != nil {
			rows.Close()
			return err
		}
		if _, exists := placements[studentID]; exists {
			rows.Close()
			return ValidationError("Riwayat penempatan siswa tidak tunggal pada tanggal kegiatan.")
		}
		placements[studentID] = placementID
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(placements) != len(students) {
		return ErrForbidden
	}
	for _, value := range values {
		placementID := placements[value.studentID]
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO app.assessment_activity_initial_scores
			(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by)
			SELECT a.id,ao.learning_objective_id,a.school_year_id,a.semester_id,a.class_id,a.subject_id,$3,$4,a.activity_date,$5::numeric,$6
			FROM app.assessment_activities a JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id
				AND ao.learning_objective_id=$2
			JOIN app.student_placements p ON p.id=$4 AND p.student_id=$3 AND p.school_year_id=a.school_year_id
			WHERE a.id=$1 AND app.student_placement_class_at(p.id,a.activity_date)=a.class_id
			ON CONFLICT (assessment_activity_id,learning_objective_id,student_id) DO NOTHING`,
			activityID, value.objectiveID, value.studentID, placementID, formatScore(value.cents), actor)
		if insertErr != nil {
			return dbError(insertErr)
		}
		inserted, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return affectedErr
		}
		if inserted == 1 {
			continue
		}
		var existing int64
		if err = tx.QueryRowContext(ctx, `SELECT (score*100)::bigint FROM app.assessment_activity_initial_scores
			WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3 FOR SHARE`,
			activityID, value.objectiveID, value.studentID).Scan(&existing); errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		} else if err != nil {
			return err
		}
		if existing != value.cents {
			return ErrConflict
		}
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

func sortedKeys(values map[int64]bool) []int64 {
	out := make([]int64, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
