package academic

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"strings"
)

// SemesterRecommendation merupakan pembacaan pra-Final tanpa menyimpan nilai akhir.
type SemesterRecommendation struct {
	Context                    LearningObjectiveContext
	Weights                    AssessmentWeights
	UnknownPlacementCount      int
	OutOfCalendarActivityCount int
	Students                   []SemesterRecommendationStudent
}

type SemesterRecommendationStudent struct {
	ID, Name, NIS                          string
	NH, ASTS, ASAS, Status, Recommendation string
	NHExact                                string
	Objectives                             []AssessmentTPRecapObjective
	Missing                                []string
	Finalized                              bool
	FinalValue                             string
	FinalVersion                           int64
	NeedsReview                            bool
	ExceptionEligible                      bool     `json:"-"`
	ExceptionEvidence                      []string `json:"-"`
	FinalType                              string   `json:"-"`
}

type examEvidence struct {
	count, localCount int
	value             string
}

// SemesterRecommendations membaca bobot, nilai dan penempatan dari satu snapshot.
func (s *Store) SemesterRecommendations(ctx context.Context, actor, semesterID, classID, subjectID int64) (SemesterRecommendation, error) {
	if semesterID < 1 || classID < 1 || subjectID < 1 {
		return SemesterRecommendation{}, ValidationError("Pilih konteks semester, kelas, dan mata pelajaran.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return SemesterRecommendation{}, err
	}
	defer tx.Rollback()
	out, err := s.semesterRecommendationsTx(ctx, tx, actor, semesterID, classID, subjectID)
	if err != nil {
		return out, err
	}
	if err = s.overlaySemesterFinals(ctx, tx, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) semesterRecommendationsTx(ctx context.Context, tx *sql.Tx, actor, semesterID, classID, subjectID int64) (SemesterRecommendation, error) {
	return s.semesterRecommendationsStudentTx(ctx, tx, actor, semesterID, classID, subjectID, 0)
}

func (s *Store) semesterRecommendationsStudentTx(ctx context.Context, tx *sql.Tx, actor, semesterID, classID, subjectID, studentID int64) (SemesterRecommendation, error) {
	return s.semesterRecommendationsStudentModeTx(ctx, tx, actor, semesterID, classID, subjectID, studentID, 0)
}

func (s *Store) semesterRecommendationsHistoricalScoreStudentTx(ctx context.Context, tx *sql.Tx, actor, semesterID, classID, subjectID, studentID, activityID int64) (SemesterRecommendation, error) {
	return s.semesterRecommendationsStudentModeTx(ctx, tx, actor, semesterID, classID, subjectID, studentID, activityID)
}

func (s *Store) semesterRecommendationsStudentModeTx(ctx context.Context, tx *sql.Tx, actor, semesterID, classID, subjectID, studentID, historicalActivityID int64) (SemesterRecommendation, error) {
	var recap AssessmentTPRecap
	var err error
	if historicalActivityID > 0 {
		recap, err = s.assessmentTPRecapForHistoricalScore(ctx, tx, actor, semesterID, classID, subjectID, studentID, historicalActivityID)
	} else {
		recap, err = s.assessmentTPRecapForStudent(ctx, tx, actor, semesterID, classID, subjectID, true, studentID)
	}
	if err != nil {
		return SemesterRecommendation{}, err
	}
	// Pemilik historis tetap dapat mempertahankan Final ketika penempatan
	// terbaru tidak lagi menyediakan roster yang dapat dihitung.
	if studentID > 0 && len(recap.Students) == 0 {
		var name, nis string
		if err = tx.QueryRowContext(ctx, `SELECT name,coalesce(nis,'') FROM app.students WHERE id=$1`, studentID).Scan(&name, &nis); err != nil {
			return SemesterRecommendation{}, err
		}
		recap.Students = append(recap.Students, AssessmentTPRecapStudent{ID: fmtID(studentID), Name: name, NIS: nis})
	}
	out := SemesterRecommendation{Context: recap.Context, UnknownPlacementCount: recap.UnknownPlacementCount, OutOfCalendarActivityCount: recap.OutOfCalendarActivityCount}
	out.Weights.SemesterID = semesterID
	err = tx.QueryRowContext(ctx, `SELECT nh,asts,asas FROM app.semester_assessment_weights WHERE semester_id=$1`, semesterID).Scan(&out.Weights.NH, &out.Weights.ASTS, &out.Weights.ASAS)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	out.Weights.Configured = err == nil
	if out.Weights.Configured {
		if err = validateAssessmentWeights(out.Weights); err != nil {
			return out, err
		}
	}
	// Semua sumber kelas diperiksa: nilai kelas lain tidak boleh diakui otomatis.
	rows, err := tx.QueryContext(ctx, `SELECT e.student_id,e.class_id,e.component,
 greatest(coalesce(c.after_score,e.score),coalesce(r.best,coalesce(c.after_score,e.score)))::text
 FROM app.exam_initial_scores e
 LEFT JOIN LATERAL (SELECT after_score FROM app.exam_score_corrections WHERE initial_score_id=e.id ORDER BY version DESC LIMIT 1) c ON true
 LEFT JOIN LATERAL (SELECT max(score) AS best FROM app.exam_score_remedials WHERE initial_score_id=e.id) r ON true
 WHERE e.semester_id=$1 AND e.subject_id=$2 AND ($3::bigint=0 OR e.student_id=$3) AND EXISTS (SELECT 1 FROM app.student_placements p WHERE p.id=e.placement_id AND p.student_id=e.student_id AND p.starts_on IS NOT NULL AND p.starts_on<=e.exam_date AND app.student_placement_class_at(p.id,e.exam_date)=e.class_id)
 ORDER BY e.id`, semesterID, subjectID, studentID)
	if err != nil {
		return out, err
	}
	evidence := map[string]examEvidence{}
	for rows.Next() {
		var id, sourceClass int64
		var component, value string
		if err = rows.Scan(&id, &sourceClass, &component, &value); err != nil {
			rows.Close()
			return out, err
		}
		key := fmtID(id) + ":" + component
		e := evidence[key]
		e.count++
		if sourceClass == classID {
			e.localCount++
			e.value = value
		}
		evidence[key] = e
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	for _, student := range recap.Students {
		item := SemesterRecommendationStudent{ID: student.ID, Name: student.Name, NIS: student.NIS, NH: student.NH, Objectives: student.Objectives}
		var nh *big.Rat
		scored := false
		for _, objective := range student.Objectives {
			if objective.ScoredCount > 0 {
				scored = true
			}
		}
		if student.NHComplete {
			nh = new(big.Rat)
			for _, objective := range student.Objectives {
				avg, e := exactAverage(objective.Activities)
				if e != nil {
					return out, e
				}
				nh.Add(nh, avg)
			}
			nh.Quo(nh, big.NewRat(int64(len(student.Objectives)), 1))
			item.NHExact = nh.RatString()
		}
		asts := evidence[student.ID+":asts"]
		asas := evidence[student.ID+":asas"]
		var a, b *big.Rat
		for _, x := range []struct {
			ev      examEvidence
			name    string
			display *string
			value   **big.Rat
		}{{asts, "ASTS", &item.ASTS, &a}, {asas, "ASAS", &item.ASAS, &b}} {
			if x.ev.count > 1 || (x.ev.count == 1 && x.ev.localCount == 0) {
				*x.display = "Perlu pengakuan atau pemilihan sumber"
				continue
			}
			if x.ev.localCount == 1 {
				v, ok := new(big.Rat).SetString(x.ev.value)
				if !ok {
					return out, errors.New("nilai ujian tidak valid")
				}
				*x.value = v
				*x.display = strings.ReplaceAll(x.ev.value, ".", ",")
			}
		}
		if out.Weights.Configured {
			ready, hasEvidence := semesterRecommendationReadiness(
				out.Weights,
				student.NHComplete, scored,
				a != nil, asts.localCount > 0,
				b != nil, asas.localCount > 0,
			)
			if out.Weights.NH > 0 && nh == nil {
				if len(student.Objectives) == 0 {
					item.Missing = append(item.Missing, "NH belum memiliki TP atau nilai Sumatif lengkap.")
				} else {
					for _, objective := range student.Objectives {
						if !objective.Complete {
							item.Missing = append(item.Missing, objective.Definition+": "+strings.Join(objective.Missing, "; "))
						}
					}
				}
			}
			if out.Weights.ASTS > 0 && a == nil {
				item.Missing = append(item.Missing, examMissing("ASTS", asts))
			}
			if out.Weights.ASAS > 0 && b == nil {
				item.Missing = append(item.Missing, examMissing("ASAS", asas))
			}
			switch {
			case ready:
				item.Status = "Siap Final"
				item.Recommendation = semesterRound(nh, a, b, out.Weights)
			case !hasEvidence:
				item.Status = "Belum Dinilai"
			default:
				item.Status = "Belum Lengkap"
			}
		}
		// Bukti pengecualian berasal dari nilai yang sah pada komponen berbobot positif.
		// Field presentasi tidak ikut hash snapshot agar Final lama tetap stabil.
		if out.Weights.Configured && item.Status == "Belum Lengkap" {
			if out.Weights.NH > 0 {
				for _, objective := range item.Objectives {
					for _, activity := range objective.Activities {
						if activity.HasValue {
							item.ExceptionEvidence = append(item.ExceptionEvidence, "Sumatif "+activity.Name+" · "+objective.Definition+": "+activity.EffectiveValue)
						}
					}
				}
			}
			if out.Weights.ASTS > 0 && a != nil {
				item.ExceptionEvidence = append(item.ExceptionEvidence, "ASTS: "+item.ASTS)
			}
			if out.Weights.ASAS > 0 && b != nil {
				item.ExceptionEvidence = append(item.ExceptionEvidence, "ASAS: "+item.ASAS)
			}
			item.ExceptionEligible = len(item.ExceptionEvidence) > 0
		}
		out.Students = append(out.Students, item)
	}
	return out, nil
}

// semesterRecommendationReadiness adalah satu-satunya aturan kanonis untuk
// membedakan kelengkapan komponen berbobot dari keberadaan bukti berbobot.
// Komponen berbobot 0 tidak menghalangi Final dan tidak membuat siswa seolah
// sudah dinilai.
func semesterRecommendationReadiness(w AssessmentWeights, nhComplete, nhEvidence, astsComplete, astsEvidence, asasComplete, asasEvidence bool) (ready, hasEvidence bool) {
	if !w.Configured {
		return false, false
	}
	ready = (w.NH == 0 || nhComplete) &&
		(w.ASTS == 0 || astsComplete) &&
		(w.ASAS == 0 || asasComplete)
	hasEvidence = (w.NH > 0 && nhEvidence) ||
		(w.ASTS > 0 && astsEvidence) ||
		(w.ASAS > 0 && asasEvidence)
	return ready, hasEvidence
}

func examMissing(name string, e examEvidence) string {
	if e.count > 1 {
		return name + " memiliki beberapa sumber; pemilihan sumber belum tersedia."
	}
	if e.count == 1 && e.localCount == 0 {
		return name + " dari kelas asal memerlukan pengakuan."
	}
	return name + " belum dinilai."
}

// semesterRound membulatkan sekali pada hasil akhir, memakai rasional eksak.
func semesterRound(nh, asts, asas *big.Rat, w AssessmentWeights) string {
	total := new(big.Rat)
	for _, part := range []struct {
		value  *big.Rat
		weight int
	}{{nh, w.NH}, {asts, w.ASTS}, {asas, w.ASAS}} {
		if part.weight > 0 && part.value != nil {
			total.Add(total, new(big.Rat).Mul(part.value, big.NewRat(int64(part.weight), 100)))
		}
	}
	numerator := new(big.Int).Add(new(big.Int).Mul(total.Num(), big.NewInt(2)), total.Denom())
	denominator := new(big.Int).Mul(total.Denom(), big.NewInt(2))
	return new(big.Int).Quo(numerator, denominator).String()
}
