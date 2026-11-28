package academic

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ExamInitialInput struct {
	StudentID              int64
	Component, Date, Value string
}
type ExamInitialCell struct {
	Component, Date, Value, CorrectedValue string
	TeacherName, RecordedBy, RecordedAt    string
	AttemptedValue, AttemptedDate          string
	Recorded, Entered                      bool
	CanCorrect                             bool
	ScoreID                                int64
}
type ExamInitialStudent struct {
	ID         int64
	Name, NIS  string
	ASTS, ASAS ExamInitialCell
}
type ExamInitialSheet struct {
	Students              []ExamInitialStudent
	UnknownPlacementCount int
}

// ExamInitialScores menampilkan roster berdasarkan tanggal yang dipilih dan bukti kelas asal.
func (s *Store) ExamInitialScores(ctx context.Context, actor, semester, class, subject int64, date string) (ExamInitialSheet, error) {
	var out ExamInitialSheet
	if _, err := civilDate(date); err != nil {
		return out, err
	}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	ok, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, ErrForbidden
	}
	var year int64
	if err = tx.QueryRowContext(ctx, `SELECT school_year_id FROM app.semesters WHERE id=$1`, semester).Scan(&year); errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.student_placements p WHERE p.school_year_id=$1 AND p.starts_on IS NULL AND app.student_placement_class_at(p.id,$3::date)=$2`, year, class, date).Scan(&out.UnknownPlacementCount); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `WITH pupils AS (
 SELECT p.student_id FROM app.student_placements p WHERE p.school_year_id=$1 AND p.starts_on IS NOT NULL AND p.starts_on<=$4::date AND app.student_placement_class_at(p.id,$4::date)=$2
 UNION SELECT student_id FROM app.exam_initial_scores WHERE semester_id=$5 AND class_id=$2 AND subject_id=$3)
 SELECT s.id,s.name,coalesce(s.nis,''), e.component,e.exam_date::text,e.score::text,coalesce(latest.after_score,e.score)::text,e.id,e.teacher_id,e.recorded_by,t.name,u.username,to_char(e.recorded_at AT TIME ZONE 'Asia/Jakarta','DD-MM-YYYY HH24:MI')
 FROM pupils p JOIN app.students s ON s.id=p.student_id LEFT JOIN app.exam_initial_scores e ON e.student_id=s.id AND e.semester_id=$5 AND e.class_id=$2 AND e.subject_id=$3 LEFT JOIN LATERAL (SELECT c.after_score FROM app.exam_score_corrections c WHERE c.initial_score_id=e.id ORDER BY c.version DESC LIMIT 1) latest ON true LEFT JOIN app.teachers t ON t.id=e.teacher_id LEFT JOIN app.users u ON u.id=e.recorded_by ORDER BY s.name,s.id,e.component`, year, class, subject, date, semester)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	index := map[int64]int{}
	for rows.Next() {
		var id int64
		var name, nis string
		var component, examDate, value, corrected, teacherName, recordedBy, recordedAt sql.NullString
		var scoreID, ownerID, actorID sql.NullInt64
		if err = rows.Scan(&id, &name, &nis, &component, &examDate, &value, &corrected, &scoreID, &ownerID, &actorID, &teacherName, &recordedBy, &recordedAt); err != nil {
			return out, err
		}
		i, exists := index[id]
		if !exists {
			i = len(out.Students)
			index[id] = i
			out.Students = append(out.Students, ExamInitialStudent{ID: id, Name: name, NIS: nis})
		}
		if component.Valid {
			cell := ExamInitialCell{Component: component.String, Date: examDate.String, Value: strings.ReplaceAll(value.String, ".", ","), CorrectedValue: strings.ReplaceAll(corrected.String, ".", ","), ScoreID: scoreID.Int64, CanCorrect: ownerID.Int64 == teacher && actorID.Int64 == actor, Recorded: true, TeacherName: teacherName.String, RecordedBy: recordedBy.String, RecordedAt: recordedAt.String}
			if component.String == "asts" {
				out.Students[i].ASTS = cell
			} else {
				out.Students[i].ASAS = cell
			}
		}
	}
	return out, rows.Err()
}

// SaveExamInitialScores memeriksa seluruh input dan menyimpan satu batch secara atomik.
func (s *Store) SaveExamInitialScores(ctx context.Context, actor, semester, class, subject int64, inputs []ExamInitialInput) error {
	type parsed struct {
		student         int64
		component, date string
		cents           int64
	}
	values := make([]parsed, 0, len(inputs))
	seen := map[string]bool{}
	for _, in := range inputs {
		if in.StudentID < 1 || (in.Component != "asts" && in.Component != "asas") {
			return ValidationError("Siswa atau komponen ujian tidak valid.")
		}
		key := strconv.FormatInt(in.StudentID, 10) + ":" + in.Component
		if seen[key] {
			return ValidationError("Nilai siswa dikirim lebih dari sekali.")
		}
		seen[key] = true
		cents, present, err := parseInitialScore(in.Value)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		d, err := time.Parse("2006-01-02", in.Date)
		if err != nil || d.Format("2006-01-02") != in.Date {
			return ValidationError("Tanggal pelaksanaan tidak valid.")
		}
		values = append(values, parsed{in.StudentID, in.Component, in.Date, cents})
	}
	if len(values) == 0 {
		return ValidationError("Isi sedikitnya satu nilai sebelum menyimpan.")
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].student != values[j].student {
			return values[i].student < values[j].student
		}
		return values[i].component < values[j].component
	})
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	ok, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	var year int64
	if err = tx.QueryRowContext(ctx, `SELECT school_year_id FROM app.semesters WHERE id=$1`, semester).Scan(&year); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	for _, v := range values {
		var placement int64
		var n int
		err = tx.QueryRowContext(ctx, `SELECT coalesce(min(id),0),count(*) FROM app.student_placements WHERE school_year_id=$1 AND student_id=$2 AND starts_on IS NOT NULL AND starts_on<=$4::date AND app.student_placement_class_at(id,$4::date)=$3 AND $4::date <= (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date`, year, v.student, class, v.date).Scan(&placement, &n)
		if err != nil {
			return err
		}
		if n != 1 {
			return ValidationError("Tanggal pelaksanaan atau penempatan siswa tidak sah.")
		}
		result, e := tx.ExecContext(ctx, `INSERT INTO app.exam_initial_scores(school_year_id,semester_id,class_id,subject_id,student_id,placement_id,teacher_id,component,exam_date,score,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::date,$10::numeric,$11) ON CONFLICT (semester_id,class_id,subject_id,student_id,component) DO NOTHING`, year, semester, class, subject, v.student, placement, teacher, v.component, v.date, formatScore(v.cents), actor)
		if e != nil {
			return dbError(e)
		}
		count, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if count == 0 {
			var oldCents, owner int64
			var oldDate string
			err = tx.QueryRowContext(ctx, `SELECT (score*100)::bigint,teacher_id,exam_date::text FROM app.exam_initial_scores WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4 AND component=$5`, semester, class, subject, v.student, v.component).Scan(&oldCents, &owner, &oldDate)
			if err != nil {
				return err
			}
			if oldCents != v.cents || owner != teacher || oldDate != v.date {
				return ErrConflict
			}
		}
	}
	return tx.Commit()
}
