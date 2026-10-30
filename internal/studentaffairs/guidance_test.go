package studentaffairs

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGuidanceInputValidate(t *testing.T) {
	for _, status := range []GuidanceStatus{GuidanceNeedsFollowUp, GuidanceCompleted} {
		t.Run(string(status), func(t *testing.T) {
			input := GuidanceInput{
				StudentID: 42,
				Date:      "2026-10-29",
				Incident:  "  Datang terlambat  ",
				Action:    "  Pembinaan dan pengingat  ",
				Status:    status,
			}
			if err := input.Validate(); err != nil {
				t.Fatal(err)
			}
			if input.Incident != "Datang terlambat" || input.Action != "Pembinaan dan pengingat" {
				t.Fatalf("teks belum dinormalisasi: %+v", input)
			}
		})
	}
}

func TestGuidanceInputRejectsInvalidFields(t *testing.T) {
	valid := GuidanceInput{StudentID: 1, Date: "2026-10-29", Incident: "Kejadian", Action: "Pembinaan", Status: GuidanceNeedsFollowUp}
	tests := []struct {
		name   string
		change func(*GuidanceInput)
	}{
		{"master ID wajib", func(in *GuidanceInput) { in.StudentID = 0 }},
		{"ID negatif", func(in *GuidanceInput) { in.StudentID = -1 }},
		{"tanggal mustahil", func(in *GuidanceInput) { in.Date = "2025-02-29" }},
		{"tanggal format lain", func(in *GuidanceInput) { in.Date = "29-10-2026" }},
		{"tanggal tidak lengkap", func(in *GuidanceInput) { in.Date = "2026-1-2" }},
		{"kejadian kosong", func(in *GuidanceInput) { in.Incident = " \t\n" }},
		{"tindakan kosong", func(in *GuidanceInput) { in.Action = " " }},
		{"status lain", func(in *GuidanceInput) { in.Status = "alpa" }},
		{"status kosong", func(in *GuidanceInput) { in.Status = "" }},
		{"kejadian UTF-8 rusak", func(in *GuidanceInput) { in.Incident = string([]byte{0xff}) }},
		{"tindakan NUL", func(in *GuidanceInput) { in.Action = "tindakan\x00lain" }},
		{"kejadian terlalu panjang", func(in *GuidanceInput) { in.Incident = strings.Repeat("界", GuidanceTextMaxRunes+1) }},
		{"tindakan terlalu panjang", func(in *GuidanceInput) { in.Action = strings.Repeat("a", GuidanceTextMaxRunes+1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.change(&input)
			if err := input.Validate(); err == nil {
				t.Fatal("masukan tidak valid diterima")
			}
		})
	}
}

func TestGuidanceTextLimitsCountUnicodeRunes(t *testing.T) {
	input := GuidanceInput{StudentID: 1, Date: "2026-10-29", Incident: strings.Repeat("界", GuidanceTextMaxRunes), Action: strings.Repeat("ñ", GuidanceTextMaxRunes), Status: GuidanceNeedsFollowUp}
	if err := input.Validate(); err != nil {
		t.Fatalf("teks tepat pada batas Unicode harus diterima: %v", err)
	}
}

func TestGuidanceStatusLabels(t *testing.T) {
	if GuidanceNeedsFollowUp.Label() != "Perlu tindak lanjut" || GuidanceCompleted.Label() != "Selesai" {
		t.Fatal("label status tidak sesuai D-018")
	}
	if (GuidanceStatus("tidak_dikenal")).Label() != "" || (GuidanceStatus("tidak_dikenal")).Valid() {
		t.Fatal("status di luar dua nilai D-018 diterima")
	}
}

func TestGuidanceDateMayBeFutureWhenCalendarValid(t *testing.T) {
	input := GuidanceInput{StudentID: 1, Date: "2035-01-01", Incident: "Kejadian", Action: "Pembinaan", Status: GuidanceNeedsFollowUp}
	if err := input.Validate(); err != nil {
		t.Fatalf("tanggal valid tidak boleh ditolak tanpa aturan batas masa depan: %v", err)
	}
	if strings.TrimSpace(input.Date) != input.Date {
		t.Fatal("tanggal canonical berubah")
	}
}

func TestGuidanceStatusChangeRecordsHistoryAndCanReturnToFollowUp(t *testing.T) {
	at := time.Date(2026, 10, 29, 10, 30, 0, 0, time.FixedZone("WIB", 7*60*60))
	current := GuidanceRecord{
		ID: 8, StudentID: 42, RecordedByID: 7, Version: 1,
		Date: "2026-10-20", StudentNameAtEvent: "Nama saat kejadian", ClassIDAtEvent: 14,
		ClassNameAtEvent: "8A", Incident: "Kejadian", Action: "Pembinaan",
		Status: GuidanceNeedsFollowUp,
	}
	completed, firstHistory, err := ChangeGuidanceStatus(current, 9, 1, GuidanceStatusChange{Status: GuidanceCompleted, Reason: "Tindak lanjut selesai"}, at)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != GuidanceCompleted || completed.Version != 2 || !completed.UpdatedAt.Equal(at) {
		t.Fatalf("status atau version tidak diperbarui: %+v", completed)
	}
	if firstHistory.RecordID != current.ID || firstHistory.ActorID != 9 || firstHistory.Change != GuidanceStatusChanged || firstHistory.Reason != "Tindak lanjut selesai" || firstHistory.Before == nil || firstHistory.Before.Status != GuidanceNeedsFollowUp || firstHistory.After.Status != GuidanceCompleted {
		t.Fatalf("histori perubahan status tidak lengkap: %+v", firstHistory)
	}
	if firstHistory.Before.StudentID != 42 || firstHistory.Before.ClassIDAtEvent != 14 || firstHistory.After.StudentNameAtEvent != "Nama saat kejadian" || firstHistory.After.ClassNameAtEvent != "8A" {
		t.Fatal("perubahan status menghilangkan konteks siswa/kelas pada tanggal kejadian")
	}

	reopened, secondHistory, err := ChangeGuidanceStatus(completed, 9, 2, GuidanceStatusChange{Status: GuidanceNeedsFollowUp, Reason: "Perlu pemantauan lanjutan"}, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != GuidanceNeedsFollowUp || reopened.Version != 3 || secondHistory.Before.Status != GuidanceCompleted || secondHistory.After.Status != GuidanceNeedsFollowUp {
		t.Fatalf("status tidak dapat kembali ke tindak lanjut: record=%+v history=%+v", reopened, secondHistory)
	}
}

func TestGuidanceStatusChangeRequiresReasonAndValidTransition(t *testing.T) {
	current := GuidanceRecord{ID: 1, StudentID: 2, Version: 4, Status: GuidanceNeedsFollowUp}
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		actor   int64
		version int64
		change  GuidanceStatusChange
		want    error
	}{
		{name: "alasan wajib", actor: 3, version: 4, change: GuidanceStatusChange{Status: GuidanceCompleted}},
		{name: "alasan kosong", actor: 3, version: 4, change: GuidanceStatusChange{Status: GuidanceCompleted, Reason: " \t"}},
		{name: "alasan terlalu panjang", actor: 3, version: 4, change: GuidanceStatusChange{Status: GuidanceCompleted, Reason: strings.Repeat("a", GuidanceReasonMaxRunes+1)}},
		{name: "alasan Unicode rusak", actor: 3, version: 4, change: GuidanceStatusChange{Status: GuidanceCompleted, Reason: string([]byte{0xff})}},
		{name: "status ilegal", actor: 3, version: 4, change: GuidanceStatusChange{Status: "alpa", Reason: "Alasan"}},
		{name: "status sama", actor: 3, version: 4, change: GuidanceStatusChange{Status: GuidanceNeedsFollowUp, Reason: "Alasan"}},
		{name: "konflik versi", actor: 3, version: 3, change: GuidanceStatusChange{Status: GuidanceCompleted, Reason: "Alasan"}, want: ErrGuidanceConflict},
		{name: "actor invalid", actor: 0, version: 4, change: GuidanceStatusChange{Status: GuidanceCompleted, Reason: "Alasan"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, history, err := ChangeGuidanceStatus(current, tt.actor, tt.version, tt.change, at)
			if err == nil {
				t.Fatal("perubahan tidak valid diterima")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("error=%v want=%v", err, tt.want)
			}
			if updated != current || history.RecordID != 0 {
				t.Fatal("kegagalan mengubah catatan atau menghasilkan histori parsial")
			}
		})
	}
}

func TestGuidanceContentChangeRequiresBothFieldsAndReason(t *testing.T) {
	change := GuidanceContentChange{Incident: "  Kejadian baru  ", Action: "  Tindakan baru  ", Reason: "  Klarifikasi  "}
	if err := change.Validate(); err != nil {
		t.Fatal(err)
	}
	if change.Incident != "Kejadian baru" || change.Action != "Tindakan baru" || change.Reason != "Klarifikasi" {
		t.Fatalf("perubahan belum dinormalisasi: %+v", change)
	}
	for _, invalid := range []GuidanceContentChange{
		{Action: "Tindakan", Reason: "Alasan"},
		{Incident: "Kejadian", Reason: "Alasan"},
		{Incident: "Kejadian", Action: "Tindakan"},
		{Incident: strings.Repeat("a", GuidanceTextMaxRunes+1), Action: "Tindakan", Reason: "Alasan"},
		{Incident: "Kejadian", Action: "Tindakan", Reason: strings.Repeat("a", GuidanceReasonMaxRunes+1)},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("perubahan tidak valid diterima: %+v", invalid)
		}
	}
}

func TestGuidanceStatusChangeReasonLimit(t *testing.T) {
	change := GuidanceStatusChange{Status: GuidanceCompleted, Reason: " " + strings.Repeat("界", GuidanceReasonMaxRunes) + " "}
	if err := change.Validate(GuidanceNeedsFollowUp); err != nil {
		t.Fatalf("alasan tepat pada batas Unicode harus diterima: %v", err)
	}
	if len([]rune(change.Reason)) != GuidanceReasonMaxRunes {
		t.Fatalf("alasan tidak dinormalisasi: %d rune", len([]rune(change.Reason)))
	}
}

func TestGuidanceReadScopeFollowsCurrentPlacementAndAssignment(t *testing.T) {
	scope := GuidanceReadScope{CurrentClassID: 20, AssignedClassIDs: []int64{20}, PlacementResolved: true, ActivePlacement: true, AssignmentsResolved: true}
	if !CanReadStudentGuidance([]string{GuidanceRoleHomeroom}, scope) {
		t.Fatal("wali kelas aktif harus dapat membaca siswa pada kelas yang ditugaskan")
	}
	// Setelah siswa naik/pindah kelas, penugasan historis kelas lama tidak memberi akses.
	scope.CurrentClassID = 21
	if CanReadStudentGuidance([]string{GuidanceRoleHomeroom}, scope) {
		t.Fatal("mantan wali kelas masih dapat membaca setelah siswa pindah kelas")
	}
	scope.AssignedClassIDs = []int64{21}
	if !CanReadStudentGuidance([]string{GuidanceRoleHomeroom}, scope) {
		t.Fatal("wali kelas baru harus mengikuti penempatan aktif siswa")
	}
}

func TestGuidanceReadScopeFailsClosedWhenPlacementOrAssignmentUnknown(t *testing.T) {
	base := GuidanceReadScope{CurrentClassID: 21, AssignedClassIDs: []int64{21}, PlacementResolved: true, ActivePlacement: true, AssignmentsResolved: true}
	tests := []struct {
		name   string
		change func(*GuidanceReadScope)
	}{
		{"placement tidak dapat dipastikan", func(s *GuidanceReadScope) { s.PlacementResolved = false }},
		{"placement nonaktif", func(s *GuidanceReadScope) { s.ActivePlacement = false }},
		{"penugasan tidak dapat dipastikan", func(s *GuidanceReadScope) { s.AssignmentsResolved = false }},
		{"kelas tidak diketahui", func(s *GuidanceReadScope) { s.CurrentClassID = 0 }},
		{"kelas tidak cocok", func(s *GuidanceReadScope) { s.AssignedClassIDs = []int64{20} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope := base
			scope.AssignedClassIDs = append([]int64(nil), base.AssignedClassIDs...)
			tt.change(&scope)
			if CanReadStudentGuidance([]string{GuidanceRoleHomeroom}, scope) {
				t.Fatal("cakupan wali kelas yang tidak pasti tidak ditolak")
			}
		})
	}
}

func TestGuidanceRoleIsolationAndRoleCombination(t *testing.T) {
	unclear := GuidanceReadScope{}
	if !CanManageStudentGuidance([]string{GuidanceRoleKesiswaan}) || !CanReadStudentGuidance([]string{GuidanceRoleKesiswaan}, unclear) {
		t.Fatal("Kesiswaan harus mengelola dan membaca seluruh sekolah")
	}
	if CanManageStudentGuidance([]string{GuidanceRoleHeadmaster}) || !CanReadStudentGuidance([]string{GuidanceRoleHeadmaster}, unclear) {
		t.Fatal("Kepala Sekolah hanya membaca seluruh sekolah")
	}
	for _, role := range []string{"bk", "admin", "guru_mapel", "petugas_absensi", "pegawai_tu"} {
		if CanManageStudentGuidance([]string{role}) || CanReadStudentGuidance([]string{role}, unclear) {
			t.Fatalf("role %s memperoleh akses otomatis", role)
		}
	}
	scope := GuidanceReadScope{CurrentClassID: 20, AssignedClassIDs: []int64{20}, PlacementResolved: true, ActivePlacement: true, AssignmentsResolved: true}
	if !CanReadStudentGuidance([]string{"admin", GuidanceRoleHomeroom}, scope) {
		t.Fatal("role gabungan harus memperoleh hanya hak Wali Kelas yang sah")
	}
	scope.CurrentClassID = 21
	if CanReadStudentGuidance([]string{"admin", GuidanceRoleHomeroom}, scope) {
		t.Fatal("Admin tidak boleh memperluas cakupan Wali Kelas ke seluruh sekolah")
	}
	scope.CurrentClassID = 20
	if CanReadStudentGuidance([]string{"bk", GuidanceRoleHomeroom}, unclear) {
		t.Fatal("role BK tidak boleh memperluas cakupan Wali Kelas yang tidak pasti")
	}
	if !CanReadStudentGuidance([]string{"bk", GuidanceRoleHomeroom}, scope) {
		t.Fatal("role gabungan tetap dapat memakai cakupan Wali Kelas yang sah")
	}
	if !CanReadStudentGuidance([]string{GuidanceRoleKesiswaan, "bk"}, unclear) {
		t.Fatal("role gabungan sah Kesiswaan harus tetap mengikuti hak Kesiswaan")
	}
}
