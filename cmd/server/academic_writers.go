package main

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Koneksi penulis tidak dipakai handler login, sesi, atau query akademik rutin.
func openAcademicWriters(ctx context.Context, cfg config.Config, store *academic.Store) (func(), error) {
	base, err := pgx.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, errors.New("konfigurasi koneksi utama tidak valid")
	}
	var oid uint32
	var runtime string
	if err := store.DB.QueryRowContext(ctx, `SELECT oid, current_user FROM pg_catalog.pg_database
		WHERE datname=pg_catalog.current_database()`).Scan(&oid, &runtime); err != nil || runtime != "sim_guru_app" {
		return nil, errors.New("koneksi utama harus memakai role runtime akademik")
	}
	specs := []struct {
		env, role string
		dest      **sql.DB
	}{
		{"ACADEMIC_CALENDAR_DATABASE_URL", "ux02e_calendar_writer", &store.CalendarWriter},
		{"ACADEMIC_PLACEMENT_DATABASE_URL", "ux02e_placement_writer", &store.PlacementWriter},
		{"ACADEMIC_SCORE_DATABASE_URL", "ux02e_tp_score_writer", &store.ScoreWriter},
		{"ACADEMIC_S06_DATABASE_URL", "ux02e_s06_writer", &store.ApplicabilityWriter},
	}
	closeWriters := func() {
		for _, s := range specs {
			if db := *s.dest; db != nil {
				db.Close()
			}
		}
	}
	fail := func(env string) (func(), error) {
		closeWriters()
		return nil, errors.New("koneksi penulis akademik tidak sesuai: " + env)
	}
	for _, s := range specs {
		raw := os.Getenv(s.env)
		writer, err := pgx.ParseConfig(raw)
		if raw == "" || err != nil || writer.Host != base.Host || writer.Port != base.Port ||
			writer.Database != base.Database || writer.User != s.role ||
			(cfg.Environment == "production" && (writer.TLSConfig == nil || writer.TLSConfig.InsecureSkipVerify)) {
			return fail(s.env)
		}
		for _, f := range writer.Fallbacks {
			if f.Host != base.Host || f.Port != base.Port || (cfg.Environment == "production" && (f.TLSConfig == nil || f.TLSConfig.InsecureSkipVerify)) {
				return fail(s.env)
			}
		}
		role := s.role
		*s.dest = stdlib.OpenDB(*writer, stdlib.OptionAfterConnect(func(ctx context.Context, c *pgx.Conn) error {
			var matches bool
			err := c.QueryRow(ctx, `SELECT oid=$1 AND session_user=$2 AND current_user=$2
				FROM pg_catalog.pg_database WHERE datname=pg_catalog.current_database()`, oid, role).Scan(&matches)
			if err != nil || !matches {
				return errors.New("identitas penulis akademik tidak sesuai")
			}
			return nil
		}))
		(*s.dest).SetMaxOpenConns(5)
		if err := (*s.dest).PingContext(ctx); err != nil {
			return fail(s.env)
		}
	}
	return closeWriters, nil
}
