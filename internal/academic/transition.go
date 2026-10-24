package academic

import (
	"context"
	"database/sql"
	"fmt"
)

type TransitionStudent struct {
	PlacementID, StudentID int64
	Name, NIS, NISN        string
	FromClass              string
	FromGrade              int
}
type TransitionDecision struct {
	StudentID, TargetClassID int64
	Outcome                  string
}
type TransitionPreview struct {
	SourceYear, TargetYear int64
	Students               []TransitionStudent
	Classes                []Class
	Decisions              []TransitionDecision
}

func (s *Store) TransitionPreview(ctx context.Context, actor, sourceYear, targetYear int64) (TransitionPreview, error) {
	var out TransitionPreview
	if sourceYear < 1 || targetYear < 1 || sourceYear == targetYear {
		return out, ValidationError("Pilih dua tahun ajaran yang berbeda.")
	}
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out, err = s.transitionPreviewTx(ctx, tx, sourceYear, targetYear)
	if err != nil {
		return out, err
	}
	for _, student := range out.Students {
		out.Decisions = append(out.Decisions, TransitionDecision{StudentID: student.StudentID, Outcome: ""})
	}
	return out, nil
}

func validateTransitionPlan(preview TransitionPreview, decisions []TransitionDecision) error {
	if len(preview.Students) == 0 {
		return ValidationError("Tidak ada siswa dengan penempatan pada tahun sumber.")
	}
	if len(decisions) != len(preview.Students) {
		return ValidationError("Tentukan hasil transisi untuk setiap siswa pada pratinjau.")
	}
	students := map[int64]TransitionStudent{}
	for _, s := range preview.Students {
		students[s.StudentID] = s
	}
	classes := map[int64]Class{}
	for _, c := range preview.Classes {
		classes[c.ID] = c
	}
	seen := map[int64]bool{}
	for _, d := range decisions {
		s, ok := students[d.StudentID]
		if !ok || seen[d.StudentID] {
			return ValidationError("Daftar siswa berubah atau memuat pilihan ganda.")
		}
		seen[d.StudentID] = true
		switch d.Outcome {
		case "promote", "repeat":
			if d.Outcome == "promote" && s.FromGrade == 9 {
				return ValidationError("Siswa kelas 9 harus dipilih lulus, tidak lulus — mengulang kelas 9, atau keluar.")
			}
			c, ok := classes[d.TargetClassID]
			if !ok {
				return ValidationError(fmt.Sprintf("Pilih kelas tujuan pada tahun ajaran baru untuk %s.", s.Name))
			}
			want := s.FromGrade
			if d.Outcome == "promote" {
				want++
			}
			if c.Grade != want {
				return ValidationError(fmt.Sprintf("Kelas tujuan %d%s tidak sesuai hasil transisi %s.", c.Grade, c.Name, d.Outcome))
			}
		case "graduate":
			if s.FromGrade != 9 || d.TargetClassID != 0 {
				return ValidationError("Hanya siswa kelas 9 yang dapat ditandai lulus; kelas tujuan harus kosong.")
			}
		case "leave":
			if d.TargetClassID != 0 {
				return ValidationError("Siswa keluar tidak memiliki kelas tujuan.")
			}
		default:
			return ValidationError(fmt.Sprintf("Pilih hasil transisi untuk %s sebelum menjalankan seluruh batch.", s.Name))
		}
	}
	return nil
}

func ValidateTransitionDecisions(preview TransitionPreview, decisions []TransitionDecision) error {
	return validateTransitionPlan(preview, decisions)
}

func (s *Store) ExecuteStudentTransition(ctx context.Context, actor int64, sourceYear, targetYear int64, decisions []TransitionDecision) error {
	if sourceYear < 1 || targetYear < 1 || sourceYear == targetYear {
		return ValidationError("Pilih dua tahun ajaran yang berbeda.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err := s.transitionPreviewTx(ctx, tx, sourceYear, targetYear)
	if err != nil {
		return err
	}
	if err = validateTransitionPlan(p, decisions); err != nil {
		return err
	}
	for _, d := range decisions {
		var placementID int64
		if err = tx.QueryRowContext(ctx, `SELECT id FROM app.student_placements WHERE school_year_id=$1 AND student_id=$2`, sourceYear, d.StudentID).Scan(&placementID); err != nil {
			return ErrConflict
		}
		var targetPlacement any
		var targetClass any
		if d.TargetClassID > 0 {
			var id int64
			err = tx.QueryRowContext(ctx, `INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id) VALUES($1,$2,$3,$4) RETURNING id`, targetYear, d.StudentID, d.TargetClassID, actor).Scan(&id)
			if err != nil {
				return dbError(err)
			}
			targetPlacement = id
			targetClass = d.TargetClassID
			if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_placement_history(placement_id,actor_id,from_class_id,to_class_id,reason) VALUES($1,$2,(SELECT class_id FROM app.current_student_placements WHERE id=$3),$4,$5)`, id, actor, placementID, d.TargetClassID, transitionReason(d.Outcome)); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_year_transitions(source_year_id,target_year_id,student_id,source_placement_id,outcome,target_class_id,target_placement_id,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, sourceYear, targetYear, d.StudentID, placementID, d.Outcome, targetClass, targetPlacement, actor); err != nil {
			return dbError(err)
		}
	}
	return tx.Commit()
}
func transitionReason(outcome string) string {
	switch outcome {
	case "promote":
		return "Transisi tahun ajaran: naik kelas"
	case "repeat":
		return "Transisi tahun ajaran: tidak naik kelas"
	case "graduate":
		return "Transisi tahun ajaran: lulus"
	default:
		return "Transisi tahun ajaran: keluar"
	}
}
func (s *Store) transitionPreviewTx(ctx context.Context, tx *sql.Tx, source, target int64) (TransitionPreview, error) {
	var p TransitionPreview
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT active FROM app.school_years WHERE id=$1`, source).Scan(&active); err != nil {
		return p, ErrNotFound
	}
	if !active {
		return p, ValidationError("Tahun sumber harus aktif.")
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM app.school_years WHERE id=$1 AND id<>$2 AND active=false`, target, source).Scan(&target); err != nil {
		return p, ValidationError("Pilih tahun ajaran tujuan yang belum aktif.")
	}
	rows, err := tx.QueryContext(ctx, `SELECT p.id,p.student_id,s.name,coalesce(s.nis,''),coalesce(s.nisn,''),c.grade,c.name FROM app.current_student_placements p JOIN app.students s ON s.id=p.student_id JOIN app.classes c ON c.id=p.class_id WHERE p.school_year_id=$1 AND NOT EXISTS(SELECT 1 FROM app.outstanding_student_school_exits e WHERE e.student_id=p.student_id) ORDER BY c.grade,c.name,s.name,s.id`, source)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var v TransitionStudent
		if err = rows.Scan(&v.PlacementID, &v.StudentID, &v.Name, &v.NIS, &v.NISN, &v.FromGrade, &v.FromClass); err != nil {
			return p, err
		}
		p.Students = append(p.Students, v)
	}
	if err = rows.Err(); err != nil {
		return p, err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.school_years WHERE id=$1)`, target).Scan(&exists); err != nil {
		return p, err
	}
	if !exists {
		return p, ErrNotFound
	}
	var pending bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_class_moves m JOIN app.student_placements p ON p.id=m.placement_id WHERE p.school_year_id=$1 AND m.effective_date > (now() AT TIME ZONE 'Asia/Jakarta')::date)`, source).Scan(&pending); err != nil {
		return p, err
	}
	if !pending {
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_school_returns r JOIN app.student_placements p ON p.id=r.placement_id WHERE p.school_year_id=$1 AND r.effective_date > (now() AT TIME ZONE 'Asia/Jakarta')::date)`, source).Scan(&pending); err != nil {
			return p, err
		}
	}
	if pending {
		return p, ValidationError("Masih ada perpindahan kelas atau penerimaan kembali terjadwal. Jalankan transisi setelah kejadian berlaku.")
	}
	var already bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_year_transitions WHERE source_year_id=$1)`, source).Scan(&already); err != nil {
		return p, err
	}
	if already {
		return p, ValidationError("Transisi dari tahun sumber ini sudah pernah dijalankan.")
	}
	var targetPlacements bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_placements source JOIN app.student_placements target ON target.student_id=source.student_id WHERE source.school_year_id=$1 AND target.school_year_id=$2)`, source, target).Scan(&targetPlacements); err != nil {
		return p, err
	}
	if targetPlacements {
		return p, ValidationError("Sebagian siswa sudah memiliki penempatan di tahun tujuan. Periksa penempatan Admin sebelum menjalankan transisi.")
	}
	classes, err := tx.QueryContext(ctx, `SELECT id,school_year_id,grade,name FROM app.classes WHERE school_year_id=$1 ORDER BY grade,name`, target)
	if err != nil {
		return p, err
	}
	defer classes.Close()
	for classes.Next() {
		var c Class
		if err = classes.Scan(&c.ID, &c.SchoolYearID, &c.Grade, &c.Name); err != nil {
			return p, err
		}
		p.Classes = append(p.Classes, c)
	}
	if err = classes.Err(); err != nil {
		return p, err
	}
	p.SourceYear, p.TargetYear = source, target
	return p, nil
}
