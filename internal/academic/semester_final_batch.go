package academic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// SemesterFinalBatchItem mengikat siswa dengan bukti pratinjau individual.
type SemesterFinalBatchItem struct {
	StudentID     int64
	SnapshotToken string
	Student       SemesterRecommendationStudent
}

type SemesterFinalBatchPreview struct {
	Context     LearningObjectiveContext
	Weights     AssessmentWeights
	TeacherName string
	Items       []SemesterFinalBatchItem
}

func normalizedFinalBatch(items []SemesterFinalBatchItem) ([]SemesterFinalBatchItem, error) {
	if len(items) == 0 || len(items) > 500 {
		return nil, ValidationError("Pilih antara 1 dan 500 siswa.")
	}
	out := append([]SemesterFinalBatchItem(nil), items...)
	sort.Slice(out, func(i, j int) bool { return out[i].StudentID < out[j].StudentID })
	for i, item := range out {
		if item.StudentID < 1 || (i > 0 && out[i-1].StudentID == item.StudentID) {
			return nil, ValidationError("Daftar siswa tidak valid atau duplikat.")
		}
	}
	return out, nil
}

func (s *Store) semesterFinalBatchRead(ctx context.Context, tx *sql.Tx, actor, semester, class, subject int64, items []SemesterFinalBatchItem) (SemesterFinalBatchPreview, error) {
	var out SemesterFinalBatchPreview
	result, err := s.semesterRecommendationsTx(ctx, tx, actor, semester, class, subject)
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT teacher_name FROM academic_write.final_batch_actor($1,NULL)`, actor).Scan(&out.TeacherName)
	if err != nil {
		return out, err
	}
	out.Context = result.Context
	out.Weights = result.Weights
	students := make(map[int64]SemesterRecommendationStudent, len(result.Students))
	for _, st := range result.Students {
		id, e := strconv.ParseInt(st.ID, 10, 64)
		if e != nil {
			return out, e
		}
		students[id] = st
	}
	for _, item := range items {
		student, ok := students[item.StudentID]
		if !ok {
			return out, ErrForbidden
		}
		_, hash, e := semesterFinalInputSnapshot(ctx, tx, semester, class, subject, student)
		if e != nil {
			return out, e
		}
		item.Student = student
		item.SnapshotToken = hex.EncodeToString(hash[:])
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// SemesterFinalBatchPreview membaca seluruh bukti dalam satu snapshot transaksi.
func (s *Store) SemesterFinalBatchPreview(ctx context.Context, actor, semester, class, subject int64, items []SemesterFinalBatchItem) (SemesterFinalBatchPreview, error) {
	items, err := normalizedFinalBatch(items)
	if err != nil {
		return SemesterFinalBatchPreview{}, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return SemesterFinalBatchPreview{}, err
	}
	defer tx.Rollback()
	out, err := s.semesterFinalBatchRead(ctx, tx, actor, semester, class, subject, items)
	if err != nil {
		return out, err
	}
	for _, item := range out.Items {
		var finalized bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4)`, semester, class, subject, item.StudentID).Scan(&finalized); err != nil {
			return out, err
		}
		if finalized || item.Student.Status != "Siap Final" || item.Student.Recommendation == "" || !out.Weights.Configured {
			return out, ValidationError("Seluruh siswa harus berstatus Siap Final dan belum memiliki Final.")
		}
	}
	return out, nil
}

// FinalizeSemesterBatch memvalidasi semua siswa sebelum menyimpan batch atomik.
func (s *Store) FinalizeSemesterBatch(ctx context.Context, actor, semester, class, subject int64, items []SemesterFinalBatchItem, key string) error {
	items, err := normalizedFinalBatch(items)
	if err != nil {
		return err
	}
	if !validSubmissionUUID(key) {
		return ValidationError("Kunci pengiriman tidak valid.")
	}
	type requestItem struct {
		StudentID     int64
		SnapshotToken string
	}
	payload := struct {
		Semester, Class, Subject int64
		Items                    []requestItem
	}{Semester: semester, Class: class, Subject: subject}
	for _, item := range items {
		h, e := hex.DecodeString(item.SnapshotToken)
		if e != nil || len(h) != 32 {
			return ValidationError("Pratinjau tidak valid.")
		}
		payload.Items = append(payload.Items, requestItem{item.StudentID, item.SnapshotToken})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	requestHash := sha256.Sum256(raw)
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
	var old []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM academic_write.final_batch_actor($1,$2::uuid)`, actor, key).Scan(&old)
	if err != nil {
		return academicWriterError(err)
	}
	if old != nil {
		if !equalHash(old, requestHash[:]) {
			return ErrConflict
		}
		return tx.Commit()
	}
	preview, err := s.semesterFinalBatchRead(ctx, tx, actor, semester, class, subject, items)
	if err != nil {
		return err
	}
	type prepared struct {
		Student     int64  `json:"student"`
		Score       int    `json:"score"`
		Snapshot    []byte `json:"snapshot"`
		Hash        []byte `json:"snapshot_hash"`
		Submission  string `json:"submission"`
		RequestHash []byte `json:"request_hash"`
	}
	ready := make([]prepared, 0, len(items))
	for i, item := range preview.Items {
		if item.SnapshotToken != items[i].SnapshotToken {
			return ErrConflict
		}
		if item.Student.Status != "Siap Final" || item.Student.Recommendation == "" || !preview.Weights.Configured {
			return ValidationError("Seluruh siswa harus berstatus Siap Final.")
		}
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4)`, semester, class, subject, item.StudentID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrConflict
		}
		snapshot, hash, e := semesterFinalInputSnapshot(ctx, tx, semester, class, subject, item.Student)
		if e != nil {
			return e
		}
		score, e := strconv.Atoi(item.Student.Recommendation)
		if e != nil {
			return fmt.Errorf("rekomendasi Final tidak valid: %w", e)
		}
		var uuid [16]byte
		if _, err = rand.Read(uuid[:]); err != nil {
			return err
		}
		uuid[6] = uuid[6]&0x0f | 0x40
		uuid[8] = uuid[8]&0x3f | 0x80
		h := hex.EncodeToString(uuid[:])
		submission := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
		request, _ := json.Marshal(finalizationPayload{semester, class, subject, item.StudentID, score, "", item.SnapshotToken})
		individualHash := sha256.Sum256(request)
		ready = append(ready, prepared{item.StudentID, score, snapshot, hash[:], submission, individualHash[:]})
	}
	batch, err := json.Marshal(ready)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `SELECT academic_write.append_final_batch($1,$2,$3,$4,$5::uuid,$6,$7::jsonb)`,
		actor, semester, class, subject, key, requestHash[:], string(batch)); err != nil {
		return academicWriterError(dbError(err))
	}
	return tx.Commit()
}
