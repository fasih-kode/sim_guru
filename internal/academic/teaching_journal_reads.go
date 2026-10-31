package academic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const teachingJournalPageSize = 10

type TeachingJournalReadFilter struct {
	SemesterID int64
	ClassID    int64
	SubjectID  int64
	Search     string
	FromDate   string
	ToDate     string
	State      string
	Outcome    string
	Page       int
}

type TeachingJournalListItem struct {
	ID, LogicalLessonID, SemesterID, RevisionID, EntryID int64
	ClassID, SubjectID, TeacherID                        int64
	MeetingDate, ClassSnapshot, SubjectSnapshot          string
	TeacherSnapshot, State, Outcome                      string
	Material, ActivityNotes                              string
	ReviewReason                                         string
	Version                                              int
	RecordedAt                                           time.Time
}

type TeachingJournalPage struct {
	Items   []TeachingJournalListItem
	Page    int
	HasNext bool
}

type TeachingJournalFilterOption struct {
	ID   int64
	Name string
}

type TeachingJournalFilterOptions struct {
	Semesters []TeachingJournalFilterOption
	Classes   []TeachingJournalFilterOption
	Subjects  []TeachingJournalFilterOption
}

type TeachingJournalPeriodSnapshot struct {
	Ordinal  int    `json:"ordinal"`
	PeriodNo int    `json:"period_no"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
}

type TeachingJournalVersionRecord struct {
	ID, ActorID                     int64
	Number                          int
	State, Outcome                  string
	Material, ActivityNotes         string
	NotHeldReason, CorrectionReason string
	ActualStart, ActualEnd          string
	ActorName, Event, EventReason   string
	RecordedAt                      time.Time
	Periods                         []TeachingJournalPeriodSnapshot
}

type TeachingJournalLineageSnapshot struct {
	TransitionID                  int64
	FromRevisionID, SourceEntryID int64
	Kind, Decision, Reason        string
	ActorID                       int64
	RecordedAt, EffectiveFrom     time.Time
}

type TeachingJournalDetail struct {
	ID, LogicalLessonID, SemesterID, RevisionID, EntryID int64
	AssignmentID, ClassID, SubjectID, TeacherID          int64
	OwnerUserID                                          int64
	MeetingDate                                          string
	SemesterSnapshot                                     string
	RevisionNumber                                       int
	ClassSnapshot, SubjectSnapshot, TeacherSnapshot      string
	CreatedAt                                            time.Time
	CurrentVersion                                       TeachingJournalVersionRecord
	Lineage                                              TeachingJournalLineageSnapshot
	ReviewReason                                         string
}

type TeachingJournalHistory struct {
	JournalID int64
	Versions  []TeachingJournalVersionRecord
}

type TeachingJournalOccurrenceOption struct {
	LogicalLessonID, SemesterID, RevisionID, EntryID                        int64
	Date, SemesterSnapshot, ClassSnapshot, SubjectSnapshot, TeacherSnapshot string
	RevisionNumber                                                          int
	Periods                                                                 []TeachingJournalPeriodSnapshot
}

// Indikator hanya membandingkan snapshot draf milik pengguna dengan slot pada
// revision published yang efektif untuk tanggal occurrence menurut WIB.
// Keputusan lineage tetap menjadi sumber pemetaan; kemiripan slot tidak ditebak.
const teachingJournalReviewJoins = `
	LEFT JOIN LATERAL (
		SELECT r.id
		FROM app.weekly_timetable_revisions r
		WHERE r.semester_id=j.semester_id AND r.status='published' AND r.effective_from<=j.meeting_date
		ORDER BY r.effective_from DESC,r.revision_no DESC LIMIT 1
	) journal_effective ON true
	LEFT JOIN app.teaching_journal_lineage_status journal_lineage ON journal_lineage.revision_id=journal_effective.id
	LEFT JOIN LATERAL (
		SELECT e.id,e.weekday,e.class_id,e.subject_id,e.teacher_id,e.assignment_id,e.period_start,e.period_end
		FROM app.teaching_journal_lineage_decisions d
		JOIN app.weekly_timetable_entries e ON e.id=d.target_entry_id AND e.revision_id=d.to_revision_id
		WHERE d.to_revision_id=journal_effective.id AND d.logical_lesson_id=j.logical_lesson_id AND d.target_entry_id IS NOT NULL
	) journal_slot ON true
`

const teachingJournalReviewReasonSQL = `CASE
	WHEN v.state<>'draft' THEN ''
	WHEN journal_effective.id IS NULL THEN ''
	WHEN journal_effective.id=j.revision_id THEN ''
	WHEN journal_lineage.lineage_status='pending' THEN 'Revisi jadwal efektif belum memiliki keputusan kesinambungan; kecocokan draf belum dapat dipastikan.'
	WHEN journal_slot.id IS NULL THEN 'Slot pada draf tidak ditemukan pada revisi jadwal efektif untuk tanggal ini.'
	WHEN journal_slot.weekday<>EXTRACT(ISODOW FROM j.meeting_date)::integer
		OR journal_slot.class_id<>j.class_id OR journal_slot.subject_id<>j.subject_id
		OR journal_slot.teacher_id<>j.teacher_id OR journal_slot.assignment_id<>j.assignment_id
		OR COALESCE((SELECT jsonb_agg(jsonb_build_array(p.period_no,p.starts_at::text,p.ends_at::text) ORDER BY p.period_no)
			FROM app.weekly_timetable_periods p
			WHERE p.revision_id=journal_effective.id AND p.weekday=journal_slot.weekday
				AND p.period_no BETWEEN journal_slot.period_start AND journal_slot.period_end),'[]'::jsonb)
			IS DISTINCT FROM COALESCE((SELECT jsonb_agg(jsonb_build_array(p.period_no,p.starts_at::text,p.ends_at::text) ORDER BY p.period_no)
				FROM app.teaching_journal_version_periods p WHERE p.version_id=v.id),'[]'::jsonb)
		THEN 'Rincian jadwal efektif untuk tanggal ini berbeda dari snapshot draf.'
	ELSE '' END`

func (s *Store) teachingJournalReader(ctx context.Context, actor int64) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func validateTeachingJournalReadFilter(f TeachingJournalReadFilter) error {
	if f.Page < 1 || f.Page > 100000 || f.SemesterID < 0 || f.ClassID < 0 || f.SubjectID < 0 ||
		(f.State != "" && f.State != "draft" && f.State != "final") ||
		(f.Outcome != "" && f.Outcome != "held" && f.Outcome != "not_held") {
		return ValidationError("Filter jurnal atau nomor halaman tidak valid.")
	}
	if len([]rune(f.Search)) > 120 || strings.ContainsRune(f.Search, '\x00') {
		return ValidationError("Pencarian jurnal terlalu panjang atau tidak valid.")
	}
	for _, value := range []string{f.FromDate, f.ToDate} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil || parsed.Format("2006-01-02") != value {
			return ValidationError("Tanggal filter jurnal tidak valid.")
		}
	}
	if f.FromDate != "" && f.ToDate != "" && f.ToDate < f.FromDate {
		return ValidationError("Tanggal akhir filter harus sama atau setelah tanggal awal.")
	}
	return nil
}

// TeachingJournals membaca halaman jurnal yang dimiliki actor. Cakupan guru
// selalu diambil dari sesi; ID guru tidak pernah diterima sebagai filter.
func (s *Store) TeachingJournals(ctx context.Context, actor int64, f TeachingJournalReadFilter) (TeachingJournalPage, error) {
	if err := validateTeachingJournalReadFilter(f); err != nil {
		return TeachingJournalPage{}, err
	}
	tx, err := s.teachingJournalReader(ctx, actor)
	if err != nil {
		return TeachingJournalPage{}, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT j.id,j.logical_lesson_id,j.semester_id,j.revision_id,j.timetable_entry_id,
		j.class_id,j.subject_id,j.teacher_id,j.meeting_date::text,j.class_snapshot,j.subject_snapshot,j.teacher_snapshot,
		v.state,v.outcome,v.material,v.activity_notes,j.current_version_no,v.recorded_at,`+teachingJournalReviewReasonSQL+`
		FROM app.teaching_journals j
		JOIN app.teaching_journal_versions v ON v.journal_id=j.id AND v.version_no=j.current_version_no
		`+teachingJournalReviewJoins+`
		WHERE j.owner_user_id=$1
		AND ($2=0 OR j.semester_id=$2) AND ($3=0 OR j.class_id=$3) AND ($4=0 OR j.subject_id=$4)
		AND ($5='' OR j.meeting_date >= $5::date) AND ($6='' OR j.meeting_date <= $6::date)
		AND ($7='' OR v.state=$7) AND ($8='' OR v.outcome=$8)
		AND ($9='' OR j.class_snapshot ILIKE '%'||$9||'%' OR j.subject_snapshot ILIKE '%'||$9||'%' OR j.teacher_snapshot ILIKE '%'||$9||'%')
		ORDER BY j.meeting_date DESC,j.id DESC LIMIT $10 OFFSET $11`, actor, f.SemesterID, f.ClassID,
		f.SubjectID, f.FromDate, f.ToDate, f.State, f.Outcome, f.Search, teachingJournalPageSize+1,
		(f.Page-1)*teachingJournalPageSize)
	if err != nil {
		return TeachingJournalPage{}, err
	}
	defer rows.Close()
	page := TeachingJournalPage{Page: f.Page, Items: make([]TeachingJournalListItem, 0, teachingJournalPageSize)}
	for rows.Next() {
		var item TeachingJournalListItem
		if err = rows.Scan(&item.ID, &item.LogicalLessonID, &item.SemesterID, &item.RevisionID, &item.EntryID,
			&item.ClassID, &item.SubjectID, &item.TeacherID, &item.MeetingDate, &item.ClassSnapshot,
			&item.SubjectSnapshot, &item.TeacherSnapshot, &item.State, &item.Outcome, &item.Material,
			&item.ActivityNotes, &item.Version, &item.RecordedAt, &item.ReviewReason); err != nil {
			return TeachingJournalPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err = rows.Err(); err != nil {
		return TeachingJournalPage{}, err
	}
	if len(page.Items) > teachingJournalPageSize {
		page.HasNext = true
		page.Items = page.Items[:teachingJournalPageSize]
	}
	return page, nil
}

// TeachingJournalFilterOptions hanya menampilkan label yang muncul pada jurnal
// milik actor; opsi tidak membuka kelas atau mapel milik guru lain.
func (s *Store) TeachingJournalFilterOptions(ctx context.Context, actor int64) (TeachingJournalFilterOptions, error) {
	tx, err := s.teachingJournalReader(ctx, actor)
	if err != nil {
		return TeachingJournalFilterOptions{}, err
	}
	defer tx.Rollback()
	options := TeachingJournalFilterOptions{}
	queries := []struct {
		target *[]TeachingJournalFilterOption
		query  string
	}{
		{&options.Semesters, `SELECT DISTINCT j.semester_id,y.name||' · Semester '||sm.number::text FROM app.teaching_journals j JOIN app.semesters sm ON sm.id=j.semester_id JOIN app.school_years y ON y.id=sm.school_year_id WHERE j.owner_user_id=$1 ORDER BY 2`},
		{&options.Classes, `SELECT DISTINCT j.class_id,j.class_snapshot FROM app.teaching_journals j WHERE j.owner_user_id=$1 ORDER BY 2`},
		{&options.Subjects, `SELECT DISTINCT j.subject_id,j.subject_snapshot FROM app.teaching_journals j WHERE j.owner_user_id=$1 ORDER BY 2`},
	}
	for _, item := range queries {
		rows, queryErr := tx.QueryContext(ctx, item.query, actor)
		if queryErr != nil {
			return TeachingJournalFilterOptions{}, queryErr
		}
		for rows.Next() {
			var option TeachingJournalFilterOption
			if queryErr = rows.Scan(&option.ID, &option.Name); queryErr != nil {
				rows.Close()
				return TeachingJournalFilterOptions{}, queryErr
			}
			*item.target = append(*item.target, option)
		}
		queryErr = rows.Err()
		rows.Close()
		if queryErr != nil {
			return TeachingJournalFilterOptions{}, queryErr
		}
	}
	return options, nil
}

// TeachingJournalOccurrences hanya menampilkan pertemuan terbit yang efektif
// pada tanggal pilihan, memiliki lineage lengkap, dan ditugaskan kepada actor.
func (s *Store) TeachingJournalOccurrences(ctx context.Context, actor int64, date string) ([]TeachingJournalOccurrenceOption, bool, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		return nil, false, ValidationError("Tanggal pertemuan tidak valid.")
	}
	tx, err := s.teachingJournalReader(ctx, actor)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var beforeCutover bool
	err = tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM app.teaching_journal_cutover_batches WHERE cutover_date<=$1::date)`, date).Scan(&beforeCutover)
	if err != nil {
		return nil, false, err
	}
	if beforeCutover {
		return nil, false, ValidationError("Jurnal baru hanya dapat dibuat untuk tanggal sejak cutover.")
	}
	var teacherID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teachers WHERE user_id=$1 AND active`, actor).Scan(&teacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrForbidden
	}
	if err != nil {
		return nil, false, err
	}
	var pending bool
	err = tx.QueryRowContext(ctx, `WITH effective AS (
		SELECT DISTINCT ON (r.semester_id) r.id,r.semester_id,r.revision_no
		FROM app.weekly_timetable_revisions r
		JOIN app.semester_calendars sc ON sc.semester_id=r.semester_id AND $1::date BETWEEN sc.starts_on AND sc.ends_on
		WHERE r.status='published' AND r.effective_from<=$1::date
		ORDER BY r.semester_id,r.effective_from DESC,r.revision_no DESC
	) SELECT EXISTS(SELECT 1 FROM effective e
		JOIN app.teaching_journal_lineage_status ls ON ls.revision_id=e.id AND ls.lineage_status='pending'
		JOIN app.weekly_timetable_entries te ON te.revision_id=e.id AND te.teacher_id=$2
		JOIN app.teaching_assignments ta ON ta.id=te.assignment_id AND ta.semester_id=e.semester_id AND ta.class_id=te.class_id AND ta.subject_id=te.subject_id AND ta.teacher_id=te.teacher_id AND ta.effective_from<=$1::date AND (ta.effective_until IS NULL OR ta.effective_until>$1::date))`, date, teacherID).Scan(&pending)
	if err != nil {
		return nil, false, err
	}
	rows, err := tx.QueryContext(ctx, `WITH effective AS (
		SELECT DISTINCT ON (r.semester_id) r.id,r.semester_id,r.revision_no
		FROM app.weekly_timetable_revisions r
		JOIN app.semester_calendars sc ON sc.semester_id=r.semester_id AND $1::date BETWEEN sc.starts_on AND sc.ends_on
		WHERE r.status='published' AND r.effective_from<=$1::date
		ORDER BY r.semester_id,r.effective_from DESC,r.revision_no DESC
	) SELECT l.id,l.semester_id,r.id,e.id,y.name||' · Semester '||sm.number::text,r.revision_no,c.grade::text||c.name,s.name,t.name,
		coalesce((SELECT jsonb_agg(jsonb_build_object('ordinal',p.period_no-e.period_start+1,'period_no',p.period_no,'starts_at',p.starts_at::text,'ends_at',p.ends_at::text) ORDER BY p.period_no)
			FROM app.weekly_timetable_periods p WHERE p.revision_id=r.id AND p.period_no BETWEEN e.period_start AND e.period_end),'[]'::jsonb)
		FROM effective r
		JOIN app.weekly_timetable_entries e ON e.revision_id=r.id AND e.teacher_id=$2
		JOIN app.semesters sm ON sm.id=r.semester_id JOIN app.school_years y ON y.id=sm.school_year_id
		JOIN app.teaching_journal_lineage_decisions d ON d.to_revision_id=r.id AND d.target_entry_id=e.id AND d.logical_lesson_id IS NOT NULL
		JOIN app.logical_lessons l ON l.id=d.logical_lesson_id AND l.semester_id=r.semester_id
		JOIN app.classes c ON c.id=e.class_id JOIN app.subjects s ON s.id=e.subject_id JOIN app.teachers t ON t.id=e.teacher_id
		JOIN app.teaching_assignments a ON a.id=e.assignment_id AND a.semester_id=r.semester_id AND a.class_id=e.class_id AND a.subject_id=e.subject_id AND a.teacher_id=e.teacher_id AND a.effective_from<=$1::date AND (a.effective_until IS NULL OR a.effective_until>$1::date)
		WHERE NOT EXISTS(SELECT 1 FROM app.teaching_journals j WHERE j.logical_lesson_id=l.id AND j.meeting_date=$1::date)
		ORDER BY c.grade,c.name,s.name,e.period_start`, date, teacherID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	options := make([]TeachingJournalOccurrenceOption, 0)
	for rows.Next() {
		var option TeachingJournalOccurrenceOption
		var periods []byte
		option.Date = date
		if err = rows.Scan(&option.LogicalLessonID, &option.SemesterID, &option.RevisionID, &option.EntryID, &option.SemesterSnapshot, &option.RevisionNumber, &option.ClassSnapshot, &option.SubjectSnapshot, &option.TeacherSnapshot, &periods); err != nil {
			return nil, false, err
		}
		if err = json.Unmarshal(periods, &option.Periods); err != nil {
			return nil, false, err
		}
		options = append(options, option)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	return options, pending, nil
}

func loadTeachingJournalPeriods(ctx context.Context, tx *sql.Tx, actor, journalID int64, versionNo int) ([]TeachingJournalPeriodSnapshot, error) {
	rows, err := tx.QueryContext(ctx, `SELECT p.ordinal,p.period_no,p.starts_at::text,p.ends_at::text
		FROM app.teaching_journal_version_periods p
		JOIN app.teaching_journal_versions v ON v.id=p.version_id
		JOIN app.teaching_journals j ON j.id=v.journal_id
		WHERE j.id=$1 AND j.owner_user_id=$2 AND v.version_no=$3 ORDER BY p.ordinal`, journalID, actor, versionNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	periods := make([]TeachingJournalPeriodSnapshot, 0, 4)
	for rows.Next() {
		var p TeachingJournalPeriodSnapshot
		if err = rows.Scan(&p.Ordinal, &p.PeriodNo, &p.StartsAt, &p.EndsAt); err != nil {
			return nil, err
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

func scanTeachingJournalVersion(ctx context.Context, tx *sql.Tx, actor, journalID int64, versionNo int) (TeachingJournalVersionRecord, error) {
	var v TeachingJournalVersionRecord
	err := tx.QueryRowContext(ctx, `SELECT v.id,v.version_no,v.state,v.outcome,v.material,v.activity_notes,
		v.not_held_reason,coalesce(v.actual_starts_at::text,''),coalesce(v.actual_ends_at::text,''),
		v.actor_id,u.display_name,coalesce(v.correction_reason,''),coalesce(h.event,''),coalesce(h.reason,''),v.recorded_at
		FROM app.teaching_journal_versions v
		JOIN app.teaching_journals j ON j.id=v.journal_id
		JOIN app.users u ON u.id=v.actor_id
		LEFT JOIN LATERAL (SELECT event,reason FROM app.teaching_journal_history WHERE version_id=v.id ORDER BY id LIMIT 1) h ON true
		WHERE j.id=$1 AND j.owner_user_id=$2 AND v.version_no=$3`, journalID, actor, versionNo).Scan(
		&v.ID, &v.Number, &v.State, &v.Outcome, &v.Material, &v.ActivityNotes, &v.NotHeldReason,
		&v.ActualStart, &v.ActualEnd, &v.ActorID, &v.ActorName, &v.CorrectionReason, &v.Event, &v.EventReason, &v.RecordedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TeachingJournalVersionRecord{}, ErrNotFound
	}
	if err != nil {
		return TeachingJournalVersionRecord{}, err
	}
	v.Periods, err = loadTeachingJournalPeriods(ctx, tx, actor, journalID, versionNo)
	if err != nil {
		return TeachingJournalVersionRecord{}, err
	}
	return v, nil
}

// TeachingJournalDetail hanya mengembalikan occurrence milik actor dan versi
// saat ini bersama snapshot lineage dan JP yang disimpan saat versi dibuat.
func (s *Store) TeachingJournalDetail(ctx context.Context, actor, journalID int64) (TeachingJournalDetail, error) {
	if journalID < 1 {
		return TeachingJournalDetail{}, ValidationError("ID jurnal tidak valid.")
	}
	tx, err := s.teachingJournalReader(ctx, actor)
	if err != nil {
		return TeachingJournalDetail{}, err
	}
	defer tx.Rollback()
	var d TeachingJournalDetail
	var currentVersion int
	err = tx.QueryRowContext(ctx, `SELECT j.id,j.logical_lesson_id,j.semester_id,j.revision_id,j.timetable_entry_id,
		j.assignment_id,j.class_id,j.subject_id,j.teacher_id,j.owner_user_id,j.meeting_date::text,
		y.name||' · Semester '||sm.number::text,r.revision_no,j.class_snapshot,j.subject_snapshot,j.teacher_snapshot,j.created_at,j.current_version_no,
		coalesce(tr.id,0),coalesce(tr.from_revision_id,0),coalesce(ld.source_entry_id,0),
		coalesce(tr.transition_kind,''),coalesce(ld.decision,''),coalesce(ld.reason,''),
		coalesce(ld.actor_id,0),coalesce(ld.recorded_at,tr.recorded_at,j.created_at),coalesce(tr.effective_from,j.meeting_date),`+teachingJournalReviewReasonSQL+`
		FROM app.teaching_journals j
		JOIN app.semesters sm ON sm.id=j.semester_id JOIN app.school_years y ON y.id=sm.school_year_id
		JOIN app.weekly_timetable_revisions r ON r.id=j.revision_id
		JOIN app.teaching_journal_versions v ON v.journal_id=j.id AND v.version_no=j.current_version_no
		LEFT JOIN app.teaching_journal_lineage_decisions ld ON ld.to_revision_id=j.revision_id AND ld.target_entry_id=j.timetable_entry_id AND ld.logical_lesson_id=j.logical_lesson_id
		LEFT JOIN app.teaching_journal_lineage_transitions tr ON tr.id=ld.transition_id
		`+teachingJournalReviewJoins+`
		WHERE j.id=$1 AND j.owner_user_id=$2`, journalID, actor).Scan(
		&d.ID, &d.LogicalLessonID, &d.SemesterID, &d.RevisionID, &d.EntryID, &d.AssignmentID,
		&d.ClassID, &d.SubjectID, &d.TeacherID, &d.OwnerUserID, &d.MeetingDate, &d.SemesterSnapshot, &d.RevisionNumber, &d.ClassSnapshot,
		&d.SubjectSnapshot, &d.TeacherSnapshot, &d.CreatedAt, &currentVersion,
		&d.Lineage.TransitionID, &d.Lineage.FromRevisionID, &d.Lineage.SourceEntryID,
		&d.Lineage.Kind, &d.Lineage.Decision, &d.Lineage.Reason, &d.Lineage.ActorID,
		&d.Lineage.RecordedAt, &d.Lineage.EffectiveFrom, &d.ReviewReason)
	if errors.Is(err, sql.ErrNoRows) {
		return TeachingJournalDetail{}, ErrNotFound
	}
	if err != nil {
		return TeachingJournalDetail{}, err
	}
	d.CurrentVersion, err = scanTeachingJournalVersion(ctx, tx, actor, journalID, currentVersion)
	if err != nil {
		return TeachingJournalDetail{}, err
	}
	return d, nil
}

// TeachingJournalVersionDetail membaca versi historis tertentu tanpa memakai
// jadwal aktif atau assignment terkini.
func (s *Store) TeachingJournalVersionDetail(ctx context.Context, actor, journalID int64, versionNo int) (TeachingJournalVersionRecord, error) {
	if journalID < 1 || versionNo < 1 {
		return TeachingJournalVersionRecord{}, ValidationError("ID jurnal atau nomor versi tidak valid.")
	}
	tx, err := s.teachingJournalReader(ctx, actor)
	if err != nil {
		return TeachingJournalVersionRecord{}, err
	}
	defer tx.Rollback()
	return scanTeachingJournalVersion(ctx, tx, actor, journalID, versionNo)
}

// TeachingJournalVersions mengembalikan seluruh event versi secara kronologis
// dan mengambil seluruh snapshot JP dalam satu query tambahan, tanpa N+1.
func (s *Store) TeachingJournalVersions(ctx context.Context, actor, journalID int64) (TeachingJournalHistory, error) {
	if journalID < 1 {
		return TeachingJournalHistory{}, ValidationError("ID jurnal tidak valid.")
	}
	tx, err := s.teachingJournalReader(ctx, actor)
	if err != nil {
		return TeachingJournalHistory{}, err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teaching_journals WHERE id=$1 AND owner_user_id=$2)`, journalID, actor).Scan(&exists); err != nil {
		return TeachingJournalHistory{}, err
	}
	if !exists {
		return TeachingJournalHistory{}, ErrNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT v.id,v.version_no,v.state,v.outcome,v.material,v.activity_notes,
		v.not_held_reason,coalesce(v.actual_starts_at::text,''),coalesce(v.actual_ends_at::text,''),
		v.actor_id,u.display_name,coalesce(v.correction_reason,''),h.event,coalesce(h.reason,''),h.recorded_at
		FROM app.teaching_journal_versions v
	JOIN app.users u ON u.id=v.actor_id
	JOIN app.teaching_journal_history h ON h.version_id=v.id
	WHERE v.journal_id=$1 ORDER BY v.version_no,h.id`, journalID)
	if err != nil {
		return TeachingJournalHistory{}, err
	}
	history := TeachingJournalHistory{JournalID: journalID, Versions: make([]TeachingJournalVersionRecord, 0)}
	versionIDs := make(map[int64]int, 8)
	for rows.Next() {
		var v TeachingJournalVersionRecord
		if err = rows.Scan(&v.ID, &v.Number, &v.State, &v.Outcome, &v.Material, &v.ActivityNotes,
			&v.NotHeldReason, &v.ActualStart, &v.ActualEnd, &v.ActorID, &v.ActorName,
			&v.CorrectionReason, &v.Event, &v.EventReason, &v.RecordedAt); err != nil {
			rows.Close()
			return TeachingJournalHistory{}, err
		}
		versionIDs[v.ID] = v.Number
		history.Versions = append(history.Versions, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return TeachingJournalHistory{}, err
	}
	rows.Close()
	if len(history.Versions) == 0 {
		return TeachingJournalHistory{}, ErrNotFound
	}
	periodRows, err := tx.QueryContext(ctx, `SELECT p.version_id,p.ordinal,p.period_no,p.starts_at::text,p.ends_at::text
		FROM app.teaching_journal_version_periods p JOIN app.teaching_journal_versions v ON v.id=p.version_id
		WHERE v.journal_id=$1 ORDER BY v.version_no,p.ordinal`, journalID)
	if err != nil {
		return TeachingJournalHistory{}, err
	}
	defer periodRows.Close()
	byVersion := make(map[int64][]TeachingJournalPeriodSnapshot, len(versionIDs))
	for periodRows.Next() {
		var versionID int64
		var p TeachingJournalPeriodSnapshot
		if err = periodRows.Scan(&versionID, &p.Ordinal, &p.PeriodNo, &p.StartsAt, &p.EndsAt); err != nil {
			return TeachingJournalHistory{}, err
		}
		byVersion[versionID] = append(byVersion[versionID], p)
	}
	if err = periodRows.Err(); err != nil {
		return TeachingJournalHistory{}, err
	}
	for i := range history.Versions {
		history.Versions[i].Periods = byVersion[history.Versions[i].ID]
	}
	return history, nil
}

func BuildTeachingJournalReadFilter(page int, semesterID, classID, subjectID int64, fromDate, toDate, state, outcome string) TeachingJournalReadFilter {
	return TeachingJournalReadFilter{Page: page, SemesterID: semesterID, ClassID: classID, SubjectID: subjectID,
		FromDate: strings.TrimSpace(fromDate), ToDate: strings.TrimSpace(toDate), State: strings.TrimSpace(state), Outcome: strings.TrimSpace(outcome)}
}
