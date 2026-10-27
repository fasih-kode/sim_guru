package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/jackc/pgx/v5/pgconn"
)

type Store struct {
	DB     *sql.DB
	Photos TeacherAttendancePhotoStorage
}

type rowScanner interface{ Scan(...any) error }

func (s *Store) authorize(ctx context.Context, tx *sql.Tx, actor int64, roles ...string) error {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.users u JOIN app.user_roles r ON r.user_id=u.id WHERE u.id=$1 AND u.active AND r.role_code=ANY($2::text[]))`, actor, roles).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *Store) begin(ctx context.Context, actor int64, role string) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err = s.authorize(ctx, tx, actor, role); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (s *Store) reader(ctx context.Context, actor int64, roles ...string) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	if err = s.authorize(ctx, tx, actor, roles...); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func scanYear(row rowScanner) (Year, error) {
	var y Year
	err := row.Scan(&y.ID, &y.Name, &y.Active, &y.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return y, ErrNotFound
	}
	return y, err
}

func (s *Store) Years(ctx context.Context, actor int64) ([]Year, error) {
	// Admin needs the year catalog to select a class's year, without calendar write access.
	tx, err := s.reader(ctx, actor, "admin", "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,name,active,version FROM app.school_years ORDER BY name DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var years []Year
	for rows.Next() {
		y, err := scanYear(rows)
		if err != nil {
			return nil, err
		}
		years = append(years, y)
	}
	return years, rows.Err()
}

func (s *Store) CreateYear(ctx context.Context, actor int64, name string) (Year, error) {
	name, err := validateYear(name)
	if err != nil {
		return Year{}, err
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return Year{}, err
	}
	defer tx.Rollback()
	y, err := scanYear(tx.QueryRowContext(ctx, `INSERT INTO app.school_years(name) VALUES($1) RETURNING id,name,active,version`, name))
	if err != nil {
		return Year{}, dbError(err)
	}
	if err = record(ctx, tx, actor, "school_year", y.ID, nil, y, "Pembuatan tahun ajaran"); err != nil {
		return Year{}, err
	}
	for number := 1; number <= 2; number++ {
		term, err := scanSemester(tx.QueryRowContext(ctx, `INSERT INTO app.semesters(school_year_id,number) VALUES($1,$2) RETURNING id,school_year_id,$3::text,number,active,version,false`, y.ID, number, y.Name))
		if err != nil {
			return Year{}, err
		}
		if err = record(ctx, tx, actor, "semester", term.ID, nil, term, "Semester dibuat otomatis bersama tahun ajaran"); err != nil {
			return Year{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Year{}, dbError(err)
	}
	return y, nil
}

func (s *Store) ActivateYear(ctx context.Context, actor, id int64) error {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	target, err := scanYear(tx.QueryRowContext(ctx, `SELECT id,name,active,version FROM app.school_years WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if target.Active {
		return tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,name,active,version FROM app.school_years WHERE active FOR UPDATE`)
	if err != nil {
		return err
	}
	var previous []Year
	for rows.Next() {
		v, scanErr := scanYear(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		previous = append(previous, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	terms, err := tx.QueryContext(ctx, `SELECT t.id,t.school_year_id,y.name,t.number,t.active,t.version,y.active FROM app.semesters t JOIN app.school_years y ON y.id=t.school_year_id WHERE t.active AND t.school_year_id<>$1 FOR UPDATE OF t`, id)
	if err != nil {
		return err
	}
	var previousTerms []Semester
	for terms.Next() {
		v, scanErr := scanSemester(terms)
		if scanErr != nil {
			terms.Close()
			return scanErr
		}
		previousTerms = append(previousTerms, v)
	}
	if err = terms.Err(); err != nil {
		terms.Close()
		return err
	}
	terms.Close()
	for _, old := range previous {
		updated, err := scanYear(tx.QueryRowContext(ctx, `UPDATE app.school_years SET active=false,version=version+1,updated_at=now() WHERE id=$1 RETURNING id,name,active,version`, old.ID))
		if err != nil {
			return err
		}
		if err = record(ctx, tx, actor, "school_year", old.ID, old, updated, "Mengaktifkan tahun ajaran lain"); err != nil {
			return err
		}
	}
	for _, old := range previousTerms {
		updated, err := scanSemester(tx.QueryRowContext(ctx, `UPDATE app.semesters SET active=false,version=version+1,updated_at=now() WHERE id=$1 RETURNING id,school_year_id,$2::text,number,active,version,false`, old.ID, old.YearName))
		if err != nil {
			return err
		}
		if err = record(ctx, tx, actor, "semester", old.ID, old, updated, "Tahun ajaran induk dinonaktifkan"); err != nil {
			return err
		}
	}
	target, err = scanYear(tx.QueryRowContext(ctx, `UPDATE app.school_years SET active=true,version=version+1,updated_at=now() WHERE id=$1 RETURNING id,name,active,version`, id))
	if err != nil {
		return err
	}
	if err = record(ctx, tx, actor, "school_year", target.ID, Year{ID: target.ID, Name: target.Name, Active: false, Version: target.Version - 1}, target, "Mengaktifkan tahun ajaran"); err != nil {
		return err
	}
	return tx.Commit()
}

func scanSemester(row rowScanner) (Semester, error) {
	var term Semester
	err := row.Scan(&term.ID, &term.SchoolYearID, &term.YearName, &term.Number, &term.Active, &term.Version, &term.YearActive)
	if errors.Is(err, sql.ErrNoRows) {
		return term, ErrNotFound
	}
	return term, err
}

func (s *Store) Semesters(ctx context.Context, actor int64) ([]Semester, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT t.id,t.school_year_id,y.name,t.number,t.active,t.version,y.active FROM app.semesters t JOIN app.school_years y ON y.id=t.school_year_id ORDER BY y.name DESC,t.number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var semesters []Semester
	for rows.Next() {
		term, err := scanSemester(rows)
		if err != nil {
			return nil, err
		}
		semesters = append(semesters, term)
	}
	return semesters, rows.Err()
}

func (s *Store) ActivateSemester(ctx context.Context, actor, id int64) error {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	target, err := scanSemester(tx.QueryRowContext(ctx, `SELECT t.id,t.school_year_id,y.name,t.number,t.active,t.version,y.active FROM app.semesters t JOIN app.school_years y ON y.id=t.school_year_id WHERE t.id=$1 FOR UPDATE OF t`, id))
	if err != nil {
		return err
	}
	if !target.YearActive {
		return ValidationError("Aktifkan tahun ajaran terlebih dahulu sebelum memilih semesternya.")
	}
	if target.Active {
		return tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT t.id,t.school_year_id,y.name,t.number,t.active,t.version,y.active FROM app.semesters t JOIN app.school_years y ON y.id=t.school_year_id WHERE t.active FOR UPDATE OF t`)
	if err != nil {
		return err
	}
	var previous []Semester
	for rows.Next() {
		v, scanErr := scanSemester(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		previous = append(previous, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, old := range previous {
		updated, err := scanSemester(tx.QueryRowContext(ctx, `UPDATE app.semesters SET active=false,version=version+1,updated_at=now() WHERE id=$1 RETURNING id,school_year_id,$2::text,number,active,version,$3::bool`, old.ID, old.YearName, old.YearActive))
		if err != nil {
			return err
		}
		if err = record(ctx, tx, actor, "semester", old.ID, old, updated, "Mengaktifkan semester lain"); err != nil {
			return err
		}
	}
	target, err = scanSemester(tx.QueryRowContext(ctx, `UPDATE app.semesters SET active=true,version=version+1,updated_at=now() WHERE id=$1 RETURNING id,school_year_id,$2::text,number,active,version,true`, id, target.YearName))
	if err != nil {
		return err
	}
	if err = record(ctx, tx, actor, "semester", target.ID, Semester{ID: target.ID, SchoolYearID: target.SchoolYearID, YearName: target.YearName, Number: target.Number, Active: false, Version: target.Version - 1}, target, "Mengaktifkan semester"); err != nil {
		return err
	}
	return tx.Commit()
}

func scanClass(row rowScanner) (Class, error) {
	var c Class
	err := row.Scan(&c.ID, &c.SchoolYearID, &c.YearName, &c.Grade, &c.Name, &c.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

const classSelect = `SELECT c.id,c.school_year_id,y.name,c.grade,c.name,c.version FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id`

func (s *Store) Classes(ctx context.Context, actor int64) ([]Class, []Year, error) {
	tx, err := s.reader(ctx, actor, "admin", "kurikulum")
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, classSelect+` ORDER BY y.name DESC,c.grade,c.name,c.id`)
	if err != nil {
		return nil, nil, err
	}
	var classes []Class
	for rows.Next() {
		c, err := scanClass(rows)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		classes = append(classes, c)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT id,name,active,version FROM app.school_years ORDER BY name DESC,id DESC`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var years []Year
	for rows.Next() {
		y, err := scanYear(rows)
		if err != nil {
			return nil, nil, err
		}
		years = append(years, y)
	}
	return classes, years, rows.Err()
}

func (s *Store) Class(ctx context.Context, actor, id int64) (Class, error) {
	tx, err := s.reader(ctx, actor, "admin", "kurikulum")
	if err != nil {
		return Class{}, err
	}
	defer tx.Rollback()
	return scanClass(tx.QueryRowContext(ctx, classSelect+` WHERE c.id=$1`, id))
}

func (s *Store) SaveClass(ctx context.Context, actor, id int64, schoolYearID int64, grade int, name, reason string, version int64) (Class, error) {
	name, reason, err := validateClass(schoolYearID, grade, name, reason, id == 0)
	if err != nil {
		return Class{}, err
	}
	tx, err := s.begin(ctx, actor, "admin")
	if err != nil {
		return Class{}, err
	}
	defer tx.Rollback()
	var c Class
	if id == 0 {
		var yearName string
		if err = tx.QueryRowContext(ctx, `SELECT name FROM app.school_years WHERE id=$1`, schoolYearID).Scan(&yearName); errors.Is(err, sql.ErrNoRows) {
			return Class{}, ErrNotFound
		} else if err != nil {
			return Class{}, err
		}
		c, err = scanClass(tx.QueryRowContext(ctx, `INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,$2,$3) RETURNING id,school_year_id,$4::text,grade,name,version`, schoolYearID, grade, name, yearName))
		if err != nil {
			return Class{}, dbError(err)
		}
		if err = record(ctx, tx, actor, "class", c.ID, nil, c, "Pembuatan kelas"); err != nil {
			return Class{}, err
		}
	} else {
		before, err := scanClass(tx.QueryRowContext(ctx, classSelect+` WHERE c.id=$1 FOR UPDATE OF c`, id))
		if err != nil {
			return Class{}, err
		}
		if before.Version != version || before.SchoolYearID != schoolYearID {
			return Class{}, ErrConflict
		}
		c, err = scanClass(tx.QueryRowContext(ctx, `UPDATE app.classes SET grade=$1,name=$2,version=version+1,updated_at=now() WHERE id=$3 AND version=$4 RETURNING id,school_year_id,$5::text,grade,name,version`, grade, name, id, version, before.YearName))
		if err != nil {
			return Class{}, dbError(err)
		}
		if err = record(ctx, tx, actor, "class", id, before, c, reason); err != nil {
			return Class{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Class{}, dbError(err)
	}
	return c, nil
}

func (s *Store) History(ctx context.Context, actor int64, entityType string, id int64) ([]History, error) {
	if !validEntity(entityType) || id < 1 {
		return nil, ErrNotFound
	}
	roles := []string{"kurikulum"}
	if entityType == "class" {
		roles = append(roles, "admin")
	}
	tx, err := s.reader(ctx, actor, roles...)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT h.id,h.entity_type,u.display_name,COALESCE(h.before_state::text,''),h.after_state::text,h.reason,h.created_at FROM app.academic_history h JOIN app.users u ON u.id=h.actor_id WHERE h.entity_type=$1 AND h.entity_id=$2 ORDER BY h.id DESC LIMIT 100`, entityType, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []History
	for rows.Next() {
		var h History
		if err = rows.Scan(&h.ID, &h.EntityType, &h.Actor, &h.Before, &h.After, &h.Reason, &h.At); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

func (s *Store) UnplacedStudents(ctx context.Context, actor int64, yearID int64, search string, page int) ([]StudentOption, error) {
	if yearID < 1 || page < 1 || page > 10000 || len(search) > 480 {
		return nil, ValidationError("Pencarian, tahun ajaran, atau halaman tidak valid.")
	}
	tx, err := s.reader(ctx, actor, "admin")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.name,COALESCE(s.nis,''),COALESCE(s.nisn,''),s.active FROM app.students s WHERE (s.name ILIKE $2 OR COALESCE(s.nis,'') ILIKE $2 OR COALESCE(s.nisn,'') ILIKE $2) AND NOT EXISTS(SELECT 1 FROM app.outstanding_student_school_exits e WHERE e.student_id=s.id) AND NOT EXISTS(SELECT 1 FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=s.id) ORDER BY s.name,s.id LIMIT 11 OFFSET $3`, yearID, "%"+search+"%", (page-1)*10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var students []StudentOption
	for rows.Next() {
		var student StudentOption
		if err = rows.Scan(&student.ID, &student.Name, &student.NIS, &student.NISN, &student.Active); err != nil {
			return nil, err
		}
		students = append(students, student)
	}
	return students, rows.Err()
}

func (s *Store) Placements(ctx context.Context, actor int64, yearID, classID int64, search string, page int) ([]Placement, error) {
	if yearID < 1 || classID < 0 || page < 1 || page > 10000 || len(search) > 480 {
		return nil, ValidationError("Pencarian, tahun ajaran, atau halaman tidak valid.")
	}
	tx, err := s.reader(ctx, actor, "admin", "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT p.id,p.student_id,p.school_year_id,p.class_id,s.name,COALESCE(s.nis,''),COALESCE(s.nisn,''),y.name,c.grade,c.name,u.display_name,p.created_at,coalesce(original.starts_on::text,'') FROM app.current_student_placements p JOIN app.student_placements original ON original.id=p.id JOIN app.students s ON s.id=p.student_id JOIN app.school_years y ON y.id=p.school_year_id JOIN app.classes c ON c.id=p.class_id JOIN app.users u ON u.id=p.actor_id WHERE p.school_year_id=$1 AND ($2=0 OR p.class_id=$2) AND (s.name ILIKE $3 OR COALESCE(s.nis,'') ILIKE $3 OR COALESCE(s.nisn,'') ILIKE $3) ORDER BY c.grade,c.name,s.name,p.id LIMIT 11 OFFSET $4`, yearID, classID, "%"+search+"%", (page-1)*10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var placements []Placement
	for rows.Next() {
		var placement Placement
		if err = rows.Scan(&placement.ID, &placement.StudentID, &placement.SchoolYearID, &placement.ClassID, &placement.StudentName, &placement.NIS, &placement.NISN, &placement.YearName, &placement.Grade, &placement.ClassName, &placement.Actor, &placement.At, &placement.StartsOn); err != nil {
			return nil, err
		}
		placements = append(placements, placement)
	}
	return placements, rows.Err()
}

func (s *Store) PlacementCounts(ctx context.Context, actor int64) (map[int64]int, error) {
	tx, err := s.reader(ctx, actor, "admin", "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT class_id,count(*) FROM app.current_student_placements GROUP BY class_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int64]int)
	for rows.Next() {
		var item ClassPlacementCount
		if err = rows.Scan(&item.ClassID, &item.Count); err != nil {
			return nil, err
		}
		counts[item.ClassID] = item.Count
	}
	return counts, rows.Err()
}

// CreatePlacement accepts an explicit participation date for new UI writes.
// Omitted dates retain compatibility for pre-foundation service callers; such
// placements require Admin review before use in attendance contexts.
func (s *Store) CreatePlacement(ctx context.Context, actor, studentID, classID int64, startsOn ...string) (int64, error) {
	if studentID < 1 || classID < 1 {
		return 0, ValidationError("Pilih siswa dan kelas yang valid.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return 0, err
	}
	if err = s.authorize(ctx, tx, actor, "admin"); err != nil {
		return 0, err
	}
	var yearID int64
	err = tx.QueryRowContext(ctx, `SELECT school_year_id FROM app.classes WHERE id=$1 FOR KEY SHARE`, classID).Scan(&yearID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM app.students WHERE id=$1 FOR KEY SHARE`, studentID).Scan(&studentID); errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	var departed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.outstanding_student_school_exits WHERE student_id=$1)`, studentID).Scan(&departed); err != nil {
		return 0, err
	}
	if departed {
		return 0, ValidationError("Siswa sudah memiliki catatan pindah sekolah.")
	}
	var start any
	if len(startsOn) > 0 {
		if err = validatePlacementStart(ctx, tx, 0, yearID, startsOn[0]); err != nil {
			return 0, err
		}
		start = startsOn[0]
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5) RETURNING id`, yearID, studentID, classID, actor, start).Scan(&id)
	if err != nil {
		return 0, dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_placement_history(placement_id,actor_id,to_class_id,reason) VALUES($1,$2,$3,'Penempatan awal oleh Admin')`, id, actor, classID); err != nil {
		return 0, err
	}
	if start != nil {
		if err = foundationHistory(ctx, tx, actor, "placement_date", id, nil, map[string]any{"StartsOn": start, "Version": 1}, "Tanggal mulai penempatan awal oleh Admin"); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, dbError(err)
	}
	return id, nil
}

func (s *Store) PlacementHistory(ctx context.Context, actor, placementID int64) ([]PlacementHistory, error) {
	if placementID < 1 {
		return nil, ErrNotFound
	}
	tx, err := s.reader(ctx, actor, "admin", "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT h.id,h.placement_id,s.name,COALESCE(from_c.grade::text||from_c.name,''),to_c.grade::text||to_c.name,u.display_name,h.reason,h.created_at FROM app.student_placement_history h JOIN app.student_placements p ON p.id=h.placement_id JOIN app.students s ON s.id=p.student_id JOIN app.classes to_c ON to_c.id=h.to_class_id LEFT JOIN app.classes from_c ON from_c.id=h.from_class_id JOIN app.users u ON u.id=h.actor_id WHERE h.placement_id=$1 ORDER BY h.id DESC LIMIT 100`, placementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []PlacementHistory
	for rows.Next() {
		var item PlacementHistory
		if err = rows.Scan(&item.ID, &item.PlacementID, &item.StudentName, &item.FromClass, &item.ToClass, &item.Actor, &item.Reason, &item.At); err != nil {
			return nil, err
		}
		history = append(history, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(history) == 0 {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_placements WHERE id=$1)`, placementID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return history, nil
}

// HomeroomTeachers returns only active teacher identities linked to an active account
// that currently has the wali_kelas role.
func (s *Store) HomeroomTeachers(ctx context.Context, actor int64) ([]HomeroomTeacher, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT t.id,t.name FROM app.teachers t JOIN app.users u ON u.id=t.user_id JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='wali_kelas' WHERE t.active AND u.active ORDER BY t.name,t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HomeroomTeacher
	for rows.Next() {
		var x HomeroomTeacher
		if err = rows.Scan(&x.ID, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) HomeroomAssignments(ctx context.Context, actor int64) ([]HomeroomAssignment, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT c.id,a.teacher_id,y.name,c.grade::text||c.name,t.name,a.updated_at FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id LEFT JOIN app.homeroom_assignments a ON a.class_id=c.id LEFT JOIN app.teachers t ON t.id=a.teacher_id ORDER BY y.name DESC,c.grade,c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HomeroomAssignment
	for rows.Next() {
		var x HomeroomAssignment
		var teacher sql.NullString
		var teacherID sql.NullInt64
		var at sql.NullTime
		if err = rows.Scan(&x.ClassID, &teacherID, &x.YearName, &x.ClassName, &teacher, &at); err != nil {
			return nil, err
		}
		x.TeacherID = teacherID.Int64
		x.TeacherName = teacher.String
		x.At = at.Time
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) AssignHomeroom(ctx context.Context, actor, classID, teacherID int64) error {
	if classID < 1 || teacherID < 1 {
		return ValidationError("Pilih kelas dan guru wali kelas yang valid.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var classExists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.classes WHERE id=$1)`, classID).Scan(&classExists); err != nil {
		return err
	}
	if !classExists {
		return ErrNotFound
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teachers t JOIN app.users u ON u.id=t.user_id JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='wali_kelas' WHERE t.id=$1 AND t.active AND u.active)`, teacherID).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ValidationError("Guru harus aktif, terhubung ke akun aktif, dan memiliki role Wali Kelas.")
	}
	var assignmentID int64
	var beforeID sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT id,teacher_id FROM app.homeroom_assignments WHERE class_id=$1 FOR UPDATE`, classID).Scan(&assignmentID, &beforeID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3) RETURNING id`, classID, teacherID, actor).Scan(&assignmentID)
	} else if err == nil && beforeID.Int64 == teacherID {
		return tx.Commit()
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE app.homeroom_assignments SET teacher_id=$1,assigned_by=$2,updated_at=now() WHERE id=$3`, teacherID, actor, assignmentID)
	}
	if err != nil {
		return dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.homeroom_assignment_history(assignment_id,class_id,before_teacher_id,after_teacher_id,actor_id) VALUES($1,$2,$3,$4,$5)`, assignmentID, classID, beforeID, teacherID, actor); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) HomeroomAssignmentHistory(ctx context.Context, actor, classID int64) ([]HomeroomHistory, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT COALESCE(before_t.name,''),after_t.name,u.display_name,h.created_at FROM app.homeroom_assignment_history h LEFT JOIN app.teachers before_t ON before_t.id=h.before_teacher_id JOIN app.teachers after_t ON after_t.id=h.after_teacher_id JOIN app.users u ON u.id=h.actor_id WHERE h.class_id=$1 ORDER BY h.id DESC LIMIT 100`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HomeroomHistory
	for rows.Next() {
		var x HomeroomHistory
		if err = rows.Scan(&x.BeforeTeacher, &x.AfterTeacher, &x.Actor, &x.At); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) MyHomerooms(ctx context.Context, actor int64) ([]HomeroomAssignment, error) {
	tx, err := s.reader(ctx, actor, "wali_kelas")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT c.id,a.teacher_id,y.name,c.grade::text||c.name,t.name,a.updated_at FROM app.homeroom_assignments a JOIN app.classes c ON c.id=a.class_id JOIN app.school_years y ON y.id=c.school_year_id JOIN app.teachers t ON t.id=a.teacher_id WHERE t.user_id=$1 AND t.active AND y.active ORDER BY y.name DESC,c.grade,c.name`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HomeroomAssignment
	for rows.Next() {
		var x HomeroomAssignment
		if err = rows.Scan(&x.ClassID, &x.TeacherID, &x.YearName, &x.ClassName, &x.TeacherName, &x.At); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) HomeroomRoster(ctx context.Context, actor, classID int64) ([]ClassStudent, error) {
	tx, err := s.reader(ctx, actor, "wali_kelas")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var assigned bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.homeroom_assignments a JOIN app.teachers t ON t.id=a.teacher_id JOIN app.classes c ON c.id=a.class_id JOIN app.school_years y ON y.id=c.school_year_id WHERE a.class_id=$1 AND t.user_id=$2 AND t.active AND y.active)`, classID, actor).Scan(&assigned); err != nil {
		return nil, err
	}
	if !assigned {
		return nil, ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.name,COALESCE(s.nis,''),COALESCE(s.nisn,'') FROM app.current_student_placements p JOIN app.students s ON s.id=p.student_id WHERE p.class_id=$1 ORDER BY s.name,s.id`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClassStudent
	for rows.Next() {
		var x ClassStudent
		if err = rows.Scan(&x.Name, &x.NIS, &x.NISN); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func validEntity(entity string) bool {
	return entity == "school_year" || entity == "semester" || entity == "class"
}

func record(ctx context.Context, tx *sql.Tx, actor int64, entityType string, entityID int64, before, after any, reason string) error {
	var rawBefore any
	if before != nil {
		b, err := json.Marshal(before)
		if err != nil {
			return err
		}
		rawBefore = string(b)
	}
	b, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.academic_history(entity_type,entity_id,actor_id,before_state,after_state,reason) VALUES($1,$2,$3,$4,$5,$6)`, entityType, entityID, actor, rawBefore, string(b), reason)
	return err
}

func dbError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23503") {
		return ErrConflict
	}
	return err
}
