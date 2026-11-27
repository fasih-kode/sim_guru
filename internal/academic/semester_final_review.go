package academic

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"slices"
)

type finalReviewCalendar struct {
	SemesterID int64  `json:"semester_id"`
	Start      string `json:"starts_on"`
	End        string `json:"ends_on"`
	SchoolDays []int  `json:"school_days"`
}

func finalSemanticCalendar(raw json.RawMessage, semester int64) (finalReviewCalendar, bool) {
	var c finalReviewCalendar
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil || json.Unmarshal(raw, &c) != nil || c.SemesterID != semester {
		return c, false
	}
	start, errStart := civilDate(c.Start)
	end, errEnd := civilDate(c.End)
	if errStart != nil || errEnd != nil || end.Before(start) || len(c.SchoolDays) < 1 || len(c.SchoolDays) > 7 {
		return c, false
	}
	slices.Sort(c.SchoolDays)
	for i, d := range c.SchoolDays {
		if d < 1 || d > 7 || (i > 0 && c.SchoolDays[i-1] == d) {
			return c, false
		}
	}
	for key := range obj {
		switch key {
		case "semester_id", "starts_on", "ends_on", "school_days", "version":
		default:
			return c, false
		}
	}
	return c, true
}
func finalEvidenceObject(raw json.RawMessage, nullable bool) bool {
	if nullable && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true
	}
	var obj map[string]json.RawMessage
	return json.Unmarshal(raw, &obj) == nil && len(obj) != 0
}
func finalEvidenceList(raw json.RawMessage) (int, bool) {
	var v []json.RawMessage
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return 0, false
	}
	for _, item := range v {
		if !finalEvidenceObject(item, false) {
			return 0, false
		}
	}
	return len(v), true
}
func finalEvidenceBase(ev map[string]json.RawMessage) bool {
	for _, k := range []string{"semester", "class", "subject", "student"} {
		if !finalEvidenceObject(ev[k], false) {
			return false
		}
	}
	if !finalEvidenceObject(ev["weights"], true) {
		return false
	}
	for _, k := range []string{"teacher_assignments", "placements", "placement_events", "objectives", "exam_sources"} {
		if _, ok := finalEvidenceList(ev[k]); !ok {
			return false
		}
	}
	return true
}

type finalObjectiveReview struct {
	ID, Applicability, Average, AverageExact, Threshold, ThresholdSource, Achievement string
	Scored, Required                                                                  int
	Complete                                                                          bool
}
type finalStudentReview struct {
	NH, NHExact, ASTS, ASAS, Status, Recommendation string
	Objectives                                      []finalObjectiveReview
}

func finalStudentSemantics(s SemesterRecommendationStudent) finalStudentReview {
	result := finalStudentReview{NH: s.NH, NHExact: s.NHExact, ASTS: s.ASTS, ASAS: s.ASAS, Status: s.Status, Recommendation: s.Recommendation, Objectives: []finalObjectiveReview{}}
	for _, o := range s.Objectives {
		result.Objectives = append(result.Objectives, finalObjectiveReview{o.ID, o.HistoricalApplicability, o.Average, o.AverageExact, o.Threshold, o.ThresholdSource, o.Achievement, o.ScoredCount, o.RequiredCount, o.Complete})
	}
	slices.SortFunc(result.Objectives, func(a, b finalObjectiveReview) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return result
}

// semesterFinalNeedsReview membandingkan semantik tanpa menulis ulang bukti/hash.
// Bukti lama dibaca dengan format aslinya dan tidak dihitung ulang sebagai NH langsung.
func semesterFinalNeedsReview(ctx context.Context, tx *sql.Tx, semester, class, subject int64, student string, savedSnapshot, currentSnapshot []byte) (bool, bool, error) {
	var saved, active semesterFinalSnapshot
	if json.Unmarshal(savedSnapshot, &saved) != nil || json.Unmarshal(currentSnapshot, &active) != nil {
		return false, true, nil
	}
	validScope := func(s semesterFinalSnapshot) bool {
		return s.SemesterID == semester && s.ClassID == class && s.SubjectID == subject && s.Student.ID == student
	}
	if !validScope(saved) || !validScope(active) {
		return false, true, nil
	}
	var old, now map[string]json.RawMessage
	if json.Unmarshal(saved.Evidence, &old) != nil || old == nil || json.Unmarshal(active.Evidence, &now) != nil || now == nil {
		return false, true, nil
	}
	_, direct := old["direct_tp_scores"]
	_, hasDecisions := old["tp_applicability"]
	beforeCal, okBefore := finalSemanticCalendar(old["calendar"], semester)
	afterCal, okAfter := finalSemanticCalendar(now["calendar"], semester)
	if !okBefore || !okAfter || !finalEvidenceBase(old) || !finalEvidenceBase(now) {
		return false, true, nil
	}
	oldSource := "activities"
	if direct {
		oldSource = "direct_tp_scores"
	}
	if _, ok := finalEvidenceList(old[oldSource]); !ok {
		return false, true, nil
	}
	// Snapshot lama dapat memuat keputusan TP sebagai bagian dari bukti transisi.
	if hasDecisions {
		if _, ok := finalEvidenceList(old["tp_applicability"]); !ok {
			return false, true, nil
		}
	}
	if !direct {
		// Snapshot aktif memang membuang kegiatan lama. Query penyusun bukti lama diperlukan di cabang ini.
		var raw string
		if err := tx.QueryRowContext(ctx, semesterFinalEvidenceQuery, semester, class, subject, student).Scan(&raw); err != nil {
			return false, true, err
		}
		if json.Unmarshal([]byte(raw), &now) != nil || now == nil {
			return false, true, nil
		}
	}
	if _, ok := finalEvidenceList(now[oldSource]); !ok {
		return false, true, nil
	}
	activeDirect, dOK := finalEvidenceList(now["direct_tp_scores"])
	activeDecisions, aOK := finalEvidenceList(now["tp_applicability"])
	if !dOK || !aOK {
		return false, true, nil
	}
	// Cocokkan fakta sumber yang tersedia pada format itu. Nomor versi kalender
	// tidak ikut perbandingan, tetapi tanggal konteks dan bukti lain tetap terikat.
	keys := []string{"semester", "class", "subject", "teacher_assignments", "weights", "student", "placements", "placement_events", "objectives", "exam_sources", oldSource}
	if hasDecisions {
		keys = append(keys, "tp_applicability")
	}
	before, after := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	for _, k := range keys {
		before[k] = old[k]
		after[k] = now[k]
	}
	if direct && hasDecisions {
		before["calculation"] = mustJSON(finalStudentSemantics(saved.Student))
		after["calculation"] = mustJSON(finalStudentSemantics(active.Student))
	}
	var same bool
	if err := tx.QueryRowContext(ctx, "SELECT $1::jsonb=$2::jsonb", string(mustJSON(before)), string(mustJSON(after))).Scan(&same); err != nil {
		return false, true, err
	}
	if !same || beforeCal.Start != afterCal.Start || beforeCal.End != afterCal.End ||
		(!direct && activeDirect > 0) || (!hasDecisions && activeDecisions > 0) {
		return true, false, nil
	}
	// Hari sekolah belum dibuktikan relevansinya bagi semua format Final.
	if !slices.Equal(beforeCal.SchoolDays, afterCal.SchoolDays) {
		return false, true, nil
	}
	return false, false, nil
}

// Ringkasan versi tetap dapat dibaca saat rincian snapshot tidak tersedia.
func semesterFinalHistoricalDetails(raw []byte) (SemesterRecommendationStudent, AssessmentWeights) {
	var snapshot semesterFinalSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return SemesterRecommendationStudent{}, AssessmentWeights{}
	}
	var evidence struct {
		Weights struct{ NH, ASTS, ASAS *int } `json:"weights"`
	}
	if err := json.Unmarshal(snapshot.Evidence, &evidence); err != nil {
		return snapshot.Student, AssessmentWeights{}
	}
	w := evidence.Weights
	if w.NH == nil || w.ASTS == nil || w.ASAS == nil {
		return snapshot.Student, AssessmentWeights{}
	}
	return snapshot.Student, AssessmentWeights{NH: *w.NH, ASTS: *w.ASTS, ASAS: *w.ASAS, Configured: true}
}
