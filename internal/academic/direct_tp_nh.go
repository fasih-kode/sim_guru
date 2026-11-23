package academic

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// directTPApplicability harus berasal dari fakta yang dapat diverifikasi atau
// penetapan sah; ketidakpastian tidak boleh diubah menjadi kewajiban otomatis.
type directTPApplicability uint8

const (
	directTPUnknown directTPApplicability = iota
	directTPApplicable
	directTPNotApplicable
)

type directTPNHCell struct {
	Applicability directTPApplicability
	Value         string // Kosong berarti belum dinilai; "0" adalah nilai sah.
}

type directTPNHResult struct {
	Exact       *big.Rat
	Complete    bool
	HasEvidence bool
	Unknown     bool
	Missing     int
}

// calculateDirectTPNH tidak membaca nilai kegiatan maupun membulatkan rata-rata.
// Nilai parsial tidak dipublikasikan sebagai NH ketika penyebut belum pasti.
func calculateDirectTPNH(cells []directTPNHCell) (directTPNHResult, error) {
	var out directTPNHResult
	sum := new(big.Rat)
	var count int64
	for _, cell := range cells {
		switch cell.Applicability {
		case directTPNotApplicable:
			continue
		case directTPUnknown:
			out.Unknown = true
			continue
		case directTPApplicable:
		default:
			return out, errors.New("status keberlakuan TP tidak valid")
		}
		count++
		if strings.TrimSpace(cell.Value) == "" {
			out.Missing++
			continue
		}
		v, ok := new(big.Rat).SetString(strings.ReplaceAll(cell.Value, ",", "."))
		if !ok || v.Sign() < 0 || v.Cmp(big.NewRat(100, 1)) > 0 {
			return out, errors.New("nilai TP langsung tidak valid")
		}
		out.HasEvidence = true
		sum.Add(sum, v)
	}
	if count > 0 && out.Missing == 0 && !out.Unknown {
		out.Exact = sum.Quo(sum, big.NewRat(count, 1))
		out.Complete = true
	}
	return out, nil
}

// directTPRecapForStudent membaca nilai langsung dan penempatan pada transaksi
// pemanggil yang sama. Sel yang belum jelas keberlakuannya tidak dinilai otomatis.
func (s *Store) directTPRecapForStudent(ctx context.Context, tx *sql.Tx, actor, semester, class, subject, student, historicalActivity int64) (AssessmentTPRecap, error) {
	var base AssessmentTPRecap
	if err := s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return base, err
	}
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return base, err
	}
	var allowed bool
	if historicalActivity > 0 && student > 0 {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.assessment_activities a WHERE a.id=$1 AND (a.semester_id,a.class_id,a.subject_id,a.teacher_id)=($2,$3,$4,$5) AND a.activity_type='sumatif' AND EXISTS(SELECT 1 FROM app.teaching_assignments ta WHERE (ta.teacher_id,ta.semester_id,ta.class_id,ta.subject_id)=(a.teacher_id,a.semester_id,a.class_id,a.subject_id) AND ta.effective_from<=a.activity_date AND (ta.effective_until IS NULL OR ta.effective_until>a.activity_date)) AND (EXISTS(SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=a.id AND h.teacher_id=a.teacher_id) OR EXISTS(SELECT 1 FROM app.historical_assessment_objective_links l WHERE l.activity_id=a.id AND l.teacher_id=a.teacher_id)))`, historicalActivity, semester, class, subject, teacher).Scan(&allowed)
	} else if student > 0 {
		allowed, err = semesterFinalOwned(ctx, tx, teacher, semester, class, subject, student)
	} else {
		allowed, err = learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	}
	if err != nil {
		return base, err
	}
	if !allowed {
		return base, ErrForbidden
	}
	err = tx.QueryRowContext(ctx, `SELECT y.name || ' · Semester ' || sm.number::text,c.grade::text || c.name,s.name FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id JOIN app.classes c ON c.id=$2 AND c.school_year_id=sm.school_year_id JOIN app.subjects s ON s.id=$3 AND ($4::boolean OR s.active) JOIN app.semester_calendars cal ON cal.semester_id=sm.id WHERE sm.id=$1`, semester, class, subject, student > 0).Scan(&base.Context.SemesterName, &base.Context.ClassName, &base.Context.SubjectName)
	if errors.Is(err, sql.ErrNoRows) {
		return base, ErrNotFound
	}
	if err != nil {
		return base, err
	}
	base.Context.SemesterID, base.Context.ClassID, base.Context.SubjectID = semester, class, subject
	criteria := map[int64]string{}
	var defaultThreshold string
	criteriaRows, err := tx.QueryContext(ctx, `SELECT coalesce(objective_id,0),threshold::text FROM app.tp_achievement_criteria WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND threshold IS NOT NULL`, semester, class, subject)
	if err != nil {
		return base, err
	}
	for criteriaRows.Next() {
		var id int64
		var value string
		if err = criteriaRows.Scan(&id, &value); err != nil {
			criteriaRows.Close()
			return base, err
		}
		if id == 0 {
			defaultThreshold = value
		} else {
			criteria[id] = value
		}
	}
	if err = criteriaRows.Err(); err != nil {
		criteriaRows.Close()
		return base, err
	}
	criteriaRows.Close()
	rows, err := tx.QueryContext(ctx, `
SELECT p.student_id, st.name, coalesce(st.nis,''),
 COALESCE(p.starts_on IS NOT NULL AND p.starts_on<=cal.starts_on
 AND app.student_placement_class_at(p.id,cal.starts_on)=$2
 AND app.student_placement_class_at(p.id,least(cal.ends_on,(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date))=$2
 AND NOT EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND m.effective_date BETWEEN cal.starts_on AND cal.ends_on)
 AND NOT EXISTS(SELECT 1 FROM app.student_school_exits x WHERE x.student_id=p.student_id AND x.effective_date BETWEEN cal.starts_on AND cal.ends_on)
 AND NOT EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.effective_date BETWEEN cal.starts_on AND cal.ends_on) AND st.active,false),
 COALESCE(p.starts_on IS NOT NULL AND p.starts_on<=cal.ends_on AND (app.student_placement_class_at(p.id,greatest(p.starts_on,cal.starts_on))=$2 OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND m.to_class_id=$2 AND m.effective_date BETWEEN cal.starts_on AND cal.ends_on) OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=$2 AND r.effective_date BETWEEN cal.starts_on AND cal.ends_on)),false),
 lo.id,lo.definition,coalesce(c.after_score,sc.score)::text
FROM app.student_placements p JOIN app.students st ON st.id=p.student_id
JOIN app.semesters sm ON sm.id=$1 AND sm.school_year_id=p.school_year_id
JOIN app.semester_calendars cal ON cal.semester_id=sm.id
LEFT JOIN app.learning_objectives lo ON (lo.semester_id,lo.class_id,lo.subject_id)=($1,$2,$3)
LEFT JOIN app.direct_tp_initial_scores sc ON (sc.semester_id,sc.class_id,sc.subject_id,sc.student_id,sc.objective_id)=($1,$2,$3,p.student_id,lo.id)
LEFT JOIN LATERAL (SELECT after_score FROM app.direct_tp_score_corrections WHERE initial_score_id=sc.id ORDER BY version DESC LIMIT 1) c ON true
WHERE ($4::bigint=0 OR p.student_id=$4) AND (p.class_id=$2
 OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.from_class_id=$2 OR m.to_class_id=$2))
 OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=$2))
ORDER BY st.name,p.student_id,lo.id`, semester, class, subject, student)
	if err != nil {
		return base, err
	}
	defer rows.Close()
	indexes := map[int64]int{}
	values := map[string]string{}
	for rows.Next() {
		var sid int64
		var name, nis string
		var certain, placementVerified bool
		var objective sql.NullInt64
		var definition, value sql.NullString
		if err = rows.Scan(&sid, &name, &nis, &certain, &placementVerified, &objective, &definition, &value); err != nil {
			return base, err
		}
		i, ok := indexes[sid]
		if !ok {
			i = len(base.Students)
			indexes[sid] = i
			base.Students = append(base.Students, AssessmentTPRecapStudent{ID: fmtID(sid), Name: name, NIS: nis, PlacementUncertain: !placementVerified})
		}
		if !objective.Valid {
			continue
		}
		o := AssessmentTPRecapObjective{ID: fmtID(objective.Int64), Definition: definition.String, RequiredCount: 1, Achievement: "Belum Lengkap", Threshold: defaultThreshold}
		if o.Threshold != "" {
			o.ThresholdSource = "Bawaan"
		}
		if threshold, ok := criteria[objective.Int64]; ok {
			o.Threshold = threshold
			o.ThresholdSource = "Pengecualian"
		}
		if value.Valid {
			values[fmtID(sid)+":"+fmtID(objective.Int64)] = value.String
		}
		base.Students[i].Objectives = append(base.Students[i].Objectives, o)
	}
	if err = rows.Err(); err != nil {
		return base, err
	}
	if err = rows.Close(); err != nil {
		return base, err
	}
	for i := range base.Students {
		st := &base.Students[i]
		studentID, e := strconv.ParseInt(st.ID, 10, 64)
		if e != nil {
			return base, e
		}
		cells := make([]directTPNHCell, 0, len(st.Objectives))
		for j := range st.Objectives {
			o := &st.Objectives[j]
			objectiveID, e := strconv.ParseInt(o.ID, 10, 64)
			if e != nil {
				return base, e
			}
			a, _, _, e := readDirectTPApplicability(ctx, tx, semester, class, subject, studentID, objectiveID)
			if e != nil {
				return base, e
			}
			value := values[st.ID+":"+o.ID]
			switch a {
			case directTPUnknown:
				o.HistoricalApplicability = "belum pasti"
				o.Missing = []string{"Keberlakuan TP memerlukan penetapan atau verifikasi penempatan."}
			case directTPNotApplicable:
				o.HistoricalApplicability = "tidak berlaku"
				o.RequiredCount = 0
				o.Achievement = "Tidak Berlaku"
			case directTPApplicable:
				if value == "" {
					o.Missing = []string{"Nilai TP belum diisi."}
				} else {
					v, ok := new(big.Rat).SetString(value)
					if !ok {
						return base, errors.New("nilai TP langsung tidak valid")
					}
					o.ScoredCount = 1
					o.Complete = true
					o.Average = strings.ReplaceAll(v.FloatString(2), ".", ",")
					o.AverageExact = v.RatString()
					o.Achievement = tpAchievement(v, true, o.Threshold)
				}
			}
			cells = append(cells, directTPNHCell{Applicability: a, Value: value})
			if !o.Complete && a != directTPNotApplicable {
				st.Missing = append(st.Missing, o.Definition)
			}
		}
		nh, e := calculateDirectTPNH(cells)
		if e != nil {
			return base, e
		}
		if st.PlacementUncertain || nh.Unknown {
			base.UnknownPlacementCount++
		}
		st.NHComplete = nh.Complete
		if nh.Complete {
			st.NH = strings.ReplaceAll(nh.Exact.FloatString(2), ".", ",")
			st.NHExact = nh.Exact.RatString()
		}
	}
	return base, nil
}
