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
