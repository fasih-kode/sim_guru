package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type TeacherAttendanceCorrectionInput struct {
	EventID, ExpectedVersion int64
	OccurredAt               string
	ManualReason             *string
	CorrectionReason         string
}

type TeacherAttendanceCorrectionResult struct {
	EventID, TeacherID, RecordedBy, CorrectedBy int64
	TeacherName, OnDate, Source                 string
	Action                                      TeacherAttendanceAction
	OccurredAt, RecordedAt, CorrectedAt         time.Time
	ManualReason                                string
	Version                                     int64
	Noop                                        bool
}

type teacherAttendanceCorrectionSnapshot struct {
	EventID         int64                   `json:"event_id"`
	TeacherID       int64                   `json:"teacher_id"`
	TeacherName     string                  `json:"teacher_name"`
	OnDate          string                  `json:"on_date"`
	Action          TeacherAttendanceAction `json:"action"`
	Source          string                  `json:"source"`
	RecordedBy      int64                   `json:"recorded_by"`
	OccurredAt      time.Time               `json:"occurred_at"`
	RecordedAt      time.Time               `json:"recorded_at"`
	Version         int64                   `json:"version"`
	DistanceM       *float64                `json:"distance_m,omitempty"`
	AccuracyM       *float64                `json:"accuracy_m,omitempty"`
	SchoolLatitude  *float64                `json:"school_latitude,omitempty"`
	SchoolLongitude *float64                `json:"school_longitude,omitempty"`
	GeofenceRadiusM *int                    `json:"geofence_radius_m,omitempty"`
	PhotoMetadataID *int64                  `json:"photo_metadata_id,omitempty"`
	PhotoMIMEType   *string                 `json:"photo_mime_type,omitempty"`
	PhotoSizeBytes  *int                    `json:"photo_size_bytes,omitempty"`
	PhotoChecksum   *string                 `json:"photo_checksum_sha256,omitempty"`
	PhotoCreatedAt  *time.Time              `json:"photo_created_at,omitempty"`
	RetainUntil     *time.Time              `json:"retain_until,omitempty"`
	PhotoDeletedAt  *time.Time              `json:"photo_deleted_at,omitempty"`
	CleanupAttempts *int                    `json:"photo_cleanup_attempts,omitempty"`
	ManualReason    *string                 `json:"manual_reason,omitempty"`
	Reason          string                  `json:"reason,omitempty"`
}

type teacherAttendanceCorrectionEvent struct {
	ID, TeacherID, RecordedBy, Version          int64
	TeacherName, OnDate, Source                 string
	Action                                      TeacherAttendanceAction
	OccurredAt, RecordedAt                      time.Time
	ManualReason                                sql.NullString
	DistanceM, AccuracyM                        sql.NullFloat64
	SchoolLatitude, SchoolLongitude             sql.NullFloat64
	GeofenceRadiusM, PhotoMetadataID            sql.NullInt64
	PhotoMIMEType, PhotoChecksum                sql.NullString
	PhotoSizeBytes                              sql.NullInt64
	PhotoCreatedAt, RetainUntil, PhotoDeletedAt sql.NullTime
	PhotoCleanupAttempts                        sql.NullInt64
}

type teacherAttendanceCorrectionValidatedInput struct {
	EventID, ExpectedVersion int64
	OccurredAt               time.Time
	ManualReason             *string
	CorrectionReason         string
}

// CorrectTeacherAttendance mengoreksi catatan dengan histori tanpa mengubah bukti awal.
func (s *Store) CorrectTeacherAttendance(ctx context.Context, actorID int64, input TeacherAttendanceCorrectionInput) (TeacherAttendanceCorrectionResult, error) {
	validated, err := validateTeacherAttendanceCorrectionInput(input)
	if err != nil {
		return TeacherAttendanceCorrectionResult{}, err
	}
	tx, err := s.beginAny(ctx, actorID, "admin", "pengelola_kehadiran_guru")
	if err != nil {
		return TeacherAttendanceCorrectionResult{}, err
	}
	defer tx.Rollback()

	current, err := loadTeacherAttendanceCorrectionEvent(ctx, tx, validated.EventID)
	if errors.Is(err, sql.ErrNoRows) {
		return TeacherAttendanceCorrectionResult{}, ErrNotFound
	}
	if err != nil {
		return TeacherAttendanceCorrectionResult{}, err
	}
	if err = teacherAttendanceManagerAccess(ctx, tx, actorID, current.TeacherID); err != nil {
		return TeacherAttendanceCorrectionResult{}, err
	}
	if current.Version != validated.ExpectedVersion {
		return TeacherAttendanceCorrectionResult{}, ErrConflict
	}

	var serverNow time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&serverNow); err != nil {
		return TeacherAttendanceCorrectionResult{}, err
	}
	if err = validateTeacherAttendanceCorrectionTime(validated.OccurredAt, current.OnDate, serverNow); err != nil {
		return TeacherAttendanceCorrectionResult{}, err
	}
	newManualReason, err := teacherAttendanceCorrectionManualReason(current.Source, current.ManualReason, validated.ManualReason)
	if err != nil {
		return TeacherAttendanceCorrectionResult{}, err
	}
	result := teacherAttendanceCorrectionResult(current, actorID)
	result.OccurredAt = validated.OccurredAt
	result.ManualReason = newManualReason
	if current.OccurredAt.Equal(validated.OccurredAt) && current.ManualReason.String == newManualReason && current.ManualReason.Valid == (current.Source == "manual") {
		result.Noop = true
		result.CorrectedBy = 0
		return result, nil
	}

	beforeState, err := json.Marshal(current.snapshot())
	if err != nil {
		return TeacherAttendanceCorrectionResult{}, ErrTeacherAttendanceWrite
	}
	current.OccurredAt = validated.OccurredAt
	current.Version++
	if current.Source == "manual" {
		current.ManualReason = sql.NullString{String: newManualReason, Valid: true}
	}
	afterState, err := json.Marshal(current.snapshot())
	if err != nil {
		return TeacherAttendanceCorrectionResult{}, ErrTeacherAttendanceWrite
	}

	if current.Source == "manual" {
		err = tx.QueryRowContext(ctx, `
			UPDATE app.teacher_attendance_events
			SET occurred_at=$1,manual_reason=$2,version=version+1
			WHERE id=$3 AND version=$4
			RETURNING version`, validated.OccurredAt, newManualReason, current.ID, validated.ExpectedVersion).Scan(&result.Version)
	} else {
		err = tx.QueryRowContext(ctx, `
			UPDATE app.teacher_attendance_events
			SET occurred_at=$1,version=version+1
			WHERE id=$2 AND version=$3
			RETURNING version`, validated.OccurredAt, current.ID, validated.ExpectedVersion).Scan(&result.Version)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return TeacherAttendanceCorrectionResult{}, ErrConflict
	}
	if err != nil {
		return TeacherAttendanceCorrectionResult{}, dbError(err)
	}
	if err = tx.QueryRowContext(ctx, `
		INSERT INTO app.teacher_attendance_history(event_id,actor_id,action,before_state,after_state,reason)
		VALUES($1,$2,'corrected',$3::jsonb,$4::jsonb,$5)
		RETURNING created_at`, current.ID, actorID, string(beforeState), string(afterState), validated.CorrectionReason).Scan(&result.CorrectedAt); err != nil {
		return TeacherAttendanceCorrectionResult{}, dbError(err)
	}
	if err = tx.Commit(); err != nil {
		return TeacherAttendanceCorrectionResult{}, ErrTeacherAttendanceCommit
	}
	result.Version = current.Version
	return result, nil
}

func validateTeacherAttendanceCorrectionInput(input TeacherAttendanceCorrectionInput) (teacherAttendanceCorrectionValidatedInput, error) {
	validated := teacherAttendanceCorrectionValidatedInput{EventID: input.EventID, ExpectedVersion: input.ExpectedVersion}
	if input.EventID < 1 {
		return validated, ErrNotFound
	}
	if input.ExpectedVersion < 1 {
		return validated, ValidationError("Versi catatan kehadiran tidak valid.")
	}
	occurredAt, err := parseTeacherAttendanceManualTime(input.OccurredAt)
	if err != nil {
		return validated, err
	}
	correctionReason, err := validateTeacherAttendanceCorrectionReason(input.CorrectionReason, "Alasan koreksi wajib diisi, maksimal 500 karakter.")
	if err != nil {
		return validated, err
	}
	if input.ManualReason != nil {
		manualReason, reasonErr := validateTeacherAttendanceCorrectionReason(*input.ManualReason, "Alasan pencatatan manual wajib diisi, maksimal 500 karakter.")
		if reasonErr != nil {
			return validated, ValidationError("Alasan pencatatan manual wajib diisi, maksimal 500 karakter.")
		}
		validated.ManualReason = &manualReason
	}
	validated.OccurredAt = occurredAt
	validated.CorrectionReason = correctionReason
	return validated, nil
}

func validateTeacherAttendanceCorrectionReason(value, message string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 500 || strings.ContainsRune(value, 0) {
		return "", ValidationError(message)
	}
	return value, nil
}

func validateTeacherAttendanceCorrectionTime(occurredAt time.Time, onDate string, serverNow time.Time) error {
	if occurredAt.After(serverNow) {
		return ValidationError("Waktu kejadian tidak boleh di masa depan.")
	}
	if TeacherAttendanceWIBDate(occurredAt) != onDate {
		return ValidationError("Waktu koreksi harus tetap pada tanggal kehadiran yang sama dalam WIB.")
	}
	return nil
}

func teacherAttendanceCorrectionManualReason(source string, current sql.NullString, requested *string) (string, error) {
	if source == "gps_selfie" {
		if requested != nil {
			return "", ValidationError("Alasan pencatatan manual tidak berlaku untuk catatan GPS/selfie.")
		}
		return "", nil
	}
	if source != "manual" {
		return "", ErrConflict
	}
	if requested == nil {
		if !current.Valid {
			return "", ErrConflict
		}
		return current.String, nil
	}
	return *requested, nil
}

func loadTeacherAttendanceCorrectionEvent(ctx context.Context, tx *sql.Tx, eventID int64) (teacherAttendanceCorrectionEvent, error) {
	var current teacherAttendanceCorrectionEvent
	err := tx.QueryRowContext(ctx, `
		SELECT e.id,e.teacher_id,e.teacher_name,e.on_date::text,e.action,e.source,e.actor_id,
		       e.occurred_at,e.recorded_at,e.version,e.manual_reason,e.distance_m,e.accuracy_m,
		       e.school_latitude,e.school_longitude,e.geofence_radius_m,e.photo_metadata_id,
	       p.mime_type,p.size_bytes,p.checksum_sha256,p.created_at,p.retain_until,p.deleted_at,p.cleanup_attempts
		FROM app.teacher_attendance_events e
		LEFT JOIN app.teacher_attendance_photos p ON p.id=e.photo_metadata_id AND p.event_id=e.id
		WHERE e.id=$1
		FOR UPDATE OF e`, eventID).Scan(
		&current.ID, &current.TeacherID, &current.TeacherName, &current.OnDate, &current.Action,
		&current.Source, &current.RecordedBy, &current.OccurredAt, &current.RecordedAt, &current.Version,
		&current.ManualReason, &current.DistanceM, &current.AccuracyM, &current.SchoolLatitude,
		&current.SchoolLongitude, &current.GeofenceRadiusM, &current.PhotoMetadataID,
		&current.PhotoMIMEType, &current.PhotoSizeBytes, &current.PhotoChecksum, &current.PhotoCreatedAt,
		&current.RetainUntil, &current.PhotoDeletedAt, &current.PhotoCleanupAttempts)
	return current, err
}

func (current teacherAttendanceCorrectionEvent) snapshot() teacherAttendanceCorrectionSnapshot {
	snapshot := teacherAttendanceCorrectionSnapshot{
		EventID: current.ID, TeacherID: current.TeacherID, TeacherName: current.TeacherName,
		OnDate: current.OnDate, Action: current.Action, Source: current.Source, RecordedBy: current.RecordedBy,
		OccurredAt: current.OccurredAt, RecordedAt: current.RecordedAt, Version: current.Version,
	}
	snapshot.DistanceM = nullableFloat64(current.DistanceM)
	snapshot.AccuracyM = nullableFloat64(current.AccuracyM)
	snapshot.SchoolLatitude = nullableFloat64(current.SchoolLatitude)
	snapshot.SchoolLongitude = nullableFloat64(current.SchoolLongitude)
	snapshot.GeofenceRadiusM = nullableInt(current.GeofenceRadiusM)
	snapshot.PhotoMetadataID = nullableInt64(current.PhotoMetadataID)
	if current.PhotoMIMEType.Valid {
		snapshot.PhotoMIMEType = &current.PhotoMIMEType.String
	}
	snapshot.PhotoSizeBytes = nullableInt(current.PhotoSizeBytes)
	if current.PhotoChecksum.Valid {
		snapshot.PhotoChecksum = &current.PhotoChecksum.String
	}
	if current.PhotoCreatedAt.Valid {
		snapshot.PhotoCreatedAt = &current.PhotoCreatedAt.Time
	}
	if current.RetainUntil.Valid {
		snapshot.RetainUntil = &current.RetainUntil.Time
	}
	if current.PhotoDeletedAt.Valid {
		snapshot.PhotoDeletedAt = &current.PhotoDeletedAt.Time
	}
	snapshot.CleanupAttempts = nullableInt(current.PhotoCleanupAttempts)
	if current.ManualReason.Valid {
		snapshot.ManualReason = &current.ManualReason.String
	}
	return snapshot
}

func nullableFloat64(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func teacherAttendanceCorrectionResult(current teacherAttendanceCorrectionEvent, actorID int64) TeacherAttendanceCorrectionResult {
	return TeacherAttendanceCorrectionResult{
		EventID: current.ID, TeacherID: current.TeacherID, RecordedBy: current.RecordedBy, CorrectedBy: actorID,
		TeacherName: current.TeacherName, OnDate: current.OnDate, Source: current.Source,
		Action: current.Action, OccurredAt: current.OccurredAt, RecordedAt: current.RecordedAt,
		ManualReason: current.ManualReason.String, Version: current.Version,
	}
}
