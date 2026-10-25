package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Dates are civil dates in WIB, never inferred from record creation timestamps.
func civilDate(raw string) (time.Time, error) {
	d, e := time.Parse("2006-01-02", raw)
	if e != nil || d.Year() < 1900 || d.Year() > 2200 {
		return time.Time{}, ValidationError("Tanggal harus valid (1900–2200).")
	}
	return d, nil
}
func foundationReason(reason string) error {
	if strings.TrimSpace(reason) == "" || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 500 || strings.ContainsRune(reason, 0) {
		return ValidationError("Alasan wajib diisi, maksimal 500 karakter.")
	}
	return nil
}

type SemesterCalendar struct {
	SemesterID, Version int64
	StartsOn, EndsOn    string
	SchoolDays          []int
	Exceptions          []SchoolDateException
}
type SchoolDateException struct {
	Date, Label string
	SchoolDay   bool
}

func (c SemesterCalendar) HasDay(day int) bool {
	for _, d := range c.SchoolDays {
		if d == day {
			return true
		}
	}
	return false
}
func (c SemesterCalendar) IsSchoolDay(date string) (bool, error) {
	d, e := civilDate(date)
	if e != nil {
		return false, e
	}
	if c.Version == 0 {
		return false, ValidationError("Kalender semester belum diatur oleh Kurikulum.")
	}
	if date < c.StartsOn || date > c.EndsOn {
		return false, ValidationError("Tanggal di luar rentang semester.")
	}
	for _, x := range c.Exceptions {
		if x.Date == date {
			return x.SchoolDay, nil
		}
	}
	day := int(d.Weekday())
	if day == 0 {
		day = 7
	}
	return c.HasDay(day), nil
}
func validateCalendar(c SemesterCalendar) error {
	a, e := civilDate(c.StartsOn)
	if e != nil {
		return e
	}
	b, e := civilDate(c.EndsOn)
	if e != nil {
		return e
	}
	if c.SemesterID < 1 || b.Before(a) || b.Sub(a) > 366*24*time.Hour {
		return ValidationError("Rentang semester harus berurutan dan maksimal 366 hari.")
	}
	seen := map[int]bool{}
	for _, d := range c.SchoolDays {
		if d < 1 || d > 7 || seen[d] {
			return ValidationError("Pilih hari sekolah yang valid tanpa duplikat.")
		}
		seen[d] = true
	}
	if len(seen) == 0 {
		return ValidationError("Pilih sedikitnya satu hari sekolah.")
	}
	return nil
}
func querySemesterCalendar(ctx context.Context, tx *sql.Tx, id int64) (SemesterCalendar, error) {
	c := SemesterCalendar{SemesterID: id}
	var days string
	e := tx.QueryRowContext(ctx, `SELECT starts_on::text,ends_on::text,school_days::text,version FROM app.semester_calendars WHERE semester_id=$1`, id).Scan(&c.StartsOn, &c.EndsOn, &days, &c.Version)
	if errors.Is(e, sql.ErrNoRows) {
		var exists bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semesters WHERE id=$1)`, id).Scan(&exists)
		if e == nil && !exists {
			e = ErrNotFound
		}
		return c, e
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal([]byte(days), &c.SchoolDays); e != nil {
		return c, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT on_date::text,school_day,label FROM app.school_date_exceptions WHERE semester_id=$1 ORDER BY on_date`, id)
	if e != nil {
		return c, e
	}
	defer rows.Close()
	for rows.Next() {
		var v SchoolDateException
		if e = rows.Scan(&v.Date, &v.SchoolDay, &v.Label); e != nil {
			return c, e
		}
		c.Exceptions = append(c.Exceptions, v)
	}
	return c, rows.Err()
}
func foundationHistory(ctx context.Context, tx *sql.Tx, actor int64, kind string, id int64, before, after any, reason string) error {
	b, e := json.Marshal(before)
	if e != nil {
		return e
	}
	a, e := json.Marshal(after)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO app.attendance_foundation_history(entity_type,entity_id,actor_id,before_state,after_state,reason) VALUES($1,$2,$3,$4,$5,$6)`, kind, id, actor, b, a, strings.TrimSpace(reason))
	return e
}
func (s *Store) SemesterCalendar(ctx context.Context, actor, id int64) (SemesterCalendar, error) {
	tx, e := s.reader(ctx, actor, "kurikulum")
	if e != nil {
		return SemesterCalendar{}, e
	}
	defer tx.Rollback()
	return querySemesterCalendar(ctx, tx, id)
}
func (s *Store) SaveSemesterCalendar(ctx context.Context, actor int64, c SemesterCalendar, reason string) error {
	if e := validateCalendar(c); e != nil {
		return e
	}
	if e := foundationReason(reason); e != nil {
		return e
	}
	tx, e := s.begin(ctx, actor, "kurikulum")
	if e != nil {
		return e
	}
	defer tx.Rollback()
	old, e := querySemesterCalendar(ctx, tx, c.SemesterID)
	if e != nil {
		return e
	}
	if old.Version != c.Version {
		return ErrConflict
	}
	var conflict bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_calendars c JOIN app.semesters other ON other.id=c.semester_id JOIN app.semesters target ON target.school_year_id=other.school_year_id WHERE target.id=$1 AND other.id<>target.id AND (NOT(c.ends_on<$2::date OR c.starts_on>$3::date) OR (other.number<target.number AND c.ends_on>=$2::date) OR (other.number>target.number AND c.starts_on<=$3::date)))`, c.SemesterID, c.StartsOn, c.EndsOn).Scan(&conflict)
	if e != nil {
		return e
	}
	if conflict {
		return ValidationError("Rentang semester harus berurutan dan tidak bertumpang tindih dalam tahun yang sama.")
	}
	for _, x := range old.Exceptions {
		if x.Date < c.StartsOn || x.Date > c.EndsOn {
			return ValidationError("Rentang baru meninggalkan tanggal pengecualian di luar semester. Sesuaikan pengecualian dahulu.")
		}
	}
	sort.Ints(c.SchoolDays)
	days, e := json.Marshal(c.SchoolDays)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,$2,$3,$4) ON CONFLICT(semester_id) DO UPDATE SET starts_on=excluded.starts_on,ends_on=excluded.ends_on,school_days=excluded.school_days,version=app.semester_calendars.version+1`, c.SemesterID, c.StartsOn, c.EndsOn, days)
	if e != nil {
		return dbError(e)
	}
	c.Version++
	c.Exceptions = old.Exceptions
	if e = foundationHistory(ctx, tx, actor, "calendar", c.SemesterID, old, c, reason); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) SaveSchoolDateException(ctx context.Context, actor, semester, version int64, x SchoolDateException, reset bool, reason string) error {
	if _, e := civilDate(x.Date); e != nil {
		return e
	}
	if e := foundationReason(reason); e != nil {
		return e
	}
	if !reset && (strings.TrimSpace(x.Label) == "" || utf8.RuneCountInString(x.Label) > 200 || !utf8.ValidString(x.Label) || strings.ContainsRune(x.Label, 0)) {
		return ValidationError("Keterangan tanggal wajib diisi, maksimal 200 karakter.")
	}
	tx, e := s.begin(ctx, actor, "kurikulum")
	if e != nil {
		return e
	}
	defer tx.Rollback()
	old, e := querySemesterCalendar(ctx, tx, semester)
	if e != nil {
		return e
	}
	if old.Version == 0 {
		return ValidationError("Atur kalender semester terlebih dahulu.")
	}
	if old.Version != version {
		return ErrConflict
	}
	if x.Date < old.StartsOn || x.Date > old.EndsOn {
		return ValidationError("Tanggal pengecualian di luar semester.")
	}
	if reset {
		_, e = tx.ExecContext(ctx, `DELETE FROM app.school_date_exceptions WHERE semester_id=$1 AND on_date=$2`, semester, x.Date)
	} else {
		_, e = tx.ExecContext(ctx, `INSERT INTO app.school_date_exceptions(semester_id,on_date,school_day,label) VALUES($1,$2,$3,$4) ON CONFLICT(semester_id,on_date) DO UPDATE SET school_day=excluded.school_day,label=excluded.label`, semester, x.Date, x.SchoolDay, strings.TrimSpace(x.Label))
	}
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE app.semester_calendars SET version=version+1 WHERE semester_id=$1`, semester); e != nil {
		return e
	}
	now, e := querySemesterCalendar(ctx, tx, semester)
	if e != nil {
		return e
	}
	if e = foundationHistory(ctx, tx, actor, "calendar", semester, old, now, reason); e != nil {
		return e
	}
	return tx.Commit()
}

type PlacementDate struct {
	ID, YearID, ClassID, Version     int64
	StudentName, ClassName, StartsOn string
}

func (s *Store) PlacementDates(ctx context.Context, actor, year int64) ([]PlacementDate, error) {
	tx, e := s.reader(ctx, actor, "admin")
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT p.id,p.school_year_id,p.class_id,p.date_version,s.name,c.grade::text||c.name,coalesce(p.starts_on::text,'') FROM app.student_placements p JOIN app.students s ON s.id=p.student_id JOIN app.classes c ON c.id=p.class_id WHERE p.school_year_id=$1 ORDER BY c.grade,c.name,s.name,p.id`, year)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var result []PlacementDate
	for rows.Next() {
		var v PlacementDate
		if e = rows.Scan(&v.ID, &v.YearID, &v.ClassID, &v.Version, &v.StudentName, &v.ClassName, &v.StartsOn); e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func validatePlacementStart(ctx context.Context, tx *sql.Tx, placement, year int64, date string) error {
	if _, e := civilDate(date); e != nil {
		return e
	}
	// Initial participation cannot begin after a known move, exit, or return.
	var invalid bool
	e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_placements p WHERE p.id=$1 AND (EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND m.effective_date<$3::date) OR EXISTS(SELECT 1 FROM app.student_school_exits x WHERE x.student_id=p.student_id AND x.from_class_id IN(SELECT id FROM app.classes WHERE school_year_id=p.school_year_id) AND x.effective_date<$3::date) OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND (r.effective_date<$3::date OR (r.created_placement AND r.effective_date<>$3::date))))) OR EXISTS(SELECT 1 FROM app.semester_calendars c JOIN app.semesters sm ON sm.id=c.semester_id WHERE sm.school_year_id=$2 AND ((sm.number=1 AND $3::date<c.starts_on) OR (sm.number=2 AND $3::date>c.ends_on)))`, placement, year, date).Scan(&invalid)
	if e != nil {
		return e
	}
	if invalid {
		return ValidationError("Tanggal mulai di luar batas kalender tahun atau tidak sesuai kejadian perpindahan/penerimaan kembali yang sudah tercatat.")
	}
	return nil
}
func (s *Store) SavePlacementStart(ctx context.Context, actor, id, version int64, date, reason string) error {
	if e := foundationReason(reason); e != nil {
		return e
	}
	tx, e := s.begin(ctx, actor, "admin")
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var old PlacementDate
	e = tx.QueryRowContext(ctx, `SELECT id,school_year_id,date_version,coalesce(starts_on::text,'') FROM app.student_placements WHERE id=$1 FOR UPDATE`, id).Scan(&old.ID, &old.YearID, &old.Version, &old.StartsOn)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if old.Version != version {
		return ErrConflict
	}
	if e = validatePlacementStart(ctx, tx, id, old.YearID, date); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE app.student_placements SET starts_on=$2,date_version=date_version+1 WHERE id=$1`, id, date); e != nil {
		return e
	}
	now := old
	now.StartsOn = date
	now.Version++
	if e = foundationHistory(ctx, tx, actor, "placement_date", id, old, now, reason); e != nil {
		return e
	}
	return tx.Commit()
}

// AttendanceContext is a read-only foundation for the subsequent attendance module.
// Unknown initial dates block historical attendance; existing operational lists
// retain their compatibility behavior until Admin has reviewed those dates.
type AttendanceStudent struct {
	ID, PlacementID, PlacementVersion, StudentVersion int64
	Name, NIS, NISN, StartsOn                         string
}
type AttendanceContext struct {
	SemesterID, RevisionID int64
	Date                   string
	Fingerprint            string
	SchoolDay              bool
	Students               []AttendanceStudent
	Lessons                []TimetableEntry
}

func (s *Store) AttendanceContextOnDate(ctx context.Context, actor, class int64, date string) (AttendanceContext, error) {
	result := AttendanceContext{Date: date}
	if _, e := civilDate(date); e != nil {
		return result, e
	}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return result, e
	}
	defer tx.Rollback()
	if e = s.authorize(ctx, tx, actor, "petugas_absensi", "guru_mapel", "wali_kelas", "kesiswaan", "kepala_sekolah"); e != nil {
		return result, e
	}
	return s.attendanceContextOnDateTx(ctx, tx, actor, class, date, 0, true)
}

func (s *Store) attendanceContextOnDateTx(ctx context.Context, tx *sql.Tx, actor, class int64, date string, teacherOnly int64, includeRoster bool) (AttendanceContext, error) {
	result := AttendanceContext{Date: date}
	if _, e := civilDate(date); e != nil {
		return result, e
	}
	if teacherOnly == 0 {
		if e := s.authorize(ctx, tx, actor, "petugas_absensi", "guru_mapel", "wali_kelas", "kesiswaan", "kepala_sekolah"); e != nil {
			return result, e
		}
	} else {
		if e := s.authorize(ctx, tx, actor, "guru_mapel"); e != nil {
			return result, e
		}
	}
	var year int64
	e := tx.QueryRowContext(ctx, `SELECT school_year_id FROM app.classes WHERE id=$1`, class).Scan(&year)
	if errors.Is(e, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if e != nil {
		return result, e
	}
	var allowed bool
	if teacherOnly > 0 {
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_assignments a JOIN app.teachers t ON t.id=a.teacher_id WHERE a.class_id=$2 AND a.teacher_id=$4 AND t.user_id=$1 AND t.active AND a.semester_id IN(SELECT sc.semester_id FROM app.semester_calendars sc WHERE $3::date BETWEEN sc.starts_on AND sc.ends_on) AND a.effective_from<=$3::date AND (a.effective_until IS NULL OR a.effective_until>$3::date))`, actor, class, date, teacherOnly).Scan(&allowed)
	} else {
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$1 AND (r.role_code IN('petugas_absensi','kesiswaan','kepala_sekolah') OR (r.role_code='wali_kelas' AND EXISTS(SELECT 1 FROM app.homeroom_assignments h JOIN app.teachers t ON t.id=h.teacher_id WHERE h.class_id=$2 AND t.user_id=$1 AND t.active)) OR (r.role_code='guru_mapel' AND EXISTS(SELECT 1 FROM app.teaching_assignments a JOIN app.teachers t ON t.id=a.teacher_id WHERE a.class_id=$2 AND t.user_id=$1 AND t.active AND a.semester_id IN(SELECT sc.semester_id FROM app.semester_calendars sc WHERE $3::date BETWEEN sc.starts_on AND sc.ends_on) AND a.effective_from<=$3::date AND (a.effective_until IS NULL OR a.effective_until>$3::date)))))`, actor, class, date).Scan(&allowed)
	}
	if e != nil {
		return result, e
	}
	if !allowed {
		return result, ErrForbidden
	}
	e = tx.QueryRowContext(ctx, `SELECT sm.id FROM app.semesters sm JOIN app.semester_calendars c ON c.semester_id=sm.id WHERE sm.school_year_id=$1 AND $2::date BETWEEN c.starts_on AND c.ends_on`, year, date).Scan(&result.SemesterID)
	if errors.Is(e, sql.ErrNoRows) {
		return result, ValidationError("Tanggal belum tercakup kalender semester. Minta Kurikulum melengkapinya.")
	}
	if e != nil {
		return result, e
	}
	c, e := querySemesterCalendar(ctx, tx, result.SemesterID)
	if e != nil {
		return result, e
	}
	result.SchoolDay, e = c.IsSchoolDay(date)
	if e != nil {
		return result, e
	}
	if includeRoster {
		var unknown bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_placements p WHERE p.school_year_id=$1 AND p.starts_on IS NULL AND app.student_placement_class_at(p.id,$3::date)=$2)`, year, class, date).Scan(&unknown)
		if e != nil {
			return result, e
		}
		if unknown {
			return result, ValidationError("Tanggal mulai penempatan siswa belum lengkap. Minta Admin melengkapinya.")
		}
		rows, e := tx.QueryContext(ctx, `SELECT s.id,p.id,p.date_version,s.version,p.starts_on::text,s.name,coalesce(s.nis,''),coalesce(s.nisn,'') FROM app.student_placements p JOIN app.students s ON s.id=p.student_id WHERE p.school_year_id=$1 AND p.starts_on<=$3::date AND app.student_placement_class_at(p.id,$3::date)=$2 ORDER BY s.name,s.id`, year, class, date)
		if e != nil {
			return result, e
		}
		for rows.Next() {
			var v AttendanceStudent
			if e = rows.Scan(&v.ID, &v.PlacementID, &v.PlacementVersion, &v.StudentVersion, &v.StartsOn, &v.Name, &v.NIS, &v.NISN); e != nil {
				rows.Close()
				return result, e
			}
			result.Students = append(result.Students, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return result, e
		}
	}
	e = tx.QueryRowContext(ctx, `SELECT id FROM app.weekly_timetable_revisions WHERE semester_id=$1 AND status='published' AND effective_from<=$2::date ORDER BY effective_from DESC,revision_no DESC LIMIT 1`, result.SemesterID, date).Scan(&result.RevisionID)
	if errors.Is(e, sql.ErrNoRows) {
		return result, nil
	}
	if e != nil {
		return result, e
	}
	if !result.SchoolDay {
		return result, nil
	}
	var broad bool
	if teacherOnly == 0 {
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$1 AND (r.role_code IN('petugas_absensi','kesiswaan','kepala_sekolah') OR (r.role_code='wali_kelas' AND EXISTS(SELECT 1 FROM app.homeroom_assignments h JOIN app.teachers t ON t.id=h.teacher_id WHERE h.class_id=$2 AND t.user_id=$1 AND t.active))))`, actor, class).Scan(&broad)
	}
	if e != nil {
		return result, e
	}
	entries, e := queryTimetableEntries(ctx, tx, result.RevisionID, 0)
	if e != nil {
		return result, e
	}
	d, _ := civilDate(date)
	day := int(d.Weekday())
	if day == 0 {
		day = 7
	}
	for _, v := range entries {
		if v.ClassID == class && v.Weekday == day {
			if !broad {
				var own bool
				if teacherOnly > 0 {
					e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_assignments a JOIN app.teachers t ON t.id=a.teacher_id WHERE a.id=$7 AND a.class_id=$2 AND a.subject_id=$3 AND a.teacher_id=$4 AND t.user_id=$1 AND t.active AND a.semester_id=$5 AND a.effective_from<=$6::date AND (a.effective_until IS NULL OR a.effective_until>$6::date))`, actor, class, v.SubjectID, v.TeacherID, result.SemesterID, date, v.AssignmentID).Scan(&own)
				} else {
					e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_assignments a JOIN app.teachers t ON t.id=a.teacher_id WHERE a.class_id=$2 AND a.subject_id=$3 AND a.teacher_id=$4 AND t.user_id=$1 AND t.active AND a.semester_id=$5 AND a.effective_from<=$6::date AND (a.effective_until IS NULL OR a.effective_until>$6::date))`, actor, class, v.SubjectID, v.TeacherID, result.SemesterID, date).Scan(&own)
				}
				if e != nil {
					return result, e
				}
				if !own {
					continue
				}
			}
			result.Lessons = append(result.Lessons, v)
		}
	}
	if includeRoster && teacherOnly > 0 && result.RevisionID != 0 && result.SchoolDay {
		result.Fingerprint, e = lessonAttendanceFingerprint(ctx, tx, actor, teacherOnly, result, c)
		if e != nil {
			return result, e
		}
	}
	return result, nil
}

func (s *Store) FoundationHistory(ctx context.Context, actor int64, kind string, id int64) ([]History, error) {
	role := "admin"
	if kind == "calendar" {
		role = "kurikulum"
	} else if kind != "placement_date" {
		return nil, ErrNotFound
	}
	tx, e := s.reader(ctx, actor, role)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT h.id,u.display_name,coalesce(h.before_state::text,''),h.after_state::text,h.reason,h.created_at FROM app.attendance_foundation_history h JOIN app.users u ON u.id=h.actor_id WHERE h.entity_type=$1 AND h.entity_id=$2 ORDER BY h.id DESC LIMIT 100`, kind, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []History
	for rows.Next() {
		var h History
		if e = rows.Scan(&h.ID, &h.Actor, &h.Before, &h.After, &h.Reason, &h.At); e != nil {
			return nil, e
		}
		h.At = h.At.In(time.FixedZone("WIB", 7*3600))
		out = append(out, h)
	}
	return out, rows.Err()
}
