package academic

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// AssessmentTPRecap adalah rekap baca-saja untuk satu konteks penugasan.
type AssessmentTPRecap struct {
	Context  LearningObjectiveContext
	Students []AssessmentTPRecapStudent
}

type AssessmentTPRecapStudent struct {
	ID, NIS    string
	Name       string
	Objectives []AssessmentTPRecapObjective
	NH         string
	NHComplete bool
	Missing    []string
}

type AssessmentTPRecapObjective struct {
	ID, Definition string
	Activities     []AssessmentTPRecapActivity
	RequiredCount  int
	ScoredCount    int
	Average        string
	Complete       bool
	Missing        []string
}

type AssessmentTPRecapActivity struct {
	ID, Name, Date string
	EffectiveValue string
	HasValue       bool
}

// AssessmentTPRecap membaca satu snapshot konsisten. Kewajiban kegiatan
// ditentukan dari kelas siswa pada tanggal kegiatan, bukan dari penempatan
// hari ini atau dari nilai kelas asal.
func (s *Store) AssessmentTPRecap(ctx context.Context, actor, semesterID, classID, subjectID int64) (AssessmentTPRecap, error) {
	if semesterID < 1 || classID < 1 || subjectID < 1 {
		return AssessmentTPRecap{}, ValidationError("Pilih konteks semester, kelas, dan mata pelajaran.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return AssessmentTPRecap{}, err
	}
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, semesterID, classID, subjectID)
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	if !allowed {
		return AssessmentTPRecap{}, ErrForbidden
	}
	var out AssessmentTPRecap
	var schoolYearID int64
	err = tx.QueryRowContext(ctx, `
		SELECT sm.school_year_id, y.name || ' · Semester ' || sm.number::text,
		       c.grade::text || c.name, s.name
		FROM app.semesters sm
		JOIN app.school_years y ON y.id=sm.school_year_id
		JOIN app.classes c ON c.id=$2 AND c.school_year_id=sm.school_year_id
		JOIN app.subjects s ON s.id=$3 AND s.active
		JOIN app.semester_calendars cal ON cal.semester_id=sm.id
		WHERE sm.id=$1`, semesterID, classID, subjectID).Scan(
		&schoolYearID, &out.Context.SemesterName, &out.Context.ClassName, &out.Context.SubjectName)
	if errors.Is(err, sql.ErrNoRows) {
		return AssessmentTPRecap{}, ErrNotFound
	}
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	out.Context.SemesterID, out.Context.ClassID, out.Context.SubjectID = semesterID, classID, subjectID

	rows, err := tx.QueryContext(ctx, `
		WITH calendar AS (
			SELECT starts_on, ends_on FROM app.semester_calendars WHERE semester_id=$1
		), candidates AS (
			SELECT p.id AS placement_id, p.student_id,
			       greatest(p.starts_on, cal.starts_on) AS on_date
			FROM app.student_placements p
			JOIN calendar cal ON p.school_year_id=$2
			WHERE p.starts_on IS NOT NULL AND p.starts_on<=cal.ends_on
			UNION
			SELECT p.id, p.student_id, m.effective_date
			FROM app.student_placements p
			JOIN app.student_class_moves m ON m.placement_id=p.id
			JOIN calendar cal ON p.school_year_id=$2
			WHERE p.starts_on IS NOT NULL AND m.effective_date BETWEEN cal.starts_on AND cal.ends_on
			UNION
			SELECT p.id, p.student_id, r.effective_date
			FROM app.student_placements p
			JOIN app.student_school_returns r ON r.placement_id=p.id
			JOIN calendar cal ON p.school_year_id=$2
			WHERE p.starts_on IS NOT NULL AND r.effective_date BETWEEN cal.starts_on AND cal.ends_on
		), class_students AS (
			SELECT DISTINCT p.student_id, st.name, coalesce(st.nis,'') AS nis
			FROM candidates cand
			JOIN app.student_placements p ON p.id=cand.placement_id
			JOIN app.students st ON st.id=p.student_id
			CROSS JOIN calendar cal
			WHERE cand.on_date BETWEEN cal.starts_on AND cal.ends_on
			  AND app.student_placement_class_at(cand.placement_id,cand.on_date)=$3
		)
		SELECT st.student_id, st.name, st.nis,
		       lo.id, lo.definition,
		       a.id, a.name, a.activity_date::text,
		       CASE WHEN a.id IS NULL THEN false ELSE EXISTS (
		         SELECT 1 FROM app.student_placements p
		         WHERE p.student_id=st.student_id AND p.school_year_id=$2
		           AND p.starts_on IS NOT NULL AND p.starts_on<=a.activity_date
		           AND app.student_placement_class_at(p.id,a.activity_date)=$3
		       ) END AS applicable,
		       CASE WHEN sc.id IS NULL THEN NULL ELSE
		           greatest(coalesce(latest.after_score,sc.score),coalesce(remedial.best_score,sc.score))::text
		       END AS effective_value
		FROM class_students st
		CROSS JOIN app.learning_objectives lo
		CROSS JOIN calendar cal
		LEFT JOIN app.assessment_activity_objectives ao
		  ON ao.learning_objective_id=lo.id
		LEFT JOIN app.assessment_activities a
		  ON a.id=ao.activity_id AND a.activity_type='sumatif'
		 AND a.activity_date BETWEEN cal.starts_on AND cal.ends_on
		LEFT JOIN app.assessment_activity_initial_scores sc
		  ON sc.assessment_activity_id=a.id AND sc.learning_objective_id=lo.id
		 AND sc.student_id=st.student_id
		 AND EXISTS (
		   SELECT 1 FROM app.student_placements p
		   WHERE p.student_id=st.student_id AND p.school_year_id=$2
		     AND p.starts_on IS NOT NULL AND p.starts_on<=a.activity_date
		     AND app.student_placement_class_at(p.id,a.activity_date)=$3
		 )
		LEFT JOIN LATERAL (
			SELECT c.after_score FROM app.assessment_activity_score_corrections c
			WHERE c.initial_score_id=sc.id ORDER BY c.version DESC LIMIT 1
		) latest ON true
		LEFT JOIN LATERAL (
			SELECT max(r.score) AS best_score FROM app.assessment_activity_score_remedials r
			WHERE r.initial_score_id=sc.id
		) remedial ON true
		WHERE lo.school_year_id=$2 AND lo.semester_id=$1 AND lo.class_id=$3 AND lo.subject_id=$4
		ORDER BY st.name,st.student_id,lo.id,a.activity_date,a.id`,
		semesterID, schoolYearID, classID, subjectID)
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	defer rows.Close()
	studentIndex := make(map[int64]int)
	for rows.Next() {
		var studentID, objectiveID int64
		var studentName, nis, definition string
		var activityID, activityName, activityDate, effective sql.NullString
		var applicable bool
		if err = rows.Scan(&studentID, &studentName, &nis, &objectiveID, &definition,
			&activityID, &activityName, &activityDate, &applicable, &effective); err != nil {
			return AssessmentTPRecap{}, err
		}
		idx, exists := studentIndex[studentID]
		if !exists {
			idx = len(out.Students)
			studentIndex[studentID] = idx
			out.Students = append(out.Students, AssessmentTPRecapStudent{ID: fmtID(studentID), NIS: nis, Name: studentName})
		}
		student := &out.Students[idx]
		objectiveIdx := len(student.Objectives) - 1
		if objectiveIdx < 0 || student.Objectives[objectiveIdx].ID != fmtID(objectiveID) {
			student.Objectives = append(student.Objectives, AssessmentTPRecapObjective{ID: fmtID(objectiveID), Definition: definition})
			objectiveIdx = len(student.Objectives) - 1
		}
		objective := &student.Objectives[objectiveIdx]
		if !activityID.Valid {
			continue
		}
		// Kegiatan yang tidak berlaku pada tanggal penempatan siswa tidak
		// menjadi kewajiban; query menyisakannya tanpa nilai terkait.
		if !applicable {
			continue
		}
		activity := AssessmentTPRecapActivity{ID: activityID.String, Name: activityName.String, Date: activityDate.String, HasValue: effective.Valid}
		if effective.Valid {
			activity.EffectiveValue = strings.ReplaceAll(effective.String, ".", ",")
			objective.ScoredCount++
		} else {
			objective.Missing = append(objective.Missing, activityName.String+" ("+activityDate.String+")")
		}
		objective.RequiredCount++
		objective.Activities = append(objective.Activities, activity)
	}
	if err = rows.Err(); err != nil {
		return AssessmentTPRecap{}, err
	}
	if err = rows.Close(); err != nil {
		return AssessmentTPRecap{}, err
	}
	for i := range out.Students {
		student := &out.Students[i]
		values := make([]*big.Rat, 0, len(student.Objectives))
		for j := range student.Objectives {
			objective := &student.Objectives[j]
			if objective.RequiredCount == 0 {
				objective.Missing = append(objective.Missing, "Belum ada kegiatan Sumatif yang menjadi kewajiban siswa pada TP ini.")
				continue
			}
			if objective.ScoredCount != objective.RequiredCount {
				continue
			}
			average, averageErr := exactAverage(objective.Activities)
			if averageErr != nil {
				return AssessmentTPRecap{}, averageErr
			}
			objective.Average = strings.ReplaceAll(average.FloatString(2), ".", ",")
			objective.Complete = true
			values = append(values, average)
		}
		if len(student.Objectives) > 0 && len(values) == len(student.Objectives) {
			nh := new(big.Rat)
			for _, value := range values {
				nh.Add(nh, value)
			}
			nh.Quo(nh, new(big.Rat).SetInt64(int64(len(values))))
			student.NH, student.NHComplete = strings.ReplaceAll(nh.FloatString(2), ".", ","), true
		} else {
			for _, objective := range student.Objectives {
				if !objective.Complete {
					student.Missing = append(student.Missing, objective.Definition)
				}
			}
			sort.Strings(student.Missing)
		}
	}
	return out, nil
}

// exactAverage menjaga presisi rata-rata TP sampai perhitungan NH selesai.
func exactAverage(activities []AssessmentTPRecapActivity) (*big.Rat, error) {
	total := new(big.Rat)
	count := int64(0)
	for _, activity := range activities {
		if !activity.HasValue {
			continue
		}
		value, ok := new(big.Rat).SetString(strings.ReplaceAll(activity.EffectiveValue, ",", "."))
		if !ok {
			return nil, errors.New("nilai efektif Sumatif bukan angka desimal valid")
		}
		total.Add(total, value)
		count++
	}
	if count == 0 {
		return nil, errors.New("rata-rata TP memerlukan sedikitnya satu nilai Sumatif")
	}
	return total.Quo(total, new(big.Rat).SetInt64(count)), nil
}

func fmtID(id int64) string { return strconv.FormatInt(id, 10) }
