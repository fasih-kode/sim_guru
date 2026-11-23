package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// DirectTPApplicabilityInput mencatat penetapan S-06; versi 0 berarti belum ada.
type DirectTPApplicabilityInput struct {
	SemesterID, ClassID, SubjectID, StudentID, ObjectiveID int64
	ExpectedVersion                                        int64
	EffectiveDate                                          string
	Applicable                                             bool
	EvidenceVerified                                       bool
	Reason                                                 string
	SubmissionKey                                          string
}

// SaveDirectTPApplicability menambahkan satu keputusan berversi tanpa mengubah
// nilai atau Final. Kasus pasca-Final ditahan sampai alur peninjauan tersedia.
func (s *Store) SaveDirectTPApplicability(ctx context.Context, actor int64, in DirectTPApplicabilityInput) error {
	in.Reason = strings.TrimSpace(in.Reason)
	date, err := time.Parse("2006-01-02", in.EffectiveDate)
	if err != nil || date.Format("2006-01-02") != in.EffectiveDate || in.SemesterID < 1 || in.ClassID < 1 || in.SubjectID < 1 || in.StudentID < 1 || in.ObjectiveID < 1 || in.ExpectedVersion < 0 || in.Reason == "" || !in.EvidenceVerified || !utf8ValidReason(in.Reason) || !validSubmissionUUID(in.SubmissionKey) {
		return ValidationError("Penetapan, tanggal, alasan, versi atau identitas pengiriman tidak valid.")
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	tx, err := s.beginDirectTP(ctx, actor)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, in.SemesterID, in.ClassID, in.SubjectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	var existing []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM app.direct_tp_applicability_history WHERE actor_id=$1 AND submission_key=$2::uuid`, actor, in.SubmissionKey).Scan(&existing)
	if err == nil {
		if !equalHash(existing, digest[:]) {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var year, placement int64
	err = tx.QueryRowContext(ctx, `SELECT sm.school_year_id,p.id FROM app.semesters sm JOIN app.semester_calendars cal ON cal.semester_id=sm.id JOIN app.student_placements p ON p.school_year_id=sm.school_year_id AND p.student_id=$4 JOIN app.students st ON st.id=p.student_id AND st.active JOIN app.learning_objectives o ON (o.id,o.school_year_id,o.semester_id,o.class_id,o.subject_id)=($5,sm.school_year_id,$1,$2,$3) WHERE sm.id=$1 AND p.starts_on IS NOT NULL AND $6::date BETWEEN cal.starts_on AND cal.ends_on AND $6::date <= (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date AND p.starts_on<=$6::date AND app.student_placement_class_at(p.id,$6::date)=$2 FOR SHARE OF p`, in.SemesterID, in.ClassID, in.SubjectID, in.StudentID, in.ObjectiveID, in.EffectiveDate).Scan(&year, &placement)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	var protected bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_versions WHERE (semester_id,class_id,subject_id,student_id)=($1,$2,$3,$4)) OR EXISTS(SELECT 1 FROM app.direct_tp_initial_scores WHERE (semester_id,class_id,subject_id,student_id,objective_id)=($1,$2,$3,$4,$5) AND owner_teacher_id<>$6)`, in.SemesterID, in.ClassID, in.SubjectID, in.StudentID, in.ObjectiveID, teacher).Scan(&protected)
	if err != nil {
		return err
	}
	if protected {
		return ErrConflict
	}
	var version int64
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(max(version),0) FROM app.direct_tp_applicability_history WHERE (semester_id,class_id,subject_id,student_id,objective_id)=($1,$2,$3,$4,$5)`, in.SemesterID, in.ClassID, in.SubjectID, in.StudentID, in.ObjectiveID).Scan(&version); err != nil {
		return err
	}
	if version != in.ExpectedVersion {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.direct_tp_applicability_history(school_year_id,semester_id,class_id,subject_id,objective_id,student_id,placement_id,teacher_id,actor_id,effective_date,applicable,reason,evidence_verified,version,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::date,$11,$12,$13,$14,$15::uuid,$16)`, year, in.SemesterID, in.ClassID, in.SubjectID, in.ObjectiveID, in.StudentID, placement, teacher, actor, in.EffectiveDate, in.Applicable, in.Reason, in.EvidenceVerified, version+1, in.SubmissionKey, digest[:])
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}

// readDirectTPApplicability adalah sumber tunggal bagi input dan perhitungan.
// Keputusan yang tanggal/penempatannya tidak lagi cocok diperlakukan tidak pasti.
func readDirectTPApplicability(ctx context.Context, tx *sql.Tx, semester, class, subject, student, objective int64) (directTPApplicability, int64, int64, error) {
	var placement, version int64
	var normal, validDecision bool
	var decision sql.NullBool
	err := tx.QueryRowContext(ctx, `SELECT p.id,
 COALESCE(st.active AND p.starts_on IS NOT NULL AND p.starts_on<=cal.starts_on AND app.student_placement_class_at(p.id,cal.starts_on)=$2 AND app.student_placement_class_at(p.id,least(cal.ends_on,(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date))=$2 AND NOT EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND m.effective_date BETWEEN cal.starts_on AND cal.ends_on) AND NOT EXISTS(SELECT 1 FROM app.student_school_exits x WHERE x.student_id=p.student_id AND x.effective_date BETWEEN cal.starts_on AND cal.ends_on) AND NOT EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.effective_date BETWEEN cal.starts_on AND cal.ends_on) AND NOT EXISTS(SELECT 1 FROM app.historical_learning_objective_insertions h WHERE h.learning_objective_id=o.id AND (p.starts_on>h.assessment_date OR app.student_placement_class_at(p.id,h.assessment_date) IS DISTINCT FROM $2)),false),
 d.applicable,coalesce(d.version,0),COALESCE(st.active AND d.placement_id=p.id AND p.starts_on IS NOT NULL AND p.starts_on<=d.effective_date AND d.effective_date BETWEEN cal.starts_on AND cal.ends_on AND d.effective_date <= (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date AND app.student_placement_class_at(p.id,d.effective_date)=$2,false)
 FROM app.semesters sm JOIN app.semester_calendars cal ON cal.semester_id=sm.id JOIN app.student_placements p ON p.school_year_id=sm.school_year_id AND p.student_id=$4 JOIN app.students st ON st.id=p.student_id JOIN app.learning_objectives o ON (o.id,o.school_year_id,o.semester_id,o.class_id,o.subject_id)=($5,sm.school_year_id,$1,$2,$3)
 LEFT JOIN LATERAL (SELECT applicable,version,effective_date,placement_id FROM app.direct_tp_applicability_history WHERE (semester_id,class_id,subject_id,student_id,objective_id)=($1,$2,$3,$4,$5) ORDER BY version DESC LIMIT 1) d ON true WHERE sm.id=$1`, semester, class, subject, student, objective).Scan(&placement, &normal, &decision, &version, &validDecision)
	if errors.Is(err, sql.ErrNoRows) {
		return directTPUnknown, 0, 0, nil
	}
	if err != nil {
		return directTPUnknown, 0, 0, err
	}
	if normal {
		return directTPApplicable, placement, version, nil
	}
	if validDecision && decision.Valid {
		if decision.Bool {
			return directTPApplicable, placement, version, nil
		}
		return directTPNotApplicable, placement, version, nil
	}
	return directTPUnknown, placement, version, nil
}
