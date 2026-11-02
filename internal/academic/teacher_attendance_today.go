package academic

import (
	"context"
	"database/sql"
	"time"
)

type TeacherAttendanceSummary struct {
	Action                 TeacherAttendanceAction
	Source                 string
	OccurredAt, RecordedAt time.Time
}

type TeacherAttendanceDay struct {
	TeacherName, Date string
	Events            []TeacherAttendanceSummary
}

// TeacherAttendanceToday membaca identitas serta tanggal dari server, tanpa parameter guru bebas.
func (s *Store) TeacherAttendanceToday(ctx context.Context, actorID int64) (TeacherAttendanceDay, error) {
	var day TeacherAttendanceDay
	if actorID < 1 {
		return day, ErrForbidden
	}
	// Helper otorisasi memakai row lock; transaksi ini tidak menulis data apa pun.
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return day, err
	}
	defer tx.Rollback()
	id, name, err := activeTeacherForAttendance(ctx, tx, actorID)
	if err != nil {
		return day, err
	}
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return day, err
	}
	day.TeacherName, day.Date = name, TeacherAttendanceWIBDate(now)
	rows, err := tx.QueryContext(ctx, `SELECT action,source,occurred_at,recorded_at FROM app.teacher_attendance_events WHERE teacher_id=$1 AND on_date=$2::date ORDER BY action`, id, day.Date)
	if err != nil {
		return day, err
	}
	defer rows.Close()
	for rows.Next() {
		var event TeacherAttendanceSummary
		if err = rows.Scan(&event.Action, &event.Source, &event.OccurredAt, &event.RecordedAt); err != nil {
			return day, err
		}
		event.OccurredAt = TeacherAttendanceWIBTime(event.OccurredAt)
		event.RecordedAt = TeacherAttendanceWIBTime(event.RecordedAt)
		day.Events = append(day.Events, event)
	}
	return day, rows.Err()
}
