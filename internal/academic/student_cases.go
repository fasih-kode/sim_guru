package academic

import (
	"context"
	"database/sql"
	"errors"

	"github.com/fasih-kode/sim_guru/internal/studentcases"
)

const studentCasePageSize = 10

var studentCaseRoles = []string{studentcases.RoleHomeroom, studentcases.RoleCounselor, studentcases.RoleStudentAffairs, studentcases.RoleHeadmaster}

func (s *Store) CreateStudentCase(ctx context.Context, actor int64, input studentcases.CreateInput) (studentcases.Case, error) {
	if err := input.Validate(); err != nil {
		return studentcases.Case{}, err
	}
	tx, err := s.begin(ctx, actor, studentcases.RoleHomeroom)
	if err != nil {
		return studentcases.Case{}, caseAuthError(err)
	}
	defer tx.Rollback()
	name, classID, className, err := currentWaliStudentSnapshot(ctx, tx, actor, input.StudentID)
	if err != nil {
		return studentcases.Case{}, err
	}
	var item studentcases.Case
	err = tx.QueryRowContext(ctx, `INSERT INTO app.student_cases(student_id,student_name_at_open,class_id_at_open,class_name_at_open,stage,responsible_user_id,created_by)
		VALUES($1,$2,$3,$4,'wali_kelas',$5,$5)
		RETURNING id,student_id,student_name_at_open,class_id_at_open,class_name_at_open,stage,responsible_user_id,(SELECT display_name FROM app.users WHERE id=$5),status,version,created_at,updated_at`,
		input.StudentID, name, classID, className, actor).Scan(caseDestinations(&item)...)
	if err != nil {
		return studentcases.Case{}, dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_case_entries(case_id,actor_id,entry_type,content) VALUES($1,$2,'wali_pembinaan',$3)`, item.ID, actor, input.Content); err != nil {
		return studentcases.Case{}, err
	}
	if err = insertStudentCaseHistory(ctx, tx, item.ID, actor, studentcases.EventCreated, "", studentcases.StageHomeroom, 0, actor, ""); err != nil {
		return studentcases.Case{}, err
	}
	if err = tx.Commit(); err != nil {
		return studentcases.Case{}, err
	}
	return item, nil
}

func currentWaliStudentSnapshot(ctx context.Context, tx *sql.Tx, actor, studentID int64) (string, int64, string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT s.name,c.id,c.grade::text||c.name
		FROM app.students s
		JOIN app.student_placements p ON p.student_id=s.id
		JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
		JOIN app.classes c ON c.id=app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AND c.school_year_id=sy.id
		JOIN app.homeroom_assignments h ON h.class_id=c.id
		JOIN app.teachers t ON t.id=h.teacher_id AND t.user_id=$1 AND t.active
		WHERE s.id=$2 AND s.active AND p.starts_on IS NOT NULL AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
		AND app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL
		AND (SELECT count(*) FROM app.student_placements p2 JOIN app.school_years sy2 ON sy2.id=p2.school_year_id AND sy2.active
			WHERE p2.student_id=s.id AND p2.starts_on IS NOT NULL AND p2.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
			AND app.student_placement_class_at(p2.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL)=1`, actor, studentID)
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
		return "", 0, "", studentcases.ErrScopeUnclear
	}
	return name, classID, className, nil
}

func (s *Store) StudentCaseStudents(ctx context.Context, actor int64) ([]studentcases.CaseStudentOption, error) {
	tx, err := s.reader(ctx, actor, studentcases.RoleHomeroom)
	if err != nil {
		return nil, caseAuthError(err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.name,c.grade::text||c.name
		FROM app.students s
		JOIN app.student_placements p ON p.student_id=s.id
		JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
		JOIN app.classes c ON c.id=app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AND c.school_year_id=sy.id
		JOIN app.homeroom_assignments h ON h.class_id=c.id
		JOIN app.teachers t ON t.id=h.teacher_id AND t.user_id=$1 AND t.active
		WHERE s.active AND p.starts_on IS NOT NULL AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
		AND app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL
		AND (SELECT count(*) FROM app.student_placements p2 JOIN app.school_years sy2 ON sy2.id=p2.school_year_id AND sy2.active WHERE p2.student_id=s.id AND p2.starts_on IS NOT NULL AND p2.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date AND app.student_placement_class_at(p2.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL)=1
		ORDER BY c.grade,c.name,s.name,s.id`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []studentcases.CaseStudentOption{}
	for rows.Next() {
		var item studentcases.CaseStudentOption
		if err = rows.Scan(&item.ID, &item.Name, &item.ClassName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) StudentCaseBKOptions(ctx context.Context, actor int64) ([]studentcases.UserOption, error) {
	tx, err := s.reader(ctx, actor, studentcases.RoleHomeroom)
	if err != nil {
		return nil, caseAuthError(err)
	}
	defer tx.Rollback()
	items, err := caseUserOptions(ctx, tx, studentcases.RoleCounselor)
	if err == nil && len(items) == 0 {
		return nil, studentcases.ErrNoBKAvailable
	}
	return items, err
}

func (s *Store) StudentCaseKesiswaanOptions(ctx context.Context, actor int64) ([]studentcases.UserOption, error) {
	tx, err := s.reader(ctx, actor, studentcases.RoleCounselor)
	if err != nil {
		return nil, caseAuthError(err)
	}
	defer tx.Rollback()
	items, err := caseUserOptions(ctx, tx, studentcases.RoleStudentAffairs)
	if err == nil && len(items) == 0 {
		return nil, studentcases.ErrNoKesAvailable
	}
	return items, err
}

func caseUserOptions(ctx context.Context, tx *sql.Tx, role string) ([]studentcases.UserOption, error) {
	rows, err := tx.QueryContext(ctx, `SELECT u.id,u.display_name FROM app.users u JOIN app.user_roles r ON r.user_id=u.id WHERE u.active AND r.role_code=$1 ORDER BY u.display_name,u.id`, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []studentcases.UserOption{}
	for rows.Next() {
		var item studentcases.UserOption
		if err = rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListStudentCases(ctx context.Context, actor int64, filter studentcases.Filter) (studentcases.List, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if err := filter.Validate(); err != nil {
		return studentcases.List{}, err
	}
	tx, err := s.caseReader(ctx, actor)
	if err != nil {
		return studentcases.List{}, err
	}
	defer tx.Rollback()
	query := studentCaseSelect + ` WHERE ` + caseReadablePredicate + `
		AND ($2='' OR c.stage=$2) AND ($3='' OR c.status=$3)
		AND ($4='' OR c.id::text ILIKE $4 OR c.student_name_at_open ILIKE $4 OR c.class_name_at_open ILIKE $4)
		ORDER BY c.updated_at DESC,c.id DESC LIMIT $5 OFFSET $6`
	rows, err := tx.QueryContext(ctx, query, actor, string(filter.Stage), string(filter.Status), "%"+filter.Search+"%", studentCasePageSize+1, (filter.Page-1)*studentCasePageSize)
	if err != nil {
		return studentcases.List{}, err
	}
	defer rows.Close()
	result := studentcases.List{Cases: []studentcases.Case{}, Page: filter.Page}
	for rows.Next() {
		var item studentcases.Case
		if err = rows.Scan(caseDestinations(&item)...); err != nil {
			return studentcases.List{}, err
		}
		result.Cases = append(result.Cases, item)
	}
	if err = rows.Err(); err != nil {
		return studentcases.List{}, err
	}
	if len(result.Cases) > studentCasePageSize {
		result.HasNext = true
		result.Cases = result.Cases[:studentCasePageSize]
	}
	return result, nil
}

const studentCaseSelect = `SELECT c.id,c.student_id,c.student_name_at_open,c.class_id_at_open,c.class_name_at_open,c.stage,c.responsible_user_id,u.display_name,c.status,c.version,c.created_at,c.updated_at FROM app.student_cases c JOIN app.users u ON u.id=c.responsible_user_id`

func caseDestinations(item *studentcases.Case) []any {
	return []any{&item.ID, &item.StudentID, &item.StudentName, &item.ClassID, &item.ClassName, &item.Stage, &item.ResponsibleID, &item.ResponsibleName, &item.Status, &item.Version, &item.CreatedAt, &item.UpdatedAt}
}

const currentCaseWaliScope = `EXISTS(
	SELECT 1 FROM app.student_placements p
	JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
	JOIN app.classes cl ON cl.id=app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AND cl.school_year_id=sy.id
	JOIN app.homeroom_assignments h ON h.class_id=cl.id
	JOIN app.teachers t ON t.id=h.teacher_id AND t.user_id=$1 AND t.active
	WHERE p.student_id=c.student_id AND p.starts_on IS NOT NULL AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
	AND app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL
	AND (SELECT count(*) FROM app.student_placements p2 JOIN app.school_years sy2 ON sy2.id=p2.school_year_id AND sy2.active WHERE p2.student_id=c.student_id AND p2.starts_on IS NOT NULL AND p2.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date AND app.student_placement_class_at(p2.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL)=1
)`

const caseReadablePredicate = `(
	EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$1 AND r.role_code='kepala_sekolah') OR
	(EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$1 AND r.role_code='wali_kelas') AND ` + currentCaseWaliScope + `) OR
	(c.stage='guru_bk' AND c.responsible_user_id=$1 AND EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$1 AND r.role_code='bk')) OR
	(c.stage='kesiswaan' AND c.responsible_user_id=$1 AND EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$1 AND r.role_code='kesiswaan'))
)`

func (s *Store) caseReader(ctx context.Context, actor int64) (*sql.Tx, error) {
	tx, err := s.reader(ctx, actor, studentCaseRoles...)
	if err != nil {
		return nil, caseAuthError(err)
	}
	return tx, nil
}

func (s *Store) GetStudentCase(ctx context.Context, actor, caseID int64) (studentcases.Detail, error) {
	tx, err := s.caseReader(ctx, actor)
	if err != nil {
		return studentcases.Detail{}, err
	}
	defer tx.Rollback()
	var item studentcases.Case
	err = tx.QueryRowContext(ctx, studentCaseSelect+` WHERE c.id=$2 AND `+caseReadablePredicate, actor, caseID).Scan(caseDestinations(&item)...)
	if errors.Is(err, sql.ErrNoRows) {
		return studentcases.Detail{}, studentcases.ErrNotFound
	}
	if err != nil {
		return studentcases.Detail{}, err
	}
	detail := studentcases.Detail{Case: item}
	ownerRole, err := hasActiveCaseRole(ctx, tx, actor, item.Stage.Role())
	if err != nil {
		return studentcases.Detail{}, err
	}
	detail.CanWrite = item.ResponsibleID == actor && item.Stage.Role() != "" && ownerRole
	if item.Stage == studentcases.StageHomeroom {
		currentWaliID, scopeErr := currentStudentWaliUser(ctx, tx, item.StudentID)
		currentWali := scopeErr == nil && currentWaliID == actor
		detail.CanWrite = detail.CanWrite && currentWali
		detail.CanTakeOver = item.Status == studentcases.StatusOpen && ownerRole && currentWali && item.ResponsibleID != actor
	}
	detail.CanReferToBK = detail.CanWrite && item.Status == studentcases.StatusOpen && item.Stage == studentcases.StageHomeroom
	detail.CanReferToKes = detail.CanWrite && item.Status == studentcases.StatusOpen && item.Stage == studentcases.StageCounselor
	detail.CanReadCounseling = detail.CanWrite && item.Stage == studentcases.StageCounselor
	if err = loadStudentCaseEntries(ctx, tx, item.ID, &detail); err != nil {
		return studentcases.Detail{}, err
	}
	if err = loadStudentCaseHistory(ctx, tx, item.ID, &detail); err != nil {
		return studentcases.Detail{}, err
	}
	if detail.CanReadCounseling {
		if err = loadStudentCaseCounselingNotes(ctx, tx, item.ID, &detail); err != nil {
			return studentcases.Detail{}, err
		}
	}
	return detail, nil
}

// currentStudentWaliUser hanya mengembalikan Wali dari satu penempatan aktif
// yang kelas dan penugasan Walinya dapat diverifikasi secara tunggal.
func currentStudentWaliUser(ctx context.Context, tx *sql.Tx, studentID int64) (int64, error) {
	var placements int
	if err := tx.QueryRowContext(ctx, `SELECT count(*)
		FROM app.student_placements p JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
		WHERE p.student_id=$1 AND p.starts_on IS NOT NULL AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
		AND app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL`, studentID).Scan(&placements); err != nil {
		return 0, err
	}
	if placements != 1 {
		return 0, studentcases.ErrScopeUnclear
	}
	var count int
	var userID sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT count(DISTINCT u.id),min(u.id)
		FROM app.student_placements p
		JOIN app.school_years sy ON sy.id=p.school_year_id AND sy.active
		JOIN app.classes c ON c.id=app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) AND c.school_year_id=sy.id
		JOIN app.homeroom_assignments h ON h.class_id=c.id
		JOIN app.teachers t ON t.id=h.teacher_id AND t.active
		JOIN app.users u ON u.id=t.user_id AND u.active
		JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='wali_kelas'
		WHERE p.student_id=$1 AND p.starts_on IS NOT NULL AND p.starts_on<=(now() AT TIME ZONE 'Asia/Jakarta')::date
		AND app.student_placement_class_at(p.id,(now() AT TIME ZONE 'Asia/Jakarta')::date) IS NOT NULL`, studentID).Scan(&count, &userID)
	if err != nil {
		return 0, err
	}
	if count != 1 || !userID.Valid {
		return 0, studentcases.ErrScopeUnclear
	}
	return userID.Int64, nil
}

// TakeOverStudentCase mengalihkan kasus tahap Wali secara eksplisit dan atomik.
func (s *Store) TakeOverStudentCase(ctx context.Context, actor, caseID, expectedVersion int64, input studentcases.HandoffInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	tx, err := s.begin(ctx, actor, studentcases.RoleHomeroom)
	if err != nil {
		return caseAuthError(err)
	}
	defer tx.Rollback()
	if caseID < 1 {
		return studentcases.ErrNotFound
	}
	var item studentcases.Case
	err = tx.QueryRowContext(ctx, studentCaseSelect+` WHERE c.id=$1 FOR UPDATE OF c`, caseID).Scan(caseDestinations(&item)...)
	if errors.Is(err, sql.ErrNoRows) {
		return studentcases.ErrNotFound
	}
	if err != nil {
		return err
	}
	if item.Version != expectedVersion {
		return studentcases.ErrConflict
	}
	if item.Stage != studentcases.StageHomeroom || item.Status != studentcases.StatusOpen || item.ResponsibleID == actor {
		return studentcases.ErrForbidden
	}
	currentWaliID, err := currentStudentWaliUser(ctx, tx, item.StudentID)
	if err != nil {
		return err
	}
	if currentWaliID != actor {
		return studentcases.ErrForbidden
	}
	if currentWaliID == item.ResponsibleID {
		return studentcases.ErrForbidden
	}
	result, err := tx.ExecContext(ctx, `UPDATE app.student_cases SET responsible_user_id=$1,version=version+1,updated_at=clock_timestamp() WHERE id=$2 AND version=$3 AND stage='wali_kelas' AND status='perlu_tindak_lanjut'`, actor, caseID, expectedVersion)
	if err != nil {
		return dbError(err)
	}
	if n, rowsErr := result.RowsAffected(); rowsErr != nil {
		return rowsErr
	} else if n != 1 {
		return studentcases.ErrConflict
	}
	if err = insertStudentCaseHistory(ctx, tx, caseID, actor, studentcases.EventHomeroomHandoff, item.Stage, item.Stage, item.ResponsibleID, actor, input.Reason); err != nil {
		return err
	}
	return tx.Commit()
}

func hasActiveCaseRole(ctx context.Context, tx *sql.Tx, actor int64, role string) (bool, error) {
	var ok bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.users u JOIN app.user_roles r ON r.user_id=u.id WHERE u.id=$1 AND u.active AND r.role_code=$2)`, actor, role).Scan(&ok)
	return ok, err
}

func loadStudentCaseEntries(ctx context.Context, tx *sql.Tx, caseID int64, detail *studentcases.Detail) error {
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.case_id,e.actor_id,u.display_name,e.entry_type,e.content,e.created_at FROM app.student_case_entries e JOIN app.users u ON u.id=e.actor_id WHERE e.case_id=$1 ORDER BY e.id`, caseID)
	if err != nil {
		return err
	}
	defer rows.Close()
	detail.Entries = []studentcases.Entry{}
	for rows.Next() {
		var item studentcases.Entry
		if err = rows.Scan(&item.ID, &item.CaseID, &item.ActorID, &item.ActorName, &item.Type, &item.Content, &item.CreatedAt); err != nil {
			return err
		}
		detail.Entries = append(detail.Entries, item)
	}
	return rows.Err()
}

func loadStudentCaseHistory(ctx context.Context, tx *sql.Tx, caseID int64, detail *studentcases.Detail) error {
	rows, err := tx.QueryContext(ctx, `SELECT h.id,h.case_id,h.actor_id,u.display_name,h.event_type,COALESCE(h.from_stage,''),COALESCE(h.to_stage,''),COALESCE(h.from_responsible_user_id,0),COALESCE(f.display_name,''),COALESCE(h.to_responsible_user_id,0),COALESCE(t.display_name,''),h.reason,h.created_at FROM app.student_case_history h JOIN app.users u ON u.id=h.actor_id LEFT JOIN app.users f ON f.id=h.from_responsible_user_id LEFT JOIN app.users t ON t.id=h.to_responsible_user_id WHERE h.case_id=$1 ORDER BY h.id DESC`, caseID)
	if err != nil {
		return err
	}
	defer rows.Close()
	detail.History = []studentcases.History{}
	for rows.Next() {
		var item studentcases.History
		if err = rows.Scan(&item.ID, &item.CaseID, &item.ActorID, &item.ActorName, &item.Event, &item.FromStage, &item.ToStage, &item.FromResponsible, &item.FromResponsibleName, &item.ToResponsible, &item.ToResponsibleName, &item.Reason, &item.CreatedAt); err != nil {
			return err
		}
		detail.History = append(detail.History, item)
	}
	return rows.Err()
}

func loadStudentCaseCounselingNotes(ctx context.Context, tx *sql.Tx, caseID int64, detail *studentcases.Detail) error {
	rows, err := tx.QueryContext(ctx, `SELECT n.id,n.case_id,n.actor_id,u.display_name,n.content,n.created_at FROM app.student_case_counseling_notes n JOIN app.users u ON u.id=n.actor_id WHERE n.case_id=$1 ORDER BY n.id DESC`, caseID)
	if err != nil {
		return err
	}
	defer rows.Close()
	detail.CounselingNotes = []studentcases.CounselingNote{}
	for rows.Next() {
		var item studentcases.CounselingNote
		if err = rows.Scan(&item.ID, &item.CaseID, &item.ActorID, &item.ActorName, &item.Content, &item.CreatedAt); err != nil {
			return err
		}
		detail.CounselingNotes = append(detail.CounselingNotes, item)
	}
	return rows.Err()
}

func (s *Store) AddStudentCaseEntry(ctx context.Context, actor, caseID, expectedVersion int64, input studentcases.EntryInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	tx, err := s.beginAny(ctx, actor, studentCaseRoles...)
	if err != nil {
		return caseAuthError(err)
	}
	defer tx.Rollback()
	item, err := studentCaseForMutation(ctx, tx, actor, caseID)
	if err != nil {
		return err
	}
	if item.Version != expectedVersion {
		return studentcases.ErrConflict
	}
	if item.Status != studentcases.StatusOpen {
		return studentcases.ErrInvalid
	}
	entryType := caseEntryType(item.Stage)
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_case_entries(case_id,actor_id,entry_type,content) VALUES($1,$2,$3,$4)`, caseID, actor, string(entryType), input.Content); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.student_cases SET version=version+1,updated_at=clock_timestamp() WHERE id=$1`, caseID); err != nil {
		return err
	}
	if err = insertStudentCaseHistory(ctx, tx, caseID, actor, studentcases.EventEntryAdded, item.Stage, item.Stage, actor, actor, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func caseEntryType(stage studentcases.Stage) studentcases.EntryType {
	switch stage {
	case studentcases.StageHomeroom:
		return studentcases.EntryFollowUp
	case studentcases.StageCounselor:
		return studentcases.EntryBKSummary
	case studentcases.StageDiscipline:
		return studentcases.EntryDiscipline
	default:
		return ""
	}
}

func (s *Store) AddStudentCaseCounselingNote(ctx context.Context, actor, caseID, expectedVersion int64, input studentcases.CounselingInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	tx, err := s.begin(ctx, actor, studentcases.RoleCounselor)
	if err != nil {
		return caseAuthError(err)
	}
	defer tx.Rollback()
	item, err := studentCaseForMutation(ctx, tx, actor, caseID)
	if err != nil {
		return err
	}
	if item.Stage != studentcases.StageCounselor || item.Status != studentcases.StatusOpen {
		return studentcases.ErrForbidden
	}
	if item.Version != expectedVersion {
		return studentcases.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_case_counseling_notes(case_id,actor_id,content) VALUES($1,$2,$3)`, caseID, actor, input.Content); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.student_cases SET version=version+1,updated_at=clock_timestamp() WHERE id=$1`, caseID); err != nil {
		return err
	}
	if err = insertStudentCaseHistory(ctx, tx, caseID, actor, studentcases.EventCounselingNoteAdded, item.Stage, item.Stage, actor, actor, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func studentCaseForMutation(ctx context.Context, tx *sql.Tx, actor, caseID int64) (studentcases.Case, error) {
	if caseID < 1 {
		return studentcases.Case{}, studentcases.ErrNotFound
	}
	var item studentcases.Case
	err := tx.QueryRowContext(ctx, studentCaseSelect+` WHERE c.id=$1 FOR UPDATE OF c`, caseID).Scan(caseDestinations(&item)...)
	if errors.Is(err, sql.ErrNoRows) {
		return studentcases.Case{}, studentcases.ErrNotFound
	}
	if err != nil {
		return studentcases.Case{}, err
	}
	ownerRole, roleErr := hasActiveCaseRole(ctx, tx, actor, item.Stage.Role())
	if roleErr != nil {
		return studentcases.Case{}, roleErr
	}
	if item.ResponsibleID != actor || !ownerRole {
		return studentcases.Case{}, studentcases.ErrNotFound
	}
	if item.Stage == studentcases.StageHomeroom {
		var currentWali bool
		if err = tx.QueryRowContext(ctx, `SELECT `+currentCaseWaliScope+` AND EXISTS(SELECT 1 FROM app.user_roles WHERE user_id=$1 AND role_code='wali_kelas') FROM app.student_cases c WHERE c.id=$2`, actor, item.ID).Scan(&currentWali); err != nil {
			return studentcases.Case{}, err
		}
		if !currentWali {
			return studentcases.Case{}, studentcases.ErrNotFound
		}
	}
	return item, nil
}

func (s *Store) ReferStudentCaseToBK(ctx context.Context, actor, caseID, expectedVersion int64, input studentcases.ReferralInput) error {
	return s.referStudentCase(ctx, actor, caseID, expectedVersion, input, studentcases.StageHomeroom, studentcases.StageCounselor, studentcases.RoleHomeroom, studentcases.RoleCounselor, studentcases.EventReferredToBK)
}

func (s *Store) ReferStudentCaseToKesiswaan(ctx context.Context, actor, caseID, expectedVersion int64, input studentcases.ReferralInput) error {
	return s.referStudentCase(ctx, actor, caseID, expectedVersion, input, studentcases.StageCounselor, studentcases.StageDiscipline, studentcases.RoleCounselor, studentcases.RoleStudentAffairs, studentcases.EventReferredToStudentAffairs)
}

func (s *Store) referStudentCase(ctx context.Context, actor, caseID, expectedVersion int64, input studentcases.ReferralInput, from, to studentcases.Stage, actorRole, targetRole string, event studentcases.EventType) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if err := studentcases.ValidateReferral(from, to); err != nil {
		return err
	}
	tx, err := s.begin(ctx, actor, actorRole)
	if err != nil {
		return caseAuthError(err)
	}
	defer tx.Rollback()
	item, err := studentCaseForMutation(ctx, tx, actor, caseID)
	if err != nil {
		return err
	}
	if item.Stage != from || item.Status != studentcases.StatusOpen {
		return studentcases.ErrForbidden
	}
	if item.Version != expectedVersion {
		return studentcases.ErrConflict
	}
	var targetValid bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.users u JOIN app.user_roles r ON r.user_id=u.id WHERE u.id=$1 AND u.active AND r.role_code=$2)`, input.TargetUserID, targetRole).Scan(&targetValid); err != nil {
		return err
	}
	if !targetValid {
		return studentcases.ErrInvalid
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.student_cases SET stage=$1,responsible_user_id=$2,version=version+1,updated_at=clock_timestamp() WHERE id=$3 AND version=$4`, string(to), input.TargetUserID, caseID, expectedVersion); err != nil {
		return err
	}
	entryType := studentcases.EntryFollowUp
	if from == studentcases.StageCounselor {
		entryType = studentcases.EntryBKSummary
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_case_entries(case_id,actor_id,entry_type,content) VALUES($1,$2,$3,$4)`, caseID, actor, string(entryType), input.Reason); err != nil {
		return err
	}
	if err = insertStudentCaseHistory(ctx, tx, caseID, actor, event, from, to, actor, input.TargetUserID, input.Reason); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ChangeStudentCaseStatus(ctx context.Context, actor, caseID, expectedVersion int64, input studentcases.StatusInput) error {
	tx, err := s.beginAny(ctx, actor, studentCaseRoles...)
	if err != nil {
		return caseAuthError(err)
	}
	defer tx.Rollback()
	item, err := studentCaseForMutation(ctx, tx, actor, caseID)
	if err != nil {
		return err
	}
	if err = input.Validate(item.Status); err != nil {
		return err
	}
	if item.Version != expectedVersion {
		return studentcases.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.student_cases SET status=$1,version=version+1,updated_at=clock_timestamp() WHERE id=$2 AND version=$3`, string(input.Status), caseID, expectedVersion); err != nil {
		return err
	}
	event := studentcases.EventReopened
	if input.Status == studentcases.StatusCompleted {
		event = studentcases.EventCompleted
	}
	if err = insertStudentCaseHistory(ctx, tx, caseID, actor, event, item.Stage, item.Stage, actor, actor, input.Reason); err != nil {
		return err
	}
	return tx.Commit()
}

func insertStudentCaseHistory(ctx context.Context, tx *sql.Tx, caseID, actor int64, event studentcases.EventType, from, to studentcases.Stage, fromOwner, toOwner int64, reason string) error {
	var fromStage, toStage any
	if from != "" {
		fromStage = string(from)
	}
	if to != "" {
		toStage = string(to)
	}
	var fromUser, toUser any
	if fromOwner > 0 {
		fromUser = fromOwner
	}
	if toOwner > 0 {
		toUser = toOwner
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO app.student_case_history(case_id,actor_id,event_type,from_stage,to_stage,from_responsible_user_id,to_responsible_user_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, caseID, actor, string(event), fromStage, toStage, fromUser, toUser, reason)
	return err
}

func caseAuthError(err error) error {
	if errors.Is(err, ErrForbidden) {
		return studentcases.ErrForbidden
	}
	return err
}
