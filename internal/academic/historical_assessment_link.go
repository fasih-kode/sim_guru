package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// HistoricalAssessmentLinkInput menjelaskan satu koreksi dan pemeriksaan bukti.
type HistoricalAssessmentLinkInput struct {
	SemesterID, ClassID, SubjectID, ObjectiveID, ActivityID  int64
	Reason, Document, Archive, Section, Method, ResultSource string
	DateExplanation, DateEvidence                            string
	Acknowledged                                             bool
}

type HistoricalAssessmentLinkPreview struct {
	Input                                             HistoricalAssessmentLinkInput
	Impact                                            HistoricalTPPreview
	Objective, Activity, ActivityDate, HistoricalDate string
	YearID, ProvenanceID, TeacherID                   int64
	SnapshotToken                                     string
	ImpactNotice                                      string
	Students                                          []HistoricalAssessmentLinkStudent
}

type HistoricalAssessmentLinkStudent struct {
	Current HistoricalTPPreviewStudent
	After   string
}

type HistoricalAssessmentLinkConfirmation struct {
	Input                        HistoricalAssessmentLinkInput
	SnapshotToken, SubmissionKey string
}

type HistoricalAssessmentLinkChoice struct {
	ID    int64
	Label string
}

// HistoricalAssessmentLinkChoices hanya menampilkan sumber pada penugasan pemilik.
func (s *Store) HistoricalAssessmentLinkChoices(ctx context.Context, actor, semester, class, subject int64) ([]HistoricalAssessmentLinkChoice, []HistoricalAssessmentLinkChoice, error) {
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
	read := func(q string, args ...any) ([]HistoricalAssessmentLinkChoice, error) {
		rows, e := tx.QueryContext(ctx, q, args...)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		var v []HistoricalAssessmentLinkChoice
		for rows.Next() {
			var x HistoricalAssessmentLinkChoice
			if e = rows.Scan(&x.ID, &x.Label); e != nil {
				return nil, e
			}
			v = append(v, x)
		}
		return v, rows.Err()
	}
	tp, err := read(`SELECT o.id,o.definition FROM app.learning_objectives o JOIN app.historical_learning_objective_insertions h ON h.learning_objective_id=o.id WHERE o.semester_id=$1 AND o.class_id=$2 AND o.subject_id=$3 ORDER BY o.id`, semester, class, subject)
	if err != nil {
		return nil, nil, err
	}
	activities, err := read(`SELECT id,name||' · '||activity_date::text FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND teacher_id=$4 AND activity_type='sumatif' ORDER BY activity_date,id`, semester, class, subject, teacher)
	return tp, activities, err
}

func (s *Store) HistoricalAssessmentLinkResult(ctx context.Context, actor, id int64) (HistoricalAssessmentLinkPreview, error) {
	var result HistoricalAssessmentLinkPreview
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return result, err
	}
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT h.impact_snapshot FROM app.historical_assessment_objective_links h JOIN app.assessment_activities a ON a.id=h.activity_id WHERE h.id=$1 AND h.actor_id=$2 AND h.teacher_id=$3 AND a.teacher_id=$3`, id, actor, teacher).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrForbidden
	}
	if err != nil {
		return result, err
	}
	var data struct {
		Preview HistoricalAssessmentLinkPreview `json:"preview"`
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		return result, err
	}
	ok, err := learningObjectiveAssignment(ctx, tx, teacher, data.Preview.Input.SemesterID, data.Preview.Input.ClassID, data.Preview.Input.SubjectID)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrForbidden
	}
	return data.Preview, nil
}

func validateHistoricalAssessmentLink(in HistoricalAssessmentLinkInput) error {
	if in.SemesterID < 1 || in.ClassID < 1 || in.SubjectID < 1 || in.ObjectiveID < 1 || in.ActivityID < 1 {
		return ValidationError("Pilih konteks, TP historis dan kegiatan Sumatif.")
	}
	for _, v := range []string{in.Reason, in.Document, in.Archive, in.Section, in.Method, in.ResultSource} {
		if strings.TrimSpace(v) == "" || !utf8.ValidString(v) || utf8.RuneCountInString(v) > 500 || strings.ContainsRune(v, 0) {
			return ValidationError("Alasan dan rincian pemeriksaan sumber wajib diisi, maksimal 500 karakter.")
		}
	}
	for _, v := range []string{in.DateExplanation, in.DateEvidence} {
		if !utf8.ValidString(v) || utf8.RuneCountInString(v) > 500 || strings.ContainsRune(v, 0) {
			return ValidationError("Penjelasan tanggal tidak valid.")
		}
	}
	if !in.Acknowledged {
		return ValidationError("Pemeriksaan bukti wajib dikonfirmasi.")
	}
	return nil
}

func (s *Store) PreviewHistoricalAssessmentLink(ctx context.Context, actor int64, in HistoricalAssessmentLinkInput, page int, expected string) (HistoricalAssessmentLinkPreview, error) {
	if err := validateHistoricalAssessmentLink(in); err != nil {
		return HistoricalAssessmentLinkPreview{}, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return HistoricalAssessmentLinkPreview{}, err
	}
	defer tx.Rollback()
	return s.previewHistoricalAssessmentLinkTx(ctx, tx, actor, in, page, expected)
}

func (s *Store) previewHistoricalAssessmentLinkTx(ctx context.Context, tx *sql.Tx, actor int64, in HistoricalAssessmentLinkInput, page int, expected string) (HistoricalAssessmentLinkPreview, error) {
	var out HistoricalAssessmentLinkPreview
	if err := validateHistoricalAssessmentLink(in); err != nil {
		return out, err
	}
	if page < 1 {
		return out, ValidationError("Halaman tidak valid.")
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
	var source string
	err = tx.QueryRowContext(ctx, `SELECT o.school_year_id,h.id,o.definition,a.name,a.activity_date::text,h.assessment_date::text,jsonb_build_object('objective',to_jsonb(o),'provenance',to_jsonb(h),'activity',to_jsonb(a),'teacher',to_jsonb(t))::text
	 FROM app.learning_objectives o JOIN app.historical_learning_objective_insertions h ON h.learning_objective_id=o.id
	 JOIN app.assessment_activities a ON a.id=$2 AND a.school_year_id=o.school_year_id AND a.semester_id=o.semester_id AND a.class_id=o.class_id AND a.subject_id=o.subject_id
	 JOIN app.teachers t ON t.id=a.teacher_id
	 WHERE o.id=$1 AND o.semester_id=$3 AND o.class_id=$4 AND o.subject_id=$5 AND a.teacher_id=$6 AND a.activity_type='sumatif'`, in.ObjectiveID, in.ActivityID, in.SemesterID, in.ClassID, in.SubjectID, teacher).Scan(&out.YearID, &out.ProvenanceID, &out.Objective, &out.Activity, &out.ActivityDate, &out.HistoricalDate, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrForbidden
	}
	if err != nil {
		return out, err
	}
	var future bool
	if err = tx.QueryRowContext(ctx, `SELECT $1::date > (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date`, out.ActivityDate).Scan(&future); err != nil {
		return out, err
	}
	if future {
		return out, ValidationError("Tanggal kegiatan tidak boleh di masa depan.")
	}
	if out.ActivityDate != out.HistoricalDate && (strings.TrimSpace(in.DateExplanation) == "" || strings.TrimSpace(in.DateEvidence) == "") {
		return out, ValidationError("Perbedaan tanggal memerlukan penjelasan dan referensi bukti.")
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.assessment_activity_objectives WHERE activity_id=$1 AND learning_objective_id=$2)`, in.ActivityID, in.ObjectiveID).Scan(&exists); err != nil {
		return out, err
	}
	if exists {
		return out, ErrConflict
	}
	base := HistoricalTPPreviewInput{SemesterID: in.SemesterID, ClassID: in.ClassID, SubjectID: in.SubjectID, Definition: out.Objective, Reason: in.Reason, AssessmentDate: out.ActivityDate, DateSource: in.Document, Learning: HistoricalTPReference{Kind: "manual", Document: in.Document, Archive: in.Archive, Teacher: "Pemilik kegiatan", Date: out.HistoricalDate}, Assessment: HistoricalTPReference{Kind: "app", SourceID: fmt.Sprint(in.ActivityID)}}
	// Kumpulan seluruh siswa disertakan sebelum pagination agar fingerprint stabil.
	first, err := s.previewHistoricalTPTx(ctx, tx, actor, base, 1, "")
	if err != nil {
		return out, err
	}
	pageResult := first
	if page > 1 {
		pageResult, err = s.previewHistoricalTPTx(ctx, tx, actor, base, page, first.SnapshotToken)
		if err != nil {
			return out, err
		}
	}
	b, err := json.Marshal(struct {
		Input        HistoricalAssessmentLinkInput
		Source, Base string
	}{in, source, first.SnapshotToken})
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256(b)
	out.SnapshotToken = hex.EncodeToString(hash[:])
	if expected != "" && expected != out.SnapshotToken {
		return HistoricalAssessmentLinkPreview{}, ErrConflict
	}
	if page > 1 && expected == "" {
		return HistoricalAssessmentLinkPreview{}, ErrConflict
	}
	out.Input = in
	out.Impact = pageResult
	out.TeacherID = teacher
	out.ImpactNotice = "Nilai pasangan kegiatan–TP baru belum tercatat. Siswa yang mengikuti kegiatan menurut tanggal dan penempatannya dapat menjadi belum lengkap; dampak rekomendasi numerik belum dapat dihitung. NH berbobot 0% tidak menghalangi Final normal. Final tersimpan dapat memerlukan tinjauan struktur."
	for _, student := range pageResult.Students {
		after := "Kewajiban belum dapat diverifikasi."
		switch student.Relevance {
		case "Kandidat anggota kelas":
			after = "Nilai kegiatan pada TP ini kosong; kelengkapan NH dapat terpengaruh."
		case "Bukan anggota kelas pada tanggal penilaian":
			after = "Kegiatan ini tidak otomatis menjadi kewajiban siswa."
		}
		out.Students = append(out.Students, HistoricalAssessmentLinkStudent{Current: student, After: after})
	}
	return out, nil
}

func (s *Store) ConfirmHistoricalAssessmentLink(ctx context.Context, actor int64, req HistoricalAssessmentLinkConfirmation) (int64, error) {
	if err := validateHistoricalAssessmentLink(req.Input); err != nil {
		return 0, err
	}
	if !validSubmissionUUID(req.SubmissionKey) {
		return 0, ValidationError("Identitas permintaan tidak valid.")
	}
	token, err := hex.DecodeString(req.SnapshotToken)
	if err != nil || len(token) != sha256.Size {
		return 0, ValidationError("Pratinjau tidak valid.")
	}
	b, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	hash := sha256.Sum256(b)
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return 0, err
	}
	p := req.Input
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, p.SemesterID, p.ClassID, p.SubjectID)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, ErrForbidden
	}
	var oldHash []byte
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id,request_sha256 FROM app.historical_assessment_objective_links WHERE actor_id=$1 AND teacher_id=$2 AND submission_key=$3::uuid`, actor, teacher, req.SubmissionKey).Scan(&id, &oldHash)
	if err == nil {
		var owns bool
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.historical_assessment_objective_links h JOIN app.assessment_activities a ON a.id=h.activity_id WHERE h.id=$1 AND a.teacher_id=$2 AND a.semester_id=$3 AND a.class_id=$4 AND a.subject_id=$5)`, id, teacher, p.SemesterID, p.ClassID, p.SubjectID).Scan(&owns); e != nil {
			return 0, e
		}
		if !owns {
			return 0, ErrForbidden
		}
		if !equalHash(oldHash, hash[:]) {
			return 0, ErrConflict
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	preview, err := s.previewHistoricalAssessmentLinkTx(ctx, tx, actor, p, 1, req.SnapshotToken)
	if err != nil {
		return 0, err
	}
	students := append([]HistoricalTPPreviewStudent(nil), preview.Impact.Students...)
	for page := 2; page <= preview.Impact.PageCount; page++ {
		part, pageErr := s.previewHistoricalAssessmentLinkTx(ctx, tx, actor, p, page, req.SnapshotToken)
		if pageErr != nil {
			return 0, pageErr
		}
		students = append(students, part.Impact.Students...)
	}
	impact, err := json.Marshal(struct {
		Preview  HistoricalAssessmentLinkPreview `json:"preview"`
		Students []HistoricalTPPreviewStudent    `json:"all_students"`
	}{preview, students})
	if err != nil {
		return 0, err
	}
	evidence, err := json.Marshal(struct{ Document, Archive, Section, DateExplanation, DateEvidence string }{p.Document, p.Archive, p.Section, p.DateExplanation, p.DateEvidence})
	if err != nil {
		return 0, err
	}
	verification, err := json.Marshal(struct {
		Method, ResultSource string
		Acknowledged         bool
	}{p.Method, p.ResultSource, p.Acknowledged})
	if err != nil {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO app.historical_assessment_objective_links(activity_id,learning_objective_id,provenance_id,school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,activity_date,historical_date,reason,evidence,verification,impact_snapshot,preview_sha256,request_sha256,submission_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::date,$11::date,$12,$13::jsonb,$14::jsonb,$15::jsonb,$16,$17,$18::uuid) RETURNING id`, p.ActivityID, p.ObjectiveID, preview.ProvenanceID, preview.YearID, p.SemesterID, p.ClassID, p.SubjectID, teacher, actor, preview.ActivityDate, preview.HistoricalDate, strings.TrimSpace(p.Reason), string(evidence), string(verification), string(impact), token, hash[:], req.SubmissionKey).Scan(&id)
	if err != nil {
		return 0, dbError(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.assessment_activity_objectives(activity_id,learning_objective_id,school_year_id,semester_id,class_id,subject_id) VALUES($1,$2,$3,$4,$5,$6)`, p.ActivityID, p.ObjectiveID, preview.YearID, p.SemesterID, p.ClassID, p.SubjectID)
	if err != nil {
		return 0, dbError(err)
	}
	if err = tx.Commit(); err != nil {
		return 0, dbError(err)
	}
	return id, nil
}
