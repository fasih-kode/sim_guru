package main

import (
	"context"
	"errors"
	"github.com/fasih-kode/sim_guru/internal/config"
	"github.com/fasih-kode/sim_guru/internal/database"
	"github.com/fasih-kode/sim_guru/internal/web"
	"github.com/fasih-kode/sim_guru/migrations"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	provider, err := migrations.Provider(db)
	if err != nil {
		return errors.New("inisialisasi status migrasi gagal")
	}
	ready := func(ctx context.Context) error {
		if err := db.PingContext(ctx); err != nil {
			return err
		}
		current, target, err := provider.GetVersions(ctx)
		if err != nil || current != target {
			return errors.New("migrasi database belum sesuai")
		}
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = ready(checkCtx)
	cancel()
	if err != nil {
		return errors.New("database belum siap; jalankan migrasi dan periksa koneksi")
	}
	handler, err := web.New(ready)
	if err != nil {
		return errors.New("inisialisasi template gagal")
	}
	server := &http.Server{Addr: cfg.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	errs := make(chan error, 1)
	go func() {
		slog.Info("server dimulai", "address", cfg.Address, "environment", cfg.Environment)
		errs <- server.ListenAndServe()
	}()
	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
func main() {
	if err := run(); err != nil {
		slog.Error("server berhenti", "error", err.Error())
		os.Exit(1)
	}
}
