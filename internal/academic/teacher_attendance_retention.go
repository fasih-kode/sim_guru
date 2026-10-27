package academic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

const MaxTeacherAttendancePhotoPurgeBatch = 50

type TeacherAttendancePhotoPurgeResult struct {
	Examined int
	Purged   int
	Failed   int
}

// PurgeExpiredTeacherAttendancePhotos membersihkan bukti kedaluwarsa tanpa menghapus metadata audit.
// Operasi internal ini harus memakai instance Photos yang sama dengan service capture.
func (s *Store) PurgeExpiredTeacherAttendancePhotos(ctx context.Context, limit int) (TeacherAttendancePhotoPurgeResult, error) {
	var result TeacherAttendancePhotoPurgeResult
	if limit < 1 || limit > MaxTeacherAttendancePhotoPurgeBatch {
		return result, ValidationError("Batas pembersihan foto harus antara 1 dan 50.")
	}
	if s.Photos == nil {
		return result, ErrTeacherAttendancePhoto
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM app.teacher_attendance_photos
		WHERE deleted_at IS NULL AND retain_until<=clock_timestamp()
		ORDER BY cleanup_attempts,retain_until,id LIMIT $1`, limit)
	if err != nil {
		return result, ErrTeacherAttendanceWrite
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, ErrTeacherAttendanceWrite
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil || closeErr != nil {
		return result, ErrTeacherAttendanceWrite
	}

	var failures []error
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return result, errors.Join(append(failures, err)...)
		}
		result.Examined++
		purged, purgeErr := s.purgeTeacherAttendancePhoto(ctx, id)
		if purgeErr != nil {
			result.Failed++
			failures = append(failures, fmt.Errorf("foto %d: %w", id, purgeErr))
		} else if purged {
			result.Purged++
		}
	}
	return result, errors.Join(failures...)
}

func (s *Store) purgeTeacherAttendancePhoto(ctx context.Context, id int64) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, ErrTeacherAttendanceWrite
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auth.MutationLock); err != nil {
		return false, ErrTeacherAttendanceWrite
	}
	var key string
	var retainUntil, serverNow time.Time
	var deletedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT storage_key,retain_until,deleted_at,clock_timestamp()
		FROM app.teacher_attendance_photos WHERE id=$1 FOR UPDATE`, id).Scan(&key, &retainUntil, &deletedAt, &serverNow)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, ErrTeacherAttendanceWrite
	}
	// Hasil seleksi batch bisa basi; keputusan hapus selalu dibuat ulang di bawah mutation lock.
	if !teacherAttendancePhotoExpired(retainUntil, deletedAt, serverNow) {
		return false, nil
	}
	if err = s.Photos.Delete(key); err != nil {
		// Error storage mentah dapat memuat path privat; simpan hanya kategori tetap.
		if _, err = tx.ExecContext(ctx, `UPDATE app.teacher_attendance_photos
			SET cleanup_attempts=cleanup_attempts+1,last_cleanup_error='photo_delete_failed' WHERE id=$1`, id); err != nil {
			return false, ErrTeacherAttendanceWrite
		}
		if err = tx.Commit(); err != nil {
			return false, ErrTeacherAttendanceCommit
		}
		return false, ErrTeacherAttendanceCleanup
	}
	// File yang sudah hilang juga berhasil. Jika DB gagal sesudah ini, retry melengkapi metadata.
	if _, err = tx.ExecContext(ctx, `UPDATE app.teacher_attendance_photos
		SET deleted_at=clock_timestamp(),last_cleanup_error=NULL WHERE id=$1`, id); err != nil {
		return false, ErrTeacherAttendanceWrite
	}
	if err = tx.Commit(); err != nil {
		return false, ErrTeacherAttendanceCommit
	}
	return true, nil
}

func teacherAttendancePhotoExpired(retainUntil time.Time, deletedAt sql.NullTime, serverNow time.Time) bool {
	return !deletedAt.Valid && !retainUntil.After(serverNow)
}
