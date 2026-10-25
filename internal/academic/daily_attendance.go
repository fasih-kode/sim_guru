package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var attendanceWIB = time.FixedZone("WIB", 7*3600)
var dailyReadRoles = []string{"petugas_absensi", "wali_kelas", "kesiswaan", "kepala_sekolah"}

type DailyStudent struct {
	Policy                                                   *ArrivalRule
	LateMinutes                                              sql.NullInt64
	ID, StudentID, PlacementID, ClassID, SemesterID, Version int64
	Name, NIS, ClassName, Date, Status, Source, Arrival      string
	ArrivedAt                                                sql.NullTime
}

func (d DailyStudent) StatusLabel() string {
	switch d.Status {
	case "hadir":
		return "Hadir"
	case "sakit":
		return "Sakit"
	case "izin":
		return "Izin"
	case "alpa":
		return "Alpa"
	}
	return "Belum Dicatat"
}
func (d DailyStudent) SourceLabel() string {
	switch d.Source {
	case "qr":
		return "Kartu QR"
	case "manual":
		return "Manual"
	case "correction":
		return "Koreksi"
	}
	return "—"
}

type DailySheet struct {
	Policy           *ArrivalRule
	QuickFingerprint string
	Class            Class
	Date             string
	SchoolDay        bool
	Students         []DailyStudent
	Fingerprint      string
	BlockReason      string
}
type DailyInput struct {
	Now                                        bool
	ClassID, StudentID, Version                int64
	Date, Status, Arrival, Reason, Fingerprint string
}

func dailyClassAccess(ctx context.Context, tx *sql.Tx, actor, class int64) error {
	var allowed bool
	e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$1 AND (r.role_code IN('petugas_absensi','kesiswaan','kepala_sekolah') OR (r.role_code='wali_kelas' AND EXISTS(SELECT 1 FROM app.homeroom_assignments h JOIN app.teachers t ON t.id=h.teacher_id WHERE h.class_id=$2 AND t.user_id=$1 AND t.active))))`, actor, class).Scan(&allowed)
	if e != nil {
		return e
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
func (s *Store) DailyClasses(ctx context.Context, actor int64) ([]Class, error) {
	tx, e := s.reader(ctx, actor, dailyReadRoles...)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT c.id,c.school_year_id,y.name,c.grade,c.name,c.version FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id WHERE EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=$1 AND (r.role_code IN('petugas_absensi','kesiswaan','kepala_sekolah') OR (r.role_code='wali_kelas' AND EXISTS(SELECT 1 FROM app.homeroom_assignments h JOIN app.teachers t ON t.id=h.teacher_id WHERE h.class_id=c.id AND t.user_id=$1 AND t.active)))) ORDER BY y.active DESC,y.id DESC,c.grade,c.name`, actor)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Class
	for rows.Next() {
		var c Class
		if e = rows.Scan(&c.ID, &c.SchoolYearID, &c.YearName, &c.Grade, &c.Name, &c.Version); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func dailyDate(ctx context.Context, tx *sql.Tx, date string) error {
	if _, e := civilDate(date); e != nil {
		return e
	}
	var future bool
	if e := tx.QueryRowContext(ctx, `SELECT $1::date>(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date`, date).Scan(&future); e != nil {
		return e
	}
	if future {
		return ValidationError("Absensi tidak dapat dicatat untuk tanggal mendatang.")
	}
	return nil
}
func dailyCalendar(ctx context.Context, tx *sql.Tx, year int64, date string) (int64, bool, error) {
	var semester int64
	e := tx.QueryRowContext(ctx, `SELECT sm.id FROM app.semesters sm JOIN app.semester_calendars c ON c.semester_id=sm.id WHERE sm.school_year_id=$1 AND $2::date BETWEEN c.starts_on AND c.ends_on`, year, date).Scan(&semester)
	if errors.Is(e, sql.ErrNoRows) {
		return 0, false, ValidationError("Tanggal belum tercakup kalender semester. Minta Kurikulum melengkapinya.")
	}
	if e != nil {
		return 0, false, e
	}
	c, e := querySemesterCalendar(ctx, tx, semester)
	if e != nil {
		return 0, false, e
	}
	school, e := c.IsSchoolDay(date)
	return semester, school, e
}
func dailyRecord(row rowScanner) (DailyStudent, error) {
	var d DailyStudent
	var policy []byte
	e := row.Scan(&d.ID, &d.StudentID, &d.PlacementID, &d.ClassID, &d.SemesterID, &d.Version, &d.Name, &d.NIS, &d.ClassName, &d.Date, &d.Status, &d.Source, &d.ArrivedAt, &policy, &d.LateMinutes)
	if errors.Is(e, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if e == nil && len(policy) > 0 {
		if er := json.Unmarshal(policy, &d.Policy); er != nil {
			return d, er
		}
	}
	if d.ArrivedAt.Valid {
		d.Arrival = d.ArrivedAt.Time.In(attendanceWIB).Format("15:04")
	}
	return d, e
}

const dailyColumns = `id,student_id,placement_id,class_id,semester_id,version,student_name,nis,class_name,on_date::text,status,source,arrived_at,arrival_policy,late_minutes`

func dailySheet(ctx context.Context, tx *sql.Tx, actor, class int64, date string) (DailySheet, error) {
	out := DailySheet{Date: date}
	if e := dailyDate(ctx, tx, date); e != nil {
		return out, e
	}
	if e := dailyClassAccess(ctx, tx, actor, class); e != nil {
		return out, e
	}
	e := tx.QueryRowContext(ctx, `SELECT c.id,c.school_year_id,y.name,c.grade,c.name,c.version FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id WHERE c.id=$1`, class).Scan(&out.Class.ID, &out.Class.SchoolYearID, &out.Class.YearName, &out.Class.Grade, &out.Class.Name, &out.Class.Version)
	if errors.Is(e, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	// Already recorded evidence stays readable if a later calendar edit removes
	// this date. New records still require a configured school day at write time.
	var semester int64
	semester, out.SchoolDay, e = dailyCalendar(ctx, tx, out.Class.SchoolYearID, date)
	var recorded int
	if e != nil {
		if !isValidation(e) {
			return out, e
		}
		if er := tx.QueryRowContext(ctx, `SELECT count(*) FROM app.student_daily_attendance WHERE class_id=$1 AND on_date=$2`, class, date).Scan(&recorded); er != nil {
			return out, er
		}
		if recorded == 0 {
			return out, e
		}
	}
	if semester > 0 {
		out.Policy, e = arrivalPolicy(ctx, tx, semester, class, date)
		if e != nil {
			return out, e
		}
	}
	var unknown bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_placements p WHERE p.school_year_id=$1 AND p.starts_on IS NULL AND app.student_placement_class_at(p.id,$3::date)=$2)`, out.Class.SchoolYearID, class, date).Scan(&unknown)
	if e != nil {
		return out, e
	}
	if unknown {
		out.BlockReason = "Tanggal mulai penempatan siswa belum lengkap. Minta Admin melengkapinya sebelum mencatat siswa baru."
	}
	rows, e := tx.QueryContext(ctx, `SELECT p.student_id,p.id,s.name,coalesce(s.nis,'') FROM app.student_placements p JOIN app.students s ON s.id=p.student_id WHERE p.school_year_id=$1 AND p.starts_on<=$3::date AND app.student_placement_class_at(p.id,$3::date)=$2 AND NOT EXISTS(SELECT 1 FROM app.student_daily_attendance a WHERE a.student_id=p.student_id AND a.on_date=$3 AND a.class_id<>$2) ORDER BY s.name,s.id`, out.Class.SchoolYearID, class, date)
	if e != nil {
		return out, e
	}
	students := map[int64]DailyStudent{}
	for rows.Next() {
		var d DailyStudent
		if e = rows.Scan(&d.StudentID, &d.PlacementID, &d.Name, &d.NIS); e != nil {
			rows.Close()
			return out, e
		}
		d.ClassID = class
		d.Date = date
		d.ClassName = out.ClassLabel()
		students[d.StudentID] = d
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.QueryContext(ctx, `SELECT `+dailyColumns+` FROM app.student_daily_attendance WHERE class_id=$1 AND on_date=$2 ORDER BY student_name,student_id`, class, date)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		d, er := dailyRecord(rows)
		if er != nil {
			rows.Close()
			return out, er
		}
		students[d.StudentID] = d
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, d := range students {
		out.Students = append(out.Students, d)
	}
	sortDaily(out.Students)
	// Fingerprint guards manual forms against roster/calendar changes since GET.
	out.Fingerprint, e = dailyFingerprint(ctx, tx, out.Class.SchoolYearID, class, date)
	if e == nil {
		out.QuickFingerprint = quickFingerprint(out)
	}
	return out, e
}
func (s DailySheet) ClassLabel() string { return classLabel(s.Class) }
func isValidation(e error) bool         { var v ValidationError; return errors.As(e, &v) }
func (s *Store) DailyAttendance(ctx context.Context, actor, class int64, date string) (DailySheet, error) {
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return DailySheet{}, e
	}
	defer tx.Rollback()
	if e = s.authorize(ctx, tx, actor, dailyReadRoles...); e != nil {
		return DailySheet{}, e
	}
	return dailySheet(ctx, tx, actor, class, date)
}
func (s *Store) SaveDailyAttendance(ctx context.Context, actor int64, in DailyInput) (DailyStudent, error) {
	tx, e := s.begin(ctx, actor, "petugas_absensi")
	if e != nil {
		return DailyStudent{}, e
	}
	defer tx.Rollback()
	d, e := saveDailyAttendanceTx(ctx, tx, actor, in)
	if e != nil {
		return d, e
	}
	return d, tx.Commit()
}
func saveDailyAttendanceTx(ctx context.Context, tx *sql.Tx, actor int64, in DailyInput) (DailyStudent, error) {
	if in.Status != "hadir" && in.Status != "sakit" && in.Status != "izin" && in.Status != "alpa" {
		return DailyStudent{}, ValidationError("Pilih Hadir, Sakit, Izin, atau Alpa.")
	}
	if e := foundationReason(in.Reason); e != nil {
		return DailyStudent{}, e
	}
	if e := dailyDate(ctx, tx, in.Date); e != nil {
		return DailyStudent{}, e
	}
	old, e := dailyRecord(tx.QueryRowContext(ctx, `SELECT `+dailyColumns+` FROM app.student_daily_attendance WHERE student_id=$1 AND on_date=$2 FOR UPDATE`, in.StudentID, in.Date))
	exists := e == nil
	if e != nil && !errors.Is(e, ErrNotFound) {
		return old, e
	}
	if exists && (old.ClassID != in.ClassID || old.Version != in.Version) {
		return old, ErrConflict
	}
	if !exists && in.Version != 0 {
		return old, ErrConflict
	}
	d := old
	if !exists {
		sheet, e := dailySheet(ctx, tx, actor, in.ClassID, in.Date)
		if e != nil {
			return d, e
		}
		if sheet.BlockReason != "" {
			return d, ValidationError(sheet.BlockReason)
		}
		if !sheet.SchoolDay {
			return d, ValidationError("Tanggal ini bukan hari sekolah.")
		}
		if in.Fingerprint == "" || sheet.Fingerprint != in.Fingerprint {
			return d, ErrConflict
		}
		found := false
		for _, x := range sheet.Students {
			if x.StudentID == in.StudentID {
				d = x
				found = true
				break
			}
		}
		if !found {
			return d, ValidationError("Siswa tidak termasuk kelas pada tanggal ini.")
		}
		d.SemesterID, _, e = dailyCalendar(ctx, tx, sheet.Class.SchoolYearID, in.Date)
		if e != nil {
			return d, e
		}
	}
	d.Status = in.Status
	d.Source = "manual"
	d.ArrivedAt = sql.NullTime{}
	d.Arrival = ""
	if exists {
		d.Source = "correction"
	}
	if in.Status == "hadir" && in.Now {
		var now time.Time
		if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return d, e
		}
		if now.In(attendanceWIB).Format("2006-01-02") != in.Date {
			return d, ValidationError("Catat datang sekarang hanya untuk hari ini.")
		}
		d.ArrivedAt = sql.NullTime{Time: now, Valid: true}
		d.Arrival = now.In(attendanceWIB).Format("15:04")
	} else if in.Status == "hadir" {
		if in.Arrival == "" {
			return d, ValidationError("Jam kedatangan wajib diisi untuk Hadir.")
		}

		at, e := time.ParseInLocation("2006-01-02 15:04", in.Date+" "+in.Arrival, attendanceWIB)
		if e != nil || at.Format("15:04") != in.Arrival {
			return d, ValidationError("Gunakan waktu 24 jam HH:MM, misalnya 06:11 atau 18:11.")
		}
		var future bool
		if e = tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, at).Scan(&future); e != nil {
			return d, e
		}
		if future {
			return d, ValidationError("Waktu kedatangan tidak boleh di masa depan.")
		}
		d.ArrivedAt = sql.NullTime{Time: at, Valid: true}
		d.Arrival = in.Arrival
	}
	if d.Policy == nil {
		d.Policy, e = arrivalPolicy(ctx, tx, d.SemesterID, d.ClassID, d.Date)
		if e != nil {
			return d, e
		}
	}
	assessArrival(&d)
	policyID, policy, e := policyJSON(d.Policy)
	if e != nil {
		return d, e
	}
	if !exists {
		e = tx.QueryRowContext(ctx, `INSERT INTO app.student_daily_attendance(student_id,placement_id,class_id,semester_id,on_date,student_name,nis,class_name,status,arrived_at,source,actor_id,arrival_rule_id,arrival_policy,late_minutes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id,version`, d.StudentID, d.PlacementID, d.ClassID, d.SemesterID, in.Date, d.Name, d.NIS, d.ClassName, d.Status, d.ArrivedAt, d.Source, actor, policyID, policy, d.LateMinutes).Scan(&d.ID, &d.Version)
	} else {
		d.Version++
		_, e = tx.ExecContext(ctx, `UPDATE app.student_daily_attendance SET status=$2,arrived_at=$3,source=$4,version=version+1,actor_id=$5,updated_at=clock_timestamp(),arrival_rule_id=$6,arrival_policy=$7,late_minutes=$8 WHERE id=$1`, d.ID, d.Status, d.ArrivedAt, d.Source, actor, policyID, policy, d.LateMinutes)
	}
	if e != nil {
		return d, e
	}
	var before any
	if exists {
		before = old
	}
	if e = dailyHistory(ctx, tx, actor, d, before, strings.TrimSpace(in.Reason)); e != nil {
		return d, e
	}
	return d, nil
}
func dailyHistory(ctx context.Context, tx *sql.Tx, actor int64, d DailyStudent, before any, reason string) error {
	b, e := json.Marshal(before)
	if e != nil {
		return e
	}
	a, e := json.Marshal(d)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO app.student_daily_attendance_history(attendance_id,actor_id,before_state,after_state,reason) VALUES($1,$2,$3::jsonb,$4::jsonb,$5)`, d.ID, actor, b, a, reason)
	return e
}
func (s *Store) DailyHistory(ctx context.Context, actor, id int64) ([]History, error) {
	tx, e := s.reader(ctx, actor, dailyReadRoles...)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var class int64
	if e = tx.QueryRowContext(ctx, `SELECT class_id FROM app.student_daily_attendance WHERE id=$1`, id).Scan(&class); errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if e != nil {
		return nil, e
	}
	if e = dailyClassAccess(ctx, tx, actor, class); e != nil {
		return nil, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT h.id,u.display_name,coalesce(h.before_state::text,''),h.after_state::text,h.reason,h.created_at FROM app.student_daily_attendance_history h JOIN app.users u ON u.id=h.actor_id WHERE h.attendance_id=$1 ORDER BY h.id DESC LIMIT 100`, id)
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
		h.At = h.At.In(attendanceWIB)
		out = append(out, h)
	}
	return out, rows.Err()
}

func classLabel(c Class) string { return fmt.Sprintf("%d%s", c.Grade, c.Name) }
func sortDaily(rows []DailyStudent) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name == rows[j].Name {
			return rows[i].StudentID < rows[j].StudentID
		}
		return rows[i].Name < rows[j].Name
	})
}
func dailyFingerprint(ctx context.Context, tx *sql.Tx, year, class int64, date string) (string, error) {
	var value string
	e := tx.QueryRowContext(ctx, `SELECT md5(jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(c) ORDER BY c.semester_id) FROM app.semester_calendars c JOIN app.semesters sm ON sm.id=c.semester_id WHERE sm.school_year_id=$1),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY x.semester_id,x.on_date) FROM app.school_date_exceptions x JOIN app.semesters sm ON sm.id=x.semester_id WHERE sm.school_year_id=$1),
 (SELECT jsonb_agg(jsonb_build_array(to_jsonb(p),to_jsonb(s),app.student_placement_class_at(p.id,$3::date)) ORDER BY p.id) FROM app.student_placements p JOIN app.students s ON s.id=p.student_id WHERE p.school_year_id=$1),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM app.student_arrival_rules r JOIN app.semesters sm ON sm.id=r.semester_id WHERE sm.school_year_id=$1),
 (SELECT to_jsonb(c) FROM app.classes c WHERE c.id=$2))::text)`, year, class, date).Scan(&value)
	return value, e
}
