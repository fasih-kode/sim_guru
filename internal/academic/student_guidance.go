package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fasih-kode/sim_guru/internal/studentaffairs"
)

const studentGuidancePageSize = 10

var _ studentaffairs.GuidanceService = (*Store)(nil)

func (s *Store) CreateStudentGuidance(ctx context.Context, actor int64, input studentaffairs.GuidanceInput) (studentaffairs.GuidanceRecord, error) {
	if err := input.Validate(); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	tx, err := s.begin(ctx, actor, studentaffairs.GuidanceRoleKesiswaan)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, guidanceAuthError(err)
	}
	defer tx.Rollback()
	name, classID, className, err := guidanceStudentSnapshot(ctx, tx, input.StudentID, input.Date)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	var record studentaffairs.GuidanceRecord
	err = tx.QueryRowContext(ctx, `INSERT INTO app.student_guidance_records(student_id,student_name_at_event,class_id_at_event,class_name_at_event,occurred_on,incident,action,status,recorded_by)
		VALUES($1,$2,$3,$4,$5::date,$6,$7,$8,$9)
		RETURNING id,student_id,recorded_by,version,class_id_at_event,student_name_at_event,class_name_at_event,occurred_on::text,incident,action,status,created_at,updated_at`,
		input.StudentID, name, classID, className, input.Date, input.Incident, input.Action, string(input.Status), actor).Scan(
		&record.ID, &record.StudentID, &record.RecordedByID, &record.Version, &record.ClassIDAtEvent,
		&record.StudentNameAtEvent, &record.ClassNameAtEvent, &record.Date, &record.Incident, &record.Action,
		&record.Status, &record.RecordedAt, &record.UpdatedAt)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, dbError(err)
	}
	if err = tx.QueryRowContext(ctx, `SELECT display_name FROM app.users WHERE id=$1`, actor).Scan(&record.RecordedByName); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	snapshot := guidanceRecordSnapshot(record)
	after, err := json.Marshal(snapshot)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_guidance_history(record_id,actor_id,change_type,after_state,reason) VALUES($1,$2,'recorded',$3,'Pencatatan awal')`, record.ID, actor, string(after)); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	return record, nil
}

func guidanceStudentSnapshot(ctx context.Context, tx *sql.Tx, studentID int64, onDate string) (string, int64, string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT s.name,c.id,c.grade::text||c.name
		FROM app.students s
		JOIN app.student_placements p ON p.student_id=s.id
		JOIN app.classes c ON c.id=app.student_placement_class_at(p.id,$2::date) AND c.school_year_id=p.school_year_id
		WHERE s.id=$1 AND p.starts_on IS NOT NULL AND p.starts_on<=$2::date
		AND app.student_placement_class_at(p.id,$2::date) IS NOT NULL
		AND EXISTS(SELECT 1 FROM app.semesters sm JOIN app.semester_calendars sc ON sc.semester_id=sm.id WHERE sm.school_year_id=p.school_year_id AND $2::date BETWEEN sc.starts_on AND sc.ends_on)
		ORDER BY p.id`, studentID, onDate)
	if err != nil {
		return "", 0, "", err
	}
	defer rows.Close()
	var name, className string
	var classID int64
	count := 0
	for rows.Next() {
		if err = rows.Scan(&name, &classID, &className); err != nil {
			return "", 0, "", err
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return "", 0, "", err
	}
	if count != 1 {
		return "", 0, "", studentaffairs.ErrGuidancePlacementUnclear
	}
	return name, classID, className, nil
}

func (s *Store) UpdateStudentGuidance(ctx context.Context, actor, recordID, expectedVersion int64, change studentaffairs.GuidanceContentChange) (studentaffairs.GuidanceRecord, error) {
	if err := change.Validate(); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	tx, err := s.begin(ctx, actor, studentaffairs.GuidanceRoleKesiswaan)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, guidanceAuthError(err)
	}
	defer tx.Rollback()
	current, err := guidanceRecordForMutation(ctx, tx, recordID)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if current.Version != expectedVersion {
		return studentaffairs.GuidanceRecord{}, studentaffairs.ErrGuidanceConflict
	}
	before, err := json.Marshal(guidanceRecordSnapshot(current))
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	err = tx.QueryRowContext(ctx, `UPDATE app.student_guidance_records SET incident=$1,action=$2,version=version+1,updated_at=clock_timestamp() WHERE id=$3 AND version=$4
		RETURNING id,student_id,recorded_by,version,class_id_at_event,student_name_at_event,class_name_at_event,occurred_on::text,incident,action,status,created_at,updated_at`,
		change.Incident, change.Action, recordID, expectedVersion).Scan(
		&current.ID, &current.StudentID, &current.RecordedByID, &current.Version, &current.ClassIDAtEvent,
		&current.StudentNameAtEvent, &current.ClassNameAtEvent, &current.Date, &current.Incident, &current.Action,
		&current.Status, &current.RecordedAt, &current.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return studentaffairs.GuidanceRecord{}, studentaffairs.ErrGuidanceConflict
	}
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	after, err := json.Marshal(guidanceRecordSnapshot(current))
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_guidance_history(record_id,actor_id,change_type,before_state,after_state,reason) VALUES($1,$2,'content_changed',$3,$4,$5)`, recordID, actor, string(before), string(after), change.Reason); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	return current, nil
}

func (s *Store) ChangeStudentGuidanceStatus(ctx context.Context, actor, recordID, expectedVersion int64, change studentaffairs.GuidanceStatusChange) (studentaffairs.GuidanceRecord, error) {
	tx, err := s.begin(ctx, actor, studentaffairs.GuidanceRoleKesiswaan)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, guidanceAuthError(err)
	}
	defer tx.Rollback()
	current, err := guidanceRecordForMutation(ctx, tx, recordID)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if expectedVersion != current.Version {
		return studentaffairs.GuidanceRecord{}, studentaffairs.ErrGuidanceConflict
	}
	if err = change.Validate(current.Status); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	// Waktu histori dan pembaruan berasal dari PostgreSQL, bukan jam klien.
	var recordedAt time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&recordedAt); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	updated, history, err := studentaffairs.ChangeGuidanceStatus(current, actor, expectedVersion, change, recordedAt)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	before, err := json.Marshal(history.Before)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	after, err := json.Marshal(history.After)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	err = tx.QueryRowContext(ctx, `UPDATE app.student_guidance_records SET status=$1,version=$2,updated_at=$3 WHERE id=$4 AND version=$5
		RETURNING id,student_id,recorded_by,version,class_id_at_event,class_name_at_event,student_name_at_event,occurred_on::text,incident,action,status,created_at,updated_at`,
		string(updated.Status), updated.Version, recordedAt, recordID, expectedVersion).Scan(
		&updated.ID, &updated.StudentID, &updated.RecordedByID, &updated.Version, &updated.ClassIDAtEvent,
		&updated.ClassNameAtEvent, &updated.StudentNameAtEvent, &updated.Date, &updated.Incident, &updated.Action,
		&updated.Status, &updated.RecordedAt, &updated.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return studentaffairs.GuidanceRecord{}, studentaffairs.ErrGuidanceConflict
	}
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_guidance_history(record_id,actor_id,change_type,before_state,after_state,reason) VALUES($1,$2,'status_changed',$3,$4,$5)`, recordID, actor, string(before), string(after), history.Reason); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	return updated, nil
}

func guidanceRecordForMutation(ctx context.Context, tx *sql.Tx, id int64) (studentaffairs.GuidanceRecord, error) {
	if id < 1 {
		return studentaffairs.GuidanceRecord{}, studentaffairs.ErrGuidanceNotFound
	}
	var record studentaffairs.GuidanceRecord
	err := tx.QueryRowContext(ctx, `SELECT r.id,r.student_id,r.recorded_by,u.display_name,r.version,r.class_id_at_event,r.student_name_at_event,r.class_name_at_event,r.occurred_on::text,r.incident,r.action,r.status,r.created_at,r.updated_at FROM app.student_guidance_records r JOIN app.users u ON u.id=r.recorded_by WHERE r.id=$1 FOR UPDATE OF r`, id).Scan(
		&record.ID, &record.StudentID, &record.RecordedByID, &record.RecordedByName, &record.Version, &record.ClassIDAtEvent,
		&record.StudentNameAtEvent, &record.ClassNameAtEvent, &record.Date, &record.Incident, &record.Action,
		&record.Status, &record.RecordedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return studentaffairs.GuidanceRecord{}, studentaffairs.ErrGuidanceNotFound
	}
	return record, err
}

func guidanceRecordSnapshot(record studentaffairs.GuidanceRecord) studentaffairs.GuidanceSnapshot {
	return studentaffairs.GuidanceSnapshot{
		StudentID: record.StudentID, ClassIDAtEvent: record.ClassIDAtEvent,
		StudentNameAtEvent: record.StudentNameAtEvent, ClassNameAtEvent: record.ClassNameAtEvent,
		Date: record.Date, Incident: record.Incident, Action: record.Action, Status: record.Status,
	}
}

func (s *Store) GetStudentGuidance(ctx context.Context, actor, recordID int64) (studentaffairs.GuidanceRecord, error) {
	tx, err := s.guidanceReader(ctx, actor)
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	defer tx.Rollback()
	var record studentaffairs.GuidanceRecord
	err = tx.QueryRowContext(ctx, guidanceSelect+` WHERE r.id=$1`, recordID).Scan(guidanceRecordDestinations(&record)...)
	if errors.Is(err, sql.ErrNoRows) {
		return studentaffairs.GuidanceRecord{}, studentaffairs.ErrGuidanceNotFound
	}
	if err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	if err = guidanceRecordReadable(ctx, tx, actor, record.StudentID); err != nil {
		return studentaffairs.GuidanceRecord{}, err
	}
	return record, nil
}

const guidanceSelect = `SELECT r.id,r.student_id,r.recorded_by,u.display_name,r.version,r.class_id_at_event,r.student_name_at_event,r.class_name_at_event,r.occurred_on::text,r.incident,r.action,r.status,r.created_at,r.updated_at FROM app.student_guidance_records r JOIN app.users u ON u.id=r.recorded_by`

func guidanceRecordDestinations(record *studentaffairs.GuidanceRecord) []any {
	return []any{&record.ID, &record.StudentID, &record.RecordedByID, &record.RecordedByName, &record.Version, &record.ClassIDAtEvent,
		&record.StudentNameAtEvent, &record.ClassNameAtEvent, &record.Date, &record.Incident, &record.Action,
		&record.Status, &record.RecordedAt, &record.UpdatedAt}
}

func (s *Store) ListStudentGuidance(ctx context.Context, actor int64, filter studentaffairs.GuidanceFilter) (studentaffairs.GuidanceList, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Page > 100000 || filter.StudentID < 0 || !utf8.ValidString(filter.Search) || strings.ContainsRune(filter.Search, 0) || utf8.RuneCountInString(filter.Search) > 120 || (filter.Status != "" && !filter.Status.Valid()) {
		return studentaffairs.GuidanceList{}, ValidationError("Filter catatan pembinaan tidak valid.")
	}
	tx, err := s.guidanceReader(ctx, actor)
	if err != nil {
		return studentaffairs.GuidanceList{}, err
	}
	defer tx.Rollback()
	var fullAccess, waliAccess bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.user_roles WHERE user_id=$1 AND role_code IN('kesiswaan','kepala_sekolah')),
		EXISTS(SELECT 1 FROM app.user_roles WHERE user_id=$1 AND role_code='wali_kelas')`, actor).Scan(&fullAccess, &waliAccess)
	if err != nil {
		return studentaffairs.GuidanceList{}, err
	}
	if !fullAccess && !waliAccess {
		return studentaffairs.GuidanceList{}, studentaffairs.ErrGuidanceForbidden
	}
	result := studentaffairs.GuidanceList{Page: filter.Page, HasPrevious: filter.Page > 1}
	query := guidanceSelect + ` WHERE ($1::bigint=0 OR r.student_id=$1) AND ($2='' OR r.status=$2) AND ($3='' OR r.student_name_at_event ILIKE $3 OR r.class_name_at_event ILIKE $3 OR r.incident ILIKE $3 OR r.action ILIKE $3)
		AND ($4::boolean OR ($5::boolean AND ` + guidanceCurrentWaliScope + `))
		ORDER BY r.occurred_on DESC,r.id DESC LIMIT $6 OFFSET $7`
	rows, err := tx.QueryContext(ctx, query, filter.StudentID, string(filter.Status), "%"+strings.TrimSpace(filter.Search)+"%", fullAccess, waliAccess, studentGuidancePageSize+1, (filter.Page-1)*studentGuidancePageSize, actor)
	if err != nil {
		return studentaffairs.GuidanceList{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var record studentaffairs.GuidanceRecord
		if err = rows.Scan(guidanceRecordDestinations(&record)...); err != nil {
			return studentaffairs.GuidanceList{}, err
		}
		result.Records = append(result.Records, record)
	}
	if err = rows.Err(); err != nil {
		return studentaffairs.GuidanceList{}, err
	}
	if len(result.Records) > studentGuidancePageSize {
		result.HasNext = true
		result.Records = result.Records[:studentGuidancePageSize]
	}
	return result, nil
}

const guidanceCurrentWaliScope = `EXISTS(
	SELECT 1 FROM app.student_placements p
	JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
	JOIN app.classes c ON c.id=app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AND c.school_year_id=sy.id
	JOIN app.homeroom_assignments h ON h.class_id=c.id
	JOIN app.teachers t ON t.id=h.teacher_id AND t.user_id=$8 AND t.active
	WHERE p.student_id=r.student_id AND p.starts_on IS NOT NULL AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
	AND (SELECT count(*) FROM app.student_placements p2 JOIN app.school_years sy2 ON sy2.id=p2.school_year_id AND sy2.active
		WHERE p2.student_id=p.student_id AND p2.starts_on IS NOT NULL AND p2.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
		AND app.student_placement_class_at(p2.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL)=1
)`

func (s *Store) guidanceReader(ctx context.Context, actor int64) (*sql.Tx, error) {
	if actor < 1 {
		return nil, studentaffairs.ErrGuidanceForbidden
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	if err = s.authorize(ctx, tx, actor, studentaffairs.GuidanceRoleKesiswaan, studentaffairs.GuidanceRoleHeadmaster, studentaffairs.GuidanceRoleHomeroom); err != nil {
		_ = tx.Rollback()
		return nil, guidanceAuthError(err)
	}
	return tx, nil
}

func guidanceAuthError(err error) error {
	if errors.Is(err, ErrForbidden) {
		return studentaffairs.ErrGuidanceForbidden
	}
	return err
}

func guidanceRecordReadable(ctx context.Context, tx *sql.Tx, actor, studentID int64) error {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.user_roles WHERE user_id=$1 AND role_code IN('kesiswaan','kepala_sekolah')) OR
		(EXISTS(SELECT 1 FROM app.user_roles WHERE user_id=$1 AND role_code='wali_kelas') AND
		EXISTS(SELECT 1 FROM app.student_placements p
		JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
		JOIN app.classes c ON c.id=app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AND c.school_year_id=sy.id
		JOIN app.homeroom_assignments h ON h.class_id=c.id
		JOIN app.teachers t ON t.id=h.teacher_id AND t.user_id=$1 AND t.active
		WHERE p.student_id=$2 AND p.starts_on IS NOT NULL AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
		AND (SELECT count(*) FROM app.student_placements p2 JOIN app.school_years sy2 ON sy2.id=p2.school_year_id AND sy2.active WHERE p2.student_id=p.student_id AND p2.starts_on IS NOT NULL AND p2.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date AND app.student_placement_class_at(p2.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL)=1))`, actor, studentID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return studentaffairs.ErrGuidanceNotFound
	}
	return nil
}

func (s *Store) StudentGuidanceHistory(ctx context.Context, actor, recordID int64) ([]studentaffairs.GuidanceHistoryEntry, error) {
	tx, err := s.guidanceReader(ctx, actor)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var studentID int64
	err = tx.QueryRowContext(ctx, `SELECT student_id FROM app.student_guidance_records WHERE id=$1`, recordID).Scan(&studentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, studentaffairs.ErrGuidanceNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = guidanceRecordReadable(ctx, tx, actor, studentID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT h.id,h.record_id,h.actor_id,u.display_name,h.change_type,h.before_state,h.after_state,h.reason,h.created_at FROM app.student_guidance_history h JOIN app.users u ON u.id=h.actor_id WHERE h.record_id=$1 ORDER BY h.id DESC LIMIT 100`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := []studentaffairs.GuidanceHistoryEntry{}
	for rows.Next() {
		var entry studentaffairs.GuidanceHistoryEntry
		var change string
		var before, after []byte
		if err = rows.Scan(&entry.ID, &entry.RecordID, &entry.ActorID, &entry.ActorName, &change, &before, &after, &entry.Reason, &entry.RecordedAt); err != nil {
			return nil, err
		}
		entry.RecordedAt = entry.RecordedAt.In(time.FixedZone("WIB", 7*60*60))
		entry.Change = studentaffairs.GuidanceChange(change)
		if len(before) > 0 {
			entry.Before = new(studentaffairs.GuidanceSnapshot)
			if err = json.Unmarshal(before, entry.Before); err != nil {
				return nil, err
			}
		}
		if err = json.Unmarshal(after, &entry.After); err != nil {
			return nil, err
		}
		history = append(history, entry)
	}
	return history, rows.Err()
}

func (s *Store) StudentGuidanceStudents(ctx context.Context, actor int64) ([]studentaffairs.GuidanceStudentOption, error) {
	tx, err := s.reader(ctx, actor, studentaffairs.GuidanceRoleKesiswaan)
	if err != nil {
		return nil, guidanceAuthError(err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.name,c.grade::text||c.name
		FROM app.student_placements p
		JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
		JOIN app.students s ON s.id=p.student_id AND s.active
		JOIN app.classes c ON c.id=app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AND c.school_year_id=sy.id
		WHERE p.starts_on IS NOT NULL AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
		ORDER BY c.grade,c.name,s.name,s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	students := []studentaffairs.GuidanceStudentOption{}
	for rows.Next() {
		var item studentaffairs.GuidanceStudentOption
		if err = rows.Scan(&item.ID, &item.Name, &item.ClassName); err != nil {
			return nil, err
		}
		students = append(students, item)
	}
	return students, rows.Err()
}
