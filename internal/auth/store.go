package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const SessionLifetime = 8 * time.Hour
const mutationLock int64 = 742009

type Store struct {
	DB        *sql.DB
	dummyHash string
}

func New(db *sql.DB) (*Store, error) {
	// A fresh dummy hash gives unknown users the same bcrypt workload without a default credential.
	dummy := make([]byte, 32)
	if _, err := rand.Read(dummy); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword(dummy, 12)
	if err != nil {
		return nil, err
	}
	return &Store{DB: db, dummyHash: string(hash)}, nil
}
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const userSelect = `SELECT u.id,u.username,u.display_name,u.active,u.version,
 COALESCE((SELECT jsonb_agg(role_code ORDER BY role_code) FROM app.user_roles WHERE user_id=u.id),'[]'::jsonb) FROM app.users u `

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var raw []byte
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Active, &u.Version, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, err
	}
	err = json.Unmarshal(raw, &u.Roles)
	return u, err
}
func getUser(ctx context.Context, q queryer, id int64) (User, error) {
	return scanUser(q.QueryRowContext(ctx, userSelect+`WHERE u.id=$1`, id))
}
func (s *Store) Get(ctx context.Context, id int64) (User, error) { return getUser(ctx, s.DB, id) }
func (s *Store) Session(ctx context.Context, token string) (User, error) {
	if len(token) != 64 {
		return User{}, ErrSession
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `UPDATE app.sessions s SET last_seen_at=now() FROM app.users u WHERE u.id=s.user_id AND s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND s.last_seen_at>now()-interval '30 minutes' AND u.active RETURNING s.user_id`, TokenHash(token)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrSession
	}
	if err != nil {
		return User{}, err
	}
	u, err := s.Get(ctx, id)
	if err == nil && !u.Active {
		return User{}, ErrSession
	}
	return u, err
}
func (s *Store) Login(ctx context.Context, username, password, oldToken string) (string, error) {
	if len(password) > 72 || len(NormalizeUsername(username)) > 64 {
		return "", ErrCredentials
	}
	var id int64
	var hash string
	var active bool
	err := s.DB.QueryRowContext(ctx, `SELECT id,password_hash,active FROM app.users WHERE username=$1`, NormalizeUsername(username)).Scan(&id, &hash, &active)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if errors.Is(err, sql.ErrNoRows) {
		hash = s.dummyHash
	}
	match := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	if !match || !active {
		return "", ErrCredentials
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// Serialize login with password/role mutations; recheck the hash after bcrypt.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, mutationLock); err != nil {
		return "", err
	}
	var freshHash string
	if err = tx.QueryRowContext(ctx, `SELECT password_hash,active FROM app.users WHERE id=$1`, id).Scan(&freshHash, &active); err != nil {
		return "", err
	}
	if !active || freshHash != hash {
		return "", ErrCredentials
	}
	if oldToken != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE app.sessions SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL`, TokenHash(oldToken)); err != nil {
			return "", err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '8 hours')`, TokenHash(token), id); err != nil {
		return "", err
	}
	if err = record(ctx, tx, &id, id, "login", nil, nil); err != nil {
		return "", err
	}
	return token, tx.Commit()
}
func (s *Store) Logout(ctx context.Context, token string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `UPDATE app.sessions SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL RETURNING user_id`, TokenHash(token)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = record(ctx, tx, &id, id, "logout", nil, nil); err != nil {
		return err
	}
	return tx.Commit()
}
func record(ctx context.Context, tx *sql.Tx, actor *int64, target int64, action string, before, after *User) error {
	var b, a any
	if before != nil {
		raw, err := json.Marshal(before)
		if err != nil {
			return err
		}
		b = string(raw)
	}
	if after != nil {
		raw, err := json.Marshal(after)
		if err != nil {
			return err
		}
		a = string(raw)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO app.access_audit(actor_id,target_id,action,before_state,after_state) VALUES($1,$2,$3,$4,$5)`, actor, target, action, b, a)
	return err
}
func (s *Store) beginMutation(ctx context.Context, actor int64, admin bool) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, mutationLock); err != nil {
		tx.Rollback()
		return nil, err
	}
	if actor != 0 {
		u, err := getUser(ctx, tx, actor)
		if err != nil || !u.Active || (admin && !u.HasRole("admin")) {
			tx.Rollback()
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}
			return nil, ErrForbidden
		}
	}
	return tx, nil
}
func dbError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrConflict
	}
	return err
}
func setRoles(ctx context.Context, tx *sql.Tx, id int64, roles []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM app.user_roles WHERE user_id=$1`, id); err != nil {
		return err
	}
	for _, code := range roles {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app.user_roles(user_id,role_code) VALUES($1,$2)`, id, code); err != nil {
			return err
		}
	}
	return nil
}

// Bootstrap only succeeds on an empty account store; never upgrades an existing account.
func (s *Store) Bootstrap(ctx context.Context, in Input) (User, error) {
	return s.create(ctx, 0, in, true)
}
func (s *Store) Create(ctx context.Context, actor int64, in Input) (User, error) {
	if actor == 0 {
		return User{}, ErrForbidden
	}
	return s.create(ctx, actor, in, false)
}
func (s *Store) create(ctx context.Context, actor int64, in Input, bootstrap bool) (User, error) {
	if bootstrap {
		in.Roles = []string{"admin"}
		in.Active = true
	}
	if err := in.Validate(true); err != nil {
		return User{}, err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, err
	}
	tx, err := s.beginMutation(ctx, actor, true)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	if bootstrap {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.users`).Scan(&count); err != nil {
			return User{}, err
		}
		if count != 0 {
			return User{}, ValidationError("Bootstrap hanya boleh dilakukan saat belum ada akun.")
		}
	}
	var id int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.users(username,display_name,password_hash,active) VALUES($1,$2,$3,$4) RETURNING id`, in.Username, in.DisplayName, hash, in.Active).Scan(&id); err != nil {
		return User{}, dbError(err)
	}
	if err = setRoles(ctx, tx, id, in.Roles); err != nil {
		return User{}, err
	}
	u, err := getUser(ctx, tx, id)
	if err != nil {
		return User{}, err
	}
	var actorID *int64
	action := "user.create"
	if actor != 0 {
		actorID = &actor
	} else {
		action = "admin.bootstrap"
	}
	if err = record(ctx, tx, actorID, id, action, nil, &u); err != nil {
		return User{}, err
	}
	return u, tx.Commit()
}
func (s *Store) Update(ctx context.Context, actor, id int64, in Input) (User, error) {
	if actor == 0 {
		return User{}, ErrForbidden
	}
	if err := in.Validate(false); err != nil {
		return User{}, err
	}
	var hash string
	var err error
	if in.Password != "" {
		hash, err = HashPassword(in.Password)
		if err != nil {
			return User{}, err
		}
	}
	tx, err := s.beginMutation(ctx, actor, true)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	before, err := getUser(ctx, tx, id)
	if err != nil {
		return User{}, err
	}
	if before.Version != in.Version {
		return User{}, ErrConflict
	}
	if before.Active && before.HasRole("admin") && (!in.Active || !containsAdmin(in.Roles)) {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.users u JOIN app.user_roles r ON u.id=r.user_id WHERE u.active AND r.role_code='admin'`).Scan(&count); err != nil {
			return User{}, err
		}
		if count <= 1 {
			return User{}, ErrLastAdmin
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE app.users SET username=$1,display_name=$2,active=$3,password_hash=CASE WHEN $4='' THEN password_hash ELSE $4 END,version=version+1,updated_at=now() WHERE id=$5`, in.Username, in.DisplayName, in.Active, hash, id)
	if err != nil {
		return User{}, dbError(err)
	}
	if err = setRoles(ctx, tx, id, in.Roles); err != nil {
		return User{}, err
	}
	if !in.Active || hash != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE app.sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
			return User{}, err
		}
	}
	after, err := getUser(ctx, tx, id)
	if err != nil {
		return User{}, err
	}
	action := "user.update"
	if hash != "" {
		action = "user.update_password_reset"
	}
	if err = record(ctx, tx, &actor, id, action, &before, &after); err != nil {
		return User{}, err
	}
	return after, tx.Commit()
}
func containsAdmin(roles []string) bool {
	for _, r := range roles {
		if r == "admin" {
			return true
		}
	}
	return false
}
func (s *Store) ChangePassword(ctx context.Context, id int64, current, next string) error {
	if id < 1 {
		return ErrForbidden
	}
	if len(current) > 72 {
		return ErrCredentials
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	tx, err := s.beginMutation(ctx, id, false)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old string
	if err = tx.QueryRowContext(ctx, `SELECT password_hash FROM app.users WHERE id=$1`, id).Scan(&old); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(old), []byte(current)) != nil {
		return ErrCredentials
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.users SET password_hash=$1,version=version+1,updated_at=now() WHERE id=$2`, hash, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return err
	}
	if err = record(ctx, tx, &id, id, "password.change", nil, nil); err != nil {
		return err
	}
	return tx.Commit()
}

type Filter struct {
	Search, Role, Status string
	Page                 int
}

func (s *Store) List(ctx context.Context, f Filter) ([]User, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Page > 10000 {
		return nil, ValidationError("Halaman di luar jangkauan.")
	}
	rows, err := s.DB.QueryContext(ctx, userSelect+`WHERE ($1='' OR u.username ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%') AND ($2='' OR EXISTS(SELECT 1 FROM app.user_roles WHERE user_id=u.id AND role_code=$2)) AND ($3='' OR u.active=($3='active')) ORDER BY u.id LIMIT 50 OFFSET $4`, f.Search, f.Role, f.Status, (f.Page-1)*50)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

type Audit struct {
	ID                                   int64
	Actor, Target, Action, Before, After string
	At                                   time.Time
}

func (s *Store) Audits(ctx context.Context) ([]Audit, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id,COALESCE(actor.username,'Bootstrap CLI'),target.username,a.action,COALESCE(a.before_state::text,''),COALESCE(a.after_state::text,''),a.created_at FROM app.access_audit a LEFT JOIN app.users actor ON actor.id=a.actor_id JOIN app.users target ON target.id=a.target_id ORDER BY a.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Audit
	for rows.Next() {
		var a Audit
		if err = rows.Scan(&a.ID, &a.Actor, &a.Target, &a.Action, &a.Before, &a.After, &a.At); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
