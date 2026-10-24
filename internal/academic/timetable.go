package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *Store) CreateTimetableDraft(ctx context.Context, actor, semesterID int64, effectiveFrom time.Time) (TimetableRevision, error) {
	if semesterID < 1 || effectiveFrom.IsZero() {
		return TimetableRevision{}, ValidationError("Pilih semester dan tanggal berlaku.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return TimetableRevision{}, err
	}
	defer tx.Rollback()
	rev, err := createTimetableDraftTx(ctx, tx, actor, semesterID, effectiveFrom)
	if err != nil {
		return rev, err
	}
	return rev, tx.Commit()
}
func createTimetableDraftTx(ctx context.Context, tx *sql.Tx, actor, semesterID int64, effectiveFrom time.Time) (TimetableRevision, error) {
	var err error
	var periods int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.lesson_periods WHERE semester_id=$1`, semesterID).Scan(&periods); err != nil {
		return TimetableRevision{}, err
	}
	if periods == 0 {
		return TimetableRevision{}, ValidationError("Atur jam pelajaran sebelum membuat draf jadwal.")
	}
	var blocked bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.weekly_timetable_revisions WHERE semester_id=$1 AND (status='draft' OR effective_from >= $2))`, semesterID, effectiveFrom).Scan(&blocked)
	if err != nil {
		return TimetableRevision{}, err
	}
	if blocked {
		return TimetableRevision{}, ValidationError("Sudah ada draf atau jadwal dengan tanggal berlaku yang sama atau lebih baru.")
	}
	var rev TimetableRevision
	rev.SemesterID, rev.Status, rev.EffectiveFrom = semesterID, "draft", effectiveFrom
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(max(revision_no),0)+1 FROM app.weekly_timetable_revisions WHERE semester_id=$1`, semesterID).Scan(&rev.Revision); err != nil {
		return rev, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO app.weekly_timetable_revisions(semester_id,revision_no,status,effective_from,created_by) VALUES($1,$2,'draft',$3,$4) RETURNING id`, semesterID, rev.Revision, effectiveFrom, actor).Scan(&rev.ID)
	if err != nil {
		return rev, dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_periods(revision_id,weekday,period_no,starts_at,ends_at) SELECT $1,weekday,period_no,starts_at,ends_at FROM app.lesson_periods WHERE semester_id=$2`, rev.ID, semesterID); err != nil {
		return rev, err
	}
	after, _ := json.Marshal(rev)
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,after_state) VALUES($1,$2,'draft_created',$3)`, rev.ID, actor, after); err != nil {
		return rev, err
	}
	plan, err := querySchedulePlan(ctx, tx, semesterID, effectiveFrom)
	if err != nil {
		return rev, err
	}
	if err = savePlanSnapshot(ctx, tx, rev.ID, plan); err != nil {
		return rev, err
	}
	return rev, nil
}

func (s *Store) Timetable(ctx context.Context, actor, semesterID int64) (TimetableRevision, []LessonPeriod, []TimetableEntry, error) {
	return s.TimetableAtRevision(ctx, actor, semesterID, 0)
}
func (s *Store) TimetableAtRevision(ctx context.Context, actor, semesterID, revisionID int64) (TimetableRevision, []LessonPeriod, []TimetableEntry, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return TimetableRevision{}, nil, nil, err
	}
	defer tx.Rollback()
	var rev TimetableRevision
	err = tx.QueryRowContext(ctx, `SELECT r.id,r.semester_id,y.name||' · Semester '||sm.number::text,r.revision_no,r.status,r.effective_from FROM app.weekly_timetable_revisions r JOIN app.semesters sm ON sm.id=r.semester_id JOIN app.school_years y ON y.id=sm.school_year_id WHERE r.semester_id=$1 AND ($2::bigint=0 OR r.id=$2) ORDER BY (r.status='draft') DESC,r.effective_from DESC LIMIT 1`, semesterID, revisionID).Scan(&rev.ID, &rev.SemesterID, &rev.SemesterName, &rev.Revision, &rev.Status, &rev.EffectiveFrom)
	if errors.Is(err, sql.ErrNoRows) {
		if revisionID != 0 {
			return TimetableRevision{}, nil, nil, ErrNotFound
		}
		return TimetableRevision{SemesterID: semesterID}, nil, nil, nil
	}
	if err != nil {
		return rev, nil, nil, err
	}
	periods, err := queryRevisionPeriods(ctx, tx, rev.ID)
	if err != nil {
		return rev, nil, nil, err
	}
	entries, err := queryTimetableEntries(ctx, tx, rev.ID, 0)
	if err != nil {
		return rev, periods, nil, err
	}
	snapshot, err := queryPlanSnapshot(ctx, tx, rev.ID)
	if err != nil {
		return rev, periods, nil, err
	}
	if snapshot != nil {
		entries = timetableTimeline(entries, snapshot.Blocks)
	}
	return rev, periods, entries, nil
}

func queryRevisionPeriods(ctx context.Context, tx *sql.Tx, id int64) ([]LessonPeriod, error) {
	rows, err := tx.QueryContext(ctx, `SELECT weekday,period_no,starts_at::text,ends_at::text FROM app.weekly_timetable_periods WHERE revision_id=$1 ORDER BY weekday,period_no`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LessonPeriod
	for rows.Next() {
		var v LessonPeriod
		if err = rows.Scan(&v.Weekday, &v.Number, &v.StartsAt, &v.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func queryTimetableEntries(ctx context.Context, tx *sql.Tx, revisionID, teacherID int64) ([]TimetableEntry, error) {
	query := `SELECT e.id,e.class_id,e.subject_id,e.teacher_id,e.assignment_id,e.weekday,e.period_start,e.period_end,c.grade::text||c.name,s.name,t.name,p1.starts_at::text,p2.ends_at::text FROM app.weekly_timetable_entries e JOIN app.classes c ON c.id=e.class_id JOIN app.subjects s ON s.id=e.subject_id JOIN app.teachers t ON t.id=e.teacher_id JOIN app.weekly_timetable_periods p1 ON p1.revision_id=e.revision_id AND p1.weekday=e.weekday AND p1.period_no=e.period_start JOIN app.weekly_timetable_periods p2 ON p2.revision_id=e.revision_id AND p2.weekday=e.weekday AND p2.period_no=e.period_end WHERE e.revision_id=$1`
	args := []any{revisionID}
	if teacherID > 0 {
		query += ` AND e.teacher_id=$2`
		args = append(args, teacherID)
	}
	query += ` ORDER BY e.weekday,e.period_start,c.grade,c.name`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimetableEntry
	for rows.Next() {
		var v TimetableEntry
		if err = rows.Scan(&v.ID, &v.ClassID, &v.SubjectID, &v.TeacherID, &v.AssignmentID, &v.Weekday, &v.PeriodStart, &v.PeriodEnd, &v.ClassName, &v.SubjectName, &v.TeacherName, &v.StartsAt, &v.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	periods, err := queryRevisionPeriods(ctx, tx, revisionID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].TimeSegments = entrySegments(out[i], periods)
	}
	return out, nil
}

func (s *Store) AddTimetableEntry(ctx context.Context, actor, revisionID int64, weekday int, classID, subjectID, teacherID int64, periodStart, periodEnd int) error {
	if revisionID < 1 || weekday < 1 || weekday > 7 || classID < 1 || subjectID < 1 || teacherID < 1 || periodStart < 1 || periodEnd < periodStart {
		return ValidationError("Lengkapi hari, kelas, mata pelajaran, guru, dan rentang jam yang valid.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = saveTimetableEntryTx(ctx, tx, actor, revisionID, 0, weekday, classID, subjectID, teacherID, periodStart, periodEnd); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CorrectTimetableEntry(ctx context.Context, actor, revisionID, entryID int64, weekday int, classID, subjectID, teacherID int64, periodStart, periodEnd int) error {
	if revisionID < 1 || entryID < 1 || weekday < 1 || weekday > 7 || classID < 1 || subjectID < 1 || teacherID < 1 || periodStart < 1 || periodEnd < periodStart {
		return ValidationError("Lengkapi data koreksi jadwal.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = saveTimetableEntryTx(ctx, tx, actor, revisionID, entryID, weekday, classID, subjectID, teacherID, periodStart, periodEnd); err != nil {
		return err
	}
	return tx.Commit()
}
func saveTimetableEntryTx(ctx context.Context, tx *sql.Tx, actor, revisionID, entryID int64, weekday int, classID, subjectID, teacherID int64, periodStart, periodEnd int) error {
	var err error
	var semesterID int64
	var date time.Time
	err = tx.QueryRowContext(ctx, `SELECT semester_id,effective_from FROM app.weekly_timetable_revisions WHERE id=$1 AND status='draft' FOR UPDATE`, revisionID).Scan(&semesterID, &date)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	var assignmentID int64
	err = tx.QueryRowContext(ctx, `SELECT a.id FROM app.teaching_assignments a JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.classes c ON c.id=a.class_id AND c.school_year_id=sm.school_year_id JOIN app.teachers t ON t.id=a.teacher_id JOIN app.users u ON u.id=t.user_id JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' JOIN app.subjects s ON s.id=a.subject_id WHERE a.semester_id=$1 AND a.class_id=$2 AND a.subject_id=$3 AND a.teacher_id=$4 AND a.effective_from<=$5 AND (a.effective_until IS NULL OR a.effective_until>$5) AND t.active AND u.active AND s.active`, semesterID, classID, subjectID, teacherID, date).Scan(&assignmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationError("Guru belum ditugaskan pada kelas dan mata pelajaran ini untuk tanggal berlaku jadwal.")
	}
	if err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.weekly_timetable_periods WHERE revision_id=$1 AND weekday=$2 AND period_no BETWEEN $3 AND $4`, revisionID, weekday, periodStart, periodEnd).Scan(&count); err != nil {
		return err
	}
	if count != periodEnd-periodStart+1 {
		return ValidationError("Rentang jam harus memakai nomor yang sudah diatur secara berurutan.")
	}
	var conflict bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.weekly_timetable_entries e JOIN app.weekly_timetable_periods ep ON ep.revision_id=e.revision_id AND ep.weekday=e.weekday AND ep.period_no BETWEEN e.period_start AND e.period_end JOIN app.weekly_timetable_periods np ON np.revision_id=$1 AND np.weekday=$4 AND np.period_no BETWEEN $2 AND $3 WHERE e.revision_id=$1 AND e.weekday=$4 AND (e.class_id=$5 OR e.teacher_id=$6) AND e.id<>$7 AND ep.starts_at<np.ends_at AND ep.ends_at>np.starts_at)`, revisionID, periodStart, periodEnd, weekday, classID, teacherID, entryID).Scan(&conflict)
	if err != nil {
		return err
	}
	if conflict {
		return ValidationError("Jadwal bentrok dengan kelas atau guru pada waktu tersebut.")
	}
	existing, err := queryTimetableEntries(ctx, tx, revisionID, 0)
	if err != nil {
		return err
	}
	var before *TimetableEntry
	filtered := existing[:0]
	for i := range existing {
		if existing[i].ID == entryID {
			value := existing[i]
			before = &value
		} else {
			filtered = append(filtered, existing[i])
		}
	}
	if entryID > 0 && before == nil {
		return ErrNotFound
	}
	existing = filtered
	candidate := TimetableEntry{ClassID: classID, SubjectID: subjectID, TeacherID: teacherID, AssignmentID: assignmentID, Weekday: weekday, PeriodStart: periodStart, PeriodEnd: periodEnd}
	if err = validateRevisionPlan(ctx, tx, revisionID, semesterID, date, append(existing, candidate), false); err != nil {
		return err
	}
	var id int64
	if entryID == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO app.weekly_timetable_entries(revision_id,weekday,class_id,subject_id,teacher_id,assignment_id,period_start,period_end) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, revisionID, weekday, classID, subjectID, teacherID, assignmentID, periodStart, periodEnd).Scan(&id)
	} else {
		id = entryID
		_, err = tx.ExecContext(ctx, `UPDATE app.weekly_timetable_entries SET weekday=$1,class_id=$2,subject_id=$3,teacher_id=$4,assignment_id=$5,period_start=$6,period_end=$7 WHERE id=$8 AND revision_id=$9`, weekday, classID, subjectID, teacherID, assignmentID, periodStart, periodEnd, id, revisionID)
		if err == nil {
			b, e := json.Marshal(before)
			if e != nil {
				return e
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,before_state,after_state) VALUES($1,$2,'entry_removed',$3,'{"reason":"edit entri"}'::jsonb)`, revisionID, actor, string(b))
		}
	}

	if err != nil {
		return dbError(err)
	}
	after, _ := json.Marshal(map[string]any{"entry_id": id, "weekday": weekday, "class_id": classID, "subject_id": subjectID, "teacher_id": teacherID, "period_start": periodStart, "period_end": periodEnd})
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,after_state) VALUES($1,$2,'entry_added',$3)`, revisionID, actor, after); err != nil {
		return err
	}
	return nil
}

func (s *Store) RemoveTimetableEntry(ctx context.Context, actor, revisionID, entryID int64) error {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT jsonb_build_object('entry_id',e.id,'weekday',e.weekday,'class_id',e.class_id,'subject_id',e.subject_id,'teacher_id',e.teacher_id,'period_start',e.period_start,'period_end',e.period_end)::text FROM app.weekly_timetable_entries e JOIN app.weekly_timetable_revisions r ON r.id=e.revision_id WHERE e.id=$1 AND e.revision_id=$2 AND r.status='draft' FOR UPDATE OF r`, entryID, revisionID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM app.weekly_timetable_entries WHERE id=$1`, entryID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,before_state,after_state) VALUES($1,$2,'entry_removed',$3,'{}'::jsonb)`, revisionID, actor, state); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PublishTimetable(ctx context.Context, actor, revisionID int64) error {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var semesterID int64
	var date time.Time
	err = tx.QueryRowContext(ctx, `SELECT semester_id,effective_from FROM app.weekly_timetable_revisions WHERE id=$1 AND status='draft' FOR UPDATE`, revisionID).Scan(&semesterID, &date)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.weekly_timetable_entries WHERE revision_id=$1`, revisionID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return ValidationError("Tambahkan setidaknya satu jadwal sebelum publikasi.")
	}
	var invalid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.weekly_timetable_entries e LEFT JOIN app.teaching_assignments a ON a.id=e.assignment_id AND a.semester_id=$2 AND a.class_id=e.class_id AND a.subject_id=e.subject_id AND a.teacher_id=e.teacher_id AND a.effective_from<=$3 AND (a.effective_until IS NULL OR a.effective_until>$3) JOIN app.teachers t ON t.id=e.teacher_id JOIN app.users u ON u.id=t.user_id WHERE e.revision_id=$1 AND (a.id IS NULL OR NOT t.active OR NOT u.active OR NOT EXISTS(SELECT 1 FROM app.user_roles ur WHERE ur.user_id=u.id AND ur.role_code='guru_mapel') OR NOT EXISTS(SELECT 1 FROM app.semesters sm JOIN app.classes c ON c.school_year_id=sm.school_year_id WHERE sm.id=$2 AND c.id=e.class_id)))`, revisionID, semesterID, date).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return ValidationError("Ada jadwal tanpa penugasan aktif yang sesuai; periksa kembali daftar.")
	}
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.weekly_timetable_entries a JOIN app.weekly_timetable_entries b ON b.revision_id=a.revision_id AND b.weekday=a.weekday AND b.id>a.id AND (b.class_id=a.class_id OR b.teacher_id=a.teacher_id) JOIN app.weekly_timetable_periods ap ON ap.revision_id=a.revision_id AND ap.weekday=a.weekday AND ap.period_no BETWEEN a.period_start AND a.period_end JOIN app.weekly_timetable_periods bp ON bp.revision_id=b.revision_id AND bp.weekday=b.weekday AND bp.period_no BETWEEN b.period_start AND b.period_end WHERE a.revision_id=$1 AND ap.starts_at<bp.ends_at AND ap.ends_at>bp.starts_at)`, revisionID).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return ValidationError("Draf masih memiliki benturan jadwal.")
	}
	entries, err := queryTimetableEntries(ctx, tx, revisionID, 0)
	if err != nil {
		return err
	}
	if err = validateRevisionPlan(ctx, tx, revisionID, semesterID, date, entries, true); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.weekly_timetable_revisions SET status='published' ,published_by=$1,published_at=now() WHERE id=$2`, actor, revisionID); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]any{"status": "published", "effective_from": date.Format("2006-01-02")})
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,after_state) VALUES($1,$2,'published',$3)`, revisionID, actor, after); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PublishedTimetableForTeacher(ctx context.Context, actor int64) (TimetableRevision, []TimetableEntry, error) {
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return TimetableRevision{}, nil, err
	}
	defer tx.Rollback()
	var teacherID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active`, actor).Scan(&teacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return TimetableRevision{}, nil, ErrForbidden
	}
	if err != nil {
		return TimetableRevision{}, nil, err
	}
	var rev TimetableRevision
	err = tx.QueryRowContext(ctx, `SELECT r.id,r.semester_id,y.name||' · Semester '||sm.number::text,r.revision_no,r.status,r.effective_from FROM app.weekly_timetable_revisions r JOIN app.semesters sm ON sm.id=r.semester_id JOIN app.school_years y ON y.id=sm.school_year_id WHERE r.status='published' AND r.effective_from<=CURRENT_DATE AND sm.active ORDER BY r.effective_from DESC,r.revision_no DESC LIMIT 1`).Scan(&rev.ID, &rev.SemesterID, &rev.SemesterName, &rev.Revision, &rev.Status, &rev.EffectiveFrom)
	if errors.Is(err, sql.ErrNoRows) {
		return TimetableRevision{}, nil, nil
	}
	if err != nil {
		return rev, nil, err
	}
	entries, err := queryTimetableEntries(ctx, tx, rev.ID, teacherID)
	if err != nil {
		return rev, nil, err
	}
	snapshot, err := queryPlanSnapshot(ctx, tx, rev.ID)
	if err != nil {
		return rev, nil, err
	}
	if snapshot != nil {
		entries = timetableTimeline(entries, snapshot.Blocks)
	}
	return rev, entries, nil
}

func (s *Store) PublishedTimetables(ctx context.Context, actor, semesterID int64) ([]TimetableRevision, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.semester_id,y.name||' · Semester '||sm.number::text,r.revision_no,r.status,r.effective_from FROM app.weekly_timetable_revisions r JOIN app.semesters sm ON sm.id=r.semester_id JOIN app.school_years y ON y.id=sm.school_year_id WHERE r.semester_id=$1 AND r.status='published' ORDER BY r.effective_from DESC,r.revision_no DESC`, semesterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revisions []TimetableRevision
	for rows.Next() {
		var revision TimetableRevision
		if err = rows.Scan(&revision.ID, &revision.SemesterID, &revision.SemesterName, &revision.Revision, &revision.Status, &revision.EffectiveFrom); err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}
