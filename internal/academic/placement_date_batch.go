package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

type PlacementStartBatch struct {
	YearID, ClassID int64
	Date, Reason    string
	IDs             []int64
}
type PlacementStartRow struct {
	PlacementDate
	Issue string
}
type PlacementStartPreview struct {
	Rows   []PlacementStartRow
	Digest string
	Valid  bool
}

func (s *Store) SuggestedPlacementStart(ctx context.Context, actor, year int64) (string, error) {
	tx, e := s.reader(ctx, actor, "admin")
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var date string
	e = tx.QueryRowContext(ctx, `SELECT c.starts_on::text FROM app.semester_calendars c JOIN app.semesters sm ON sm.id=c.semester_id WHERE sm.school_year_id=$1 AND sm.number=1`, year).Scan(&date)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil
	}
	return date, e
}
func placementStartPreview(ctx context.Context, tx *sql.Tx, b PlacementStartBatch) (PlacementStartPreview, error) {
	p := PlacementStartPreview{Valid: true}
	if _, e := civilDate(b.Date); e != nil {
		return p, e
	}
	if e := foundationReason(b.Reason); e != nil {
		return p, e
	}
	if b.YearID < 1 || b.ClassID < 0 || len(b.IDs) == 0 || len(b.IDs) > 5000 {
		return p, ValidationError("Pilih 1–5000 siswa yang belum memiliki tanggal.")
	}
	ids := append([]int64(nil), b.IDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		if id < 1 || (i > 0 && id == ids[i-1]) {
			return p, ValidationError("Pilihan siswa tidak valid.")
		}
	}
	// Include calendar and movement state, not only the placement version, in the
	// confirmation fingerprint. Re-read under the mutation lock before any write.
	snapshots := []string{}
	for _, id := range ids {
		var row PlacementStartRow
		var state string
		e := tx.QueryRowContext(ctx, `SELECT p.id,p.school_year_id,p.date_version,s.name,c.grade::text||c.name,coalesce(p.starts_on::text,''),jsonb_build_array(to_jsonb(p),to_jsonb(s),to_jsonb(c),(SELECT jsonb_agg(to_jsonb(m) ORDER BY m.id) FROM app.student_class_moves m WHERE m.placement_id=p.id),(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM app.student_school_exits x WHERE x.student_id=p.student_id),(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM app.student_school_returns r WHERE r.placement_id=p.id),(SELECT jsonb_agg(to_jsonb(cal) ORDER BY cal.semester_id) FROM app.semester_calendars cal JOIN app.semesters sm ON sm.id=cal.semester_id WHERE sm.school_year_id=p.school_year_id))::text FROM app.student_placements p JOIN app.students s ON s.id=p.student_id JOIN app.classes c ON c.id=p.class_id WHERE p.id=$1 AND p.school_year_id=$2 AND ($3::bigint=0 OR p.class_id=$3)`, id, b.YearID, b.ClassID).Scan(&row.ID, &row.YearID, &row.Version, &row.StudentName, &row.ClassName, &row.StartsOn, &state)
		if errors.Is(e, sql.ErrNoRows) {
			return p, ErrConflict
		}
		if e != nil {
			return p, e
		}
		if row.StartsOn != "" {
			row.Issue = "Tanggal sudah terisi; tidak akan ditimpa."
		} else if e = validatePlacementStart(ctx, tx, id, b.YearID, b.Date); e != nil {
			var validation ValidationError
			if !errors.As(e, &validation) {
				return p, e
			}
			row.Issue = string(validation)
		}
		if row.Issue != "" {
			p.Valid = false
		}
		p.Rows = append(p.Rows, row)
		snapshots = append(snapshots, state)
	}
	b.IDs = ids
	raw, e := json.Marshal(struct {
		Batch  PlacementStartBatch
		States []string
	}{b, snapshots})
	if e != nil {
		return p, e
	}
	sum := sha256.Sum256(raw)
	p.Digest = hex.EncodeToString(sum[:])
	return p, nil
}
func (s *Store) PreviewPlacementStartBatch(ctx context.Context, actor int64, b PlacementStartBatch) (PlacementStartPreview, error) {
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return PlacementStartPreview{}, e
	}
	defer tx.Rollback()
	if e = s.authorize(ctx, tx, actor, "admin"); e != nil {
		return PlacementStartPreview{}, e
	}
	if s.PlacementWriter != nil {
		if _, err := civilDate(b.Date); err != nil {
			return PlacementStartPreview{}, err
		}
		return placementWriterPreview(ctx, tx, actor, b, true)
	}
	return placementStartPreview(ctx, tx, b)
}
func (s *Store) ApplyPlacementStartBatch(ctx context.Context, actor int64, b PlacementStartBatch, digest string) (int, error) {
	if s.PlacementWriter != nil {
		return s.applyPlacementWithWriter(ctx, actor, b, digest, true)
	}
	tx, e := s.begin(ctx, actor, "admin")
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	p, e := placementStartPreview(ctx, tx, b)
	if e != nil {
		return 0, e
	}
	if !p.Valid || digest == "" || p.Digest != digest {
		return 0, ErrConflict
	}
	for _, row := range p.Rows {
		result, e := tx.ExecContext(ctx, `UPDATE app.student_placements SET starts_on=$2,date_version=date_version+1 WHERE id=$1 AND starts_on IS NULL AND date_version=$3`, row.ID, b.Date, row.Version)
		if e != nil {
			return 0, e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return 0, e
		}
		if n != 1 {
			return 0, ErrConflict
		}
		after := row.PlacementDate
		after.StartsOn = b.Date
		after.Version++
		if e = foundationHistory(ctx, tx, actor, "placement_date", row.ID, row.PlacementDate, after, b.Reason); e != nil {
			return 0, e
		}
	}
	if e = tx.Commit(); e != nil {
		return 0, e
	}
	return len(p.Rows), nil
}
