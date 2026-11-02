package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type StudentMovement struct {
	ExitID, OriginYear                                                                                    int64
	OriginGrade                                                                                           int
	ExitDate, ReturnDate                                                                                  string
	At                                                                                                    time.Time
	ID, StudentID, PlacementID, ClassID, Version                                                          int64
	StudentName, YearName, ClassName, EffectiveDate, Reason, DestinationSchool, Actor, FromClass, ToClass string
	Grade                                                                                                 int
}

func validateMovement(date, reason, school string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return ValidationError("Tanggal berlaku wajib diisi dengan benar.")
	}
	if strings.TrimSpace(reason) == "" || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 500 || strings.ContainsRune(reason, 0) {
		return ValidationError("Alasan wajib diisi, maksimal 500 karakter.")
	}
	if !utf8.ValidString(school) || utf8.RuneCountInString(school) > 200 || strings.ContainsRune(school, 0) {
		return ValidationError("Nama sekolah maksimal 200 karakter.")
	}
	return nil
}

func movementStudent(ctx context.Context, tx *sql.Tx, student, placement int64) (StudentMovement, error) {
	var v StudentMovement
	err := tx.QueryRowContext(ctx, `SELECT s.id,s.name,s.version,coalesce(p.id,0),coalesce(p.class_id,0),coalesce(c.grade,0),coalesce(c.grade::text||c.name,''),coalesce(y.name,''),coalesce(e.id,0),coalesce(e.effective_date::text,''),coalesce(e.return_date::text,''),coalesce(f.school_year_id,0),coalesce(f.grade,0) FROM app.students s LEFT JOIN app.current_student_placements p ON p.student_id=s.id AND p.school_year_id IN(SELECT id FROM app.school_years WHERE active) LEFT JOIN app.classes c ON c.id=p.class_id LEFT JOIN app.school_years y ON y.id=p.school_year_id LEFT JOIN app.outstanding_student_school_exits e ON e.student_id=s.id LEFT JOIN app.classes f ON f.id=e.from_class_id WHERE ($1::bigint>0 AND s.id=$1) OR ($2::bigint>0 AND p.id=$2)`, student, placement).Scan(&v.StudentID, &v.StudentName, &v.Version, &v.PlacementID, &v.ClassID, &v.Grade, &v.ClassName, &v.YearName, &v.ExitID, &v.ExitDate, &v.ReturnDate, &v.OriginYear, &v.OriginGrade)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
func (s *Store) MovementStudent(ctx context.Context, actor, student, placement int64) (StudentMovement, error) {
	tx, err := s.reader(ctx, actor, "admin", "kurikulum")
	if err != nil {
		return StudentMovement{}, err
	}
	defer tx.Rollback()
	return movementStudent(ctx, tx, student, placement)
}
func (s *Store) MoveStudentClass(ctx context.Context, actor, placement, expectedClass, target int64, date, reason string) error {
	if err := validateMovement(date, reason, ""); err != nil {
		return err
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var beforeStart bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_placements WHERE id=$1 AND starts_on>$2::date)`, placement, date).Scan(&beforeStart); err != nil {
		return err
	}
	if beforeStart {
		return ValidationError("Tanggal pindah mendahului mulai penempatan siswa.")
	}
	v, err := movementStudent(ctx, tx, 0, placement)
	if err != nil {
		return err
	}
	if v.ClassID != expectedClass {
		return ErrConflict
	}
	var year int64
	var grade int
	if err = tx.QueryRowContext(ctx, `SELECT school_year_id,grade FROM app.classes WHERE id=$1`, target).Scan(&year, &grade); err != nil {
		return ErrNotFound
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT $2::bigint<>$3::bigint AND $4=c.grade AND $5=p.school_year_id AND y.active AND NOT EXISTS(SELECT 1 FROM app.student_class_moves m WHERE m.placement_id=p.id AND (m.effective_date >= $6::date OR m.effective_date > (now() AT TIME ZONE 'Asia/Jakarta')::date)) AND NOT EXISTS(SELECT 1 FROM app.outstanding_student_school_exits e WHERE e.student_id=p.student_id) AND NOT EXISTS(SELECT 1 FROM app.student_school_returns r WHERE r.placement_id=p.id AND r.effective_date >= $6::date) AND NOT EXISTS(SELECT 1 FROM app.student_year_transitions t WHERE t.source_placement_id=p.id) FROM app.student_placements p JOIN app.classes c ON c.id=p.class_id JOIN app.school_years y ON y.id=p.school_year_id WHERE p.id=$1`, placement, v.ClassID, target, grade, year, date).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ValidationError("Pilih kelas berbeda pada tahun aktif dan tingkat yang sama. Periksa catatan pindah sekolah, perpindahan terjadwal, tanggal, dan transisi akhir tahun yang sudah dijalankan.")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6)`, placement, v.ClassID, target, date, strings.TrimSpace(reason), actor)
	if err != nil {
		return dbError(err)
	}
	return tx.Commit()
}
func (s *Store) ExitStudentSchool(ctx context.Context, actor, student, version int64, date, reason, school string) error {
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
	if v.Version != version {
		return ErrConflict
	}
	if v.ExitID > 0 {
		return ValidationError("Siswa masih memiliki catatan keluar atau kembali terjadwal.")
	}
	var beforeStart bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_placements p JOIN app.school_years y ON y.id=p.school_year_id WHERE p.student_id=$1 AND y.active AND p.starts_on>$2::date)`, student, date).Scan(&beforeStart); err != nil {
		return err
	}
	if beforeStart {
		return ValidationError("Tanggal keluar mendahului mulai penempatan siswa.")
	}
	var conflict bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.student_school_returns r JOIN app.student_school_exits e ON e.id=r.exit_id WHERE e.student_id=$1 AND r.effective_date >= $2::date) OR EXISTS(SELECT 1 FROM app.student_class_moves m JOIN app.student_placements p ON p.id=m.placement_id WHERE p.student_id=$1 AND m.effective_date >= $2::date) OR EXISTS(SELECT 1 FROM app.student_year_transitions t JOIN app.school_years y ON y.id=t.source_year_id WHERE t.student_id=$1 AND y.active)`, student, date).Scan(&conflict)
	if err != nil {
		return err
	}
	if conflict {
		return ValidationError("Tanggal keluar bertentangan dengan perpindahan kelas atau siswa sudah diproses transisi akhir tahun. Periksa histori terlebih dahulu.")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.student_school_exits(student_id,effective_date,destination_school,reason,actor_id,from_class_id) VALUES($1,$2,$3,$4,$5,(SELECT app.student_placement_class_at(p.id,$2::date) FROM app.student_placements p JOIN app.school_years y ON y.id=p.school_year_id WHERE p.student_id=$1 AND y.active))`, student, date, strings.TrimSpace(school), strings.TrimSpace(reason), actor)
	if err != nil {
		return dbError(err)
	}
	// Bump the master version so stale Admin edits cannot silently overwrite the situation.
	if _, err = tx.ExecContext(ctx, `UPDATE app.students SET version=version+1,updated_at=now() WHERE id=$1`, student); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) StudentMovements(ctx context.Context, actor, student int64) ([]StudentMovement, error) {
	tx, err := s.reader(ctx, actor, "admin", "kurikulum")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT m.id,m.effective_date::text,m.reason,'' AS school,u.display_name,f.grade::text||f.name,t.grade::text||t.name,m.created_at FROM app.student_class_moves m JOIN app.student_placements p ON p.id=m.placement_id JOIN app.classes f ON f.id=m.from_class_id JOIN app.classes t ON t.id=m.to_class_id JOIN app.users u ON u.id=m.actor_id WHERE p.student_id=$1 UNION ALL SELECT e.id,e.effective_date::text,e.reason,e.destination_school,u.display_name,coalesce(f.grade::text||f.name,''),'Pindah sekolah',e.created_at FROM app.student_school_exits e JOIN app.users u ON u.id=e.actor_id LEFT JOIN app.classes f ON f.id=e.from_class_id WHERE e.student_id=$1 UNION ALL SELECT r.id,r.effective_date::text,r.reason,CASE WHEN r.source_school='' THEN '' ELSE 'Asal: '||r.source_school END,u.display_name,'Di luar sekolah','Terima kembali · '||c.grade::text||c.name,r.created_at FROM app.student_school_returns r JOIN app.student_school_exits e ON e.id=r.exit_id JOIN app.classes c ON c.id=r.target_class_id JOIN app.users u ON u.id=r.actor_id WHERE e.student_id=$1 ORDER BY 2 DESC,8 DESC`, student)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudentMovement
	for rows.Next() {
		var v StudentMovement
		var at time.Time
		if err = rows.Scan(&v.ID, &v.EffectiveDate, &v.Reason, &v.DestinationSchool, &v.Actor, &v.FromClass, &v.ToClass, &at); err != nil {
			return nil, err
		}
		v.At = at.In(time.FixedZone("WIB", 7*3600))
		out = append(out, v)
	}
	return out, rows.Err()
}
