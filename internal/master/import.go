package master

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

func importKind(kind Kind) error {
	if kind != Students && kind != Teachers {
		return ValidationError("Impor hanya tersedia untuk siswa dan guru.")
	}
	return nil
}
func importKeys(kind Kind, r Record) []string {
	var keys []string
	if kind == Students {
		if r.NIS != "" {
			keys = append(keys, "nis:"+r.NIS)
		}
		if r.NISN != "" {
			keys = append(keys, "nisn:"+r.NISN)
		}
		keys = append(keys, "identity:"+strings.ToLower(r.Name)+"|"+r.BirthDate+"|"+r.Gender)
	} else {
		if r.NIP != "" {
			keys = append(keys, "nip:"+r.NIP)
		}
		if r.NUPTK != "" {
			keys = append(keys, "nuptk:"+r.NUPTK)
		}
		if r.NIP == "" && r.NUPTK == "" {
			keys = append(keys, "identity:"+strings.ToLower(r.Name)+"|"+strings.ToLower(r.Email)+"|"+r.Phone)
		}
	}
	return keys
}
func existingImportRow(ctx context.Context, tx *sql.Tx, kind Kind, r Record) (bool, error) {
	var exists bool
	var err error
	if kind == Students {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.students WHERE ($1<>'' AND nis=$1) OR ($2<>'' AND nisn=$2) OR (lower(btrim(name))=lower($3) AND birth_date=$4 AND gender=$5))`, r.NIS, r.NISN, r.Name, r.BirthDate, r.Gender).Scan(&exists)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.teachers WHERE ($1<>'' AND nip=$1) OR ($2<>'' AND nuptk=$2) OR ($1='' AND $2='' AND lower(btrim(name))=lower($3) AND lower(email)=lower($4) AND phone=$5))`, r.NIP, r.NUPTK, r.Name, r.Email, r.Phone).Scan(&exists)
	}
	return exists, err
}
func validateImport(ctx context.Context, tx *sql.Tx, kind Kind, rows []ImportRow) (ImportReport, error) {
	report := ImportReport{Rows: rows}
	seen := map[string]int{}
	if len(rows) == 0 || len(rows) > MaxImportRows {
		return report, ValidationError("Impor harus berisi 1–500 baris.")
	}
	for i := range report.Rows {
		row := &report.Rows[i]
		if row.Row < 2 {
			return report, ValidationError("Nomor baris impor tidak valid.")
		}
		if err := row.Input.Validate(kind, true); err != nil {
			if len(row.Errors) == 0 {
				row.Errors = append(row.Errors, err.Error())
			}
		}
		// Uploads can never assign accounts, mutate existing records, or change roles.
		row.Input.ID = 0
		row.Input.Version = 0
		row.Input.UserID = 0
		if len(row.Errors) > 0 {
			continue
		}
		for _, key := range importKeys(kind, row.Input.Record) {
			if previous, exists := seen[key]; exists {
				addImportError(row, fmt.Sprintf("Potensi duplikat dengan baris %d dalam file.", report.Rows[previous].Row))
				addImportError(&report.Rows[previous], fmt.Sprintf("Potensi duplikat dengan baris %d dalam file.", row.Row))
			} else {
				seen[key] = i
			}
		}
		exists, err := existingImportRow(ctx, tx, kind, row.Input.Record)
		if err != nil {
			return report, err
		}
		if exists {
			row.Errors = append(row.Errors, "Potensi duplikat dengan data tersimpan. Periksa data induk sebelum menambahkan.")
		}
	}
	for _, row := range report.Rows {
		if len(row.Errors) > 0 {
			report.ErrorCount++
		}
	}
	return report, nil
}
func (s *Store) PreviewImport(ctx context.Context, actor int64, kind Kind, rows []ImportRow) (ImportReport, error) {
	if err := importKind(kind); err != nil {
		return ImportReport{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ImportReport{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return ImportReport{}, err
	}
	if err = authorize(ctx, tx, actor, kind, true); err != nil {
		return ImportReport{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.master_imports SET state='expired',payload='[]'::jsonb WHERE state='pending' AND expires_at<=now()`); err != nil {
		return ImportReport{}, err
	}
	report, err := validateImport(ctx, tx, kind, rows)
	if err != nil {
		return report, err
	}
	if report.ErrorCount > 0 {
		return report, tx.Commit()
	}
	var pending int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.master_imports WHERE owner_id=$1 AND state='pending'`, actor).Scan(&pending); err != nil {
		return report, err
	}
	if pending >= 5 {
		return report, ValidationError("Maksimal 5 pratinjau aktif. Selesaikan impor atau tunggu 15 menit sebelum mengunggah lagi.")
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return report, err
	}
	report.Token = hex.EncodeToString(token)
	payload, err := json.Marshal(report.Rows)
	if err != nil {
		return report, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.master_imports(token_hash,owner_id,kind,payload,row_count) VALUES($1,$2,$3,$4,$5)`, auth.TokenHash(report.Token), actor, kind, string(payload), len(rows))
	if err != nil {
		return report, err
	}
	return report, tx.Commit()
}

type ImportResult struct {
	Count           int
	AlreadyComplete bool
}

func (s *Store) ConfirmImport(ctx context.Context, actor int64, kind Kind, token string) (ImportResult, error) {
	var result ImportResult
	if err := importKind(kind); err != nil {
		return result, err
	}
	if len(token) != 64 {
		return result, ValidationError("Pratinjau tidak sah. Unggah ulang file.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return result, err
	}
	if err = authorize(ctx, tx, actor, kind, true); err != nil {
		return result, err
	}
	var payload []byte
	var state string
	var expiry time.Time
	err = tx.QueryRowContext(ctx, `SELECT payload,state,row_count,expires_at FROM app.master_imports WHERE token_hash=$1 AND owner_id=$2 AND kind=$3 FOR UPDATE`, auth.TokenHash(token), actor, kind).Scan(&payload, &state, &result.Count, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ValidationError("Pratinjau tidak ditemukan untuk akun ini. Unggah ulang file.")
	}
	if err != nil {
		return result, err
	}
	if state == "complete" {
		result.AlreadyComplete = true
		return result, tx.Commit()
	}
	if state != "pending" || !time.Now().Before(expiry) {
		if _, err = tx.ExecContext(ctx, `UPDATE app.master_imports SET state='expired',payload='[]'::jsonb WHERE token_hash=$1`, auth.TokenHash(token)); err != nil {
			return result, err
		}
		if err = tx.Commit(); err != nil {
			return result, err
		}
		return result, ValidationError("Pratinjau kedaluwarsa setelah 15 menit. Unggah ulang file.")
	}
	var rows []ImportRow
	if json.Unmarshal(payload, &rows) != nil {
		return result, ValidationError("Data pratinjau tidak valid. Unggah ulang file.")
	}
	if len(rows) != result.Count {
		return result, ValidationError("Jumlah baris pratinjau tidak konsisten. Unggah ulang file.")
	}
	report, err := validateImport(ctx, tx, kind, rows)
	if err != nil {
		return result, err
	}
	for _, row := range report.Rows {
		if len(row.Errors) > 0 {
			return result, importConflict(row.Row)
		}
	}
	for _, row := range report.Rows {
		in := row.Input
		in.Reason = "Impor Excel"
		if _, err = saveRecord(ctx, tx, actor, kind, 0, in); err != nil {
			if errors.Is(err, ErrConflict) {
				return result, importConflict(row.Row)
			}
			return result, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.master_imports SET state='complete',completed_at=now(),payload='[]'::jsonb WHERE token_hash=$1`, auth.TokenHash(token)); err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func addImportError(row *ImportRow, message string) {
	if !slices.Contains(row.Errors, message) {
		row.Errors = append(row.Errors, message)
	}
}
