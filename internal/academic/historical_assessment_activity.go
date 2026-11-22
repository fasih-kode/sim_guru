package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// HistoricalAssessmentActivityInput memuat kegiatan Sumatif lama dan referensi
// bukti yang diperiksa guru; data ini tidak mencatat nilai siswa.
type HistoricalAssessmentActivityInput struct {
	SemesterID, ClassID, SubjectID int64
	ObjectiveIDs                   []int64
	Name, ActivityDate             string
	Reason, SourceIdentity         string
	SourceDocument, SourceArchive  string
	SourceTeacher, SourceSection   string
	ResultSource, Method           string
	OutsideCalendarReason          string
	DuplicateExplanation           string
	Acknowledged, DuplicateChecked bool
}

type HistoricalAssessmentActivityObjective struct {
	ID         int64
	Definition string
}

type HistoricalAssessmentActivityStudent struct {
	ID, FinalVersion                       int64
	Name, Relevance                        string
	NH, ASTS, ASAS, Status, Recommendation string
	FinalValue                             string
	After                                  string
}

type HistoricalAssessmentActivityDuplicate struct {
	ID         int64
	Name, Date string
}

type HistoricalAssessmentActivityPreview struct {
	Input                                         HistoricalAssessmentActivityInput
	Context                                       LearningObjectiveContext
	YearID, TeacherID                             int64
	Weights                                       AssessmentWeights
	Objectives                                    []HistoricalAssessmentActivityObjective
	Students                                      []HistoricalAssessmentActivityStudent
	SimilarActivities                             []HistoricalAssessmentActivityDuplicate
	StudentCount, MemberCount, NonMemberCount     int
	UnknownCount, FinalCount, Page, PageCount     int
	PreviousPage, NextPage                        int
	OutsideCalendar, HasFinal, NumericImpactKnown bool
	ImpactNotice, SnapshotToken                   string
}

type HistoricalAssessmentActivityConfirmation struct {
	Input                        HistoricalAssessmentActivityInput
	SnapshotToken, SubmissionKey string
}

type HistoricalAssessmentActivityResult struct {
	AuditID, ActivityID int64
	Preview             HistoricalAssessmentActivityPreview
}

func validateHistoricalAssessmentActivityInput(in HistoricalAssessmentActivityInput) error {
	if in.SemesterID < 1 || in.ClassID < 1 || in.SubjectID < 1 {
		return ValidationError("Pilih konteks semester, kelas, dan mata pelajaran.")
	}
	if len(in.ObjectiveIDs) == 0 || len(in.ObjectiveIDs) > 100 {
		return ValidationError("Pilih satu sampai 100 Tujuan Pembelajaran.")
	}
	seen := make(map[int64]bool, len(in.ObjectiveIDs))
	for _, id := range in.ObjectiveIDs {
		if id < 1 || seen[id] {
			return ValidationError("Pilihan TP tidak valid atau berulang.")
		}
		seen[id] = true
	}
	valid := func(v string, max int, required bool) bool {
		return utf8.ValidString(v) && utf8.RuneCountInString(v) <= max && !strings.ContainsRune(v, 0) && (!required || strings.TrimSpace(v) != "")
	}
	for _, field := range []struct {
		value string
		max   int
	}{
		{in.Name, 200}, {in.Reason, 500}, {in.SourceIdentity, 500},
		{in.SourceDocument, 500}, {in.SourceArchive, 500}, {in.SourceTeacher, 200},
		{in.SourceSection, 500}, {in.ResultSource, 500}, {in.Method, 500},
		{in.OutsideCalendarReason, 500}, {in.DuplicateExplanation, 500},
	} {
		if !valid(field.value, field.max, false) {
			return ValidationError("Uraian melebihi batas atau tidak valid.")
		}
	}
	for _, v := range []string{in.Name, in.Reason, in.SourceIdentity, in.SourceDocument, in.SourceArchive, in.SourceTeacher, in.SourceSection, in.ResultSource, in.Method} {
		if strings.TrimSpace(v) == "" {
			return ValidationError("Nama, alasan, identitas sumber, dan rincian bukti wajib diisi.")
		}
	}
	if !in.Acknowledged {
		return ValidationError("Pemeriksaan sumber dan dampak wajib dikonfirmasi.")
	}
	d, err := time.Parse("2006-01-02", in.ActivityDate)
	if err != nil || d.Format("2006-01-02") != in.ActivityDate {
		return ValidationError("Tanggal kegiatan tidak valid.")
	}
	return nil
}

func canonicalHistoricalActivitySourceIdentity(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func canonicalHistoricalTeacherName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

// HistoricalAssessmentActivityChoices menampilkan semua TP pada konteks
// penugasan guru yang berwenang; pilihan CP10-21 tetap memakai layanannya sendiri.
func (s *Store) HistoricalAssessmentActivityChoices(ctx context.Context, actor, semester, class, subject int64) ([]HistoricalAssessmentLinkChoice, []HistoricalAssessmentLinkChoice, error) {
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return nil, nil, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return nil, nil, err
	}
	if !allowed {
		return nil, nil, ErrForbidden
	}
	read := func(query string, args ...any) ([]HistoricalAssessmentLinkChoice, error) {
		rows, queryErr := tx.QueryContext(ctx, query, args...)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()
		var choices []HistoricalAssessmentLinkChoice
		for rows.Next() {
			var choice HistoricalAssessmentLinkChoice
			if queryErr = rows.Scan(&choice.ID, &choice.Label); queryErr != nil {
				return nil, queryErr
			}
			choices = append(choices, choice)
		}
		return choices, rows.Err()
	}
	objectives, err := read(`SELECT o.id,o.definition FROM app.learning_objectives o
		JOIN app.semesters sm ON sm.id=o.semester_id AND sm.school_year_id=o.school_year_id
		JOIN app.classes c ON c.id=o.class_id AND c.school_year_id=sm.school_year_id
		JOIN app.subjects s ON s.id=o.subject_id AND s.active
		WHERE o.semester_id=$1 AND o.class_id=$2 AND o.subject_id=$3 ORDER BY o.id`, semester, class, subject)
	if err != nil {
		return nil, nil, err
	}
	activities, err := read(`SELECT id,name||' · '||activity_date::text FROM app.assessment_activities
		WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND teacher_id=$4 AND activity_type='sumatif'
		ORDER BY activity_date,id`, semester, class, subject, teacher)
	return objectives, activities, err
}

// PreviewHistoricalAssessmentActivity menghitung konteks dan dampak secara baca-saja.
func (s *Store) PreviewHistoricalAssessmentActivity(ctx context.Context, actor int64, in HistoricalAssessmentActivityInput, page int, expected string) (HistoricalAssessmentActivityPreview, error) {
	if err := validateHistoricalAssessmentActivityInput(in); err != nil {
		return HistoricalAssessmentActivityPreview{}, err
	}
	if page < 1 {
		return HistoricalAssessmentActivityPreview{}, ValidationError("Halaman tidak valid.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return HistoricalAssessmentActivityPreview{}, err
	}
	defer tx.Rollback()
	return s.previewHistoricalAssessmentActivityTx(ctx, tx, actor, in, page, expected)
}

func (s *Store) previewHistoricalAssessmentActivityTx(ctx context.Context, tx *sql.Tx, actor int64, in HistoricalAssessmentActivityInput, page int, expected string) (HistoricalAssessmentActivityPreview, error) {
	var out HistoricalAssessmentActivityPreview
	if err := validateHistoricalAssessmentActivityInput(in); err != nil {
		return out, err
	}
	if err := s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return out, err
	}
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, in.SemesterID, in.ClassID, in.SubjectID)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, ErrForbidden
	}
	var eventAssignment, future bool
	err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM app.teaching_assignments a WHERE a.teacher_id=$1 AND a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4 AND a.effective_from<=$5::date AND (a.effective_until IS NULL OR a.effective_until>$5::date)),
		$5::date>(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date`, teacher, in.SemesterID, in.ClassID, in.SubjectID, in.ActivityDate).Scan(&eventAssignment, &future)
	if err != nil {
		return out, err
	}
	if !eventAssignment {
		return out, ErrForbidden
	}
	if future {
		return out, ValidationError("Tanggal kegiatan historis tidak boleh di masa depan.")
	}
	var teacherName string
	if err = tx.QueryRowContext(ctx, `SELECT t.name FROM app.teachers t JOIN app.users u ON u.id=t.user_id WHERE t.id=$1 AND t.active AND u.active`, teacher).Scan(&teacherName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrForbidden
		}
		return out, err
	}
	if canonicalHistoricalTeacherName(in.SourceTeacher) != canonicalHistoricalTeacherName(teacherName) {
		// Pada saat konfirmasi, perbedaan ini berarti metadata yang terikat pada
		// pratinjau berubah; pada pratinjau baru, sumber berbeda harus ditolak.
		if expected != "" {
			return out, ErrConflict
		}
		return out, ValidationError("Nama guru penanggung jawab pada sumber harus sesuai identitas guru pada penugasan kegiatan.")
	}
	out.Input, out.TeacherID = in, teacher
	var calendarExists bool
	err = tx.QueryRowContext(ctx, `SELECT sm.school_year_id,y.name||' · Semester '||sm.number::text,c.grade::text||c.name,s.name,EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=sm.id AND f.class_id=c.id AND f.subject_id=s.id),
		EXISTS(SELECT 1 FROM app.semester_calendars cal WHERE cal.semester_id=sm.id),
		EXISTS(SELECT 1 FROM app.semester_calendars cal WHERE cal.semester_id=sm.id AND $4::date NOT BETWEEN cal.starts_on AND cal.ends_on)
		FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id JOIN app.classes c ON c.id=$2 AND c.school_year_id=sm.school_year_id JOIN app.subjects s ON s.id=$3 AND s.active WHERE sm.id=$1`, in.SemesterID, in.ClassID, in.SubjectID, in.ActivityDate).Scan(&out.YearID, &out.Context.SemesterName, &out.Context.ClassName, &out.Context.SubjectName, &out.HasFinal, &calendarExists, &out.OutsideCalendar)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrForbidden
	}
	if err != nil {
		return out, err
	}
	if !calendarExists {
		return out, ValidationError("Kalender semester belum tersedia untuk memeriksa tanggal kegiatan.")
	}
	out.Context.SemesterID, out.Context.ClassID, out.Context.SubjectID = in.SemesterID, in.ClassID, in.SubjectID
	if !out.HasFinal {
		return out, ValidationError("Koreksi kegiatan historis hanya tersedia pada konteks yang sudah memiliki Final.")
	}
	if out.OutsideCalendar && strings.TrimSpace(in.OutsideCalendarReason) == "" {
		return out, ValidationError("Tanggal di luar kalender memerlukan alasan historis khusus.")
	}
	// Metadata otoritas dan tanggal disertakan utuh, bukan hanya status izin
	// atau kategori kalender, agar perubahan yang masih menyisakan izin tetap
	// membatalkan pratinjau lama.
	var teacherMetadata, assignmentMetadata, calendarMetadata string
	if err = tx.QueryRowContext(ctx, `SELECT jsonb_build_object(
		'teacher',jsonb_build_object('id',t.id,'user_id',t.user_id,'name',t.name,'active',t.active,'version',t.version),
		'user',jsonb_build_object('id',u.id,'username',u.username,'display_name',u.display_name,'active',u.active,'version',u.version)
		)::text
		FROM app.teachers t JOIN app.users u ON u.id=t.user_id WHERE t.id=$1`, teacher).Scan(&teacherMetadata); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY a.id),'[]'::jsonb)::text
		FROM app.teaching_assignments a WHERE a.teacher_id=$1 AND a.semester_id=$2 AND a.class_id=$3 AND a.subject_id=$4`, teacher, in.SemesterID, in.ClassID, in.SubjectID).Scan(&assignmentMetadata); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(c) ORDER BY c.semester_id),'[]'::jsonb)::text
		FROM app.semester_calendars c WHERE c.semester_id=$1`, in.SemesterID).Scan(&calendarMetadata); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,definition FROM app.learning_objectives WHERE id=ANY($1::bigint[]) AND school_year_id=$2 AND semester_id=$3 AND class_id=$4 AND subject_id=$5 ORDER BY id`, in.ObjectiveIDs, out.YearID, in.SemesterID, in.ClassID, in.SubjectID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var objective HistoricalAssessmentActivityObjective
		if err = rows.Scan(&objective.ID, &objective.Definition); err != nil {
			rows.Close()
			return out, err
		}
		out.Objectives = append(out.Objectives, objective)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	if err = rows.Close(); err != nil {
		return out, err
	}
	if len(out.Objectives) != len(in.ObjectiveIDs) {
		return out, ErrForbidden
	}
	sort.Slice(out.Objectives, func(i, j int) bool { return out.Objectives[i].ID < out.Objectives[j].ID })
	var duplicate bool
	sourceIdentity := canonicalHistoricalActivitySourceIdentity(in.SourceIdentity)
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.historical_assessment_activity_insertions h WHERE h.school_year_id=$1 AND h.semester_id=$2 AND h.class_id=$3 AND h.subject_id=$4 AND h.source_identity=$5)`, out.YearID, in.SemesterID, in.ClassID, in.SubjectID, sourceIdentity).Scan(&duplicate); err != nil {
		return out, err
	}
	if duplicate {
		return out, ErrConflict
	}
	dupeRows, err := tx.QueryContext(ctx, `SELECT id,name,activity_date::text FROM app.assessment_activities WHERE school_year_id=$1 AND semester_id=$2 AND class_id=$3 AND subject_id=$4 AND activity_type='sumatif' AND (lower(name)=lower($5) OR activity_date=$6::date) ORDER BY activity_date,id LIMIT 20`, out.YearID, in.SemesterID, in.ClassID, in.SubjectID, strings.TrimSpace(in.Name), in.ActivityDate)
	if err != nil {
		return out, err
	}
	for dupeRows.Next() {
		var candidate HistoricalAssessmentActivityDuplicate
		if err = dupeRows.Scan(&candidate.ID, &candidate.Name, &candidate.Date); err != nil {
			dupeRows.Close()
			return out, err
		}
		out.SimilarActivities = append(out.SimilarActivities, candidate)
	}
	if err = dupeRows.Err(); err != nil {
		dupeRows.Close()
		return out, err
	}
	if err = dupeRows.Close(); err != nil {
		return out, err
	}
	if len(out.SimilarActivities) > 0 && (!in.DuplicateChecked || strings.TrimSpace(in.DuplicateExplanation) == "") {
		return out, ValidationError("Ada kegiatan bernama atau bertanggal mirip. Periksa sumber dan jelaskan mengapa ini peristiwa berbeda.")
	}
	current, err := s.semesterRecommendationsTx(ctx, tx, actor, in.SemesterID, in.ClassID, in.SubjectID)
	if err != nil {
		return out, err
	}
	if err = s.overlaySemesterFinals(ctx, tx, &current); err != nil {
		return out, err
	}
	currentByStudent := make(map[int64]SemesterRecommendationStudent, len(current.Students))
	for _, st := range current.Students {
		id, parseErr := parsePositiveID(st.ID)
		if parseErr == nil {
			currentByStudent[id] = st
		}
	}
	out.Weights = current.Weights
	if err = tx.QueryRowContext(ctx, `SELECT count(DISTINCT student_id) FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, in.SemesterID, in.ClassID, in.SubjectID).Scan(&out.FinalCount); err != nil {
		return out, err
	}
	students, err := tx.QueryContext(ctx, `WITH candidates AS (
		SELECT p.student_id FROM app.student_placements p WHERE p.school_year_id=$1 AND (p.class_id=$2 OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.from_class_id=$2 OR m.to_class_id=$2)) OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=$2))
		UNION SELECT student_id FROM app.semester_final_versions WHERE semester_id=$3 AND class_id=$2 AND subject_id=$4
	) SELECT st.id,st.name,
		EXISTS(SELECT 1 FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=st.id AND p.starts_on IS NULL),
		(SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=st.id),
		(SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=st.id AND p.starts_on IS NOT NULL AND p.starts_on<=$5::date AND app.student_placement_class_at(p.id,$5::date)=$2),
		(SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=st.id AND p.starts_on IS NOT NULL AND p.starts_on<=$5::date AND app.student_placement_class_at(p.id,$5::date) IS NOT NULL),
		coalesce((SELECT fv.version FROM app.semester_final_versions fv WHERE fv.semester_id=$3 AND fv.class_id=$2 AND fv.subject_id=$4 AND fv.student_id=st.id ORDER BY fv.version DESC LIMIT 1),0),
		coalesce((SELECT fv.final_score::text FROM app.semester_final_versions fv WHERE fv.semester_id=$3 AND fv.class_id=$2 AND fv.subject_id=$4 AND fv.student_id=st.id ORDER BY fv.version DESC LIMIT 1),'')
	FROM candidates c JOIN app.students st ON st.id=c.student_id GROUP BY st.id,st.name ORDER BY st.name,st.id`, out.YearID, in.ClassID, in.SemesterID, in.SubjectID, in.ActivityDate)
	if err != nil {
		return out, err
	}
	var all []HistoricalAssessmentActivityStudent
	for students.Next() {
		var st HistoricalAssessmentActivityStudent
		var unknown bool
		var placements, member, anyPlaced int
		if err = students.Scan(&st.ID, &st.Name, &unknown, &placements, &member, &anyPlaced, &st.FinalVersion, &st.FinalValue); err != nil {
			students.Close()
			return out, err
		}
		st.Relevance = "Belum dapat diverifikasi"
		if placements > 0 && !unknown && anyPlaced <= 1 {
			if member > 0 {
				st.Relevance = "Kandidat anggota kelas"
			} else {
				st.Relevance = "Bukan anggota kelas pada tanggal kegiatan"
			}
		}
		if currentState, ok := currentByStudent[st.ID]; ok {
			st.NH, st.ASTS, st.ASAS, st.Status, st.Recommendation = currentState.NH, currentState.ASTS, currentState.ASAS, currentState.Status, currentState.Recommendation
		}
		st.After = "Kewajiban belum dapat diverifikasi."
		if st.Relevance == "Kandidat anggota kelas" {
			st.After = "Nilai kegiatan baru tetap kosong. Kewajiban Sumatif dapat membuat NH belum lengkap; nilai tidak dihitung otomatis."
			if out.Weights.Configured && out.Weights.NH == 0 {
				st.After = "Kegiatan tetap bagian dari struktur; NH berbobot 0% tidak menghalangi Final normal. Struktur Final lama dapat perlu ditinjau."
			}
		} else if st.Relevance == "Bukan anggota kelas pada tanggal kegiatan" {
			st.After = "Tidak otomatis menjadi kewajiban siswa karena bukan anggota pada tanggal kegiatan."
		}
		if st.FinalVersion > 0 {
			st.Status = "Final"
		}
		all = append(all, st)
	}
	if err = students.Err(); err != nil {
		students.Close()
		return out, err
	}
	if err = students.Close(); err != nil {
		return out, err
	}
	for _, st := range all {
		out.StudentCount++
		switch st.Relevance {
		case "Kandidat anggota kelas":
			out.MemberCount++
		case "Bukan anggota kelas pada tanggal kegiatan":
			out.NonMemberCount++
		default:
			out.UnknownCount++
		}
	}
	const pageSize = 10
	out.PageCount = (len(all) + pageSize - 1) / pageSize
	if out.PageCount == 0 {
		out.PageCount = 1
	}
	if page > out.PageCount {
		return out, ValidationError("Halaman pratinjau tidak tersedia.")
	}
	out.Page = page
	if page > 1 {
		out.PreviousPage = page - 1
	}
	if page < out.PageCount {
		out.NextPage = page + 1
	}
	start, end := (page-1)*pageSize, page*pageSize
	if start > len(all) {
		start = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	out.Students = all[start:end]
	out.NumericImpactKnown = false
	out.ImpactNotice = "Kegiatan dan relasi TP akan tercatat, tetapi nilai siswa tetap kosong. Dampak numerik NH tidak dapat dipastikan sebelum nilai dicatat melalui proses yang berwenang. Final lama tidak berubah otomatis; struktur Final dapat ditandai perlu ditinjau."
	if out.Weights.Configured && out.Weights.NH == 0 {
		out.ImpactNotice += " NH berbobot 0% tidak menjadi penghalang Final normal."
	}
	input := in
	input.ObjectiveIDs = append([]int64(nil), in.ObjectiveIDs...)
	sort.Slice(input.ObjectiveIDs, func(i, j int) bool { return input.ObjectiveIDs[i] < input.ObjectiveIDs[j] })
	source := struct {
		Input              HistoricalAssessmentActivityInput
		Context            LearningObjectiveContext
		Weights            AssessmentWeights
		Students           []HistoricalAssessmentActivityStudent
		Objectives         []HistoricalAssessmentActivityObjective
		Recap              AssessmentTPRecap
		TeacherMetadata    string
		AssignmentMetadata string
		CalendarMetadata   string
		Finals             []struct {
			StudentID    int64  `json:"student_id"`
			Version      int64  `json:"version"`
			SnapshotHash string `json:"snapshot_hash"`
			Snapshot     string `json:"snapshot"`
		}
	}{Input: input, Context: out.Context, Weights: out.Weights, Students: all, Objectives: out.Objectives, TeacherMetadata: teacherMetadata, AssignmentMetadata: assignmentMetadata, CalendarMetadata: calendarMetadata}
	source.Recap, err = s.assessmentTPRecap(ctx, tx, actor, in.SemesterID, in.ClassID, in.SubjectID, true)
	if err != nil {
		return out, err
	}
	finalRows, err := tx.QueryContext(ctx, `SELECT student_id,version,encode(snapshot_sha256,'hex'),snapshot::text FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 ORDER BY student_id,version`, in.SemesterID, in.ClassID, in.SubjectID)
	if err != nil {
		return out, err
	}
	for finalRows.Next() {
		var f struct {
			StudentID    int64  `json:"student_id"`
			Version      int64  `json:"version"`
			SnapshotHash string `json:"snapshot_hash"`
			Snapshot     string `json:"snapshot"`
		}
		if err = finalRows.Scan(&f.StudentID, &f.Version, &f.SnapshotHash, &f.Snapshot); err != nil {
			finalRows.Close()
			return out, err
		}
		source.Finals = append(source.Finals, f)
	}
	if err = finalRows.Err(); err != nil {
		finalRows.Close()
		return out, err
	}
	if err = finalRows.Close(); err != nil {
		return out, err
	}
	encoded, err := json.Marshal(struct {
		Source  any
		Similar []HistoricalAssessmentActivityDuplicate
	}{source, out.SimilarActivities})
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256(encoded)
	out.SnapshotToken = hex.EncodeToString(hash[:])
	if expected != "" && expected != out.SnapshotToken {
		return HistoricalAssessmentActivityPreview{}, ErrConflict
	}
	if page > 1 && expected == "" {
		return HistoricalAssessmentActivityPreview{}, ErrConflict
	}
	return out, nil
}

func (s *Store) ConfirmHistoricalAssessmentActivity(ctx context.Context, actor int64, req HistoricalAssessmentActivityConfirmation) (int64, int64, error) {
	if err := validateHistoricalAssessmentActivityInput(req.Input); err != nil {
		return 0, 0, err
	}
	if !validSubmissionUUID(req.SubmissionKey) {
		return 0, 0, ValidationError("Identitas permintaan tidak valid.")
	}
	token, err := hex.DecodeString(req.SnapshotToken)
	if err != nil || len(token) != sha256.Size {
		return 0, 0, ValidationError("Pratinjau tidak valid.")
	}
	request, err := json.Marshal(req)
	if err != nil {
		return 0, 0, err
	}
	requestHash := sha256.Sum256(request)
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(742009)`); err != nil {
		return 0, 0, err
	}
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return 0, 0, err
	}
	var oldHash []byte
	var oldID, oldActivity int64
	err = tx.QueryRowContext(ctx, `SELECT id,activity_id,request_sha256 FROM app.historical_assessment_activity_insertions WHERE actor_id=$1 AND teacher_id=$2 AND submission_key=$3::uuid`, actor, teacher, req.SubmissionKey).Scan(&oldID, &oldActivity, &oldHash)
	if err == nil {
		if !equalHash(oldHash, requestHash[:]) {
			return 0, 0, ErrConflict
		}
		var semesterID, classID, subjectID int64
		if err = tx.QueryRowContext(ctx, `SELECT semester_id,class_id,subject_id FROM app.historical_assessment_activity_insertions WHERE id=$1 AND actor_id=$2 AND teacher_id=$3`, oldID, actor, teacher).Scan(&semesterID, &classID, &subjectID); err != nil {
			return 0, 0, ErrForbidden
		}
		allowed, assignmentErr := learningObjectiveAssignment(ctx, tx, teacher, semesterID, classID, subjectID)
		if assignmentErr != nil {
			return 0, 0, assignmentErr
		}
		if !allowed {
			return 0, 0, ErrForbidden
		}
		return oldID, oldActivity, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	p := req.Input
	preview, err := s.previewHistoricalAssessmentActivityTx(ctx, tx, actor, p, 1, req.SnapshotToken)
	if err != nil {
		return 0, 0, err
	}
	allStudents := append([]HistoricalAssessmentActivityStudent(nil), preview.Students...)
	for page := 2; page <= preview.PageCount; page++ {
		part, pageErr := s.previewHistoricalAssessmentActivityTx(ctx, tx, actor, p, page, req.SnapshotToken)
		if pageErr != nil {
			return 0, 0, pageErr
		}
		allStudents = append(allStudents, part.Students...)
	}
	impact, err := json.Marshal(struct {
		Preview  HistoricalAssessmentActivityPreview   `json:"preview"`
		Students []HistoricalAssessmentActivityStudent `json:"all_students"`
	}{preview, allStudents})
	if err != nil {
		return 0, 0, err
	}
	evidence, err := json.Marshal(struct{ SourceIdentity, Document, Archive, Teacher, Section, ResultSource, Method, OutsideCalendarReason, DuplicateExplanation string }{p.SourceIdentity, p.SourceDocument, p.SourceArchive, p.SourceTeacher, p.SourceSection, p.ResultSource, p.Method, p.OutsideCalendarReason, p.DuplicateExplanation})
	if err != nil {
		return 0, 0, err
	}
	objectivesJSON, err := json.Marshal(preview.Objectives)
	if err != nil {
		return 0, 0, err
	}
	var activityID, auditID int64
	if err = tx.QueryRowContext(ctx, `SELECT nextval(pg_get_serial_sequence('app.assessment_activities','id'))`).Scan(&activityID); err != nil {
		return 0, 0, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO app.historical_assessment_activity_insertions(activity_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,reason,source_identity,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8::date,$9,$10,$11::jsonb,jsonb_build_object('acknowledged',true,'objectives',$12::jsonb),$13::jsonb,$14,$15,$16::uuid) RETURNING id`, activityID, preview.YearID, p.SemesterID, p.ClassID, p.SubjectID, teacher, actor, p.ActivityDate, strings.TrimSpace(p.Reason), canonicalHistoricalActivitySourceIdentity(p.SourceIdentity), string(evidence), string(objectivesJSON), string(impact), token, requestHash[:], req.SubmissionKey).Scan(&auditID)
	if err != nil {
		return 0, 0, dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.historical_assessment_activity_objectives(audit_id,learning_objective_id) SELECT $1,unnest($2::bigint[])`, auditID, p.ObjectiveIDs); err != nil {
		return 0, 0, dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.assessment_activities(id,school_year_id,semester_id,class_id,subject_id,teacher_id,name,activity_type,activity_date,created_by) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,$7,'sumatif',$8::date,$9)`, activityID, preview.YearID, p.SemesterID, p.ClassID, p.SubjectID, teacher, strings.TrimSpace(p.Name), p.ActivityDate, actor); err != nil {
		return 0, 0, dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.assessment_activity_objectives(activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id) SELECT $1,lo.id,lo.school_year_id,lo.semester_id,lo.class_id,lo.subject_id FROM app.learning_objectives lo WHERE lo.id=ANY($2::bigint[]) ORDER BY lo.id`, activityID, p.ObjectiveIDs); err != nil {
		return 0, 0, dbError(err)
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, dbError(err)
	}
	return auditID, activityID, nil
}

// HistoricalAssessmentActivityResult membatasi hasil audit kepada pembuatnya
// dan penugasan aktif pada konteks yang tercatat.
func (s *Store) HistoricalAssessmentActivityResult(ctx context.Context, actor, auditID int64) (HistoricalAssessmentActivityResult, error) {
	var out HistoricalAssessmentActivityResult
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
	err = tx.QueryRowContext(ctx, `SELECT h.activity_id,h.impact_snapshot FROM app.historical_assessment_activity_insertions h WHERE h.id=$1 AND h.actor_id=$2 AND h.teacher_id=$3`, auditID, actor, teacher).Scan(&out.ActivityID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrForbidden
	}
	if err != nil {
		return out, err
	}
	var stored struct {
		Preview  HistoricalAssessmentActivityPreview   `json:"preview"`
		Students []HistoricalAssessmentActivityStudent `json:"all_students"`
	}
	if err = json.Unmarshal(raw, &stored); err != nil {
		return out, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, stored.Preview.Input.SemesterID, stored.Preview.Input.ClassID, stored.Preview.Input.SubjectID)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, ErrForbidden
	}
	stored.Preview.Students = stored.Students
	out.AuditID, out.Preview = auditID, stored.Preview
	return out, nil
}

func parsePositiveID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
