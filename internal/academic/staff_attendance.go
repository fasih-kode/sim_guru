package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrStaffAttendancePhoto = errors.New("penyimpanan foto kehadiran pegawai tidak tersedia")
	ErrStaffAttendanceWrite = errors.New("pencatatan kehadiran pegawai gagal")
)

// StaffAttendancePhotoStorage menerima file privat khusus domain pegawai.
type StaffAttendancePhotoStorage interface {
	Prepare(io.Reader, string) (teacherphoto.PreparedPhoto, error)
	SavePrepared(teacherphoto.PreparedPhoto) (teacherphoto.Metadata, error)
	Delete(string) error
	Open(string) (*os.File, error)
	ListKeys() ([]string, error)
}

type StaffAttendanceCaptureInput struct {
	IdempotencyKey      string
	Action              TeacherAttendanceAction
	Latitude, Longitude float64
	AccuracyM           float64
	Selfie              io.Reader
	SelfieMIMEType      string
}

type StaffAttendanceEvent struct {
	ID, PhotoID                  int64
	Name, Date                   string
	Action                       TeacherAttendanceAction
	OccurredAt, RecordedAt       time.Time
	DistanceM, AccuracyM         float64
	RetainUntil                  time.Time
	PhotoDeleted                 bool
	PhotoMIMEType, PhotoChecksum string
	PhotoSizeBytes               int
	Idempotent                   bool
}

type StaffAttendanceDay struct {
	StaffID int64
	Name    string
	Date    string
	Active  bool
	Events  []StaffAttendanceEvent
}

type StaffAttendanceHistory struct {
	Events   []StaffAttendanceEvent
	Page     int
	Previous int
	Next     int
}

type staffAttendanceInputFingerprint struct {
	ActorID, StaffID int64
	Action           TeacherAttendanceAction
	AccuracyM        float64
	DistanceM        float64
	SchoolLatitude   float64
	SchoolLongitude  float64
	RadiusM          int
	LocationVersion  int64
	LocationProof    string
	PhotoChecksum    string
	PhotoSize        int
	IdempotencyHash  string
}

type staffAttendanceRecordedSnapshot struct {
	StaffID, StaffUserID, ActorID   int64
	StaffName, OnDate               string
	Action                          TeacherAttendanceAction
	OccurredAt, RecordedAt          time.Time
	DistanceM, AccuracyM            float64
	SchoolLatitude, SchoolLongitude float64
	GeofenceRadiusM                 int
	LocationVersion                 int64
	PhotoMetadataID                 int64
	PhotoMIMEType                   string
	PhotoSizeBytes                  int
	PhotoChecksumSHA256             string
	IdempotencyKeyHash              string
	InputFingerprint                string
	DeviceLocationProof             string
}

func (s *Store) StaffAttendanceToday(ctx context.Context, actorID int64) (StaffAttendanceDay, error) {
	var day StaffAttendanceDay
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return day, err
	}
	defer tx.Rollback()
	id, name, _, err := linkedStaffForAttendance(ctx, tx, actorID)
	if err != nil {
		return day, err
	}
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return day, err
	}
	day.StaffID, day.Name, day.Date = id, name, TeacherAttendanceWIBDate(now)
	if err = tx.QueryRowContext(ctx, `SELECT active FROM app.staff_members WHERE id=$1`, id).Scan(&day.Active); err != nil {
		return StaffAttendanceDay{}, err
	}
	day.Events, err = readStaffAttendanceEvents(ctx, tx, id, day.Date, 0, 2)
	if err != nil {
		return StaffAttendanceDay{}, err
	}
	return day, nil
}

func (s *Store) StaffAttendanceOwnHistory(ctx context.Context, actorID int64, page int) (StaffAttendanceHistory, error) {
	result := StaffAttendanceHistory{Page: page}
	if page < 1 || page > 100000 {
		return result, ValidationError("Halaman riwayat tidak valid.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	id, _, _, err := linkedStaffForAttendance(ctx, tx, actorID)
	if err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.photo_metadata_id,e.staff_name,e.on_date::text,e.action,e.occurred_at,e.recorded_at,e.distance_m,e.accuracy_m,p.retain_until,(p.deleted_at IS NOT NULL OR p.retain_until<=clock_timestamp()),p.mime_type,p.size_bytes,p.checksum_sha256
		FROM app.staff_attendance_events e JOIN app.staff_attendance_photos p ON p.id=e.photo_metadata_id AND p.event_id=e.id
		WHERE e.staff_member_id=$1 ORDER BY e.on_date DESC,e.action,e.id DESC LIMIT 11 OFFSET $2`, id, (page-1)*10)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var event StaffAttendanceEvent
		if err = rows.Scan(&event.ID, &event.PhotoID, &event.Name, &event.Date, &event.Action, &event.OccurredAt, &event.RecordedAt, &event.DistanceM, &event.AccuracyM, &event.RetainUntil, &event.PhotoDeleted, &event.PhotoMIMEType, &event.PhotoSizeBytes, &event.PhotoChecksum); err != nil {
			return result, err
		}
		result.Events = append(result.Events, event)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(result.Events) > 10 {
		result.Next = page + 1
		result.Events = result.Events[:10]
	}
	if page > 1 {
		result.Previous = page - 1
	}
	return result, nil
}

func (s *Store) CaptureStaffAttendance(ctx context.Context, actorID int64, input StaffAttendanceCaptureInput) (StaffAttendanceEvent, error) {
	var result StaffAttendanceEvent
	if actorID < 1 {
		return result, ErrForbidden
	}
	teacherInput := TeacherAttendanceCaptureInput{IdempotencyKey: input.IdempotencyKey, Action: input.Action, Latitude: input.Latitude, Longitude: input.Longitude, AccuracyM: input.AccuracyM}
	if err := validateTeacherAttendanceCapture(teacherInput); err != nil {
		return result, err
	}
	if s.StaffPhotos == nil {
		return result, ErrStaffAttendancePhoto
	}
	prepared, err := s.StaffPhotos.Prepare(input.Selfie, input.SelfieMIMEType)
	if err != nil {
		return result, err
	}
	keyBytes, _ := hex.DecodeString(input.IdempotencyKey)
	keyHashBytes := sha256.Sum256(keyBytes)
	keyHash := hex.EncodeToString(keyHashBytes[:])
	locationProof := teacherAttendanceDeviceLocationProof(keyBytes, input.Latitude, input.Longitude)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return result, err
	}
	staffID, staffName, staffUserID, err := activeStaffForAttendanceIdentity(ctx, tx, actorID)
	if err != nil {
		return result, err
	}
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return result, err
	}
	onDate := TeacherAttendanceWIBDate(now)
	// Retry key dicek sebelum tanggal dan lokasi terkini agar respons hilang lintas tengah malam tetap aman.
	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.staff_attendance_events WHERE staff_member_id=$1 AND staff_user_id=$2 AND action=$3 AND idempotency_key_hash=$4`, staffID, actorID, string(input.Action), keyHash).Scan(&existingID)
	if err == nil {
		event, fingerprint, retryErr := loadStaffAttendanceRetry(ctx, tx, existingID, actorID, staffID, input, prepared, keyHash, locationProof)
		if retryErr != nil {
			return result, retryErr
		}
		if fingerprint == "" {
			return result, ErrConflict
		}
		event.Idempotent = true
		if err = tx.Commit(); err != nil {
			return StaffAttendanceEvent{}, ErrStaffAttendanceWrite
		}
		return event, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	var duplicate int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.staff_attendance_events WHERE staff_member_id=$1 AND on_date=$2::date AND action=$3`, staffID, onDate, string(input.Action)).Scan(&duplicate)
	if err == nil {
		return result, ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	location, err := teacherAttendanceCaptureLocation(tx.QueryRowContext(ctx, `SELECT latitude,longitude,radius_m,version,updated_by,updated_at FROM app.teacher_attendance_location WHERE id=1 FOR SHARE`))
	if err != nil {
		return result, err
	}
	distance, err := TeacherAttendanceDistanceM(location.Latitude, location.Longitude, input.Latitude, input.Longitude)
	if err != nil {
		return result, err
	}
	within, err := TeacherAttendanceWithinRadius(distance, location.RadiusM)
	if err != nil {
		return result, err
	}
	if !within {
		return result, ValidationError("Lokasi berada di luar radius kehadiran sekolah.")
	}
	fingerprint, err := staffAttendanceFingerprint(staffAttendanceInputFingerprint{ActorID: actorID, StaffID: staffID, Action: input.Action, AccuracyM: input.AccuracyM, DistanceM: distance, SchoolLatitude: location.Latitude, SchoolLongitude: location.Longitude, RadiusM: location.RadiusM, LocationVersion: location.Version, LocationProof: locationProof, PhotoChecksum: prepared.ChecksumSHA256(), PhotoSize: prepared.SizeBytes(), IdempotencyHash: keyHash})
	if err != nil {
		return result, ErrStaffAttendanceWrite
	}
	photo, err := s.StaffPhotos.SavePrepared(prepared)
	if err != nil {
		return result, err
	}
	proof, writeErr := insertStaffAttendanceCapture(ctx, tx, actorID, staffID, staffUserID, staffName, now, onDate, input, location, distance, photo, keyHash, fingerprint, locationProof)
	if writeErr != nil {
		_ = tx.Rollback()
		if cleanupErr := s.StaffPhotos.Delete(photo.StorageKey); cleanupErr != nil {
			return result, ErrTeacherAttendanceCleanup
		}
		return result, writeErr
	}
	if err = tx.Commit(); err == nil {
		return proof, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if cleanupErr := s.StaffPhotos.Delete(photo.StorageKey); cleanupErr != nil {
			return result, ErrTeacherAttendanceCleanup
		}
		if pgErr.Code == "23505" {
			return result, ErrConflict
		}
		return result, ErrStaffAttendanceWrite
	}
	// Commit tidak terkonfirmasi: jangan hapus file yang mungkin sudah menjadi bukti tersimpan.
	confirmed, confirmErr := s.staffAttendanceRetryExists(ctx, staffID, actorID, input.Action, keyHash, fingerprint)
	if confirmErr != nil {
		return result, ErrTeacherAttendanceCommit
	}
	if confirmed {
		proof.Idempotent = true
		return proof, nil
	}
	if cleanupErr := s.StaffPhotos.Delete(photo.StorageKey); cleanupErr != nil {
		return result, ErrTeacherAttendanceCleanup
	}
	return result, ErrStaffAttendanceWrite
}

func linkedStaffForAttendance(ctx context.Context, tx *sql.Tx, actorID int64) (int64, string, int64, error) {
	var id, userID int64
	var name string
	err := tx.QueryRowContext(ctx, `SELECT s.id,s.name,s.user_id FROM app.users u JOIN app.user_roles r ON r.user_id=u.id AND r.role_code IN ('pegawai_tu','kepala_sekolah') JOIN app.staff_members s ON s.user_id=u.id WHERE u.id=$1 AND u.active FOR KEY SHARE OF u,s`, actorID).Scan(&id, &name, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", 0, ErrForbidden
	}
	return id, name, userID, err
}

func activeStaffForAttendanceIdentity(ctx context.Context, tx *sql.Tx, actorID int64) (int64, string, int64, error) {
	var id, userID int64
	var name string
	err := tx.QueryRowContext(ctx, `SELECT s.id,s.name,s.user_id FROM app.users u JOIN app.user_roles r ON r.user_id=u.id AND r.role_code IN ('pegawai_tu','kepala_sekolah') JOIN app.staff_members s ON s.user_id=u.id AND s.active WHERE u.id=$1 AND u.active FOR KEY SHARE OF u,s`, actorID).Scan(&id, &name, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", 0, ErrForbidden
	}
	return id, name, userID, err
}

func loadStaffAttendanceRetry(ctx context.Context, tx *sql.Tx, eventID, actorID, staffID int64, input StaffAttendanceCaptureInput, prepared teacherphoto.PreparedPhoto, keyHash, locationProof string) (StaffAttendanceEvent, string, error) {
	var event StaffAttendanceEvent
	var storedActor, storedStaff, storedUserID int64
	var locationVersion int64
	var radius int
	var schoolLat, schoolLon float64
	var inputFingerprint, storedKeyHash, storedLocationProof string
	err := tx.QueryRowContext(ctx, `SELECT e.id,e.photo_metadata_id,e.staff_member_id,e.staff_name,e.staff_user_id,e.actor_id,e.on_date::text,e.action,e.occurred_at,e.recorded_at,e.distance_m,e.accuracy_m,e.school_latitude,e.school_longitude,e.geofence_radius_m,e.location_version,e.input_fingerprint,e.idempotency_key_hash,e.device_location_proof,p.retain_until,p.deleted_at IS NOT NULL,p.mime_type,p.size_bytes,p.checksum_sha256
	FROM app.staff_attendance_events e JOIN app.staff_attendance_photos p ON p.event_id=e.id AND p.id=e.photo_metadata_id WHERE e.id=$1`, eventID).Scan(&event.ID, &event.PhotoID, &storedStaff, &event.Name, &storedUserID, &storedActor, &event.Date, &event.Action, &event.OccurredAt, &event.RecordedAt, &event.DistanceM, &event.AccuracyM, &schoolLat, &schoolLon, &radius, &locationVersion, &inputFingerprint, &storedKeyHash, &storedLocationProof, &event.RetainUntil, &event.PhotoDeleted, &event.PhotoMIMEType, &event.PhotoSizeBytes, &event.PhotoChecksum)
	if err != nil {
		return event, "", err
	}
	if storedActor != actorID || storedStaff != staffID || storedUserID != actorID || storedKeyHash != keyHash || storedLocationProof != locationProof || event.Action != input.Action || event.PhotoChecksum != prepared.ChecksumSHA256() || event.PhotoSizeBytes != prepared.SizeBytes() || event.PhotoMIMEType != prepared.MIMEType() {
		return event, "", nil
	}
	want, err := staffAttendanceFingerprint(staffAttendanceInputFingerprint{ActorID: actorID, StaffID: staffID, Action: input.Action, AccuracyM: input.AccuracyM, DistanceM: event.DistanceM, SchoolLatitude: schoolLat, SchoolLongitude: schoolLon, RadiusM: radius, LocationVersion: locationVersion, LocationProof: locationProof, PhotoChecksum: prepared.ChecksumSHA256(), PhotoSize: prepared.SizeBytes(), IdempotencyHash: keyHash})
	if err != nil {
		return event, "", err
	}
	if want != inputFingerprint {
		return event, "", nil
	}
	return event, want, nil
}

func staffAttendanceFingerprint(input staffAttendanceInputFingerprint) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func insertStaffAttendanceCapture(ctx context.Context, tx *sql.Tx, actorID, staffID, staffUserID int64, staffName string, now time.Time, onDate string, input StaffAttendanceCaptureInput, location TeacherAttendanceLocationConfig, distance float64, photo teacherphoto.Metadata, keyHash, fingerprint, locationProof string) (StaffAttendanceEvent, error) {
	var proof StaffAttendanceEvent
	var photoID, eventID int64
	if err := tx.QueryRowContext(ctx, `SELECT nextval(pg_get_serial_sequence('app.staff_attendance_photos','id'))`).Scan(&photoID); err != nil {
		return proof, err
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO app.staff_attendance_events(staff_member_id,staff_name,staff_user_id,actor_id,on_date,action,occurred_at,recorded_at,distance_m,accuracy_m,school_latitude,school_longitude,geofence_radius_m,location_version,photo_metadata_id,idempotency_key_hash,input_fingerprint,device_location_proof)
	VALUES($1,$2,$3,$3,$4::date,$5,$6,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`, staffID, staffName, staffUserID, onDate, string(input.Action), now, distance, input.AccuracyM, location.Latitude, location.Longitude, location.RadiusM, location.Version, photoID, keyHash, fingerprint, locationProof).Scan(&eventID)
	if err != nil {
		return proof, err
	}
	retainUntil := TeacherAttendancePhotoRetainUntil(now)
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.staff_attendance_photos(id,event_id,storage_key,mime_type,size_bytes,checksum_sha256,retain_until) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,$7)`, photoID, eventID, photo.StorageKey, photo.MIMEType, photo.SizeBytes, photo.ChecksumSHA256, retainUntil); err != nil {
		return proof, err
	}
	var recordedAt time.Time
	if err = tx.QueryRowContext(ctx, `SELECT recorded_at FROM app.staff_attendance_events WHERE id=$1`, eventID).Scan(&recordedAt); err != nil {
		return proof, err
	}
	snapshot := staffAttendanceRecordedSnapshot{StaffID: staffID, StaffUserID: staffUserID, ActorID: actorID, StaffName: staffName, OnDate: onDate, Action: input.Action, OccurredAt: recordedAt, RecordedAt: recordedAt, DistanceM: distance, AccuracyM: input.AccuracyM, SchoolLatitude: location.Latitude, SchoolLongitude: location.Longitude, GeofenceRadiusM: location.RadiusM, LocationVersion: location.Version, PhotoMetadataID: photoID, PhotoMIMEType: photo.MIMEType, PhotoSizeBytes: photo.SizeBytes, PhotoChecksumSHA256: photo.ChecksumSHA256, IdempotencyKeyHash: keyHash, InputFingerprint: fingerprint, DeviceLocationProof: locationProof}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return proof, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.staff_attendance_history(event_id,action,actor_id,after_state,reason) VALUES($1,'recorded',$2,$3,'Pencatatan mandiri')`, eventID, actorID, string(encoded)); err != nil {
		return proof, err
	}
	proof = StaffAttendanceEvent{ID: eventID, PhotoID: photoID, Name: staffName, Date: onDate, Action: input.Action, OccurredAt: recordedAt, RecordedAt: recordedAt, DistanceM: distance, AccuracyM: input.AccuracyM, RetainUntil: retainUntil, PhotoMIMEType: photo.MIMEType, PhotoChecksum: photo.ChecksumSHA256, PhotoSizeBytes: photo.SizeBytes}
	return proof, nil
}

func readStaffAttendanceEvents(ctx context.Context, tx *sql.Tx, staffID int64, onDate string, offset, limit int) ([]StaffAttendanceEvent, error) {
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.photo_metadata_id,e.staff_name,e.on_date::text,e.action,e.occurred_at,e.recorded_at,e.distance_m,e.accuracy_m,p.retain_until,(p.deleted_at IS NOT NULL OR p.retain_until<=clock_timestamp()),p.mime_type,p.size_bytes,p.checksum_sha256 FROM app.staff_attendance_events e JOIN app.staff_attendance_photos p ON p.event_id=e.id AND p.id=e.photo_metadata_id WHERE e.staff_member_id=$1 AND e.on_date=$2::date ORDER BY e.action,e.id LIMIT $3 OFFSET $4`, staffID, onDate, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []StaffAttendanceEvent{}
	for rows.Next() {
		var e StaffAttendanceEvent
		if err = rows.Scan(&e.ID, &e.PhotoID, &e.Name, &e.Date, &e.Action, &e.OccurredAt, &e.RecordedAt, &e.DistanceM, &e.AccuracyM, &e.RetainUntil, &e.PhotoDeleted, &e.PhotoMIMEType, &e.PhotoSizeBytes, &e.PhotoChecksum); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) staffAttendanceRetryExists(ctx context.Context, staffID, actorID int64, action TeacherAttendanceAction, keyHash, fingerprint string) (bool, error) {
	var found bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.staff_attendance_events WHERE staff_member_id=$1 AND staff_user_id=$2 AND action=$3 AND idempotency_key_hash=$4 AND input_fingerprint=$5)`, staffID, actorID, string(action), keyHash, fingerprint).Scan(&found)
	return found, err
}

func (s *Store) StaffAttendancePhoto(ctx context.Context, actorID, photoID int64) (io.ReadCloser, string, error) {
	if s.StaffPhotos == nil {
		return nil, "", ErrStaffAttendancePhoto
	}
	if actorID < 1 || photoID < 1 {
		return nil, "", ErrForbidden
	}
	var key, mimeType string
	err := s.DB.QueryRowContext(ctx, `SELECT p.storage_key,p.mime_type FROM app.staff_attendance_photos p JOIN app.staff_attendance_events e ON e.id=p.event_id JOIN app.staff_members s ON s.id=e.staff_member_id JOIN app.users u ON u.id=s.user_id AND u.active JOIN app.user_roles r ON r.user_id=u.id AND r.role_code IN ('pegawai_tu','kepala_sekolah') WHERE p.id=$1 AND p.deleted_at IS NULL AND p.retain_until>clock_timestamp() AND u.id=$2`, photoID, actorID).Scan(&key, &mimeType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	file, err := s.StaffPhotos.Open(key)
	if err != nil {
		return nil, "", err
	}
	return file, mimeType, nil
}

func (s *Store) PurgeExpiredStaffAttendancePhotos(ctx context.Context, limit int) (TeacherAttendancePhotoPurgeResult, error) {
	var result TeacherAttendancePhotoPurgeResult
	if s.StaffPhotos == nil {
		return result, ErrStaffAttendancePhoto
	}
	if limit < 1 || limit > MaxTeacherAttendancePhotoPurgeBatch {
		return result, ValidationError("Jumlah foto yang dibersihkan tidak valid.")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM app.staff_attendance_photos WHERE deleted_at IS NULL AND retain_until<=clock_timestamp() ORDER BY cleanup_attempts,retain_until,id LIMIT $1`, limit)
	if err != nil {
		return result, ErrTeacherAttendanceWrite
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, ErrTeacherAttendanceWrite
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, ErrTeacherAttendanceWrite
	}
	if err = rows.Close(); err != nil {
		return result, ErrTeacherAttendanceWrite
	}
	var failures []error
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return result, errors.Join(append(failures, err)...)
		}
		result.Examined++
		purged, purgeErr := s.purgeStaffAttendancePhoto(ctx, id)
		if purgeErr != nil {
			result.Failed++
			failures = append(failures, purgeErr)
		} else if purged {
			result.Purged++
		}
	}
	return result, errors.Join(failures...)
}

func (s *Store) purgeStaffAttendancePhoto(ctx context.Context, id int64) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, ErrTeacherAttendanceWrite
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return false, ErrTeacherAttendanceWrite
	}
	var key string
	var retainUntil, now time.Time
	var deletedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT storage_key,retain_until,deleted_at,clock_timestamp() FROM app.staff_attendance_photos WHERE id=$1 FOR UPDATE`, id).Scan(&key, &retainUntil, &deletedAt, &now)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, ErrTeacherAttendanceWrite
	}
	if deletedAt.Valid || retainUntil.After(now) {
		return false, nil
	}
	if err = s.StaffPhotos.Delete(key); err != nil {
		if _, err = tx.ExecContext(ctx, `UPDATE app.staff_attendance_photos SET cleanup_attempts=cleanup_attempts+1,last_cleanup_error='photo_delete_failed' WHERE id=$1`, id); err != nil {
			return false, ErrTeacherAttendanceWrite
		}
		if err = tx.Commit(); err != nil {
			return false, ErrTeacherAttendanceCommit
		}
		return false, ErrTeacherAttendanceCleanup
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.staff_attendance_photos SET deleted_at=clock_timestamp(),last_cleanup_error=NULL WHERE id=$1`, id); err != nil {
		return false, ErrTeacherAttendanceWrite
	}
	if err = tx.Commit(); err != nil {
		return false, ErrTeacherAttendanceCommit
	}
	return true, nil
}
