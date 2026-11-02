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
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

var ErrTeacherReportOverlap = errors.New("Rentang tanggal bertumpuk dengan laporan aktif. Hubungi TU untuk mengoreksi laporan sebelumnya.")

type TeacherAttendanceReportInput struct {
	Kind, StartsOn, EndsOn, Description, IdempotencyKey string
}

type TeacherAttendanceReportCorrection struct {
	ReportID, ExpectedVersion                   int64
	Kind, StartsOn, EndsOn, Description, Reason string
	Cancel                                      bool
}

type TeacherAttendanceReport struct {
	ID, TeacherID, RecordedBy, Version               int64
	TeacherName, Kind, StartsOn, EndsOn, Description string
	Active                                           bool
	RecordedAt, UpdatedAt                            time.Time
	CanManage                                        bool
	History                                          []TeacherAttendanceReportHistory
}

func (r TeacherAttendanceReport) KindLabel() string {
	switch r.Kind {
	case "izin":
		return "Izin"
	case "sakit":
		return "Sakit"
	case "dinas_luar":
		return "Dinas Luar"
	}
	return ""
}
func (r TeacherAttendanceReport) CategoryLabel() string {
	if r.Kind == "dinas_luar" {
		return "Hadir (Dinas Luar)"
	}
	return "Tidak Hadir (" + r.KindLabel() + ")"
}

type TeacherAttendanceReportHistory struct {
	Action, Actor, Reason string
	At                    time.Time
	Before                *TeacherAttendanceReport
	After                 TeacherAttendanceReport
}

type validatedTeacherReport struct{ Kind, StartsOn, EndsOn, Description string }

func validateTeacherReport(in TeacherAttendanceReportInput) (validatedTeacherReport, error) {
	v := validatedTeacherReport{strings.TrimSpace(in.Kind), strings.TrimSpace(in.StartsOn), strings.TrimSpace(in.EndsOn), strings.TrimSpace(in.Description)}
	if v.Kind != "izin" && v.Kind != "sakit" && v.Kind != "dinas_luar" {
		return v, ValidationError("Pilih jenis laporan Izin, Sakit, atau Dinas Luar.")
	}
	start, e := time.Parse("2006-01-02", v.StartsOn)
	end, e2 := time.Parse("2006-01-02", v.EndsOn)
	if e != nil || e2 != nil || start.Year() < 1 || end.Year() < 1 || start.Format("2006-01-02") != v.StartsOn || end.Format("2006-01-02") != v.EndsOn {
		return v, ValidationError("Tanggal mulai dan selesai harus berupa tanggal kalender yang valid.")
	}
	if start.After(end) {
		return v, ValidationError("Tanggal mulai tidak boleh melewati tanggal selesai.")
	}
	if !utf8.ValidString(v.Description) || utf8.RuneCountInString(v.Description) < 1 || utf8.RuneCountInString(v.Description) > 500 || strings.ContainsRune(v.Description, 0) {
		return v, ValidationError("Keterangan wajib diisi, maksimal 500 karakter.")
	}
	return v, nil
}

const teacherReportColumns = `id,teacher_id,teacher_name,kind,starts_on::text,ends_on::text,description,active,recorded_by,recorded_at,updated_at,version`

func scanTeacherReport(row rowScanner) (TeacherAttendanceReport, error) {
	var r TeacherAttendanceReport
	e := row.Scan(&r.ID, &r.TeacherID, &r.TeacherName, &r.Kind, &r.StartsOn, &r.EndsOn, &r.Description, &r.Active, &r.RecordedBy, &r.RecordedAt, &r.UpdatedAt, &r.Version)
	if errors.Is(e, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	r.RecordedAt, r.UpdatedAt = TeacherAttendanceWIBTime(r.RecordedAt), TeacherAttendanceWIBTime(r.UpdatedAt)
	return r, e
}

func teacherReportDBError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23P01" {
			return ErrTeacherReportOverlap
		}
		if pg.Code == "23505" {
			return ErrConflict
		}
	}
	return err
}

// CreateTeacherAttendanceReport mencatat satu rentang inklusif tanpa mengubah event Masuk/Pulang.
func (s *Store) CreateTeacherAttendanceReport(ctx context.Context, actor int64, in TeacherAttendanceReportInput) (TeacherAttendanceReport, error) {
	v, e := validateTeacherReport(in)
	if e != nil {
		return TeacherAttendanceReport{}, e
	}
	keyBytes, keyError := hex.DecodeString(in.IdempotencyKey)
	if keyError != nil || len(keyBytes) != 16 {
		return TeacherAttendanceReport{}, ValidationError("Kunci pengiriman laporan tidak valid. Muat ulang formulir.")
	}
	tx, e := s.beginAny(ctx, actor, "guru_mapel", "wali_kelas")
	if e != nil {
		return TeacherAttendanceReport{}, e
	}
	defer tx.Rollback()
	teacher, name, e := activeTeacherForAttendance(ctx, tx, actor)
	if e != nil {
		return TeacherAttendanceReport{}, e
	}
	raw, marshalErr := json.Marshal(struct {
		Actor, Teacher int64
		Input          validatedTeacherReport
	}{actor, teacher, v})
	if marshalErr != nil {
		return TeacherAttendanceReport{}, marshalErr
	}
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	key := sha256.Sum256([]byte(in.IdempotencyKey))
	keyHash := hex.EncodeToString(key[:])
	var id int64
	var storedFingerprint string
	e = tx.QueryRowContext(ctx, `SELECT id,creation_fingerprint FROM app.teacher_attendance_reports WHERE request_key_hash=$1`, keyHash).Scan(&id, &storedFingerprint)
	if e == nil {
		if storedFingerprint != fingerprint {
			return TeacherAttendanceReport{}, ErrConflict
		}
		// Retry membaca hasil permanen; koreksi TU sesudahnya tidak diputar ulang.
		return scanTeacherReport(tx.QueryRowContext(ctx, `SELECT `+teacherReportColumns+` FROM app.teacher_attendance_reports WHERE id=$1`, id))
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return TeacherAttendanceReport{}, e
	}
	r, e := scanTeacherReport(tx.QueryRowContext(ctx, `INSERT INTO app.teacher_attendance_reports(teacher_id,teacher_name,kind,starts_on,ends_on,description,recorded_by,request_key_hash,creation_fingerprint) VALUES($1,$2,$3,$4::date,$5::date,$6,$7,$8,$9) RETURNING `+teacherReportColumns, teacher, name, v.Kind, v.StartsOn, v.EndsOn, v.Description, actor, keyHash, fingerprint))
	if e != nil {
		return r, teacherReportDBError(e)
	}
	if e = recordTeacherReportHistory(ctx, tx, actor, "recorded", "", nil, r); e != nil {
		return TeacherAttendanceReport{}, e
	}
	if e = tx.Commit(); e != nil {
		return TeacherAttendanceReport{}, teacherReportDBError(e)
	}
	return r, nil
}

// CorrectTeacherAttendanceReport menjaga versi dan histori koreksi/pembatalan atomik.
func (s *Store) CorrectTeacherAttendanceReport(ctx context.Context, actor int64, in TeacherAttendanceReportCorrection) (TeacherAttendanceReport, error) {
	if in.ReportID < 1 || in.ExpectedVersion < 1 {
		return TeacherAttendanceReport{}, ValidationError("ID atau versi laporan tidak valid.")
	}
	reason, e := validateTeacherAttendanceCorrectionReason(in.Reason, "Alasan koreksi atau pembatalan wajib diisi, maksimal 500 karakter.")
	if e != nil {
		return TeacherAttendanceReport{}, e
	}
	var v validatedTeacherReport
	if !in.Cancel {
		v, e = validateTeacherReport(TeacherAttendanceReportInput{Kind: in.Kind, StartsOn: in.StartsOn, EndsOn: in.EndsOn, Description: in.Description})
		if e != nil {
			return TeacherAttendanceReport{}, e
		}
	}
	tx, e := s.beginAny(ctx, actor, "admin", "pengelola_kehadiran_guru")
	if e != nil {
		return TeacherAttendanceReport{}, e
	}
	defer tx.Rollback()
	old, e := scanTeacherReport(tx.QueryRowContext(ctx, `SELECT `+teacherReportColumns+` FROM app.teacher_attendance_reports WHERE id=$1 FOR UPDATE`, in.ReportID))
	if e != nil {
		return old, e
	}
	if e = teacherAttendanceManagerAccess(ctx, tx, actor, old.TeacherID); e != nil {
		return TeacherAttendanceReport{}, e
	}
	if old.Version != in.ExpectedVersion {
		return TeacherAttendanceReport{}, ErrConflict
	}
	if !old.Active {
		return TeacherAttendanceReport{}, ValidationError("Laporan sudah dibatalkan dan tidak dapat diubah kembali.")
	}
	action := "corrected"
	if in.Cancel {
		v = validatedTeacherReport{old.Kind, old.StartsOn, old.EndsOn, old.Description}
		action = "cancelled"
	} else if old.Kind == v.Kind && old.StartsOn == v.StartsOn && old.EndsOn == v.EndsOn && old.Description == v.Description {
		return old, nil
	}
	r, e := scanTeacherReport(tx.QueryRowContext(ctx, `UPDATE app.teacher_attendance_reports SET kind=$2,starts_on=$3::date,ends_on=$4::date,description=$5,active=$6,version=version+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING `+teacherReportColumns, old.ID, v.Kind, v.StartsOn, v.EndsOn, v.Description, !in.Cancel))
	if e != nil {
		return TeacherAttendanceReport{}, teacherReportDBError(e)
	}
	if e = recordTeacherReportHistory(ctx, tx, actor, action, reason, &old, r); e != nil {
		return TeacherAttendanceReport{}, e
	}
	if e = tx.Commit(); e != nil {
		return TeacherAttendanceReport{}, teacherReportDBError(e)
	}
	return r, nil
}

func recordTeacherReportHistory(ctx context.Context, tx *sql.Tx, actor int64, action, reason string, before *TeacherAttendanceReport, after TeacherAttendanceReport) error {
	var b any
	if before != nil {
		raw, e := json.Marshal(before)
		if e != nil {
			return e
		}
		b = string(raw)
	}
	a, e := json.Marshal(after)
	if e != nil {
		return e
	}
	var why any
	if reason != "" {
		why = reason
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO app.teacher_attendance_report_history(report_id,actor_id,action,reason,before_state,after_state) VALUES($1,$2,$3,$4,$5,$6)`, after.ID, actor, action, why, b, string(a))
	return e
}
