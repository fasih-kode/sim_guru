package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *Store) TeachingOptions(ctx context.Context, actor, semesterID int64) (classes, teachers, subjects []TeachingOption, err error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, nil, nil, err
	}
	defer tx.Rollback()
	var yearID int64
	err = tx.QueryRowContext(ctx, `SELECT school_year_id FROM app.semesters WHERE id=$1`, semesterID).Scan(&yearID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if classes, err = queryTeachingOptions(ctx, tx, `SELECT id,grade::text||name FROM app.classes WHERE school_year_id=$1 ORDER BY grade,name`, yearID); err != nil {
		return nil, nil, nil, err
	}
	if teachers, err = queryTeachingOptions(ctx, tx, `SELECT t.id,t.name FROM app.teachers t JOIN app.users u ON u.id=t.user_id JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' WHERE t.active AND u.active ORDER BY t.name,t.id`); err != nil {
		return nil, nil, nil, err
	}
	subjects, err = queryTeachingOptions(ctx, tx, `SELECT id,name FROM app.subjects WHERE active ORDER BY name,id`)
	return
}

func queryTeachingOptions(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]TeachingOption, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeachingOption
	for rows.Next() {
		var item TeachingOption
		if err = rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) TeachingAssignments(ctx context.Context, actor, semesterID int64) ([]TeachingAssignment, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.semester_id,a.class_id,a.subject_id,a.teacher_id,y.name||' · Semester '||sm.number::text,c.grade::text||c.name,s.name,t.name,a.effective_from,a.effective_until FROM app.teaching_assignments a JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.school_years y ON y.id=sm.school_year_id JOIN app.classes c ON c.id=a.class_id JOIN app.subjects s ON s.id=a.subject_id JOIN app.teachers t ON t.id=a.teacher_id WHERE a.semester_id=$1 ORDER BY c.grade,c.name,s.name,a.effective_from DESC`, semesterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeachingAssignment
	for rows.Next() {
		var item TeachingAssignment
		if err = rows.Scan(&item.ID, &item.SemesterID, &item.ClassID, &item.SubjectID, &item.TeacherID, &item.SemesterName, &item.ClassName, &item.SubjectName, &item.TeacherName, &item.EffectiveFrom, &item.EffectiveUntil); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) AssignTeacher(ctx context.Context, actor, semesterID, classID, subjectID, teacherID int64, effectiveFrom time.Time) error {
	if semesterID < 1 || classID < 1 || subjectID < 1 || teacherID < 1 || effectiveFrom.IsZero() {
		return ValidationError("Lengkapi semester, kelas, mata pelajaran, guru, dan tanggal berlaku.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semesters sm JOIN app.classes c ON c.school_year_id=sm.school_year_id JOIN app.subjects s ON s.id=$3 AND s.active WHERE sm.id=$1 AND c.id=$2)`, semesterID, classID, subjectID).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ValidationError("Kelas harus berasal dari tahun ajaran semester dan mata pelajaran harus aktif.")
	}
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teachers t JOIN app.users u ON u.id=t.user_id JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' WHERE t.id=$1 AND t.active AND u.active)`, teacherID).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ValidationError("Guru harus aktif, tertaut ke akun aktif, dan memiliki role Guru Mapel.")
	}
	var currentID, currentTeacher int64
	var currentFrom time.Time
	err = tx.QueryRowContext(ctx, `SELECT id,teacher_id,effective_from FROM app.teaching_assignments WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND effective_until IS NULL FOR UPDATE`, semesterID, classID, subjectID).Scan(&currentID, &currentTeacher, &currentFrom)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if currentTeacher == teacherID {
			return tx.Commit()
		}
		if !effectiveFrom.After(currentFrom) {
			return ValidationError("Tanggal pergantian harus setelah tanggal mulai penugasan saat ini.")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE app.teaching_assignments SET effective_until=$1 WHERE id=$2`, effectiveFrom, currentID); err != nil {
			return err
		}
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app.teaching_assignments(semester_id,class_id,subject_id,teacher_id,effective_from,assigned_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, semesterID, classID, subjectID, teacherID, effectiveFrom, actor).Scan(&id)
	if err != nil {
		return dbError(err)
	}
	after, _ := json.Marshal(map[string]any{"semester_id": semesterID, "class_id": classID, "subject_id": subjectID, "teacher_id": teacherID, "effective_from": effectiveFrom.Format("2006-01-02")})
	var before any
	if currentID > 0 {
		before, _ = json.Marshal(map[string]any{"teacher_id": currentTeacher, "effective_from": currentFrom.Format("2006-01-02")})
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.teaching_assignment_history(assignment_id,actor_id,before_state,after_state) VALUES($1,$2,$3,$4)`, id, actor, before, after); err != nil {
		return err
	}
	return tx.Commit()
}

// CorrectTeachingAssignment updates a mistaken assignment while retaining the
// previous values in the immutable assignment history.
func (s *Store) CorrectTeachingAssignment(ctx context.Context, actor, assignmentID, semesterID, classID, subjectID, teacherID int64, effectiveFrom time.Time, effectiveUntil *time.Time) error {
	if assignmentID < 1 || semesterID < 1 || classID < 1 || subjectID < 1 || teacherID < 1 || effectiveFrom.IsZero() {
		return ValidationError("Lengkapi kelas, mata pelajaran, guru, dan tanggal berlaku.")
	}
	if effectiveUntil != nil && !effectiveUntil.After(effectiveFrom) {
		return ValidationError("Tanggal selesai harus setelah tanggal mulai.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldSemester, oldClass, oldSubject, oldTeacher int64
	var oldFrom time.Time
	var oldUntil sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT semester_id,class_id,subject_id,teacher_id,effective_from,effective_until FROM app.teaching_assignments WHERE id=$1 FOR UPDATE`, assignmentID).Scan(&oldSemester, &oldClass, &oldSubject, &oldTeacher, &oldFrom, &oldUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if oldSemester != semesterID {
		return ErrNotFound
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semesters sm JOIN app.classes c ON c.school_year_id=sm.school_year_id AND c.id=$2 JOIN app.subjects s ON s.id=$3 AND s.active WHERE sm.id=$1)`, semesterID, classID, subjectID).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ValidationError("Kelas harus berasal dari tahun ajaran semester dan mata pelajaran harus aktif.")
	}
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teachers t JOIN app.users u ON u.id=t.user_id JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' WHERE t.id=$1 AND t.active AND u.active)`, teacherID).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ValidationError("Guru harus aktif, tertaut ke akun aktif, dan memiliki role Guru Mapel.")
	}
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_assignments a WHERE a.id<>$1 AND a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4 AND (a.effective_until IS NULL OR a.effective_until>$5) AND ($6::date IS NULL OR a.effective_from<$6))`, assignmentID, semesterID, classID, subjectID, effectiveFrom, effectiveUntil).Scan(&eligible)
	if err != nil {
		return err
	}
	if eligible {
		return ValidationError("Tanggal penugasan bertabrakan dengan penugasan kelas dan mata pelajaran yang sama.")
	}
	var beforeUntil any
	if oldUntil.Valid {
		beforeUntil = oldUntil.Time.Format("2006-01-02")
	}
	var afterUntil any
	if effectiveUntil != nil {
		afterUntil = effectiveUntil.Format("2006-01-02")
	}
	before, _ := json.Marshal(map[string]any{"semester_id": oldSemester, "class_id": oldClass, "subject_id": oldSubject, "teacher_id": oldTeacher, "effective_from": oldFrom.Format("2006-01-02"), "effective_until": beforeUntil})
	after, _ := json.Marshal(map[string]any{"semester_id": semesterID, "class_id": classID, "subject_id": subjectID, "teacher_id": teacherID, "effective_from": effectiveFrom.Format("2006-01-02"), "effective_until": afterUntil})
	if oldSemester == semesterID && oldClass == classID && oldSubject == subjectID && oldTeacher == teacherID && oldFrom.Equal(effectiveFrom) && oldUntil.Valid == (effectiveUntil != nil) && (!oldUntil.Valid || oldUntil.Time.Equal(*effectiveUntil)) {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.teaching_assignments SET class_id=$1,subject_id=$2,teacher_id=$3,effective_from=$4,effective_until=$5 WHERE id=$6`, classID, subjectID, teacherID, effectiveFrom, effectiveUntil, assignmentID); err != nil {
		return dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.teaching_assignment_history(assignment_id,actor_id,before_state,after_state) VALUES($1,$2,$3,$4)`, assignmentID, actor, before, after); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CopyTeachingAssignments(ctx context.Context, actor, sourceSemester, targetSemester int64, effectiveFrom time.Time) (int, error) {
	if sourceSemester < 1 || targetSemester < 1 || sourceSemester == targetSemester || effectiveFrom.IsZero() {
		return 0, ValidationError("Pilih semester sumber, semester tujuan, dan tanggal berlaku yang valid.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var sourceYear, targetYear int64
	var sourceNumber, targetNumber int
	err = tx.QueryRowContext(ctx, `SELECT sa.school_year_id,sa.number,sb.school_year_id,sb.number FROM app.semesters sa JOIN app.semesters sb ON sb.id=$2 WHERE sa.id=$1`, sourceSemester, targetSemester).Scan(&sourceYear, &sourceNumber, &targetYear, &targetNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if targetYear < sourceYear || (targetYear == sourceYear && targetNumber <= sourceNumber) {
		return 0, ValidationError("Penugasan hanya dapat disalin dari semester yang lebih awal.")
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT ON(c.grade,lower(c.name),a.subject_id) c.grade,lower(c.name),a.subject_id,a.teacher_id FROM app.teaching_assignments a JOIN app.classes c ON c.id=a.class_id JOIN app.subjects s ON s.id=a.subject_id AND s.active WHERE a.semester_id=$1 AND a.effective_from<=$2 AND a.effective_until IS NULL ORDER BY c.grade,lower(c.name),a.subject_id,a.effective_from DESC`, sourceSemester, effectiveFrom)
	if err != nil {
		return 0, err
	}
	type copied struct {
		grade            int
		name             string
		subject, teacher int64
	}
	var source []copied
	for rows.Next() {
		var v copied
		if err = rows.Scan(&v.grade, &v.name, &v.subject, &v.teacher); err != nil {
			rows.Close()
			return 0, err
		}
		source = append(source, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	created := 0
	for _, v := range source {
		var classID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM app.classes WHERE school_year_id=$1 AND grade=$2 AND lower(name)=$3`, targetYear, v.grade, v.name).Scan(&classID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return created, err
		}
		var exists, eligible bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_assignments WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND effective_until IS NULL)`, targetSemester, classID, v.subject).Scan(&exists); err != nil {
			return created, err
		}
		if exists {
			continue
		}
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teachers t JOIN app.users u ON u.id=t.user_id JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' WHERE t.id=$1 AND t.active AND u.active)`, v.teacher).Scan(&eligible); err != nil {
			return created, err
		}
		if !eligible {
			continue
		}
		var id int64
		err = tx.QueryRowContext(ctx, `INSERT INTO app.teaching_assignments(semester_id,class_id,subject_id,teacher_id,effective_from,assigned_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, targetSemester, classID, v.subject, v.teacher, effectiveFrom, actor).Scan(&id)
		if err != nil {
			return created, dbError(err)
		}
		after, _ := json.Marshal(map[string]any{"semester_id": targetSemester, "class_id": classID, "subject_id": v.subject, "teacher_id": v.teacher, "effective_from": effectiveFrom.Format("2006-01-02"), "copied_from_semester": sourceSemester})
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.teaching_assignment_history(assignment_id,actor_id,after_state) VALUES($1,$2,$3)`, id, actor, after); err != nil {
			return created, err
		}
		created++
	}
	return created, tx.Commit()
}

func (s *Store) LessonPeriods(ctx context.Context, actor, semesterID int64) ([]LessonPeriod, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,weekday,period_no,starts_at::text,ends_at::text FROM app.lesson_periods WHERE semester_id=$1 ORDER BY weekday,period_no`, semesterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LessonPeriod
	for rows.Next() {
		var v LessonPeriod
		if err = rows.Scan(&v.ID, &v.Weekday, &v.Number, &v.StartsAt, &v.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveLessonPeriod preserves the single-period API for existing callers.
func (s *Store) SaveLessonPeriod(ctx context.Context, actor, semesterID int64, weekday, number int, starts, ends string) error {
	if starts == "" || ends == "" {
		return ValidationError("Isi waktu mulai dan selesai yang valid.")
	}
	return s.SaveLessonDay(ctx, actor, semesterID, weekday, []LessonPeriod{{Number: number, StartsAt: starts, EndsAt: ends}})
}

// SaveLessonDay validates the final day before writing any period. Blank pairs
// leave existing data untouched; an unused template row is never persisted.
func (s *Store) SaveLessonDay(ctx context.Context, actor, semesterID int64, weekday int, input []LessonPeriod) error {
	if semesterID < 1 || weekday < 1 || weekday > 7 || len(input) > 30 {
		return ValidationError("Pilih semester, hari, dan nomor jam 1–30.")
	}
	updates := make(map[int]LessonPeriod)
	seen := make(map[int]bool)
	for _, p := range input {
		if p.Number < 1 || p.Number > 30 || seen[p.Number] {
			return ValidationError("Nomor jam tidak valid atau berulang.")
		}
		seen[p.Number] = true
		if p.StartsAt == "" && p.EndsAt == "" {
			continue
		}
		start, e := time.Parse("15:04", p.StartsAt)
		if e != nil {
			return ValidationError("Isi waktu mulai dan selesai yang valid untuk setiap jam yang digunakan.")
		}
		end, e := time.Parse("15:04", p.EndsAt)
		if e != nil || !start.Before(end) {
			return ValidationError("Waktu selesai harus setelah waktu mulai pada setiap jam.")
		}
		p.Weekday = weekday
		p.StartsAt, p.EndsAt = start.Format("15:04"), end.Format("15:04")
		updates[p.Number] = p
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semesters WHERE id=$1)`, semesterID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT period_no,to_char(starts_at,'HH24:MI'),to_char(ends_at,'HH24:MI') FROM app.lesson_periods WHERE semester_id=$1 AND weekday=$2`, semesterID, weekday)
	if err != nil {
		return err
	}
	original := make(map[int]LessonPeriod)
	for rows.Next() {
		var p LessonPeriod
		if err = rows.Scan(&p.Number, &p.StartsAt, &p.EndsAt); err != nil {
			rows.Close()
			return err
		}
		original[p.Number] = p
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	merged := make(map[int]LessonPeriod)
	for n, p := range original {
		merged[n] = p
	}
	for n, p := range updates {
		merged[n] = p
	}
	for n, p := range merged {
		for m, q := range merged {
			if n < m && p.StartsAt < q.EndsAt && p.EndsAt > q.StartsAt {
				return ValidationError("Waktu jam pelajaran bertumpang tindih dengan nomor jam lain pada hari ini.")
			}
		}
	}
	// Lock drafts in the same transaction before synchronizing their snapshots.
	locks, err := tx.QueryContext(ctx, `SELECT id FROM app.weekly_timetable_revisions WHERE semester_id=$1 AND status='draft' ORDER BY id FOR UPDATE`, semesterID)
	if err != nil {
		return err
	}
	for locks.Next() {
	}
	err = locks.Err()
	locks.Close()
	if err != nil {
		return err
	}
	for n := 1; n <= 30; n++ {
		p, ok := updates[n]
		if !ok {
			continue
		}
		old, had := original[n]
		if had && old.StartsAt == p.StartsAt && old.EndsAt == p.EndsAt {
			continue
		}
		var id int64
		err = tx.QueryRowContext(ctx, `INSERT INTO app.lesson_periods(semester_id,weekday,period_no,starts_at,ends_at,updated_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(semester_id,weekday,period_no) DO UPDATE SET starts_at=excluded.starts_at,ends_at=excluded.ends_at,updated_by=excluded.updated_by,updated_at=now() RETURNING id`, semesterID, weekday, n, p.StartsAt, p.EndsAt, actor).Scan(&id)
		if err != nil {
			return dbError(err)
		}
		var before any
		if had {
			before, _ = json.Marshal(map[string]any{"weekday": weekday, "starts_at": old.StartsAt, "ends_at": old.EndsAt})
		}
		after, _ := json.Marshal(map[string]any{"semester_id": semesterID, "weekday": weekday, "period_no": n, "starts_at": p.StartsAt, "ends_at": p.EndsAt})
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.lesson_period_history(period_id,actor_id,before_state,after_state) VALUES($1,$2,$3,$4)`, id, actor, before, after); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_periods(revision_id,weekday,period_no,starts_at,ends_at) SELECT r.id,$2,$3,$4,$5 FROM app.weekly_timetable_revisions r WHERE r.semester_id=$1 AND r.status='draft' ON CONFLICT(revision_id,weekday,period_no) DO UPDATE SET starts_at=excluded.starts_at,ends_at=excluded.ends_at`, semesterID, weekday, n, p.StartsAt, p.EndsAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ScheduleAssignments only offers eligible assignments at the draft's effective date.
func (s *Store) ScheduleAssignments(ctx context.Context, actor, semesterID int64, date time.Time) ([]TeachingAssignment, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.class_id,a.subject_id,a.teacher_id,c.grade::text||c.name,s.name,t.name FROM app.teaching_assignments a JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id JOIN app.subjects s ON s.id=a.subject_id AND s.active JOIN app.teachers t ON t.id=a.teacher_id AND t.active JOIN app.users u ON u.id=t.user_id AND u.active JOIN app.user_roles ur ON ur.user_id=u.id AND ur.role_code='guru_mapel' WHERE a.semester_id=$1 AND a.effective_from<=$2 AND (a.effective_until IS NULL OR a.effective_until>$2) ORDER BY c.grade,c.name,s.name`, semesterID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeachingAssignment
	for rows.Next() {
		var a TeachingAssignment
		if err = rows.Scan(&a.ID, &a.ClassID, &a.SubjectID, &a.TeacherID, &a.ClassName, &a.SubjectName, &a.TeacherName); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
