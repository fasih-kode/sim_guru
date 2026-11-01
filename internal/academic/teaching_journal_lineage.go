package academic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

// TeachingJournalLineageDecision adalah satu keputusan eksplisit Kurikulum
// untuk slot sumber dan tujuan pada dua revision terbit.
type TeachingJournalLineageDecision struct {
	Decision      string
	SourceEntryID int64
	TargetEntryID int64
	Reason        string
}

// CutoverTeachingJournalLineage memberi ID baru untuk semua slot pada revision
// terbit yang efektif hari ini (tanggal dihitung PostgreSQL dalam WIB).
func (s *Store) CutoverTeachingJournalLineage(ctx context.Context, actor int64, reason string) (int64, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) == 0 || len([]rune(reason)) > 500 {
		return 0, ValidationError("Alasan cutover wajib diisi, maksimal 500 karakter.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return 0, err
	}
	if err = s.authorize(ctx, tx, actor, "kurikulum"); err != nil {
		return 0, err
	}
	var existing int64
	err = tx.QueryRowContext(ctx, `SELECT b.id FROM app.teaching_journal_cutover_batches b WHERE b.cutover_date=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date AND EXISTS(SELECT 1 FROM app.teaching_journal_lineage_transitions tr JOIN app.semesters sm ON sm.id=tr.semester_id AND sm.active WHERE tr.cutover_batch_id=b.id AND tr.transition_kind='cutover') ORDER BY b.id LIMIT 1`).Scan(&existing)
	if err == nil {
		if err = tx.Commit(); err != nil {
			return 0, err
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var earlierID int64
	err = tx.QueryRowContext(ctx, `SELECT b.id FROM app.teaching_journal_cutover_batches b JOIN app.teaching_journal_lineage_transitions tr ON tr.cutover_batch_id=b.id AND tr.transition_kind='cutover' JOIN app.semesters sm ON sm.id=tr.semester_id AND sm.active ORDER BY b.cutover_date,b.id LIMIT 1`).Scan(&earlierID)
	if err == nil {
		return 0, ValidationError("Cutover Jurnal Mengajar sudah pernah dilakukan untuk semester aktif.")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT sm.id,r.id,r.effective_from FROM app.semesters sm JOIN app.semester_calendars sc ON sc.semester_id=sm.id AND (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date BETWEEN sc.starts_on AND sc.ends_on JOIN LATERAL(SELECT id,effective_from FROM app.weekly_timetable_revisions WHERE semester_id=sm.id AND status='published' AND effective_from<=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date ORDER BY effective_from DESC,revision_no DESC LIMIT 1) r ON true WHERE sm.active ORDER BY sm.id FOR UPDATE OF sm`)
	if err != nil {
		return 0, err
	}
	type revision struct {
		semesterID, revisionID int64
		effective              time.Time
	}
	var revisions []revision
	for rows.Next() {
		var v revision
		if err = rows.Scan(&v.semesterID, &v.revisionID, &v.effective); err != nil {
			rows.Close()
			return 0, err
		}
		revisions = append(revisions, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if len(revisions) == 0 {
		return 0, ValidationError("Belum ada semester aktif dengan jadwal terbit yang efektif untuk cutover.")
	}
	// Validasi semua semester dahulu. Batch dan logical lesson belum ditulis
	// sampai seluruh slot revision efektif lolos pemeriksaan yang sama dengan
	// kontrak publikasi, dievaluasi kembali pada tanggal cutover WIB.
	var slotCount int
	for _, rev := range revisions {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.weekly_timetable_entries WHERE revision_id=$1`, rev.revisionID).Scan(&count); err != nil {
			return 0, err
		}
		if count == 0 {
			return 0, ValidationError("Jadwal terbit yang dipilih tidak memiliki slot untuk cutover.")
		}
		var invalid bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM app.weekly_timetable_entries e
			LEFT JOIN app.teaching_assignments a ON a.id=e.assignment_id AND a.semester_id=$2
				AND a.class_id=e.class_id AND a.subject_id=e.subject_id AND a.teacher_id=e.teacher_id
				AND a.effective_from<=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date
				AND (a.effective_until IS NULL OR a.effective_until>(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date)
			JOIN app.teachers t ON t.id=e.teacher_id
			JOIN app.users u ON u.id=t.user_id
			JOIN app.subjects s ON s.id=e.subject_id
			JOIN app.classes c ON c.id=e.class_id
			WHERE e.revision_id=$1 AND (a.id IS NULL OR NOT t.active OR NOT u.active OR NOT s.active
				OR NOT EXISTS(SELECT 1 FROM app.user_roles ur WHERE ur.user_id=u.id AND ur.role_code='guru_mapel')
				OR NOT EXISTS(SELECT 1 FROM app.semesters sm WHERE sm.id=$2 AND c.school_year_id=sm.school_year_id)
				OR EXISTS(SELECT 1 FROM generate_series(e.period_start::int,e.period_end::int) n(period_no)
					WHERE NOT EXISTS(SELECT 1 FROM app.weekly_timetable_periods p WHERE p.revision_id=e.revision_id AND p.weekday=e.weekday AND p.period_no=n.period_no)))
		)`, rev.revisionID, rev.semesterID).Scan(&invalid)
		if err != nil {
			return 0, err
		}
		if invalid {
			return 0, ValidationError("Cutover ditolak: ada slot tanpa penugasan dan identitas guru aktif yang cocok atau snapshot JP lengkap.")
		}
		slotCount += count
	}
	if slotCount == 0 {
		return 0, ValidationError("Jadwal aktif tidak memiliki slot untuk cutover.")
	}
	var batchID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.teaching_journal_cutover_batches(cutover_date,actor_id) VALUES((clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date,$1) RETURNING id`, actor).Scan(&batchID); err != nil {
		return 0, dbError(err)
	}
	for _, rev := range revisions {
		var transitionID int64
		if err = tx.QueryRowContext(ctx, `INSERT INTO app.teaching_journal_lineage_transitions(cutover_batch_id,semester_id,to_revision_id,effective_from,transition_kind,actor_id,reason) VALUES($1,$2,$3,$4,'cutover',$5,$6) RETURNING id`, batchID, rev.semesterID, rev.revisionID, rev.effective, actor, reason).Scan(&transitionID); err != nil {
			return 0, dbError(err)
		}
		entries, e := tx.QueryContext(ctx, `SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 ORDER BY id`, rev.revisionID)
		if e != nil {
			return 0, e
		}
		var entryIDs []int64
		for entries.Next() {
			var id int64
			if e = entries.Scan(&id); e != nil {
				entries.Close()
				return 0, e
			}
			entryIDs = append(entryIDs, id)
		}
		if e = entries.Err(); e != nil {
			entries.Close()
			return 0, e
		}
		entries.Close()
		if len(entryIDs) == 0 {
			return 0, ValidationError("Jadwal terbit yang dipilih tidak memiliki slot untuk cutover.")
		}
		for _, entryID := range entryIDs {
			var logicalID int64
			if err = tx.QueryRowContext(ctx, `INSERT INTO app.logical_lessons(semester_id,created_by) VALUES($1,$2) RETURNING id`, rev.semesterID, actor).Scan(&logicalID); err != nil {
				return 0, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO app.teaching_journal_lineage_decisions(transition_id,semester_id,to_revision_id,target_entry_id,logical_lesson_id,decision,actor_id,reason) VALUES($1,$2,$3,$4,$5,'cutover',$6,$7)`, transitionID, rev.semesterID, rev.revisionID, entryID, logicalID, actor, reason); err != nil {
				return 0, dbError(err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, dbError(err)
	}
	return batchID, nil
}

// ResolveTeachingJournalLineage menyimpan satu pemetaan lengkap revision-ke-
// revision. Draft timetable dan absensi tidak dikunci oleh status lineage.
func (s *Store) ResolveTeachingJournalLineage(ctx context.Context, actor, fromRevisionID, toRevisionID int64, reason string, decisions []TeachingJournalLineageDecision) error {
	reason = strings.TrimSpace(reason)
	if fromRevisionID < 1 || toRevisionID < 1 || fromRevisionID == toRevisionID || len([]rune(reason)) == 0 || len([]rune(reason)) > 500 || len(decisions) == 0 || len(decisions) > 2400 {
		return ValidationError("Revision, alasan, dan keputusan lineage wajib valid.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return err
	}
	if err = s.authorize(ctx, tx, actor, "kurikulum"); err != nil {
		return err
	}
	var semesterID int64
	var effectiveFrom time.Time
	var fromEffective time.Time
	err = tx.QueryRowContext(ctx, `SELECT r.semester_id,r.effective_from,p.effective_from FROM app.weekly_timetable_revisions r JOIN app.weekly_timetable_revisions p ON p.id=$1 AND p.semester_id=r.semester_id WHERE r.id=$2 AND r.status='published' AND p.status='published' FOR UPDATE OF r,p`, fromRevisionID, toRevisionID).Scan(&semesterID, &effectiveFrom, &fromEffective)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if !effectiveFrom.After(fromEffective) {
		return ValidationError("Tanggal efektif revision tujuan harus setelah revision sumber.")
	}
	var previousID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.weekly_timetable_revisions WHERE semester_id=$1 AND status='published' AND effective_from<$2 ORDER BY effective_from DESC,revision_no DESC LIMIT 1`, semesterID, effectiveFrom).Scan(&previousID)
	if err != nil {
		return err
	}
	if previousID != fromRevisionID {
		return ValidationError("Pilih revision lineage sebelumnya yang efektif tepat sebelum revision tujuan.")
	}
	var already bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_journal_lineage_transitions WHERE to_revision_id=$1)`, toRevisionID).Scan(&already); err != nil {
		return err
	}
	if already {
		return ErrConflict
	}
	var transitionID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.teaching_journal_lineage_transitions(semester_id,from_revision_id,to_revision_id,effective_from,transition_kind,actor_id,reason) VALUES($1,$2,$3,$4,'revision',$5,$6) RETURNING id`, semesterID, fromRevisionID, toRevisionID, effectiveFrom, actor, reason).Scan(&transitionID); err != nil {
		return dbError(err)
	}
	seenSources, seenTargets := map[int64]bool{}, map[int64]bool{}
	for _, d := range decisions {
		d.Decision = strings.TrimSpace(d.Decision)
		d.Reason = strings.TrimSpace(d.Reason)
		if len([]rune(d.Reason)) == 0 || len([]rune(d.Reason)) > 500 {
			return ValidationError("Alasan setiap keputusan wajib diisi, maksimal 500 karakter.")
		}
		if (d.Decision == "continue" || d.Decision == "stop") != (d.SourceEntryID > 0) || (d.Decision == "continue" || d.Decision == "new") != (d.TargetEntryID > 0) || (d.Decision != "continue" && d.Decision != "new" && d.Decision != "stop") {
			return ValidationError("Kombinasi keputusan dan entry lineage tidak valid.")
		}
		if d.SourceEntryID > 0 {
			if seenSources[d.SourceEntryID] {
				return ValidationError("Satu slot sumber hanya boleh dipetakan sekali.")
			}
			seenSources[d.SourceEntryID] = true
		}
		if d.TargetEntryID > 0 {
			if seenTargets[d.TargetEntryID] {
				return ValidationError("Satu slot tujuan hanya boleh dipetakan sekali.")
			}
			seenTargets[d.TargetEntryID] = true
		}
		var logicalID int64
		if d.Decision == "continue" || d.Decision == "stop" {
			err = tx.QueryRowContext(ctx, `SELECT logical_lesson_id FROM app.teaching_journal_lineage_decisions WHERE to_revision_id=$1 AND target_entry_id=$2`, fromRevisionID, d.SourceEntryID).Scan(&logicalID)
			if errors.Is(err, sql.ErrNoRows) {
				return ValidationError("Slot sumber belum mempunyai lineage yang terselesaikan.")
			}
			if err != nil {
				return err
			}
		}
		if d.Decision == "new" {
			if err = tx.QueryRowContext(ctx, `INSERT INTO app.logical_lessons(semester_id,created_by) VALUES($1,$2) RETURNING id`, semesterID, actor).Scan(&logicalID); err != nil {
				return err
			}
		}
		var source any
		if d.SourceEntryID > 0 {
			source = d.SourceEntryID
		}
		var target any
		if d.TargetEntryID > 0 {
			target = d.TargetEntryID
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.teaching_journal_lineage_decisions(transition_id,semester_id,from_revision_id,to_revision_id,source_entry_id,target_entry_id,logical_lesson_id,decision,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, transitionID, semesterID, fromRevisionID, toRevisionID, source, target, logicalID, d.Decision, actor, d.Reason); err != nil {
			return dbError(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return dbError(err)
	}
	return nil
}

// TeachingJournalOccurrenceRevision mengembalikan revision terbit efektif pada tanggal WIB.
// Fungsi ini hanya baca; tanggal pra-cutover ditolak tanpa mapping legacy eksplisit.
func (s *Store) TeachingJournalOccurrenceRevision(ctx context.Context, actor int64, logicalLessonID int64, occurrence time.Time) (int64, int64, error) {
	if logicalLessonID < 1 || occurrence.IsZero() {
		return 0, 0, ValidationError("Pertemuan dan tanggal occurrence wajib valid.")
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	date := occurrence.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	var cutoverDate time.Time
	err = tx.QueryRowContext(ctx, `SELECT cutover_date FROM app.teaching_journal_cutover_batches ORDER BY id LIMIT 1`).Scan(&cutoverDate)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, err
	}
	if date < cutoverDate.Format("2006-01-02") {
		return 0, 0, ValidationError("Jurnal untuk tanggal sebelum cutover belum dapat dibuat tanpa pemetaan historis.")
	}
	var revisionID, entryID int64
	err = tx.QueryRowContext(ctx, `WITH effective AS (SELECT r.id,r.semester_id FROM app.logical_lessons l JOIN app.weekly_timetable_revisions r ON r.semester_id=l.semester_id AND r.status='published' AND r.effective_from<=$2::date WHERE l.id=$1 AND EXISTS(SELECT 1 FROM app.semester_calendars sc WHERE sc.semester_id=l.semester_id AND $2::date BETWEEN sc.starts_on AND sc.ends_on) ORDER BY r.effective_from DESC,r.revision_no DESC LIMIT 1) SELECT r.id,e.id FROM effective r JOIN app.teaching_journal_lineage_decisions d ON d.to_revision_id=r.id AND d.logical_lesson_id=$1 AND d.target_entry_id IS NOT NULL JOIN app.weekly_timetable_entries e ON e.id=d.target_entry_id AND e.revision_id=r.id`, logicalLessonID, date).Scan(&revisionID, &entryID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ValidationError("Jadwal efektif atau lineage untuk tanggal tersebut belum tersedia.")
	}
	if err != nil {
		return 0, 0, err
	}
	var userTeacher int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active`, actor).Scan(&userTeacher)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrForbidden
	}
	if err != nil {
		return 0, 0, err
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.weekly_timetable_entries e JOIN app.teaching_assignments a ON a.id=e.assignment_id AND a.semester_id=(SELECT semester_id FROM app.logical_lessons WHERE id=$1) AND a.class_id=e.class_id AND a.subject_id=e.subject_id AND a.teacher_id=e.teacher_id AND a.effective_from<=$3::date AND (a.effective_until IS NULL OR a.effective_until>$3::date) WHERE e.id=$2 AND e.revision_id=$4 AND e.teacher_id=$5)`, logicalLessonID, entryID, date, revisionID, userTeacher).Scan(&eligible)
	if err != nil {
		return 0, 0, err
	}
	if !eligible {
		return 0, 0, ErrForbidden
	}
	return revisionID, entryID, nil
}

func (d TeachingJournalLineageDecision) String() string {
	return fmt.Sprintf("%s:%d:%d", d.Decision, d.SourceEntryID, d.TargetEntryID)
}
