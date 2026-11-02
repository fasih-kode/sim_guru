package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/config"
	"github.com/fasih-kode/sim_guru/internal/database"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/web"
	"github.com/fasih-kode/sim_guru/migrations"
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
	photos, err := openTeacherPhotoStorage(cfg)
	if err != nil {
		return err
	}
	slog.Info("storage foto kehadiran guru", "active", photos != nil)
	if photos != nil {
		defer photos.Close()
	}
	staffPhotos, err := openStaffPhotoStorage(cfg)
	if err != nil {
		return err
	}
	if staffPhotos != nil {
		defer staffPhotos.Close()
	}
	slog.Info("storage foto kehadiran pegawai", "active", staffPhotos != nil)
	academics := &academic.Store{DB: db}
	if photos != nil {
		academics.Photos = photos
	}
	academics.StaffPhotos = staffPhotos
	var purge any
	switch {
	case photos != nil && staffPhotos != nil:
		purge = allPhotoRetentionService{store: academics}
	case photos != nil:
		purge = teacherPhotoRetentionService{store: academics}
	case staffPhotos != nil:
		purge = staffPhotoRetentionService{store: academics}
	}
	accounts, err := auth.New(db)
	if err != nil {
		return errors.New("inisialisasi autentikasi gagal")
	}
	handler, err := web.New(ready, web.Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: academics, CSRFKey: cfg.CSRFKey, SecureCookies: cfg.Environment == "production", TrustedProxyCIDRs: cfg.TrustedProxyCIDRs})
	if err != nil {
		return errors.New("inisialisasi template gagal")
	}
	server := &http.Server{Addr: cfg.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	return serve(ctx, server, purge, slog.Default().With("environment", cfg.Environment))
}
func main() {
	if err := run(); err != nil {
		slog.Error("server berhenti", "error", err.Error())
		os.Exit(1)
	}
}
