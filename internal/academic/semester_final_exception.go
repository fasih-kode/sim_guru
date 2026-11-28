package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// FinalizeSemesterException menetapkan Final manual hanya jika bukti sah
// berbobot positif masih ada dan rekomendasi belum lengkap.
func (s *Store) FinalizeSemesterException(ctx context.Context, actor, semester, class, subject, studentID int64, score int, reason, token, key string) error {
	if semester < 1 || class < 1 || subject < 1 || studentID < 1 || score < 0 || score > 100 {
		return ValidationError("Nilai Final harus berupa bilangan bulat 0–100.")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || !utf8ValidReason(reason) {
		return ValidationError("Alasan pengecualian wajib diisi dan maksimal 500 karakter.")
	}
	hash, err := hex.DecodeString(token)
	if err != nil || len(hash) != 32 {
		return ValidationError("Pratinjau tidak valid. Muat ulang sebelum menyimpan Final.")
	}
	if !validSubmissionUUID(key) {
		return ValidationError("Kunci pengiriman tidak valid. Muat ulang pratinjau.")
	}
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	if _, err = tx.ExecContext(ctx, `SELECT academic_write.lock_final_assignment($1,$2,$3,$4)`, actor, semester, class, subject); err != nil {
		return academicWriterError(err)
	}
	payload := struct {
		Type  string
		Final finalizationPayload
	}{"exception", finalizationPayload{semester, class, subject, studentID, score, reason, token}}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	requestHash := sha256.Sum256(raw)
	var previous []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM app.semester_final_versions WHERE recorded_by=$1 AND submission_key=$2::uuid`, actor, key).Scan(&previous)
	if err == nil {
		if !equalHash(previous, requestHash[:]) {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	result, err := s.semesterRecommendationsTx(ctx, tx, actor, semester, class, subject)
	if err != nil {
		return err
	}
	var student *SemesterRecommendationStudent
	for i := range result.Students {
		if result.Students[i].ID == strconv.FormatInt(studentID, 10) {
			student = &result.Students[i]
			break
		}
	}
	if student == nil {
		return ErrForbidden
	}
	snapshot, snapshotHash, err := semesterFinalInputSnapshot(ctx, tx, semester, class, subject, *student)
	if err != nil {
		return err
	}
	if !equalHash(hash, snapshotHash[:]) {
		return ErrConflict
	}
	if !result.Weights.Configured || !student.ExceptionEligible || student.Status != "Belum Lengkap" {
		return ValidationError("Pengecualian hanya untuk siswa Belum Lengkap dengan bukti Sumatif sah berbobot positif.")
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4)`, semester, class, subject, studentID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `SELECT academic_write.append_final_version(
		$1,$2,$3,$4,$5,0,'exception',NULL,$6,$7,$8,$9,$10::uuid,$11)`,
		actor, semester, class, subject, studentID, score, reason, snapshot, snapshotHash[:], key, requestHash[:]); err != nil {
		return academicWriterError(dbError(err))
	}
	return tx.Commit()
}
