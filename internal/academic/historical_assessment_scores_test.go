package academic

import "testing"

func historicalScoreInputForTest(value, sourceScore, sourceKind, locator string) HistoricalAssessmentScoreInput {
	return HistoricalAssessmentScoreInput{
		ActivityID: 1, Reason: "Koreksi transkripsi", Verification: "sumber dicocokkan", Acknowledged: true,
		Entries: []HistoricalAssessmentScoreEntry{{
			StudentID: 2, ObjectiveID: 3, Value: value, SourceKind: sourceKind,
			SourceIdentity: "Arsip nilai kelas A", SourceLocator: locator,
			SourceScore: sourceScore, SourceResult: "nilai pada kolom siswa dan TP",
			Verification: "nama siswa dan TP dicocokkan",
		}},
	}
}

func TestHistoricalAssessmentScoreEvidenceMustMatchTranscription(t *testing.T) {
	for _, value := range []string{"0", "82,50", "100"} {
		t.Run("valid-"+value, func(t *testing.T) {
			if _, err := validateHistoricalAssessmentScoreInput(historicalScoreInputForTest(value, value, "arsip_manual", "halaman 2, siswa 1, TP A"), true); err != nil {
				t.Fatalf("nilai sumber yang sama ditolak: %v", err)
			}
		})
	}
	for name, in := range map[string]HistoricalAssessmentScoreInput{
		"nilai berbeda":                      historicalScoreInputForTest("82,50", "82,49", "arsip_manual", "halaman 2, siswa 1, TP A"),
		"format sumber invalid":              historicalScoreInputForTest("82,50", "82,501", "arsip_manual", "halaman 2, siswa 1, TP A"),
		"lokator kosong":                     historicalScoreInputForTest("82,50", "82,50", "arsip_manual", ""),
		"sumber aplikasi belum diverifikasi": historicalScoreInputForTest("82,50", "82,50", "aplikasi", "ID 123, siswa 1, TP A"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateHistoricalAssessmentScoreInput(in, true); err == nil {
				t.Fatal("input bukti tidak sah diterima")
			}
		})
	}
	duplicate := historicalScoreInputForTest("82,50", "82,50", "arsip_manual", "halaman 2, siswa A, TP A")
	duplicate.Entries = append(duplicate.Entries, HistoricalAssessmentScoreEntry{StudentID: 4, ObjectiveID: 5, Value: "70", SourceKind: "arsip_manual", SourceIdentity: " arsip nilai kelas a ", SourceLocator: " HALAMAN 2,   SISWA A, TP A ", SourceScore: "70", SourceResult: "nilai pada sel", Verification: "nama dan TP dicocokkan"})
	if _, err := validateHistoricalAssessmentScoreInput(duplicate, true); err == nil {
		t.Fatal("lokator yang sama dipakai untuk dua pasangan dalam satu batch")
	}
}

func TestHistoricalAssessmentScoreProjectionPreservesUnselectedStudent(t *testing.T) {
	rec := SemesterRecommendation{Students: []SemesterRecommendationStudent{{
		ID: "42", NH: "", Status: "Belum Dinilai", Recommendation: "", NeedsReview: true,
	}}}
	nh, status, recommendation := projectHistoricalScoreAfter(rec, 9, 42, map[[2]int64]HistoricalAssessmentScoreEntry{}, AssessmentWeights{NH: 100, Configured: true})
	student := rec.Students[0]
	if nh != student.NH || status != student.Status || recommendation != student.Recommendation || !student.NeedsReview {
		t.Fatalf("proyeksi siswa tanpa nilai terpilih berubah: NH=%q status=%q rekomendasi=%q student=%+v", nh, status, recommendation, student)
	}
}

func TestHistoricalAssessmentScoreProjectionUsesCanonicalReadiness(t *testing.T) {
	weightsZeroNH := AssessmentWeights{NH: 0, ASTS: 40, ASAS: 60, Configured: true}
	weightsPositiveNH := AssessmentWeights{NH: 100, Configured: true}
	activity := func(id string, scored bool, value string) AssessmentTPRecapActivity {
		return AssessmentTPRecapActivity{ID: id, HasValue: scored, EffectiveValue: value}
	}
	tests := []struct {
		name, asts, asas, wantNH, wantStatus, wantRecommendation string
		weights                                                  AssessmentWeights
		activities                                               []AssessmentTPRecapActivity
		required, scored                                         int
	}{
		{
			name:    "NH berbobot nol tanpa komponen ujian tetap Belum Dinilai",
			weights: weightsZeroNH, required: 1, activities: []AssessmentTPRecapActivity{activity("99", false, "")},
			wantNH: "75,00", wantStatus: "Belum Dinilai",
		},
		{
			name: "ASTS ada dan ASAS kosong Belum Lengkap",
			asts: "70,00", weights: weightsZeroNH, required: 1, activities: []AssessmentTPRecapActivity{activity("99", false, "")},
			wantNH: "75,00", wantStatus: "Belum Lengkap",
		},
		{
			name: "NH belum lengkap tidak menghalangi komponen ujian lengkap saat bobot nol",
			asts: "70,00", asas: "90,00", weights: weightsZeroNH, required: 2,
			activities: []AssessmentTPRecapActivity{activity("99", false, ""), activity("100", false, "")},
			wantStatus: "Siap Final", wantRecommendation: "82",
		},
		{
			name:    "NH positif dengan bukti baru tetapi masih tidak lengkap",
			weights: weightsPositiveNH, required: 2, activities: []AssessmentTPRecapActivity{activity("99", false, ""), activity("100", false, "")},
			wantStatus: "Belum Lengkap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			student := SemesterRecommendationStudent{
				ID: "42", ASTS: tt.asts, ASAS: tt.asas,
				Objectives: []AssessmentTPRecapObjective{{ID: "7", RequiredCount: tt.required, ScoredCount: tt.scored, Activities: tt.activities}},
			}
			rec := SemesterRecommendation{Students: []SemesterRecommendationStudent{student}}
			entries := map[[2]int64]HistoricalAssessmentScoreEntry{{42, 7}: {StudentID: 42, ObjectiveID: 7, Value: "75"}}
			nh, status, recommendation := projectHistoricalScoreAfter(rec, 99, 42, entries, tt.weights)
			if nh != tt.wantNH || status != tt.wantStatus || recommendation != tt.wantRecommendation {
				t.Fatalf("proyeksi=%q/%q/%q, ingin %q/%q/%q", nh, status, recommendation, tt.wantNH, tt.wantStatus, tt.wantRecommendation)
			}
		})
	}
}
