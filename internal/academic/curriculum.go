package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func configurationHistory(ctx context.Context, tx *sql.Tx, actor, semester int64, entity string, id int64, before, after any) error {
	var b, a any
	if before != nil {
		v, e := json.Marshal(before)
		if e != nil {
			return e
		}
		b = string(v)
	}
	if after != nil {
		v, e := json.Marshal(after)
		if e != nil {
			return e
		}
		a = string(v)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO app.schedule_configuration_history(semester_id,actor_id,entity,entity_id,before_state,after_state) VALUES($1,$2,$3,$4,$5,$6)`, semester, actor, entity, id, b, a)
	return err
}
func queryCurriculumRules(ctx context.Context, tx *sql.Tx, semester int64) ([]CurriculumRule, error) {
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.semester_id,r.grade,coalesce(r.class_id,0),r.subject_id,r.weekly_jp,r.meetings::text,r.different_days,r.version,coalesce(c.grade::text||c.name,''),s.name FROM app.curriculum_rules r LEFT JOIN app.classes c ON c.id=r.class_id JOIN app.subjects s ON s.id=r.subject_id WHERE r.semester_id=$1 ORDER BY r.grade,s.name,r.class_id NULLS FIRST`, semester)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CurriculumRule
	for rows.Next() {
		var r CurriculumRule
		var pattern string
		if err = rows.Scan(&r.ID, &r.SemesterID, &r.Grade, &r.ClassID, &r.SubjectID, &r.WeeklyJP, &pattern, &r.DifferentDays, &r.Version, &r.ClassName, &r.SubjectName); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(pattern), &r.Meetings); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) CurriculumRules(ctx context.Context, actor, semester int64) ([]CurriculumRule, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return queryCurriculumRules(ctx, tx, semester)
}

func saveCurriculumRule(ctx context.Context, tx *sql.Tx, actor int64, r CurriculumRule) error {
	if err := validateCurriculumRule(r); err != nil {
		return err
	}
	var eligible bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semesters sm JOIN app.subjects s ON s.id=$2 AND s.active WHERE sm.id=$1 AND ($3::bigint=0 OR EXISTS(SELECT 1 FROM app.classes c WHERE c.id=$3 AND c.school_year_id=sm.school_year_id AND c.grade=$4)))`, r.SemesterID, r.SubjectID, r.ClassID, r.Grade).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ValidationError("Semester, tingkat, rombel atau mapel tidak sesuai.")
	}
	var before any
	if r.ID > 0 {
		existing, e := queryCurriculumRules(ctx, tx, r.SemesterID)
		if e != nil {
			return e
		}
		found := false
		for _, old := range existing {
			if old.ID == r.ID {
				if old.Version != r.Version {
					return ErrConflict
				}
				before = old
				found = true
			}
		}
		if !found {
			return ErrNotFound
		}
	}
	if r.Meetings == nil {
		r.Meetings = []int{}
	}
	pattern, err := json.Marshal(r.Meetings)
	if err != nil {
		return err
	}
	if r.ID == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO app.curriculum_rules(semester_id,grade,class_id,subject_id,weekly_jp,meetings,different_days,updated_by) VALUES($1,$2,NULLIF($3,0),$4,$5,$6,$7,$8) RETURNING id,version`, r.SemesterID, r.Grade, r.ClassID, r.SubjectID, r.WeeklyJP, string(pattern), r.DifferentDays, actor).Scan(&r.ID, &r.Version)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE app.curriculum_rules SET grade=$1,class_id=NULLIF($2,0),subject_id=$3,weekly_jp=$4,meetings=$5,different_days=$6,updated_by=$7,updated_at=now(),version=version+1 WHERE id=$8 AND semester_id=$9 AND version=$10 RETURNING version`, r.Grade, r.ClassID, r.SubjectID, r.WeeklyJP, string(pattern), r.DifferentDays, actor, r.ID, r.SemesterID, r.Version).Scan(&r.Version)
	}
	if err != nil {
		return dbError(err)
	}
	return configurationHistory(ctx, tx, actor, r.SemesterID, "rule", r.ID, before, r)
}
func (s *Store) SaveCurriculumRule(ctx context.Context, actor int64, r CurriculumRule) error {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = saveCurriculumRule(ctx, tx, actor, r); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ResetCurriculumOverride(ctx context.Context, actor, semester, id, version int64) error {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rules, err := queryCurriculumRules(ctx, tx, semester)
	if err != nil {
		return err
	}
	for _, r := range rules {
		if r.ID == id && r.ClassID > 0 {
			if r.Version != version {
				return ErrConflict
			}
			var hasDefault bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.curriculum_rules WHERE semester_id=$1 AND grade=$2 AND subject_id=$3 AND class_id IS NULL)`, semester, r.Grade, r.SubjectID).Scan(&hasDefault); err != nil {
				return err
			}
			if !hasDefault {
				return ValidationError("Bawaan tingkat belum tersedia. Buat bawaan tingkat terlebih dahulu sebelum menghapus pengecualian.")
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM app.curriculum_rules WHERE id=$1`, id); err != nil {
				return err
			}
			if err = configurationHistory(ctx, tx, actor, semester, "rule", id, r, nil); err != nil {
				return err
			}
			return tx.Commit()
		}
	}
	return ErrNotFound
}
func (s *Store) CopyCurriculum(ctx context.Context, actor, source, target int64) error {
	if source < 1 || target < 1 || source == target {
		return ValidationError("Pilih semester sumber yang lebih awal.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semesters a JOIN app.school_years ay ON ay.id=a.school_year_id JOIN app.semesters b ON b.id=$2 JOIN app.school_years byear ON byear.id=b.school_year_id WHERE a.id=$1 AND (ay.name,a.number)<(byear.name,b.number))`, source, target).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ValidationError("Semester sumber harus lebih awal dari semester tujuan.")
	}
	rules, err := queryCurriculumRules(ctx, tx, source)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return ValidationError("Semester sumber belum memiliki struktur kurikulum.")
	}
	for _, r := range rules {
		if r.ClassID > 0 {
			var class int64
			err = tx.QueryRowContext(ctx, `SELECT c.id FROM app.classes old JOIN app.semesters sm ON sm.id=$1 JOIN app.classes c ON c.school_year_id=sm.school_year_id AND c.grade=old.grade AND lower(c.name)=lower(old.name) WHERE old.id=$2`, target, r.ClassID).Scan(&class)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			r.ClassID = class
		}
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.curriculum_rules WHERE semester_id=$1 AND grade=$2 AND subject_id=$3 AND coalesce(class_id,0)=$4)`, target, r.Grade, r.SubjectID, r.ClassID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		r.ID = 0
		r.Version = 0
		r.SemesterID = target
		if err = saveCurriculumRule(ctx, tx, actor, r); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func querySchoolBlocks(ctx context.Context, tx *sql.Tx, semester int64) ([]SchoolTimeBlock, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,semester_id,weekday,kind,label,coalesce(period_no,0),starts_at::text,ends_at::text,version FROM app.school_time_blocks WHERE semester_id=$1 ORDER BY weekday,starts_at,id`, semester)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SchoolTimeBlock
	for rows.Next() {
		var b SchoolTimeBlock
		if err = rows.Scan(&b.ID, &b.SemesterID, &b.Weekday, &b.Kind, &b.Label, &b.PeriodNo, &b.StartsAt, &b.EndsAt, &b.Version); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Store) SchoolBlocks(ctx context.Context, actor, semester int64) ([]SchoolTimeBlock, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return querySchoolBlocks(ctx, tx, semester)
}
func (s *Store) SaveSchoolBlock(ctx context.Context, actor int64, b SchoolTimeBlock) error {
	b.Label = strings.TrimSpace(b.Label)
	start, e := time.Parse("15:04", b.StartsAt)
	end, e2 := time.Parse("15:04", b.EndsAt)
	if b.SemesterID < 1 || b.Weekday < 1 || b.Weekday > 7 || len([]rune(b.Label)) < 1 || len([]rune(b.Label)) > 100 || e != nil || e2 != nil || !start.Before(end) || (b.Kind != "break" && b.Kind != "activity") || (b.Kind == "break" && b.PeriodNo != 0) || (b.Kind == "activity" && (b.PeriodNo < 1 || b.PeriodNo > 30)) {
		return ValidationError("Isi jenis, nama kegiatan, hari, waktu, dan nomor jam kegiatan yang sesuai; istirahat tanpa nomor jam.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	blocks, err := querySchoolBlocks(ctx, tx, b.SemesterID)
	if err != nil {
		return err
	}
	var before any
	found := b.ID == 0
	for _, old := range blocks {
		if old.ID == b.ID {
			found = true
			if old.Version != b.Version {
				return ErrConflict
			}
			before = old
			continue
		}
		if old.Weekday == b.Weekday && clockMinute(old.StartsAt) < clockMinute(b.EndsAt) && clockMinute(old.EndsAt) > clockMinute(b.StartsAt) {
			return ValidationError("Waktu istirahat/kegiatan bertumpang tindih dengan kegiatan lain.")
		}
	}
	if !found {
		return ErrNotFound
	}
	var conflict bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.lesson_periods WHERE semester_id=$1 AND weekday=$2 AND starts_at<$4::time AND ends_at>$3::time AND NOT($5='activity' AND period_no=$6 AND starts_at=$3::time AND ends_at=$4::time))`, b.SemesterID, b.Weekday, b.StartsAt, b.EndsAt, b.Kind, b.PeriodNo).Scan(&conflict)
	if err != nil {
		return err
	}
	if conflict {
		return ValidationError("Waktu ini memotong slot belajar. Sesuaikan Jam Pelajaran; kegiatan harus tepat mengisi nomor jam yang dipilih.")
	}
	if b.Kind == "activity" {
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.lesson_periods WHERE semester_id=$1 AND weekday=$2 AND period_no=$3 AND starts_at=$4::time AND ends_at=$5::time)`, b.SemesterID, b.Weekday, b.PeriodNo, b.StartsAt, b.EndsAt).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return ValidationError("Atur nomor dan waktu jam kegiatan terlebih dahulu di Jam Pelajaran.")
		}
	}
	if b.ID == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO app.school_time_blocks(semester_id,weekday,kind,label,period_no,starts_at,ends_at,updated_by) VALUES($1,$2,$3,$4,NULLIF($5,0),$6,$7,$8) RETURNING id,version`, b.SemesterID, b.Weekday, b.Kind, b.Label, b.PeriodNo, b.StartsAt, b.EndsAt, actor).Scan(&b.ID, &b.Version)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE app.school_time_blocks SET weekday=$1,kind=$2,label=$3,period_no=NULLIF($4,0),starts_at=$5,ends_at=$6,updated_by=$7,updated_at=now(),version=version+1 WHERE id=$8 AND semester_id=$9 AND version=$10 RETURNING version`, b.Weekday, b.Kind, b.Label, b.PeriodNo, b.StartsAt, b.EndsAt, actor, b.ID, b.SemesterID, b.Version).Scan(&b.Version)
	}
	if err != nil {
		return dbError(err)
	}
	if err = configurationHistory(ctx, tx, actor, b.SemesterID, "block", b.ID, before, b); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) RemoveSchoolBlock(ctx context.Context, actor, semester, id, version int64) error {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	blocks, err := querySchoolBlocks(ctx, tx, semester)
	if err != nil {
		return err
	}
	for _, b := range blocks {
		if b.ID == id {
			if b.Version != version {
				return ErrConflict
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM app.school_time_blocks WHERE id=$1`, id); err != nil {
				return err
			}
			if err = configurationHistory(ctx, tx, actor, semester, "block", id, b, nil); err != nil {
				return err
			}
			return tx.Commit()
		}
	}
	return ErrNotFound
}

func querySchedulePlan(ctx context.Context, tx *sql.Tx, semester int64, date time.Time) (SchedulePlan, error) {
	var p SchedulePlan
	rows, err := tx.QueryContext(ctx, `SELECT c.id,c.grade::text||c.name,c.grade,s.id,s.name,coalesce(r.id,0),coalesce(r.class_id,0),coalesce(r.weekly_jp,-1),coalesce(r.meetings,'[]'::jsonb)::text,coalesce(r.different_days,true),coalesce(r.version,0),coalesce(a.id,0),coalesce(a.teacher_id,0),coalesce(t.name,'') FROM app.semesters sm JOIN app.classes c ON c.school_year_id=sm.school_year_id CROSS JOIN app.subjects s LEFT JOIN LATERAL(SELECT cr.* FROM app.curriculum_rules cr WHERE cr.semester_id=sm.id AND cr.subject_id=s.id AND (cr.class_id=c.id OR (cr.class_id IS NULL AND cr.grade=c.grade)) ORDER BY cr.class_id NULLS LAST LIMIT 1) r ON true LEFT JOIN app.teaching_assignments a ON a.semester_id=sm.id AND a.class_id=c.id AND a.subject_id=s.id AND a.effective_from<=$2 AND (a.effective_until IS NULL OR a.effective_until>$2) AND s.active AND EXISTS(SELECT 1 FROM app.teachers at JOIN app.users u ON u.id=at.user_id AND u.active JOIN app.user_roles ur ON ur.user_id=u.id AND ur.role_code='guru_mapel' WHERE at.id=a.teacher_id AND at.active) LEFT JOIN app.teachers t ON t.id=a.teacher_id WHERE sm.id=$1 AND (r.id IS NOT NULL OR EXISTS(SELECT 1 FROM app.teaching_assignments x WHERE x.semester_id=sm.id AND x.class_id=c.id AND x.subject_id=s.id AND x.effective_from<=$2 AND (x.effective_until IS NULL OR x.effective_until>$2))) ORDER BY c.grade,c.name,s.name,s.id`, semester, date)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var r EffectiveCurriculum
		var pattern string
		var sourceClass int64
		err = rows.Scan(&r.ClassID, &r.ClassName, &r.Grade, &r.SubjectID, &r.SubjectName, &r.ID, &sourceClass, &r.WeeklyJP, &pattern, &r.DifferentDays, &r.Version, &r.AssignmentID, &r.TeacherID, &r.TeacherName)
		if err != nil {
			rows.Close()
			return p, err
		}
		r.SemesterID = semester
		r.Missing = r.ID == 0
		err = json.Unmarshal([]byte(pattern), &r.Meetings)
		if err != nil {
			rows.Close()
			return p, err
		}
		r.Inherited = sourceClass == 0
		p.Rules = append(p.Rules, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return p, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT weekday,period_no,starts_at::text,ends_at::text FROM app.lesson_periods WHERE semester_id=$1 ORDER BY weekday,period_no`, semester)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var lp LessonPeriod
		if err = rows.Scan(&lp.Weekday, &lp.Number, &lp.StartsAt, &lp.EndsAt); err != nil {
			rows.Close()
			return p, err
		}
		p.Periods = append(p.Periods, lp)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT c.id,c.grade::text||c.name FROM app.classes c JOIN app.semesters sm ON sm.school_year_id=c.school_year_id WHERE sm.id=$1 ORDER BY c.grade,c.name`, semester)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var c TeachingOption
		if err = rows.Scan(&c.ID, &c.Name); err != nil {
			rows.Close()
			return p, err
		}
		p.Classes = append(p.Classes, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	p.Blocks, err = querySchoolBlocks(ctx, tx, semester)
	if err != nil {
		return p, err
	}
	p.Constraints, err = queryScheduleConstraints(ctx, tx, semester)
	if err != nil {
		return p, err
	}
	p.ConstraintGrids, err = queryConstraintGrids(ctx, tx, semester)
	p.Constraints = effectiveGridConstraints(p.Constraints, p.ConstraintGrids)
	return p, err
}
func (s *Store) SchedulePlan(ctx context.Context, actor, semester int64, date time.Time) (SchedulePlan, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return SchedulePlan{}, err
	}
	defer tx.Rollback()
	return querySchedulePlan(ctx, tx, semester, date)
}

func clockMinute(s string) int {
	if len(s) < 5 {
		return 0
	}
	return (int(s[0]-'0')*10+int(s[1]-'0'))*60 + int(s[3]-'0')*10 + int(s[4]-'0')
}
