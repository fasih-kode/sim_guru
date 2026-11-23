package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// SemesterFinalVersion menyajikan histori tanpa mengubah snapshot yang tersimpan.
type SemesterFinalVersion struct {
	Version                                        int64
	Value, Type, Recommendation, Reason, Actor, At string
	Student                                        SemesterRecommendationStudent
	Weights                                        AssessmentWeights
}
type SemesterFinalRetention struct {
	Version           int64
	Reason, Actor, At string
}
type OwnedSemesterFinal struct {
	SemesterID, ClassID, SubjectID, StudentID         int64
	SemesterName, ClassName, SubjectName, StudentName string
}

func semesterFinalOwned(ctx context.Context, tx *sql.Tx, teacher, semester, class, subject, student int64) (bool, error) {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT coalesce((SELECT teacher_id=$1 FROM app.semester_final_versions WHERE semester_id=$2 AND class_id=$3 AND subject_id=$4 AND student_id=$5 ORDER BY version DESC LIMIT 1),false)`, teacher, semester, class, subject, student).Scan(&allowed)
	return allowed, err
}

// SemesterFinalReviewResult memeriksa bukti tindakan milik guru untuk notifikasi.
func (s *Store) SemesterFinalReviewResult(ctx context.Context, actor, semester, class, subject, student int64, action, key string) (bool, error) {
	if !validSubmissionUUID(key) || (action != "revision" && action != "retain") {
		return false, nil
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return false, err
	}
	owned, err := semesterFinalOwned(ctx, tx, teacher, semester, class, subject, student)
	if err != nil {
		return false, err
	}
	if !owned {
		return false, ErrForbidden
	}
	var found bool
	if action == "revision" {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4 AND teacher_id=$5 AND recorded_by=$6 AND submission_key=$7::uuid AND version>1)`, semester, class, subject, student, teacher, actor, key).Scan(&found)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_retentions r JOIN app.semester_final_versions v ON v.id=r.final_version_id WHERE v.semester_id=$1 AND v.class_id=$2 AND v.subject_id=$3 AND v.student_id=$4 AND v.teacher_id=$5 AND r.recorded_by=$6 AND r.submission_key=$7::uuid)`, semester, class, subject, student, teacher, actor, key).Scan(&found)
	}
	return found, err
}

func semesterFinalVersions(ctx context.Context, tx *sql.Tx, semester, class, subject, student int64) ([]SemesterFinalVersion, error) {
	rows, err := tx.QueryContext(ctx, `SELECT v.version,v.final_score::text,v.final_type,coalesce(v.recommendation_score::text,''),v.reason,u.display_name,to_char(v.recorded_at AT TIME ZONE 'Asia/Jakarta','YYYY-MM-DD HH24:MI:SS'),v.snapshot FROM app.semester_final_versions v JOIN app.users u ON u.id=v.recorded_by WHERE v.semester_id=$1 AND v.class_id=$2 AND v.subject_id=$3 AND v.student_id=$4 ORDER BY v.version DESC`, semester, class, subject, student)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SemesterFinalVersion
	for rows.Next() {
		var v SemesterFinalVersion
		var raw []byte
		if err = rows.Scan(&v.Version, &v.Value, &v.Type, &v.Recommendation, &v.Reason, &v.Actor, &v.At, &raw); err != nil {
			return nil, err
		}
		var snapshot semesterFinalSnapshot
		if err = json.Unmarshal(raw, &snapshot); err != nil {
			return nil, err
		}
		v.Student = snapshot.Student
		var evidence struct {
			Weights struct{ NH, ASTS, ASAS int } `json:"weights"`
		}
		if err = json.Unmarshal(snapshot.Evidence, &evidence); err != nil {
			return nil, err
		}
		v.Weights = AssessmentWeights{NH: evidence.Weights.NH, ASTS: evidence.Weights.ASTS, ASAS: evidence.Weights.ASAS}
		out = append(out, v)
	}
	return out, rows.Err()
}

func semesterFinalRetentions(ctx context.Context, tx *sql.Tx, semester, class, subject, student int64) ([]SemesterFinalRetention, error) {
	rows, err := tx.QueryContext(ctx, `SELECT v.version,r.reason,u.display_name,to_char(r.recorded_at AT TIME ZONE 'Asia/Jakarta','YYYY-MM-DD HH24:MI:SS') FROM app.semester_final_retentions r JOIN app.semester_final_versions v ON v.id=r.final_version_id JOIN app.users u ON u.id=r.recorded_by WHERE v.semester_id=$1 AND v.class_id=$2 AND v.subject_id=$3 AND v.student_id=$4 ORDER BY r.id DESC`, semester, class, subject, student)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SemesterFinalRetention
	for rows.Next() {
		var v SemesterFinalRetention
		if err = rows.Scan(&v.Version, &v.Reason, &v.Actor, &v.At); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// OwnedSemesterFinals tidak memberikan akses pada siswa lain dalam kelas yang pernah diampu.
func (s *Store) OwnedSemesterFinals(ctx context.Context, actor int64) ([]OwnedSemesterFinal, error) {
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT v.semester_id,v.class_id,v.subject_id,v.student_id,y.name||' · Semester '||sm.number::text,c.grade::text||c.name,s.name,st.name FROM (SELECT DISTINCT ON (semester_id,class_id,subject_id,student_id) * FROM app.semester_final_versions ORDER BY semester_id,class_id,subject_id,student_id,version DESC) v JOIN app.semesters sm ON sm.id=v.semester_id JOIN app.school_years y ON y.id=sm.school_year_id JOIN app.classes c ON c.id=v.class_id JOIN app.subjects s ON s.id=v.subject_id JOIN app.students st ON st.id=v.student_id WHERE v.teacher_id=$1 ORDER BY y.name DESC,c.name,st.name`, teacher)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OwnedSemesterFinal
	for rows.Next() {
		var v OwnedSemesterFinal
		if err = rows.Scan(&v.SemesterID, &v.ClassID, &v.SubjectID, &v.StudentID, &v.SemesterName, &v.ClassName, &v.SubjectName, &v.StudentName); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ReviewSemesterFinal merekam versi baru, atau keputusan mempertahankan versi lama.
func (s *Store) ReviewSemesterFinal(ctx context.Context, actor, semester, class, subject, student, expected int64, score int, reason, token, key, action string) error {
	if semester < 1 || class < 1 || subject < 1 || student < 1 || expected < 1 || score < 0 || score > 100 {
		return ValidationError("Konteks atau nilai Final tidak valid.")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || !utf8ValidReason(reason) {
		return ValidationError("Alasan wajib diisi dan maksimal 500 karakter.")
	}
	hash, err := hex.DecodeString(token)
	if err != nil || len(hash) != 32 || !validSubmissionUUID(key) || (action != "revision" && action != "retain") {
		return ValidationError("Pratinjau atau tindakan tidak valid.")
	}
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	allowed, err := semesterFinalOwned(ctx, tx, teacher, semester, class, subject, student)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	payload, _ := json.Marshal(struct {
		Action                                      string
		Semester, Class, Subject, Student, Expected int64
		Score                                       int
		Reason, Token                               string
	}{action, semester, class, subject, student, expected, score, reason, token})
	requestHash := sha256.Sum256(payload)
	var existing []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM app.semester_final_versions WHERE recorded_by=$1 AND submission_key=$2::uuid UNION ALL SELECT request_sha256 FROM app.semester_final_retentions WHERE recorded_by=$1 AND submission_key=$2::uuid`, actor, key).Scan(&existing)
	if err == nil {
		if !equalHash(existing, requestHash[:]) {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var version, finalID int64
	var finalScore int
	var savedSnapshot, savedHash []byte
	if err = tx.QueryRowContext(ctx, `SELECT id,version,final_score,snapshot,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4 ORDER BY version DESC LIMIT 1 FOR UPDATE`, semester, class, subject, student).Scan(&finalID, &version, &finalScore, &savedSnapshot, &savedHash); err != nil {
		return err
	}
	if version != expected {
		return ErrConflict
	}
	result, err := s.semesterRecommendationsStudentTx(ctx, tx, actor, semester, class, subject, student)
	if err != nil {
		return err
	}
	if len(result.Students) != 1 {
		return ErrForbidden
	}
	item := result.Students[0]
	snapshot, currentHash, err := semesterFinalInputSnapshot(ctx, tx, semester, class, subject, item)
	if err != nil {
		return err
	}
	if !equalHash(hash, currentHash[:]) {
		return ErrConflict
	}
	_, uncertain, err := semesterFinalNeedsReview(ctx, tx, semester, class, subject, fmtID(student), savedSnapshot, savedHash, currentHash[:])
	if err != nil {
		return err
	}
	if uncertain {
		return ValidationError("Bukti Final historis belum dapat diverifikasi; perubahan belum tersedia.")
	}
	if action == "retain" {
		if score != finalScore {
			return ValidationError("Untuk mempertahankan Final, gunakan nilai Final yang tersimpan.")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO app.semester_final_retentions(final_version_id,recorded_by,reason,evidence_snapshot,evidence_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4::jsonb,$5,$6::uuid,$7)`, finalID, actor, reason, string(snapshot), currentHash[:], key, requestHash[:])
		if err != nil {
			return dbError(err)
		}
		return tx.Commit()
	}
	var recommendation any
	finalType := "normal"
	if item.Status == "Siap Final" && result.Weights.Configured {
		n, e := strconv.Atoi(item.Recommendation)
		if e != nil {
			return e
		}
		recommendation = n
	} else if item.ExceptionEligible {
		finalType = "exception"
	} else {
		return ValidationError("Bukti Sumatif sah belum mencukupi untuk versi Final baru; nilai lama dapat dipertahankan dengan alasan.")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,final_type,recommendation_score,final_score,reason,snapshot,snapshot_sha256,submission_key,request_sha256) SELECT sm.school_year_id,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14::uuid,$15 FROM app.semesters sm WHERE sm.id=$1`, semester, class, subject, student, teacher, actor, version+1, finalType, recommendation, score, reason, string(snapshot), currentHash[:], key, requestHash[:])
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}
