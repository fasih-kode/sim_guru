package main

import (
	"context"
	"fmt"
	"github.com/fasih-kode/sim_guru/internal/config"
	"github.com/fasih-kode/sim_guru/internal/database"
	"github.com/fasih-kode/sim_guru/migrations"
	"os"
	"time"
)

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "status") {
		return fmt.Errorf("gunakan: go run ./cmd/migrate up|status; rollback memerlukan rencana dan persetujuan eksplisit")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	provider, err := migrations.Provider(db)
	if err != nil {
		return fmt.Errorf("inisialisasi migrasi gagal")
	}
	if os.Args[1] == "up" {
		results, err := provider.Up(ctx)
		if err != nil {
			return fmt.Errorf("migrasi gagal; periksa status dan SQL tanpa membagikan kredensial")
		}
		fmt.Printf("Migrasi diterapkan: %d\n", len(results))
		return nil
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("status migrasi tidak dapat dibaca")
	}
	fmt.Printf("Versi database: %d; target: %d\n", current, target)
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
