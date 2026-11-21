package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// HistoricalTPVerification mencatat pemeriksaan substansi oleh guru yang berwenang.
type HistoricalTPVerification struct {
	LearningSection, AssessmentSection, Method, ResultSource string
	Acknowledged                                             bool
}

type HistoricalTPConfirmation struct {
	Preview                      HistoricalTPPreviewInput
	Verification                 HistoricalTPVerification
	SnapshotToken, SubmissionKey string
}

// ConfirmHistoricalLearningObjective menyimpan TP dan provenance atomik setelah
// pemeriksaan ulang; bukti arsip diperiksa manusia dan tidak mengimpor nilai.
func (s *Store) ConfirmHistoricalLearningObjective(ctx context.Context, actor int64, input HistoricalTPConfirmation) (int64, error) {
	if err := validateHistoricalTPPreviewInput(input.Preview); err != nil {
		return 0, err
	}
	if !input.Verification.Acknowledged {
		return 0, ValidationError("Konfirmasi pemeriksaan sumber wajib diberikan.")
	}
	for _, value := range []string{input.Verification.LearningSection, input.Verification.AssessmentSection, input.Verification.Method, input.Verification.ResultSource} {
		if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 500 {
			return 0, ValidationError("Bagian bukti, metode pemeriksaan, dan sumber hasil wajib diisi.")
		}
	}
	if input.Preview.AssessmentDate == "" || strings.TrimSpace(input.Preview.DateSource) == "" || !validSubmissionUUID(input.SubmissionKey) {
		return 0, ValidationError("Tanggal, asal tanggal, dan identitas permintaan wajib tersedia.")
	}
	token, err := hex.DecodeString(input.SnapshotToken)
	if err != nil || len(token) != sha256.Size {
		return 0, ValidationError("Pratinjau tidak valid.")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	requestHash := sha256.Sum256(payload)
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	id, err := s.confirmHistoricalLearningObjectiveTx(ctx, tx, actor, input, token, requestHash)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, dbError(err)
	}
	return id, nil
}

func (s *Store) confirmHistoricalLearningObjectiveTx(ctx context.Context, tx *sql.Tx, actor int64, input HistoricalTPConfirmation, token []byte, requestHash [32]byte) (int64, error) {
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return 0, err
	}
	p := input.Preview
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, p.SemesterID, p.ClassID, p.SubjectID)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, ErrForbidden
	}
	var existingID int64
	var oldHash []byte
	err = tx.QueryRowContext(ctx, `SELECT learning_objective_id,request_sha256 FROM app.historical_learning_objective_insertions WHERE actor_id=$1 AND submission_key=$2::uuid`, actor, input.SubmissionKey).Scan(&existingID, &oldHash)
	if err == nil {
		if !equalHash(oldHash, requestHash[:]) {
			return 0, ErrConflict
		}
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var today string
	if err = tx.QueryRowContext(ctx, `SELECT (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&today); err != nil {
		return 0, err
	}
	if p.AssessmentDate > today || (p.Learning.Date != "" && p.Learning.Date > today) {
		return 0, ValidationError("Tanggal bukti historis tidak boleh di masa depan.")
	}
	preview, err := s.previewHistoricalTPTx(ctx, tx, actor, p, 1, input.SnapshotToken)
	if err != nil {
		return 0, err
	}
	if preview.LearningStatus == "Referensi tidak memadai" || preview.AssessmentStatus == "Referensi tidak memadai" {
		return 0, ValidationError("Bukti pembelajaran dan Sumatif wajib dapat ditelusuri.")
	}
	if p.Learning.Kind == "app" {
		var held bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_journals j JOIN app.teaching_journal_versions v ON v.journal_id=j.id AND v.version_no=j.current_version_no WHERE j.id=$1 AND v.state='final' AND v.outcome='held' AND j.meeting_date<=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date)`, p.Learning.SourceID).Scan(&held)
		if err != nil {
			return 0, err
		}
		if !held {
			return 0, ValidationError("Jurnal aplikasi harus final dan terlaksana.")
		}
	}
	_, learningRaw, err := historicalTPReferenceStatus(ctx, tx, actor, teacher, p, true)
	if err != nil {
		return 0, err
	}
	_, assessmentRaw, err := historicalTPReferenceStatus(ctx, tx, actor, teacher, p, false)
	if err != nil {
		return 0, err
	}
	learning, err := json.Marshal(struct {
		Reference HistoricalTPReference `json:"reference"`
		Metadata  string                `json:"metadata"`
	}{p.Learning, learningRaw})
	if err != nil {
		return 0, err
	}
	assessment, err := json.Marshal(struct {
		Reference HistoricalTPReference `json:"reference"`
		Metadata  string                `json:"metadata"`
	}{p.Assessment, assessmentRaw})
	if err != nil {
		return 0, err
	}
	verification, err := json.Marshal(input.Verification)
	if err != nil {
		return 0, err
	}
	allStudents := append([]HistoricalTPPreviewStudent(nil), preview.Students...)
	for page := 2; page <= preview.PageCount; page++ {
		part, pageErr := s.previewHistoricalTPTx(ctx, tx, actor, p, page, input.SnapshotToken)
		if pageErr != nil {
			return 0, pageErr
		}
		allStudents = append(allStudents, part.Students...)
	}
	impact, err := json.Marshal(struct {
		Preview  HistoricalTPPreview          `json:"preview"`
		Students []HistoricalTPPreviewStudent `json:"all_students"`
	}{preview, allStudents})
	if err != nil {
		return 0, err
	}
	var year, id int64
	if err = tx.QueryRowContext(ctx, `SELECT school_year_id FROM app.semesters WHERE id=$1`, p.SemesterID).Scan(&year); err != nil {
		return 0, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT nextval(pg_get_serial_sequence('app.learning_objectives','id'))`).Scan(&id); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.historical_learning_objective_insertions(learning_objective_id,semester_id,class_id,subject_id,actor_id,teacher_id,assessment_date,reason,learning_evidence,assessment_evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11::jsonb,$12::jsonb,$13,$14,$15::uuid)`, id, p.SemesterID, p.ClassID, p.SubjectID, actor, teacher, p.AssessmentDate, strings.TrimSpace(p.Reason), string(learning), string(assessment), string(verification), string(impact), token, requestHash[:], input.SubmissionKey)
	if err != nil {
		return 0, dbError(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.learning_objectives(id,school_year_id,semester_id,class_id,subject_id,definition,created_by) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,$7)`, id, year, p.SemesterID, p.ClassID, p.SubjectID, strings.TrimSpace(p.Definition), actor)
	if err != nil {
		return 0, dbError(err)
	}
	return id, nil
}

// HistoricalLearningObjectiveResult membatasi hasil tersimpan pada pencatat berhak.
func (s *Store) HistoricalLearningObjectiveResult(ctx context.Context, actor, id int64) (string, int64, error) {
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return "", 0, err
	}
	var semester, class, subject, auditID int64
	var definition string
	err = tx.QueryRowContext(ctx, `SELECT h.semester_id,h.class_id,h.subject_id,o.definition,h.id FROM app.historical_learning_objective_insertions h JOIN app.learning_objectives o ON o.id=h.learning_objective_id WHERE h.learning_objective_id=$1 AND h.actor_id=$2 AND h.teacher_id=$3`, id, actor, teacher).Scan(&semester, &class, &subject, &definition, &auditID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrForbidden
	}
	if err != nil {
		return "", 0, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return "", 0, err
	}
	if !allowed {
		return "", 0, ErrForbidden
	}
	return definition, auditID, nil
}
