package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

const (
	teacherPhotoRetentionInterval = 24 * time.Hour
	teacherPhotoRetentionTimeout  = time.Minute
)

type teacherPhotoPurger interface {
	PurgeExpiredTeacherAttendancePhotos(context.Context, int) (academic.TeacherAttendancePhotoPurgeResult, error)
}

// runTeacherPhotoRetention menjalankan satu batch sinkron per siklus agar tidak overlap.
// Kanal tick dipisahkan dari loop agar pengujian tidak menunggu interval produksi.
func runTeacherPhotoRetention(ctx context.Context, purger teacherPhotoPurger, ticks <-chan time.Time, logger *slog.Logger) {
	cycle := func() {
		cycleCtx, cancel := context.WithTimeout(ctx, teacherPhotoRetentionTimeout)
		defer cancel()
		result, err := purger.PurgeExpiredTeacherAttendancePhotos(cycleCtx, academic.MaxTeacherAttendancePhotoPurgeBatch)
		attributes := []any{"examined", result.Examined, "purged", result.Purged, "failed", result.Failed}
		if err != nil {
			category := "purge_failed"
			switch {
			case errors.Is(err, context.Canceled):
				category = "cancelled"
			case errors.Is(err, context.DeadlineExceeded):
				category = "timeout"
			case errors.Is(err, academic.ErrTeacherAttendanceCommit):
				category = "commit_unconfirmed"
			}
			logger.Warn("pembersihan foto guru belum tuntas", append(attributes, "category", category)...)
			return
		}
		logger.Info("pembersihan foto guru selesai", attributes...)
	}
	if ctx.Err() != nil {
		return
	}
	cycle()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok || ctx.Err() != nil {
				return
			}
			cycle()
		}
	}
}
