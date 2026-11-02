package academic

import "testing"

func TestValidateTeachingJournalContentAllowsCorrectedOutcomes(t *testing.T) {
	tests := []struct {
		name    string
		input   TeachingJournalInput
		wantErr bool
	}{
		{
			name:  "koreksi tidak terlaksana menjadi terlaksana",
			input: TeachingJournalInput{State: "final", Outcome: "held", Material: "Materi aktual", ActivityNotes: "Catatan kegiatan", CorrectionReason: "Pilihan sebelumnya keliru"},
		},
		{
			name:  "koreksi terlaksana menjadi tidak terlaksana dengan konteks opsional",
			input: TeachingJournalInput{State: "final", Outcome: "not_held", Material: "Materi rencana, bukan materi yang diajarkan", ActivityNotes: "Catatan konteks", NotHeldReason: "Pertemuan dibatalkan", CorrectionReason: "Pertemuan ternyata tidak terjadi"},
		},
		{
			name:    "tidak terlaksana tetap memerlukan alasan",
			input:   TeachingJournalInput{State: "final", Outcome: "not_held", Material: "Materi konteks"},
			wantErr: true,
		},
		{
			name:  "tidak terlaksana tidak meminta waktu selain snapshot jadwal",
			input: TeachingJournalInput{State: "final", Outcome: "not_held", NotHeldReason: "Dibatalkan"},
		},
		{
			name:    "alasan tidak terlaksana tidak boleh ikut koreksi terlaksana",
			input:   TeachingJournalInput{State: "final", Outcome: "held", Material: "Materi", NotHeldReason: "Alasan lama"},
			wantErr: true,
		},
		{
			name:    "final terlaksana tetap mewajibkan materi",
			input:   TeachingJournalInput{State: "final", Outcome: "held"},
			wantErr: true,
		},
		{
			name:  "draf terlaksana boleh belum memiliki materi",
			input: TeachingJournalInput{State: "draft", Outcome: "held"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTeachingJournalContent(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTeachingJournalContent() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateTeachingJournalTransitionKeepsFinalCorrectionsReasoned(t *testing.T) {
	tests := []struct {
		name    string
		input   TeachingJournalInput
		isNew   bool
		wantErr bool
	}{
		{name: "final koreksi wajib beralasan", input: TeachingJournalInput{State: "final"}, wantErr: true},
		{name: "final koreksi dapat mengubah hasil pertemuan dengan alasan", input: TeachingJournalInput{State: "final", Outcome: "held", CorrectionReason: "Membetulkan status yang keliru"}},
		{name: "final tidak dapat kembali menjadi draf", input: TeachingJournalInput{State: "draft", CorrectionReason: "Alasan"}, wantErr: true},
		{name: "jurnal baru dapat disimpan sebagai draf", input: TeachingJournalInput{State: "draft"}, isNew: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTeachingJournalTransition(test.isNew, "final", test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTeachingJournalTransition() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
