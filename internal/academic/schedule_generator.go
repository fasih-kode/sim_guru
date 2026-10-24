package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

func queryPlanSnapshot(ctx context.Context, tx *sql.Tx, id int64) (*SchedulePlan, error) {
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT plan_snapshot FROM app.weekly_timetable_revisions WHERE id=$1`, id).Scan(&raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var p SchedulePlan
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
func configuredPlan(p SchedulePlan) bool {
	if len(p.Blocks) > 0 || len(p.Constraints) > 0 || len(p.ConstraintGrids) > 0 {
		return true
	}
	for _, r := range p.Rules {
		if !r.Missing {
			return true
		}
	}
	return false
}
func savePlanSnapshot(ctx context.Context, tx *sql.Tx, id int64, p SchedulePlan) error {
	if !configuredPlan(p) {
		return nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE app.weekly_timetable_revisions SET plan_snapshot=$1 WHERE id=$2 AND status='draft'`, string(data), id)
	return err
}
func validateRevisionPlan(ctx context.Context, tx *sql.Tx, revision, semester int64, date time.Time, entries []TimetableEntry, complete bool) error {
	snapshot, err := queryPlanSnapshot(ctx, tx, revision)
	if err != nil {
		return err
	}
	current, err := querySchedulePlan(ctx, tx, semester, date)
	if err != nil {
		return err
	}
	if snapshot == nil {
		if configuredPlan(current) {
			return ValidationError("Pengaturan struktur/waktu baru tersedia. Generate ulang draf agar memakai aturan terbaru.")
		}
		return nil
	}
	if snapshot.fingerprint() != current.fingerprint() {
		return ValidationError("Pengaturan atau pengampu berubah setelah draf dibuat. Generate ulang untuk meninjau aturan terbaru.")
	}
	return ValidatePlannedEntries(*snapshot, entries, complete)
}

// GenerateTimetable solves outside the mutation lock, then rechecks all inputs
// under that lock before committing the complete draft and history atomically.
func (s *Store) GenerateTimetable(ctx context.Context, actor, semester int64, date time.Time, expectedRevision int64, replace bool) (TimetableRevision, error) {
	if date.IsZero() || semester < 1 {
		return TimetableRevision{}, ValidationError("Pilih semester dan tanggal berlaku.")
	}
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return TimetableRevision{}, err
	}
	p, err := querySchedulePlan(ctx, tx, semester, date)
	if err != nil {
		tx.Rollback()
		return TimetableRevision{}, err
	}
	var draftID int64
	var draftDate time.Time
	err = tx.QueryRowContext(ctx, `SELECT id,effective_from FROM app.weekly_timetable_revisions WHERE semester_id=$1 AND status='draft'`, semester).Scan(&draftID, &draftDate)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		return TimetableRevision{}, err
	}
	if draftID != expectedRevision {
		tx.Rollback()
		return TimetableRevision{}, ErrConflict
	}
	var before []TimetableEntry
	if draftID > 0 {
		if !draftDate.Equal(date) {
			tx.Rollback()
			return TimetableRevision{}, ValidationError("Tanggal generate harus sama dengan tanggal draf yang sedang dibuka.")
		}
		before, err = queryTimetableEntries(ctx, tx, draftID, 0)
		if err != nil {
			tx.Rollback()
			return TimetableRevision{}, err
		}
		if len(before) > 0 && !replace {
			tx.Rollback()
			return TimetableRevision{}, ValidationError("Draf sudah berisi jadwal. Centang persetujuan penggantian isi draf sebelum generate ulang.")
		}
	}
	var preferred []TimetableEntry
	var published int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.weekly_timetable_revisions WHERE semester_id=$1 AND status='published' ORDER BY effective_from DESC LIMIT 1`, semester).Scan(&published)
	if err == nil {
		preferred, err = queryTimetableEntries(ctx, tx, published, 0)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		return TimetableRevision{}, err
	}
	tx.Rollback()
	entries, err := solveWithBudget(ctx, p, preferred)
	if err != nil {
		return TimetableRevision{}, err
	}
	tx, err = s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return TimetableRevision{}, err
	}
	defer tx.Rollback()
	current, err := querySchedulePlan(ctx, tx, semester, date)
	if err != nil {
		return TimetableRevision{}, err
	}
	if current.fingerprint() != p.fingerprint() {
		return TimetableRevision{}, ValidationError("Pengaturan berubah selama pencarian. Silakan generate kembali.")
	}
	var rev TimetableRevision
	if draftID == 0 {
		rev, err = createTimetableDraftTx(ctx, tx, actor, semester, date)
		if err != nil {
			return rev, err
		}
	} else {
		var id int64
		if err = tx.QueryRowContext(ctx, `SELECT id FROM app.weekly_timetable_revisions WHERE id=$1 AND semester_id=$2 AND status='draft' FOR UPDATE`, draftID, semester).Scan(&id); err != nil {
			return rev, ErrConflict
		}
		now, e := queryTimetableEntries(ctx, tx, draftID, 0)
		if e != nil {
			return rev, e
		}
		a, _ := json.Marshal(before)
		b, _ := json.Marshal(now)
		if string(a) != string(b) {
			return rev, ValidationError("Isi draf berubah selama pencarian. Muat ulang sebelum generate.")
		}
		rev = TimetableRevision{ID: draftID, SemesterID: semester, Status: "draft", EffectiveFrom: date}
		if len(before) > 0 {
			if _, err = tx.ExecContext(ctx, `DELETE FROM app.weekly_timetable_entries WHERE revision_id=$1`, draftID); err != nil {
				return rev, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,before_state,after_state) VALUES($1,$2,'entry_removed',$3,'{"reason":"generate ulang disetujui"}'::jsonb)`, draftID, actor, string(a)); err != nil {
				return rev, err
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM app.weekly_timetable_periods WHERE revision_id=$1`, draftID); err != nil {
			return rev, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_periods(revision_id,weekday,period_no,starts_at,ends_at) SELECT $1,weekday,period_no,starts_at,ends_at FROM app.lesson_periods WHERE semester_id=$2`, draftID, semester); err != nil {
			return rev, err
		}
	}
	if err = savePlanSnapshot(ctx, tx, rev.ID, p); err != nil {
		return rev, err
	}
	for _, e := range entries {
		err = tx.QueryRowContext(ctx, `INSERT INTO app.weekly_timetable_entries(revision_id,weekday,class_id,subject_id,teacher_id,assignment_id,period_start,period_end) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, rev.ID, e.Weekday, e.ClassID, e.SubjectID, e.TeacherID, e.AssignmentID, e.PeriodStart, e.PeriodEnd).Scan(&e.ID)
		if err != nil {
			return rev, err
		}
		after, err := json.Marshal(e)
		if err != nil {
			return rev, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,after_state) VALUES($1,$2,'entry_added',$3)`, rev.ID, actor, string(after)); err != nil {
			return rev, err
		}
	}
	return rev, tx.Commit()
}

// Nonteaching time is displayed explicitly, without creating fake subjects,
// teachers or teaching assignments. It never contributes to lesson JP counts.
func timetableTimeline(entries []TimetableEntry, blocks []SchoolTimeBlock) []TimetableEntry {
	out := append([]TimetableEntry(nil), entries...)
	for _, b := range blocks {
		out = append(out, TimetableEntry{Kind: b.Kind, Weekday: b.Weekday, PeriodStart: b.PeriodNo, PeriodEnd: b.PeriodNo, StartsAt: b.StartsAt, EndsAt: b.EndsAt, TimeSegments: shortClock(b.StartsAt) + "–" + shortClock(b.EndsAt), ClassName: "Semua rombel", SubjectName: b.Label, TeacherName: "—"})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Weekday != b.Weekday {
			return a.Weekday < b.Weekday
		}
		if a.StartsAt != b.StartsAt {
			return a.StartsAt < b.StartsAt
		}
		return a.ClassName < b.ClassName
	})
	return out
}
func shortClock(s string) string {
	if len(s) > 5 {
		return s[:5]
	}
	return s
}
func entrySegments(e TimetableEntry, periods []LessonPeriod) string {
	var segments []string
	start, end := "", ""
	for _, p := range periods {
		if p.Weekday != e.Weekday || p.Number < e.PeriodStart || p.Number > e.PeriodEnd {
			continue
		}
		if start != "" && clockMinute(p.StartsAt) != clockMinute(end) {
			segments = append(segments, shortClock(start)+"–"+shortClock(end))
			start = ""
		}
		if start == "" {
			start = p.StartsAt
		}
		end = p.EndsAt
	}
	if start != "" {
		segments = append(segments, shortClock(start)+"–"+shortClock(end))
	}
	return strings.Join(segments, " · jeda · ")
}
func (s *Store) DraftPlanSummary(ctx context.Context, actor, revision int64) ([]EffectiveCurriculum, string, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	var semester int64
	var date time.Time
	if err = tx.QueryRowContext(ctx, `SELECT semester_id,effective_from FROM app.weekly_timetable_revisions WHERE id=$1`, revision).Scan(&semester, &date); err != nil {
		return nil, "", err
	}
	snapshot, err := queryPlanSnapshot(ctx, tx, revision)
	if err != nil || snapshot == nil {
		return nil, "", err
	}
	entries, err := queryTimetableEntries(ctx, tx, revision, 0)
	if err != nil {
		return nil, "", err
	}
	rules := append([]EffectiveCurriculum(nil), snapshot.Rules...)
	for i := range rules {
		for _, e := range entries {
			if e.ClassID == rules[i].ClassID && e.SubjectID == rules[i].SubjectID {
				rules[i].ScheduledJP += e.PeriodEnd - e.PeriodStart + 1
			}
		}
	}
	var warning string
	if err = validateRevisionPlan(ctx, tx, revision, semester, date, entries, true); err != nil {
		var validation ValidationError
		if errors.As(err, &validation) {
			warning = string(validation)
		} else {
			return nil, "", err
		}
	}
	return rules, warning, nil
}
