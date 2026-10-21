// Package master manages permanent student, teacher, and subject identities.
package master

import (
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Kind string

const (
	Students Kind = "students"
	Teachers Kind = "teachers"
	Subjects Kind = "subjects"
)

func (k Kind) Valid() bool { return k == Students || k == Teachers || k == Subjects }
func (k Kind) Label() string {
	switch k {
	case Students:
		return "Siswa"
	case Teachers:
		return "Guru"
	case Subjects:
		return "Mata Pelajaran"
	}
	return ""
}
func (k Kind) WriterRole() string {
	if k == Subjects {
		return "kurikulum"
	}
	return "admin"
}
func (k Kind) HistoryTable() string {
	switch k {
	case Students:
		return "app.student_history"
	case Teachers:
		return "app.teacher_history"
	case Subjects:
		return "app.subject_history"
	}
	return ""
}

var (
	ErrForbidden = errors.New("Anda tidak memiliki hak untuk mengakses data ini.")
	ErrNotFound  = errors.New("Data tidak ditemukan.")
	ErrConflict  = errors.New("Identitas/kode sudah digunakan atau data telah berubah. Periksa duplikat dan muat ulang data terbaru.")
)

type ValidationError string

func (e ValidationError) Error() string { return string(e) }

type Record struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BirthDate string `json:"birth_date,omitempty"`
	Gender    string `json:"gender,omitempty"`
	NIS       string `json:"nis,omitempty"`
	NISN      string `json:"nisn,omitempty"`
	NIP       string `json:"nip,omitempty"`
	NUPTK     string `json:"nuptk,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
	UserID    int64  `json:"user_id,omitempty"`
	Code      string `json:"code,omitempty"`
	Active    bool   `json:"active"`
	Version   int64  `json:"version"`
}
type Input struct {
	Record
	Reason string
}

func bounded(s string, n int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= n && !strings.ContainsRune(s, 0)
}
func (in *Input) Validate(kind Kind, creating bool) error {
	if !kind.Valid() {
		return ValidationError("Jenis master data tidak valid.")
	}
	in.Name = strings.TrimSpace(in.Name)
	in.NIS = strings.TrimSpace(in.NIS)
	in.NISN = strings.TrimSpace(in.NISN)
	in.NIP = strings.TrimSpace(in.NIP)
	in.NUPTK = strings.TrimSpace(in.NUPTK)
	in.Email = strings.TrimSpace(in.Email)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Name == "" || !bounded(in.Name, 200) {
		return ValidationError("Nama wajib diisi, maksimal 200 karakter.")
	}
	if !bounded(in.Phone, 40) {
		return ValidationError("Telepon maksimal 40 karakter.")
	}
	if !creating && (in.Reason == "" || !bounded(in.Reason, 500)) {
		return ValidationError("Alasan perubahan wajib diisi, maksimal 500 karakter.")
	}
	switch kind {
	case Students:
		date, err := time.Parse("2006-01-02", in.BirthDate)
		today := time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02")
		if err != nil || date.Year() < 1 || in.BirthDate > today {
			return ValidationError("Tanggal lahir wajib berupa tanggal sah dan tidak di masa depan.")
		}
		if in.Gender != "L" && in.Gender != "P" {
			return ValidationError("Pilih jenis kelamin siswa.")
		}
		if !bounded(in.NIS, 64) || !bounded(in.NISN, 64) {
			return ValidationError("NIS dan NISN maksimal 64 karakter; boleh dikosongkan.")
		}
	case Teachers:
		if !bounded(in.NIP, 64) || !bounded(in.NUPTK, 64) {
			return ValidationError("NIP dan NUPTK maksimal 64 karakter.")
		}
		if !bounded(in.Email, 254) {
			return ValidationError("Email maksimal 254 karakter.")
		}
		if in.Email != "" {
			address, err := mail.ParseAddress(in.Email)
			if err != nil || address.Address != in.Email {
				return ValidationError("Format email tidak valid.")
			}
		}
		if in.UserID < 0 {
			return ValidationError("Akun terkait tidak valid.")
		}
	case Subjects:
		if in.Code == "" || !bounded(in.Code, 64) || strings.IndexFunc(in.Code, unicode.IsControl) >= 0 {
			return ValidationError("Kode mapel wajib diisi, maksimal 64 karakter.")
		}
	}
	return nil
}

type Filter struct {
	Search, Status string
	Page           int
}

func (f *Filter) Validate() error {
	f.Search = strings.TrimSpace(f.Search)
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Page > 10000 || !bounded(f.Search, 120) || (f.Status != "" && f.Status != "active" && f.Status != "inactive") {
		return ValidationError("Filter atau halaman tidak valid.")
	}
	return nil
}

type History struct {
	ID                           int64
	Actor, Before, After, Reason string
	At                           time.Time
}
type Account struct {
	ID                    int64
	Username, DisplayName string
	Active                bool
}
