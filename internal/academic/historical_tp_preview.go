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
	"time"
	"unicode/utf8"
)

// HistoricalTPReference menjelaskan lokasi bukti; kelengkapan referensi tidak
// membuktikan isi arsip atau hubungannya dengan TP secara otomatis.
type HistoricalTPReference struct {
	Kind, SourceID, Document, Archive, Teacher, ResultSource, Date string
}

// HistoricalTPPreviewInput hanya digunakan untuk pratinjau, bukan penyimpanan.
type HistoricalTPPreviewInput struct {
	SemesterID, ClassID, SubjectID                 int64
	Definition, Reason, AssessmentDate, DateSource string
	Learning, Assessment                           HistoricalTPReference
}

// validateHistoricalTPPreviewInput membatasi data sebelum pembacaan bukti.
func validateHistoricalTPPreviewInput(in HistoricalTPPreviewInput) error {
	if in.SemesterID < 1 || in.ClassID < 1 || in.SubjectID < 1 {
		return ValidationError("Pilih konteks semester, kelas, dan mata pelajaran.")
	}
	validText := func(s string, max int) bool {
		return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max
	}
	if !validText(in.Definition, 4000) || !validText(in.Reason, 500) {
		return ValidationError("Uraian TP dan alasan wajib diisi dalam batas yang tersedia.")
	}
	if in.AssessmentDate != "" {
		d, err := time.Parse("2006-01-02", in.AssessmentDate)
		if err != nil || d.Format("2006-01-02") != in.AssessmentDate {
			return ValidationError("Tanggal penilaian tidak valid.")
		}
	}
	if !utf8.ValidString(in.DateSource) || utf8.RuneCountInString(in.DateSource) > 500 {
		return ValidationError("Asal tanggal tidak valid.")
	}
	for _, ref := range []HistoricalTPReference{in.Learning, in.Assessment} {
		if ref.Kind != "app" && ref.Kind != "manual" {
			return ValidationError("Pilih jenis sumber bukti pembelajaran dan Sumatif.")
		}
		for _, field := range []string{ref.SourceID, ref.Document, ref.Archive, ref.Teacher, ref.ResultSource} {
			if !utf8.ValidString(field) || utf8.RuneCountInString(field) > 500 {
				return ValidationError("Referensi bukti melebihi batas atau tidak valid.")
			}
		}
		if ref.Date != "" {
			d, err := time.Parse("2006-01-02", ref.Date)
			if err != nil || d.Format("2006-01-02") != ref.Date {
				return ValidationError("Tanggal sumber bukti tidak valid.")
			}
		}
	}
	return nil
}

type HistoricalTPPreviewStudent struct {
	ID                                                      int64
	Name, Relevance, NH, Status, Recommendation, FinalValue string
	FinalVersion                                            int64
}

type historicalTPImpactStudent struct {
	StudentID    int64  `json:"student_id"`
	Relevance    string `json:"relevance"`
	FinalID      int64  `json:"final_id,omitempty"`
	FinalVersion int64  `json:"final_version,omitempty"`
}

type HistoricalTPPreview struct {
	Input                                                   HistoricalTPPreviewInput
	Context                                                 LearningObjectiveContext
	LearningStatus, AssessmentStatus, DateNotice            string
	Weights                                                 AssessmentWeights
	Students                                                []HistoricalTPPreviewStudent
	FinalCount, Page, PageCount, PreviousPage, NextPage     int
	StudentCount, MemberCount, NonMemberCount, UnknownCount int
	SnapshotToken                                           string
	displayHash                                             [32]byte
	impact                                                  []byte
}

// PreviewHistoricalTP membaca seluruh data yang digunakan dalam satu snapshot.
// Tidak ada penyimpanan keputusan atau perubahan akademik pada jalur ini.
func (s *Store) PreviewHistoricalTP(ctx context.Context, actor int64, in HistoricalTPPreviewInput, page int, expectedToken string) (HistoricalTPPreview, error) {
	var out HistoricalTPPreview
	if err := validateHistoricalTPPreviewInput(in); err != nil {
		return out, err
	}
	if page < 1 {
		return out, ValidationError("Halaman tidak valid.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	return s.previewHistoricalTPTx(ctx, tx, actor, in, page, expectedToken)
}

func (s *Store) previewHistoricalTPTx(ctx context.Context, tx *sql.Tx, actor int64, in HistoricalTPPreviewInput, page int, expectedToken string) (HistoricalTPPreview, error) {
	var out HistoricalTPPreview
	if err := validateHistoricalTPPreviewInput(in); err != nil {
		return out, err
	}
	if page < 1 {
		return out, ValidationError("Halaman tidak valid.")
	}
	var err error
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
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
	out.Input = in
	out.Context.SemesterID, out.Context.ClassID, out.Context.SubjectID = in.SemesterID, in.ClassID, in.SubjectID
	var yearID int64
	err = tx.QueryRowContext(ctx, `SELECT sm.school_year_id,y.name||' · Semester '||sm.number::text,c.grade::text||c.name,s.name
	 FROM app.semesters sm JOIN app.school_years y ON y.id=sm.school_year_id
	 JOIN app.classes c ON c.id=$2 AND c.school_year_id=sm.school_year_id
	 JOIN app.subjects s ON s.id=$3 AND s.active WHERE sm.id=$1`, in.SemesterID, in.ClassID, in.SubjectID).
		Scan(&yearID, &out.Context.SemesterName, &out.Context.ClassName, &out.Context.SubjectName)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrForbidden
	}
	if err != nil {
		return out, err
	}
	var hasFinal bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3)`, in.SemesterID, in.ClassID, in.SubjectID).Scan(&hasFinal); err != nil {
		return out, err
	}
	if !hasFinal {
		return out, ValidationError("Pratinjau ini hanya tersedia untuk konteks yang sudah memiliki Final.")
	}
	var learningRaw, assessmentRaw string
	out.LearningStatus, learningRaw, err = historicalTPReferenceStatus(ctx, tx, actor, in, true)
	if err != nil {
		return out, err
	}
	out.AssessmentStatus, assessmentRaw, err = historicalTPReferenceStatus(ctx, tx, actor, in, false)
	if err != nil {
		return out, err
	}
	dateKnown := in.AssessmentDate != "" && strings.TrimSpace(in.DateSource) != "" && out.AssessmentStatus != "Referensi tidak memadai"
	if !dateKnown {
		out.DateNotice = "Tanggal, asal tanggal, atau referensi penilaian belum dapat diverifikasi."
	} else {
		out.DateNotice = "Simulasi bersyarat berdasarkan tanggal yang dinyatakan; bukan penetapan kewajiban."
	}
	// Rekomendasi memakai rumus existing; nilai berikut adalah keadaan saat ini.
	rec, err := s.semesterRecommendationsTx(ctx, tx, actor, in.SemesterID, in.ClassID, in.SubjectID)
	if err != nil {
		return out, err
	}
	out.Weights = rec.Weights
	current := make(map[string]SemesterRecommendationStudent, len(rec.Students))
	for _, st := range rec.Students {
		current[st.ID] = st
	}
	rows, err := tx.QueryContext(ctx, `WITH candidates AS (
	 SELECT DISTINCT p.student_id FROM app.student_placements p WHERE p.school_year_id=$1 AND
	 (p.class_id=$2 OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.from_class_id=$2 OR m.to_class_id=$2))
	 OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=$2))
	 UNION SELECT student_id FROM app.semester_final_versions WHERE semester_id=$3 AND class_id=$2 AND subject_id=$4
	 ) SELECT st.id,st.name,coalesce(f.final_score::text,''),coalesce(f.id,0),coalesce(f.version,0),
	 coalesce((SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=st.id AND p.starts_on IS NULL),0),
	 coalesce((SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=st.id),0),
	 coalesce((SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=st.id AND p.starts_on IS NOT NULL AND p.starts_on<=nullif($5,'')::date AND app.student_placement_class_at(p.id,nullif($5,'')::date)=$2),0),
	 coalesce((SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$1 AND p.student_id=st.id AND p.starts_on IS NOT NULL AND p.starts_on<=nullif($5,'')::date AND app.student_placement_class_at(p.id,nullif($5,'')::date) IS NOT NULL),0)
	 FROM candidates c JOIN app.students st ON st.id=c.student_id LEFT JOIN LATERAL
	 (SELECT id,final_score,version FROM app.semester_final_versions WHERE semester_id=$3 AND class_id=$2 AND subject_id=$4 AND student_id=st.id ORDER BY version DESC LIMIT 1) f ON true
	 ORDER BY st.name,st.id LIMIT 251`, yearID, in.ClassID, in.SemesterID, in.SubjectID, in.AssessmentDate)
	if err != nil {
		return out, err
	}
	impact := make([]historicalTPImpactStudent, 0)
	for rows.Next() {
		var st HistoricalTPPreviewStudent
		var finalID int64
		var unknown, placements, member, placed int
		if err = rows.Scan(&st.ID, &st.Name, &st.FinalValue, &finalID, &st.FinalVersion, &unknown, &placements, &member, &placed); err != nil {
			rows.Close()
			return out, err
		}
		st.Relevance = "Belum dapat diverifikasi"
		relevance := "unverified"
		if dateKnown && placements > 0 && unknown == 0 && placed <= 1 {
			if member == 1 {
				st.Relevance = "Kandidat anggota kelas"
				relevance = "candidate_member"
			} else {
				st.Relevance = "Bukan anggota kelas pada tanggal penilaian"
				relevance = "not_member"
			}
		}
		impact = append(impact, historicalTPImpactStudent{st.ID, relevance, finalID, st.FinalVersion})
		if st.FinalVersion > 0 {
			out.FinalCount++
		}
		out.StudentCount++
		switch st.Relevance {
		case "Kandidat anggota kelas":
			out.MemberCount++
		case "Bukan anggota kelas pada tanggal penilaian":
			out.NonMemberCount++
		default:
			out.UnknownCount++
		}
		if v, ok := current[strconv.FormatInt(st.ID, 10)]; ok {
			st.NH, st.Status, st.Recommendation = v.NH, v.Status, v.Recommendation
		}
		if st.FinalVersion > 0 {
			st.Status = "Final"
		}
		out.Students = append(out.Students, st)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	if len(out.Students) > 250 {
		return HistoricalTPPreview{}, ValidationError("Pratinjau dibatasi hingga 250 siswa terkait; hubungi pengelola aplikasi.")
	}
	// Sertakan sumber perubahan yang tidak selalu tampak pada hasil, terutama
	// pergerakan, penempatan, assignment dan bukti Final dalam fingerprint.
	var state string
	err = tx.QueryRowContext(ctx, `SELECT jsonb_build_object(
	 'assignments',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY a.id),'[]') FROM app.teaching_assignments a WHERE a.semester_id=$1 AND a.class_id=$2 AND a.subject_id=$3),
	 'placements',(SELECT coalesce(jsonb_agg(jsonb_build_object('placement',to_jsonb(p),'moves',(SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY m.id),'[]') FROM app.student_class_moves m WHERE m.placement_id=p.id),'returns',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY r.id),'[]') FROM app.student_school_returns r WHERE r.placement_id=p.id),'exits',(SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.id),'[]') FROM app.student_school_exits e WHERE e.student_id=p.student_id)) ORDER BY p.id),'[]') FROM app.student_placements p WHERE p.school_year_id=$4 AND (p.class_id=$2 OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.from_class_id=$2 OR m.to_class_id=$2)) OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=$2) OR EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=$1 AND f.class_id=$2 AND f.subject_id=$3 AND f.student_id=p.student_id))),
	 'finals',(SELECT coalesce(jsonb_agg(to_jsonb(f) ORDER BY f.id),'[]') FROM app.semester_final_versions f WHERE f.semester_id=$1 AND f.class_id=$2 AND f.subject_id=$3),
	 'objectives',(SELECT coalesce(jsonb_agg(to_jsonb(o) ORDER BY o.id),'[]') FROM app.learning_objectives o WHERE o.semester_id=$1 AND o.class_id=$2 AND o.subject_id=$3),
	 'weights',(SELECT to_jsonb(w) FROM app.semester_assessment_weights w WHERE w.semester_id=$1))::text`, in.SemesterID, in.ClassID, in.SubjectID, yearID).Scan(&state)
	if err != nil {
		return out, err
	}
	// Hash bukti seluruh siswa agar perubahan nilai atau histori yang tidak
	// mengubah angka tampilan tetap membatalkan pagination lama.
	evidenceHashes := make([]string, 0, len(out.Students))
	for _, st := range out.Students {
		_, hash, e := semesterFinalInputSnapshot(ctx, tx, in.SemesterID, in.ClassID, in.SubjectID, SemesterRecommendationStudent{ID: strconv.FormatInt(st.ID, 10)})
		if e != nil {
			return out, e
		}
		evidenceHashes = append(evidenceHashes, hex.EncodeToString(hash[:]))
	}
	data, err := json.Marshal(struct {
		Input                       HistoricalTPPreviewInput
		Learning, Assessment, State string
		Recommendation              SemesterRecommendation
		Students                    []HistoricalTPPreviewStudent
		EvidenceHashes              []string
	}{in, learningRaw, assessmentRaw, state, rec, out.Students, evidenceHashes})
	if err != nil {
		return out, err
	}
	out.displayHash = sha256.Sum256(data)
	out.impact, err = json.Marshal(impact)
	if err != nil {
		return out, err
	}
	// Ikat proyeksi audit ke fingerprint yang sama sebelum hasil dipaginasi.
	impactHash := sha256.Sum256(out.impact)
	proofHash := sha256.Sum256(append(out.displayHash[:], impactHash[:]...))
	input, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	var proof []byte
	if err = tx.QueryRowContext(ctx, `SELECT academic_write.historical_tp_proof($1,$2::jsonb,$3)`, actor, string(input), proofHash[:]).Scan(&proof); err != nil {
		return out, academicWriterError(err)
	}
	out.SnapshotToken = hex.EncodeToString(proof)
	if page > 1 && (expectedToken == "" || expectedToken != out.SnapshotToken) {
		return HistoricalTPPreview{}, ErrConflict
	}
	if page == 1 && expectedToken != "" && expectedToken != out.SnapshotToken {
		return HistoricalTPPreview{}, ErrConflict
	}
	out.PageCount = (len(out.Students) + 9) / 10
	if out.PageCount == 0 {
		out.PageCount = 1
	}
	if page > out.PageCount {
		return HistoricalTPPreview{}, ValidationError("Halaman tidak tersedia.")
	}
	out.Page = page
	out.PreviousPage = page - 1
	out.NextPage = page + 1
	start := (page - 1) * 10
	end := start + 10
	if end > len(out.Students) {
		end = len(out.Students)
	}
	out.Students = out.Students[start:end]
	return out, nil
}

func historicalTPReferenceStatus(ctx context.Context, tx *sql.Tx, actor int64, in HistoricalTPPreviewInput, learning bool) (string, string, error) {
	ref := in.Assessment
	if learning {
		ref = in.Learning
	}
	if ref.Kind == "manual" {
		if strings.TrimSpace(ref.Document) == "" || strings.TrimSpace(ref.Archive) == "" || strings.TrimSpace(ref.Teacher) == "" || (learning && ref.Date == "") || (!learning && (strings.TrimSpace(ref.ResultSource) == "" || in.AssessmentDate == "")) {
			return "Referensi tidak memadai", "", nil
		}
		return "Perlu Verifikasi Manual", "", nil
	}
	id, err := strconv.ParseInt(strings.TrimSpace(ref.SourceID), 10, 64)
	if err != nil || id < 1 {
		return "Referensi tidak memadai", "", nil
	}
	input, err := json.Marshal(in)
	if err != nil {
		return "", "", err
	}
	var raw, date, teacherName string
	err = tx.QueryRowContext(ctx, `SELECT event_date,teacher_name,fingerprint
		FROM academic_write.historical_tp_metadata($1,$2::jsonb,$3)`, actor, string(input), learning).Scan(&date, &teacherName, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrForbidden
	}
	if err != nil {
		return "", "", academicWriterError(err)
	}
	claimedDate := in.AssessmentDate
	if learning {
		claimedDate = ref.Date
	}
	if (claimedDate != "" && claimedDate != date) || (strings.TrimSpace(ref.Teacher) != "" && !strings.EqualFold(strings.TrimSpace(ref.Teacher), strings.TrimSpace(teacherName))) {
		return "Referensi tidak memadai", raw, nil
	}
	return fmt.Sprintf("Metadata aplikasi cocok (ID %d, tanggal %s, guru %s); hubungan substansi TP dan sumber hasil belum diverifikasi.", id, date, teacherName), raw, nil
}
