package academic

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type StudentCard struct {
	StudentID, CardID           int64
	Name, NIS, ClassName, Token string
	History                     []History
}

func currentCardStudent(ctx context.Context, tx *sql.Tx, student int64) (StudentCard, error) {
	var c StudentCard
	e := tx.QueryRowContext(ctx, `SELECT s.id,s.name,coalesce(s.nis,''),c.grade::text||c.name,coalesce(q.id,0),coalesce(q.token,'') FROM app.students s JOIN app.current_student_placements p ON p.student_id=s.id JOIN app.school_years y ON y.id=p.school_year_id AND y.active JOIN app.classes c ON c.id=p.class_id LEFT JOIN app.student_qr_cards q ON q.student_id=s.id AND q.revoked_at IS NULL WHERE s.id=$1 AND s.active`, student).Scan(&c.StudentID, &c.Name, &c.NIS, &c.ClassName, &c.CardID, &c.Token)
	if errors.Is(e, sql.ErrNoRows) {
		return c, ValidationError("Siswa tidak memiliki penempatan aktif di tahun ajaran aktif.")
	}
	return c, e
}
func (s *Store) StudentCards(ctx context.Context, actor int64) ([]StudentCard, error) {
	tx, e := s.reader(ctx, actor, "petugas_absensi")
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT s.id,s.name,coalesce(s.nis,''),c.grade::text||c.name,coalesce(q.id,0) FROM app.students s JOIN app.current_student_placements p ON p.student_id=s.id JOIN app.school_years y ON y.id=p.school_year_id AND y.active JOIN app.classes c ON c.id=p.class_id LEFT JOIN app.student_qr_cards q ON q.student_id=s.id AND q.revoked_at IS NULL WHERE s.active ORDER BY c.grade,c.name,s.name,s.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []StudentCard
	for rows.Next() {
		var c StudentCard
		if e = rows.Scan(&c.StudentID, &c.Name, &c.NIS, &c.ClassName, &c.CardID); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) StudentCard(ctx context.Context, actor, student int64) (StudentCard, error) {
	tx, e := s.reader(ctx, actor, "petugas_absensi")
	if e != nil {
		return StudentCard{}, e
	}
	defer tx.Rollback()
	c, e := currentCardStudent(ctx, tx, student)
	if e != nil {
		return c, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT h.id,u.display_name,h.action,h.reason,h.created_at FROM app.student_qr_card_history h JOIN app.users u ON u.id=h.actor_id WHERE h.student_id=$1 ORDER BY h.id DESC LIMIT 100`, student)
	if e != nil {
		return c, e
	}
	defer rows.Close()
	for rows.Next() {
		var h History
		if e = rows.Scan(&h.ID, &h.Actor, &h.EntityType, &h.Reason, &h.At); e != nil {
			return c, e
		}
		h.At = h.At.In(attendanceWIB)
		c.History = append(c.History, h)
	}
	return c, rows.Err()
}
func (s *Store) ManageStudentCard(ctx context.Context, actor, student, expected int64, action, reason string) (StudentCard, error) {
	if action != "issue" && action != "replace" && action != "reprint" {
		return StudentCard{}, ValidationError("Aksi kartu tidak valid.")
	}
	if e := foundationReason(reason); e != nil {
		return StudentCard{}, e
	}
	tx, e := s.begin(ctx, actor, "petugas_absensi")
	if e != nil {
		return StudentCard{}, e
	}
	defer tx.Rollback()
	c, e := currentCardStudent(ctx, tx, student)
	if e != nil {
		return c, e
	}
	if c.CardID != expected {
		return c, ErrConflict
	}
	if action == "issue" && c.CardID != 0 {
		return c, ErrConflict
	}
	if action != "issue" && c.CardID == 0 {
		return c, ValidationError("Terbitkan kartu terlebih dahulu.")
	}
	previous := c.CardID
	if action != "reprint" {
		if previous > 0 {
			if _, e = tx.ExecContext(ctx, `UPDATE app.student_qr_cards SET revoked_at=clock_timestamp() WHERE id=$1`, previous); e != nil {
				return c, e
			}
		}
		secret := make([]byte, 32)
		if _, e = rand.Read(secret); e != nil {
			return c, e
		}
		c.Token = "SGQR1:" + hex.EncodeToString(secret)
		e = tx.QueryRowContext(ctx, `INSERT INTO app.student_qr_cards(student_id,token,issued_by) VALUES($1,$2,$3) RETURNING id`, student, c.Token, actor).Scan(&c.CardID)
		if e != nil {
			return c, e
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO app.student_qr_card_history(student_id,card_id,previous_card_id,action,actor_id,reason) VALUES($1,$2,nullif($3,0),$4,$5,$6)`, student, c.CardID, previous, action, actor, strings.TrimSpace(reason))
	if e != nil {
		return c, e
	}
	return c, tx.Commit()
}

type DailyScanResult struct {
	Student   DailyStudent
	Duplicate bool
}

func validStudentQR(token string) bool {
	if len(token) != 70 || !strings.HasPrefix(token, "SGQR1:") {
		return false
	}
	_, e := hex.DecodeString(token[6:])
	return e == nil
}
func (s *Store) ScanStudentQR(ctx context.Context, actor int64, token string) (DailyScanResult, error) {
	var result DailyScanResult
	if !validStudentQR(token) {
		return result, ValidationError("QR bukan kartu siswa yang valid.")
	}
	tx, e := s.begin(ctx, actor, "petugas_absensi")
	if e != nil {
		return result, e
	}
	defer tx.Rollback()
	var student int64
	e = tx.QueryRowContext(ctx, `SELECT q.student_id FROM app.student_qr_cards q JOIN app.students s ON s.id=q.student_id WHERE q.token=$1 AND q.revoked_at IS NULL AND s.active`, token).Scan(&student)
	if errors.Is(e, sql.ErrNoRows) {
		return result, ValidationError("Kartu tidak berlaku. Periksa apakah kartu sudah diganti atau siswa tidak aktif.")
	}
	if e != nil {
		return result, e
	}
	var now time.Time
	var date string
	e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now)
	if e != nil {
		return result, e
	}
	date = now.In(attendanceWIB).Format("2006-01-02")
	var class int64
	e = tx.QueryRowContext(ctx, `SELECT app.student_placement_class_at(p.id,$2::date) FROM app.student_placements p JOIN app.school_years y ON y.id=p.school_year_id AND y.active WHERE p.student_id=$1 AND p.starts_on<=$2::date AND app.student_placement_class_at(p.id,$2::date) IS NOT NULL`, student, date).Scan(&class)
	if errors.Is(e, sql.ErrNoRows) {
		return result, ValidationError("Siswa belum memiliki penempatan bertanggal yang berlaku hari ini.")
	}
	if e != nil {
		return result, e
	}
	sheet, e := dailySheet(ctx, tx, actor, class, date)
	if e != nil {
		return result, e
	}
	if sheet.BlockReason != "" {
		return result, ValidationError(sheet.BlockReason)
	}
	if !sheet.SchoolDay {
		return result, ValidationError("Hari ini bukan hari sekolah.")
	}
	var d DailyStudent
	found := false
	for _, x := range sheet.Students {
		if x.StudentID == student {
			d = x
			found = true
			break
		}
	}
	if !found {
		return result, ValidationError("Siswa tidak termasuk roster hari ini atau sudah memiliki catatan di kelas lain. Periksa absensi harian.")
	}
	if d.ID != 0 {
		if d.Status != "hadir" {
			return result, ValidationError("Sudah tercatat " + d.StatusLabel() + ". Scan tidak menimpanya; periksa dan lakukan Koreksi di absensi harian.")
		}
		result.Student = d
		result.Duplicate = true
		return result, nil
	}
	semester, _, e := dailyCalendar(ctx, tx, sheet.Class.SchoolYearID, date)
	if e != nil {
		return result, e
	}
	d.SemesterID = semester
	d.Status = "hadir"
	d.Source = "qr"
	d.ArrivedAt = sql.NullTime{Time: now, Valid: true}
	d.Arrival = now.In(attendanceWIB).Format("15:04")
	d.Policy, e = arrivalPolicy(ctx, tx, semester, class, date)
	if e != nil {
		return result, e
	}
	assessArrival(&d)
	policyID, policy, e := policyJSON(d.Policy)
	if e != nil {
		return result, e
	}
	e = tx.QueryRowContext(ctx, `INSERT INTO app.student_daily_attendance(student_id,placement_id,class_id,semester_id,on_date,student_name,nis,class_name,status,arrived_at,source,actor_id,arrival_rule_id,arrival_policy,late_minutes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'hadir',$9,'qr',$10,$11,$12,$13) RETURNING id,version`, student, d.PlacementID, class, semester, date, d.Name, d.NIS, d.ClassName, now, actor, policyID, policy, d.LateMinutes).Scan(&d.ID, &d.Version)
	if e != nil {
		return result, e
	}
	if e = dailyHistory(ctx, tx, actor, d, nil, "Kedatangan melalui kartu QR"); e != nil {
		return result, e
	}
	result.Student = d
	return result, tx.Commit()
}
