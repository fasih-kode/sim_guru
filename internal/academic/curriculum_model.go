package academic

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type CurriculumRule struct {
	ID, SemesterID, ClassID, SubjectID, Version int64
	Grade, WeeklyJP                             int
	ClassName, SubjectName                      string
	Meetings                                    []int
	DifferentDays                               bool
}

func (r CurriculumRule) Pattern() string {
	parts := make([]string, len(r.Meetings))
	for i, n := range r.Meetings {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, " + ")
}
func ParseMeetings(text string) ([]int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return []int{}, nil
	}
	parts := strings.Split(strings.ReplaceAll(text, ",", "+"), "+")
	if len(parts) > 210 {
		return nil, ValidationError("Terlalu banyak pertemuan.")
	}
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		n, e := strconv.Atoi(strings.TrimSpace(p))
		if e != nil || n < 1 || n > 30 {
			return nil, ValidationError("Pembagian pertemuan harus berupa JP positif, misalnya 3+2 atau 4+4.")
		}
		result = append(result, n)
	}
	return result, nil
}
func validateCurriculumRule(r CurriculumRule) error {
	total := 0
	for _, n := range r.Meetings {
		if n < 1 || n > 30 {
			return ValidationError("JP tiap pertemuan harus 1–30.")
		}
		total += n
	}
	if r.Grade < 7 || r.Grade > 9 || r.SemesterID < 1 || r.SubjectID < 1 || r.WeeklyJP < 0 || r.WeeklyJP > 210 || total != r.WeeklyJP || len(r.Meetings) > 210 || (r.DifferentDays && len(r.Meetings) > 7) {
		return ValidationError("Jumlah pembagian pertemuan harus sama dengan JP mingguan. Untuk 0 JP kosongkan pembagian; hari berbeda maksimal 7 pertemuan.")
	}
	return nil
}

type SchoolTimeBlock struct {
	ID, SemesterID, Version       int64
	Weekday, PeriodNo             int
	Kind, Label, StartsAt, EndsAt string
}

type EffectiveCurriculum struct {
	CurriculumRule
	TeacherID, AssignmentID int64
	TeacherName             string
	Missing                 bool
	Inherited               bool
	ScheduledJP             int
}

type SchedulePlan struct {
	Classes         []TeachingOption
	Rules           []EffectiveCurriculum
	Periods         []LessonPeriod
	Blocks          []SchoolTimeBlock
	Constraints     []ScheduleConstraint
	ConstraintGrids []ConstraintGrid `json:",omitempty"`
}

func (p SchedulePlan) fingerprint() string {
	b, _ := json.Marshal(p)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
