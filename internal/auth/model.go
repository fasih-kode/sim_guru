package auth

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrCredentials = errors.New("Nama pengguna atau kata sandi tidak sesuai.")
	ErrSession     = errors.New("Sesi tidak berlaku. Silakan masuk kembali.")
	ErrForbidden   = errors.New("Anda tidak memiliki hak untuk tindakan ini.")
	ErrConflict    = errors.New("Nama pengguna sudah dipakai atau data telah berubah. Muat ulang dan periksa kembali.")
	ErrLastAdmin   = errors.New("Minimal satu akun Admin aktif harus dipertahankan.")
	ErrNotFound    = errors.New("Akun tidak ditemukan.")
)

type ValidationError string

func (e ValidationError) Error() string { return string(e) }

type Role struct{ Code, Name string }

var Roles = []Role{{"admin", "Admin"}, {"kurikulum", "Kurikulum"}, {"guru_mapel", "Guru Mapel"}, {"wali_kelas", "Wali Kelas"}, {"kepala_sekolah", "Kepala Sekolah"}, {"bk", "BK"}, {"kesiswaan", "Kesiswaan"}, {"petugas_absensi", "Petugas Absensi"}}

type User struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Active      bool     `json:"active"`
	Version     int64    `json:"version"`
	Roles       []string `json:"roles"`
}

func (u User) HasRole(role string) bool { return slices.Contains(u.Roles, role) }
func (u User) RoleNames() string {
	var names []string
	for _, r := range Roles {
		if u.HasRole(r.Code) {
			names = append(names, r.Name)
		}
	}
	return strings.Join(names, ", ")
}

type Input struct {
	Username, DisplayName, Password string
	Active                          bool
	Roles                           []string
	Version                         int64
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

func NormalizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func (in *Input) Validate(creating bool) error {
	in.Username = NormalizeUsername(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if !usernamePattern.MatchString(in.Username) {
		return ValidationError("Nama pengguna harus 3–64 karakter: huruf kecil, angka, titik, garis bawah, atau tanda hubung.")
	}
	if !utf8.ValidString(in.DisplayName) || utf8.RuneCountInString(in.DisplayName) < 1 || utf8.RuneCountInString(in.DisplayName) > 120 {
		return ValidationError("Nama tampilan wajib diisi, maksimal 120 karakter.")
	}
	if creating || in.Password != "" {
		if err := ValidatePassword(in.Password); err != nil {
			return err
		}
	}
	if len(in.Roles) == 0 {
		return ValidationError("Pilih minimal satu role.")
	}
	seen := map[string]bool{}
	for _, code := range in.Roles {
		valid := false
		for _, r := range Roles {
			if r.Code == code {
				valid = true
			}
		}
		if !valid || seen[code] {
			return ValidationError("Pilihan role tidak valid.")
		}
		seen[code] = true
	}
	slices.Sort(in.Roles)
	return nil
}
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || len(password) > 72 {
		return ValidationError("Kata sandi minimal 12 karakter dan maksimal 72 byte UTF-8.")
	}
	return nil
}
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash), err
}
