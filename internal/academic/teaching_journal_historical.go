package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// TeachingJournalHistoricalCandidate adalah slot resmi yang dapat dipilih
// Kurikulum untuk pemetaan satu occurrence pra-cutover.
type TeachingJournalHistoricalCandidate struct {
	SemesterID, RevisionID, EntryID, AssignmentID int64
	ClassID, SubjectID, TeacherID, OwnerUserID    int64
	Date, SemesterSnapshot, ClassSnapshot         string
	SubjectSnapshot, TeacherSnapshot              string
	AttendanceReason                              string
	Periods                                       []TeachingJournalPeriodSnapshot
}

type TeachingJournalHistoricalMappingInput struct {
	Date       string
	RevisionID int64
	EntryID    int64
	Reason     string
}

type TeachingJournalHistoricalMappingResult struct {
	ID, LogicalLessonID int64
}

type historicalJournalSource struct {
	SemesterID, RevisionID, EntryID, AssignmentID   int64
	ClassID, SubjectID, TeacherID, OwnerUserID      int64
	ClassSnapshot, SubjectSnapshot, TeacherSnapshot string
	Periods                                         []TeachingJournalPeriodSnapshot
}

func loadHistoricalJournalSource(ctx context.Context, tx *sql.Tx, actor, logicalLessonID int64, date string) (*historicalJournalSource, error) {
	var source historicalJournalSource
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT semester_id,revision_id,timetable_entry_id,assignment_id,class_id,subject_id,teacher_id,owner_user_id,class_snapshot,subject_snapshot,teacher_snapshot,periods_snapshot
		FROM app.teaching_journal_historical_mappings WHERE logical_lesson_id=$1 AND meeting_date=$2::date`, logicalLessonID, date).Scan(
		&source.SemesterID, &source.RevisionID, &source.EntryID, &source.AssignmentID, &source.ClassID, &source.SubjectID, &source.TeacherID, &source.OwnerUserID,
		&source.ClassSnapshot, &source.SubjectSnapshot, &source.TeacherSnapshot, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if source.OwnerUserID != actor {
		return nil, ErrForbidden
	}
	if err = json.Unmarshal(raw, &source.Periods); err != nil {
		return nil, err
	}
	if len(source.Periods) == 0 {
		return nil, ValidationError("Snapshot JP occurrence historis tidak tersedia.")
	}
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM app.teaching_journal_historical_mappings hm
		JOIN app.teaching_assignments a ON a.id=hm.assignment_id AND a.semester_id=hm.semester_id AND a.class_id=hm.class_id AND a.subject_id=hm.subject_id AND a.teacher_id=hm.teacher_id AND a.effective_from<=hm.meeting_date AND (a.effective_until IS NULL OR a.effective_until>hm.meeting_date)
		JOIN app.teachers t ON t.id=hm.teacher_id AND t.active AND t.user_id=hm.owner_user_id
		JOIN app.users u ON u.id=hm.owner_user_id AND u.active
		JOIN app.user_roles ur ON ur.user_id=u.id AND ur.role_code='guru_mapel'
		JOIN app.weekly_timetable_entries e ON e.id=hm.timetable_entry_id AND e.revision_id=hm.revision_id AND e.assignment_id=hm.assignment_id AND e.teacher_id=hm.teacher_id AND e.class_id=hm.class_id AND e.subject_id=hm.subject_id
		JOIN app.weekly_timetable_revisions r ON r.id=hm.revision_id AND r.semester_id=hm.semester_id AND r.status='published' AND r.effective_from<=hm.meeting_date
		WHERE hm.logical_lesson_id=$1 AND hm.meeting_date=$2::date)`, logicalLessonID, date).Scan(&valid)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ValidationError("Pemetaan historis, assignment, atau identitas guru tidak lagi memenuhi syarat.")
	}
	return &source, nil
}

// TeachingJournalCutoverState hanya menampilkan status cutover kepada Kurikulum.
func (s *Store) TeachingJournalCutoverState(ctx context.Context, actor int64) (string, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var date string
	err = tx.QueryRowContext(ctx, `SELECT cutover_date::text FROM app.teaching_journal_cutover_batches ORDER BY cutover_date,id LIMIT 1`).Scan(&date)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return date, err
}

// TeachingJournalHistoricalCandidates memberi Kurikulum pilihan slot dari
// revision resmi efektif pada tanggal yang dipilih, tanpa menebak kesinambungan.
func (s *Store) TeachingJournalHistoricalCandidates(ctx context.Context, actor int64, date string) ([]TeachingJournalHistoricalCandidate, error) {
	if _, err := civilDate(date); err != nil {
		return nil, err
	}
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = ensureHistoricalDateBeforeCutover(ctx, tx, date); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, historicalCandidatesSQL, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TeachingJournalHistoricalCandidate, 0)
	for rows.Next() {
		var item TeachingJournalHistoricalCandidate
		var raw []byte
		if err = rows.Scan(&item.SemesterID, &item.RevisionID, &item.EntryID, &item.AssignmentID,
			&item.ClassID, &item.SubjectID, &item.TeacherID, &item.OwnerUserID, &item.Date,
			&item.SemesterSnapshot, &item.ClassSnapshot, &item.SubjectSnapshot, &item.TeacherSnapshot, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &item.Periods); err != nil {
			return nil, err
		}
		if len(item.Periods) == 0 {
			continue
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	// Tutup result set sebelum query evaluasi presensi pada transaksi yang sama.
	// Jika tidak, beberapa driver menandai koneksi sibuk saat daftar memuat banyak slot.
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		eligibility, e := teacherLessonEligibilityTx(ctx, tx, items[i].TeacherID, date, items[i].Periods[len(items[i].Periods)-1].EndsAt)
		if e != nil {
			return nil, e
		}
		if !eligibility.Allowed {
			items[i].AttendanceReason = eligibility.Reason
		}
	}
	return items, nil
}

const historicalCandidatesSQL = `WITH effective AS (
	SELECT DISTINCT ON (r.semester_id) r.id,r.semester_id,r.revision_no
	FROM app.weekly_timetable_revisions r
	JOIN app.semester_calendars sc ON sc.semester_id=r.semester_id AND $1::date BETWEEN sc.starts_on AND sc.ends_on
	WHERE r.status='published' AND r.effective_from<=$1::date
	ORDER BY r.semester_id,r.effective_from DESC,r.revision_no DESC
)
SELECT r.semester_id,r.id,e.id,e.assignment_id,e.class_id,e.subject_id,e.teacher_id,t.user_id,$1::date::text,
	y.name||' · Semester '||sm.number::text,c.grade::text||c.name,s.name,t.name,
	coalesce((SELECT jsonb_agg(jsonb_build_object('ordinal',p.ordinal,'period_no',p.period_no,'starts_at',p.starts_at,'ends_at',p.ends_at) ORDER BY p.ordinal)
		FROM (SELECT row_number() OVER(ORDER BY wp.period_no)::integer AS ordinal,wp.period_no,wp.starts_at::text,wp.ends_at::text
			FROM app.weekly_timetable_periods wp WHERE wp.revision_id=r.id AND wp.weekday=e.weekday AND wp.period_no BETWEEN e.period_start AND e.period_end) p),'[]'::jsonb)
FROM effective r
JOIN app.weekly_timetable_entries e ON e.revision_id=r.id AND e.weekday=EXTRACT(ISODOW FROM $1::date)::integer
JOIN app.semesters sm ON sm.id=r.semester_id JOIN app.school_years y ON y.id=sm.school_year_id
JOIN app.classes c ON c.id=e.class_id AND c.school_year_id=sm.school_year_id
JOIN app.subjects s ON s.id=e.subject_id AND s.active
JOIN app.teachers t ON t.id=e.teacher_id AND t.active
JOIN app.users u ON u.id=t.user_id AND u.active
JOIN app.user_roles ur ON ur.user_id=u.id AND ur.role_code='guru_mapel'
JOIN app.teaching_assignments a ON a.id=e.assignment_id AND a.semester_id=r.semester_id AND a.class_id=e.class_id AND a.subject_id=e.subject_id AND a.teacher_id=e.teacher_id AND a.effective_from<=$1::date AND (a.effective_until IS NULL OR a.effective_until>$1::date)
WHERE NOT EXISTS(SELECT 1 FROM app.teaching_journal_historical_mappings hm WHERE hm.revision_id=r.id AND hm.timetable_entry_id=e.id AND hm.meeting_date=$1::date)
	AND NOT EXISTS(SELECT 1 FROM app.teaching_journals j WHERE j.revision_id=r.id AND j.timetable_entry_id=e.id AND j.meeting_date=$1::date)
ORDER BY c.grade,c.name,s.name,e.period_start,e.id`

func ensureHistoricalDateBeforeCutover(ctx context.Context, tx *sql.Tx, date string) error {
	var cutoff string
	err := tx.QueryRowContext(ctx, `SELECT cutover_date::text FROM app.teaching_journal_cutover_batches ORDER BY cutover_date,id LIMIT 1`).Scan(&cutoff)
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationError("Jalankan cutover terlebih dahulu sebelum menetapkan occurrence historis.")
	}
	if err != nil {
		return err
	}
	if date >= cutoff {
		return ValidationError("Pemetaan ini hanya untuk occurrence sebelum tanggal cutover.")
	}
	return nil
}

func historicalJournalOccurrencesForTeacher(ctx context.Context, tx *sql.Tx, actor, teacherID int64, date string) ([]TeachingJournalOccurrenceOption, bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT hm.logical_lesson_id,hm.semester_id,hm.revision_id,hm.timetable_entry_id,
		y.name||' · Semester '||sm.number::text,r.revision_no,hm.class_snapshot,hm.subject_snapshot,hm.teacher_snapshot,hm.periods_snapshot
		FROM app.teaching_journal_historical_mappings hm
		JOIN app.logical_lessons l ON l.id=hm.logical_lesson_id AND l.semester_id=hm.semester_id
		JOIN app.semesters sm ON sm.id=hm.semester_id JOIN app.school_years y ON y.id=sm.school_year_id
		JOIN app.weekly_timetable_revisions r ON r.id=hm.revision_id AND r.semester_id=hm.semester_id
		JOIN app.weekly_timetable_entries e ON e.id=hm.timetable_entry_id AND e.revision_id=hm.revision_id AND e.assignment_id=hm.assignment_id AND e.class_id=hm.class_id AND e.subject_id=hm.subject_id AND e.teacher_id=hm.teacher_id AND e.weekday=EXTRACT(ISODOW FROM $3::date)::integer
		JOIN app.teaching_assignments a ON a.id=hm.assignment_id AND a.semester_id=hm.semester_id AND a.class_id=hm.class_id AND a.subject_id=hm.subject_id AND a.teacher_id=hm.teacher_id AND a.effective_from<=hm.meeting_date AND (a.effective_until IS NULL OR a.effective_until>hm.meeting_date)
		JOIN app.teachers t ON t.id=hm.teacher_id AND t.user_id=hm.owner_user_id AND t.active
		JOIN app.users u ON u.id=hm.owner_user_id AND u.active
		JOIN app.user_roles ur ON ur.user_id=u.id AND ur.role_code='guru_mapel'
		WHERE hm.owner_user_id=$1 AND hm.teacher_id=$2 AND hm.meeting_date=$3::date
		AND NOT EXISTS(SELECT 1 FROM app.teaching_journals j WHERE j.logical_lesson_id=hm.logical_lesson_id AND j.meeting_date=hm.meeting_date)
		ORDER BY hm.id`, actor, teacherID, date)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]TeachingJournalOccurrenceOption, 0)
	for rows.Next() {
		var item TeachingJournalOccurrenceOption
		var raw []byte
		item.Date = date
		if err = rows.Scan(&item.LogicalLessonID, &item.SemesterID, &item.RevisionID, &item.EntryID, &item.SemesterSnapshot, &item.RevisionNumber, &item.ClassSnapshot, &item.SubjectSnapshot, &item.TeacherSnapshot, &raw); err != nil {
			return nil, false, err
		}
		if err = json.Unmarshal(raw, &item.Periods); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	if err = rows.Close(); err != nil {
		return nil, false, err
	}
	var hasMapping bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_journal_historical_mappings WHERE meeting_date=$1::date)`, date).Scan(&hasMapping); err != nil {
		return nil, false, err
	}
	evidence, err := teacherLessonAttendanceEvidenceTx(ctx, tx, teacherID, date)
	if err != nil {
		return nil, false, err
	}
	for i := range items {
		if len(items[i].Periods) == 0 {
			items[i].AttendanceReason = "Snapshot waktu resmi pertemuan tidak tersedia."
			continue
		}
		eligibility := evaluateTeacherLessonEligibility(evidence, date, items[i].Periods[len(items[i].Periods)-1].EndsAt)
		if !eligibility.Allowed {
			items[i].AttendanceReason = eligibility.Reason
		}
	}
	return items, hasMapping, nil
}

// MapTeachingJournalHistoricalOccurrence mencatat keputusan Kurikulum untuk
// tepat satu slot revision dan tanggal; transaksi juga memvalidasi assignment
// resmi dan gate presensi sebelum memberi logical identity.
func (s *Store) MapTeachingJournalHistoricalOccurrence(ctx context.Context, actor int64, in TeachingJournalHistoricalMappingInput) (TeachingJournalHistoricalMappingResult, error) {
	var result TeachingJournalHistoricalMappingResult
	date, err := civilDate(in.Date)
	if err != nil {
		return result, err
	}
	dateText := date.Format("2006-01-02")
	in.Reason = strings.TrimSpace(in.Reason)
	if in.RevisionID < 1 || in.EntryID < 1 || len([]rune(in.Reason)) == 0 || len([]rune(in.Reason)) > 500 {
		return result, ValidationError("Pilih slot resmi dan isi alasan pemetaan, maksimal 500 karakter.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = ensureHistoricalDateBeforeCutover(ctx, tx, dateText); err != nil {
		return result, err
	}
	var batchID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM app.teaching_journal_cutover_batches ORDER BY cutover_date,id LIMIT 1`).Scan(&batchID); err != nil {
		return result, err
	}
	var candidate TeachingJournalHistoricalCandidate
	var periodsJSON []byte
	err = tx.QueryRowContext(ctx, historicalCandidateByEntrySQL, dateText, in.RevisionID, in.EntryID).Scan(
		&candidate.SemesterID, &candidate.RevisionID, &candidate.EntryID, &candidate.AssignmentID,
		&candidate.ClassID, &candidate.SubjectID, &candidate.TeacherID, &candidate.OwnerUserID, &candidate.Date,
		&candidate.SemesterSnapshot, &candidate.ClassSnapshot, &candidate.SubjectSnapshot, &candidate.TeacherSnapshot, &periodsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ValidationError("Slot tidak sesuai dengan revision terbit efektif, assignment aktif, atau identitas guru pada tanggal tersebut.")
	}
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal(periodsJSON, &candidate.Periods); err != nil {
		return result, err
	}
	if len(candidate.Periods) == 0 {
		return result, ValidationError("Snapshot JP resmi untuk occurrence tidak tersedia.")
	}
	eligibility, err := teacherLessonEligibilityTx(ctx, tx, candidate.TeacherID, dateText, candidate.Periods[len(candidate.Periods)-1].EndsAt)
	if err != nil {
		return result, err
	}
	if err = teacherLessonEligibilityError(eligibility); err != nil {
		return result, err
	}
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.logical_lessons(semester_id,created_by) VALUES($1,$2) RETURNING id`, candidate.SemesterID, actor).Scan(&result.LogicalLessonID); err != nil {
		return TeachingJournalHistoricalMappingResult{}, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO app.teaching_journal_historical_mappings(cutover_batch_id,logical_lesson_id,semester_id,revision_id,timetable_entry_id,meeting_date,assignment_id,class_id,subject_id,teacher_id,owner_user_id,class_snapshot,subject_snapshot,teacher_snapshot,periods_snapshot,actor_id,reason)
		VALUES($1,$2,$3,$4,$5,$6::date,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16,$17) RETURNING id`, batchID, result.LogicalLessonID, candidate.SemesterID, candidate.RevisionID, candidate.EntryID, dateText, candidate.AssignmentID, candidate.ClassID, candidate.SubjectID, candidate.TeacherID, candidate.OwnerUserID, candidate.ClassSnapshot, candidate.SubjectSnapshot, candidate.TeacherSnapshot, periodsJSON, actor, in.Reason).Scan(&result.ID)
	if err != nil {
		return TeachingJournalHistoricalMappingResult{}, dbError(err)
	}
	if err = tx.Commit(); err != nil {
		return TeachingJournalHistoricalMappingResult{}, dbError(err)
	}
	return result, nil
}

const historicalCandidateByEntrySQL = `WITH effective AS (
	SELECT DISTINCT ON (r.semester_id) r.id,r.semester_id,r.revision_no FROM app.weekly_timetable_revisions r
	JOIN app.semester_calendars sc ON sc.semester_id=r.semester_id AND $1::date BETWEEN sc.starts_on AND sc.ends_on
	WHERE r.status='published' AND r.effective_from<=$1::date
	ORDER BY r.semester_id,r.effective_from DESC,r.revision_no DESC
)
SELECT r.semester_id,r.id,e.id,e.assignment_id,e.class_id,e.subject_id,e.teacher_id,t.user_id,$1::date::text,
	y.name||' · Semester '||sm.number::text,c.grade::text||c.name,s.name,t.name,
	coalesce((SELECT jsonb_agg(jsonb_build_object('ordinal',p.ordinal,'period_no',p.period_no,'starts_at',p.starts_at,'ends_at',p.ends_at) ORDER BY p.ordinal)
		FROM (SELECT row_number() OVER(ORDER BY wp.period_no)::integer AS ordinal,wp.period_no,wp.starts_at::text,wp.ends_at::text
			FROM app.weekly_timetable_periods wp WHERE wp.revision_id=r.id AND wp.weekday=e.weekday AND wp.period_no BETWEEN e.period_start AND e.period_end) p),'[]'::jsonb)
FROM effective r JOIN app.weekly_timetable_entries e ON e.revision_id=r.id AND e.id=$3 AND e.weekday=EXTRACT(ISODOW FROM $1::date)::integer
JOIN app.semesters sm ON sm.id=r.semester_id JOIN app.school_years y ON y.id=sm.school_year_id
JOIN app.classes c ON c.id=e.class_id AND c.school_year_id=sm.school_year_id
JOIN app.subjects s ON s.id=e.subject_id AND s.active
JOIN app.teachers t ON t.id=e.teacher_id AND t.active JOIN app.users u ON u.id=t.user_id AND u.active
JOIN app.user_roles ur ON ur.user_id=u.id AND ur.role_code='guru_mapel'
JOIN app.teaching_assignments a ON a.id=e.assignment_id AND a.semester_id=r.semester_id AND a.class_id=e.class_id AND a.subject_id=e.subject_id AND a.teacher_id=e.teacher_id AND a.effective_from<=$1::date AND (a.effective_until IS NULL OR a.effective_until>$1::date)
WHERE r.id=$2
	AND NOT EXISTS(SELECT 1 FROM app.teaching_journal_historical_mappings hm WHERE hm.revision_id=r.id AND hm.timetable_entry_id=e.id AND hm.meeting_date=$1::date)
	AND NOT EXISTS(SELECT 1 FROM app.teaching_journals j WHERE j.revision_id=r.id AND j.timetable_entry_id=e.id AND j.meeting_date=$1::date)`
