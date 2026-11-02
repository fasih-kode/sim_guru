package academic

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrTeacherAttendanceAction  = errors.New("aksi kehadiran guru tidak valid")
	ErrTeacherAttendancePhoto   = errors.New("penyimpanan foto kehadiran guru tidak tersedia")
	ErrTeacherAttendanceCommit  = errors.New("hasil penyimpanan kehadiran guru belum dapat dipastikan")
	ErrTeacherAttendanceCleanup = errors.New("file bukti kehadiran memerlukan rekonsiliasi")
	ErrTeacherAttendanceWrite   = errors.New("pencatatan kehadiran guru gagal")
)

type TeacherAttendanceAction string

const (
	TeacherCheckIn  TeacherAttendanceAction = "check_in"
	TeacherCheckOut TeacherAttendanceAction = "check_out"
)

// TeacherAttendancePhotoStorage membatasi kebutuhan service pada storage privat.
type TeacherAttendancePhotoStorage interface {
	Prepare(io.Reader, string) (teacherphoto.PreparedPhoto, error)
	SavePrepared(teacherphoto.PreparedPhoto) (teacherphoto.Metadata, error)
	Delete(string) error
	ListKeys() ([]string, error)
}

type TeacherAttendanceCaptureInput struct {
	// IdempotencyKey adalah token acak 128-bit yang sama untuk retry satu pengiriman.
	// Hanya hash token yang disimpan pada histori.
	IdempotencyKey      string
	Action              TeacherAttendanceAction
	Latitude, Longitude float64
	AccuracyM           float64
	Selfie              io.Reader
	SelfieMIMEType      string
}

// NewTeacherAttendanceIdempotencyKey membuat token acak untuk satu pengiriman.
// Pemanggil harus mempertahankan token yang sama ketika mengulang request.
func NewTeacherAttendanceIdempotencyKey() (string, error) {
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(key[:]), nil
}

type TeacherAttendanceCaptureProof struct {
	EventID, TeacherID, PhotoMetadataID int64
	TeacherName, OnDate                 string
	Action                              TeacherAttendanceAction
	OccurredAt, RecordedAt, RetainUntil time.Time
	DistanceM, AccuracyM                float64
	SchoolLatitude, SchoolLongitude     float64
	GeofenceRadiusM                     int
	PhotoMIMEType, PhotoChecksumSHA256  string
	PhotoSizeBytes                      int
	Idempotent                          bool
}

type teacherAttendanceEvidence struct {
	ActorID             int64                   `json:"actor_id"`
	TeacherID           int64                   `json:"teacher_id"`
	OnDate              string                  `json:"on_date"`
	Action              TeacherAttendanceAction `json:"action"`
	Source              string                  `json:"source"`
	DistanceM           float64                 `json:"distance_m"`
	AccuracyM           float64                 `json:"accuracy_m"`
	SchoolLatitude      float64                 `json:"school_latitude"`
	SchoolLongitude     float64                 `json:"school_longitude"`
	GeofenceRadiusM     int                     `json:"geofence_radius_m"`
	LocationVersion     int64                   `json:"location_version"`
	PhotoMIMEType       string                  `json:"photo_mime_type"`
	PhotoSizeBytes      int                     `json:"photo_size_bytes"`
	PhotoChecksumSHA256 string                  `json:"photo_checksum_sha256"`
}

type teacherAttendanceRecordedSnapshot struct {
	EventID, TeacherID, ActorID int64
	TeacherName, OnDate         string
	Action, Source              string
	OccurredAt, RecordedAt      time.Time
	Version                     int64
	DistanceM, AccuracyM        float64
	SchoolLatitude              float64
	SchoolLongitude             float64
	GeofenceRadiusM             int
	LocationVersion             int64
	PhotoMetadataID             int64
	PhotoMIMEType               string
	PhotoSizeBytes              int
	PhotoChecksumSHA256         string
	EvidenceFingerprint         string
	IdempotencyKeyHash          string
	DeviceLocationProof         string
}

// CaptureTeacherAttendance mencatat satu aksi mandiri dengan waktu dan tanggal DB.
// Koordinat perangkat hanya berada di memori untuk validasi dan tidak disimpan.
func (s *Store) CaptureTeacherAttendance(ctx context.Context, actorID int64, input TeacherAttendanceCaptureInput) (TeacherAttendanceCaptureProof, error) {
	if actorID < 1 {
		return TeacherAttendanceCaptureProof{}, ErrForbidden
	}
	if err := validateTeacherAttendanceCapture(input); err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	if s.Photos == nil {
		return TeacherAttendanceCaptureProof{}, ErrTeacherAttendancePhoto
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	teacherID, teacherName, err := activeTeacherForAttendance(ctx, tx, actorID)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	onDate := TeacherAttendanceWIBDate(now)
	keyBytes, err := hex.DecodeString(input.IdempotencyKey)
	if err != nil || len(keyBytes) != 16 {
		return TeacherAttendanceCaptureProof{}, ValidationError("Kunci pengiriman absensi tidak valid.")
	}
	keyHashBytes := sha256.Sum256(keyBytes)
	keyHash := hex.EncodeToString(keyHashBytes[:])
	locationProof := teacherAttendanceDeviceLocationProof(keyBytes, input.Latitude, input.Longitude)

	// Token memungkinkan retry aman walau respons hilang saat pergantian tanggal WIB.
	var retryEventID int64
	err = tx.QueryRowContext(ctx, `
		SELECT e.id FROM app.teacher_attendance_events e
		JOIN app.teacher_attendance_history h ON h.event_id=e.id AND h.action='recorded'
		WHERE e.teacher_id=$1 AND e.actor_id=$2 AND e.action=$3
		  AND h.after_state->>'IdempotencyKeyHash'=$4
		ORDER BY h.id LIMIT 1`, teacherID, actorID, string(input.Action), keyHash).Scan(&retryEventID)
	if err == nil {
		proof, fingerprint, retryErr := loadTeacherAttendanceRetry(ctx, tx, retryEventID, actorID, input, "", keyHash, nil, s.Photos)
		if retryErr != nil {
			return TeacherAttendanceCaptureProof{}, retryErr
		}
		if fingerprint == "" {
			return TeacherAttendanceCaptureProof{}, ErrConflict
		}
		proof.Idempotent = true
		return proof, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TeacherAttendanceCaptureProof{}, err
	}

	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teacher_attendance_events WHERE teacher_id=$1 AND on_date=$2::date AND action=$3`, teacherID, onDate, string(input.Action)).Scan(&existingID)
	if err == nil {
		proof, fingerprint, retryErr := loadTeacherAttendanceRetry(ctx, tx, existingID, actorID, input, onDate, keyHash, nil, s.Photos)
		if retryErr != nil {
			return TeacherAttendanceCaptureProof{}, retryErr
		}
		if fingerprint == "" {
			return TeacherAttendanceCaptureProof{}, ErrConflict
		}
		proof.Idempotent = true
		return proof, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TeacherAttendanceCaptureProof{}, err
	}

	location, err := teacherAttendanceCaptureLocation(tx.QueryRowContext(ctx, `SELECT latitude,longitude,radius_m,version,updated_by,updated_at FROM app.teacher_attendance_location WHERE id=1 FOR SHARE`))
	if err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	distance, err := TeacherAttendanceDistanceM(location.Latitude, location.Longitude, input.Latitude, input.Longitude)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	within, err := TeacherAttendanceWithinRadius(distance, location.RadiusM)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	if !within {
		return TeacherAttendanceCaptureProof{}, ValidationError("Lokasi berada di luar radius kehadiran sekolah.")
	}
	prepared, err := s.Photos.Prepare(input.Selfie, input.SelfieMIMEType)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	evidence := teacherAttendanceEvidence{
		ActorID: actorID, TeacherID: teacherID, OnDate: onDate, Action: input.Action, Source: "gps_selfie",
		DistanceM: distance, AccuracyM: input.AccuracyM, SchoolLatitude: location.Latitude,
		SchoolLongitude: location.Longitude, GeofenceRadiusM: location.RadiusM, LocationVersion: location.Version,
		PhotoMIMEType: prepared.MIMEType(), PhotoSizeBytes: prepared.SizeBytes(), PhotoChecksumSHA256: prepared.ChecksumSHA256(),
	}
	fingerprint, err := teacherAttendanceEvidenceFingerprint(evidence)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, ErrTeacherAttendanceWrite
	}
	photo, err := s.Photos.SavePrepared(prepared)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	proof, writeErr := s.insertTeacherAttendanceCapture(ctx, tx, actorID, teacherID, teacherName, now, onDate, input, location, distance, photo, fingerprint, keyHash, locationProof)
	if writeErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			resolvedProof, committed, resolveErr := s.resolveTeacherAttendanceCommit(ctx, teacherID, actorID, input, onDate, keyHash, prepared)
			if resolveErr != nil && !errors.Is(resolveErr, ErrConflict) {
				return TeacherAttendanceCaptureProof{}, ErrTeacherAttendanceCommit
			}
			if committed {
				resolvedProof.Idempotent = true
				return resolvedProof, nil
			}
		}
		if cleanupErr := s.Photos.Delete(photo.StorageKey); cleanupErr != nil {
			return TeacherAttendanceCaptureProof{}, ErrTeacherAttendanceCleanup
		}
		return TeacherAttendanceCaptureProof{}, writeErr
	}
	if err = tx.Commit(); err == nil {
		return proof, nil
	}
	var serverError *pgconn.PgError
	if errors.As(err, &serverError) {
		if cleanupErr := s.Photos.Delete(photo.StorageKey); cleanupErr != nil {
			return TeacherAttendanceCaptureProof{}, ErrTeacherAttendanceCleanup
		}
		if serverError.Code == "23505" {
			return TeacherAttendanceCaptureProof{}, ErrConflict
		}
		return TeacherAttendanceCaptureProof{}, ErrTeacherAttendanceWrite
	}

	resolvedProof, committed, resolveErr := s.resolveTeacherAttendanceCommit(ctx, teacherID, actorID, input, onDate, keyHash, prepared)
	if resolveErr != nil {
		// Jangan menghapus file selama commit mungkin telah tersimpan; rekonsiliasi orphan dilakukan kemudian.
		return TeacherAttendanceCaptureProof{}, ErrTeacherAttendanceCommit
	}
	if committed {
		resolvedProof.Idempotent = true
		return resolvedProof, nil
	}
	if cleanupErr := s.Photos.Delete(photo.StorageKey); cleanupErr != nil {
		return TeacherAttendanceCaptureProof{}, ErrTeacherAttendanceCleanup
	}
	return TeacherAttendanceCaptureProof{}, ErrTeacherAttendanceWrite
}

func validateTeacherAttendanceCapture(input TeacherAttendanceCaptureInput) error {
	if input.Action != TeacherCheckIn && input.Action != TeacherCheckOut {
		return ErrTeacherAttendanceAction
	}
	if math.IsNaN(input.Latitude) || math.IsInf(input.Latitude, 0) || input.Latitude < -90 || input.Latitude > 90 ||
		math.IsNaN(input.Longitude) || math.IsInf(input.Longitude, 0) || input.Longitude < -180 || input.Longitude > 180 {
		return ValidationError("Koordinat perangkat tidak valid.")
	}
	key, err := hex.DecodeString(input.IdempotencyKey)
	if err != nil || len(key) != 16 {
		return ValidationError("Kunci pengiriman absensi tidak valid.")
	}
	return ValidateTeacherLocationAccuracy(input.AccuracyM)
}

func teacherAttendanceCaptureLocation(row rowScanner) (TeacherAttendanceLocationConfig, error) {
	location, err := scanTeacherAttendanceLocation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TeacherAttendanceLocationConfig{}, ValidationError("Lokasi sekolah belum diatur oleh Admin.")
	}
	return location, err
}

func activeTeacherForAttendance(ctx context.Context, tx *sql.Tx, actorID int64) (int64, string, error) {
	var teacherID int64
	var teacherName string
	err := tx.QueryRowContext(ctx, `
		SELECT t.id,t.name
		FROM app.users u
		JOIN app.teachers t ON t.user_id=u.id AND t.active
		WHERE u.id=$1 AND u.active
		  AND EXISTS (SELECT 1 FROM app.user_roles r WHERE r.user_id=u.id AND r.role_code IN ('guru_mapel','wali_kelas','kepala_sekolah'))
		FOR KEY SHARE OF u,t`, actorID).Scan(&teacherID, &teacherName)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrForbidden
	}
	return teacherID, teacherName, err
}

func teacherAttendanceEvidenceFingerprint(evidence teacherAttendanceEvidence) (string, error) {
	data, err := json.Marshal(evidence)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func teacherAttendanceDeviceLocationProof(key []byte, latitude, longitude float64) string {
	var coordinates [16]byte
	binary.BigEndian.PutUint64(coordinates[:8], math.Float64bits(latitude))
	binary.BigEndian.PutUint64(coordinates[8:], math.Float64bits(longitude))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(coordinates[:])
	return hex.EncodeToString(mac.Sum(nil))
}

func loadTeacherAttendanceRetry(ctx context.Context, tx *sql.Tx, eventID, actorID int64, input TeacherAttendanceCaptureInput, expectedDate, keyHash string, prepared *teacherphoto.PreparedPhoto, photos TeacherAttendancePhotoStorage) (TeacherAttendanceCaptureProof, string, error) {
	var proof TeacherAttendanceCaptureProof
	var eventActor int64
	var source string
	var currentVersion int64
	var photoChecksum, photoMIME string
	var photoSize int
	var historyJSON sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT e.id,e.teacher_id,e.teacher_name,e.on_date::text,e.action,e.source,e.actor_id,
		       e.occurred_at,e.recorded_at,e.version,e.distance_m,e.accuracy_m,e.school_latitude,e.school_longitude,
		       e.geofence_radius_m,e.photo_metadata_id,p.mime_type,p.size_bytes,p.checksum_sha256,p.retain_until,
		       h.after_state
		FROM app.teacher_attendance_events e
		JOIN app.teacher_attendance_photos p ON p.id=e.photo_metadata_id AND p.event_id=e.id
		LEFT JOIN LATERAL (
		  SELECT after_state FROM app.teacher_attendance_history WHERE event_id=e.id AND action='recorded' ORDER BY id LIMIT 1
		) h ON true
		WHERE e.id=$1`, eventID).Scan(
		&proof.EventID, &proof.TeacherID, &proof.TeacherName, &proof.OnDate, &proof.Action, &source, &eventActor,
		&proof.OccurredAt, &proof.RecordedAt, &currentVersion, &proof.DistanceM, &proof.AccuracyM, &proof.SchoolLatitude,
		&proof.SchoolLongitude, &proof.GeofenceRadiusM, &proof.PhotoMetadataID, &photoMIME, &photoSize,
		&photoChecksum, &proof.RetainUntil, &historyJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return proof, "", ErrConflict
	}
	if err != nil {
		return proof, "", err
	}
	if source != "gps_selfie" || eventActor != actorID || proof.Action != input.Action || (expectedDate != "" && proof.OnDate != expectedDate) || !historyJSON.Valid || historyJSON.String == "" {
		return proof, "", nil
	}
	var snapshot struct {
		OccurredAt          time.Time `json:"OccurredAt"`
		Version             int64     `json:"Version"`
		LocationVersion     int64     `json:"LocationVersion"`
		EvidenceFingerprint string    `json:"EvidenceFingerprint"`
		IdempotencyKeyHash  string    `json:"IdempotencyKeyHash"`
		DeviceLocationProof string    `json:"DeviceLocationProof"`
	}
	if err = json.Unmarshal([]byte(historyJSON.String), &snapshot); err != nil || snapshot.EvidenceFingerprint == "" {
		return proof, "", ErrConflict
	}
	// Snapshot bukti awal tidak boleh membuat retry mengabaikan koreksi yang sah.
	if currentVersion != snapshot.Version || !proof.OccurredAt.Equal(snapshot.OccurredAt) {
		return proof, "", nil
	}
	if snapshot.IdempotencyKeyHash != keyHash {
		return proof, "", nil
	}
	keyBytes, decodeErr := hex.DecodeString(input.IdempotencyKey)
	if decodeErr != nil || teacherAttendanceDeviceLocationProof(keyBytes, input.Latitude, input.Longitude) != snapshot.DeviceLocationProof {
		return proof, "", nil
	}
	proof.PhotoMIMEType, proof.PhotoSizeBytes, proof.PhotoChecksumSHA256 = photoMIME, photoSize, photoChecksum
	var image teacherphoto.PreparedPhoto
	if prepared == nil {
		image, err = photos.Prepare(input.Selfie, input.SelfieMIMEType)
		if err != nil {
			return proof, "", err
		}
	} else {
		image = *prepared
	}
	distance, err := TeacherAttendanceDistanceM(proof.SchoolLatitude, proof.SchoolLongitude, input.Latitude, input.Longitude)
	if err != nil {
		return proof, "", err
	}
	evidence := teacherAttendanceEvidence{
		ActorID: actorID, TeacherID: proof.TeacherID, OnDate: proof.OnDate, Action: input.Action, Source: "gps_selfie",
		DistanceM: distance, AccuracyM: input.AccuracyM, SchoolLatitude: proof.SchoolLatitude,
		SchoolLongitude: proof.SchoolLongitude, GeofenceRadiusM: proof.GeofenceRadiusM,
		LocationVersion: snapshot.LocationVersion, PhotoMIMEType: image.MIMEType(),
		PhotoSizeBytes: image.SizeBytes(), PhotoChecksumSHA256: image.ChecksumSHA256(),
	}
	currentFingerprint, err := teacherAttendanceEvidenceFingerprint(evidence)
	if err != nil {
		return proof, "", ErrTeacherAttendanceWrite
	}
	if photoChecksum != image.ChecksumSHA256() || photoSize != image.SizeBytes() || photoMIME != image.MIMEType() || currentFingerprint != snapshot.EvidenceFingerprint {
		return proof, "", nil
	}
	return proof, snapshot.EvidenceFingerprint, nil
}

func (s *Store) insertTeacherAttendanceCapture(ctx context.Context, tx *sql.Tx, actorID, teacherID int64, teacherName string, now time.Time, onDate string, input TeacherAttendanceCaptureInput, location TeacherAttendanceLocationConfig, distance float64, photo teacherphoto.Metadata, fingerprint, keyHash, locationProof string) (TeacherAttendanceCaptureProof, error) {
	var metadataID, eventID int64
	if err := tx.QueryRowContext(ctx, `SELECT nextval(pg_get_serial_sequence('app.teacher_attendance_photos','id'))`).Scan(&metadataID); err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	err := tx.QueryRowContext(ctx, `
		INSERT INTO app.teacher_attendance_events
		 (teacher_id,teacher_name,on_date,action,source,actor_id,occurred_at,recorded_at,distance_m,accuracy_m,school_latitude,school_longitude,geofence_radius_m,photo_metadata_id)
		VALUES($1,$2,$3::date,$4,'gps_selfie',$5,$6,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id`,
		teacherID, teacherName, onDate, string(input.Action), actorID, now, distance, input.AccuracyM,
		location.Latitude, location.Longitude, location.RadiusM, metadataID).Scan(&eventID)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, dbError(err)
	}
	retainUntil := TeacherAttendancePhotoRetainUntil(now)
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO app.teacher_attendance_photos
		 (id,event_id,storage_key,mime_type,size_bytes,checksum_sha256,retain_until)
		OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,$7)`,
		metadataID, eventID, photo.StorageKey, photo.MIMEType, photo.SizeBytes, photo.ChecksumSHA256, retainUntil); err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	snapshot := teacherAttendanceRecordedSnapshot{
		EventID: eventID, TeacherID: teacherID, ActorID: actorID, TeacherName: teacherName, OnDate: onDate,
		Action: string(input.Action), Source: "gps_selfie", OccurredAt: now, RecordedAt: now, Version: 1,
		DistanceM: distance, AccuracyM: input.AccuracyM, SchoolLatitude: location.Latitude,
		SchoolLongitude: location.Longitude, GeofenceRadiusM: location.RadiusM, LocationVersion: location.Version,
		PhotoMetadataID: metadataID, PhotoMIMEType: photo.MIMEType, PhotoSizeBytes: photo.SizeBytes,
		PhotoChecksumSHA256: photo.ChecksumSHA256, EvidenceFingerprint: fingerprint, IdempotencyKeyHash: keyHash,
		DeviceLocationProof: locationProof,
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.teacher_attendance_history(event_id,action,actor_id,after_state) VALUES($1,'recorded',$2,$3)`, eventID, actorID, string(encoded)); err != nil {
		return TeacherAttendanceCaptureProof{}, err
	}
	return TeacherAttendanceCaptureProof{
		EventID: eventID, TeacherID: teacherID, PhotoMetadataID: metadataID, TeacherName: teacherName,
		OnDate: onDate, Action: input.Action, OccurredAt: now, RecordedAt: now, RetainUntil: retainUntil,
		DistanceM: distance, AccuracyM: input.AccuracyM, SchoolLatitude: location.Latitude,
		SchoolLongitude: location.Longitude, GeofenceRadiusM: location.RadiusM, PhotoMIMEType: photo.MIMEType,
		PhotoSizeBytes: photo.SizeBytes, PhotoChecksumSHA256: photo.ChecksumSHA256,
	}, nil
}

func (s *Store) resolveTeacherAttendanceCommit(ctx context.Context, teacherID, actorID int64, input TeacherAttendanceCaptureInput, onDate, keyHash string, prepared teacherphoto.PreparedPhoto) (TeacherAttendanceCaptureProof, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return TeacherAttendanceCaptureProof{}, false, err
	}
	var eventID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teacher_attendance_events WHERE teacher_id=$1 AND on_date=$2::date AND action=$3`, teacherID, onDate, string(input.Action)).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return TeacherAttendanceCaptureProof{}, false, tx.Commit()
	}
	if err != nil {
		return TeacherAttendanceCaptureProof{}, false, err
	}
	proof, fingerprint, err := loadTeacherAttendanceRetry(ctx, tx, eventID, actorID, input, onDate, keyHash, &prepared, s.Photos)
	if err != nil {
		return TeacherAttendanceCaptureProof{}, false, err
	}
	if fingerprint == "" {
		return TeacherAttendanceCaptureProof{}, false, ErrConflict
	}
	if err = tx.Commit(); err != nil {
		return TeacherAttendanceCaptureProof{}, false, err
	}
	return proof, true, nil
}

// ReconcileTeacherAttendancePhotoFiles menghapus file final yang tidak memiliki metadata aktif.
// Fungsi ini menunggu mutasi kehadiran agar file transaksi yang masih berjalan tidak dianggap yatim.
func (s *Store) ReconcileTeacherAttendancePhotoFiles(ctx context.Context) (int, error) {
	if s.Photos == nil {
		return 0, ErrTeacherAttendancePhoto
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT storage_key FROM app.teacher_attendance_photos WHERE deleted_at IS NULL`)
	if err != nil {
		return 0, err
	}
	protected := make(map[string]struct{})
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return 0, err
		}
		protected[key] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	keys, err := s.Photos.ListKeys()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, key := range keys {
		if _, ok := protected[key]; ok {
			continue
		}
		if err = s.Photos.Delete(key); err != nil {
			return removed, ErrTeacherAttendanceCleanup
		}
		removed++
	}
	if err = tx.Commit(); err != nil {
		return removed, ErrTeacherAttendanceWrite
	}
	return removed, nil
}
