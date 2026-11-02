package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

func serve(ctx context.Context, server *http.Server, purger any, logger *slog.Logger) error {
	if ctx.Err() != nil {
		return nil
	}
	// Listen harus berhasil sebelum worker dimulai; startup gagal tidak membuat goroutine worker.
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return errors.New("server gagal membuka listener HTTP")
	}
	defer listener.Close()
	workerCtx, cancelWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	errs := make(chan error, 1)
	go func() { errs <- server.Serve(listener) }()
	logger.Info("server dimulai", "address", server.Addr)
	if purger == nil {
		logger.Info("worker retensi foto kehadiran nonaktif")
		close(workerDone)
	} else {
		logger.Info("worker retensi foto kehadiran aktif")
		ticker := time.NewTicker(teacherPhotoRetentionInterval)
		go func() {
			defer close(workerDone)
			defer ticker.Stop()
			runTeacherPhotoRetention(workerCtx, purger, ticker.C, logger)
		}()
	}
	// Pemanggil baru boleh menutup storage dan DB setelah worker benar-benar selesai.
	defer func() {
		cancelWorker()
		_ = server.Close()
		<-workerDone
	}()
	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("layanan HTTP berhenti karena kesalahan server")
		}
		return nil
	case <-ctx.Done():
		cancelWorker()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			_ = server.Close()
		}
		<-errs
		if err != nil {
			return errors.New("batas waktu shutdown HTTP terlampaui")
		}
		return nil
	}
}
