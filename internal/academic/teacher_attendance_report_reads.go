package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type TeacherReportScope string

const (
	TeacherReportOwn   TeacherReportScope = "own"
	TeacherReportTU    TeacherReportScope = "tu"
	TeacherReportAdmin TeacherReportScope = "admin"
)

type TeacherAttendanceReportFilter struct {
	Scope                                  TeacherReportScope
	Search, StartsOn, EndsOn, Kind, Status string
	Page                                   int
}

func (s *Store) teacherReportReader(ctx context.Context, actor int64, scope TeacherReportScope) (*sql.Tx, error) {
	switch scope {
	case TeacherReportTU:
		return s.readerAll(ctx, actor, "pegawai_tu", "pengelola_kehadiran_guru")
	case TeacherReportAdmin:
		return s.reader(ctx, actor, "admin")
	case TeacherReportOwn:
		tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if e != nil {
			return nil, e
		}
		var allowed bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.users u WHERE u.id=$1 AND u.active
			AND EXISTS(SELECT 1 FROM app.user_roles r WHERE r.user_id=u.id AND r.role_code IN ('guru_mapel','wali_kelas')))`, actor).Scan(&allowed)
		if e != nil || !allowed {
			tx.Rollback()
			if e != nil {
				return nil, e
			}
			return nil, ErrForbidden
		}
		return tx, nil
	default:
		return nil, ErrForbidden
	}
}

// TeacherAttendanceReports membaca 10 laporan dan satu penanda halaman berikutnya.
func (s *Store) TeacherAttendanceReports(ctx context.Context, actor int64, f TeacherAttendanceReportFilter) ([]TeacherAttendanceReport, error) {
	if f.Page < 1 || f.Page > 100000 || len(f.Search) > 480 {
		return nil, ValidationError("Filter atau halaman laporan tidak valid.")
	}
	if f.Kind != "" && f.Kind != "izin" && f.Kind != "sakit" && f.Kind != "dinas_luar" {
		return nil, ValidationError("Jenis laporan tidak valid.")
	}
	if f.Status != "" && f.Status != "active" && f.Status != "cancelled" {
		return nil, ValidationError("Status laporan tidak valid.")
	}
	if (f.StartsOn == "") != (f.EndsOn == "") {
		return nil, ValidationError("Isi kedua tanggal filter rentang.")
	}
	if f.StartsOn != "" {
		_, e := validateTeacherReport(TeacherAttendanceReportInput{Kind: "izin", StartsOn: f.StartsOn, EndsOn: f.EndsOn, Description: "filter"})
		if e != nil {
			return nil, e
		}
	}
	tx, e := s.teacherReportReader(ctx, actor, f.Scope)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT `+teacherReportColumns+` FROM app.teacher_attendance_reports r
 WHERE ($1='tu' OR EXISTS(SELECT 1 FROM app.teachers t WHERE t.id=r.teacher_id AND t.user_id=$2))
 AND ($3='' OR teacher_name ILIKE '%'||$3||'%') AND ($4='' OR kind=$4)
 AND ($5='' OR active=($5='active'))
 AND ($6='' OR (starts_on<=NULLIF($7,'')::date AND ends_on>=NULLIF($6,'')::date))
 ORDER BY id DESC LIMIT 11 OFFSET $8`, string(f.Scope), actor, strings.TrimSpace(f.Search), f.Kind, f.Status, f.StartsOn, f.EndsOn, (f.Page-1)*10)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var list []TeacherAttendanceReport
	for rows.Next() {
		r, e := scanTeacherReport(rows)
		if e != nil {
			return nil, e
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// TeacherAttendanceReportDetail memakai ID permanen dan kepemilikan historis.
func (s *Store) TeacherAttendanceReportDetail(ctx context.Context, actor, id int64, scope TeacherReportScope, historyPage int) (TeacherAttendanceReport, error) {
	if historyPage < 1 || historyPage > 100000 {
		return TeacherAttendanceReport{}, ValidationError("Halaman histori tidak valid.")
	}
	tx, e := s.teacherReportReader(ctx, actor, scope)
	if e != nil {
		return TeacherAttendanceReport{}, e
	}
	defer tx.Rollback()
	r, e := scanTeacherReport(tx.QueryRowContext(ctx, `SELECT `+teacherReportColumns+` FROM app.teacher_attendance_reports WHERE id=$1`, id))
	if e != nil {
		return r, e
	}
	var own bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teachers WHERE id=$1 AND user_id=$2)`, r.TeacherID, actor).Scan(&own); e != nil {
		return TeacherAttendanceReport{}, e
	}
	if scope != TeacherReportTU && !own {
		return TeacherAttendanceReport{}, ErrNotFound
	}
	if scope != TeacherReportOwn {
		accessErr := teacherAttendanceManagerAccess(ctx, tx, actor, r.TeacherID)
		if accessErr != nil && !errors.Is(accessErr, ErrForbidden) {
			return TeacherAttendanceReport{}, accessErr
		}
		r.CanManage = accessErr == nil && r.Active
	}
	rows, e := tx.QueryContext(ctx, `SELECT h.action,u.display_name,coalesce(h.reason,''),h.created_at,h.before_state,h.after_state FROM app.teacher_attendance_report_history h JOIN app.users u ON u.id=h.actor_id WHERE h.report_id=$1 ORDER BY h.id DESC LIMIT 11 OFFSET $2`, id, (historyPage-1)*10)
	if e != nil {
		return TeacherAttendanceReport{}, e
	}
	defer rows.Close()
	for rows.Next() {
		var h TeacherAttendanceReportHistory
		var before, after []byte
		if e = rows.Scan(&h.Action, &h.Actor, &h.Reason, &h.At, &before, &after); e != nil {
			return TeacherAttendanceReport{}, e
		}
		if len(before) > 0 {
			h.Before = &TeacherAttendanceReport{}
			if e = json.Unmarshal(before, h.Before); e != nil {
				return TeacherAttendanceReport{}, e
			}
		}
		if e = json.Unmarshal(after, &h.After); e != nil {
			return TeacherAttendanceReport{}, e
		}
		h.At = TeacherAttendanceWIBTime(h.At)
		r.History = append(r.History, h)
	}
	return r, rows.Err()
}

// RequestTeacherAttendanceReportCorrection memakai inbox TU yang sudah tersedia.
func (s *Store) RequestTeacherAttendanceReportCorrection(ctx context.Context, actor, id int64, text string) error {
	text, e := validateTeacherAttendanceCorrectionReason(text, "Keterangan permintaan wajib diisi, maksimal 500 karakter.")
	if e != nil {
		return e
	}
	tx, e := s.beginAny(ctx, actor, "guru_mapel", "wali_kelas")
	if e != nil {
		return e
	}
	defer tx.Rollback()
	teacher, _, e := activeTeacherForAttendance(ctx, tx, actor)
	if e != nil {
		return e
	}
	r, e := scanTeacherReport(tx.QueryRowContext(ctx, `SELECT `+teacherReportColumns+` FROM app.teacher_attendance_reports WHERE id=$1 AND teacher_id=$2`, id, teacher))
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO app.teacher_attendance_correction_requests(teacher_id,teacher_name,actor_id,on_date,attendance_action,request_kind,request_text,report_id) VALUES($1,$2,$3,$4::date,NULL,'correction',$5,$6)`, r.TeacherID, r.TeacherName, actor, r.StartsOn, text, r.ID)
	if e != nil {
		return e
	}
	return tx.Commit()
}
