package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type TeacherAttendanceManagedEvent struct {
	ID, Version            int64
	Action                 TeacherAttendanceAction
	Source                 string
	OccurredAt, RecordedAt time.Time
}

type TeacherAttendanceManagedTeacher struct {
	ID                int64
	Name              string
	Active            bool
	IsActor           bool
	CanManage         bool
	CheckIn, CheckOut *TeacherAttendanceManagedEvent
}

type TeacherAttendanceCorrectionRequestInput struct {
	OnDate      string
	Action      TeacherAttendanceAction
	Kind        string
	RequestText string
}

type TeacherAttendanceCorrectionRequest struct {
	ID        int64
	Teacher   string
	OnDate    string
	Action    TeacherAttendanceAction
	Kind      string
	Text      string
	CreatedAt time.Time
}

type TeacherAttendanceManagedHistory struct {
	Action, Actor, Reason     string
	At                        time.Time
	BeforeAt, AfterAt         *time.Time
	BeforeReason, AfterReason string
}

type TeacherAttendanceManagedRecord struct {
	ID, TeacherID, RecordedBy, Version                        int64
	TeacherName, RecordedByName, OnDate, Source, ManualReason string
	Action                                                    TeacherAttendanceAction
	OccurredAt, RecordedAt                                    time.Time
	CanCorrect                                                bool
	History                                                   []TeacherAttendanceManagedHistory
}

// TeacherAttendanceManagedRecordDetail menampilkan bukti dan histori kepada TU berwenang.
func (s *Store) TeacherAttendanceManagedRecordDetail(ctx context.Context, actorID, eventID int64) (TeacherAttendanceManagedRecord, error) {
	tx, err := s.readerAll(ctx, actorID, "pegawai_tu", "pengelola_kehadiran_guru")
	if err != nil {
		return TeacherAttendanceManagedRecord{}, err
	}
	defer tx.Rollback()
	var allowed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.user_roles WHERE user_id=$1 AND role_code='pengelola_kehadiran_guru')`, actorID).Scan(&allowed); err != nil {
		return TeacherAttendanceManagedRecord{}, err
	}
	if !allowed {
		return TeacherAttendanceManagedRecord{}, ErrForbidden
	}
	return loadTeacherAttendanceManagedRecord(ctx, tx, actorID, eventID)
}

// TeacherAttendanceManagedRecordDetailForAdmin membatasi Admin pada catatan pengelola TU.
func (s *Store) TeacherAttendanceManagedRecordDetailForAdmin(ctx context.Context, actorID, eventID int64) (TeacherAttendanceManagedRecord, error) {
	tx, err := s.reader(ctx, actorID, "admin")
	if err != nil {
		return TeacherAttendanceManagedRecord{}, err
	}
	defer tx.Rollback()
	var teacherID int64
	if err = tx.QueryRowContext(ctx, `SELECT teacher_id FROM app.teacher_attendance_events WHERE id=$1`, eventID).Scan(&teacherID); err == sql.ErrNoRows {
		return TeacherAttendanceManagedRecord{}, ErrNotFound
	} else if err != nil {
		return TeacherAttendanceManagedRecord{}, err
	}
	if err = teacherAttendanceManagerAccess(ctx, tx, actorID, teacherID); err != nil {
		return TeacherAttendanceManagedRecord{}, err
	}
	return loadTeacherAttendanceManagedRecord(ctx, tx, actorID, eventID)
}

func loadTeacherAttendanceManagedRecord(ctx context.Context, tx *sql.Tx, actorID, eventID int64) (TeacherAttendanceManagedRecord, error) {
	var record TeacherAttendanceManagedRecord
	err := tx.QueryRowContext(ctx, `SELECT e.id,e.teacher_id,e.teacher_name,e.on_date::text,e.action,e.source,e.actor_id,u.display_name,e.occurred_at,e.recorded_at,e.version,coalesce(e.manual_reason,''),(NOT EXISTS(SELECT 1 FROM app.teachers t WHERE t.id=e.teacher_id AND t.user_id=$2) OR EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$2 AND r.role_code='admin')) FROM app.teacher_attendance_events e JOIN app.users u ON u.id=e.actor_id WHERE e.id=$1`, eventID, actorID).Scan(
		&record.ID, &record.TeacherID, &record.TeacherName, &record.OnDate, &record.Action, &record.Source, &record.RecordedBy, &record.RecordedByName, &record.OccurredAt, &record.RecordedAt, &record.Version, &record.ManualReason, &record.CanCorrect)
	if err == sql.ErrNoRows {
		return record, ErrNotFound
	}
	if err != nil {
		return record, err
	}
	record.OccurredAt, record.RecordedAt = TeacherAttendanceWIBTime(record.OccurredAt), TeacherAttendanceWIBTime(record.RecordedAt)
	rows, err := tx.QueryContext(ctx, `SELECT h.action,u.display_name,h.reason,h.created_at,h.before_state,h.after_state FROM app.teacher_attendance_history h JOIN app.users u ON u.id=h.actor_id WHERE h.event_id=$1 ORDER BY h.id`, eventID)
	if err != nil {
		return record, err
	}
	defer rows.Close()
	for rows.Next() {
		var item TeacherAttendanceManagedHistory
		var reason sql.NullString
		var before, after []byte
		if err = rows.Scan(&item.Action, &item.Actor, &reason, &item.At, &before, &after); err != nil {
			return record, err
		}
		item.Reason, item.At = reason.String, TeacherAttendanceWIBTime(item.At)
		if len(before) > 0 {
			var snapshot teacherAttendanceCorrectionSnapshot
			if err = json.Unmarshal(before, &snapshot); err != nil {
				return record, err
			}
			t := TeacherAttendanceWIBTime(snapshot.OccurredAt)
			item.BeforeAt, item.BeforeReason = &t, attendanceSnapshotReason(snapshot)
		}
		var snapshot teacherAttendanceCorrectionSnapshot
		if err = json.Unmarshal(after, &snapshot); err != nil {
			return record, err
		}
		t := TeacherAttendanceWIBTime(snapshot.OccurredAt)
		item.AfterAt, item.AfterReason = &t, attendanceSnapshotReason(snapshot)
		record.History = append(record.History, item)
	}
	return record, rows.Err()
}

func attendanceSnapshotReason(snapshot teacherAttendanceCorrectionSnapshot) string {
	if snapshot.ManualReason != nil {
		return *snapshot.ManualReason
	}
	return snapshot.Reason
}

// TeacherAttendanceManaged memuat catatan tanggal tertentu untuk TU berwenang.
func (s *Store) TeacherAttendanceManaged(ctx context.Context, actorID int64, onDate, search string, page int) ([]TeacherAttendanceManagedTeacher, error) {
	return s.teacherAttendanceManaged(ctx, actorID, onDate, search, page, false)
}

// TeacherAttendanceManagedForAdmin membatasi fallback Admin pada guru yang ber-role TU pengelola.
func (s *Store) TeacherAttendanceManagedForAdmin(ctx context.Context, actorID int64, onDate, search string, page int) ([]TeacherAttendanceManagedTeacher, error) {
	return s.teacherAttendanceManaged(ctx, actorID, onDate, search, page, true)
}

func (s *Store) teacherAttendanceManaged(ctx context.Context, actorID int64, onDate, search string, page int, adminOnly bool) ([]TeacherAttendanceManagedTeacher, error) {
	date, err := time.Parse("2006-01-02", onDate)
	if err != nil || page < 1 || len(search) > 120 {
		return nil, ValidationError("Tanggal atau pencarian kehadiran tidak valid.")
	}
	var tx *sql.Tx
	if adminOnly {
		tx, err = s.reader(ctx, actorID, "admin")
	} else {
		tx, err = s.readerAll(ctx, actorID, "pegawai_tu", "pengelola_kehadiran_guru")
	}
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT t.id,t.name,t.active,COALESCE(t.user_id=$4,false),i.id,i.action,i.source,i.occurred_at,i.recorded_at,i.version,o.id,o.action,o.source,o.occurred_at,o.recorded_at,o.version
		FROM app.teachers t
		LEFT JOIN app.teacher_attendance_events i ON i.teacher_id=t.id AND i.on_date=$1::date AND i.action='check_in'
		LEFT JOIN app.teacher_attendance_events o ON o.teacher_id=t.id AND o.on_date=$1::date AND o.action='check_out'
		WHERE (t.active OR i.id IS NOT NULL OR o.id IS NOT NULL)
		  AND ($2='' OR t.name ILIKE '%'||$2||'%')
		  AND (NOT $5::boolean OR t.user_id=$4)
		ORDER BY lower(t.name),t.id
		LIMIT 11 OFFSET $3`, date, strings.TrimSpace(search), (page-1)*10, actorID, adminOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var teachers []TeacherAttendanceManagedTeacher
	for rows.Next() {
		var teacher TeacherAttendanceManagedTeacher
		var checkInID, checkOutID, checkInVersion, checkOutVersion sql.NullInt64
		var checkInAction, checkInSource, checkOutAction, checkOutSource sql.NullString
		var checkInOccurred, checkInRecorded, checkOutOccurred, checkOutRecorded sql.NullTime
		if err = rows.Scan(&teacher.ID, &teacher.Name, &teacher.Active, &teacher.IsActor,
			&checkInID, &checkInAction, &checkInSource, &checkInOccurred, &checkInRecorded, &checkInVersion,
			&checkOutID, &checkOutAction, &checkOutSource, &checkOutOccurred, &checkOutRecorded, &checkOutVersion); err != nil {
			return nil, err
		}
		if checkInID.Valid {
			teacher.CheckIn = &TeacherAttendanceManagedEvent{ID: checkInID.Int64, Version: checkInVersion.Int64, Action: TeacherAttendanceAction(checkInAction.String), Source: checkInSource.String, OccurredAt: TeacherAttendanceWIBTime(checkInOccurred.Time), RecordedAt: TeacherAttendanceWIBTime(checkInRecorded.Time)}
		}
		if checkOutID.Valid {
			teacher.CheckOut = &TeacherAttendanceManagedEvent{ID: checkOutID.Int64, Version: checkOutVersion.Int64, Action: TeacherAttendanceAction(checkOutAction.String), Source: checkOutSource.String, OccurredAt: TeacherAttendanceWIBTime(checkOutOccurred.Time), RecordedAt: TeacherAttendanceWIBTime(checkOutRecorded.Time)}
		}
		teacher.CanManage = !teacher.IsActor || adminOnly
		teachers = append(teachers, teacher)
	}
	return teachers, rows.Err()
}

// TeacherAttendanceCorrectionRequests membaca permintaan guru tanpa membuka akses tulis.
func (s *Store) TeacherAttendanceCorrectionRequests(ctx context.Context, actorID int64, page int) ([]TeacherAttendanceCorrectionRequest, error) {
	if page < 1 {
		return nil, ValidationError("Halaman permintaan tidak valid.")
	}
	tx, err := s.readerAll(ctx, actorID, "pegawai_tu", "pengelola_kehadiran_guru")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,teacher_name,on_date::text,attendance_action,request_kind,request_text,created_at FROM app.teacher_attendance_correction_requests ORDER BY id DESC LIMIT 11 OFFSET $1`, (page-1)*10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var requests []TeacherAttendanceCorrectionRequest
	for rows.Next() {
		var request TeacherAttendanceCorrectionRequest
		if err = rows.Scan(&request.ID, &request.Teacher, &request.OnDate, &request.Action, &request.Kind, &request.Text, &request.CreatedAt); err != nil {
			return nil, err
		}
		request.CreatedAt = TeacherAttendanceWIBTime(request.CreatedAt)
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

// RequestTeacherAttendanceCorrection menyimpan permintaan guru tanpa mengubah catatan kehadiran.
func (s *Store) RequestTeacherAttendanceCorrection(ctx context.Context, actorID int64, input TeacherAttendanceCorrectionRequestInput) error {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(input.OnDate))
	if err != nil {
		return ValidationError("Tanggal permintaan tidak valid.")
	}
	if input.Action != TeacherCheckIn && input.Action != TeacherCheckOut {
		return ErrTeacherAttendanceAction
	}
	if input.Kind != "completion" && input.Kind != "correction" {
		return ValidationError("Jenis permintaan tidak valid.")
	}
	requestText, err := validateTeacherAttendanceCorrectionReason(input.RequestText, "Keterangan permintaan wajib diisi, maksimal 500 karakter.")
	if err != nil {
		return err
	}
	tx, err := s.beginAny(ctx, actorID, "guru_mapel", "wali_kelas")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	teacherID, teacherName, err := activeTeacherForAttendance(ctx, tx, actorID)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.teacher_attendance_correction_requests(teacher_id,teacher_name,actor_id,on_date,attendance_action,request_kind,request_text) VALUES($1,$2,$3,$4::date,$5,$6,$7)`, teacherID, teacherName, actorID, date, input.Action, input.Kind, requestText); err != nil {
		return dbError(err)
	}
	return tx.Commit()
}
