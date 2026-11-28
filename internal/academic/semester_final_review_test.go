package academic

import (
	"encoding/json"
	"testing"
)

func TestFinalReviewCalendarValidation(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		ok        bool
	}{
		{"valid", `{"semester_id":1,"starts_on":"2020-01-01","ends_on":"2020-06-30","school_days":[5,1],"version":3}`, true},
		{"missing", "", false},
		{"null", "null", false},
		{"wrong semester", `{"semester_id":2,"starts_on":"2020-01-01","ends_on":"2020-06-30","school_days":[1]}`, false},
		{"invalid date", `{"semester_id":1,"starts_on":"2020-02-30","ends_on":"2020-06-30","school_days":[1]}`, false},
		{"inverted dates", `{"semester_id":1,"starts_on":"2020-08-01","ends_on":"2020-06-30","school_days":[1]}`, false},
		{"duplicate day", `{"semester_id":1,"starts_on":"2020-01-01","ends_on":"2020-06-30","school_days":[4,4]}`, false},
		{"unknown calendar shape", `{"semester_id":1,"starts_on":"2020-01-01","ends_on":"2020-06-30","school_days":[1],"alternate_dates":[]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calendar, ok := finalSemanticCalendar(json.RawMessage(tc.raw), 1)
			if ok != tc.ok {
				t.Fatalf("valid = %t, want %t", ok, tc.ok)
			}
			if ok && calendar.SchoolDays[0] != 1 {
				t.Fatal("hari sekolah tidak diurutkan untuk pembandingan")
			}
		})
	}
}

func TestFinalReviewUncertainBeforeReadingLegacySources(t *testing.T) {
	before := mustJSON(semesterFinalSnapshot{SemesterID: 1, ClassID: 2, SubjectID: 3,
		Student: SemesterRecommendationStudent{ID: "4"}, Evidence: json.RawMessage(`{"activities":[]}`)})
	// Bukti yang kurang sudah cukup untuk menahan review tanpa membaca sumber lama.
	needs, uncertain, err := semesterFinalNeedsReview(t.Context(), nil, 1, 2, 3, "4", before, before)
	if err != nil || needs || !uncertain {
		t.Fatalf("needs=%t uncertain=%t error=%v", needs, uncertain, err)
	}
}

func TestFinalHistoricalDetailsKeepReadableData(t *testing.T) {
	snapshot := semesterFinalSnapshot{Student: SemesterRecommendationStudent{ID: "42", NH: "0,00"},
		Evidence: json.RawMessage(`{"weights":{"nh":0,"asts":40,"asas":60}}`)}
	student, weights := semesterFinalHistoricalDetails(mustJSON(snapshot))
	if student.ID != "42" || student.NH != "0,00" || !weights.Configured || weights.NH != 0 || weights.ASTS != 40 || weights.ASAS != 60 {
		t.Fatalf("student=%+v weights=%+v", student, weights)
	}
	snapshot.Evidence = json.RawMessage(`{"weights":{"nh":0}}`)
	student, weights = semesterFinalHistoricalDetails(mustJSON(snapshot))
	if student.ID != "42" || weights.Configured {
		t.Fatalf("student=%+v weights=%+v", student, weights)
	}
}
