package master

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/jackc/pgx/v5/pgconn"
)

type Store struct{ DB *sql.DB }
type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func authorize(ctx context.Context, q queryer, actor int64, kind Kind, write bool) error {
	if !kind.Valid() {
		return ValidationError("Jenis master data tidak valid.")
	}
	role := kind.WriterRole()
	readRole := role
	if !write && kind != Subjects {
		readRole = "kurikulum"
	}
	var allowed bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.users u JOIN app.user_roles r ON r.user_id=u.id WHERE u.id=$1 AND u.active AND r.role_code IN ($2,$3))`, actor, role, readRole).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
func selection(kind Kind) string {
	// Every identifier is selected from a fixed whitelist, never from user input.
	switch kind {
	case Students:
		return `SELECT id,name,to_char(birth_date,'YYYY-MM-DD') AS birth_date,gender,COALESCE(nis,'') AS nis,COALESCE(nisn,'') AS nisn,phone,active,version FROM app.students`
	case Teachers:
		return `SELECT id,name,nip,nuptk,email,phone,COALESCE(user_id,0) AS user_id,active,version FROM app.teachers`
	case Subjects:
		return `SELECT id,name,code,active,version FROM app.subjects`
	}
	return ""
}
func scan(kind Kind, row interface{ Scan(...any) error }) (Record, error) {
	var r Record
	var err error
	switch kind {
	case Students:
		err = row.Scan(&r.ID, &r.Name, &r.BirthDate, &r.Gender, &r.NIS, &r.NISN, &r.Phone, &r.Active, &r.Version)
	case Teachers:
		err = row.Scan(&r.ID, &r.Name, &r.NIP, &r.NUPTK, &r.Email, &r.Phone, &r.UserID, &r.Active, &r.Version)
	case Subjects:
		err = row.Scan(&r.ID, &r.Name, &r.Code, &r.Active, &r.Version)
	default:
		return r, ValidationError("Jenis master data tidak valid.")
	}
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}
func get(ctx context.Context, q queryer, kind Kind, id int64) (Record, error) {
	return scan(kind, q.QueryRowContext(ctx, selection(kind)+` WHERE id=$1`, id))
}
func readAccess(ctx context.Context, q queryer, actor int64, kind Kind) (bool, error) {
	if err := authorize(ctx, q, actor, kind, false); err != nil {
		return false, err
	}
	err := authorize(ctx, q, actor, kind, true)
	if errors.Is(err, ErrForbidden) {
		return false, nil
	}
	return err == nil, err
}
func redact(r Record, canWrite bool) Record {
	if !canWrite {
		r.Phone = ""
		r.Email = ""
		r.UserID = 0
	}
	return r
}
func (s *Store) Get(ctx context.Context, actor int64, kind Kind, id int64) (Record, error) {
	canWrite, err := readAccess(ctx, s.DB, actor, kind)
	if err != nil {
		return Record{}, err
	}
	r, err := get(ctx, s.DB, kind, id)
	return redact(r, canWrite), err
}
func (s *Store) List(ctx context.Context, actor int64, kind Kind, f Filter) ([]Record, error) {
	canWrite, err := readAccess(ctx, s.DB, actor, kind)
	if err != nil {
		return nil, err
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	searchFields := `name ILIKE $1`
	switch kind {
	case Students:
		searchFields += ` OR nis ILIKE $1 OR nisn ILIKE $1`
	case Teachers:
		searchFields += ` OR nip ILIKE $1 OR nuptk ILIKE $1`
	case Subjects:
		searchFields += ` OR code ILIKE $1`
	}
	query := selection(kind) + ` WHERE (` + searchFields + `) AND ($2='' OR active=($2='active')) ORDER BY id LIMIT 11 OFFSET $3`
	rows, err := s.DB.QueryContext(ctx, query, "%"+f.Search+"%", f.Status, (f.Page-1)*10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		r, err := scan(kind, rows)
		if err != nil {
			return nil, err
		}
		records = append(records, redact(r, canWrite))
	}
	return records, rows.Err()
}
func conflict(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return ValidationError("Akun terkait tidak ditemukan. Muat ulang pilihan akun.")
		}
	}
	return err
}
func (s *Store) Save(ctx context.Context, actor int64, kind Kind, id int64, in Input) (Record, error) {
	if err := in.Validate(kind, id == 0); err != nil {
		return Record{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return Record{}, err
	}
	if err = authorize(ctx, tx, actor, kind, true); err != nil {
		return Record{}, err
	}
	if id == 0 {
		in.Reason = ""
	}
	saved, err := saveRecord(ctx, tx, actor, kind, id, in)
	if err != nil {
		return Record{}, err
	}
	return saved, tx.Commit()
}

// saveRecord is shared by individual forms and atomic imports; callers validate and authorize first.
func saveRecord(ctx context.Context, tx *sql.Tx, actor int64, kind Kind, id int64, in Input) (Record, error) {
	var err error
	var before *Record
	if id != 0 {
		previous, err := get(ctx, tx, kind, id)
		if err != nil {
			return Record{}, err
		}
		if previous.Version != in.Version {
			return Record{}, ErrConflict
		}
		before = &previous
	}
	if id == 0 {
		switch kind {
		case Students:
			err = tx.QueryRowContext(ctx, `INSERT INTO app.students(name,birth_date,gender,nis,nisn,phone,active) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7) RETURNING id`, in.Name, in.BirthDate, in.Gender, in.NIS, in.NISN, in.Phone, in.Active).Scan(&id)
		case Teachers:
			err = tx.QueryRowContext(ctx, `INSERT INTO app.teachers(name,nip,nuptk,email,phone,user_id,active) VALUES($1,$2,$3,$4,$5,NULLIF($6::bigint,0),$7) RETURNING id`, in.Name, in.NIP, in.NUPTK, in.Email, in.Phone, in.UserID, in.Active).Scan(&id)
		case Subjects:
			err = tx.QueryRowContext(ctx, `INSERT INTO app.subjects(name,code,active) VALUES($1,$2,$3) RETURNING id`, in.Name, in.Code, in.Active).Scan(&id)
		}
	} else {
		switch kind {
		case Students:
			_, err = tx.ExecContext(ctx, `UPDATE app.students SET name=$1,birth_date=$2,gender=$3,nis=NULLIF($4,''),nisn=NULLIF($5,''),phone=$6,active=$7,version=version+1,updated_at=now() WHERE id=$8`, in.Name, in.BirthDate, in.Gender, in.NIS, in.NISN, in.Phone, in.Active, id)
		case Teachers:
			_, err = tx.ExecContext(ctx, `UPDATE app.teachers SET name=$1,nip=$2,nuptk=$3,email=$4,phone=$5,user_id=NULLIF($6::bigint,0),active=$7,version=version+1,updated_at=now() WHERE id=$8`, in.Name, in.NIP, in.NUPTK, in.Email, in.Phone, in.UserID, in.Active, id)
		case Subjects:
			_, err = tx.ExecContext(ctx, `UPDATE app.subjects SET name=$1,code=$2,active=$3,version=version+1,updated_at=now() WHERE id=$4`, in.Name, in.Code, in.Active, id)
		}
	}
	if err != nil {
		return Record{}, conflict(err)
	}
	if kind == Subjects {
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.subject_codes(code,subject_id) VALUES($1,$2) ON CONFLICT(code) DO NOTHING`, in.Code, id); err != nil {
			return Record{}, err
		}
		var owner int64
		if err = tx.QueryRowContext(ctx, `SELECT subject_id FROM app.subject_codes WHERE code=$1`, in.Code).Scan(&owner); err != nil {
			return Record{}, err
		}
		if owner != id {
			return Record{}, ErrConflict
		}
	}
	after, err := get(ctx, tx, kind, id)
	if err != nil {
		return Record{}, err
	}
	var beforeJSON any
	if before != nil {
		raw, err := json.Marshal(before)
		if err != nil {
			return Record{}, err
		}
		beforeJSON = string(raw)
	}
	raw, err := json.Marshal(after)
	if err != nil {
		return Record{}, err
	}
	reason := in.Reason
	if before == nil && reason == "" {
		reason = "Pembuatan data induk"
	}
	query := fmt.Sprintf(`INSERT INTO %s(entity_id,actor_id,before_state,after_state,reason) VALUES($1,$2,$3,$4,$5)`, kind.HistoryTable())
	if _, err = tx.ExecContext(ctx, query, id, actor, beforeJSON, string(raw), reason); err != nil {
		return Record{}, err
	}
	return after, nil
}

func (s *Store) History(ctx context.Context, actor int64, kind Kind, id int64) ([]History, error) {
	if err := authorize(ctx, s.DB, actor, kind, true); err != nil {
		return nil, err
	}
	if _, err := get(ctx, s.DB, kind, id); err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT h.id,u.username,COALESCE(h.before_state::text,''),h.after_state::text,h.reason,h.created_at FROM %s h JOIN app.users u ON u.id=h.actor_id WHERE h.entity_id=$1 ORDER BY h.id DESC LIMIT 100`, kind.HistoryTable())
	rows, err := s.DB.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []History
	for rows.Next() {
		var h History
		if err = rows.Scan(&h.ID, &h.Actor, &h.Before, &h.After, &h.Reason, &h.At); err != nil {
			return nil, err
		}
		records = append(records, h)
	}
	return records, rows.Err()
}
func (s *Store) Accounts(ctx context.Context, actor int64) ([]Account, error) {
	if err := authorize(ctx, s.DB, actor, Teachers, true); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,username,display_name,active FROM app.users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []Account
	for rows.Next() {
		var a Account
		if err = rows.Scan(&a.ID, &a.Username, &a.DisplayName, &a.Active); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}
