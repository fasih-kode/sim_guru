package academic

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

func academicWriterError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "42501":
			return ErrForbidden
		case "40001", "40P01", "23514":
			return ErrConflict
		case "22023", "22007", "22008":
			return ValidationError(pg.Message)
		}
	}
	return err
}

func execAcademicWriter(ctx context.Context, db *sql.DB, statement string, args ...any) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, statement, args...); err != nil {
		return academicWriterError(err)
	}
	return academicWriterError(tx.Commit())
}

func placementWriterPreview(ctx context.Context, tx *sql.Tx, actor int64, b PlacementStartBatch, batch bool) (PlacementStartPreview, error) {
	var p PlacementStartPreview
	rows, err := tx.QueryContext(ctx, `SELECT id,year_id,date_version,student_name,class_name,starts_on,date_valid,digest
		FROM academic_write.preview_placement_batch($1,$2,$3,$4::bigint[],$5::date,$6,$7)`,
		actor, b.YearID, b.ClassID, b.IDs, b.Date, b.Reason, batch)
	if err != nil {
		return p, academicWriterError(err)
	}
	defer rows.Close()
	p.Valid = true
	for rows.Next() {
		var row PlacementStartRow
		var valid bool
		if err = rows.Scan(&row.ID, &row.YearID, &row.Version, &row.StudentName, &row.ClassName, &row.StartsOn, &valid, &p.Digest); err != nil {
			return p, err
		}
		switch {
		case batch && row.StartsOn != "":
			row.Issue = "Tanggal sudah terisi; tidak akan ditimpa."
		case !valid:
			row.Issue = "Tanggal tidak sesuai kalender atau kejadian siswa."
		}
		p.Valid = p.Valid && row.Issue == ""
		p.Rows = append(p.Rows, row)
	}
	if err = rows.Err(); err != nil {
		return p, academicWriterError(err)
	}
	if len(p.Rows) != len(b.IDs) || p.Digest == "" {
		return p, ErrConflict
	}
	return p, nil
}

func (s *Store) savePlacementWithWriter(ctx context.Context, actor, id, version int64, date, reason string) error {
	if _, err := civilDate(date); err != nil {
		return err
	}
	tx, err := s.reader(ctx, actor, "admin")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	batch := PlacementStartBatch{Date: date, Reason: reason, IDs: []int64{id}}
	err = tx.QueryRowContext(ctx, `SELECT school_year_id,class_id FROM app.student_placements WHERE id=$1`, id).Scan(&batch.YearID, &batch.ClassID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	p, err := placementWriterPreview(ctx, tx, actor, batch, false)
	if err != nil {
		return err
	}
	if p.Rows[0].Version != version {
		return ErrConflict
	}
	if !p.Valid {
		return ValidationError(p.Rows[0].Issue)
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	_, err = s.applyPlacementWithWriter(ctx, actor, batch, p.Digest, false)
	return err
}

func (s *Store) applyPlacementWithWriter(ctx context.Context, actor int64, batch PlacementStartBatch, digest string, bulk bool) (int, error) {
	// Token v1 yang masih aktif harus dimuat ulang saat kontrak v2 dipasang.
	if !strings.HasPrefix(digest, "v2:") {
		return 0, ErrConflict
	}
	tx, err := s.PlacementWriter.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var count int
	err = tx.QueryRowContext(ctx, `SELECT academic_write.set_placement_dates($1,$2,$3,$4::bigint[],$5,$6::date,$7,$8)`,
		actor, batch.YearID, batch.ClassID, batch.IDs, digest, batch.Date, batch.Reason, bulk).Scan(&count)
	if err != nil {
		return 0, academicWriterError(err)
	}
	if count != len(batch.IDs) {
		return 0, ErrConflict
	}
	return count, academicWriterError(tx.Commit())
}
