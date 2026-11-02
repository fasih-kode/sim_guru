package academic

import "testing"

func TestJournalPeriodLabelPreservesOrderedNoncontiguousBlocks(t *testing.T) {
	tests := []struct {
		name    string
		periods []TeachingJournalPeriodSnapshot
		want    string
	}{
		{"kosong", nil, "—"},
		{"blok", []TeachingJournalPeriodSnapshot{{PeriodNo: 2}, {PeriodNo: 3}}, "Jam ke-2–3"},
		{"melewati jeda", []TeachingJournalPeriodSnapshot{{PeriodNo: 2}, {PeriodNo: 4}, {PeriodNo: 5}}, "Jam ke-2, 4–5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := journalPeriodLabel(tt.periods); got != tt.want {
				t.Fatalf("journalPeriodLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTeachingJournalExportRowBoundary(t *testing.T) {
	for _, tt := range []struct {
		count int
		want  error
	}{
		{count: 999},
		{count: 1000},
		{count: 1001, want: ErrTeachingJournalExportLimit},
	} {
		if err := validateTeachingJournalExportCount(tt.count); err != tt.want {
			t.Errorf("validateTeachingJournalExportCount(%d) = %v, want %v", tt.count, err, tt.want)
		}
	}
}
