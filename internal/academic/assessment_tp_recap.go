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
	Context                    LearningObjectiveContext
	Students                   []AssessmentTPRecapStudent
	UnknownPlacementCount      int
	OutOfCalendarActivityCount int
}

type AssessmentTPRecapStudent struct {
	ID, NIS            string
	Name               string
	Objectives         []AssessmentTPRecapObjective
	NH                 string
	NHComplete         bool
	NHExact            string
	PlacementUncertain bool
	Missing            []string
}

type AssessmentTPRecapObjective struct {
	ID, Definition                          string
	Activities                              []AssessmentTPRecapActivity
	RequiredCount                           int
	ScoredCount                             int
	Average, AverageExact                   string
	Threshold, ThresholdSource, Achievement string
	Complete                                bool
	HistoricalApplicability                 string `json:",omitempty"`
	Missing                                 []string
}

type AssessmentTPRecapActivity struct {
	ID, Name, Date string
	EffectiveValue string
	HasValue       bool
	OutOfCalendar  bool
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
	return s.directTPRecapForStudent(ctx, tx, actor, semesterID, classID, subjectID, 0, 0)
}

func (s *Store) assessmentTPRecap(ctx context.Context, tx *sql.Tx, actor, semesterID, classID, subjectID int64, includeEmptyRoster bool) (AssessmentTPRecap, error) {
	return s.assessmentTPRecapForStudent(ctx, tx, actor, semesterID, classID, subjectID, includeEmptyRoster, 0)
}

// studentID membatasi pembacaan historis hanya pada pemilik Final yang sah.
func (s *Store) assessmentTPRecapForStudent(ctx context.Context, tx *sql.Tx, actor, semesterID, classID, subjectID int64, includeEmptyRoster bool, studentID int64) (AssessmentTPRecap, error) {
	return s.assessmentTPRecapForStudentMode(ctx, tx, actor, semesterID, classID, subjectID, includeEmptyRoster, studentID, 0)
}

// assessmentTPRecapForHistoricalScore hanya dipakai pratinjau pencatatan
// historis setelah pemilik sumber dan penugasan pada tanggal kegiatan telah
// diverifikasi oleh service CP10-23.
func (s *Store) assessmentTPRecapForHistoricalScore(ctx context.Context, tx *sql.Tx, actor, semesterID, classID, subjectID, studentID, activityID int64) (AssessmentTPRecap, error) {
	return s.assessmentTPRecapForStudentMode(ctx, tx, actor, semesterID, classID, subjectID, true, studentID, activityID)
}

func (s *Store) assessmentTPRecapForStudentMode(ctx context.Context, tx *sql.Tx, actor, semesterID, classID, subjectID int64, includeEmptyRoster bool, studentID, historicalActivityID int64) (AssessmentTPRecap, error) {
	var err error
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return AssessmentTPRecap{}, err
	}
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	allowed := false
	if historicalActivityID > 0 && studentID > 0 {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM app.assessment_activities a
			WHERE a.id=$1 AND a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4
			AND a.teacher_id=$5 AND a.activity_type='sumatif'
			AND EXISTS(SELECT 1 FROM app.teaching_assignments ta WHERE ta.teacher_id=a.teacher_id
				AND ta.semester_id=a.semester_id AND ta.class_id=a.class_id AND ta.subject_id=a.subject_id
				AND ta.effective_from<=a.activity_date AND (ta.effective_until IS NULL OR ta.effective_until>a.activity_date))
			AND (EXISTS(SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=a.id AND h.teacher_id=a.teacher_id)
				OR EXISTS(SELECT 1 FROM app.historical_assessment_objective_links l WHERE l.activity_id=a.id AND l.teacher_id=a.teacher_id))
		)`, historicalActivityID, semesterID, classID, subjectID, teacherID).Scan(&allowed)
		if err != nil {
			return AssessmentTPRecap{}, err
		}
	} else {
		allowed, err = learningObjectiveAssignment(ctx, tx, teacherID, semesterID, classID, subjectID)
		if err != nil {
			return AssessmentTPRecap{}, err
		}
	}
	if studentID > 0 && historicalActivityID == 0 {
		allowed, err = semesterFinalOwned(ctx, tx, teacherID, semesterID, classID, subjectID, studentID)
		if err != nil {
			return AssessmentTPRecap{}, err
		}
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
		JOIN app.subjects s ON s.id=$3 AND ($4::boolean OR s.active)
		JOIN app.semester_calendars cal ON cal.semester_id=sm.id
		WHERE sm.id=$1`, semesterID, classID, subjectID, studentID > 0).Scan(
		&schoolYearID, &out.Context.SemesterName, &out.Context.ClassName, &out.Context.SubjectName)
	if errors.Is(err, sql.ErrNoRows) {
		return AssessmentTPRecap{}, ErrNotFound
	}
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	out.Context.SemesterID, out.Context.ClassID, out.Context.SubjectID = semesterID, classID, subjectID
	if err = tx.QueryRowContext(ctx, `
		SELECT count(DISTINCT a.id)
		FROM app.assessment_activities a
		JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id
		JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id
		JOIN app.semester_calendars cal ON cal.semester_id=a.semester_id
		WHERE a.semester_id=$1 AND a.class_id=$2 AND a.subject_id=$3
		  AND a.activity_type='sumatif'
		  AND (a.activity_date<cal.starts_on OR a.activity_date>cal.ends_on)
		  AND lo.school_year_id=$4 AND lo.semester_id=$1 AND lo.class_id=$2 AND lo.subject_id=$3`,
		semesterID, classID, subjectID, schoolYearID).Scan(&out.OutOfCalendarActivityCount); err != nil {
		return AssessmentTPRecap{}, err
	}
	if err = tx.QueryRowContext(ctx, `
		WITH calendar AS (
			SELECT starts_on,ends_on FROM app.semester_calendars WHERE semester_id=$1
		), context_activities AS (
			SELECT DISTINCT a.activity_date
			FROM app.assessment_activities a
			JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id
			JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id
			WHERE a.semester_id=$1 AND a.class_id=$3 AND a.subject_id=$4 AND a.activity_type='sumatif'
			  AND lo.school_year_id=$2 AND lo.semester_id=$1 AND lo.class_id=$3 AND lo.subject_id=$4
		), relevant_dates AS (
			SELECT starts_on AS on_date FROM calendar
			UNION SELECT ends_on FROM calendar
			UNION
			SELECT m.effective_date FROM app.student_class_moves m
			JOIN app.student_placements p ON p.id=m.placement_id
			JOIN calendar cal ON p.school_year_id=$2
			WHERE m.effective_date BETWEEN cal.starts_on AND cal.ends_on
			UNION
			SELECT r.effective_date FROM app.student_school_returns r
			JOIN app.student_placements p ON p.id=r.placement_id
			JOIN calendar cal ON p.school_year_id=$2
			WHERE r.effective_date BETWEEN cal.starts_on AND cal.ends_on
			UNION SELECT activity_date FROM context_activities
		)
		SELECT count(*) FROM app.student_placements p
		WHERE p.school_year_id=$2 AND p.starts_on IS NULL
		  AND EXISTS (
			SELECT 1 FROM relevant_dates d
			WHERE app.student_placement_class_at(p.id,d.on_date)=$3
		  )`, semesterID, schoolYearID, classID, subjectID).Scan(&out.UnknownPlacementCount); err != nil {
		return AssessmentTPRecap{}, err
	}

	// Kriteria dan nilai dibaca pada snapshot yang sama agar indikator konsisten.
	criteria := map[int64]string{}
	var defaultThreshold string
	criteriaRows, err := tx.QueryContext(ctx, `SELECT coalesce(objective_id,0),threshold::text FROM app.tp_achievement_criteria WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND threshold IS NOT NULL`, semesterID, classID, subjectID)
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	for criteriaRows.Next() {
		var id int64
		var threshold string
		if err = criteriaRows.Scan(&id, &threshold); err != nil {
			criteriaRows.Close()
			return AssessmentTPRecap{}, err
		}
		if id == 0 {
			defaultThreshold = threshold
		} else {
			criteria[id] = threshold
		}
	}
	if err = criteriaRows.Err(); err != nil {
		criteriaRows.Close()
		return AssessmentTPRecap{}, err
	}
	criteriaRows.Close()
	rows, err := tx.QueryContext(ctx, `
		WITH calendar AS (
			SELECT starts_on, ends_on FROM app.semester_calendars WHERE semester_id=$1
		), context_activities AS (
			SELECT DISTINCT a.id, a.activity_date
			FROM app.assessment_activities a
			JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id
			JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id
			WHERE a.semester_id=$1 AND a.class_id=$3 AND a.subject_id=$4 AND a.activity_type='sumatif'
			  AND lo.school_year_id=$2 AND lo.semester_id=$1 AND lo.class_id=$3 AND lo.subject_id=$4
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
			UNION
			SELECT p.id, p.student_id, a.activity_date
			FROM app.student_placements p
			CROSS JOIN context_activities a
			WHERE p.school_year_id=$2 AND p.starts_on IS NOT NULL AND p.starts_on<=a.activity_date
		), class_students AS (
			SELECT DISTINCT p.student_id, st.name, coalesce(st.nis,'') AS nis
			FROM candidates cand
			JOIN app.student_placements p ON p.id=cand.placement_id
			JOIN app.students st ON st.id=p.student_id
			WHERE app.student_placement_class_at(cand.placement_id,cand.on_date)=$3 AND ($5::bigint=0 OR p.student_id=$5)
			UNION
			-- Pembacaan Final historis dibatasi kepemilikan di atas; Final lama
			-- tetap dapat dibaca meskipun penempatannya tidak dapat dipastikan.
			SELECT st.id,st.name,coalesce(st.nis,'') FROM app.students st
			JOIN app.semester_final_versions f ON f.student_id=st.id
			WHERE $5::bigint>0 AND st.id=$5 AND f.semester_id=$1 AND f.class_id=$3 AND f.subject_id=$4
			  AND EXISTS (SELECT 1 FROM app.historical_learning_objective_insertions h WHERE h.semester_id=$1 AND h.class_id=$3 AND h.subject_id=$4)
		)
		SELECT st.student_id, st.name, st.nis,
		       lo.id, lo.definition,
		       CASE WHEN h.learning_objective_id IS NULL THEN ''
		         WHEN EXISTS (SELECT 1 FROM app.student_placements p WHERE p.school_year_id=$2 AND p.student_id=st.student_id AND p.starts_on IS NULL) THEN 'belum pasti'
		         WHEN (SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$2 AND p.student_id=st.student_id AND p.starts_on IS NOT NULL AND p.starts_on<=h.assessment_date AND app.student_placement_class_at(p.id,h.assessment_date) IS NOT NULL)>1 THEN 'belum pasti'
		         WHEN EXISTS (SELECT 1 FROM app.student_placements p WHERE p.school_year_id=$2 AND p.student_id=st.student_id AND p.starts_on IS NOT NULL AND p.starts_on<=h.assessment_date AND app.student_placement_class_at(p.id,h.assessment_date)=$3) THEN 'anggota'
		         WHEN EXISTS (SELECT 1 FROM app.student_placements p WHERE p.school_year_id=$2 AND p.student_id=st.student_id) THEN 'bukan anggota'
		         ELSE 'belum pasti' END AS historical_applicability,
		       a.id, a.name, a.activity_date::text,
		       CASE WHEN a.id IS NULL THEN false ELSE a.activity_date<cal.starts_on OR a.activity_date>cal.ends_on END AS out_of_calendar,
		       CASE WHEN a.id IS NULL THEN false ELSE EXISTS (
		         SELECT 1 FROM app.student_placements p
		         WHERE p.student_id=st.student_id AND p.school_year_id=$2
		           AND p.starts_on IS NOT NULL AND p.starts_on<=a.activity_date
		           AND app.student_placement_class_at(p.id,a.activity_date)=$3
		       ) END AS applicable,
		       CASE WHEN sc.id IS NULL THEN NULL ELSE
		           greatest(coalesce(latest.after_score,sc.score),coalesce(remedial.best_score,coalesce(latest.after_score,sc.score)))::text
		       END AS effective_value
		FROM class_students st
		LEFT JOIN app.learning_objectives lo ON lo.school_year_id=$2 AND lo.semester_id=$1 AND lo.class_id=$3 AND lo.subject_id=$4
		CROSS JOIN calendar cal
		LEFT JOIN app.historical_learning_objective_insertions h ON h.learning_objective_id=lo.id
		LEFT JOIN app.assessment_activity_objectives ao
		  ON ao.learning_objective_id=lo.id
		LEFT JOIN app.assessment_activities a
		  ON a.id=ao.activity_id AND a.activity_type='sumatif'
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
		ORDER BY st.name,st.student_id,lo.id,a.activity_date,a.id`,
		semesterID, schoolYearID, classID, subjectID, studentID)
	if err != nil {
		return AssessmentTPRecap{}, err
	}
	defer rows.Close()
	studentIndex := make(map[int64]int)
	for rows.Next() {
		var studentID int64
		var objectiveID sql.NullInt64
		var studentName, nis string
		var definition sql.NullString
		var historicalApplicability string
		var activityID, activityName, activityDate, effective sql.NullString
		var outOfCalendar, applicable bool
		if err = rows.Scan(&studentID, &studentName, &nis, &objectiveID, &definition, &historicalApplicability,
			&activityID, &activityName, &activityDate, &outOfCalendar, &applicable, &effective); err != nil {
			return AssessmentTPRecap{}, err
		}
		if !objectiveID.Valid && !includeEmptyRoster {
			continue
		}
		idx, exists := studentIndex[studentID]
		if !exists {
			idx = len(out.Students)
			studentIndex[studentID] = idx
			out.Students = append(out.Students, AssessmentTPRecapStudent{ID: fmtID(studentID), NIS: nis, Name: studentName})
		}
		student := &out.Students[idx]
		if !objectiveID.Valid {
			continue
		}
		objectiveIdx := len(student.Objectives) - 1
		if objectiveIdx < 0 || student.Objectives[objectiveIdx].ID != fmtID(objectiveID.Int64) {
			student.Objectives = append(student.Objectives, AssessmentTPRecapObjective{ID: fmtID(objectiveID.Int64), Definition: definition.String, HistoricalApplicability: historicalApplicability})
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
		activity := AssessmentTPRecapActivity{ID: activityID.String, Name: activityName.String, Date: activityDate.String, HasValue: effective.Valid, OutOfCalendar: outOfCalendar}
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
		if err := finishAssessmentTPStudent(&out.Students[i], defaultThreshold, criteria); err != nil {
			return AssessmentTPRecap{}, err
		}
	}
	return out, nil
}

// finishAssessmentTPStudent menjaga perhitungan NH untuk bukti yang berlaku.
func finishAssessmentTPStudent(student *AssessmentTPRecapStudent, defaultThreshold string, criteria map[int64]string) error {
	// Sumber historis tidak menetapkan kewajiban pada nonanggota; kegiatan
	// lain yang memang berlaku pada tanggalnya tetap dipertahankan.
	filtered := student.Objectives[:0]
	for _, objective := range student.Objectives {
		if objective.HistoricalApplicability == "bukan anggota" && objective.RequiredCount == 0 {
			continue
		}
		filtered = append(filtered, objective)
	}
	student.Objectives = filtered
	values := make([]*big.Rat, 0, len(student.Objectives))
	for j := range student.Objectives {
		objective := &student.Objectives[j]
		objective.Threshold = defaultThreshold
		if objective.Threshold != "" {
			objective.ThresholdSource = "Bawaan"
		}
		objectiveID, parseErr := strconv.ParseInt(objective.ID, 10, 64)
		if parseErr != nil {
			return parseErr
		}
		if v, ok := criteria[objectiveID]; ok {
			objective.Threshold, objective.ThresholdSource = v, "Pengecualian"
		}
		objective.Achievement = "Belum Lengkap"
		// Fakta historis tetap belum pasti, tetapi tidak menghalangi kegiatan
		// lain yang berlaku pada tanggal pelaksanaannya sendiri.
		if objective.HistoricalApplicability == "belum pasti" && objective.RequiredCount == 0 {
			objective.Missing = append(objective.Missing, "Keberlakuan TP historis belum dapat diverifikasi.")
			continue
		}
		if objective.RequiredCount == 0 {
			objective.Missing = append(objective.Missing, "Belum ada kegiatan Sumatif yang menjadi kewajiban siswa pada TP ini.")
			continue
		}
		if objective.ScoredCount != objective.RequiredCount {
			continue
		}
		average, averageErr := exactAverage(objective.Activities)
		if averageErr != nil {
			return averageErr
		}
		objective.Average = strings.ReplaceAll(average.FloatString(2), ".", ",")
		objective.AverageExact = average.RatString()
		objective.Complete = true
		objective.Achievement = tpAchievement(average, true, objective.Threshold)
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
	return nil
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
