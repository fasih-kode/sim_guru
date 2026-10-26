package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultTeacherAttendanceRadius = 100
	MaxTeacherLocationAccuracyM    = 50.0
)

type TeacherAttendanceLocationConfig struct {
	Latitude, Longitude float64
	RadiusM             int
	Version, UpdatedBy  int64
	UpdatedAt           time.Time
	Configured          bool
}

type TeacherAttendanceLocationInput struct {
	Latitude, Longitude float64
	RadiusM             int
	ExpectedVersion     int64
	Reason              string
}

func validateTeacherLocation(latitude, longitude float64, radius int) error {
	if math.IsNaN(latitude) || math.IsInf(latitude, 0) || latitude < -90 || latitude > 90 ||
		math.IsNaN(longitude) || math.IsInf(longitude, 0) || longitude < -180 || longitude > 180 {
		return ValidationError("Koordinat sekolah tidak valid.")
	}
	if radius < 25 || radius > 500 {
		return ValidationError("Radius harus antara 25 dan 500 meter.")
	}
	return nil
}

func validateLocationReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if !utf8.ValidString(reason) || len([]rune(trimmed)) < 1 || len([]rune(trimmed)) > 500 || strings.ContainsRune(reason, 0) {
		return ValidationError("Alasan wajib diisi, maksimal 500 karakter.")
	}
	return nil
}

func ValidateTeacherLocationAccuracy(accuracy float64) error {
	if math.IsNaN(accuracy) || math.IsInf(accuracy, 0) || accuracy < 0 || accuracy > MaxTeacherLocationAccuracyM {
		return ValidationError("Akurasi lokasi harus 50 meter atau lebih baik.")
	}
	return nil
}

func TeacherAttendanceWithinRadius(distanceM float64, radiusM int) (bool, error) {
	if radiusM < 25 || radiusM > 500 || math.IsNaN(distanceM) || math.IsInf(distanceM, 0) || distanceM < 0 {
		return false, ValidationError("Jarak atau radius lokasi tidak valid.")
	}
	return distanceM <= float64(radiusM), nil
}

// TeacherAttendanceDistanceM menghitung jarak dua koordinat dalam meter di server.
func TeacherAttendanceDistanceM(lat1, lon1, lat2, lon2 float64) (float64, error) {
	if err := validateTeacherLocation(lat1, lon1, 25); err != nil {
		return 0, err
	}
	if err := validateTeacherLocation(lat2, lon2, 25); err != nil {
		return 0, err
	}
	const earthRadiusM = 6371008.8
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(math.Min(1, a))), nil
}

func TeacherAttendanceWIBDate(at time.Time) string {
	return at.In(time.FixedZone("Asia/Jakarta", 7*60*60)).Format("2006-01-02")
}

// TeacherAttendanceMakeupAllowed menerima hari ini dan tujuh tanggal sebelumnya.
func TeacherAttendanceMakeupAllowed(eventDate, today time.Time) bool {
	eventDay, nowDay := civilDay(eventDate), civilDay(today)
	days := int(nowDay.Sub(eventDay).Hours() / 24)
	return days >= 0 && days <= 7
}

func civilDay(t time.Time) time.Time {
	loc := time.FixedZone("Asia/Jakarta", 7*60*60)
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

// TeacherAttendancePhotoRetainUntil menambah dua bulan kalender, bukan 60 hari.
func TeacherAttendancePhotoRetainUntil(recordedAt time.Time) time.Time {
	month := time.Date(recordedAt.Year(), recordedAt.Month()+2, 1, 0, 0, 0, 0, recordedAt.Location())
	lastDay := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, recordedAt.Location()).Day()
	day := recordedAt.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(month.Year(), month.Month(), day, recordedAt.Hour(), recordedAt.Minute(), recordedAt.Second(), recordedAt.Nanosecond(), recordedAt.Location())
}

func (s *Store) TeacherAttendanceLocation(ctx context.Context, actor int64) (TeacherAttendanceLocationConfig, []History, error) {
	tx, err := s.reader(ctx, actor, "admin")
	if err != nil {
		return TeacherAttendanceLocationConfig{}, nil, err
	}
	defer tx.Rollback()
	config, err := scanTeacherAttendanceLocation(tx.QueryRowContext(ctx, `SELECT latitude,longitude,radius_m,version,updated_by,updated_at FROM app.teacher_attendance_location WHERE id=1`))
	if err != nil && err != sql.ErrNoRows {
		return config, nil, err
	}
	if err == sql.ErrNoRows {
		config.RadiusM = DefaultTeacherAttendanceRadius
		return config, []History{}, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT h.id,u.display_name,coalesce(h.before_state::text,''),h.after_state::text,h.reason,h.created_at FROM app.teacher_attendance_location_history h JOIN app.users u ON u.id=h.actor_id WHERE h.location_id=1 ORDER BY h.id DESC LIMIT 100`)
	if err != nil {
		return config, nil, err
	}
	defer rows.Close()
	var history []History
	for rows.Next() {
		var item History
		if err := rows.Scan(&item.ID, &item.Actor, &item.Before, &item.After, &item.Reason, &item.At); err != nil {
			return config, nil, err
		}
		item.At = item.At.In(time.FixedZone("WIB", 7*60*60))
		history = append(history, item)
	}
	return config, history, rows.Err()
}

func scanTeacherAttendanceLocation(row rowScanner) (TeacherAttendanceLocationConfig, error) {
	var value TeacherAttendanceLocationConfig
	err := row.Scan(&value.Latitude, &value.Longitude, &value.RadiusM, &value.Version, &value.UpdatedBy, &value.UpdatedAt)
	if err == nil {
		value.Configured = true
	}
	return value, err
}

func (s *Store) SaveTeacherAttendanceLocation(ctx context.Context, actor int64, input TeacherAttendanceLocationInput) error {
	if err := validateTeacherLocation(input.Latitude, input.Longitude, input.RadiusM); err != nil {
		return err
	}
	if input.ExpectedVersion < 0 {
		return ValidationError("Versi konfigurasi tidak valid.")
	}
	if err := validateLocationReason(input.Reason); err != nil {
		return err
	}
	tx, err := s.begin(ctx, actor, "admin")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, err := scanTeacherAttendanceLocation(tx.QueryRowContext(ctx, `SELECT latitude,longitude,radius_m,version,updated_by,updated_at FROM app.teacher_attendance_location WHERE id=1 FOR UPDATE`))
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows {
		if input.ExpectedVersion != 0 {
			return ErrConflict
		}
		var updatedAt time.Time
		err = tx.QueryRowContext(ctx, `INSERT INTO app.teacher_attendance_location(id,latitude,longitude,radius_m,version,updated_by) VALUES(1,$1,$2,$3,1,$4) RETURNING updated_at`, input.Latitude, input.Longitude, input.RadiusM, actor).Scan(&updatedAt)
		if err != nil {
			return dbError(err)
		}
		current := TeacherAttendanceLocationConfig{Latitude: input.Latitude, Longitude: input.Longitude, RadiusM: input.RadiusM, Version: 1, UpdatedBy: actor, UpdatedAt: updatedAt, Configured: true}
		if err = insertTeacherLocationHistory(ctx, tx, actor, nil, current, input.Reason); err != nil {
			return err
		}
		return tx.Commit()
	}
	if old.Version != input.ExpectedVersion {
		return ErrConflict
	}
	if old.Latitude == input.Latitude && old.Longitude == input.Longitude && old.RadiusM == input.RadiusM {
		return tx.Commit()
	}
	current := TeacherAttendanceLocationConfig{Latitude: input.Latitude, Longitude: input.Longitude, RadiusM: input.RadiusM, Version: old.Version + 1, UpdatedBy: actor, Configured: true}
	if err = tx.QueryRowContext(ctx, `UPDATE app.teacher_attendance_location SET latitude=$1,longitude=$2,radius_m=$3,version=version+1,updated_by=$4,updated_at=clock_timestamp() WHERE id=1 RETURNING updated_at`, input.Latitude, input.Longitude, input.RadiusM, actor).Scan(&current.UpdatedAt); err != nil {
		return dbError(err)
	}
	if err = insertTeacherLocationHistory(ctx, tx, actor, &old, current, input.Reason); err != nil {
		return err
	}
	return tx.Commit()
}

func insertTeacherLocationHistory(ctx context.Context, tx *sql.Tx, actor int64, before *TeacherAttendanceLocationConfig, after TeacherAttendanceLocationConfig, reason string) error {
	var previous any
	if before != nil {
		b, err := json.Marshal(before)
		if err != nil {
			return err
		}
		previous = b
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.teacher_attendance_location_history(location_id,actor_id,before_state,after_state,reason) VALUES(1,$1,$2,$3,$4)`, actor, previous, a, strings.TrimSpace(reason))
	return err
}
