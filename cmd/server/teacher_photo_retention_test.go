package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type photoPurgeFunc func(context.Context, int) (academic.TeacherAttendancePhotoPurgeResult, error)

func (f photoPurgeFunc) PurgeExpiredTeacherAttendancePhotos(ctx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
	return f(ctx, limit)
}

type staffPhotoPurgeFunc func(context.Context, int) (academic.TeacherAttendancePhotoPurgeResult, error)

func (f staffPhotoPurgeFunc) PurgeExpiredStaffAttendancePhotos(ctx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
	return f(ctx, limit)
}

func waitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout menunggu worker")
	}
}

func TestTeacherPhotoRetentionCycles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time, 1)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	var calls, active, overlap atomic.Int32
	var log bytes.Buffer
	purger := photoPurgeFunc(func(cycleCtx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
		call := calls.Add(1)
		if active.Add(1) != 1 {
			overlap.Add(1)
		}
		defer active.Add(-1)
		if limit != 50 {
			t.Errorf("limit=%d", limit)
		}
		if deadline, ok := cycleCtx.Deadline(); !ok || time.Until(deadline) > teacherPhotoRetentionTimeout {
			t.Error("timeout siklus tidak dibatasi")
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-cycleCtx.Done():
			return academic.TeacherAttendancePhotoPurgeResult{}, cycleCtx.Err()
		}
		if call == 1 {
			return academic.TeacherAttendancePhotoPurgeResult{Examined: 3, Purged: 2, Failed: 1}, errors.New("sensitive-marker /private/photo storage-key")
		}
		return academic.TeacherAttendancePhotoPurgeResult{}, nil
	})
	go func() {
		defer close(done)
		runTeacherPhotoRetention(ctx, purger, ticks, slog.New(slog.NewJSONHandler(&log, nil)))
	}()
	waitSignal(t, entered)
	ticks <- time.Time{}
	release <- struct{}{}
	waitSignal(t, entered)
	cancel()
	waitSignal(t, done)
	if calls.Load() != 2 || active.Load() != 0 || overlap.Load() != 0 {
		t.Fatalf("calls=%d active=%d overlap=%d", calls.Load(), active.Load(), overlap.Load())
	}
	output := log.String()
	for _, expected := range []string{`"category":"purge_failed"`, `"examined":3`, `"purged":2`, `"failed":1`} {
		if !strings.Contains(output, expected) {
			t.Errorf("log tidak memuat %s", expected)
		}
	}
	for _, secret := range []string{"sensitive-marker", "/private/photo", "storage-key"} {
		if strings.Contains(output, secret) {
			t.Error("log membocorkan error mentah")
		}
	}
}

func TestTeacherPhotoRetentionCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	purger := photoPurgeFunc(func(context.Context, int) (academic.TeacherAttendancePhotoPurgeResult, error) {
		t.Fatal("purge dipanggil sesudah pembatalan")
		return academic.TeacherAttendancePhotoPurgeResult{}, nil
	})
	runTeacherPhotoRetention(ctx, purger, make(chan time.Time), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestStaffPhotoRetentionRunsWithoutTeacherStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	var log bytes.Buffer
	purger := staffPhotoPurgeFunc(func(cycleCtx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
		if limit != academic.MaxTeacherAttendancePhotoPurgeBatch {
			t.Errorf("limit=%d", limit)
		}
		if _, ok := cycleCtx.Deadline(); !ok {
			t.Error("retention timeout missing")
		}
		calls.Add(1)
		cancel()
		return academic.TeacherAttendancePhotoPurgeResult{Examined: 2, Purged: 1, Failed: 1}, nil
	})
	runTeacherPhotoRetention(ctx, purger, make(chan time.Time), slog.New(slog.NewJSONHandler(&log, nil)))
	if calls.Load() != 1 {
		t.Fatalf("staff purge calls=%d", calls.Load())
	}
	if !strings.Contains(log.String(), `"domain":"pegawai"`) || !strings.Contains(log.String(), `"purged":1`) {
		t.Fatal("staff retention result was not logged safely")
	}
}
