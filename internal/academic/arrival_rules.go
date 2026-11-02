package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type ArrivalRule struct {
	ID, SemesterID, ClassID                                 int64
	Weekday, Tolerance                                      int
	Date, EffectiveFrom, Deadline, ClassName, Reason, Actor string
	At                                                      time.Time
}
type ArrivalSettings struct {
	Semesters []Semester
	Classes   []Class
	Rules     []ArrivalRule
}

func (r ArrivalRule) ScopeLabel() string {
	if r.ClassID == 0 {
		return "Semua rombel"
	}
	return r.ClassName
}
func (r ArrivalRule) DayLabel() string {
	if r.Date != "" {
		return r.Date
	}
	return []string{"", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}[r.Weekday]
}
func minuteOfDay(raw string) (int, error) {
	t, e := time.Parse("15:04", raw)
	if e != nil || t.Format("15:04") != raw {
		return 0, ValidationError("Gunakan waktu 24 jam HH:MM, misalnya 06:11 atau 18:11.")
	}
	return t.Hour()*60 + t.Minute(), nil
}
func scanArrivalRule(row rowScanner) (ArrivalRule, error) {
	var r ArrivalRule
	e := row.Scan(&r.ID, &r.SemesterID, &r.ClassID, &r.Weekday, &r.Date, &r.EffectiveFrom, &r.Deadline, &r.Tolerance, &r.ClassName, &r.Reason, &r.Actor, &r.At)
	if errors.Is(e, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	r.At = r.At.In(attendanceWIB)
	return r, e
}

const arrivalRuleSelect = `SELECT r.id,r.semester_id,coalesce(r.class_id,0),r.weekday,coalesce(r.on_date::text,''),r.effective_from::text,to_char(r.deadline,'HH24:MI'),r.tolerance_minutes,coalesce(c.grade::text||c.name,''),r.reason,u.display_name,r.created_at FROM app.student_arrival_rules r LEFT JOIN app.classes c ON c.id=r.class_id JOIN app.users u ON u.id=r.actor_id `

func (s *Store) ArrivalSettings(ctx context.Context, actor, semester int64) (ArrivalSettings, error) {
	var out ArrivalSettings
	tx, e := s.reader(ctx, actor, "kesiswaan")
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT sm.id,sm.school_year_id,y.name,sm.number,sm.active,y.active,sm.version FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id ORDER BY y.active DESC,y.id DESC,sm.number`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var sm Semester
		if e = rows.Scan(&sm.ID, &sm.SchoolYearID, &sm.YearName, &sm.Number, &sm.Active, &sm.YearActive, &sm.Version); e != nil {
			rows.Close()
			return out, e
		}
		out.Semesters = append(out.Semesters, sm)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.QueryContext(ctx, `SELECT c.id,c.school_year_id,y.name,c.grade,c.name,c.version FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id WHERE c.school_year_id=(SELECT school_year_id FROM app.semesters WHERE id=$1) ORDER BY c.grade,c.name`, semester)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var c Class
		if e = rows.Scan(&c.ID, &c.SchoolYearID, &c.YearName, &c.Grade, &c.Name, &c.Version); e != nil {
			rows.Close()
			return out, e
		}
		out.Classes = append(out.Classes, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.QueryContext(ctx, arrivalRuleSelect+`WHERE r.semester_id=$1 ORDER BY r.id DESC`, semester)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		r, e := scanArrivalRule(rows)
		if e != nil {
			return out, e
		}
		out.Rules = append(out.Rules, r)
	}
	return out, rows.Err()
}
func (s *Store) SaveArrivalRule(ctx context.Context, actor, expected int64, r ArrivalRule) (int64, error) {
	if e := foundationReason(r.Reason); e != nil {
		return 0, e
	}
	minute, e := minuteOfDay(r.Deadline)
	if e != nil {
		return 0, e
	}
	if r.ClassID < 0 || r.SemesterID < 1 || r.Tolerance < 0 || minute+r.Tolerance >= 1440 {
		return 0, ValidationError("Pilih cakupan dan toleransi yang tidak melewati akhir hari.")
	}
	if r.Date != "" {
		r.EffectiveFrom = r.Date
		r.Weekday = 0
	} else if r.Weekday < 1 || r.Weekday > 7 {
		return 0, ValidationError("Pilih hari yang valid.")
	}
	if _, e = civilDate(r.EffectiveFrom); e != nil {
		return 0, e
	}
	tx, e := s.begin(ctx, actor, "kesiswaan")
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	cal, e := querySemesterCalendar(ctx, tx, r.SemesterID)
	if e != nil {
		return 0, e
	}
	if cal.Version == 0 || r.EffectiveFrom < cal.StartsOn || r.EffectiveFrom > cal.EndsOn {
		return 0, ValidationError("Tanggal berlaku harus berada dalam kalender semester yang telah diatur Kurikulum.")
	}
	if r.ClassID > 0 {
		var eligible bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.classes c JOIN app.semesters sm ON sm.school_year_id=c.school_year_id WHERE c.id=$1 AND sm.id=$2)`, r.ClassID, r.SemesterID).Scan(&eligible)
		if e != nil {
			return 0, e
		}
		if !eligible {
			return 0, ValidationError("Rombel harus berada pada tahun ajaran semester ini.")
		}
	}
	var latest int64
	e = tx.QueryRowContext(ctx, `SELECT coalesce(max(id),0) FROM app.student_arrival_rules WHERE semester_id=$1 AND coalesce(class_id,0)=$2 AND weekday=$3 AND coalesce(on_date::text,'')=$4`, r.SemesterID, r.ClassID, r.Weekday, r.Date).Scan(&latest)
	if e != nil {
		return 0, e
	}
	if latest != expected {
		return 0, ErrConflict
	}
	var id int64
	e = tx.QueryRowContext(ctx, `INSERT INTO app.student_arrival_rules(semester_id,class_id,weekday,on_date,effective_from,deadline,tolerance_minutes,reason,actor_id) VALUES($1,nullif($2,0),$3,nullif($4,'')::date,$5,$6,$7,$8,$9) RETURNING id`, r.SemesterID, r.ClassID, r.Weekday, r.Date, r.EffectiveFrom, r.Deadline, r.Tolerance, r.Reason, actor).Scan(&id)
	if e != nil {
		return 0, e
	}
	return id, tx.Commit()
}
func arrivalPolicy(ctx context.Context, tx *sql.Tx, semester, class int64, date string) (*ArrivalRule, error) {
	day, e := civilDate(date)
	if e != nil {
		return nil, e
	}
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	r, e := scanArrivalRule(tx.QueryRowContext(ctx, arrivalRuleSelect+`WHERE r.semester_id=$1 AND (r.class_id IS NULL OR r.class_id=$2) AND r.effective_from<=$3::date AND (r.on_date=$3::date OR (r.on_date IS NULL AND r.weekday=$4)) ORDER BY (r.on_date IS NOT NULL) DESC,(r.class_id IS NOT NULL) DESC,r.effective_from DESC,r.id DESC LIMIT 1`, semester, class, date, weekday))
	if errors.Is(e, ErrNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return &r, nil
}
func assessArrival(d *DailyStudent) {
	d.LateMinutes = sql.NullInt64{}
	if d.Status != "hadir" || !d.ArrivedAt.Valid || d.Policy == nil {
		return
	}
	at := d.ArrivedAt.Time.In(attendanceWIB)
	minutes := at.Hour()*60 + at.Minute()
	deadline, _ := minuteOfDay(d.Policy.Deadline)
	late := 0
	if minutes > deadline+d.Policy.Tolerance {
		late = minutes - deadline
	}
	d.LateMinutes = sql.NullInt64{Int64: int64(late), Valid: true}
}
func (d DailyStudent) PunctualityLabel() string {
	if d.Status != "hadir" {
		return "—"
	}
	if !d.ArrivedAt.Valid {
		return "Waktu belum lengkap"
	}
	if !d.LateMinutes.Valid {
		return "Belum dapat dinilai"
	}
	if d.LateMinutes.Int64 == 0 {
		return "Tepat waktu"
	}
	return fmt.Sprintf("Terlambat %d menit", d.LateMinutes.Int64)
}
func policyJSON(p *ArrivalRule) (any, any, error) {
	if p == nil {
		return nil, nil, nil
	}
	raw, e := json.Marshal(p)
	return p.ID, raw, e
}
