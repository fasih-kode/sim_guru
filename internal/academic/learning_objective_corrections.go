package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const learningObjectiveImpactPageSize = 10

// LearningObjectiveEditState memberi data aktif dan status penguncian TP.
type LearningObjectiveEditState struct {
	Objective          LearningObjective
	Context            LearningObjectiveContext
	Version            int64
	HasActivities      bool
	HasScores          bool
	HasFinals          bool
	ActivityCount      int
	ScoredStudentCount int
	FinalStudentCount  int
	Activities         []LearningObjectiveImpactActivity
	RoutineAllowed     bool
	RoutineToken       string
}

type LearningObjectiveImpactActivity struct {
	ID                 int64
	Name, Type, Date   string
	ScoredStudentCount int
}

type LearningObjectiveImpactFinal struct {
	StudentID               int64
	StudentName, FinalValue string
	Version                 int64
	Type                    string
}

// LearningObjectiveCorrectionPreview mengikat konfirmasi pada keadaan akademik
// yang dibaca dalam satu snapshot transaksi.
type LearningObjectiveCorrectionPreview struct {
	State                             LearningObjectiveEditState
	DefinitionBefore, DefinitionAfter string
	ChangeType, Reason                string
	ExpectedVersion                   int64
	SnapshotToken, SubmissionKey      string
	Page, PageCount                   int
	PreviousPage, NextPage            int
	ImpactedFinals                    []LearningObjectiveImpactFinal
}

// LearningObjectiveDefinitionRevision adalah bukti append-only perubahan uraian.
type LearningObjectiveDefinitionRevision struct {
	ID, Version                       int64
	DefinitionBefore, DefinitionAfter string
	ChangeType, Reason, Actor, At     string
	PreviewSHA256                     string
	ImpactSnapshot                    string
}

type learningObjectiveCorrectionSnapshotData struct {
	ObjectiveID, SemesterID, ClassID, SubjectID int64
	Version                                     int64
	Impact                                      json.RawMessage
}

type learningObjectiveCorrectionRequest struct {
	ObjectiveID, ExpectedVersion                                 int64
	Definition, ChangeType, Reason, SnapshotToken, SubmissionKey string
}

// LearningObjectiveEdit memverifikasi penugasan aktif dan membaca status TP.
func (s *Store) LearningObjectiveEdit(ctx context.Context, actor, objectiveID int64) (LearningObjectiveEditState, error) {
	var out LearningObjectiveEditState
	if objectiveID < 1 {
		return out, ValidationError("Tujuan Pembelajaran tidak valid.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return out, err
	}
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	if err = readLearningObjectiveEditState(ctx, tx, teacherID, objectiveID, &out); err != nil {
		return out, err
	}
	return out, nil
}

func readLearningObjectiveEditState(ctx context.Context, tx *sql.Tx, teacherID, objectiveID int64, out *LearningObjectiveEditState) error {
	var created time.Time
	err := tx.QueryRowContext(ctx, `
		SELECT lo.id,lo.semester_id,lo.class_id,lo.subject_id,lo.definition,lo.created_by,
		       lo.created_at AT TIME ZONE 'Asia/Jakarta',
		       coalesce((SELECT max(r.version) FROM app.learning_objective_definition_revisions r WHERE r.learning_objective_id=lo.id),0),
		       EXISTS(SELECT 1 FROM app.assessment_activity_objectives ao WHERE ao.learning_objective_id=lo.id),
		       EXISTS(SELECT 1 FROM app.assessment_activity_initial_scores sc WHERE sc.learning_objective_id=lo.id),
		       EXISTS(SELECT 1 FROM app.semester_final_versions fv WHERE fv.semester_id=lo.semester_id AND fv.class_id=lo.class_id AND fv.subject_id=lo.subject_id)
		FROM app.learning_objectives lo WHERE lo.id=$1`, objectiveID).
		Scan(&out.Objective.ID, &out.Objective.SemesterID, &out.Objective.ClassID, &out.Objective.SubjectID,
			&out.Objective.Definition, &out.Objective.CreatedBy, &created, &out.Version,
			&out.HasActivities, &out.HasScores, &out.HasFinals)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	out.Objective.CreatedAt = created.Format("02-01-2006 15:04")
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, out.Objective.SemesterID, out.Objective.ClassID, out.Objective.SubjectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	if err = tx.QueryRowContext(ctx, `SELECT y.name||' · Semester '||sm.number::text,c.grade::text||c.name,s.name
		FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id
		JOIN app.classes c ON c.id=$2 AND c.school_year_id=sm.school_year_id
		JOIN app.subjects s ON s.id=$3 WHERE sm.id=$1`, out.Objective.SemesterID, out.Objective.ClassID, out.Objective.SubjectID).
		Scan(&out.Context.SemesterName, &out.Context.ClassName, &out.Context.SubjectName); err != nil {
		return err
	}
	out.Context.SemesterID, out.Context.ClassID, out.Context.SubjectID = out.Objective.SemesterID, out.Objective.ClassID, out.Objective.SubjectID
	rows, err := tx.QueryContext(ctx, `
		SELECT a.id,a.name,a.activity_type,a.activity_date::text,
		       count(DISTINCT sc.student_id)
		FROM app.assessment_activities a
		JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id AND ao.learning_objective_id=$1
		LEFT JOIN app.assessment_activity_initial_scores sc ON sc.assessment_activity_id=a.id AND sc.learning_objective_id=$1
		WHERE a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4
		GROUP BY a.id ORDER BY a.activity_date,a.id`, objectiveID, out.Objective.SemesterID, out.Objective.ClassID, out.Objective.SubjectID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var activity LearningObjectiveImpactActivity
		if err = rows.Scan(&activity.ID, &activity.Name, &activity.Type, &activity.Date, &activity.ScoredStudentCount); err != nil {
			rows.Close()
			return err
		}
		out.Activities = append(out.Activities, activity)
		out.ScoredStudentCount += activity.ScoredStudentCount
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	out.ActivityCount = len(out.Activities)
	if err = tx.QueryRowContext(ctx, `
		SELECT count(DISTINCT sc.student_id)
		FROM app.assessment_activity_initial_scores sc
		JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id
		WHERE sc.learning_objective_id=$1 AND a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4`,
		objectiveID, out.Objective.SemesterID, out.Objective.ClassID, out.Objective.SubjectID).Scan(&out.ScoredStudentCount); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT DISTINCT student_id FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3) f`,
		out.Objective.SemesterID, out.Objective.ClassID, out.Objective.SubjectID).Scan(&out.FinalStudentCount); err != nil {
		return err
	}
	// Relasi kegiatan saja belum menjadi bukti nilai atau penguncian struktur.
	// TP tanpa nilai tetap dapat dikoreksi rutin sampai Final pertama pada konteks.
	out.RoutineAllowed = !out.HasScores && !out.HasFinals
	snapshot, err := readLearningObjectiveCorrectionSnapshot(ctx, tx, *out)
	if err != nil {
		return err
	}
	out.RoutineToken = objectiveRoutineSnapshotToken(learningObjectiveCorrectionSnapshotData{
		ObjectiveID: out.Objective.ID, SemesterID: out.Objective.SemesterID,
		ClassID: out.Objective.ClassID, SubjectID: out.Objective.SubjectID,
		Version: out.Version, Impact: snapshot,
	})
	return nil
}

// PreviewLearningObjectiveCorrection hanya membaca data; semua dampak dalam
// token berasal dari snapshot database yang sama.
func (s *Store) PreviewLearningObjectiveCorrection(ctx context.Context, actor, objectiveID, expectedVersion int64, definition, changeType, reason string, page int, submissionKey string, expectedSnapshotToken ...string) (LearningObjectiveCorrectionPreview, error) {
	return s.learningObjectiveCorrectionPreview(ctx, actor, objectiveID, expectedVersion, definition, changeType, reason, page, submissionKey, expectedSnapshotToken...)
}

func (s *Store) learningObjectiveCorrectionPreview(ctx context.Context, actor, objectiveID, expectedVersion int64, definition, changeType, reason string, page int, submissionKey string, expectedSnapshotToken ...string) (LearningObjectiveCorrectionPreview, error) {
	var out LearningObjectiveCorrectionPreview
	definition = strings.TrimSpace(definition)
	reason = strings.TrimSpace(reason)
	if objectiveID < 1 || expectedVersion < 0 {
		return out, ValidationError("Konteks koreksi TP tidak valid.")
	}
	if definition == "" {
		return out, ValidationError("Uraian Tujuan Pembelajaran wajib diisi.")
	}
	if changeType != "redaksi" && changeType != "substansi" {
		return out, ValidationError("Nyatakan apakah koreksi merupakan redaksi atau substansi.")
	}
	if !validLearningObjectiveReason(reason) {
		return out, ValidationError("Alasan koreksi wajib diisi dan maksimal 500 karakter.")
	}
	if page < 1 {
		page = 1
	}
	if !validSubmissionUUID(submissionKey) {
		return out, ValidationError("Kunci pengiriman tidak valid.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return out, err
	}
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	if err = readLearningObjectiveEditState(ctx, tx, teacherID, objectiveID, &out.State); err != nil {
		return out, err
	}
	if expectedVersion != out.State.Version {
		return out, ErrConflict
	}
	if definition == out.State.Objective.Definition {
		return out, ValidationError("Uraian baru sama dengan uraian aktif; tidak ada perubahan untuk dicatat.")
	}
	if !out.State.HasActivities && !out.State.HasScores && !out.State.HasFinals {
		return out, ErrConflict
	}
	snapshot, err := readLearningObjectiveCorrectionSnapshot(ctx, tx, out.State)
	if err != nil {
		return out, err
	}
	token, err := objectiveCorrectionSnapshotToken(learningObjectiveCorrectionSnapshotData{
		ObjectiveID: out.State.Objective.ID, SemesterID: out.State.Objective.SemesterID,
		ClassID: out.State.Objective.ClassID, SubjectID: out.State.Objective.SubjectID,
		Version: out.State.Version, Impact: snapshot,
	}, definition, changeType, reason)
	if err != nil {
		return out, err
	}
	out.DefinitionBefore, out.DefinitionAfter = out.State.Objective.Definition, definition
	out.ChangeType, out.Reason, out.ExpectedVersion = changeType, reason, expectedVersion
	out.SnapshotToken, out.SubmissionKey = token, submissionKey
	if len(expectedSnapshotToken) > 0 && expectedSnapshotToken[0] != "" && expectedSnapshotToken[0] != token {
		return LearningObjectiveCorrectionPreview{}, ErrConflict
	}
	out.Page = page
	out.PageCount = max(1, (out.State.FinalStudentCount+learningObjectiveImpactPageSize-1)/learningObjectiveImpactPageSize)
	if page > 1 {
		out.PreviousPage = page - 1
	}
	if page < out.PageCount {
		out.NextPage = page + 1
	}
	out.ImpactedFinals, err = learningObjectiveFinalImpactPage(ctx, tx, out.State, page)
	if err != nil {
		return out, err
	}
	return out, nil
}

// CorrectLearningObjectiveRoutine menyimpan edit TP belum digunakan.
func (s *Store) CorrectLearningObjectiveRoutine(ctx context.Context, actor, objectiveID, expectedVersion int64, definition, token, key string) error {
	return s.correctLearningObjective(ctx, actor, learningObjectiveCorrectionRequest{
		ObjectiveID: objectiveID, ExpectedVersion: expectedVersion, Definition: definition,
		ChangeType: "rutin", SnapshotToken: token, SubmissionKey: key,
	})
}

// ConfirmLearningObjectiveCorrection menyimpan hasil pratinjau setelah
// memeriksa ulang otorisasi, versi TP, token dampak, dan kunci replay.
func (s *Store) ConfirmLearningObjectiveCorrection(ctx context.Context, actor, objectiveID, expectedVersion int64, definition, changeType, reason, token, key string) error {
	return s.correctLearningObjective(ctx, actor, learningObjectiveCorrectionRequest{
		ObjectiveID: objectiveID, ExpectedVersion: expectedVersion, Definition: definition,
		ChangeType: changeType, Reason: reason, SnapshotToken: token, SubmissionKey: key,
	})
}

func (s *Store) correctLearningObjective(ctx context.Context, actor int64, input learningObjectiveCorrectionRequest) error {
	input.Definition, input.Reason = strings.TrimSpace(input.Definition), strings.TrimSpace(input.Reason)
	if input.ObjectiveID < 1 || input.ExpectedVersion < 0 || input.Definition == "" ||
		(input.ChangeType != "rutin" && input.ChangeType != "redaksi" && input.ChangeType != "substansi") ||
		!validSubmissionUUID(input.SubmissionKey) {
		return ValidationError("Data koreksi TP tidak valid.")
	}
	if input.ChangeType == "rutin" && input.Reason != "" {
		return ValidationError("Koreksi rutin tidak memakai alasan koreksi khusus.")
	}
	if input.ChangeType != "rutin" && !validLearningObjectiveReason(input.Reason) {
		return ValidationError("Alasan koreksi wajib diisi dan maksimal 500 karakter.")
	}
	var suppliedHash []byte
	if input.ChangeType == "rutin" {
		decoded, err := hex.DecodeString(input.SnapshotToken)
		if err != nil || len(decoded) != sha256.Size {
			return ValidationError("Pratinjau koreksi tidak valid.")
		}
		suppliedHash = decoded
	} else {
		decoded, err := hex.DecodeString(input.SnapshotToken)
		if err != nil || len(decoded) != sha256.Size {
			return ValidationError("Pratinjau koreksi tidak valid.")
		}
		suppliedHash = decoded
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
	var state LearningObjectiveEditState
	if err = readLearningObjectiveEditState(ctx, tx, teacherID, input.ObjectiveID, &state); err != nil {
		return err
	}
	requestPayload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	requestHash := sha256.Sum256(requestPayload)
	var existingHash []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM app.learning_objective_definition_revisions WHERE actor_id=$1 AND submission_key=$2::uuid`, actor, input.SubmissionKey).Scan(&existingHash)
	if err == nil {
		if !equalHash(existingHash, requestHash[:]) {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if input.ExpectedVersion != state.Version {
		return ErrConflict
	}
	if input.Definition == state.Objective.Definition {
		return ValidationError("Uraian baru sama dengan uraian aktif; tidak ada perubahan untuk dicatat.")
	}
	var impactSnapshot json.RawMessage
	var previewHash [32]byte
	if input.ChangeType == "rutin" {
		if !state.RoutineAllowed {
			return ErrConflict
		}
		currentSnapshotHash, decodeErr := hex.DecodeString(state.RoutineToken)
		if decodeErr != nil || !equalHash(suppliedHash, currentSnapshotHash) {
			return ErrConflict
		}
		previewHash = objectiveRoutineSnapshotHash(state, input.Definition)
		impactSnapshot, err = json.Marshal(map[string]any{
			"kind": "rutin", "objective_id": input.ObjectiveID, "version": input.ExpectedVersion,
			"definition_before": state.Objective.Definition, "definition_after": input.Definition,
		})
	} else {
		if !state.HasActivities && !state.HasScores && !state.HasFinals {
			return ErrConflict
		}
		impactSnapshot, err = readLearningObjectiveCorrectionSnapshot(ctx, tx, state)
		if err != nil {
			return err
		}
		currentToken, tokenErr := objectiveCorrectionSnapshotToken(learningObjectiveCorrectionSnapshotData{ObjectiveID: input.ObjectiveID,
			SemesterID: state.Objective.SemesterID, ClassID: state.Objective.ClassID, SubjectID: state.Objective.SubjectID,
			Version: state.Version, Impact: impactSnapshot}, input.Definition, input.ChangeType, input.Reason)
		if tokenErr != nil {
			return tokenErr
		}
		currentHash, _ := hex.DecodeString(currentToken)
		if !equalHash(suppliedHash, currentHash) {
			return ErrConflict
		}
		copy(previewHash[:], currentHash)
	}
	if err != nil {
		return err
	}
	newVersion := input.ExpectedVersion + 1
	updated, err := tx.ExecContext(ctx, `UPDATE app.learning_objectives SET definition=$2 WHERE id=$1 AND definition=$3`, input.ObjectiveID, input.Definition, state.Objective.Definition)
	if err != nil {
		return dbError(err)
	}
	updatedRows, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if updatedRows != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.learning_objective_definition_revisions
		(learning_objective_id,version,definition_before,definition_after,change_type,reason,actor_id,impact_snapshot,preview_sha256,submission_key,request_sha256)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10::uuid,$11)`,
		input.ObjectiveID, newVersion, state.Objective.Definition, input.Definition, input.ChangeType, input.Reason,
		actor, []byte(impactSnapshot), previewHash[:], input.SubmissionKey, requestHash[:]); err != nil {
		return dbError(err)
	}
	return tx.Commit()
}

func validLearningObjectiveReason(reason string) bool {
	return strings.TrimSpace(reason) != "" && utf8ValidReason(reason)
}

func readLearningObjectiveCorrectionSnapshot(ctx context.Context, tx *sql.Tx, state LearningObjectiveEditState) (json.RawMessage, error) {
	var raw []byte
	err := tx.QueryRowContext(ctx, learningObjectiveCorrectionImpactQuery,
		state.Objective.ID, state.Objective.SemesterID, state.Objective.ClassID, state.Objective.SubjectID,
		state.Version).Scan(&raw)
	return json.RawMessage(raw), err
}

const learningObjectiveCorrectionImpactQuery = `
SELECT jsonb_build_object(
	 'context',jsonb_build_object('semester_id',$2::bigint,'class_id',$3::bigint,'subject_id',$4::bigint),
	 'objective',jsonb_build_object('row',to_jsonb(lo),'version',$5::bigint),
 'assignments',coalesce((SELECT jsonb_agg(to_jsonb(ta) ORDER BY ta.id) FROM app.teaching_assignments ta WHERE ta.semester_id=$2 AND ta.class_id=$3 AND ta.subject_id=$4),'[]'::jsonb),
 'criteria',coalesce((SELECT jsonb_agg(to_jsonb(tc) ORDER BY tc.id) FROM app.tp_achievement_criteria tc WHERE tc.semester_id=$2 AND tc.class_id=$3 AND tc.subject_id=$4),'[]'::jsonb),
 'activities',coalesce((SELECT jsonb_agg(jsonb_build_object(
     'activity',to_jsonb(a),
     'objectives',coalesce((SELECT jsonb_agg(to_jsonb(ao) ORDER BY ao.learning_objective_id) FROM app.assessment_activity_objectives ao WHERE ao.activity_id=a.id),'[]'::jsonb),
     'scores',coalesce((SELECT jsonb_agg(jsonb_build_object(
         'initial',to_jsonb(sc),
         'corrections',coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.version,c.id) FROM app.assessment_activity_score_corrections c WHERE c.initial_score_id=sc.id),'[]'::jsonb),
         'remedials',coalesce((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.recorded_at,r.id) FROM app.assessment_activity_score_remedials r WHERE r.initial_score_id=sc.id),'[]'::jsonb)
       ) ORDER BY sc.student_id,sc.learning_objective_id,sc.id)
       FROM app.assessment_activity_initial_scores sc WHERE sc.assessment_activity_id=a.id),'[]'::jsonb)
   ) ORDER BY a.id) FROM app.assessment_activities a WHERE a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4),'[]'::jsonb),
	 'exam_sources',coalesce((SELECT jsonb_agg(jsonb_build_object(
     'initial',to_jsonb(e),
     'corrections',coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.version,c.id) FROM app.exam_score_corrections c WHERE c.initial_score_id=e.id),'[]'::jsonb),
     'remedials',coalesce((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.recorded_at,r.id) FROM app.exam_score_remedials r WHERE r.initial_score_id=e.id),'[]'::jsonb)
	   ) ORDER BY e.student_id,e.component,e.id) FROM app.exam_initial_scores e WHERE e.semester_id=$2 AND e.class_id=$3 AND e.subject_id=$4),'[]'::jsonb),
	 'placements',coalesce((SELECT jsonb_agg(jsonb_build_object('placement',to_jsonb(p),'moves',coalesce((
	   SELECT jsonb_agg(to_jsonb(m) ORDER BY m.effective_date,m.id) FROM app.student_class_moves m
	   WHERE m.placement_id=p.id AND (m.from_class_id=$3 OR m.to_class_id=$3)
	 ),'[]'::jsonb)) ORDER BY p.student_id,p.id) FROM app.student_placements p
	   JOIN app.semesters sm ON sm.id=$2 WHERE p.school_year_id=sm.school_year_id
	   AND (p.class_id=$3 OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.from_class_id=$3 OR m.to_class_id=$3)))),'[]'::jsonb),
 'finals',coalesce((SELECT jsonb_agg(to_jsonb(f) ORDER BY f.student_id) FROM (
   SELECT DISTINCT ON (student_id) id,student_id,version,final_type,recommendation_score,final_score,reason,snapshot,snapshot_sha256,recorded_by,teacher_id,recorded_at
   FROM app.semester_final_versions WHERE semester_id=$2 AND class_id=$3 AND subject_id=$4
   ORDER BY student_id,version DESC
 ) f),'[]'::jsonb)
 )::text
FROM app.learning_objectives lo WHERE lo.id=$1 AND lo.semester_id=$2 AND lo.class_id=$3 AND lo.subject_id=$4
`

func objectiveRoutineSnapshotToken(snapshot learningObjectiveCorrectionSnapshotData) string {
	data, _ := json.Marshal(struct {
		ObjectiveID, SemesterID, ClassID, SubjectID, Version int64
		Impact                                               json.RawMessage
	}{snapshot.ObjectiveID, snapshot.SemesterID, snapshot.ClassID, snapshot.SubjectID, snapshot.Version, snapshot.Impact})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func objectiveRoutineSnapshotHash(state LearningObjectiveEditState, definition string) [32]byte {
	var current []byte
	current, _ = hex.DecodeString(state.RoutineToken)
	return sha256.Sum256(append(append(current, 0), []byte(strings.TrimSpace(definition))...))
}

func objectiveCorrectionSnapshotToken(snapshot learningObjectiveCorrectionSnapshotData, definition, changeType, reason string) (string, error) {
	data, err := json.Marshal(struct {
		ObjectiveID, SemesterID, ClassID, SubjectID, Version int64
		Definition, ChangeType, Reason                       string
		Impact                                               json.RawMessage
	}{snapshot.ObjectiveID, snapshot.SemesterID, snapshot.ClassID, snapshot.SubjectID, snapshot.Version,
		strings.TrimSpace(definition), changeType, strings.TrimSpace(reason), snapshot.Impact})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// LearningObjectiveCorrectionSaved só autoriza notifikasi sukses jika audit
// yang cocok benar-benar tersimpan oleh aktor pada TP tersebut.
func (s *Store) LearningObjectiveCorrectionSaved(ctx context.Context, actor, objectiveID int64, key string) (bool, error) {
	if !validSubmissionUUID(key) {
		return false, nil
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return false, err
	}
	var found bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM app.learning_objectives lo JOIN app.teaching_assignments ta ON ta.semester_id=lo.semester_id AND ta.class_id=lo.class_id AND ta.subject_id=lo.subject_id AND ta.teacher_id=$1
		WHERE lo.id=$2 AND ta.effective_from <= (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date
		AND (ta.effective_until IS NULL OR ta.effective_until > (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date)
		AND EXISTS(SELECT 1 FROM app.learning_objective_definition_revisions r WHERE r.learning_objective_id=lo.id AND r.actor_id=$4 AND r.submission_key=$3::uuid)
	)`, teacher, objectiveID, key, actor).Scan(&found)
	if err != nil {
		return false, err
	}
	return found, nil
}

// LearningObjectiveDefinitionHistory hanya dapat dibaca guru pengampu aktif.
func (s *Store) LearningObjectiveDefinitionHistory(ctx context.Context, actor, objectiveID int64) (LearningObjectiveEditState, []LearningObjectiveDefinitionRevision, error) {
	var state LearningObjectiveEditState
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return state, nil, err
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return state, nil, err
	}
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return state, nil, err
	}
	if err = readLearningObjectiveEditState(ctx, tx, teacherID, objectiveID, &state); err != nil {
		return state, nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.version,r.definition_before,r.definition_after,r.change_type,r.reason,u.display_name,
		to_char(r.recorded_at AT TIME ZONE 'Asia/Jakarta','YYYY-MM-DD HH24:MI:SS'),encode(r.preview_sha256,'hex'),r.impact_snapshot::text
		FROM app.learning_objective_definition_revisions r JOIN app.users u ON u.id=r.actor_id
		WHERE r.learning_objective_id=$1 ORDER BY r.version DESC`, objectiveID)
	if err != nil {
		return state, nil, err
	}
	defer rows.Close()
	var history []LearningObjectiveDefinitionRevision
	for rows.Next() {
		var item LearningObjectiveDefinitionRevision
		if err = rows.Scan(&item.ID, &item.Version, &item.DefinitionBefore, &item.DefinitionAfter, &item.ChangeType, &item.Reason, &item.Actor, &item.At, &item.PreviewSHA256, &item.ImpactSnapshot); err != nil {
			return state, nil, err
		}
		history = append(history, item)
	}
	return state, history, rows.Err()
}

func learningObjectiveFinalImpactPage(ctx context.Context, tx *sql.Tx, state LearningObjectiveEditState, page int) ([]LearningObjectiveImpactFinal, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT v.student_id,st.name,v.final_score::text,v.version,v.final_type
		FROM (SELECT DISTINCT ON (student_id) student_id,final_score,version,final_type
		      FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3
		      ORDER BY student_id,version DESC) v
		JOIN app.students st ON st.id=v.student_id
		ORDER BY st.name,v.student_id OFFSET $4 LIMIT $5`, state.Objective.SemesterID, state.Objective.ClassID, state.Objective.SubjectID,
		(page-1)*learningObjectiveImpactPageSize, learningObjectiveImpactPageSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LearningObjectiveImpactFinal
	for rows.Next() {
		var item LearningObjectiveImpactFinal
		if err = rows.Scan(&item.StudentID, &item.StudentName, &item.FinalValue, &item.Version, &item.Type); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
