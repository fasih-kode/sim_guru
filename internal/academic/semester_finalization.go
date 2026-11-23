package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SemesterFinalPreview mengikat tampilan konfirmasi pada snapshot masukan yang
// sama dengan hasil rekomendasi yang dibaca guru.
type SemesterFinalPreview struct {
	SemesterID, ClassID, SubjectID int64
	Student                        SemesterRecommendationStudent
	Weights                        AssessmentWeights
	SnapshotToken                  string
	CanFinalize                    bool
	CanFinalizeException           bool
	FinalType                      string
	Finalized                      bool
	FinalValue                     string
	FinalVersion                   int64
	NeedsReview                    bool
	ReviewUncertain                bool
	HistoricalStudent              SemesterRecommendationStudent
	HistoricalWeights              AssessmentWeights
	RecordedBy, RecordedAt, Reason string
	History                        []SemesterFinalVersion
	Retentions                     []SemesterFinalRetention
	HistoricalAccess               bool
}

type semesterFinalSnapshot struct {
	SemesterID, ClassID, SubjectID int64
	Student                        SemesterRecommendationStudent
	Evidence                       json.RawMessage
}

type semesterFinalEvidence struct {
	Semester           json.RawMessage `json:"semester"`
	Class              json.RawMessage `json:"class"`
	Subject            json.RawMessage `json:"subject"`
	Calendar           json.RawMessage `json:"calendar"`
	TeacherAssignments json.RawMessage `json:"teacher_assignments"`
	Weights            json.RawMessage `json:"weights"`
	Student            json.RawMessage `json:"student"`
	Placements         json.RawMessage `json:"placements"`
	PlacementEvents    json.RawMessage `json:"placement_events"`
	Objectives         json.RawMessage `json:"objectives"`
	Activities         json.RawMessage `json:"activities,omitempty"`
	DirectTPScores     json.RawMessage `json:"direct_tp_scores,omitempty"`
	TPApplicability    json.RawMessage `json:"tp_applicability,omitempty"`
	ExamSources        json.RawMessage `json:"exam_sources"`
}

const semesterFinalEvidenceQuery = `
SELECT jsonb_build_object(
 'semester',(SELECT to_jsonb(sm) FROM app.semesters sm WHERE sm.id=$1),
 'class',(SELECT to_jsonb(c) FROM app.classes c JOIN app.semesters sm ON sm.id=$1 AND c.school_year_id=sm.school_year_id WHERE c.id=$2),
 'subject',(SELECT to_jsonb(s) FROM app.subjects s WHERE s.id=$3),
 'calendar',(SELECT to_jsonb(cal) FROM app.semester_calendars cal WHERE cal.semester_id=$1),
 'teacher_assignments',coalesce((SELECT jsonb_agg(to_jsonb(ta) ORDER BY ta.id) FROM app.teaching_assignments ta WHERE ta.semester_id=$1 AND ta.class_id=$2 AND ta.subject_id=$3),'[]'::jsonb),
 'weights',(SELECT to_jsonb(w) FROM app.semester_assessment_weights w WHERE w.semester_id=$1),
 'student',(SELECT to_jsonb(st) FROM app.students st WHERE st.id=$4),
 'placements',coalesce((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id) FROM app.student_placements p JOIN app.semesters sm ON sm.id=$1 WHERE p.school_year_id=sm.school_year_id AND p.student_id=$4),'[]'::jsonb),
 'placement_events',coalesce((SELECT jsonb_agg(x.event ORDER BY x.event_date,x.event_id) FROM (
    SELECT jsonb_build_object('type','class_move','row',to_jsonb(m)) AS event,m.effective_date AS event_date,m.id AS event_id
      FROM app.student_class_moves m JOIN app.student_placements p ON p.id=m.placement_id JOIN app.semesters sm ON sm.id=$1
      WHERE p.school_year_id=sm.school_year_id AND p.student_id=$4
    UNION ALL
    SELECT jsonb_build_object('type','school_return','row',to_jsonb(r)),r.effective_date,r.id
      FROM app.student_school_returns r JOIN app.student_placements p ON p.id=r.placement_id JOIN app.semesters sm ON sm.id=$1
      WHERE p.school_year_id=sm.school_year_id AND p.student_id=$4
 ) x),'[]'::jsonb),
 'objectives',coalesce((SELECT jsonb_agg(jsonb_build_object(
      'objective',to_jsonb(lo),
      'criteria',coalesce((SELECT jsonb_agg(to_jsonb(tc) ORDER BY tc.id) FROM app.tp_achievement_criteria tc WHERE tc.semester_id=$1 AND tc.class_id=$2 AND tc.subject_id=$3 AND (tc.objective_id=lo.id OR tc.objective_id IS NULL)),'[]'::jsonb)
    ) ORDER BY lo.id)
    FROM app.learning_objectives lo WHERE lo.semester_id=$1 AND lo.class_id=$2 AND lo.subject_id=$3),'[]'::jsonb),
 'activities',coalesce((SELECT jsonb_agg(jsonb_build_object(
      'activity',to_jsonb(a),
      'objectives',coalesce((SELECT jsonb_agg(to_jsonb(ao) ORDER BY ao.learning_objective_id) FROM app.assessment_activity_objectives ao WHERE ao.activity_id=a.id),'[]'::jsonb),
      'scores',coalesce((SELECT jsonb_agg(jsonb_build_object(
          'initial',to_jsonb(sc),
          'corrections',coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.version,c.id) FROM app.assessment_activity_score_corrections c WHERE c.initial_score_id=sc.id),'[]'::jsonb),
          'remedials',coalesce((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.recorded_at,r.id) FROM app.assessment_activity_score_remedials r WHERE r.initial_score_id=sc.id),'[]'::jsonb)
        ) ORDER BY sc.learning_objective_id,sc.id) FROM app.assessment_activity_initial_scores sc WHERE sc.assessment_activity_id=a.id AND sc.student_id=$4),'[]'::jsonb)
    ) ORDER BY a.id)
    FROM app.assessment_activities a WHERE a.semester_id=$1 AND a.class_id=$2 AND a.subject_id=$3 AND a.activity_type='sumatif'),'[]'::jsonb),
 'tp_applicability',coalesce((SELECT jsonb_agg(to_jsonb(h) ORDER BY h.objective_id,h.version,h.id) FROM app.direct_tp_applicability_history h WHERE (h.semester_id,h.class_id,h.subject_id,h.student_id)=($1,$2,$3,$4)),'[]'::jsonb),
 'direct_tp_scores',coalesce((SELECT jsonb_agg(jsonb_build_object(
      'initial',to_jsonb(sc),
      'corrections',coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.version,c.id) FROM app.direct_tp_score_corrections c WHERE c.initial_score_id=sc.id),'[]'::jsonb)
    ) ORDER BY sc.objective_id,sc.id)
    FROM app.direct_tp_initial_scores sc WHERE (sc.semester_id,sc.class_id,sc.subject_id,sc.student_id)=($1,$2,$3,$4)),'[]'::jsonb),
 'exam_sources',coalesce((SELECT jsonb_agg(jsonb_build_object(
      'initial',to_jsonb(e),
      'corrections',coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.version,c.id) FROM app.exam_score_corrections c WHERE c.initial_score_id=e.id),'[]'::jsonb),
      'remedials',coalesce((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.recorded_at,r.id) FROM app.exam_score_remedials r WHERE r.initial_score_id=e.id),'[]'::jsonb)
    ) ORDER BY e.id)
    FROM app.exam_initial_scores e WHERE e.semester_id=$1 AND e.subject_id=$3 AND e.student_id=$4),'[]'::jsonb)
)::text`

func semesterFinalInputSnapshot(ctx context.Context, tx *sql.Tx, semesterID, classID, subjectID int64, student SemesterRecommendationStudent) ([]byte, [32]byte, error) {
	var raw string
	if err := tx.QueryRowContext(ctx, semesterFinalEvidenceQuery, semesterID, classID, subjectID, student.ID).Scan(&raw); err != nil {
		return nil, [32]byte{}, err
	}
	var evidence semesterFinalEvidence
	if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
		return nil, [32]byte{}, err
	}
	// Aktivitas lama tetap tersimpan pada snapshot historis; snapshot baru
	// tidak menjadikannya sumber NH atau pemicu peninjauan aktif.
	evidence.Activities = nil
	data, err := json.Marshal(semesterFinalSnapshot{SemesterID: semesterID, ClassID: classID, SubjectID: subjectID, Student: student, Evidence: mustJSON(evidence)})
	if err != nil {
		return nil, [32]byte{}, err
	}
	return data, sha256.Sum256(data), nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// SemesterFinalPreview membaca rekomendasi dan semua bukti dalam snapshot yang
// konsisten; pembacaan ulang ini juga menandai Final lama yang dasarnya berubah.
func (s *Store) SemesterFinalPreview(ctx context.Context, actor, semesterID, classID, subjectID, studentID int64) (SemesterFinalPreview, error) {
	return s.semesterFinalPreview(ctx, actor, semesterID, classID, subjectID, studentID, false)
}

// SemesterFinalHistoricalPreview hanya membaca siswa yang Final-nya dimiliki guru.
func (s *Store) SemesterFinalHistoricalPreview(ctx context.Context, actor, semesterID, classID, subjectID, studentID int64) (SemesterFinalPreview, error) {
	return s.semesterFinalPreview(ctx, actor, semesterID, classID, subjectID, studentID, true)
}

func (s *Store) semesterFinalPreview(ctx context.Context, actor, semesterID, classID, subjectID, studentID int64, historical bool) (SemesterFinalPreview, error) {
	var out SemesterFinalPreview
	if semesterID < 1 || classID < 1 || subjectID < 1 || studentID < 1 {
		return out, ValidationError("Pilih konteks siswa yang valid.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var result SemesterRecommendation
	if historical {
		result, err = s.semesterRecommendationsStudentTx(ctx, tx, actor, semesterID, classID, subjectID, studentID)
	} else {
		result, err = s.semesterRecommendationsTx(ctx, tx, actor, semesterID, classID, subjectID)
	}
	if err != nil {
		return out, err
	}
	var student *SemesterRecommendationStudent
	for i := range result.Students {
		if result.Students[i].ID == strconv.FormatInt(studentID, 10) {
			student = &result.Students[i]
			break
		}
	}
	if student == nil {
		return out, ErrNotFound
	}
	out = SemesterFinalPreview{SemesterID: semesterID, ClassID: classID, SubjectID: subjectID, Student: *student, Weights: result.Weights}
	out.HistoricalAccess = historical
	_, hash, err := semesterFinalInputSnapshot(ctx, tx, semesterID, classID, subjectID, *student)
	if err != nil {
		return out, err
	}
	out.SnapshotToken = hex.EncodeToString(hash[:])
	var value int
	var version int64
	var storedHash, storedSnapshot []byte
	err = tx.QueryRowContext(ctx, `SELECT v.final_score,v.version,v.snapshot_sha256,v.final_type,v.snapshot,v.reason,u.display_name,to_char(v.recorded_at AT TIME ZONE 'Asia/Jakarta','YYYY-MM-DD HH24:MI:SS')
		FROM app.semester_final_versions v JOIN app.users u ON u.id=v.recorded_by WHERE v.semester_id=$1 AND v.class_id=$2 AND v.subject_id=$3 AND v.student_id=$4
		ORDER BY v.version DESC LIMIT 1`, semesterID, classID, subjectID, studentID).Scan(&value, &version, &storedHash, &out.FinalType, &storedSnapshot, &out.Reason, &out.RecordedBy, &out.RecordedAt)
	if err == nil {
		var historic semesterFinalSnapshot
		if err = json.Unmarshal(storedSnapshot, &historic); err != nil {
			return out, err
		}
		out.HistoricalStudent = historic.Student
		var evidence struct {
			Weights struct{ NH, ASTS, ASAS int } `json:"weights"`
		}
		if err = json.Unmarshal(historic.Evidence, &evidence); err != nil {
			return out, err
		}
		out.HistoricalWeights = AssessmentWeights{NH: evidence.Weights.NH, ASTS: evidence.Weights.ASTS, ASAS: evidence.Weights.ASAS, Configured: true}
		out.Finalized = true
		out.FinalValue = strconv.Itoa(value)
		out.FinalVersion = version
		out.NeedsReview, out.ReviewUncertain, err = semesterFinalNeedsReview(ctx, tx, semesterID, classID, subjectID, student.ID, storedSnapshot, storedHash, hash[:])
		if err != nil {
			return out, err
		}
		out.Student.Status = "Final"
		out.Student.Finalized = true
		out.Student.FinalType = out.FinalType
		out.Student.FinalValue = out.FinalValue
		out.Student.FinalVersion = version
		out.Student.NeedsReview = out.NeedsReview
		out.Student.ReviewUncertain = out.ReviewUncertain
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	out.CanFinalize = !out.Finalized && result.Weights.Configured && student.Status == "Siap Final" && student.Recommendation != ""
	out.CanFinalizeException = !out.Finalized && student.ExceptionEligible
	if historical {
		out.History, err = semesterFinalVersions(ctx, tx, semesterID, classID, subjectID, studentID)
		if err != nil {
			return out, err
		}
		out.Retentions, err = semesterFinalRetentions(ctx, tx, semesterID, classID, subjectID, studentID)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func equalHash(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// semesterFinalNeedsReview membandingkan bukti sesuai format versi yang
// tersimpan. Snapshot lama tidak dihitung ulang memakai rumus NH baru.
func semesterFinalNeedsReview(ctx context.Context, tx *sql.Tx, semester, class, subject int64, student string, savedSnapshot, savedHash, currentHash []byte) (bool, bool, error) {
	var snapshot semesterFinalSnapshot
	if err := json.Unmarshal(savedSnapshot, &snapshot); err != nil {
		return false, true, nil
	}
	var old map[string]json.RawMessage
	if err := json.Unmarshal(snapshot.Evidence, &old); err != nil || old == nil {
		return false, true, nil
	}
	_, directFormat := old["direct_tp_scores"]
	if _, hasApplicability := old["tp_applicability"]; directFormat && hasApplicability {
		return !equalHash(savedHash, currentHash), false, nil
	}
	var raw string
	if err := tx.QueryRowContext(ctx, semesterFinalEvidenceQuery, semester, class, subject, student).Scan(&raw); err != nil {
		return false, true, err
	}
	var current map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return false, true, err
	}
	// Nilai langsung baru merupakan bukti baru yang tidak ada dalam format lama.
	var direct []json.RawMessage
	if err := json.Unmarshal(current["direct_tp_scores"], &direct); err != nil {
		return false, true, err
	}
	if !directFormat && len(direct) != 0 {
		return true, false, nil
	}
	var decisions []json.RawMessage
	if err := json.Unmarshal(current["tp_applicability"], &decisions); err != nil {
		return false, true, err
	}
	if len(decisions) != 0 {
		return true, false, nil
	}
	keys := []string{"semester", "class", "subject", "calendar", "teacher_assignments", "weights", "student", "placements", "placement_events", "objectives", "exam_sources"}
	if directFormat {
		keys = append(keys, "direct_tp_scores")
	} else {
		keys = append(keys, "activities")
	}
	for _, key := range keys {
		before, ok := old[key]
		if !ok {
			return false, true, nil
		}
		after, ok := current[key]
		if !ok {
			return false, true, nil
		}
		var same bool
		if err := tx.QueryRowContext(ctx, `SELECT $1::jsonb = $2::jsonb`, string(before), string(after)).Scan(&same); err != nil {
			return false, true, err
		}
		if !same {
			return true, false, nil
		}
	}
	return false, false, nil
}

func (s *Store) overlaySemesterFinals(ctx context.Context, tx *sql.Tx, result *SemesterRecommendation) error {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT ON (student_id) student_id,final_score,version,snapshot_sha256,final_type,snapshot
		FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3
		ORDER BY student_id,version DESC`, result.Context.SemesterID, result.Context.ClassID, result.Context.SubjectID)
	if err != nil {
		return err
	}
	type finalRow struct {
		studentID int64
		value     int
		version   int64
		hash      []byte
		snapshot  []byte
		finalType string
	}
	finals := []finalRow{}
	for rows.Next() {
		var f finalRow
		if err = rows.Scan(&f.studentID, &f.value, &f.version, &f.hash, &f.finalType, &f.snapshot); err != nil {
			rows.Close()
			return err
		}
		finals = append(finals, f)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, f := range finals {
		for i := range result.Students {
			student := &result.Students[i]
			if student.ID != strconv.FormatInt(f.studentID, 10) {
				continue
			}
			_, currentHash, e := semesterFinalInputSnapshot(ctx, tx, result.Context.SemesterID, result.Context.ClassID, result.Context.SubjectID, *student)
			if e != nil {
				return e
			}
			student.Finalized = true
			student.FinalType = f.finalType
			student.FinalValue = strconv.Itoa(f.value)
			student.FinalVersion = f.version
			student.NeedsReview, student.ReviewUncertain, e = semesterFinalNeedsReview(ctx, tx, result.Context.SemesterID, result.Context.ClassID, result.Context.SubjectID, student.ID, f.snapshot, f.hash, currentHash[:])
			if e != nil {
				return e
			}
			student.Status = "Final"
			break
		}
	}
	return nil
}

type finalizationPayload struct {
	SemesterID, ClassID, SubjectID, StudentID int64
	Score                                     int
	Reason, SnapshotToken                     string
}

// FinalizeSemesterNormal mengunci mutasi akademik yang memakai kunci transaksi
// bersama, menghitung ulang rekomendasi, lalu menyimpan Final dan snapshot atomik.
func (s *Store) FinalizeSemesterNormal(ctx context.Context, actor, semesterID, classID, subjectID, studentID int64, score int, reason, snapshotToken, submissionKey string) error {
	if semesterID < 1 || classID < 1 || subjectID < 1 || studentID < 1 || score < 0 || score > 100 {
		return ValidationError("Nilai Final harus berupa bilangan bulat 0–100.")
	}
	reason = strings.TrimSpace(reason)
	if !utf8ValidReason(reason) {
		return ValidationError("Alasan perubahan wajib diisi dan maksimal 500 karakter.")
	}
	if len(snapshotToken) != 64 {
		return ValidationError("Pratinjau tidak valid. Muat ulang sebelum menyimpan Final.")
	}
	providedHash, err := hex.DecodeString(snapshotToken)
	if err != nil || len(providedHash) != 32 {
		return ValidationError("Pratinjau tidak valid. Muat ulang sebelum menyimpan Final.")
	}
	if !validSubmissionUUID(submissionKey) {
		return ValidationError("Kunci pengiriman tidak valid. Muat ulang pratinjau.")
	}
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	payload := finalizationPayload{semesterID, classID, subjectID, studentID, score, reason, snapshotToken}
	requestBytes, _ := json.Marshal(payload)
	requestHash := sha256.Sum256(requestBytes)
	var oldHash []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM app.semester_final_versions WHERE recorded_by=$1 AND submission_key=$2::uuid`, actor, submissionKey).Scan(&oldHash)
	if err == nil {
		if !equalHash(oldHash, requestHash[:]) {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	teacherID, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacherID, semesterID, classID, subjectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	var assignmentID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.teaching_assignments WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND teacher_id=$4 AND effective_from<=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date AND (effective_until IS NULL OR effective_until>=(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date) ORDER BY id DESC LIMIT 1 FOR SHARE`, semesterID, classID, subjectID, teacherID).Scan(&assignmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	result, err := s.semesterRecommendationsTx(ctx, tx, actor, semesterID, classID, subjectID)
	if err != nil {
		return err
	}
	var student *SemesterRecommendationStudent
	for i := range result.Students {
		if result.Students[i].ID == strconv.FormatInt(studentID, 10) {
			student = &result.Students[i]
			break
		}
	}
	if student == nil {
		return ErrForbidden
	}
	// Periksa kesegaran lebih dahulu. Perubahan bukti bisa membuat siswa tidak
	// lagi siap; pratinjau lama harus selalu menghasilkan konflik, bukan validasi
	// seolah guru baru saja mengirim keadaan terkini yang belum lengkap.
	snapshot, snapshotHash, err := semesterFinalInputSnapshot(ctx, tx, semesterID, classID, subjectID, *student)
	if err != nil {
		return err
	}
	if !equalHash(providedHash, snapshotHash[:]) {
		return ErrConflict
	}
	if student.Status != "Siap Final" || student.Recommendation == "" || !result.Weights.Configured {
		return ValidationError("Final normal hanya dapat disimpan untuk siswa Siap Final.")
	}
	recommended, err := strconv.Atoi(student.Recommendation)
	if err != nil {
		return fmt.Errorf("rekomendasi Final tidak valid: %w", err)
	}
	if score != recommended && reason == "" {
		return ValidationError("Alasan wajib diisi bila Nilai Final berbeda dari rekomendasi.")
	}
	var alreadyFinal bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4)`, semesterID, classID, subjectID, studentID).Scan(&alreadyFinal); err != nil {
		return err
	}
	if alreadyFinal {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.semester_final_versions(
		school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,
		recommendation_score,final_score,reason,snapshot,snapshot_sha256,submission_key,request_sha256)
		SELECT sm.school_year_id,$1,$2,$3,$4,$5,$6,1,$7,$8,$9,$10::jsonb,$11,$12::uuid,$13
		FROM app.semesters sm WHERE sm.id=$1`, semesterID, classID, subjectID, studentID, teacherID, actor, recommended, score, reason, string(snapshot), snapshotHash[:], submissionKey, requestHash[:])
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}

func utf8ValidReason(reason string) bool {
	return utf8.ValidString(reason) && !strings.ContainsRune(reason, 0) && utf8.RuneCountInString(reason) <= 500
}

func validSubmissionUUID(v string) bool {
	if len(v) != 36 || v[8] != '-' || v[13] != '-' || v[18] != '-' || v[23] != '-' {
		return false
	}
	for i, r := range v {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func ensureRoutineStructureOpen(ctx context.Context, tx *sql.Tx, semesterID, classID, subjectID int64) error {
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM app.semester_final_versions
		WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3
	)`, semesterID, classID, subjectID).Scan(&locked); err != nil {
		return err
	}
	if locked {
		return ErrConflict
	}
	return nil
}
