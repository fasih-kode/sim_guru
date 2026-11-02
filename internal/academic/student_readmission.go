package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ReadmitStudent records a new event; the exit and the original placement remain
// intact. A new placement is needed only when this school year has none.
func (s *Store) ReadmitStudent(ctx context.Context, actor, student, version, exitID, target int64, date, reason, school string) error {
	if err := validateMovement(date, reason, school); err != nil {
		return err
	}
	tx, err := s.begin(ctx, actor, "admin")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	v, err := movementStudent(ctx, tx, student, 0)
	if err != nil {
		return err
	}
	if v.Version != version || v.ExitID != exitID {
		return ErrConflict
	}
	today := time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02")
	if exitID < 1 || v.ExitDate > today || v.ReturnDate != "" || date <= v.ExitDate {
		return ValidationError("Pilih siswa yang sudah keluar, belum diterima kembali, dan tanggal kembali setelah tanggal keluar.")
	}
	var year int64
	var grade int
	var active bool
	err = tx.QueryRowContext(ctx, `SELECT c.school_year_id,c.grade,y.active FROM app.classes c JOIN app.school_years y ON y.id=c.school_year_id WHERE c.id=$1`, target).Scan(&year, &grade, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !active {
		return ValidationError("Pilih kelas pada tahun ajaran aktif.")
	}
	if year == v.OriginYear && grade != v.OriginGrade {
		return ValidationError("Kembali pada tahun yang sama harus ke tingkat kelas yang sama.")
	}
	var closed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_year_transitions WHERE source_year_id=$1)`, year).Scan(&closed); err != nil {
		return err
	}
	if closed {
		return ValidationError("Transisi tahun aktif sudah dijalankan. Kurikulum perlu mengaktifkan tahun tujuan sebelum penerimaan kembali.")
	}
	var placement int64
	var placedGrade int
	err = tx.QueryRowContext(ctx, `SELECT p.id,c.grade FROM app.student_placements p JOIN app.classes c ON c.id=p.class_id WHERE p.student_id=$1 AND p.school_year_id=$2`, student, year).Scan(&placement, &placedGrade)
	created := false
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5) RETURNING id`, year, student, target, actor, date).Scan(&placement)
		if err != nil {
			return dbError(err)
		}
		created = true
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.student_placement_history(placement_id,actor_id,to_class_id,reason) VALUES($1,$2,$3,'Diterima kembali oleh Admin')`, placement, actor, target); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if placedGrade != grade {
		return ValidationError("Siswa sudah memiliki penempatan pada tingkat berbeda di tahun aktif. Periksa riwayat akademik sebelum menerima kembali.")
	}
	var conflict bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_year_transitions WHERE source_placement_id=$1) OR EXISTS(SELECT 1 FROM app.student_class_moves WHERE placement_id=$1 AND effective_date >= $2::date)`, placement, date).Scan(&conflict)
	if err != nil {
		return err
	}
	if conflict {
		return ValidationError("Penempatan sudah diproses transisi atau tanggal kembali bertentangan dengan perpindahan kelas.")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.student_school_returns(exit_id,placement_id,target_class_id,effective_date,source_school,reason,created_placement,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, exitID, placement, target, date, strings.TrimSpace(school), strings.TrimSpace(reason), created, actor)
	if err != nil {
		return dbError(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.students SET active=true,version=version+1,updated_at=now() WHERE id=$1`, student); err != nil {
		return err
	}
	return tx.Commit()
}
