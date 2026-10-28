package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type TeacherAttendanceRecapFilter struct {
	StartsOn, EndsOn, Search string
	TeacherID                int64
	Page                     int
	Managed                  bool
}

type TeacherAttendanceRecapResult struct {
	Teachers []TeacherAttendanceRecap
	Now      time.Time
}

// TeacherAttendanceRecaps membaca konteks satu rentang dalam snapshot transaksi
// yang sama. Jumlah query tetap, bukan satu query untuk setiap guru atau tanggal.
func (s *Store) TeacherAttendanceRecaps(ctx context.Context, actor int64, f TeacherAttendanceRecapFilter) (TeacherAttendanceRecapResult, error) {
	var result TeacherAttendanceRecapResult
	start, e := civilDate(f.StartsOn)
	if e != nil {
		return result, e
	}
	end, e := civilDate(f.EndsOn)
	if e != nil {
		return result, e
	}
	// Batas satu tahun per pembacaan membatasi biaya rekap, bukan usia koreksi
	// maupun durasi laporan. Rentang lain tetap dapat dibuka melalui filter.
	if end.Before(start) || end.Sub(start) > 365*24*time.Hour || f.Page < 1 || f.Page > 100000 || !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > 120 || strings.ContainsRune(f.Search, 0) || f.TeacherID < 0 {
		return result, ValidationError("Filter rekap tidak valid. Pilih rentang berurutan maksimal 366 tanggal.")
	}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return result, e
	}
	defer tx.Rollback()
	if f.Managed {
		e = s.authorize(ctx, tx, actor, "pegawai_tu")
		if e == nil {
			e = s.authorize(ctx, tx, actor, "pengelola_kehadiran_guru")
		}
	} else {
		e = s.authorize(ctx, tx, actor, "guru_mapel", "wali_kelas")
	}
	if e != nil {
		return result, e
	}
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&result.Now); e != nil {
		return result, e
	}
	limit, offset := 11, (f.Page-1)*10
	if !f.Managed || f.TeacherID > 0 {
		limit, offset = 1, 0
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,name FROM app.teachers WHERE ($1 OR (user_id=$2 AND active)) AND ($3=0 OR id=$3) AND ($4='' OR name ILIKE '%'||$4||'%') ORDER BY name,id LIMIT $5 OFFSET $6`, f.Managed, actor, f.TeacherID, strings.TrimSpace(f.Search), limit, offset)
	if e != nil {
		return result, e
	}
	var ids []int64
	for rows.Next() {
		var r TeacherAttendanceRecap
		if e = rows.Scan(&r.TeacherID, &r.TeacherName); e != nil {
			rows.Close()
			return result, e
		}
		ids = append(ids, r.TeacherID)
		result.Teachers = append(result.Teachers, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return result, e
	}
	if len(ids) == 0 {
		if !f.Managed || f.TeacherID > 0 {
			return result, ErrNotFound
		}
		return result, nil
	}
	days, e := recapCalendarDays(ctx, tx, f.StartsOn, f.EndsOn)
	if e != nil {
		return result, e
	}
	lessons, e := recapLessons(ctx, tx, ids, f.StartsOn, f.EndsOn)
	if e != nil {
		return result, e
	}
	events, e := recapEvents(ctx, tx, ids, f.StartsOn, f.EndsOn)
	if e != nil {
		return result, e
	}
	reports, e := recapReports(ctx, tx, ids, f.StartsOn, f.EndsOn)
	if e != nil {
		return result, e
	}
	for i := range result.Teachers {
		r := &result.Teachers[i]
		for _, day := range days {
			in := TeacherAttendanceEvaluationInput{TeacherID: r.TeacherID, TeacherName: r.TeacherName, Date: day.date, Now: result.Now, ScheduleKnown: day.known, UnavailableReason: day.reason, SemesterID: day.semester, CalendarVersion: day.version, RevisionID: day.revision, CalendarNote: day.note}
			for _, lesson := range lessons[r.TeacherID] {
				if !day.school || lesson.revision != day.revision || lesson.weekday != day.weekday {
					continue
				}
				if !lesson.validOn(day.date) || lesson.start == "" || lesson.end == "" {
					in.ScheduleKnown = false
					in.UnavailableReason = "Entri jadwal tidak cocok dengan penugasan efektif; minta Kurikulum memeriksanya."
					continue
				}
				begins, err := time.ParseInLocation("2006-01-02 15:04:05", day.date+" "+lesson.start, teacherAttendanceZone)
				if err != nil {
					return result, err
				}
				ends, err := time.ParseInLocation("2006-01-02 15:04:05", day.date+" "+lesson.end, teacherAttendanceZone)
				if err != nil {
					return result, err
				}
				if !ends.After(begins) {
					return result, errors.New("invalid published lesson interval")
				}
				in.Obligations = append(in.Obligations, TeacherObligation{EntryID: lesson.id, RevisionID: lesson.revision, AssignmentID: lesson.assignment, ClassName: lesson.class, SubjectName: lesson.subject, Segments: lesson.segments, StartsAt: begins, EndsAt: ends})
			}
			for _, ev := range events[r.TeacherID] {
				if TeacherAttendanceWIBDate(ev.OccurredAt) != day.date {
					continue
				}
				v := ev
				if ev.Action == TeacherCheckIn {
					in.CheckIn = &v
				} else {
					in.CheckOut = &v
				}
			}
			for _, report := range reports[r.TeacherID] {
				if day.date >= report.StartsOn && day.date <= report.EndsOn {
					v := report
					in.Report = &v
					break
				}
			}
			r.Days = append(r.Days, EvaluateTeacherAttendance(in))
		}
		r.summarize()
	}
	return result, nil
}

type recapCalendarDay struct {
	date, note, reason          string
	semester, version, revision int64
	weekday                     int
	school, known               bool
}

func recapCalendarDays(ctx context.Context, tx *sql.Tx, start, end string) ([]recapCalendarDay, error) {
	rows, e := tx.QueryContext(ctx, `WITH dates AS (SELECT d::date AS day FROM generate_series($1::date,$2::date,interval '1 day') d)
 SELECT d.day::text,extract(isodow FROM d.day)::int,coalesce(c.semester_id,0),coalesce(c.version,0),
 coalesce(x.label,''),coalesce(x.school_day,c.school_days @> to_jsonb(extract(isodow FROM d.day)::int),false),
 coalesce(r.id,0),(SELECT count(*) FROM app.semester_calendars cal WHERE d.day BETWEEN cal.starts_on AND cal.ends_on)
 FROM dates d LEFT JOIN LATERAL (SELECT * FROM app.semester_calendars cal WHERE d.day BETWEEN cal.starts_on AND cal.ends_on ORDER BY semester_id LIMIT 1) c ON true
 LEFT JOIN app.school_date_exceptions x ON x.semester_id=c.semester_id AND x.on_date=d.day
 LEFT JOIN LATERAL (SELECT id FROM app.weekly_timetable_revisions rev WHERE rev.semester_id=c.semester_id AND rev.status='published' AND rev.effective_from<=d.day ORDER BY effective_from DESC,revision_no DESC LIMIT 1) r ON true ORDER BY d.day`, start, end)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var list []recapCalendarDay
	for rows.Next() {
		var d recapCalendarDay
		var count int
		if e = rows.Scan(&d.date, &d.weekday, &d.semester, &d.version, &d.note, &d.school, &d.revision, &count); e != nil {
			return nil, e
		}
		switch {
		case count == 0:
			d.reason = "Kalender semester untuk tanggal ini belum tersedia."
		case count > 1:
			d.reason = "Lebih dari satu kalender semester mencakup tanggal ini; perlu pemeriksaan Kurikulum."
			d.semester, d.version, d.revision, d.school, d.note = 0, 0, 0, false, ""
		case d.school && d.revision == 0:
			d.reason = "Jadwal published yang berlaku pada tanggal ini belum tersedia."
		default:
			d.known = true
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

type recapLesson struct {
	id, teacher, revision, assignment                 int64
	weekday                                           int
	start, end, class, subject, segments, from, until string
	valid                                             bool
	periodStart, periodEnd                            int
}

func (l recapLesson) validOn(date string) bool {
	return l.valid && l.from <= date && (l.until == "" || date < l.until)
}

func recapLessons(ctx context.Context, tx *sql.Tx, ids []int64, start, end string) (map[int64][]recapLesson, error) {
	rows, e := tx.QueryContext(ctx, `SELECT e.id,e.teacher_id,e.revision_id,e.assignment_id,e.weekday,coalesce(p1.starts_at::text,''),coalesce(p2.ends_at::text,''),c.grade::text||c.name,sub.name,e.period_start,e.period_end,
 coalesce(a.effective_from::text,''),coalesce(a.effective_until::text,''),coalesce(a.teacher_id=e.teacher_id AND a.class_id=e.class_id AND a.subject_id=e.subject_id AND a.semester_id=r.semester_id AND c.school_year_id=sm.school_year_id,false)
 FROM app.weekly_timetable_entries e JOIN app.weekly_timetable_revisions r ON r.id=e.revision_id AND r.status='published'
 JOIN app.semesters sm ON sm.id=r.semester_id JOIN app.semester_calendars cal ON cal.semester_id=r.semester_id
 JOIN app.classes c ON c.id=e.class_id JOIN app.subjects sub ON sub.id=e.subject_id
 LEFT JOIN app.teaching_assignments a ON a.id=e.assignment_id
 LEFT JOIN app.weekly_timetable_periods p1 ON p1.revision_id=r.id AND p1.weekday=e.weekday AND p1.period_no=e.period_start
 LEFT JOIN app.weekly_timetable_periods p2 ON p2.revision_id=r.id AND p2.weekday=e.weekday AND p2.period_no=e.period_end
 WHERE e.teacher_id=ANY($1) AND r.effective_from<=$3::date AND cal.starts_on<=$3::date AND cal.ends_on>=$2::date ORDER BY r.id,e.weekday,e.period_start,e.id`, ids, start, end)
	if e != nil {
		return nil, e
	}
	var all []recapLesson
	for rows.Next() {
		var l recapLesson
		if e = rows.Scan(&l.id, &l.teacher, &l.revision, &l.assignment, &l.weekday, &l.start, &l.end, &l.class, &l.subject, &l.periodStart, &l.periodEnd, &l.from, &l.until, &l.valid); e != nil {
			rows.Close()
			return nil, e
		}
		all = append(all, l)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	periods, e := tx.QueryContext(ctx, `SELECT p.revision_id,p.weekday,p.period_no,p.starts_at::text,p.ends_at::text FROM app.weekly_timetable_periods p JOIN app.weekly_timetable_revisions r ON r.id=p.revision_id JOIN app.semester_calendars c ON c.semester_id=r.semester_id WHERE r.status='published' AND r.effective_from<=$2::date AND c.starts_on<=$2::date AND c.ends_on>=$1::date ORDER BY p.revision_id,p.weekday,p.period_no`, start, end)
	if e != nil {
		return nil, e
	}
	defer periods.Close()
	byRev := map[int64][]LessonPeriod{}
	for periods.Next() {
		var id int64
		var p LessonPeriod
		if e = periods.Scan(&id, &p.Weekday, &p.Number, &p.StartsAt, &p.EndsAt); e != nil {
			return nil, e
		}
		byRev[id] = append(byRev[id], p)
	}
	if e = periods.Err(); e != nil {
		return nil, e
	}
	result := map[int64][]recapLesson{}
	for _, l := range all {
		l.segments = entrySegments(TimetableEntry{Weekday: l.weekday, PeriodStart: l.periodStart, PeriodEnd: l.periodEnd}, byRev[l.revision])
		result[l.teacher] = append(result[l.teacher], l)
	}
	return result, nil
}

func recapEvents(ctx context.Context, tx *sql.Tx, ids []int64, start, end string) (map[int64][]TeacherAttendanceManagedEvent, error) {
	rows, e := tx.QueryContext(ctx, `SELECT teacher_id,id,version,action,source,occurred_at,recorded_at FROM app.teacher_attendance_events WHERE teacher_id=ANY($1) AND on_date BETWEEN $2::date AND $3::date ORDER BY on_date,action`, ids, start, end)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[int64][]TeacherAttendanceManagedEvent{}
	for rows.Next() {
		var id int64
		var v TeacherAttendanceManagedEvent
		if e = rows.Scan(&id, &v.ID, &v.Version, &v.Action, &v.Source, &v.OccurredAt, &v.RecordedAt); e != nil {
			return nil, e
		}
		v.OccurredAt = TeacherAttendanceWIBTime(v.OccurredAt)
		v.RecordedAt = TeacherAttendanceWIBTime(v.RecordedAt)
		out[id] = append(out[id], v)
	}
	return out, rows.Err()
}
func recapReports(ctx context.Context, tx *sql.Tx, ids []int64, start, end string) (map[int64][]TeacherAttendanceReport, error) {
	rows, e := tx.QueryContext(ctx, `SELECT `+teacherReportColumns+` FROM app.teacher_attendance_reports WHERE teacher_id=ANY($1) AND active AND starts_on<=$3::date AND ends_on>=$2::date ORDER BY id`, ids, start, end)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[int64][]TeacherAttendanceReport{}
	for rows.Next() {
		r, e := scanTeacherReport(rows)
		if e != nil {
			return nil, e
		}
		out[r.TeacherID] = append(out[r.TeacherID], r)
	}
	return out, rows.Err()
}
