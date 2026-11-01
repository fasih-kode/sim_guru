package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

var journalRequestKeyPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type TeachingJournalInput struct {
	LogicalLessonID  int64
	OccurrenceDate   time.Time
	RequestKey       string
	State            string
	Outcome          string
	Material         string
	ActivityNotes    string
	NotHeldReason    string
	CorrectionReason string
	ExpectedVersion  int
}

type TeachingJournalResult struct {
	ID         int64
	Version    int
	RevisionID int64
	EntryID    int64
	RecordedAt time.Time
	Replayed   bool
}

// SaveTeachingJournal menyimpan draf/final sebagai versi append-only. Snapshot
// JP berasal dari revision efektif untuk tanggal occurrence dan disalin saat koreksi.
func (s *Store) SaveTeachingJournal(ctx context.Context, actor int64, in TeachingJournalInput) (TeachingJournalResult, error) {
	var result TeachingJournalResult
	date := in.OccurrenceDate.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	in.Material = strings.TrimSpace(in.Material)
	in.ActivityNotes = strings.TrimSpace(in.ActivityNotes)
	in.NotHeldReason = strings.TrimSpace(in.NotHeldReason)
	in.CorrectionReason = strings.TrimSpace(in.CorrectionReason)
	if in.LogicalLessonID < 1 || in.OccurrenceDate.IsZero() || !journalRequestKeyPattern.MatchString(in.RequestKey) {
		return result, ValidationError("Pertemuan, tanggal, dan kunci permintaan jurnal tidak valid.")
	}
	if err := validateTeachingJournalContent(in); err != nil {
		return result, err
	}
	if in.ExpectedVersion < 0 {
		return result, ValidationError("Versi jurnal tidak valid.")
	}
	canonical := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%d", in.LogicalLessonID, date, in.State, in.Outcome, in.Material, in.ActivityNotes, in.NotHeldReason, in.ExpectedVersion)
	if in.ExpectedVersion > 0 {
		canonical += "|" + in.CorrectionReason
	}
	digest := sha256.Sum256([]byte(canonical))
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return result, err
	}
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return result, err
	}
	// Retry key pertama-tama dipulihkan dari histori versi sebelum memproses occurrence.
	var replayHash []byte
	err = tx.QueryRowContext(ctx, `SELECT j.id,v.version_no,v.recorded_at,j.revision_id,j.timetable_entry_id,v.request_hash FROM app.teaching_journals j JOIN app.teaching_journal_versions v ON v.journal_id=j.id WHERE j.logical_lesson_id=$1 AND j.meeting_date=$2 AND v.request_key=$3`, in.LogicalLessonID, date, in.RequestKey).Scan(&result.ID, &result.Version, &result.RecordedAt, &result.RevisionID, &result.EntryID, &replayHash)
	if err == nil {
		if !equalBytes(replayHash, digest[:]) {
			return TeachingJournalResult{}, ErrConflict
		}
		result.Replayed = true
		if e := tx.Commit(); e != nil {
			return TeachingJournalResult{}, e
		}
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TeachingJournalResult{}, err
	}
	var cutoff string
	err = tx.QueryRowContext(ctx, `SELECT cutover_date::text FROM app.teaching_journal_cutover_batches ORDER BY id LIMIT 1`).Scan(&cutoff)
	if errors.Is(err, sql.ErrNoRows) {
		return TeachingJournalResult{}, ErrNotFound
	}
	if err != nil {
		return TeachingJournalResult{}, err
	}
	var historical *historicalJournalSource
	if date < cutoff {
		historical, err = loadHistoricalJournalSource(ctx, tx, actor, in.LogicalLessonID, date)
		if err != nil {
			return TeachingJournalResult{}, fmt.Errorf("validasi sumber occurrence historis: %w", err)
		}
		if historical == nil {
			return TeachingJournalResult{}, ValidationError("Jurnal sebelum cutover hanya dapat dibuat untuk occurrence yang telah dipetakan Kurikulum.")
		}
	}
	var teacherID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active FOR KEY SHARE`, actor).Scan(&teacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return TeachingJournalResult{}, ErrForbidden
	}
	if err != nil {
		return TeachingJournalResult{}, fmt.Errorf("memeriksa identitas guru aktif: %w", err)
	}
	if historical != nil && historical.TeacherID != teacherID {
		return TeachingJournalResult{}, ErrForbidden
	}
	var journalID, ownerUserID int64
	var currentVersion int
	var currentState string
	var isNew bool
	err = tx.QueryRowContext(ctx, `SELECT j.id,j.owner_user_id,j.current_version_no,v.state FROM app.teaching_journals j JOIN app.teaching_journal_versions v ON v.journal_id=j.id AND v.version_no=j.current_version_no WHERE j.logical_lesson_id=$1 AND j.meeting_date=$2 FOR UPDATE OF j`, in.LogicalLessonID, date).Scan(&journalID, &ownerUserID, &currentVersion, &currentState)
	isNew = errors.Is(err, sql.ErrNoRows)
	if err != nil && !isNew {
		return TeachingJournalResult{}, err
	}
	if !isNew && ownerUserID != actor {
		return TeachingJournalResult{}, ErrForbidden
	}
	var semesterID int64
	if historical != nil {
		semesterID = historical.SemesterID
	} else {
		err = tx.QueryRowContext(ctx, `SELECT semester_id FROM app.logical_lessons WHERE id=$1`, in.LogicalLessonID).Scan(&semesterID)
		if errors.Is(err, sql.ErrNoRows) {
			return TeachingJournalResult{}, ErrNotFound
		}
		if err != nil {
			return TeachingJournalResult{}, err
		}
	}
	var lineagePending bool
	if historical == nil {
		err = tx.QueryRowContext(ctx, `SELECT COALESCE((
		SELECT ls.lineage_status='pending'
		FROM app.weekly_timetable_revisions r
		LEFT JOIN app.teaching_journal_lineage_status ls ON ls.revision_id=r.id
		WHERE r.semester_id=$1 AND r.status='published' AND r.effective_from<=$2::date
		ORDER BY r.effective_from DESC,r.revision_no DESC LIMIT 1
		),false)`, semesterID, date).Scan(&lineagePending)
		if err != nil {
			return TeachingJournalResult{}, err
		}
		if lineagePending {
			return TeachingJournalResult{}, ValidationError("Revisi jadwal efektif belum memiliki keputusan kesinambungan Kurikulum; jurnal belum dapat diubah.")
		}
	}
	var revisionID, entryID, assignmentID, classID, subjectID, officialTeacherID int64
	var className, subjectName, teacherName string
	if isNew {
		if historical != nil {
			revisionID, entryID, assignmentID, classID, subjectID, officialTeacherID = historical.RevisionID, historical.EntryID, historical.AssignmentID, historical.ClassID, historical.SubjectID, historical.TeacherID
			className, subjectName, teacherName = historical.ClassSnapshot, historical.SubjectSnapshot, historical.TeacherSnapshot
		} else {
			err = tx.QueryRowContext(ctx, `WITH effective AS (SELECT r.id FROM app.weekly_timetable_revisions r WHERE r.semester_id=$1 AND r.status='published' AND r.effective_from<=$2::date AND EXISTS(SELECT 1 FROM app.semester_calendars sc WHERE sc.semester_id=r.semester_id AND $2::date BETWEEN sc.starts_on AND sc.ends_on) ORDER BY r.effective_from DESC,r.revision_no DESC LIMIT 1) SELECT e.revision_id,e.id,e.assignment_id,e.class_id,e.subject_id,e.teacher_id,c.grade::text||c.name,s.name,t.name FROM effective r JOIN app.teaching_journal_lineage_decisions d ON d.to_revision_id=r.id AND d.logical_lesson_id=$3 AND d.target_entry_id IS NOT NULL JOIN app.weekly_timetable_entries e ON e.id=d.target_entry_id AND e.revision_id=r.id JOIN app.classes c ON c.id=e.class_id JOIN app.subjects s ON s.id=e.subject_id JOIN app.teachers t ON t.id=e.teacher_id WHERE e.teacher_id=$4 AND EXISTS(SELECT 1 FROM app.teaching_assignments a WHERE a.id=e.assignment_id AND a.semester_id=$1 AND a.class_id=e.class_id AND a.subject_id=e.subject_id AND a.teacher_id=e.teacher_id AND a.effective_from<=$2::date AND (a.effective_until IS NULL OR a.effective_until>$2::date))`, semesterID, date, in.LogicalLessonID, teacherID).Scan(&revisionID, &entryID, &assignmentID, &classID, &subjectID, &officialTeacherID, &className, &subjectName, &teacherName)
			if errors.Is(err, sql.ErrNoRows) {
				return TeachingJournalResult{}, ValidationError("Revision efektif, penugasan, atau lineage untuk tanggal occurrence tidak tersedia.")
			}
			if err != nil {
				return TeachingJournalResult{}, err
			}
		}
	} else {
		err = tx.QueryRowContext(ctx, `SELECT revision_id,timetable_entry_id,assignment_id,class_id,subject_id,teacher_id,class_snapshot,subject_snapshot,teacher_snapshot FROM app.teaching_journals WHERE id=$1`, journalID).Scan(&revisionID, &entryID, &assignmentID, &classID, &subjectID, &officialTeacherID, &className, &subjectName, &teacherName)
		if err != nil {
			return TeachingJournalResult{}, err
		}
		if historical != nil && (revisionID != historical.RevisionID || entryID != historical.EntryID || assignmentID != historical.AssignmentID || classID != historical.ClassID || subjectID != historical.SubjectID || officialTeacherID != historical.TeacherID) {
			return TeachingJournalResult{}, ErrConflict
		}
	}
	result.RevisionID = revisionID
	result.EntryID = entryID
	var scheduledEnd string
	if historical != nil {
		scheduledEnd = historical.Periods[len(historical.Periods)-1].EndsAt
		err = nil // SELECT jurnal baru sebelumnya menghasilkan sql.ErrNoRows secara normal.
	} else if isNew {
		err = tx.QueryRowContext(ctx, `SELECT max(p.ends_at)::text
			FROM app.weekly_timetable_entries e
			JOIN app.weekly_timetable_periods p ON p.revision_id=e.revision_id AND p.weekday=e.weekday AND p.period_no BETWEEN e.period_start AND e.period_end
			WHERE e.id=$1 AND e.revision_id=$2`, entryID, revisionID).Scan(&scheduledEnd)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT max(p.ends_at)::text
			FROM app.teaching_journal_versions v JOIN app.teaching_journal_version_periods p ON p.version_id=v.id
			WHERE v.journal_id=$1 AND v.version_no=$2`, journalID, currentVersion).Scan(&scheduledEnd)
	}
	if err != nil {
		return TeachingJournalResult{}, fmt.Errorf("membaca snapshot waktu terjadwal: %w", err)
	}
	eligibility, err := teacherLessonEligibilityTx(ctx, tx, teacherID, date, scheduledEnd)
	if err != nil {
		return TeachingJournalResult{}, fmt.Errorf("memeriksa bukti presensi untuk occurrence: %w", err)
	}
	if err = teacherLessonEligibilityError(eligibility); err != nil {
		return TeachingJournalResult{}, err
	}
	if isNew && in.ExpectedVersion != 0 {
		return TeachingJournalResult{}, ErrConflict
	}
	if !isNew && (currentVersion != in.ExpectedVersion || in.ExpectedVersion == 0) {
		return TeachingJournalResult{}, ErrConflict
	}
	if err = validateTeachingJournalTransition(isNew, currentState, in); err != nil {
		return TeachingJournalResult{}, err
	}
	if isNew {
		if err = tx.QueryRowContext(ctx, `INSERT INTO app.teaching_journals(logical_lesson_id,meeting_date,semester_id,revision_id,timetable_entry_id,assignment_id,class_id,subject_id,teacher_id,owner_user_id,class_snapshot,subject_snapshot,teacher_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`, in.LogicalLessonID, date, semesterID, revisionID, entryID, assignmentID, classID, subjectID, officialTeacherID, actor, className, subjectName, teacherName).Scan(&journalID); err != nil {
			return TeachingJournalResult{}, dbError(err)
		}
		currentVersion = 0
	}
	versionNo := currentVersion + 1
	if !isNew {
		res, e := tx.ExecContext(ctx, `UPDATE app.teaching_journals SET current_version_no=$1 WHERE id=$2 AND current_version_no=$3`, versionNo, journalID, currentVersion)
		if e != nil {
			return TeachingJournalResult{}, e
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return TeachingJournalResult{}, ErrConflict
		}
	}
	var correction any
	event := "created"
	if !isNew {
		if currentState == "final" {
			correction = in.CorrectionReason
			event = "corrected_final"
		} else {
			event = "saved_draft"
		}
	}
	if in.State == "final" && isNew {
		event = "finalized"
	}
	if !isNew && in.State == "final" && currentState != "final" {
		event = "finalized"
	}
	var versionID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app.teaching_journal_versions(journal_id,version_no,state,outcome,material,activity_notes,not_held_reason,actor_id,correction_reason,request_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id,recorded_at`, journalID, versionNo, in.State, in.Outcome, in.Material, in.ActivityNotes, in.NotHeldReason, actor, correction, in.RequestKey, digest[:]).Scan(&versionID, &result.RecordedAt)
	if err != nil {
		return TeachingJournalResult{}, dbError(err)
	}
	if isNew {
		type jp struct {
			number     int
			start, end string
		}
		var periods []jp
		if historical != nil {
			for _, p := range historical.Periods {
				periods = append(periods, jp{number: p.PeriodNo, start: p.StartsAt, end: p.EndsAt})
			}
		} else {
			weekday := int(in.OccurrenceDate.In(time.FixedZone("WIB", 7*60*60)).Weekday())
			if weekday == 0 {
				weekday = 7
			}
			periodRows, e := tx.QueryContext(ctx, `SELECT p.period_no,p.starts_at::text,p.ends_at::text FROM app.weekly_timetable_entries e JOIN app.weekly_timetable_periods p ON p.revision_id=e.revision_id AND p.weekday=e.weekday AND p.period_no BETWEEN e.period_start AND e.period_end WHERE e.id=$1 AND e.revision_id=$2 ORDER BY p.period_no`, entryID, revisionID)
			if e != nil {
				return TeachingJournalResult{}, e
			}
			for periodRows.Next() {
				var p jp
				if e = periodRows.Scan(&p.number, &p.start, &p.end); e != nil {
					periodRows.Close()
					return TeachingJournalResult{}, e
				}
				periods = append(periods, p)
			}
			if e = periodRows.Err(); e != nil {
				periodRows.Close()
				return TeachingJournalResult{}, e
			}
			periodRows.Close()
			var weekdayValid bool
			if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.weekly_timetable_entries WHERE id=$1 AND revision_id=$2 AND weekday=$3)`, entryID, revisionID, weekday).Scan(&weekdayValid); e != nil {
				return TeachingJournalResult{}, e
			}
			if !weekdayValid {
				return TeachingJournalResult{}, ValidationError("Occurrence tidak sesuai hari pada jadwal efektif.")
			}
		}
		if len(periods) == 0 {
			return TeachingJournalResult{}, ValidationError("Snapshot jam pelajaran revision efektif tidak tersedia.")
		}
		for i, p := range periods {
			if _, err = tx.ExecContext(ctx, `INSERT INTO app.teaching_journal_version_periods(version_id,ordinal,period_no,starts_at,ends_at) VALUES($1,$2,$3,$4,$5)`, versionID, i+1, p.number, p.start, p.end); err != nil {
				return TeachingJournalResult{}, err
			}
		}
	} else if _, err = tx.ExecContext(ctx, `INSERT INTO app.teaching_journal_version_periods(version_id,ordinal,period_no,starts_at,ends_at) SELECT $1,ordinal,period_no,starts_at,ends_at FROM app.teaching_journal_version_periods WHERE version_id=(SELECT id FROM app.teaching_journal_versions WHERE journal_id=$2 AND version_no=$3) ORDER BY ordinal`, versionID, journalID, currentVersion); err != nil {
		return TeachingJournalResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.teaching_journal_history(journal_id,version_id,version_no,event,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6)`, journalID, versionID, versionNo, event, actor, correction); err != nil {
		return TeachingJournalResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return TeachingJournalResult{}, dbError(err)
	}
	result.ID = journalID
	result.Version = versionNo
	return result, nil
}

func validateTeachingJournalContent(in TeachingJournalInput) error {
	if len([]rune(in.Material)) > 500 || len([]rune(in.ActivityNotes)) > 2000 || len([]rune(in.NotHeldReason)) > 500 || len([]rune(in.CorrectionReason)) > 500 {
		return ValidationError("Materi, catatan, atau alasan melebihi batas karakter.")
	}
	if in.State != "draft" && in.State != "final" {
		return ValidationError("Status jurnal tidak valid.")
	}
	if in.Outcome != "held" && in.Outcome != "not_held" {
		return ValidationError("Status pertemuan tidak valid.")
	}
	if in.Outcome == "not_held" && len([]rune(in.NotHeldReason)) == 0 {
		return ValidationError("Pertemuan tidak terlaksana wajib memiliki alasan.")
	}
	if in.Outcome == "held" && in.NotHeldReason != "" {
		return ValidationError("Alasan tidak terlaksana hanya diisi untuk pertemuan yang tidak terlaksana.")
	}
	if in.State == "final" && in.Outcome == "held" && len([]rune(in.Material)) == 0 {
		return ValidationError("Materi wajib diisi sebelum jurnal pembelajaran difinalkan.")
	}
	return nil
}

func validateTeachingJournalTransition(isNew bool, currentState string, in TeachingJournalInput) error {
	if isNew || currentState != "final" {
		return nil
	}
	if len([]rune(in.CorrectionReason)) == 0 {
		return ValidationError("Alasan koreksi jurnal final wajib diisi.")
	}
	if in.State != "final" {
		return ValidationError("Jurnal Final hanya dapat dikoreksi sebagai versi Final baru.")
	}
	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var n byte
	for i := range a {
		n |= a[i] ^ b[i]
	}
	return n == 0
}
