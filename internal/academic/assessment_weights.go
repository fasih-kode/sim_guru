package academic

import (
	"context"
	"database/sql"
	"errors"
)

// AssessmentWeights adalah satu konfigurasi bobot sekolah untuk satu semester.
type AssessmentWeights struct {
	SemesterID     int64
	NH, ASTS, ASAS int
	Version        int64
	ActorName      string
	UpdatedAt      string
	Configured     bool
}

func validateAssessmentWeights(weights AssessmentWeights) error {
	if weights.SemesterID < 1 {
		return ValidationError("Pilih semester terlebih dahulu.")
	}
	if weights.NH < 0 || weights.NH > 100 || weights.ASTS < 0 || weights.ASTS > 100 || weights.ASAS < 0 || weights.ASAS > 100 {
		return ValidationError("Setiap bobot harus berupa bilangan bulat 0–100.")
	}
	if weights.NH+weights.ASTS+weights.ASAS != 100 {
		return ValidationError("Total bobot NH, ASTS, dan ASAS harus tepat 100%.")
	}
	return nil
}

// AssessmentWeight membaca konfigurasi semester tanpa menetapkan nilai bawaan.
func (s *Store) AssessmentWeight(ctx context.Context, actor, semesterID int64) (AssessmentWeights, error) {
	if semesterID < 1 {
		return AssessmentWeights{}, ValidationError("Pilih semester terlebih dahulu.")
	}
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return AssessmentWeights{}, err
	}
	defer tx.Rollback()
	var weights AssessmentWeights
	weights.SemesterID = semesterID
	err = tx.QueryRowContext(ctx, `
		SELECT w.nh,w.asts,w.asas,w.version,u.display_name,w.updated_at::text
		FROM app.semester_assessment_weights w
		JOIN app.users u ON u.id=w.actor_id
		WHERE w.semester_id=$1`, semesterID).Scan(
		&weights.NH, &weights.ASTS, &weights.ASAS, &weights.Version, &weights.ActorName, &weights.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return weights, nil
	}
	if err != nil {
		return AssessmentWeights{}, err
	}
	weights.Configured = true
	return weights, nil
}

// SaveAssessmentWeight menyimpan konfigurasi dan histori dalam transaksi atomik.
func (s *Store) SaveAssessmentWeight(ctx context.Context, actor int64, weights AssessmentWeights, expectedVersion int64) error {
	if err := validateAssessmentWeights(weights); err != nil {
		return err
	}
	if expectedVersion < 0 {
		return ValidationError("Versi konfigurasi tidak valid. Muat ulang halaman.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedSemesterID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app.semesters WHERE id=$1 FOR UPDATE`, weights.SemesterID).Scan(&lockedSemesterID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var previous AssessmentWeights
	err = tx.QueryRowContext(ctx, `SELECT nh,asts,asas,version FROM app.semester_assessment_weights WHERE semester_id=$1 FOR UPDATE`, weights.SemesterID).Scan(
		&previous.NH, &previous.ASTS, &previous.ASAS, &previous.Version,
	)
	configured := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if (!configured && expectedVersion != 0) || (configured && expectedVersion != previous.Version) {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,version,actor_id,updated_at)
		VALUES($1,$2,$3,$4,1,$5,clock_timestamp())
		ON CONFLICT(semester_id) DO UPDATE SET nh=EXCLUDED.nh,asts=EXCLUDED.asts,asas=EXCLUDED.asas,
			version=app.semester_assessment_weights.version+1,actor_id=EXCLUDED.actor_id,updated_at=clock_timestamp()`,
		weights.SemesterID, weights.NH, weights.ASTS, weights.ASAS, actor,
	)
	if err != nil {
		return err
	}
	var previousNH, previousASTS, previousASAS any
	if configured {
		previousNH, previousASTS, previousASAS = previous.NH, previous.ASTS, previous.ASAS
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO app.semester_assessment_weight_history(
			semester_id,previous_nh,previous_asts,previous_asas,nh,asts,asas,actor_id
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		weights.SemesterID, previousNH, previousASTS, previousASAS,
		weights.NH, weights.ASTS, weights.ASAS, actor,
	); err != nil {
		return err
	}
	return tx.Commit()
}
