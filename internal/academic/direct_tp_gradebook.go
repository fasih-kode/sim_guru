package academic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type DirectTPBook struct {
	Objectives []DirectTPBookObjective
	Students   []DirectTPBookStudent
}
type DirectTPBookObjective struct {
	ID               int64
	Code, Definition string
}
type DirectTPBookStudent struct {
	ID    int64
	Name  string
	Final bool
	Cells []DirectTPBookCell
}
type DirectTPBookCell struct {
	StudentID, ObjectiveID, Version int64
	ApplicabilityVersion            int64
	Code, Value                     string
	Editable                        bool
	CanSetApplicability             bool
	Note                            string
}

// DirectTPGradebook mengambil seluruh tabel dari satu snapshot tanpa memakai nilai kegiatan lama.
func (s *Store) DirectTPGradebook(ctx context.Context, actor, semester, class, subject int64) (DirectTPBook, error) {
	out := DirectTPBook{Objectives: []DirectTPBookObjective{}, Students: []DirectTPBookStudent{}}
	if semester < 1 || class < 1 || subject < 1 {
		return out, ValidationError("Pilih konteks penugasan.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		return out, err
	}
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, ErrForbidden
	}
	var year int64
	err = tx.QueryRowContext(ctx, `SELECT sm.school_year_id FROM app.semesters sm JOIN app.classes cl ON cl.id=$2 AND cl.school_year_id=sm.school_year_id JOIN app.semester_calendars cal ON cal.semester_id=sm.id WHERE sm.id=$1`, semester, class).Scan(&year)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ValidationError("Kalender semester belum tersedia.")
	}
	if err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT o.id,c.ordinal,o.definition FROM app.learning_objectives o JOIN app.direct_tp_codes c ON c.objective_id=o.id WHERE (o.school_year_id,o.semester_id,o.class_id,o.subject_id)=($1,$2,$3,$4) ORDER BY c.ordinal`, year, semester, class, subject)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var o DirectTPBookObjective
		var ordinal int64
		if err = rows.Scan(&o.ID, &ordinal, &o.Definition); err != nil {
			rows.Close()
			return out, err
		}
		o.Code = fmt.Sprintf("TP%d", ordinal)
		out.Objectives = append(out.Objectives, o)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT p.student_id,st.name,
 EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE (f.semester_id,f.class_id,f.subject_id,f.student_id)=($1,$3,$4,p.student_id)),
 COALESCE(p.starts_on IS NOT NULL AND p.starts_on<=cal.starts_on AND app.student_placement_class_at(p.id,cal.starts_on)=$3 AND app.student_placement_class_at(p.id,LEAST(cal.ends_on,(clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date))=$3 AND NOT EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND m.effective_date BETWEEN cal.starts_on AND cal.ends_on) AND NOT EXISTS(SELECT 1 FROM app.student_school_exits x WHERE x.student_id=p.student_id AND x.effective_date BETWEEN cal.starts_on AND cal.ends_on) AND NOT EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.effective_date BETWEEN cal.starts_on AND cal.ends_on) AND st.active,false),p.id
 FROM app.student_placements p JOIN app.students st ON st.id=p.student_id JOIN app.semester_calendars cal ON cal.semester_id=$1
 WHERE p.school_year_id=$2 AND (p.class_id=$3 OR EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.from_class_id=$3 OR m.to_class_id=$3)) OR EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.target_class_id=$3)) ORDER BY st.name,p.student_id`, semester, year, class, subject)
	if err != nil {
		return out, err
	}
	type roster struct {
		student   DirectTPBookStudent
		valid     bool
		placement int64
	}
	var rosterRows []roster
	for rows.Next() {
		var r roster
		if err = rows.Scan(&r.student.ID, &r.student.Name, &r.student.Final, &r.valid, &r.placement); err != nil {
			rows.Close()
			return out, err
		}
		rosterRows = append(rosterRows, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	type cellKey struct{ student, objective int64 }
	type storedCell struct {
		owner, version int64
		value          string
	}
	stored := make(map[cellKey]storedCell)
	rows, err = tx.QueryContext(ctx, `SELECT s.student_id,s.objective_id,s.owner_teacher_id,coalesce(c.version,0),coalesce(c.after_score,s.score)::text FROM app.direct_tp_initial_scores s LEFT JOIN LATERAL (SELECT version,after_score FROM app.direct_tp_score_corrections WHERE initial_score_id=s.id ORDER BY version DESC LIMIT 1) c ON true WHERE (s.semester_id,s.class_id,s.subject_id)=($1,$2,$3)`, semester, class, subject)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var key cellKey
		var value storedCell
		if err = rows.Scan(&key.student, &key.objective, &value.owner, &value.version, &value.value); err != nil {
			rows.Close()
			return out, err
		}
		stored[key] = value
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	applicabilities, err := loadDirectTPApplicabilities(ctx, tx, semester, class, subject, 0, 0)
	if err != nil {
		return out, err
	}
	for _, r := range rosterRows {
		for _, o := range out.Objectives {
			resolution := applicabilities[directTPKey{r.student.ID, o.ID}]
			state, applicabilityVersion := resolution.state, resolution.version
			cell := DirectTPBookCell{StudentID: r.student.ID, ObjectiveID: o.ID, Version: -1, ApplicabilityVersion: applicabilityVersion, Code: o.Code, Editable: state == directTPApplicable && !r.student.Final, CanSetApplicability: (!r.valid || state == directTPUnknown || applicabilityVersion > 0) && !r.student.Final}
			if r.student.Final {
				cell.Note = "Siswa sudah Final"
			} else if state == directTPNotApplicable {
				cell.Note = "TP tidak berlaku"
			} else if state == directTPUnknown {
				cell.Note = "Penempatan perlu diverifikasi"
			}
			key := cellKey{r.student.ID, o.ID}
			if value, exists := stored[key]; exists {
				cell.Version, cell.Value = value.version, value.value
				if value.owner != teacher {
					cell.Editable = false
					cell.CanSetApplicability = false
					cell.Note = "Nilai milik guru lain"
				}
			}
			r.student.Cells = append(r.student.Cells, cell)
		}
		out.Students = append(out.Students, r.student)
	}
	return out, nil
}

// ValidateDirectTPValue memakai validasi angka yang sama dengan service simpan.
func ValidateDirectTPValue(value string) error { _, _, err := parseInitialScore(value); return err }

// DirectTPSubmissionSaved memeriksa bahwa pesan keberhasilan merujuk batch aktor dan konteks yang benar.
func (s *Store) DirectTPSubmissionSaved(ctx context.Context, actor, semester, class, subject int64, key string) (bool, error) {
	if !validSubmissionUUID(key) {
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
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil || !allowed {
		return false, err
	}
	var found bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.direct_tp_submissions WHERE actor_id=$1 AND teacher_id=$2 AND (semester_id,class_id,subject_id)=($3,$4,$5) AND submission_key=$6::uuid)`, actor, teacher, semester, class, subject, key).Scan(&found)
	return found, err
}
