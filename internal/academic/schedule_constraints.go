package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// A subject constraint applies to that subject in every class in the semester.
// Weekdays and clock ranges are explicit; no subject/teacher policy is inferred.
type ScheduleConstraint struct {
	ID, SemesterID, TeacherID, SubjectID, Version int64
	TargetName, Severity, StartsAt, EndsAt        string
	Weekdays                                      []int
	PeriodNo                                      int `json:",omitempty"`
	AllDay                                        bool
}

func (c ScheduleConstraint) HasDay(day int) bool {
	for _, d := range c.Weekdays {
		if d == day {
			return true
		}
	}
	return false
}
func (c ScheduleConstraint) DayNames() string {
	names := []string{"", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}
	var out []string
	for _, d := range c.Weekdays {
		if d >= 1 && d <= 7 {
			out = append(out, names[d])
		}
	}
	return strings.Join(out, ", ")
}
func (c ScheduleConstraint) TimeLabel() string {
	if c.PeriodNo > 0 {
		return fmt.Sprintf("Jam ke-%d · %s–%s", c.PeriodNo, c.StartsAt, c.EndsAt)
	}
	if c.AllDay {
		return "Sepanjang hari"
	}
	return c.StartsAt + "–" + c.EndsAt
}
func (c ScheduleConstraint) matches(teacher, subject int64, lp LessonPeriod) bool {
	if !c.HasDay(lp.Weekday) || !((c.TeacherID > 0 && c.TeacherID == teacher) || (c.SubjectID > 0 && c.SubjectID == subject)) {
		return false
	}
	if c.PeriodNo > 0 {
		return lp.Number == c.PeriodNo
	}
	return c.AllDay || clockMinute(lp.StartsAt) < clockMinute(c.EndsAt) && clockMinute(lp.EndsAt) > clockMinute(c.StartsAt)
}
func hardUnavailable(p SchedulePlan, teacher, subject int64, lp LessonPeriod) bool {
	for _, c := range p.Constraints {
		if c.Severity == "hard" && c.matches(teacher, subject, lp) {
			return true
		}
	}
	return false
}
func validateScheduleConstraint(c ScheduleConstraint) error {
	if c.PeriodNo != 0 || c.SemesterID < 1 || c.ID < 0 || c.TeacherID < 0 || c.SubjectID < 0 || (c.TeacherID == 0) == (c.SubjectID == 0) || (c.Severity != "hard" && c.Severity != "soft") || len(c.Weekdays) < 1 || len(c.Weekdays) > 7 {
		return ValidationError("Pilih semester, satu guru atau mapel, hari, dan jenis batasan.")
	}
	seen := map[int]bool{}
	for _, d := range c.Weekdays {
		if d < 1 || d > 7 || seen[d] {
			return ValidationError("Pilih hari yang valid tanpa pengulangan.")
		}
		seen[d] = true
	}
	if !c.AllDay {
		start, e1 := time.Parse("15:04", c.StartsAt)
		end, e2 := time.Parse("15:04", c.EndsAt)
		if e1 != nil || e2 != nil || !start.Before(end) {
			return ValidationError("Waktu mulai dan selesai harus valid; mulai sebelum selesai.")
		}
	}
	return nil
}
func queryScheduleConstraints(ctx context.Context, tx *sql.Tx, semester int64) ([]ScheduleConstraint, error) {
	rows, err := tx.QueryContext(ctx, `SELECT c.id,c.semester_id,coalesce(c.teacher_id,0),coalesce(c.subject_id,0),c.version,coalesce(t.name,s.name),c.severity,array_to_json(c.weekdays)::text,c.all_day,coalesce(to_char(c.starts_at,'HH24:MI'),''),coalesce(to_char(c.ends_at,'HH24:MI'),'') FROM app.schedule_constraints c LEFT JOIN app.teachers t ON t.id=c.teacher_id LEFT JOIN app.subjects s ON s.id=c.subject_id WHERE c.semester_id=$1 ORDER BY c.id`, semester)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduleConstraint
	for rows.Next() {
		var c ScheduleConstraint
		var days string
		if err = rows.Scan(&c.ID, &c.SemesterID, &c.TeacherID, &c.SubjectID, &c.Version, &c.TargetName, &c.Severity, &days, &c.AllDay, &c.StartsAt, &c.EndsAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(days), &c.Weekdays); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) ScheduleConstraints(ctx context.Context, actor, semester int64) ([]ScheduleConstraint, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return queryScheduleConstraints(ctx, tx, semester)
}
func constraintHistory(ctx context.Context, tx *sql.Tx, actor int64, c ScheduleConstraint, before, after any) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.schedule_constraint_history(semester_id,actor_id,constraint_id,before_state,after_state) VALUES($1,$2,$3,$4,$5)`, c.SemesterID, actor, c.ID, string(b), string(a))
	return err
}
func (s *Store) SaveScheduleConstraint(ctx context.Context, actor int64, c ScheduleConstraint) error {
	if err := validateScheduleConstraint(c); err != nil {
		return err
	}
	sort.Ints(c.Weekdays)
	if c.AllDay {
		c.StartsAt = ""
		c.EndsAt = ""
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var gridExists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.schedule_constraint_grids WHERE semester_id=$1 AND coalesce(teacher_id,0)=$2 AND coalesce(subject_id,0)=$3 AND deleted_at IS NULL)`, c.SemesterID, c.TeacherID, c.SubjectID).Scan(&gridExists); err != nil {
		return err
	}
	if gridExists {
		return ValidationError("Sasaran ini sudah memakai kisi. Ubah melalui pilihan jam pada Batasan Jadwal.")
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT CASE WHEN $1::bigint>0 THEN EXISTS(SELECT 1 FROM app.teachers t JOIN app.users u ON u.id=t.user_id AND u.active JOIN app.user_roles ur ON ur.user_id=u.id AND ur.role_code='guru_mapel' WHERE t.id=$1 AND t.active) ELSE EXISTS(SELECT 1 FROM app.subjects WHERE id=$2 AND active) END`, c.TeacherID, c.SubjectID).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ValidationError("Guru harus aktif, terhubung akun Guru Mapel aktif; mapel harus aktif.")
	}
	var before any
	if c.ID > 0 {
		current, e := queryScheduleConstraints(ctx, tx, c.SemesterID)
		if e != nil {
			return e
		}
		found := false
		for _, old := range current {
			if old.ID == c.ID {
				if old.Version != c.Version {
					return ErrConflict
				}
				before = old
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
	}
	days, _ := json.Marshal(c.Weekdays)
	// JSON-to-array conversion keeps the driver-independent parameter representation.
	args := []any{c.SemesterID, c.TeacherID, c.SubjectID, c.Severity, string(days), c.AllDay, c.StartsAt, c.EndsAt, actor}
	const values = `$1,nullif($2::bigint,0),nullif($3::bigint,0),$4,ARRAY(SELECT value::smallint FROM jsonb_array_elements_text($5::jsonb)),$6,nullif($7,'')::time,nullif($8,'')::time,$9`
	if c.ID == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO app.schedule_constraints(semester_id,teacher_id,subject_id,severity,weekdays,all_day,starts_at,ends_at,updated_by) VALUES(`+values+`) RETURNING id,version`, args...).Scan(&c.ID, &c.Version)
	} else {
		args = append(args, c.ID, c.Version)
		err = tx.QueryRowContext(ctx, `UPDATE app.schedule_constraints SET (semester_id,teacher_id,subject_id,severity,weekdays,all_day,starts_at,ends_at,updated_by)=(`+values+`),version=version+1,updated_at=now() WHERE id=$10 AND semester_id=$1 AND version=$11 RETURNING version`, args...).Scan(&c.Version)
		if err == sql.ErrNoRows {
			return ErrConflict
		}
	}
	if err != nil {
		return dbError(err)
	}
	current, err := queryScheduleConstraints(ctx, tx, c.SemesterID)
	if err != nil {
		return err
	}
	for _, saved := range current {
		if saved.ID == c.ID {
			c = saved
			break
		}
	}
	if err = constraintHistory(ctx, tx, actor, c, before, c); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) RemoveScheduleConstraint(ctx context.Context, actor, semester, id, version int64) error {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := queryScheduleConstraints(ctx, tx, semester)
	if err != nil {
		return err
	}
	for _, c := range current {
		if c.ID == id {
			var gridExists bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.schedule_constraint_grids WHERE semester_id=$1 AND coalesce(teacher_id,0)=$2 AND coalesce(subject_id,0)=$3 AND deleted_at IS NULL)`, semester, c.TeacherID, c.SubjectID).Scan(&gridExists); err != nil {
				return err
			}
			if gridExists {
				return ValidationError("Batasan waktu lama sudah digantikan kisi; catatan lama dipertahankan.")
			}
			if c.Version != version {
				return ErrConflict
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM app.schedule_constraints WHERE id=$1`, id); err != nil {
				return err
			}
			if err = constraintHistory(ctx, tx, actor, c, c, nil); err != nil {
				return err
			}
			return tx.Commit()
		}
	}
	return ErrNotFound
}

type PreferenceViolation struct {
	TeacherID                                            int64
	TeacherName                                          string
	TargetName, ClassName, SubjectName, StartsAt, EndsAt string
	Weekday, JP                                          int
}

// Report is derived from the immutable revision snapshot, not today's settings.
// Each teaching period appears once even when multiple preferences overlap.
func PreferenceViolations(p SchedulePlan, entries []TimetableEntry) []PreferenceViolation {
	var out []PreferenceViolation
	names := map[[2]int64]EffectiveCurriculum{}
	for _, r := range p.Rules {
		names[[2]int64{r.ClassID, r.SubjectID}] = r
	}
	for _, e := range entries {
		className, subjectName, teacherName := e.ClassName, e.SubjectName, e.TeacherName
		if r, ok := names[[2]int64{e.ClassID, e.SubjectID}]; ok {
			className, subjectName, teacherName = r.ClassName, r.SubjectName, r.TeacherName
		}
		for _, lp := range p.Periods {
			if e.Kind != "" || lp.Weekday != e.Weekday || lp.Number < e.PeriodStart || lp.Number > e.PeriodEnd {
				continue
			}
			var targets []string
			seen := map[string]bool{}
			teacherHit := false
			for _, c := range p.Constraints {
				if c.Severity == "soft" && c.matches(e.TeacherID, e.SubjectID, lp) {
					if c.TeacherID > 0 {
						teacherHit = true
					}
					name := c.TargetName
					if !seen[name] {
						targets = append(targets, name)
						seen[name] = true
					}
				}
			}
			if len(targets) > 0 {
				teacherID := int64(0)
				if teacherHit {
					teacherID = e.TeacherID
				}
				out = append(out, PreferenceViolation{TeacherID: teacherID, TeacherName: teacherName, TargetName: strings.Join(targets, ", "), ClassName: className, SubjectName: subjectName, Weekday: lp.Weekday, StartsAt: lp.StartsAt[:5], EndsAt: lp.EndsAt[:5], JP: 1})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weekday != out[j].Weekday {
			return out[i].Weekday < out[j].Weekday
		}
		return out[i].StartsAt < out[j].StartsAt
	})
	return out
}
func (s *Store) RevisionPreferenceReport(ctx context.Context, actor, revision int64) ([]PreferenceViolation, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	p, err := queryPlanSnapshot(ctx, tx, revision)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	entries, err := queryTimetableEntries(ctx, tx, revision, 0)
	if err != nil {
		return nil, err
	}
	return PreferenceViolations(*p, entries), nil
}

func mandatoryCapacity(p SchedulePlan) error {
	for _, r := range p.Rules {
		slots := 0
		for _, lp := range p.Periods {
			if !periodBlocked(lp, p.Blocks) && !hardUnavailable(p, r.TeacherID, r.SubjectID, lp) {
				slots++
			}
		}
		if r.WeeklyJP > slots {
			return ValidationError(fmt.Sprintf("%s · %s membutuhkan %d JP, tetapi hanya %d JP tersedia setelah batasan wajib.", r.ClassName, r.SubjectName, r.WeeklyJP, slots))
		}
	}
	for _, c := range TeacherCapacities(p) {
		if c.Load > c.Available {
			return ValidationError(fmt.Sprintf("Beban %s %d JP, tetapi hanya %d JP tersedia setelah batasan wajib. Sesuaikan batasan wajib atau penugasan.", c.Name, c.Load, c.Available))
		}
	}
	return nil
}

type preferenceScore struct{ days, jp int }

func (a preferenceScore) less(b preferenceScore) bool {
	return a.days < b.days || a.days == b.days && a.jp < b.jp
}
func placementPreferenceScore(p SchedulePlan, tasks []scheduleTask, placed []int) preferenceScore {
	score := preferenceScore{}
	if len(p.Constraints) == 0 {
		return score
	}
	days := map[[2]int64]bool{}
	for i, ci := range placed {
		if ci < 0 {
			continue
		}
		c := tasks[i].candidates[ci]
		score.jp += c.softJP
		if c.softTeacher {
			days[[2]int64{p.Rules[tasks[i].rule].TeacherID, int64(c.day)}] = true
		}
	}
	score.days = len(days)
	return score
}

// Available is a necessary upper bound: a slot is available if at least one of
// the teacher's assigned subjects can use it. It does not prove schedulability.
type TeacherCapacity struct {
	TeacherID       int64
	Name            string
	Load, Available int
}

func TeacherCapacities(p SchedulePlan) []TeacherCapacity {
	teachers := map[int64]*TeacherCapacity{}
	for _, r := range p.Rules {
		if r.TeacherID > 0 && r.WeeklyJP > 0 {
			if teachers[r.TeacherID] == nil {
				teachers[r.TeacherID] = &TeacherCapacity{TeacherID: r.TeacherID, Name: r.TeacherName}
			}
			teachers[r.TeacherID].Load += r.WeeklyJP
		}
	}
	var out []TeacherCapacity
	for id, c := range teachers {
		for _, lp := range p.Periods {
			if periodBlocked(lp, p.Blocks) {
				continue
			}
			for _, r := range p.Rules {
				if r.TeacherID == id && r.WeeklyJP > 0 && !hardUnavailable(p, id, r.SubjectID, lp) {
					c.Available++
					break
				}
			}
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].TeacherID < out[j].TeacherID
	})
	return out
}

type TeacherPreferenceSummary struct {
	TeacherID int64
	Name      string
	Days, JP  int
}

func SummarizeTeacherPreferences(report []PreferenceViolation) []TeacherPreferenceSummary {
	teachers := map[int64]*TeacherPreferenceSummary{}
	days := map[[2]int64]bool{}
	for _, v := range report {
		if v.TeacherID == 0 {
			continue
		}
		if teachers[v.TeacherID] == nil {
			teachers[v.TeacherID] = &TeacherPreferenceSummary{TeacherID: v.TeacherID, Name: v.TeacherName}
		}
		c := teachers[v.TeacherID]
		c.JP += v.JP
		key := [2]int64{v.TeacherID, int64(v.Weekday)}
		if !days[key] {
			c.Days++
			days[key] = true
		}
	}
	var out []TeacherPreferenceSummary
	for _, c := range teachers {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TeacherID < out[j].TeacherID })
	return out
}
