package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strings"
	"unicode/utf8"
)

// HistoricalAssessmentScoreEntry adalah satu nilai asli yang ditranskripsi
// dari bukti per siswa dan per TP. Nilai kosong tidak dikirim sebagai entri.
type HistoricalAssessmentScoreEntry struct {
	StudentID, ObjectiveID int64
	StudentName            string
	Value                  string
	SourceKind             string
	SourceIdentity         string
	SourceLocator          string
	SourceResult           string
	Verification           string
}

type HistoricalAssessmentScoreInput struct {
	ActivityID   int64
	Reason       string
	Verification string
	Acknowledged bool
	Entries      []HistoricalAssessmentScoreEntry
}

type HistoricalAssessmentScoreActivityChoice struct {
	ID, SchoolYearID, SemesterID, ClassID, SubjectID, TeacherID int64
	Name, Date, ClassName, SubjectName, SemesterName            string
	Objectives                                                  []HistoricalAssessmentActivityObjective
}

type HistoricalAssessmentScoreCell struct {
	StudentID, ObjectiveID, PlacementID                                          int64
	FinalVersion                                                                 int64
	BeforeNeedsReview, AfterNeedsReview                                          bool
	StudentName, Relevance, ObjectiveDefinition                                  string
	BeforeNH, AfterNH, AfterStatus, AfterRecommendation                          string
	Value, SourceKind, SourceIdentity, SourceLocator, SourceResult, Verification string
	AlreadyRecorded                                                              bool
}

type HistoricalAssessmentScoreStudent struct {
	ID                                                                                      int64
	Name                                                                                    string
	Relevance                                                                               string
	FinalVersion                                                                            int64
	BeforeNeedsReview, AfterNeedsReview                                                     bool
	BeforeNH, AfterNH, BeforeStatus, AfterStatus, BeforeRecommendation, AfterRecommendation string
	Cells                                                                                   []HistoricalAssessmentScoreCell
}

type HistoricalAssessmentScorePreview struct {
	Input                                                                 HistoricalAssessmentScoreInput
	Activity                                                              HistoricalAssessmentScoreActivityChoice
	TeacherID                                                             int64
	Weights                                                               AssessmentWeights
	Students                                                              []HistoricalAssessmentScoreStudent
	StudentCount, EligibleCount, UnknownCount, NonMemberCount, FinalCount int
	Page, PageCount, PreviousPage, NextPage                               int
	SelectedCount                                                         int
	SourceFingerprint                                                     string
	SnapshotToken                                                         string
	ImpactNotice                                                          string
}

type HistoricalAssessmentScoreConfirmation struct {
	Input                        HistoricalAssessmentScoreInput
	SnapshotToken, SubmissionKey string
}

type HistoricalAssessmentScoreResult struct {
	BatchID    int64
	Activity   HistoricalAssessmentScoreActivityChoice
	Entries    []HistoricalAssessmentScoreEntry
	RecordedAt string
}

func validateHistoricalAssessmentScoreInput(in HistoricalAssessmentScoreInput, requireEntries bool) ([]parsedInitialScore, error) {
	if in.ActivityID < 1 {
		return nil, ValidationError("Kegiatan Sumatif historis tidak valid.")
	}
	valid := func(value string, max int) bool {
		return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && !strings.ContainsRune(value, 0)
	}
	if !valid(in.Reason, 500) || !valid(in.Verification, 1000) {
		return nil, ValidationError("Alasan atau pernyataan pemeriksaan sumber melebihi batas.")
	}
	if requireEntries && strings.TrimSpace(in.Reason) == "" {
		return nil, ValidationError("Alasan pencatatan wajib diisi.")
	}
	if requireEntries && strings.TrimSpace(in.Verification) == "" {
		return nil, ValidationError("Pernyataan pemeriksaan sumber wajib diisi.")
	}
	if requireEntries && !in.Acknowledged {
		return nil, ValidationError("Konfirmasi pemeriksaan sumber dan dampak wajib dicentang.")
	}
	if len(in.Entries) > 100 {
		return nil, ValidationError("Satu batch maksimal memuat 100 nilai.")
	}
	seen := map[[2]int64]bool{}
	locators := map[string]bool{}
	parsed := make([]parsedInitialScore, 0, len(in.Entries))
	sort.Slice(in.Entries, func(i, j int) bool {
		if in.Entries[i].StudentID == in.Entries[j].StudentID {
			return in.Entries[i].ObjectiveID < in.Entries[j].ObjectiveID
		}
		return in.Entries[i].StudentID < in.Entries[j].StudentID
	})
	for _, entry := range in.Entries {
		if entry.StudentID < 1 || entry.ObjectiveID < 1 {
			return nil, ValidationError("Siswa atau TP pada nilai historis tidak valid.")
		}
		key := [2]int64{entry.StudentID, entry.ObjectiveID}
		if seen[key] {
			return nil, ValidationError("Satu pasangan siswa dan TP hanya boleh muncul sekali dalam batch.")
		}
		seen[key] = true
		value, present, err := parseInitialScore(entry.Value)
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, ValidationError("Entri nilai historis tidak boleh kosong; hapus entri bila tidak dipilih.")
		}
		if entry.SourceKind != "arsip_manual" && entry.SourceKind != "aplikasi" {
			return nil, ValidationError("Pilih sumber arsip manual atau sumber aplikasi.")
		}
		for _, field := range []struct {
			value string
			max   int
		}{{entry.SourceIdentity, 500}, {entry.SourceLocator, 1000}, {entry.SourceResult, 500}, {entry.Verification, 1000}} {
			if !valid(field.value, field.max) || strings.TrimSpace(field.value) == "" {
				return nil, ValidationError("Setiap nilai memerlukan identitas sumber, lokasi spesifik, hasil per siswa/TP, dan pemeriksaan guru.")
			}
		}
		loc := canonicalHistoricalActivitySourceIdentity(entry.SourceIdentity) + "\x00" + canonicalHistoricalActivitySourceIdentity(entry.SourceLocator)
		if locators[loc] {
			return nil, ValidationError("Lokasi bukti yang sama tidak boleh dipakai untuk beberapa pasangan siswa/TP.")
		}
		locators[loc] = true
		parsed = append(parsed, parsedInitialScore{studentID: entry.StudentID, objectiveID: entry.ObjectiveID, cents: value})
	}
	if requireEntries && len(parsed) == 0 {
		return nil, ValidationError("Pilih sedikitnya satu nilai yang didukung bukti.")
	}
	return parsed, nil
}

func lockHistoricalScoreRows(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}

// HistoricalAssessmentScoreChoices hanya menampilkan kegiatan Sumatif lama
// yang masih dimiliki guru dan yang ia ampu pada tanggal kegiatan.
func (s *Store) HistoricalAssessmentScoreChoices(ctx context.Context, actor int64) ([]HistoricalAssessmentScoreActivityChoice, error) {
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.semester_id,a.class_id,a.subject_id,a.teacher_id,a.name,a.activity_date::text,
		c.grade::text||c.name,s.name,y.name||' · Semester '||sm.number::text
		FROM app.assessment_activities a JOIN app.semesters sm ON sm.id=a.semester_id
		JOIN app.school_years y ON y.id=sm.school_year_id JOIN app.classes c ON c.id=a.class_id
		JOIN app.subjects s ON s.id=a.subject_id
		WHERE a.teacher_id=$1 AND a.activity_type='sumatif'
		AND EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=a.semester_id AND f.class_id=a.class_id AND f.subject_id=a.subject_id)
		AND EXISTS(SELECT 1 FROM app.teaching_assignments ta WHERE ta.teacher_id=a.teacher_id AND ta.semester_id=a.semester_id AND ta.class_id=a.class_id AND ta.subject_id=a.subject_id AND ta.effective_from<=a.activity_date AND (ta.effective_until IS NULL OR ta.effective_until>a.activity_date))
		AND (EXISTS(SELECT 1 FROM app.historical_assessment_objective_links l WHERE l.activity_id=a.id AND l.teacher_id=$1)
		 OR EXISTS(SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=a.id AND h.teacher_id=$1))
		ORDER BY a.activity_date DESC,a.id DESC`, teacher)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	choices := []HistoricalAssessmentScoreActivityChoice{}
	for rows.Next() {
		var c HistoricalAssessmentScoreActivityChoice
		if err = rows.Scan(&c.ID, &c.SemesterID, &c.ClassID, &c.SubjectID, &c.TeacherID, &c.Name, &c.Date, &c.ClassName, &c.SubjectName, &c.SemesterName); err != nil {
			return nil, err
		}
		choices = append(choices, c)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range choices {
		choices[i].Objectives, err = historicalScoreObjectives(ctx, tx, teacher, choices[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return choices, nil
}

func historicalScoreObjectives(ctx context.Context, tx *sql.Tx, teacher, activity int64) ([]HistoricalAssessmentActivityObjective, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT lo.id,lo.definition FROM app.assessment_activities a
		JOIN app.assessment_activity_objectives ao ON ao.activity_id=a.id JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id
		WHERE a.id=$1 AND a.teacher_id=$2 AND ((EXISTS(SELECT 1 FROM app.historical_assessment_objective_links l WHERE l.activity_id=a.id AND l.learning_objective_id=lo.id AND l.teacher_id=$2))
		OR EXISTS(SELECT 1 FROM app.historical_assessment_activity_insertions h JOIN app.historical_assessment_activity_objectives d ON d.audit_id=h.id WHERE h.activity_id=a.id AND d.learning_objective_id=lo.id AND h.teacher_id=$2)) ORDER BY lo.id`, activity, teacher)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoricalAssessmentActivityObjective{}
	for rows.Next() {
		var o HistoricalAssessmentActivityObjective
		if err = rows.Scan(&o.ID, &o.Definition); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) PreviewHistoricalAssessmentScores(ctx context.Context, actor int64, in HistoricalAssessmentScoreInput, page int, expected string) (HistoricalAssessmentScorePreview, error) {
	if page < 1 {
		return HistoricalAssessmentScorePreview{}, ValidationError("Halaman tidak valid.")
	}
	if _, err := validateHistoricalAssessmentScoreInput(in, false); err != nil {
		return HistoricalAssessmentScorePreview{}, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return HistoricalAssessmentScorePreview{}, err
	}
	defer tx.Rollback()
	return s.previewHistoricalAssessmentScoresTx(ctx, tx, actor, in, page, expected)
}

func (s *Store) previewHistoricalAssessmentScoresTx(ctx context.Context, tx *sql.Tx, actor int64, in HistoricalAssessmentScoreInput, page int, expected string) (HistoricalAssessmentScorePreview, error) {
	out := HistoricalAssessmentScorePreview{Input: in, Page: page}
	if err := s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return out, err
	}
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	out.TeacherID = teacher
	var sourceState string
	err = tx.QueryRowContext(ctx, `SELECT jsonb_build_object('activity',to_jsonb(a),'teacher',to_jsonb(t),'user',to_jsonb(u),
		'assignment',coalesce((SELECT jsonb_agg(to_jsonb(ta) ORDER BY ta.id) FROM app.teaching_assignments ta WHERE ta.teacher_id=a.teacher_id AND ta.semester_id=a.semester_id AND ta.class_id=a.class_id AND ta.subject_id=a.subject_id),'[]'::jsonb),
		'finals',coalesce((SELECT jsonb_agg(to_jsonb(f) ORDER BY f.student_id,f.version) FROM app.semester_final_versions f WHERE f.semester_id=a.semester_id AND f.class_id=a.class_id AND f.subject_id=a.subject_id),'[]'::jsonb),
		'weights',coalesce((SELECT to_jsonb(w) FROM app.semester_assessment_weights w WHERE w.semester_id=a.semester_id),'{}'::jsonb),
		'calendar',coalesce((SELECT to_jsonb(cal) FROM app.semester_calendars cal WHERE cal.semester_id=a.semester_id),'{}'::jsonb),
		'context_objectives',coalesce((SELECT jsonb_agg(jsonb_build_object('objective',to_jsonb(lo),'criteria',coalesce((SELECT jsonb_agg(to_jsonb(tc) ORDER BY tc.id) FROM app.tp_achievement_criteria tc WHERE tc.semester_id=lo.semester_id AND tc.class_id=lo.class_id AND tc.subject_id=lo.subject_id AND (tc.objective_id IS NULL OR tc.objective_id=lo.id)),'[]'::jsonb),'cp20',coalesce((SELECT to_jsonb(h) FROM app.historical_learning_objective_insertions h WHERE h.learning_objective_id=lo.id),'{}'::jsonb)) ORDER BY lo.id) FROM app.learning_objectives lo WHERE lo.school_year_id=a.school_year_id AND lo.semester_id=a.semester_id AND lo.class_id=a.class_id AND lo.subject_id=a.subject_id),'[]'::jsonb),
		'context_activities',coalesce((SELECT jsonb_agg(jsonb_build_object('activity',to_jsonb(ca),'objectives',coalesce((SELECT jsonb_agg(to_jsonb(cao) ORDER BY cao.learning_objective_id) FROM app.assessment_activity_objectives cao WHERE cao.activity_id=ca.id),'[]'::jsonb),'scores',coalesce((SELECT jsonb_agg(jsonb_build_object('score',to_jsonb(sc),'corrections',coalesce((SELECT jsonb_agg(to_jsonb(cc) ORDER BY cc.version) FROM app.assessment_activity_score_corrections cc WHERE cc.initial_score_id=sc.id),'[]'::jsonb),'remedials',coalesce((SELECT jsonb_agg(to_jsonb(cr) ORDER BY cr.id) FROM app.assessment_activity_score_remedials cr WHERE cr.initial_score_id=sc.id),'[]'::jsonb)) ORDER BY sc.student_id,sc.learning_objective_id) FROM app.assessment_activity_initial_scores sc WHERE sc.assessment_activity_id=ca.id),'[]'::jsonb)) ORDER BY ca.id) FROM app.assessment_activities ca WHERE ca.semester_id=a.semester_id AND ca.class_id=a.class_id AND ca.subject_id=a.subject_id),'[]'::jsonb),
		'exam_scores',coalesce((SELECT jsonb_agg(jsonb_build_object('score',to_jsonb(e),'corrections',coalesce((SELECT jsonb_agg(to_jsonb(ec) ORDER BY ec.version) FROM app.exam_score_corrections ec WHERE ec.initial_score_id=e.id),'[]'::jsonb),'remedials',coalesce((SELECT jsonb_agg(to_jsonb(er) ORDER BY er.id) FROM app.exam_score_remedials er WHERE er.initial_score_id=e.id),'[]'::jsonb)) ORDER BY e.student_id,e.component) FROM app.exam_initial_scores e WHERE e.semester_id=a.semester_id AND e.class_id=a.class_id AND e.subject_id=a.subject_id),'[]'::jsonb),
		'objectives',coalesce((SELECT jsonb_agg(jsonb_build_object('objective',to_jsonb(lo),'relation',to_jsonb(ao),'cp21',to_jsonb(l)) ORDER BY lo.id) FROM app.assessment_activity_objectives ao JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id LEFT JOIN app.historical_assessment_objective_links l ON l.activity_id=ao.activity_id AND l.learning_objective_id=ao.learning_objective_id WHERE ao.activity_id=a.id),'[]'::jsonb),
		'cp22',coalesce((SELECT jsonb_agg(jsonb_build_object('audit',to_jsonb(h),'detail',to_jsonb(d)) ORDER BY d.learning_objective_id) FROM app.historical_assessment_activity_insertions h JOIN app.historical_assessment_activity_objectives d ON d.audit_id=h.id WHERE h.activity_id=a.id),'[]'::jsonb),
		'placements',coalesce((SELECT jsonb_agg(jsonb_build_object('placement',to_jsonb(p),'moves',coalesce((SELECT jsonb_agg(to_jsonb(m) ORDER BY m.effective_date,m.id) FROM app.student_class_moves m WHERE m.placement_id=p.id),'[]'::jsonb),'returns',coalesce((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.effective_date,r.id) FROM app.student_school_returns r WHERE r.placement_id=p.id),'[]'::jsonb)) ORDER BY p.student_id,p.id) FROM app.student_placements p WHERE p.school_year_id=a.school_year_id AND (p.class_id=a.class_id OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.from_class_id=a.class_id OR m.to_class_id=a.class_id)) OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=a.class_id))),'[]'::jsonb),
		'scores',coalesce((SELECT jsonb_agg(jsonb_build_object('score',to_jsonb(sc),'corrections',coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.version) FROM app.assessment_activity_score_corrections c WHERE c.initial_score_id=sc.id),'[]'::jsonb),'remedials',coalesce((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM app.assessment_activity_score_remedials r WHERE r.initial_score_id=sc.id),'[]'::jsonb)) ORDER BY sc.student_id,sc.learning_objective_id) FROM app.assessment_activity_initial_scores sc WHERE sc.assessment_activity_id=a.id),'[]'::jsonb))::text,
		a.id,a.semester_id,a.class_id,a.subject_id,a.teacher_id,a.name,a.activity_date::text,a.school_year_id,
		c.grade::text||c.name,s.name,y.name||' · Semester '||sm.number::text,
		(SELECT count(DISTINCT f.student_id) FROM app.semester_final_versions f WHERE f.semester_id=a.semester_id AND f.class_id=a.class_id AND f.subject_id=a.subject_id)
		FROM app.assessment_activities a JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.school_years y ON y.id=sm.school_year_id
		JOIN app.classes c ON c.id=a.class_id JOIN app.subjects s ON s.id=a.subject_id
		JOIN app.teachers t ON t.id=a.teacher_id AND t.user_id=$2 AND t.active JOIN app.users u ON u.id=$2 AND u.active
		WHERE a.id=$1 AND a.teacher_id=$3 AND a.activity_type='sumatif'
		AND EXISTS(SELECT 1 FROM app.teaching_assignments ta WHERE ta.teacher_id=a.teacher_id AND ta.semester_id=a.semester_id AND ta.class_id=a.class_id AND ta.subject_id=a.subject_id AND ta.effective_from<=a.activity_date AND (ta.effective_until IS NULL OR ta.effective_until>a.activity_date))
		AND EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=a.semester_id AND f.class_id=a.class_id AND f.subject_id=a.subject_id)
		AND (EXISTS(SELECT 1 FROM app.historical_assessment_objective_links l WHERE l.activity_id=a.id AND l.teacher_id=a.teacher_id) OR EXISTS(SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.activity_id=a.id AND h.teacher_id=a.teacher_id))`, in.ActivityID, actor, teacher).Scan(&sourceState, &out.Activity.ID, &out.Activity.SemesterID, &out.Activity.ClassID, &out.Activity.SubjectID, &out.Activity.TeacherID, &out.Activity.Name, &out.Activity.Date, &out.Activity.SchoolYearID, &out.Activity.ClassName, &out.Activity.SubjectName, &out.Activity.SemesterName, &out.FinalCount)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrForbidden
	}
	if err != nil {
		return out, err
	}
	out.Activity.ID = in.ActivityID
	// Source date may not be in the future; validity is based on WIB.
	var future bool
	if err = tx.QueryRowContext(ctx, `SELECT $1::date>(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date`, out.Activity.Date).Scan(&future); err != nil {
		return out, err
	}
	if future {
		return out, ValidationError("Tanggal kegiatan Sumatif tidak boleh di masa depan.")
	}
	out.Activity.Objectives, err = historicalScoreObjectives(ctx, tx, teacher, in.ActivityID)
	if err != nil {
		return out, err
	}
	if len(out.Activity.Objectives) == 0 {
		return out, ErrForbidden
	}
	objectiveSet := map[int64]bool{}
	for _, o := range out.Activity.Objectives {
		objectiveSet[o.ID] = true
	}
	parsed, err := validateHistoricalAssessmentScoreInput(in, false)
	if err != nil {
		return out, err
	}
	if len(parsed) != len(in.Entries) {
		return out, ValidationError("Daftar nilai pratinjau tidak valid.")
	}
	entryByKey := map[[2]int64]HistoricalAssessmentScoreEntry{}
	for _, e := range in.Entries {
		if !objectiveSet[e.ObjectiveID] {
			return out, ErrForbidden
		}
		entryByKey[[2]int64{e.StudentID, e.ObjectiveID}] = e
	}
	out.SelectedCount = len(in.Entries)
	if err = tx.QueryRowContext(ctx, `SELECT nh,asts,asas,version,TRUE FROM app.semester_assessment_weights WHERE semester_id=$1`, out.Activity.SemesterID).Scan(&out.Weights.NH, &out.Weights.ASTS, &out.Weights.ASAS, &out.Weights.Version, &out.Weights.Configured); errors.Is(err, sql.ErrNoRows) {
		out.Weights.SemesterID = out.Activity.SemesterID
		err = nil
	}
	if err != nil {
		return out, err
	}
	out.Weights.SemesterID = out.Activity.SemesterID
	var rosterRows *sql.Rows
	rosterRows, err = tx.QueryContext(ctx, `WITH candidates AS (
		SELECT p.student_id FROM app.student_placements p WHERE p.school_year_id=(SELECT school_year_id FROM app.semesters WHERE id=$1) AND (p.class_id=$2 OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.from_class_id=$2 OR m.to_class_id=$2)) OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=$2))
		UNION SELECT student_id FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3
	) SELECT st.id,st.name,
		(SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=(SELECT school_year_id FROM app.semesters WHERE id=$1) AND p.student_id=st.id AND p.starts_on IS NULL),
		(SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=(SELECT school_year_id FROM app.semesters WHERE id=$1) AND p.student_id=st.id AND p.starts_on IS NOT NULL AND p.starts_on<=$4::date AND app.student_placement_class_at(p.id,$4::date)=$2),
		(SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=(SELECT school_year_id FROM app.semesters WHERE id=$1) AND p.student_id=st.id AND p.starts_on IS NOT NULL AND p.starts_on<=$4::date AND app.student_placement_class_at(p.id,$4::date) IS NOT NULL),
		coalesce((SELECT fv.version FROM app.semester_final_versions fv WHERE fv.semester_id=$1 AND fv.class_id=$2 AND fv.subject_id=$3 AND fv.student_id=st.id ORDER BY fv.version DESC LIMIT 1),0)
	FROM candidates ca JOIN app.students st ON st.id=ca.student_id ORDER BY st.name,st.id`, out.Activity.SemesterID, out.Activity.ClassID, out.Activity.SubjectID, out.Activity.Date)
	if err != nil {
		return out, err
	}
	type roster struct {
		id                       int64
		name                     string
		unknown, members, placed int
		final                    int64
	}
	all := []roster{}
	for rosterRows.Next() {
		var r roster
		if err = rosterRows.Scan(&r.id, &r.name, &r.unknown, &r.members, &r.placed, &r.final); err != nil {
			rosterRows.Close()
			return out, err
		}
		all = append(all, r)
	}
	if err = rosterRows.Err(); err != nil {
		rosterRows.Close()
		return out, err
	}
	if err = rosterRows.Close(); err != nil {
		return out, err
	}
	rosterByID := make(map[int64]roster, len(all))
	for _, r := range all {
		rosterByID[r.id] = r
	}
	placementByStudent := make(map[int64]int64, len(entryByKey))
	for key := range entryByKey {
		r, ok := rosterByID[key[0]]
		if !ok || r.unknown != 0 || r.placed > 1 || r.members != 1 {
			return out, ErrForbidden
		}
		if _, found := placementByStudent[key[0]]; !found {
			var placementID int64
			if err = tx.QueryRowContext(ctx, `SELECT p.id FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=$2 AND p.starts_on IS NOT NULL AND p.starts_on<=$3::date AND app.student_placement_class_at(p.id,$3::date)=$4 ORDER BY p.id`, out.Activity.SchoolYearID, key[0], out.Activity.Date, out.Activity.ClassID).Scan(&placementID); err != nil {
				return out, err
			}
			placementByStudent[key[0]] = placementID
		}
	}
	// Kisi bukti secara terarah pada setiap sel yang dipilih dan pastikan sumber belum pernah dicatat.
	for key := range entryByKey {
		var scoreExists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3)`, in.ActivityID, key[1], key[0]).Scan(&scoreExists); err != nil {
			return out, err
		}
		if scoreExists {
			return out, ErrConflict
		}
	}
	out.FinalCount = 0
	for _, r := range all {
		out.StudentCount++
		rel := "Belum dapat diverifikasi"
		if r.unknown == 0 && r.placed <= 1 {
			if r.members == 1 {
				rel = "Kandidat anggota pada tanggal kegiatan"
			} else {
				rel = "Bukan anggota pada tanggal kegiatan"
			}
		}
		if rel == "Kandidat anggota pada tanggal kegiatan" {
			out.EligibleCount++
		} else if rel == "Bukan anggota pada tanggal kegiatan" {
			out.NonMemberCount++
		} else {
			out.UnknownCount++
		}
		if r.final > 0 {
			out.FinalCount++
		}
	}
	const pageSize = 10
	out.PageCount = (len(all) + pageSize - 1) / pageSize
	if out.PageCount == 0 {
		out.PageCount = 1
	}
	if page > out.PageCount {
		return out, ValidationError("Halaman nilai historis tidak tersedia.")
	}
	if page > 1 && expected == "" {
		return out, ErrConflict
	}
	if page > 1 {
		out.PreviousPage = page - 1
	}
	if page < out.PageCount {
		out.NextPage = page + 1
	}
	start, end := (page-1)*pageSize, page*pageSize
	if end > len(all) {
		end = len(all)
	}
	for _, r := range all[start:end] {
		st := HistoricalAssessmentScoreStudent{ID: r.id, Name: r.name, FinalVersion: r.final}
		if r.unknown == 0 && r.placed <= 1 && r.members == 1 {
			st.Relevance = "Kandidat anggota pada tanggal kegiatan"
		} else if r.unknown == 0 && r.placed <= 1 {
			st.Relevance = "Bukan anggota pada tanggal kegiatan"
		} else {
			st.Relevance = "Belum dapat diverifikasi"
		}
		for _, o := range out.Activity.Objectives {
			cell := HistoricalAssessmentScoreCell{StudentID: r.id, StudentName: r.name, Relevance: st.Relevance, ObjectiveID: o.ID, ObjectiveDefinition: o.Definition, PlacementID: placementByStudent[r.id]}
			if e, ok := entryByKey[[2]int64{r.id, o.ID}]; ok {
				cell.Value = e.Value
				cell.SourceKind = e.SourceKind
				cell.SourceIdentity = e.SourceIdentity
				cell.SourceLocator = e.SourceLocator
				cell.SourceResult = e.SourceResult
				cell.Verification = e.Verification
			}
			var exists bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND learning_objective_id=$2 AND student_id=$3)`, in.ActivityID, o.ID, r.id).Scan(&exists); err != nil {
				return out, err
			}
			cell.AlreadyRecorded = exists
			st.Cells = append(st.Cells, cell)
		}
		if st.Relevance == "Kandidat anggota pada tanggal kegiatan" {
			rec, e := s.semesterRecommendationsHistoricalScoreStudentTx(ctx, tx, actor, out.Activity.SemesterID, out.Activity.ClassID, out.Activity.SubjectID, r.id, in.ActivityID)
			if e != nil {
				return out, e
			}
			if e = s.overlaySemesterFinals(ctx, tx, &rec); e != nil {
				return out, e
			}
			for _, rs := range rec.Students {
				if rs.ID == fmtID(r.id) {
					st.BeforeNH = rs.NH
					st.BeforeStatus = rs.Status
					st.BeforeRecommendation = rs.Recommendation
					st.BeforeNeedsReview = rs.NeedsReview
					st.AfterNeedsReview = rs.NeedsReview
					if rs.Finalized {
						for key := range entryByKey {
							if key[0] == r.id {
								// Snapshot Final memuat sumber nilai awal. Setiap slot baru
								// mengubah fingerprint input Final secara dinamis, tanpa
								// menulis ulang versi atau snapshot historis.
								st.AfterNeedsReview = true
								break
							}
						}
					}
					st.AfterNH, st.AfterStatus, st.AfterRecommendation = projectHistoricalScoreAfter(rec, out.Activity.ID, r.id, entryByKey, out.Weights)
					break
				}
			}
		}
		for i := range st.Cells {
			st.Cells[i].FinalVersion = st.FinalVersion
			st.Cells[i].BeforeNeedsReview = st.BeforeNeedsReview
			st.Cells[i].AfterNeedsReview = st.AfterNeedsReview
			st.Cells[i].BeforeNH = st.BeforeNH
			st.Cells[i].AfterNH = st.AfterNH
			st.Cells[i].AfterStatus = st.AfterStatus
			st.Cells[i].AfterRecommendation = st.AfterRecommendation
		}
		out.Students = append(out.Students, st)
	}
	out.ImpactNotice = "Nilai hanya ditranskripsi untuk sel yang dipilih dan didukung bukti per siswa/TP. Nilai kosong tetap kosong, nilai 0 sah. Nilai Final dan snapshot lama tidak diubah; Final terkait dapat ditandai Perlu Ditinjau setelah penyimpanan."
	stateHash := sha256.Sum256([]byte(sourceState))
	out.SourceFingerprint = hex.EncodeToString(stateHash[:])
	inputJSON, err := json.Marshal(struct {
		Input             HistoricalAssessmentScoreInput
		SourceFingerprint string
	}{in, out.SourceFingerprint})
	if err != nil {
		return out, err
	}
	sum := sha256.Sum256(inputJSON)
	out.SnapshotToken = hex.EncodeToString(sum[:])
	if expected != "" && expected != out.SourceFingerprint {
		return HistoricalAssessmentScorePreview{}, ErrConflict
	}
	return out, nil
}

func projectHistoricalScoreAfter(rec SemesterRecommendation, activityID, studentID int64, entries map[[2]int64]HistoricalAssessmentScoreEntry, w AssessmentWeights) (string, string, string) {
	// Reuse the per-student TP/activity snapshot returned by the approved
	// rekap service. This projection changes only selected cells in memory.
	selected := map[int64]*big.Rat{}
	for k, e := range entries {
		if k[0] == studentID {
			v, ok := new(big.Rat).SetString(strings.ReplaceAll(e.Value, ",", "."))
			if ok {
				selected[k[1]] = v
			}
		}
	}
	var nh *big.Rat
	complete := false
	scored := false
	var student *SemesterRecommendationStudent
	for i := range rec.Students {
		if rec.Students[i].ID == fmtID(studentID) {
			student = &rec.Students[i]
			break
		}
	}
	if student == nil {
		return "", "Belum dapat diverifikasi", ""
	}
	complete = len(student.Objectives) > 0
	for _, objective := range student.Objectives {
		activities := append([]AssessmentTPRecapActivity(nil), objective.Activities...)
		required, scoredCount := objective.RequiredCount, objective.ScoredCount
		scored = scored || scoredCount > 0
		if val, ok := selectedObjectiveValue(selected, objective.ID); ok {
			found := false
			for i := range activities {
				if activities[i].ID == fmtID(activityID) {
					activities[i].HasValue = true
					activities[i].EffectiveValue = val.FloatString(2)
					found = true
					break
				}
			}
			if !found {
				// Bila service rekap tidak menganggap kegiatan berlaku bagi siswa,
				// jangan mengarang kewajiban atau angka pada proyeksi.
				complete = false
				continue
			}
			scoredCount++
			scored = true
		}
		if required > 0 && scoredCount < required {
			complete = false
			continue
		}
		if len(activities) == 0 {
			complete = false
			continue
		}
		avg, err := exactAverage(activities)
		if err != nil {
			complete = false
			continue
		}
		if nh == nil {
			nh = new(big.Rat)
		}
		nh.Add(nh, avg)
	}
	if nh != nil && len(student.Objectives) > 0 {
		nh.Quo(nh, big.NewRat(int64(len(student.Objectives)), 1))
	}
	if w.NH > 0 && !complete {
		return "", "Belum Lengkap", ""
	}
	if !w.Configured {
		return "Belum dikonfigurasi", "Belum dapat ditentukan", ""
	}
	asts, asas := new(big.Rat), new(big.Rat)
	hasASTS, hasASAS := false, false
	if v, ok := new(big.Rat).SetString(strings.ReplaceAll(student.ASTS, ",", ".")); ok {
		asts = v
		hasASTS = true
	}
	if v, ok := new(big.Rat).SetString(strings.ReplaceAll(student.ASAS, ",", ".")); ok {
		asas = v
		hasASAS = true
	}
	if w.ASTS > 0 && !hasASTS || w.ASAS > 0 && !hasASAS {
		return "", "Belum Lengkap", ""
	}
	if !scored && !hasASTS && !hasASAS {
		return "0,00", "Belum Dinilai", ""
	}
	var recommendationNH *big.Rat
	if w.NH > 0 {
		recommendationNH = nh
	}
	var shownNH string
	if nh != nil && complete {
		shownNH = strings.ReplaceAll(nh.FloatString(2), ".", ",")
	}
	return shownNH, "Siap Final", semesterRound(recommendationNH, asts, asas, w)
}

func selectedObjectiveValue(selected map[int64]*big.Rat, objectiveID string) (*big.Rat, bool) {
	for id, value := range selected {
		if fmtID(id) == objectiveID {
			return value, true
		}
	}
	return nil, false
}

func (s *Store) ConfirmHistoricalAssessmentScores(ctx context.Context, actor int64, req HistoricalAssessmentScoreConfirmation) (int64, error) {
	parsed, err := validateHistoricalAssessmentScoreInput(req.Input, true)
	if err != nil {
		return 0, err
	}
	if !validSubmissionUUID(req.SubmissionKey) {
		return 0, ValidationError("Identitas pengiriman tidak valid.")
	}
	token, err := hex.DecodeString(req.SnapshotToken)
	if err != nil || len(token) != sha256.Size {
		return 0, ValidationError("Snapshot pratinjau tidak valid.")
	}
	requestBytes, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	requestHash := sha256.Sum256(requestBytes)
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(742009)`); err != nil {
		return 0, err
	}
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return 0, err
	}
	var oldID int64
	var oldHash []byte
	err = tx.QueryRowContext(ctx, `SELECT id,request_sha256 FROM app.historical_assessment_score_batches WHERE actor_id=$1 AND teacher_id=$2 AND submission_key=$3::uuid`, actor, teacher, req.SubmissionKey).Scan(&oldID, &oldHash)
	if err == nil {
		if !equalHash(oldHash, requestHash[:]) {
			return 0, ErrConflict
		}
		return oldID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	// Kunci sumber dan semua fakta yang dapat mengubah hak atau kelayakan sebelum
	// menghitung ulang fingerprint dan menulis satu batch atomik.
	var owner int64
	if err = tx.QueryRowContext(ctx, `SELECT teacher_id FROM app.assessment_activities WHERE id=$1 FOR UPDATE`, req.Input.ActivityID).Scan(&owner); errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if owner != teacher {
		return 0, ErrForbidden
	}
	if err = lockHistoricalScoreRows(ctx, tx, `SELECT id FROM app.teaching_assignments WHERE teacher_id=$1 AND (semester_id,class_id,subject_id)=(SELECT semester_id,class_id,subject_id FROM app.assessment_activities WHERE id=$2) ORDER BY id FOR SHARE`, teacher, req.Input.ActivityID); err != nil {
		return 0, err
	}
	if err = lockHistoricalScoreRows(ctx, tx, `SELECT f.id FROM app.semester_final_versions f WHERE (f.semester_id,f.class_id,f.subject_id)=(SELECT semester_id,class_id,subject_id FROM app.assessment_activities WHERE id=$1) ORDER BY f.id FOR SHARE`, req.Input.ActivityID); err != nil {
		return 0, err
	}
	if err = lockHistoricalScoreRows(ctx, tx, `SELECT lo.id FROM app.assessment_activity_objectives ao JOIN app.learning_objectives lo ON lo.id=ao.learning_objective_id WHERE ao.activity_id=$1 ORDER BY lo.id FOR SHARE OF ao,lo`, req.Input.ActivityID); err != nil {
		return 0, err
	}
	for _, v := range parsed {
		if err = lockHistoricalScoreRows(ctx, tx, `SELECT id FROM app.student_placements WHERE student_id=$1 AND school_year_id=(SELECT school_year_id FROM app.semesters WHERE id=(SELECT semester_id FROM app.assessment_activities WHERE id=$2)) ORDER BY id FOR SHARE`, v.studentID, req.Input.ActivityID); err != nil {
			return 0, err
		}
	}
	preview, err := s.previewHistoricalAssessmentScoresTx(ctx, tx, actor, req.Input, 1, "")
	if err != nil {
		return 0, err
	}
	if preview.SnapshotToken != req.SnapshotToken {
		return 0, ErrConflict
	}
	pages := []HistoricalAssessmentScorePreview{preview}
	for page := 2; page <= preview.PageCount; page++ {
		pagePreview, pageErr := s.previewHistoricalAssessmentScoresTx(ctx, tx, actor, req.Input, page, preview.SourceFingerprint)
		if pageErr != nil {
			return 0, pageErr
		}
		pages = append(pages, pagePreview)
	}
	previewRaw, err := json.Marshal(struct {
		Input                                                                 HistoricalAssessmentScoreInput `json:"input"`
		PreviewHash                                                           string                         `json:"preview_hash"`
		StudentCount, EligibleCount, UnknownCount, NonMemberCount, FinalCount int
		Pages                                                                 []HistoricalAssessmentScorePreview `json:"pages"`
	}{req.Input, req.SnapshotToken, preview.StudentCount, preview.EligibleCount, preview.UnknownCount, preview.NonMemberCount, preview.FinalCount, pages})
	if err != nil {
		return 0, err
	}
	var batchID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app.historical_assessment_score_batches(assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) SELECT a.id,a.school_year_id,a.semester_id,a.class_id,a.subject_id,a.teacher_id,$2,a.activity_date,$3,$4,$5::jsonb,$6,$7,$8::uuid FROM app.assessment_activities a WHERE a.id=$1 RETURNING id`, req.Input.ActivityID, actor, strings.TrimSpace(req.Input.Reason), strings.TrimSpace(req.Input.Verification), string(previewRaw), token, requestHash[:], req.SubmissionKey).Scan(&batchID)
	if err != nil {
		return 0, dbError(err)
	}
	for i, entry := range req.Input.Entries {
		value := formatScore(parsed[i].cents)
		var placement int64
		err = tx.QueryRowContext(ctx, `SELECT p.id FROM app.student_placements p JOIN app.assessment_activities a ON a.id=$2 WHERE p.student_id=$1 AND p.school_year_id=a.school_year_id AND p.starts_on IS NOT NULL AND p.starts_on<=a.activity_date AND app.student_placement_class_at(p.id,a.activity_date)=a.class_id ORDER BY p.id FOR SHARE`, entry.StudentID, req.Input.ActivityID).Scan(&placement)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrConflict
		}
		if err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO app.historical_assessment_initial_score_audits(batch_id,assessment_activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,learning_objective_id,student_id,placement_id,score,source_kind,source_identity,source_locator,source_result,verification) SELECT $1,a.id,a.school_year_id,a.semester_id,a.class_id,a.subject_id,a.teacher_id,$2,a.activity_date,$3,$4,$5,$6,$7,$8,$9,$10,$11 FROM app.assessment_activities a WHERE a.id=$12`, batchID, actor, entry.ObjectiveID, entry.StudentID, placement, value, entry.SourceKind, strings.TrimSpace(entry.SourceIdentity), strings.TrimSpace(entry.SourceLocator), strings.TrimSpace(entry.SourceResult), strings.TrimSpace(entry.Verification), req.Input.ActivityID)
		if err != nil {
			return 0, dbError(err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO app.assessment_activity_initial_scores(assessment_activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id,student_id,placement_id,activity_date_snapshot,score,recorded_by) SELECT a.id,$2,a.school_year_id,a.semester_id,a.class_id,a.subject_id,$3,$4,a.activity_date,$5::numeric,$6 FROM app.assessment_activities a WHERE a.id=$1`, req.Input.ActivityID, entry.ObjectiveID, entry.StudentID, placement, value, actor)
		if err != nil {
			return 0, dbError(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, dbError(err)
	}
	return batchID, nil
}

func (s *Store) HistoricalAssessmentScoreResult(ctx context.Context, actor, batchID int64) (HistoricalAssessmentScoreResult, error) {
	var out HistoricalAssessmentScoreResult
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT h.assessment_activity_id,h.recorded_at::text,h.impact_snapshot FROM app.historical_assessment_score_batches h WHERE h.id=$1 AND h.actor_id=$2 AND h.teacher_id=$3`, batchID, actor, teacher).Scan(&out.Activity.ID, &out.RecordedAt, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrForbidden
	}
	if err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT a.semester_id,a.class_id,a.subject_id,a.teacher_id,a.name,a.activity_date::text,c.grade::text||c.name,s.name,y.name||' · Semester '||sm.number::text FROM app.assessment_activities a JOIN app.semesters sm ON sm.id=a.semester_id JOIN app.school_years y ON y.id=sm.school_year_id JOIN app.classes c ON c.id=a.class_id JOIN app.subjects s ON s.id=a.subject_id WHERE a.id=$1 AND a.teacher_id=$2`, out.Activity.ID, teacher).Scan(&out.Activity.SemesterID, &out.Activity.ClassID, &out.Activity.SubjectID, &out.Activity.TeacherID, &out.Activity.Name, &out.Activity.Date, &out.Activity.ClassName, &out.Activity.SubjectName, &out.Activity.SemesterName); err != nil {
		return out, ErrForbidden
	}
	out.Activity.Objectives, err = historicalScoreObjectives(ctx, tx, teacher, out.Activity.ID)
	if err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT h.student_id,st.name,h.learning_objective_id,h.score::text,h.source_kind,h.source_identity,h.source_locator,h.source_result,h.verification FROM app.historical_assessment_initial_score_audits h JOIN app.students st ON st.id=h.student_id WHERE h.batch_id=$1 ORDER BY h.student_id,h.learning_objective_id`, batchID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e HistoricalAssessmentScoreEntry
		if err = rows.Scan(&e.StudentID, &e.StudentName, &e.ObjectiveID, &e.Value, &e.SourceKind, &e.SourceIdentity, &e.SourceLocator, &e.SourceResult, &e.Verification); err != nil {
			return out, err
		}
		out.Entries = append(out.Entries, e)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	out.BatchID = batchID
	return out, nil
}

func formatHistoricalNH(value *big.Rat) string {
	if value == nil {
		return ""
	}
	return strings.ReplaceAll(value.FloatString(2), ".", ",")
}
