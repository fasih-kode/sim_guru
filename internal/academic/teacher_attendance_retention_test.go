package academic

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestTeacherAttendancePhotoExpiry(t *testing.T) {
	wib := teacherAttendanceWIBLocation()
	for _, recorded := range []time.Time{
		time.Date(2026, 12, 31, 1, 0, 0, 123, wib),
		time.Date(2026, 12, 30, 18, 0, 0, 123, time.UTC),
		time.Date(2027, 12, 31, 1, 0, 0, 123, wib),
		time.Date(2026, 6, 10, 13, 45, 0, 123, wib),
	} {
		until := TeacherAttendancePhotoRetainUntil(recorded)
		for _, tc := range []struct {
			name    string
			now     time.Time
			deleted sql.NullTime
			want    bool
		}{
			{"before", until.Add(-time.Nanosecond), sql.NullTime{}, false},
			{"at deadline", until, sql.NullTime{}, true},
			{"after", until.Add(time.Nanosecond), sql.NullTime{}, true},
			{"already deleted", until.Add(time.Hour), sql.NullTime{Time: until, Valid: true}, false},
		} {
			t.Run(recorded.String()+"/"+tc.name, func(t *testing.T) {
				if got := teacherAttendancePhotoExpired(until, tc.deleted, tc.now); got != tc.want {
					t.Fatalf("expired=%v want %v", got, tc.want)
				}
			})
		}
	}
}

func TestTeacherAttendancePhotoPurgeLimit(t *testing.T) {
	service := &Store{}
	for _, limit := range []int{-1, 0, 51} {
		if _, err := service.PurgeExpiredTeacherAttendancePhotos(context.Background(), limit); err == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
	for _, limit := range []int{1, 50} {
		if _, err := service.PurgeExpiredTeacherAttendancePhotos(context.Background(), limit); err != ErrTeacherAttendancePhoto {
			t.Fatalf("limit %d error=%v", limit, err)
		}
	}
}
