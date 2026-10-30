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

type staffPhotoPurger interface {
	PurgeExpiredStaffAttendancePhotos(context.Context, int) (academic.TeacherAttendancePhotoPurgeResult, error)
}

type teacherPhotoRetentionService struct{ store *academic.Store }

func (s teacherPhotoRetentionService) PurgeExpiredTeacherAttendancePhotos(ctx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
	return s.store.PurgeExpiredTeacherAttendancePhotos(ctx, limit)
}

type staffPhotoRetentionService struct{ store *academic.Store }

func (s staffPhotoRetentionService) PurgeExpiredStaffAttendancePhotos(ctx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
	return s.store.PurgeExpiredStaffAttendancePhotos(ctx, limit)
}

type allPhotoRetentionService struct{ store *academic.Store }

func (s allPhotoRetentionService) PurgeExpiredTeacherAttendancePhotos(ctx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
	return s.store.PurgeExpiredTeacherAttendancePhotos(ctx, limit)
}
func (s allPhotoRetentionService) PurgeExpiredStaffAttendancePhotos(ctx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
	return s.store.PurgeExpiredStaffAttendancePhotos(ctx, limit)
}

// runTeacherPhotoRetention menjalankan satu batch sinkron per siklus agar tidak overlap.
// Kanal tick dipisahkan dari loop agar pengujian tidak menunggu interval produksi.
func runTeacherPhotoRetention(ctx context.Context, purger any, ticks <-chan time.Time, logger *slog.Logger) {
	cycle := func() {
		cycleCtx, cancel := context.WithTimeout(ctx, teacherPhotoRetentionTimeout)
		defer cancel()
		if teacherPurger, ok := purger.(teacherPhotoPurger); ok {
			result, err := teacherPurger.PurgeExpiredTeacherAttendancePhotos(cycleCtx, academic.MaxTeacherAttendancePhotoPurgeBatch)
			logPhotoPurge(logger, "guru", result, err)
		}
		if staffPurger, ok := purger.(staffPhotoPurger); ok {
			staffResult, staffErr := staffPurger.PurgeExpiredStaffAttendancePhotos(cycleCtx, academic.MaxTeacherAttendancePhotoPurgeBatch)
			logPhotoPurge(logger, "pegawai", staffResult, staffErr)
		}
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

func logPhotoPurge(logger *slog.Logger, domain string, result academic.TeacherAttendancePhotoPurgeResult, err error) {
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
		logger.Warn("pembersihan foto kehadiran belum tuntas", append(attributes, "domain", domain, "category", category)...)
		return
	}
	logger.Info("pembersihan foto kehadiran selesai", append(attributes, "domain", domain)...)
}
