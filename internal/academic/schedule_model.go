package academic

import (
	"database/sql"
	"time"
)

type TeachingOption struct {
	ID   int64
	Name string
}

type TeachingAssignment struct {
	ID, SemesterID, ClassID, SubjectID, TeacherID     int64
	SemesterName, ClassName, SubjectName, TeacherName string
	EffectiveFrom                                     time.Time
	EffectiveUntil                                    sql.NullTime
}

type LessonPeriod struct {
	ID               int64
	Weekday, Number  int
	StartsAt, EndsAt string
}

type TimetableRevision struct {
	ID, SemesterID       int64
	SemesterName, Status string
	Revision             int
	EffectiveFrom        time.Time
}

type TimetableEntry struct {
	ID, ClassID, SubjectID, TeacherID, AssignmentID int64
	ClassName, SubjectName, TeacherName             string
	Weekday, PeriodStart, PeriodEnd                 int
	StartsAt, EndsAt                                string
	Kind, TimeSegments                              string
}
