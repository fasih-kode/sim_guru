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
	err = tx.QueryRowContext(ctx, `SELECT learning_objective_id,request_sha256 FROM academic_write.lookup_historical_tp_replay($1,$2::uuid)`, actor, input.SubmissionKey).Scan(&existingID, &oldHash)
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
	payload, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT academic_write.confirm_historical_tp($1,$2,$3)`,
		actor, payload, preview.displayHash[:]).Scan(&id)
	if err != nil {
		return 0, academicWriterError(dbError(err))
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
	var definition string
	var auditID int64
	err = tx.QueryRowContext(ctx, `SELECT definition,audit_id FROM academic_write.historical_tp_result($1,$2)`,
		actor, id).Scan(&definition, &auditID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrForbidden
	}
	return definition, auditID, academicWriterError(err)
}
