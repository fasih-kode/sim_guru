package academic

import "testing"

func TestSemesterCalendarDates(t *testing.T) {
	c := SemesterCalendar{SemesterID: 1, Version: 1, StartsOn: "2026-07-01", EndsOn: "2026-12-31", SchoolDays: []int{1, 2, 3, 4, 5}, Exceptions: []SchoolDateException{{Date: "2026-07-06", SchoolDay: false}, {Date: "2026-07-11", SchoolDay: true}}}
	for _, tc := range []struct {
		date          string
		want, invalid bool
	}{{"2026-07-06", false, false}, {"2026-07-11", true, false}, {"2026-07-12", false, false}, {"2026-07-07", true, false}, {"2026-06-30", false, true}, {"2027-01-01", false, true}, {"2026-02-30", false, true}} {
		got, e := c.IsSchoolDay(tc.date)
		if (e != nil) != tc.invalid || got != tc.want {
			t.Fatalf("%s: got %v/%v", tc.date, got, e)
		}
	}
	if e := validateCalendar(c); e != nil {
		t.Fatal(e)
	}
	for _, days := range [][]int{nil, {0}, {8}, {1, 1}} {
		bad := c
		bad.SchoolDays = days
		if validateCalendar(bad) == nil {
			t.Fatal("invalid days accepted", days)
		}
	}
	c.StartsOn = "2027-01-01"
	if validateCalendar(c) == nil {
		t.Fatal("inverted range accepted")
	}
	c = SemesterCalendar{}
	if _, e := c.IsSchoolDay("2026-07-01"); e == nil {
		t.Fatal("unconfigured calendar accepted")
	}
}
