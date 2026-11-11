package academic

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"strings"
	"unicode/utf8"
)

type TPCriterion struct {
	ObjectiveID int64
	Threshold   string
	Version     int64
	Configured  bool
}
type TPCriteriaHistory struct {
	ObjectiveID                      int64
	Before, After, Actor, Reason, At string
	Version                          int64
}
type TPCriteria struct {
	Default   TPCriterion
	Overrides map[int64]TPCriterion
	History   []TPCriteriaHistory
}
type TPGuidance struct {
	Text    string
	Version int64
	History []TPGuidanceHistory
}
type TPGuidanceHistory struct {
	Before, After, Actor, Reason, At string
	Version                          int64
}

func tpReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if !utf8.ValidString(reason) || strings.ContainsRune(reason, 0) || utf8.RuneCountInString(reason) < 1 || utf8.RuneCountInString(reason) > 500 {
		return "", ValidationError("Alasan wajib diisi, maksimal 500 karakter.")
	}
	return reason, nil
}
func tpAuthorized(ctx context.Context, tx *sql.Tx, actor, semester, class, subject int64) error {
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
	return nil
}

// TPCriteriaForContext memuat batas dan histori hanya untuk konteks penugasan yang sah.
func (s *Store) TPCriteriaForContext(ctx context.Context, actor, semester, class, subject int64) (TPCriteria, error) {
	out := TPCriteria{Overrides: map[int64]TPCriterion{}, History: []TPCriteriaHistory{}}
	tx, err := s.reader(ctx, actor, "guru_mapel")
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = tpAuthorized(ctx, tx, actor, semester, class, subject); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT coalesce(objective_id,0),threshold::text,version FROM app.tp_achievement_criteria WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semester, class, subject)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item TPCriterion
		var value sql.NullString
		if err = rows.Scan(&item.ObjectiveID, &value, &item.Version); err != nil {
			rows.Close()
			return out, err
		}
		item.Configured = value.Valid
		item.Threshold = value.String
		if item.ObjectiveID == 0 {
			out.Default = item
		} else {
			out.Overrides[item.ObjectiveID] = item
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT coalesce(c.objective_id,0),coalesce(h.previous_threshold::text,'—'),coalesce(h.threshold::text,'Bawaan'),h.version,u.display_name,h.reason,to_char(h.changed_at AT TIME ZONE 'Asia/Jakarta','DD-MM-YYYY HH24:MI') FROM app.tp_achievement_criteria_history h JOIN app.tp_achievement_criteria c ON c.id=h.criteria_id JOIN app.users u ON u.id=h.actor_id WHERE c.semester_id=$1 AND c.class_id=$2 AND c.subject_id=$3 ORDER BY h.id DESC`, semester, class, subject)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var h TPCriteriaHistory
		if err = rows.Scan(&h.ObjectiveID, &h.Before, &h.After, &h.Version, &h.Actor, &h.Reason, &h.At); err != nil {
			return out, err
		}
		out.History = append(out.History, h)
	}
	return out, rows.Err()
}

// SaveTPCriterion menulis konfigurasi dan histori atomik, dengan versi serta batas konteks.
func (s *Store) SaveTPCriterion(ctx context.Context, actor, semester, class, subject, objective, expected int64, raw, reason string, reset bool) error {
	if semester < 1 || class < 1 || subject < 1 || objective < 0 || expected < 0 || reset && objective == 0 {
		return ValidationError("Konteks atau versi kriteria tidak valid.")
	}
	reason, err := tpReason(reason)
	if err != nil {
		return err
	}
	var value any
	if !reset {
		cents, present, e := parseInitialScore(raw)
		if e != nil {
			return e
		}
		if !present {
			return ValidationError("Batas ketercapaian wajib diisi.")
		}
		value = formatScore(cents)
	}
	tx, err := s.begin(ctx, actor, "guru_mapel")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = tpAuthorized(ctx, tx, actor, semester, class, subject); err != nil {
		return err
	}
	if objective > 0 {
		var ok bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.learning_objectives WHERE id=$1 AND semester_id=$2 AND class_id=$3 AND subject_id=$4)`, objective, semester, class, subject).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return ErrForbidden
		}
	}
	var id, version int64
	var previous sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id,version,threshold::text FROM app.tp_achievement_criteria WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND objective_id IS NOT DISTINCT FROM NULLIF($4,0) FOR UPDATE`, semester, class, subject, objective).Scan(&id, &version, &previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		if expected != 0 {
			return ErrConflict
		}
		if reset {
			return ValidationError("Pengecualian belum diatur.")
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO app.tp_achievement_criteria(semester_id,class_id,subject_id,objective_id,threshold,version,actor_id) VALUES($1,$2,$3,NULLIF($4,0),$5::numeric,1,$6) RETURNING id`, semester, class, subject, objective, value, actor).Scan(&id)
		version = 1
	} else {
		if version != expected {
			return ErrConflict
		}
		version++
		_, err = tx.ExecContext(ctx, `UPDATE app.tp_achievement_criteria SET threshold=$1::numeric,version=$2,actor_id=$3,updated_at=clock_timestamp() WHERE id=$4`, value, version, actor, id)
	}
	if err != nil {
		return dbError(err)
	}
	var before any
	if previous.Valid {
		before = previous.String
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.tp_achievement_criteria_history(criteria_id,previous_threshold,threshold,version,actor_id,reason) VALUES($1,$2::numeric,$3::numeric,$4,$5,$6)`, id, before, value, version, actor, reason); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) TPGuidance(ctx context.Context, actor int64) (TPGuidance, error) {
	var out TPGuidance
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT guidance,version FROM app.tp_achievement_guidance WHERE id=1`).Scan(&out.Text, &out.Version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT coalesce(h.previous_guidance,'—'),h.guidance,h.version,u.display_name,h.reason,to_char(h.changed_at AT TIME ZONE 'Asia/Jakarta','DD-MM-YYYY HH24:MI') FROM app.tp_achievement_guidance_history h JOIN app.users u ON u.id=h.actor_id ORDER BY h.version DESC`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var h TPGuidanceHistory
		if err = rows.Scan(&h.Before, &h.After, &h.Version, &h.Actor, &h.Reason, &h.At); err != nil {
			return out, err
		}
		out.History = append(out.History, h)
	}
	return out, rows.Err()
}
func (s *Store) SaveTPGuidance(ctx context.Context, actor, expected int64, text, reason string) error {
	text = strings.TrimSpace(text)
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) || utf8.RuneCountInString(text) < 1 || utf8.RuneCountInString(text) > 4000 || expected < 0 {
		return ValidationError("Pedoman wajib diisi, maksimal 4.000 karakter.")
	}
	reason, err := tpReason(reason)
	if err != nil {
		return err
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old string
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT guidance,version FROM app.tp_achievement_guidance WHERE id=1 FOR UPDATE`).Scan(&old, &version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if version != expected {
		return ErrConflict
	}
	version++
	_, err = tx.ExecContext(ctx, `INSERT INTO app.tp_achievement_guidance(id,guidance,version,actor_id) VALUES(1,$1,$2,$3) ON CONFLICT(id) DO UPDATE SET guidance=excluded.guidance,version=excluded.version,actor_id=excluded.actor_id,updated_at=clock_timestamp()`, text, version, actor)
	if err != nil {
		return err
	}
	var previous any
	if version > 1 {
		previous = old
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.tp_achievement_guidance_history(previous_guidance,guidance,version,actor_id,reason) VALUES($1,$2,$3,$4,$5)`, previous, text, version, actor, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// tpAchievement membandingkan rata-rata eksak, sebelum pembulatan tampilan.
func tpAchievement(average *big.Rat, complete bool, threshold string) string {
	if !complete {
		return "Belum Lengkap"
	}
	if threshold == "" {
		return "Belum Diatur"
	}
	limit, ok := new(big.Rat).SetString(threshold)
	if !ok || average == nil {
		return "Belum Diatur"
	}
	if average.Cmp(limit) >= 0 {
		return "Mencapai"
	}
	return "Belum Mencapai"
}
