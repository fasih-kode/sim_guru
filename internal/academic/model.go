// Package academic manages the school calendar and class catalog.
package academic

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrForbidden = errors.New("Anda tidak memiliki hak untuk mengelola data akademik ini.")
	ErrNotFound  = errors.New("Data akademik tidak ditemukan.")
	ErrConflict  = errors.New("Data telah berubah atau nama sudah digunakan. Muat ulang data terbaru.")
)

type ValidationError string

func (e ValidationError) Error() string { return string(e) }

type Year struct {
	ID      int64
	Name    string
	Active  bool
	Version int64
}

type Semester struct {
	ID           int64
	SchoolYearID int64
	YearName     string
	Number       int
	Active       bool
	YearActive   bool
	Version      int64
}

type Class struct {
	ID           int64
	SchoolYearID int64
	YearName     string
	Grade        int
	Name         string
	Version      int64
}

type History struct {
	ID                                       int64
	EntityType, Actor, Before, After, Reason string
	At                                       time.Time
}

type StudentOption struct {
	ID              int64
	Name, NIS, NISN string
	Active          bool
}

type Placement struct {
	StartsOn                             string
	ID, StudentID, SchoolYearID, ClassID int64
	StudentName, NIS, NISN               string
	YearName, ClassName                  string
	Grade                                int
	Actor                                string
	At                                   time.Time
}

type ClassPlacementCount struct {
	ClassID int64
	Count   int
}

type HomeroomTeacher struct {
	ID   int64
	Name string
}

type HomeroomAssignment struct {
	ClassID, TeacherID               int64
	YearName, ClassName, TeacherName string
	At                               time.Time
}

type HomeroomHistory struct {
	BeforeTeacher, AfterTeacher, Actor string
	At                                 time.Time
}

type ClassStudent struct {
	Name, NIS, NISN string
}

type PlacementHistory struct {
	ID, PlacementID int64
	StudentName     string
	FromClass       string
	ToClass         string
	Actor, Reason   string
	At              time.Time
}

func validateYear(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 40 || strings.ContainsRune(name, 0) {
		return "", ValidationError("Nama tahun ajaran wajib diisi, maksimal 40 karakter.")
	}
	return name, nil
}

func validateClass(yearID int64, grade int, name, reason string, creating bool) (string, string, error) {
	name, reason = strings.TrimSpace(name), strings.TrimSpace(reason)
	if yearID < 1 {
		return "", "", ValidationError("Pilih tahun ajaran.")
	}
	if grade < 7 || grade > 9 {
		return "", "", ValidationError("Tingkat kelas SMP harus 7, 8, atau 9.")
	}
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 40 || strings.ContainsRune(name, 0) {
		return "", "", ValidationError("Nama/rombongan kelas wajib diisi, maksimal 40 karakter.")
	}
	if !creating && (reason == "" || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 500 || strings.ContainsRune(reason, 0)) {
		return "", "", ValidationError("Alasan perubahan wajib diisi, maksimal 500 karakter.")
	}
	return name, reason, nil
}
