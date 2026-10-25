package academic

import (
	"database/sql"
	"testing"
	"time"
)

func TestAssessArrival(t *testing.T) {
	policy := &ArrivalRule{ID: 1, Deadline: "07:00", Tolerance: 5}
	for _, tc := range []struct {
		at   string
		late int64
	}{{"06:59:59", 0}, {"07:00:59", 0}, {"07:05:59", 0}, {"07:06:00", 6}, {"07:08:00", 8}} {
		at, e := time.ParseInLocation("15:04:05", tc.at, attendanceWIB)
		if e != nil {
			t.Fatal(e)
		}
		d := DailyStudent{Status: "hadir", Policy: policy, ArrivedAt: sql.NullTime{Time: at, Valid: true}}
		assessArrival(&d)
		if !d.LateMinutes.Valid || d.LateMinutes.Int64 != tc.late {
			t.Fatal(tc, d.LateMinutes)
		}
	}
	d := DailyStudent{Status: "hadir"}
	assessArrival(&d)
	if d.PunctualityLabel() != "Waktu belum lengkap" {
		t.Fatal(d)
	}
	d.ArrivedAt = sql.NullTime{Time: time.Now(), Valid: true}
	assessArrival(&d)
	if d.LateMinutes.Valid || d.PunctualityLabel() != "Belum dapat dinilai" {
		t.Fatal(d)
	}
	d.Status = "sakit"
	d.Policy = policy
	assessArrival(&d)
	if d.LateMinutes.Valid || d.PunctualityLabel() != "—" {
		t.Fatal(d)
	}
}
