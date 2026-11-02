// Package studentaffairs menyediakan domain operasional Kesiswaan.
// Catatan BK sengaja tidak dimasukkan ke package ini.
package studentaffairs

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	GuidanceTextMaxRunes   = 2000
	GuidanceReasonMaxRunes = 500
)

const (
	GuidanceRoleKesiswaan  = "kesiswaan"
	GuidanceRoleHeadmaster = "kepala_sekolah"
	GuidanceRoleHomeroom   = "wali_kelas"
)

var ErrGuidanceConflict = errors.New("Catatan pembinaan telah berubah. Muat ulang sebelum menyimpan perubahan.")
var ErrGuidanceForbidden = errors.New("Akses catatan pembinaan ditolak.")
var ErrGuidanceNotFound = errors.New("Catatan pembinaan tidak ditemukan.")
var ErrGuidancePlacementUnclear = errors.New("Penempatan siswa pada tanggal kejadian belum dapat dipastikan.")

type GuidanceStatus string

const (
	GuidanceNeedsFollowUp GuidanceStatus = "perlu_tindak_lanjut"
	GuidanceCompleted     GuidanceStatus = "selesai"
)

func (s GuidanceStatus) Valid() bool {
	return s == GuidanceNeedsFollowUp || s == GuidanceCompleted
}

func (s GuidanceStatus) Label() string {
	switch s {
	case GuidanceNeedsFollowUp:
		return "Perlu tindak lanjut"
	case GuidanceCompleted:
		return "Selesai"
	default:
		return ""
	}
}

// GuidanceInput hanya menerima ID siswa resmi dan isi catatan.
// Nama/kelas historis dan ID petugas harus disnapshot oleh service dari data
// master, penempatan pada tanggal kejadian, serta sesi akun; form tidak dipercaya.
type GuidanceInput struct {
	StudentID int64
	Date      string
	Incident  string
	Action    string
	Status    GuidanceStatus
}

func (in *GuidanceInput) Validate() error {
	if in.StudentID < 1 {
		return ValidationError("Pilih siswa dari data master.")
	}
	date, err := time.Parse("2006-01-02", in.Date)
	if err != nil || date.Year() < 1 || date.Format("2006-01-02") != in.Date {
		return ValidationError("Tanggal kejadian tidak valid.")
	}
	var textErr error
	in.Incident, textErr = requiredText(in.Incident, "Jelaskan kejadian atau pelanggaran (maksimal 2.000 karakter).", GuidanceTextMaxRunes)
	if textErr != nil {
		return textErr
	}
	in.Action, textErr = requiredText(in.Action, "Catat tindakan pembinaan (maksimal 2.000 karakter).", GuidanceTextMaxRunes)
	if textErr != nil {
		return textErr
	}
	if !in.Status.Valid() {
		return ValidationError("Pilih Perlu tindak lanjut atau Selesai.")
	}
	return nil
}

func requiredText(value, message string, maxRunes int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) > maxRunes {
		return "", ValidationError(message)
	}
	return value, nil
}

type GuidanceStatusChange struct {
	Status GuidanceStatus
	Reason string
}

// GuidanceContentChange hanya mengubah uraian dan tindakan; identitas,
// tanggal, serta snapshot konteks awal tidak dapat diganti setelah dicatat.
type GuidanceContentChange struct {
	Incident string
	Action   string
	Reason   string
}

func (in *GuidanceContentChange) Validate() error {
	var err error
	in.Incident, err = requiredText(in.Incident, "Jelaskan kejadian atau pelanggaran (maksimal 2.000 karakter).", GuidanceTextMaxRunes)
	if err != nil {
		return err
	}
	in.Action, err = requiredText(in.Action, "Catat tindakan pembinaan (maksimal 2.000 karakter).", GuidanceTextMaxRunes)
	if err != nil {
		return err
	}
	in.Reason, err = requiredText(in.Reason, "Alasan perubahan wajib diisi, maksimal 500 karakter.", GuidanceReasonMaxRunes)
	return err
}

func (in *GuidanceStatusChange) Validate(current GuidanceStatus) error {
	if !current.Valid() || !in.Status.Valid() {
		return ValidationError("Pilih Perlu tindak lanjut atau Selesai.")
	}
	if current == in.Status {
		return ValidationError("Pilih status yang berbeda untuk mencatat perubahan.")
	}
	var err error
	in.Reason, err = requiredText(in.Reason, "Alasan perubahan status wajib diisi, maksimal 500 karakter.", GuidanceReasonMaxRunes)
	return err
}

type GuidanceRecord struct {
	ID, StudentID, RecordedByID, Version int64
	RecordedByName                       string
	ClassIDAtEvent                       int64
	StudentNameAtEvent, ClassNameAtEvent string
	Date, Incident, Action               string
	Status                               GuidanceStatus
	RecordedAt, UpdatedAt                time.Time
}

type GuidanceStudentOption struct {
	ID        int64
	Name      string
	ClassName string
}

type GuidanceSnapshot struct {
	StudentID, ClassIDAtEvent            int64
	StudentNameAtEvent, ClassNameAtEvent string
	Date, Incident, Action               string
	Status                               GuidanceStatus
}

type GuidanceChange string

const (
	GuidanceRecorded       GuidanceChange = "recorded"
	GuidanceContentChanged GuidanceChange = "content_changed"
	GuidanceStatusChanged  GuidanceChange = "status_changed"
)

// GuidanceHistoryEntry adalah data append-only: perubahan menyimpan actor
// autentikasi serta keadaan sebelum/sesudah, bukan menimpa histori terdahulu.
type GuidanceHistoryEntry struct {
	ID, RecordID, ActorID int64
	ActorName             string
	Change                GuidanceChange
	Before                *GuidanceSnapshot
	After                 GuidanceSnapshot
	Reason                string
	RecordedAt            time.Time
}

// CanManageStudentGuidance memodelkan hak mutasi; otorisasi nyata tetap milik service.
func CanManageStudentGuidance(roles []string) bool {
	return hasGuidanceRole(roles, GuidanceRoleKesiswaan)
}

// CanReadStudentGuidance memodelkan D-042 memakai penempatan dan penugasan
// aktif dari sumber resmi. Ketiadaan/ketidakjelasan salah satunya menolak
// Wali; helper ini bukan pengganti otorisasi service.
func CanReadStudentGuidance(roles []string, scope GuidanceReadScope) bool {
	if hasGuidanceRole(roles, GuidanceRoleKesiswaan, GuidanceRoleHeadmaster) {
		return true
	}
	if !hasGuidanceRole(roles, GuidanceRoleHomeroom) || !scope.PlacementResolved || !scope.ActivePlacement || !scope.AssignmentsResolved || scope.CurrentClassID < 1 {
		return false
	}
	for _, classID := range scope.AssignedClassIDs {
		if classID == scope.CurrentClassID {
			return true
		}
	}
	return false
}

type GuidanceReadScope struct {
	CurrentClassID      int64
	AssignedClassIDs    []int64
	PlacementResolved   bool
	ActivePlacement     bool
	AssignmentsResolved bool
}

func hasGuidanceRole(roles []string, wanted ...string) bool {
	for _, role := range roles {
		for _, candidate := range wanted {
			if role == candidate {
				return true
			}
		}
	}
	return false
}

// ChangeGuidanceStatus membuat snapshot histori sebelum/sesudah. actorID harus
// berasal dari sesi terautentikasi; otorisasi role dan commit tetap pada service.
func ChangeGuidanceStatus(current GuidanceRecord, actorID, expectedVersion int64, change GuidanceStatusChange, recordedAt time.Time) (GuidanceRecord, GuidanceHistoryEntry, error) {
	if actorID < 1 || current.ID < 1 || current.StudentID < 1 || current.Version < 1 || recordedAt.IsZero() {
		return current, GuidanceHistoryEntry{}, ValidationError("Konteks perubahan catatan tidak valid.")
	}
	if expectedVersion != current.Version {
		return current, GuidanceHistoryEntry{}, ErrGuidanceConflict
	}
	if err := change.Validate(current.Status); err != nil {
		return current, GuidanceHistoryEntry{}, err
	}
	before := guidanceSnapshot(current)
	updated := current
	updated.Status = change.Status
	updated.Version++
	updated.UpdatedAt = recordedAt
	entry := GuidanceHistoryEntry{
		RecordID: current.ID, ActorID: actorID, Change: GuidanceStatusChanged,
		Before: &before, After: guidanceSnapshot(updated), Reason: change.Reason, RecordedAt: recordedAt,
	}
	return updated, entry, nil
}

func guidanceSnapshot(record GuidanceRecord) GuidanceSnapshot {
	return GuidanceSnapshot{
		StudentID: record.StudentID, ClassIDAtEvent: record.ClassIDAtEvent,
		StudentNameAtEvent: record.StudentNameAtEvent, ClassNameAtEvent: record.ClassNameAtEvent,
		Date: record.Date, Incident: record.Incident, Action: record.Action, Status: record.Status,
	}
}

type GuidanceFilter struct {
	StudentID int64
	Search    string
	Status    GuidanceStatus
	Page      int
}

type GuidanceList struct {
	Records     []GuidanceRecord
	Page        int
	HasPrevious bool
	HasNext     bool
}

// GuidanceService mendeskripsikan operasi domain. Adapter PostgreSQL wajib
// memeriksa role/cakupan dari sumber resmi, mengabaikan actor dari klien,
// mengambil snapshot siswa/kelas dari data sah, memakai version optimistis,
// serta menulis perubahan dan histori dalam transaksi yang sama. Domain BK
// tetap terpisah dan kontrak tidak menyediakan penghapusan.
type GuidanceService interface {
	CreateStudentGuidance(context.Context, int64, GuidanceInput) (GuidanceRecord, error)
	UpdateStudentGuidance(context.Context, int64, int64, int64, GuidanceContentChange) (GuidanceRecord, error)
	ChangeStudentGuidanceStatus(context.Context, int64, int64, int64, GuidanceStatusChange) (GuidanceRecord, error)
	StudentGuidanceStudents(context.Context, int64) ([]GuidanceStudentOption, error)
	GetStudentGuidance(context.Context, int64, int64) (GuidanceRecord, error)
	ListStudentGuidance(context.Context, int64, GuidanceFilter) (GuidanceList, error)
	StudentGuidanceHistory(context.Context, int64, int64) ([]GuidanceHistoryEntry, error)
}

// ValidationError menandai masukan domain yang perlu diperbaiki pemanggil.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }
