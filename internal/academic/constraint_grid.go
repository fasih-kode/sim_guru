package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type ConstraintSlot struct {
	Weekday, Number int
	State           string
}
type ConstraintGrid struct {
	ID, SemesterID, TeacherID, SubjectID, Version int64
	TargetName                                    string
	Periods                                       []LessonPeriod
	Blocks                                        []SchoolTimeBlock
	Slots                                         []ConstraintSlot
}
type ConstraintCell struct {
	Weekday, Number                  int
	State, StartsAt, EndsAt, Blocked string
}

func (c ConstraintCell) Field() string { return fmt.Sprintf("cell_%d_%d", c.Weekday, c.Number) }
func (c ConstraintCell) Label() string {
	switch c.State {
	case "soft":
		return "Kondisional"
	case "hard":
		return "Tidak tersedia"
	default:
		return "Tersedia"
	}
}
func (c ConstraintCell) Symbol() string {
	switch c.State {
	case "soft":
		return "?"
	case "hard":
		return "×"
	default:
		return "✓"
	}
}

type ConstraintDay struct {
	Weekday int
	Cells   []ConstraintCell
}
type ConstraintEditor struct {
	TeacherID, SubjectID int64
	TargetName, Token    string
	Days                 []ConstraintDay
	Columns              []int
	Legacy, Review       bool
}

func sameTarget(teacher, subject, t, s int64) bool { return teacher == t && subject == s }
func gridClockFingerprint(periods []LessonPeriod, blocks []SchoolTimeBlock) string {
	normalized := append([]LessonPeriod(nil), periods...)
	for i := range normalized {
		normalized[i].ID = 0
		normalized[i].StartsAt = normalized[i].StartsAt[:5]
		normalized[i].EndsAt = normalized[i].EndsAt[:5]
	}
	b, _ := json.Marshal(struct {
		Periods []LessonPeriod
		Blocks  []SchoolTimeBlock
	}{normalized, blocks})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (g ConstraintGrid) NeedsReview(p SchedulePlan) bool {
	return gridClockFingerprint(g.Periods, g.Blocks) != gridClockFingerprint(p.Periods, p.Blocks)
}
func queryConstraintGrids(ctx context.Context, tx *sql.Tx, semester int64) ([]ConstraintGrid, error) {
	rows, err := tx.QueryContext(ctx, `SELECT g.id,g.semester_id,coalesce(g.teacher_id,0),coalesce(g.subject_id,0),g.version,coalesce(t.name,s.name),g.payload FROM app.schedule_constraint_grids g LEFT JOIN app.teachers t ON t.id=g.teacher_id LEFT JOIN app.subjects s ON s.id=g.subject_id WHERE g.semester_id=$1 AND g.deleted_at IS NULL ORDER BY g.id`, semester)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConstraintGrid
	for rows.Next() {
		var g ConstraintGrid
		var raw []byte
		if err = rows.Scan(&g.ID, &g.SemesterID, &g.TeacherID, &g.SubjectID, &g.Version, &g.TargetName, &raw); err != nil {
			return nil, err
		}
		var data ConstraintGrid
		if err = json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		g.Periods, g.Blocks, g.Slots = data.Periods, data.Blocks, data.Slots
		out = append(out, g)
	}
	return out, rows.Err()
}
func effectiveGridConstraints(legacy []ScheduleConstraint, grids []ConstraintGrid) []ScheduleConstraint {
	var out []ScheduleConstraint
	for _, c := range legacy {
		overridden := false
		for _, g := range grids {
			if sameTarget(c.TeacherID, c.SubjectID, g.TeacherID, g.SubjectID) {
				overridden = true
				break
			}
		}
		if !overridden {
			out = append(out, c)
		}
	}
	for _, g := range grids {
		for _, slot := range g.Slots {
			if slot.State == "available" {
				continue
			}
			for _, lp := range g.Periods {
				if lp.Weekday == slot.Weekday && lp.Number == slot.Number {
					out = append(out, ScheduleConstraint{ID: -g.ID, Version: g.Version, SemesterID: g.SemesterID, TeacherID: g.TeacherID, SubjectID: g.SubjectID, TargetName: g.TargetName, Severity: slot.State, Weekdays: []int{slot.Weekday}, PeriodNo: slot.Number, StartsAt: lp.StartsAt[:5], EndsAt: lp.EndsAt[:5]})
					break
				}
			}
		}
	}
	return out
}
func constraintEditorToken(p SchedulePlan, teacher, subject int64) string {
	var constraints []ScheduleConstraint
	var grids []ConstraintGrid
	for _, c := range p.Constraints {
		if sameTarget(teacher, subject, c.TeacherID, c.SubjectID) {
			constraints = append(constraints, c)
		}
	}
	for _, g := range p.ConstraintGrids {
		if sameTarget(teacher, subject, g.TeacherID, g.SubjectID) {
			grids = append(grids, g)
		}
	}
	raw, _ := json.Marshal(struct {
		Clock       string
		Constraints []ScheduleConstraint
		Grids       []ConstraintGrid
	}{gridClockFingerprint(p.Periods, p.Blocks), constraints, grids})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func BuildConstraintEditor(p SchedulePlan, teacher, subject int64, name string) ConstraintEditor {
	e := ConstraintEditor{TeacherID: teacher, SubjectID: subject, TargetName: name, Token: constraintEditorToken(p, teacher, subject)}
	var stored *ConstraintGrid
	for i := range p.ConstraintGrids {
		g := &p.ConstraintGrids[i]
		if sameTarget(teacher, subject, g.TeacherID, g.SubjectID) {
			stored = g
			e.Review = g.NeedsReview(p)
			break
		}
	}
	max := 12
	days := 6
	for _, lp := range p.Periods {
		if lp.Number > max {
			max = lp.Number
		}
		if lp.Weekday > days {
			days = lp.Weekday
		}
	}
	for n := 1; n <= max; n++ {
		e.Columns = append(e.Columns, n)
	}
	for d := 1; d <= days; d++ {
		day := ConstraintDay{Weekday: d}
		for n := 1; n <= max; n++ {
			c := ConstraintCell{Weekday: d, Number: n, State: "available", Blocked: "Belum diatur"}
			for _, lp := range p.Periods {
				if lp.Weekday != d || lp.Number != n {
					continue
				}
				c.StartsAt, c.EndsAt, c.Blocked = lp.StartsAt[:5], lp.EndsAt[:5], ""
				for _, b := range p.Blocks {
					if periodBlocked(lp, []SchoolTimeBlock{b}) {
						c.Blocked = b.Label
						break
					}
				}
				if stored != nil {
					for _, slot := range stored.Slots {
						if slot.Weekday == d && slot.Number == n {
							c.State = slot.State
							break
						}
					}
				} else {
					for _, rule := range p.Constraints {
						if sameTarget(teacher, subject, rule.TeacherID, rule.SubjectID) {
							e.Legacy = true
							if rule.matches(teacher, subject, lp) && (c.State != "hard" || rule.Severity == "hard") {
								c.State = rule.Severity
							}
						}
					}
				}
				break
			}
			day.Cells = append(day.Cells, c)
		}
		e.Days = append(e.Days, day)
	}
	// Legacy rules can exist even if none of their days has configured periods.
	if stored == nil {
		for _, c := range p.Constraints {
			if sameTarget(teacher, subject, c.TeacherID, c.SubjectID) {
				e.Legacy = true
			}
		}
	}
	return e
}
func validateGridSubmission(editor ConstraintEditor, states map[string]string) ([]ConstraintSlot, error) {
	var slots []ConstraintSlot
	allowed := map[string]bool{}
	for _, day := range editor.Days {
		for _, c := range day.Cells {
			if c.Blocked != "" {
				continue
			}
			key := c.Field()
			allowed[key] = true
			state, ok := states[key]
			if !ok || state != "available" && state != "soft" && state != "hard" {
				return nil, ValidationError("Setiap jam pembelajaran harus memiliki status yang valid.")
			}
			slots = append(slots, ConstraintSlot{Weekday: c.Weekday, Number: c.Number, State: state})
		}
	}
	for key := range states {
		if !allowed[key] {
			return nil, ValidationError("Jam belum diatur atau merupakan kegiatan sekolah; tidak dapat diubah.")
		}
	}
	if len(slots) == 0 {
		return nil, ValidationError("Atur Jam Pelajaran terlebih dahulu.")
	}
	return slots, nil
}
func (s *Store) SaveConstraintGrid(ctx context.Context, actor, semester, teacher, subject int64, token string, states map[string]string, reviewed bool) error {
	if semester < 1 || teacher < 0 || subject < 0 || (teacher == 0) == (subject == 0) {
		return ValidationError("Pilih satu guru atau mata pelajaran.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name string
	err = tx.QueryRowContext(ctx, `SELECT name FROM app.teachers t WHERE t.id=$1 AND t.active AND EXISTS(SELECT 1 FROM app.users u JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' WHERE u.id=t.user_id AND u.active) UNION ALL SELECT name FROM app.subjects WHERE id=$2 AND active`, teacher, subject).Scan(&name)
	if err == sql.ErrNoRows {
		return ValidationError("Guru/akun Guru Mapel atau mata pelajaran harus aktif.")
	}
	if err != nil {
		return err
	}
	p, err := querySchedulePlan(ctx, tx, semester, time.Now())
	if err != nil {
		return err
	}
	editor := BuildConstraintEditor(p, teacher, subject, name)
	if token != editor.Token {
		return ErrConflict
	}
	if (editor.Legacy || editor.Review) && !reviewed {
		return ValidationError("Tinjau pemetaan jam dan centang persetujuan sebelum menyimpan.")
	}
	slots, err := validateGridSubmission(editor, states)
	if err != nil {
		return err
	}
	var before any
	g := ConstraintGrid{SemesterID: semester, TeacherID: teacher, SubjectID: subject, TargetName: name, Periods: p.Periods, Blocks: p.Blocks, Slots: slots}
	for _, old := range p.ConstraintGrids {
		if sameTarget(teacher, subject, old.TeacherID, old.SubjectID) {
			g.ID, g.Version = old.ID, old.Version
			before = old
			break
		}
	}
	payload, err := json.Marshal(g)
	if err != nil {
		return err
	}
	if g.ID == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO app.schedule_constraint_grids(semester_id,teacher_id,subject_id,payload,updated_by) VALUES($1,nullif($2::bigint,0),nullif($3::bigint,0),$4,$5) RETURNING id,version`, semester, teacher, subject, string(payload), actor).Scan(&g.ID, &g.Version)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE app.schedule_constraint_grids SET payload=$1,version=version+1,updated_by=$2,updated_at=now() WHERE id=$3 AND version=$4 RETURNING version`, string(payload), actor, g.ID, g.Version).Scan(&g.Version)
	}
	if err != nil {
		return dbError(err)
	}
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.schedule_constraint_grid_history(grid_id,actor_id,before_state,after_state) VALUES($1,$2,$3,$4)`, g.ID, actor, string(b), string(a))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RemoveConstraintGrid(ctx context.Context, actor, semester, id, version int64) error {
	if semester < 1 || id < 1 || version < 1 {
		return ValidationError("Kisi yang akan dihapus tidak valid.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()

	grids, err := queryConstraintGrids(ctx, tx, semester)
	if err != nil {
		return err
	}
	var grid *ConstraintGrid
	for i := range grids {
		if grids[i].ID == id {
			grid = &grids[i]
			break
		}
	}
	if grid == nil {
		return ErrNotFound
	}
	if grid.Version != version {
		return ErrConflict
	}

	// Legacy constraints replaced by this grid must not become active again
	// after deletion. Remove them atomically while retaining their audit snapshots.
	legacy, err := queryScheduleConstraints(ctx, tx, semester)
	if err != nil {
		return err
	}
	for _, constraint := range legacy {
		if !sameTarget(grid.TeacherID, grid.SubjectID, constraint.TeacherID, constraint.SubjectID) {
			continue
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM app.schedule_constraints WHERE id=$1 AND semester_id=$2`, constraint.ID, semester); err != nil {
			return err
		}
		if err = constraintHistory(ctx, tx, actor, constraint, constraint, nil); err != nil {
			return err
		}
	}

	var nextVersion int64
	err = tx.QueryRowContext(ctx, `UPDATE app.schedule_constraint_grids SET deleted_at=now(),version=version+1,updated_by=$1,updated_at=now() WHERE id=$2 AND semester_id=$3 AND version=$4 AND deleted_at IS NULL RETURNING version`, actor, id, semester, version).Scan(&nextVersion)
	if err == sql.ErrNoRows {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	before, err := json.Marshal(grid)
	if err != nil {
		return err
	}
	after, err := json.Marshal(map[string]any{"deleted": true, "version": nextVersion})
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.schedule_constraint_grid_history(grid_id,actor_id,before_state,after_state) VALUES($1,$2,$3,$4)`, id, actor, string(before), string(after)); err != nil {
		return err
	}
	return tx.Commit()
}

// ApplyGridForm keeps submitted states visible after validation errors.
func (e *ConstraintEditor) ApplyGridForm(states map[string]string) {
	for i := range e.Days {
		for j := range e.Days[i].Cells {
			c := &e.Days[i].Cells[j]
			if c.Blocked == "" {
				state := states[c.Field()]
				if state == "available" || state == "soft" || state == "hard" {
					c.State = state
				}
			}
		}
	}
}

func (d ConstraintDay) Enabled() bool {
	for _, cell := range d.Cells {
		if cell.Blocked == "" {
			return true
		}
	}
	return false
}
