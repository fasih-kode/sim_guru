// Package studentcases memodelkan koordinasi kasus dan batas data konseling.
package studentcases

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SummaryMaxRunes           = 4000
	CounselingMaxRunes        = 8000
	ReasonMaxRunes            = 1000
	SearchMaxRunes            = 120
	RoleHomeroom              = "wali_kelas"
	RoleCounselor             = "bk"
	RoleStudentAffairs        = "kesiswaan"
	RoleHeadmaster            = "kepala_sekolah"
	StageHomeroom      Stage  = "wali_kelas"
	StageCounselor     Stage  = "guru_bk"
	StageDiscipline    Stage  = "kesiswaan"
	StatusOpen         Status = "perlu_tindak_lanjut"
	StatusCompleted    Status = "selesai"
)

var (
	ErrInvalid        = errors.New("Data kasus tidak valid.")
	ErrForbidden      = errors.New("Akses kasus ditolak.")
	ErrNotFound       = errors.New("Kasus tidak ditemukan.")
	ErrConflict       = errors.New("Kasus telah berubah. Muat ulang sebelum melanjutkan.")
	ErrScopeUnclear   = errors.New("Penempatan atau penugasan aktif belum dapat dipastikan.")
	ErrNoBKAvailable  = errors.New("Tidak ada Guru BK aktif yang dapat dipilih.")
	ErrNoKesAvailable = errors.New("Tidak ada petugas Kesiswaan aktif yang dapat dipilih.")
)

type Stage string
type Status string
type EntryType string
type EventType string

const (
	EntryHomeroom   EntryType = "wali_pembinaan"
	EntryFollowUp   EntryType = "tindak_lanjut"
	EntryBKSummary  EntryType = "bk_ringkasan"
	EntryDiscipline EntryType = "kesiswaan_keputusan"

	EventCreated                  EventType = "created"
	EventEntryAdded               EventType = "entry_added"
	EventCounselingNoteAdded      EventType = "counseling_note_added"
	EventReferredToBK             EventType = "referred_to_bk"
	EventReferredToStudentAffairs EventType = "referred_to_kesiswaan"
	EventHomeroomHandoff          EventType = "wali_handoff"
	EventCompleted                EventType = "completed"
	EventReopened                 EventType = "reopened"
)

func (s Stage) Valid() bool {
	return s == StageHomeroom || s == StageCounselor || s == StageDiscipline
}

func (s Status) Valid() bool { return s == StatusOpen || s == StatusCompleted }

func (s Stage) Role() string {
	switch s {
	case StageHomeroom:
		return RoleHomeroom
	case StageCounselor:
		return RoleCounselor
	case StageDiscipline:
		return RoleStudentAffairs
	default:
		return ""
	}
}

// ValidateReferral menjaga urutan formal dan menolak lompatan tahap.
func ValidateReferral(from, to Stage) error {
	if (from == StageHomeroom && to == StageCounselor) || (from == StageCounselor && to == StageDiscipline) {
		return nil
	}
	return ErrInvalid
}

type CreateInput struct {
	StudentID int64
	Content   string
}

func (in *CreateInput) Validate() error {
	if in.StudentID < 1 {
		return ErrInvalid
	}
	var err error
	in.Content, err = requiredText(in.Content, SummaryMaxRunes)
	return err
}

type EntryInput struct{ Content string }

func (in *EntryInput) Validate() error {
	var err error
	in.Content, err = requiredText(in.Content, SummaryMaxRunes)
	return err
}

type CounselingInput struct{ Content string }

func (in *CounselingInput) Validate() error {
	var err error
	in.Content, err = requiredText(in.Content, CounselingMaxRunes)
	return err
}

type ReferralInput struct {
	TargetUserID int64
	Reason       string
}

func (in *ReferralInput) Validate() error {
	if in.TargetUserID < 1 {
		return ErrInvalid
	}
	var err error
	in.Reason, err = requiredText(in.Reason, ReasonMaxRunes)
	return err
}

type StatusInput struct {
	Status Status
	Reason string
}

type HandoffInput struct{ Reason string }

func (in *HandoffInput) Validate() error {
	var err error
	in.Reason, err = requiredText(in.Reason, ReasonMaxRunes)
	return err
}

func (in *StatusInput) Validate(current Status) error {
	if !in.Status.Valid() || in.Status == current {
		return ErrInvalid
	}
	var err error
	in.Reason, err = requiredText(in.Reason, ReasonMaxRunes)
	return err
}

type Filter struct {
	Page   int
	Search string
	Stage  Stage
	Status Status
}

func (f *Filter) Validate() error {
	if f.Page < 1 || f.Page > 100000 || (f.Stage != "" && !f.Stage.Valid()) || (f.Status != "" && !f.Status.Valid()) || !utf8.ValidString(f.Search) || strings.ContainsRune(f.Search, 0) || utf8.RuneCountInString(f.Search) > SearchMaxRunes {
		return ErrInvalid
	}
	f.Search = strings.TrimSpace(f.Search)
	return nil
}

type Case struct {
	ID              int64
	StudentID       int64
	StudentName     string
	ClassID         int64
	ClassName       string
	Stage           Stage
	ResponsibleID   int64
	ResponsibleName string
	Status          Status
	Version         int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Entry struct {
	ID        int64
	CaseID    int64
	ActorID   int64
	ActorName string
	Type      EntryType
	Content   string
	CreatedAt time.Time
}

type CounselingNote struct {
	ID        int64
	CaseID    int64
	ActorID   int64
	ActorName string
	Content   string
	CreatedAt time.Time
}

type History struct {
	ID                  int64
	CaseID              int64
	ActorID             int64
	ActorName           string
	Event               EventType
	FromStage           Stage
	ToStage             Stage
	FromResponsible     int64
	FromResponsibleName string
	ToResponsible       int64
	ToResponsibleName   string
	Reason              string
	CreatedAt           time.Time
}

type List struct {
	Cases   []Case
	Page    int
	HasNext bool
}

type CaseStudentOption struct {
	ID        int64
	Name      string
	ClassName string
}

type UserOption struct {
	ID   int64
	Name string
}

type Detail struct {
	Case              Case
	Entries           []Entry
	CounselingNotes   []CounselingNote // Hanya diisi service untuk BK penanggung jawab aktif.
	History           []History
	CanWrite          bool
	CanReferToBK      bool
	CanReferToKes     bool
	CanReadCounseling bool
	CanTakeOver       bool
}

func requiredText(value string, maxRunes int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) > maxRunes {
		return "", ErrInvalid
	}
	return value, nil
}
