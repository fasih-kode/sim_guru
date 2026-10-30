package studentcases

import (
	"strings"
	"testing"
)

func TestCaseFlowAllowsOnlyNextStage(t *testing.T) {
	if err := ValidateReferral(StageHomeroom, StageCounselor); err != nil {
		t.Fatalf("rujukan wali ke BK ditolak: %v", err)
	}
	if err := ValidateReferral(StageCounselor, StageDiscipline); err != nil {
		t.Fatalf("rujukan BK ke Kesiswaan ditolak: %v", err)
	}
	for _, pair := range [][2]Stage{
		{StageHomeroom, StageDiscipline},
		{StageCounselor, StageHomeroom},
		{StageDiscipline, StageHomeroom},
		{StageDiscipline, StageCounselor},
		{StageHomeroom, StageHomeroom},
	} {
		if err := ValidateReferral(pair[0], pair[1]); err == nil {
			t.Errorf("transisi %s → %s seharusnya ditolak", pair[0], pair[1])
		}
	}
}

func TestStageResponsibleRolesAreDistinct(t *testing.T) {
	for stage, role := range map[Stage]string{StageHomeroom: RoleHomeroom, StageCounselor: RoleCounselor, StageDiscipline: RoleStudentAffairs} {
		if got := stage.Role(); got != role {
			t.Errorf("role tahap %s = %q, ingin %q", stage, got, role)
		}
	}
	if Stage("unknown").Role() != "" {
		t.Fatal("tahap tak dikenal memiliki role")
	}
}

func TestCaseStatusAndReasonValidation(t *testing.T) {
	for _, status := range []Status{StatusOpen, StatusCompleted} {
		if !status.Valid() {
			t.Errorf("status %q tidak dikenali", status)
		}
	}
	for _, status := range []Status{"", "ditinjau", "ditolak"} {
		if status.Valid() {
			t.Errorf("status tambahan %q diterima", status)
		}
	}
	input := StatusInput{Status: StatusCompleted, Reason: "   "}
	if input.Validate(StatusOpen) == nil {
		t.Fatal("alasan kosong untuk penyelesaian diterima")
	}
	input = StatusInput{Status: StatusCompleted, Reason: "Pembinaan berhasil"}
	if err := input.Validate(StatusOpen); err != nil {
		t.Fatalf("status sah ditolak: %v", err)
	}
	if input.Validate(StatusCompleted) == nil {
		t.Fatal("status yang sama bukan no-op dan seharusnya ditolak")
	}
	input = StatusInput{Status: StatusOpen, Reason: "Kasus perlu dilanjutkan"}
	if err := input.Validate(StatusCompleted); err != nil {
		t.Fatalf("pembukaan kembali ditolak: %v", err)
	}
}

func TestCaseInputsValidateTextSafely(t *testing.T) {
	valid := CreateInput{StudentID: 1, Content: "Pembinaan awal"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("input awal sah ditolak: %v", err)
	}
	for _, content := range []string{"", " \t", "isi\x00sensitif", string([]byte{0xff}), strings.Repeat("x", SummaryMaxRunes+1)} {
		input := CreateInput{StudentID: 1, Content: content}
		if input.Validate() == nil {
			t.Errorf("teks tak valid diterima: panjang=%d", len(content))
		}
	}
	note := CounselingInput{Content: strings.Repeat("konseling", CounselingMaxRunes/len([]rune("konseling"))+1)}
	if note.Validate() == nil {
		t.Fatal("catatan konseling melewati batas teknis")
	}
}

func TestReferralInputRequiresTargetAndReason(t *testing.T) {
	input := ReferralInput{TargetUserID: 4, Reason: "Perlu layanan lanjutan"}
	if err := input.Validate(); err != nil {
		t.Fatalf("rujukan sah ditolak: %v", err)
	}
	for _, invalid := range []ReferralInput{{Reason: "Alasan"}, {TargetUserID: 4}, {TargetUserID: 4, Reason: "x\x00y"}} {
		if invalid.Validate() == nil {
			t.Errorf("rujukan tidak valid diterima: %+v", invalid)
		}
	}
}

func TestHandoffRequiresReasonAndKeepsExistingCaseStatuses(t *testing.T) {
	for _, status := range []Status{StatusOpen, StatusCompleted} {
		if !status.Valid() {
			t.Errorf("handoff menambah atau menghilangkan status lama: %q", status)
		}
	}
	input := HandoffInput{Reason: "  Perpindahan siswa ke kelas baru  "}
	if err := input.Validate(); err != nil || input.Reason != "Perpindahan siswa ke kelas baru" {
		t.Fatalf("alasan handoff sah tidak dinormalisasi: reason=%q err=%v", input.Reason, err)
	}
	for _, reason := range []string{"", "  ", "teks\x00tidak valid", strings.Repeat("x", ReasonMaxRunes+1)} {
		input := HandoffInput{Reason: reason}
		if input.Validate() == nil {
			t.Errorf("alasan handoff tidak valid diterima: panjang=%d", len(reason))
		}
	}
}
