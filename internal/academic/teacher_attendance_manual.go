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

const teacherAttendanceManualTimeLayout = "2006-01-02T15:04"

type TeacherAttendanceManualInput struct {
	TeacherID      int64
	Action         TeacherAttendanceAction
	OccurredAt     string
	Reason         string
	IdempotencyKey string
}

type TeacherAttendanceManualProof struct {
	EventID, TeacherID, ActorID int64
	TeacherName, OnDate         string
	Action                      TeacherAttendanceAction
	OccurredAt, RecordedAt      time.Time
	Reason                      string
	Version                     int64
	Idempotent                  bool
}

type teacherAttendanceManualValidatedInput struct {
	TeacherID  int64
	Action     TeacherAttendanceAction
	OccurredAt time.Time
	OnDate     string
	Reason     string
	KeyHash    string
}

type teacherAttendanceManualEvidence struct {
	ActorID     int64                   `json:"actor_id"`
	TeacherID   int64                   `json:"teacher_id"`
	TeacherName string                  `json:"teacher_name"`
	OnDate      string                  `json:"on_date"`
	Action      TeacherAttendanceAction `json:"action"`
	Source      string                  `json:"source"`
	OccurredAt  string                  `json:"occurred_at"`
	Reason      string                  `json:"reason"`
}

type teacherAttendanceManualSnapshot struct {
	EventID             int64                   `json:"event_id"`
	TeacherID           int64                   `json:"teacher_id"`
	ActorID             int64                   `json:"actor_id"`
	TeacherName         string                  `json:"teacher_name"`
	OnDate              string                  `json:"on_date"`
	Action              TeacherAttendanceAction `json:"action"`
	Source              string                  `json:"source"`
	OccurredAt          time.Time               `json:"occurred_at"`
	RecordedAt          time.Time               `json:"recorded_at"`
	Reason              string                  `json:"reason"`
	Version             int64                   `json:"version"`
	IdempotencyKeyHash  string                  `json:"idempotency_key_hash"`
	EvidenceFingerprint string                  `json:"evidence_fingerprint"`
}

// RecordTeacherAttendanceManual melengkapi Masuk/Pulang yang terlewat oleh TU berwenang.
func (s *Store) RecordTeacherAttendanceManual(ctx context.Context, actorID int64, input TeacherAttendanceManualInput) (TeacherAttendanceManualProof, error) {
	validated, err := validateTeacherAttendanceManualInput(input)
	if err != nil {
		return TeacherAttendanceManualProof{}, err
	}
	tx, err := s.beginAny(ctx, actorID, "admin", "pengelola_kehadiran_guru")
	if err != nil {
		return TeacherAttendanceManualProof{}, err
	}
	defer tx.Rollback()

	var recordedAt time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&recordedAt); err != nil {
		return TeacherAttendanceManualProof{}, err
	}
	validated.OnDate = TeacherAttendanceWIBDate(validated.OccurredAt)
	if err = teacherAttendanceManagerAccess(ctx, tx, actorID, validated.TeacherID); err != nil {
		return TeacherAttendanceManualProof{}, err
	}

	teacherName, err := activeTeacherName(ctx, tx, validated.TeacherID)
	if err != nil {
		return TeacherAttendanceManualProof{}, err
	}

	var eventID int64
	err = tx.QueryRowContext(ctx, `
		SELECT e.id
		FROM app.teacher_attendance_events e
		JOIN app.teacher_attendance_history h ON h.event_id=e.id AND h.action='recorded'
		WHERE h.after_state->>'idempotency_key_hash'=$1
		ORDER BY h.id LIMIT 1`, validated.KeyHash).Scan(&eventID)
	if err == nil {
		proof, matched, retryErr := loadTeacherAttendanceManualRetry(ctx, tx, eventID, actorID, validated)
		if retryErr != nil {
			return TeacherAttendanceManualProof{}, retryErr
		}
		if !matched {
			return TeacherAttendanceManualProof{}, ErrConflict
		}
		proof.Idempotent = true
		return proof, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TeacherAttendanceManualProof{}, err
	}

	var existingID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM app.teacher_attendance_events
		WHERE teacher_id=$1 AND on_date=$2::date AND action=$3`,
		validated.TeacherID, validated.OnDate, string(validated.Action)).Scan(&existingID)
	if err == nil {
		return TeacherAttendanceManualProof{}, ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TeacherAttendanceManualProof{}, err
	}
	if err = validateTeacherAttendanceManualWindow(validated.OccurredAt, recordedAt); err != nil {
		return TeacherAttendanceManualProof{}, err
	}

	var version int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO app.teacher_attendance_events
			(teacher_id,teacher_name,on_date,action,source,actor_id,occurred_at,recorded_at,manual_reason)
		VALUES($1,$2,$3::date,$4,'manual',$5,$6,$7,$8)
		RETURNING id,version`,
		validated.TeacherID, teacherName, validated.OnDate, string(validated.Action), actorID,
		validated.OccurredAt, recordedAt, validated.Reason).Scan(&eventID, &version)
	if err != nil {
		return TeacherAttendanceManualProof{}, dbError(err)
	}

	evidence := teacherAttendanceManualEvidence{
		ActorID: actorID, TeacherID: validated.TeacherID, TeacherName: teacherName,
		OnDate: validated.OnDate, Action: validated.Action, Source: "manual",
		OccurredAt: canonicalTeacherAttendanceManualTime(validated.OccurredAt), Reason: validated.Reason,
	}
	fingerprint, err := teacherAttendanceManualEvidenceFingerprint(evidence)
	if err != nil {
		return TeacherAttendanceManualProof{}, ErrTeacherAttendanceWrite
	}
	snapshot := teacherAttendanceManualSnapshot{
		EventID: eventID, TeacherID: validated.TeacherID, ActorID: actorID,
		TeacherName: teacherName, OnDate: validated.OnDate, Action: validated.Action,
		Source: "manual", OccurredAt: validated.OccurredAt, RecordedAt: recordedAt,
		Reason: validated.Reason, Version: version, IdempotencyKeyHash: validated.KeyHash,
		EvidenceFingerprint: fingerprint,
	}
	afterState, err := json.Marshal(snapshot)
	if err != nil {
		return TeacherAttendanceManualProof{}, ErrTeacherAttendanceWrite
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO app.teacher_attendance_history(event_id,actor_id,action,after_state)
		VALUES($1,$2,'recorded',$3)`, eventID, actorID, string(afterState)); err != nil {
		return TeacherAttendanceManualProof{}, dbError(err)
	}
	if err = tx.Commit(); err != nil {
		if dbError(err) == ErrConflict {
			return TeacherAttendanceManualProof{}, ErrConflict
		}
		return TeacherAttendanceManualProof{}, ErrTeacherAttendanceCommit
	}
	return TeacherAttendanceManualProof{
		EventID: eventID, TeacherID: validated.TeacherID, ActorID: actorID,
		TeacherName: teacherName, OnDate: validated.OnDate, Action: validated.Action,
		OccurredAt: validated.OccurredAt, RecordedAt: recordedAt, Reason: validated.Reason,
		Version: version,
	}, nil
}

func validateTeacherAttendanceManualInput(input TeacherAttendanceManualInput) (teacherAttendanceManualValidatedInput, error) {
	validated := teacherAttendanceManualValidatedInput{
		TeacherID: input.TeacherID, Action: input.Action, Reason: strings.TrimSpace(input.Reason),
	}
	if validated.TeacherID < 1 {
		return validated, ValidationError("Guru wajib dipilih.")
	}
	if validated.Action != TeacherCheckIn && validated.Action != TeacherCheckOut {
		return validated, ErrTeacherAttendanceAction
	}
	if err := validateLocationReason(input.Reason); err != nil {
		return validated, err
	}
	occurredAt, err := parseTeacherAttendanceManualTime(input.OccurredAt)
	if err != nil {
		return validated, err
	}
	keyBytes, err := hex.DecodeString(input.IdempotencyKey)
	if err != nil || len(keyBytes) != 16 {
		return validated, ValidationError("Kunci pengiriman absensi tidak valid.")
	}
	keyHash := sha256.Sum256(keyBytes)
	validated.OccurredAt = occurredAt
	validated.OnDate = TeacherAttendanceWIBDate(occurredAt)
	validated.KeyHash = hex.EncodeToString(keyHash[:])
	return validated, nil
}

func parseTeacherAttendanceManualTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	parsed, err := time.ParseInLocation(teacherAttendanceManualTimeLayout, value, teacherAttendanceWIBLocation())
	if err != nil {
		return time.Time{}, ValidationError("Waktu kejadian harus diisi dengan tanggal dan waktu WIB.")
	}
	return parsed, nil
}

func validateTeacherAttendanceManualWindow(occurredAt, recordedAt time.Time) error {
	if occurredAt.After(recordedAt) {
		return ValidationError("Waktu kejadian tidak boleh di masa depan.")
	}
	return nil
}

func activeTeacherName(ctx context.Context, tx *sql.Tx, teacherID int64) (string, error) {
	var name string
	err := tx.QueryRowContext(ctx, `SELECT name FROM app.teachers WHERE id=$1 AND active FOR KEY SHARE`, teacherID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

func loadTeacherAttendanceManualRetry(ctx context.Context, tx *sql.Tx, eventID, actorID int64, input teacherAttendanceManualValidatedInput) (TeacherAttendanceManualProof, bool, error) {
	var proof TeacherAttendanceManualProof
	var source string
	var snapshotJSON []byte
	err := tx.QueryRowContext(ctx, `
		SELECT e.id,e.teacher_id,e.teacher_name,e.on_date::text,e.action,e.source,e.actor_id,
		       e.occurred_at,e.recorded_at,e.manual_reason,e.version,h.after_state
		FROM app.teacher_attendance_events e
		JOIN LATERAL (
		  SELECT after_state FROM app.teacher_attendance_history
		  WHERE event_id=e.id AND action='recorded' ORDER BY id LIMIT 1
		) h ON true
		WHERE e.id=$1`, eventID).Scan(
		&proof.EventID, &proof.TeacherID, &proof.TeacherName, &proof.OnDate, &proof.Action,
		&source, &proof.ActorID, &proof.OccurredAt, &proof.RecordedAt, &proof.Reason,
		&proof.Version, &snapshotJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return proof, false, ErrConflict
	}
	if err != nil {
		return proof, false, err
	}
	if source != "manual" || proof.TeacherID != input.TeacherID || proof.ActorID != actorID ||
		proof.Action != input.Action || proof.OnDate != input.OnDate || proof.Reason != input.Reason ||
		!proof.OccurredAt.Equal(input.OccurredAt) {
		return proof, false, nil
	}
	var snapshot teacherAttendanceManualSnapshot
	if err = json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return proof, false, ErrConflict
	}
	if !teacherAttendanceManualSnapshotMatches(snapshot, proof, input.KeyHash) {
		return proof, false, ErrConflict
	}
	evidence := teacherAttendanceManualEvidence{
		ActorID: actorID, TeacherID: proof.TeacherID, TeacherName: proof.TeacherName,
		OnDate: proof.OnDate, Action: proof.Action, Source: source,
		OccurredAt: canonicalTeacherAttendanceManualTime(proof.OccurredAt), Reason: proof.Reason,
	}
	fingerprint, err := teacherAttendanceManualEvidenceFingerprint(evidence)
	if err != nil {
		return proof, false, ErrConflict
	}
	if fingerprint != snapshot.EvidenceFingerprint {
		return proof, false, ErrConflict
	}
	return proof, true, nil
}

func teacherAttendanceManualSnapshotMatches(snapshot teacherAttendanceManualSnapshot, proof TeacherAttendanceManualProof, keyHash string) bool {
	return snapshot.EventID == proof.EventID && snapshot.TeacherID == proof.TeacherID &&
		snapshot.ActorID == proof.ActorID && snapshot.TeacherName == proof.TeacherName &&
		snapshot.OnDate == proof.OnDate && snapshot.Action == proof.Action && snapshot.Source == "manual" &&
		snapshot.OccurredAt.Equal(proof.OccurredAt) && snapshot.RecordedAt.Equal(proof.RecordedAt) &&
		snapshot.Reason == proof.Reason && snapshot.Version == proof.Version &&
		snapshot.IdempotencyKeyHash == keyHash && snapshot.EvidenceFingerprint != ""
}

func canonicalTeacherAttendanceManualTime(value time.Time) string {
	return TeacherAttendanceWIBTime(value).Format(time.RFC3339Nano)
}

func teacherAttendanceManualEvidenceFingerprint(evidence teacherAttendanceManualEvidence) (string, error) {
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return "", err
	}
	checksum := sha256.Sum256(encoded)
	return hex.EncodeToString(checksum[:]), nil
}
