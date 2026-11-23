package academic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/jackc/pgx/v5/pgconn"
)

// DirectTPScoreInput uses version -1 for an empty cell and 0 for an original score.
type DirectTPScoreInput struct {
	StudentID, ObjectiveID int64
	ExpectedVersion        int64
	Value                  string
}

type directTPParsed struct {
	StudentID, ObjectiveID int64
	ExpectedVersion        int64
	Cents                  int64
	Present                bool
}

type DirectTPScore struct {
	StudentID, ObjectiveID, InitialID, Version int64
	Original, Current                          string
}

func (s *Store) beginDirectTP(ctx context.Context, actor int64) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err = s.authorize(ctx, tx, actor, "guru_mapel"); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// DirectTPScores reads existing records for an actively assigned teacher.
func (s *Store) DirectTPScores(ctx context.Context, actor, semester, class, subject int64) ([]DirectTPScore, error) {
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return nil, err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.student_id,s.objective_id,s.id,coalesce(c.version,0),s.score::text,coalesce(c.after_score,s.score)::text
		FROM app.direct_tp_initial_scores s LEFT JOIN LATERAL (SELECT version,after_score FROM app.direct_tp_score_corrections WHERE initial_score_id=s.id ORDER BY version DESC LIMIT 1) c ON true
		WHERE (s.semester_id,s.class_id,s.subject_id)=($1,$2,$3) ORDER BY s.student_id,s.objective_id`, semester, class, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]DirectTPScore, 0)
	for rows.Next() {
		var item DirectTPScore
		if err = rows.Scan(&item.StudentID, &item.ObjectiveID, &item.InitialID, &item.Version, &item.Original, &item.Current); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// SaveDirectTPScores records an atomic batch without modifying Final or existing originals.
func (s *Store) SaveDirectTPScores(ctx context.Context, actor, semester, class, subject int64, key string, inputs []DirectTPScoreInput) error {
	if semester < 1 || class < 1 || subject < 1 || !validSubmissionUUID(key) || len(inputs) == 0 || len(inputs) > 5000 {
		return ValidationError("Konteks atau daftar nilai tidak valid.")
	}
	items := make([]directTPParsed, 0, len(inputs))
	seen := make(map[[2]int64]bool, len(inputs))
	for _, in := range inputs {
		if in.StudentID < 1 || in.ObjectiveID < 1 || in.ExpectedVersion < -1 || seen[[2]int64{in.StudentID, in.ObjectiveID}] {
			return ValidationError("Siswa atau TP tidak valid atau duplikat.")
		}
		seen[[2]int64{in.StudentID, in.ObjectiveID}] = true
		cents, present, err := parseInitialScore(in.Value)
		if err != nil {
			return err
		}
		if !present && in.ExpectedVersion >= 0 {
			return ValidationError("Mengosongkan nilai yang sudah tercatat belum tersedia.")
		}
		items = append(items, directTPParsed{in.StudentID, in.ObjectiveID, in.ExpectedVersion, cents, present})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StudentID != items[j].StudentID {
			return items[i].StudentID < items[j].StudentID
		}
		return items[i].ObjectiveID < items[j].ObjectiveID
	})
	payload, err := json.Marshal(struct {
		Semester, Class, Subject int64
		Items                    []directTPParsed
	}{semester, class, subject, items})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	tx, err := s.beginDirectTP(ctx, actor)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	teacher, err := learningObjectiveTeacherID(ctx, tx, actor)
	if err != nil {
		return err
	}
	allowed, err := learningObjectiveAssignment(ctx, tx, teacher, semester, class, subject)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	var old []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM app.direct_tp_submissions WHERE actor_id=$1 AND submission_key=$2::uuid`, actor, key).Scan(&old)
	if err == nil {
		if !equalHash(old, digest[:]) {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var year int64
	err = tx.QueryRowContext(ctx, `SELECT sm.school_year_id FROM app.semesters sm JOIN app.classes c ON c.id=$2 AND c.school_year_id=sm.school_year_id JOIN app.semester_calendars cal ON cal.semester_id=sm.id WHERE sm.id=$1`, semester, class).Scan(&year)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	type change struct {
		item                             directTPParsed
		placement, initial, version, old int64
	}
	changes := make([]change, 0, len(items))
	for _, item := range items {
		applicability, placement, _, err := readDirectTPApplicability(ctx, tx, semester, class, subject, item.StudentID, item.ObjectiveID)
		if err != nil {
			return err
		}
		if applicability != directTPApplicable {
			return ErrForbidden
		}
		if err = tx.QueryRowContext(ctx, `SELECT id FROM app.student_placements WHERE id=$1 FOR SHARE`, placement).Scan(&placement); err != nil {
			return err
		}
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.direct_tp_codes WHERE objective_id=$1)`, item.ObjectiveID).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return ErrForbidden
		}
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.semester_final_versions WHERE (semester_id,class_id,subject_id,student_id)=($1,$2,$3,$4))`, semester, class, subject, item.StudentID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			return ErrConflict
		}
		var initial, owner, version, old int64
		err = tx.QueryRowContext(ctx, `SELECT s.id,s.owner_teacher_id,coalesce(c.version,0),(coalesce(c.after_score,s.score)*100)::bigint FROM app.direct_tp_initial_scores s LEFT JOIN LATERAL (SELECT version,after_score FROM app.direct_tp_score_corrections WHERE initial_score_id=s.id ORDER BY version DESC LIMIT 1) c ON true WHERE (s.semester_id,s.class_id,s.subject_id,s.objective_id,s.student_id)=($1,$2,$3,$4,$5)`, semester, class, subject, item.ObjectiveID, item.StudentID).Scan(&initial, &owner, &version, &old)
		if errors.Is(err, sql.ErrNoRows) {
			if item.ExpectedVersion != -1 {
				return ErrConflict
			}
			if item.Present {
				changes = append(changes, change{item: item, placement: placement})
			}
			continue
		}
		if err != nil {
			return err
		}
		if owner != teacher {
			return ErrForbidden
		}
		if item.ExpectedVersion != version {
			return ErrConflict
		}
		if !item.Present {
			return ValidationError("Mengosongkan nilai yang sudah tercatat belum tersedia.")
		}
		if item.Cents != old {
			changes = append(changes, change{item: item, placement: placement, initial: initial, version: version, old: old})
		}
	}
	if len(changes) == 0 {
		anyPresent := false
		for _, item := range items {
			anyPresent = anyPresent || item.Present
		}
		if !anyPresent {
			return ValidationError("Isi sedikitnya satu nilai sebelum menyimpan.")
		}
	}
	var batch int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app.direct_tp_submissions(school_year_id,semester_id,class_id,subject_id,teacher_id,actor_id,submission_key,request_sha256,expected_count) VALUES($1,$2,$3,$4,$5,$6,$7::uuid,$8,$9) RETURNING id`, year, semester, class, subject, teacher, actor, key, digest[:], len(changes)).Scan(&batch)
	if err != nil {
		return dbError(err)
	}
	for _, c := range changes {
		if c.initial == 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO app.direct_tp_initial_scores(school_year_id,semester_id,class_id,subject_id,objective_id,student_id,placement_id,owner_teacher_id,recorded_by,score,submission_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::numeric,$11)`, year, semester, class, subject, c.item.ObjectiveID, c.item.StudentID, c.placement, teacher, actor, formatScore(c.item.Cents), batch)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO app.direct_tp_score_corrections(initial_score_id,version,before_score,after_score,corrected_by,submission_id) VALUES($1,$2,$3::numeric,$4::numeric,$5,$6)`, c.initial, c.version+1, formatScore(c.old), formatScore(c.item.Cents), actor, batch)
		}
		if err != nil {
			return directTPError(err)
		}
	}
	return directTPError(tx.Commit())
}

func directTPError(err error) error {
	if err == nil {
		return nil
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "40001" || pg.Code == "40P01" || pg.Code == "23514") {
		return ErrConflict
	}
	return dbError(err)
}
