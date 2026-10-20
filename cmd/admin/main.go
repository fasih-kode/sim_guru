// Bootstrap a single initial Admin without placing a password in arguments or environment.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/config"
	"github.com/fasih-kode/sim_guru/internal/database"
	"golang.org/x/term"
)

func run() error {
	username := flag.String("username", "", "Nama pengguna Admin pertama")
	name := flag.String("name", "", "Nama tampilan Admin pertama")
	flag.Parse()
	if *username == "" || *name == "" || flag.NArg() != 0 {
		return errors.New("gunakan: go run ./cmd/admin -username NAMA -name 'Nama Admin'")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("bootstrap membutuhkan terminal interaktif; jangan kirim password melalui argumen atau pipe")
	}
	fmt.Print("Kata sandi (minimal 12 karakter): ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return errors.New("gagal membaca kata sandi")
	}
	defer clear(password)
	fmt.Print("Ulangi kata sandi: ")
	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return errors.New("gagal membaca konfirmasi")
	}
	defer clear(confirm)
	if string(password) != string(confirm) {
		return errors.New("konfirmasi tidak sesuai")
	}
	fmt.Print("Buat Admin pertama pada database yang dikonfigurasi? Ketik BUAT: ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil || strings.TrimSpace(answer) != "BUAT" {
		return errors.New("bootstrap dibatalkan")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	store, err := auth.New(db)
	if err != nil {
		return errors.New("inisialisasi autentikasi gagal")
	}
	_, err = store.Bootstrap(ctx, auth.Input{Username: *username, DisplayName: *name, Password: string(password)})
	if err != nil {
		var validation auth.ValidationError
		if errors.As(err, &validation) {
			return err
		}
		return errors.New("bootstrap gagal; periksa migrasi dan pastikan belum ada akun")
	}
	fmt.Println("Admin pertama berhasil dibuat. Silakan masuk melalui /login.")
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
