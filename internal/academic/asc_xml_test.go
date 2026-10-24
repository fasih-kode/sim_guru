package academic

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func ascFixtureXML() string {
	raw, err := os.ReadFile("testdata/asc-basic.xml")
	if err != nil {
		panic(err)
	}
	return string(raw)
}
func parsedASCFixture(t *testing.T) ASCDocument {
	t.Helper()
	doc, err := ParseASCXML(strings.NewReader(ascFixtureXML()))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
func TestASCXMLBlocksDaysAndEncoding(t *testing.T) {
	doc := parsedASCFixture(t)
	if len(doc.Issues) != 0 || doc.CardCount != 5 || doc.ExpectedJP != 5 || len(doc.Meetings) != 2 || doc.Meetings[0].End != 2 || doc.Meetings[1].End != 3 {
		t.Fatalf("wrong JP/meeting expansion: %+v", doc)
	}
	if doc.Days[0].Name != "Senin" || doc.Days[1].Name != "Selasa" {
		t.Fatal("day masks not resolved", doc.Days)
	}
	raw := bytes.ReplaceAll([]byte(ascFixtureXML()), []byte("Teacher A"), []byte{'C', 'a', 'f', 0xe9})
	accented, err := ParseASCXML(bytes.NewReader(raw))
	if err != nil || accented.Teachers[0].Name != "Café" {
		t.Fatal("windows-1252 not supported", err)
	}
	truncated := strings.Replace(ascFixtureXML(), `<card lessonid="L2" period="3" days="01" weeks="1" terms="1"/>`, "", 1)
	cut, err := ParseASCXML(strings.NewReader(truncated))
	if err != nil || len(cut.Issues) == 0 || !strings.Contains(strings.Join(cut.Issues, " "), "3 JP, tetapi penempatan berisi 2 JP") {
		t.Fatal("truncation not detected", err, cut.Issues)
	}
}
func TestASCXMLRejectsUnsafeAndUnsupportedData(t *testing.T) {
	for _, bad := range []string{`<!DOCTYPE timetable [<!ENTITY x SYSTEM "file:///etc/passwd">]>` + ascFixtureXML(), `<wrong/>`, strings.Replace(ascFixtureXML(), `<teachers>`, `<teachers><teacher id="T1" name="Duplicate"/>`, 1), strings.Replace(ascFixtureXML(), `period="1" starttime="8:00"`, `period="0" starttime="8:00"`, 1), strings.Replace(ascFixtureXML(), `periodsperweek="2.0"`, `periodsperweek="2.5"`, 1)} {
		if _, err := ParseASCXML(strings.NewReader(bad)); err == nil {
			t.Fatal("unsafe/invalid XML accepted")
		}
	}
	for _, bad := range []string{strings.Replace(ascFixtureXML(), `teacherids="T1"`, `teacherids="T1,T2"`, 1), strings.Replace(ascFixtureXML(), `entireclass="1"`, `entireclass="0"`, 1), strings.Replace(ascFixtureXML(), `weeks="1"`, `weeks="11"`, 1), strings.Replace(ascFixtureXML(), `subjectid="M"`, `subjectid="missing"`, 1)} {
		doc, err := ParseASCXML(strings.NewReader(bad))
		if err != nil {
			continue
		}
		if len(doc.Issues) == 0 {
			t.Fatal("unsupported/reference error not flagged")
		}
	}
	if _, err := ParseASCXML(bytes.NewReader(bytes.Repeat([]byte{'x'}, int(MaxASCBytes)+1))); err == nil {
		t.Fatal("oversize XML accepted")
	}
}
func TestASCPreviewValidatesMappingsAndSchoolRules(t *testing.T) {
	doc := parsedASCFixture(t)
	p := SchedulePlan{Classes: []TeachingOption{{ID: 1, Name: "7A"}}, Rules: []EffectiveCurriculum{
		{CurriculumRule: CurriculumRule{SemesterID: 1, Grade: 7, ClassID: 1, SubjectID: 1, WeeklyJP: 2, Meetings: []int{2}}, TeacherID: 1, AssignmentID: 1},
	}}
	p.Rules[0].ClassName = "7A"
	p.Rules[0].SubjectName = "Matematika"
	p.Rules[0].TeacherName = "Teacher A"
	p.Rules = append(p.Rules, EffectiveCurriculum{CurriculumRule: CurriculumRule{SemesterID: 1, Grade: 7, ClassID: 1, SubjectID: 2, WeeklyJP: 3, Meetings: []int{3}, ClassName: "7A", SubjectName: "Bahasa Inggris"}, TeacherID: 2, AssignmentID: 2, TeacherName: "Teacher B"})
	for day := 1; day <= 2; day++ {
		for _, period := range doc.Periods {
			p.Periods = append(p.Periods, LessonPeriod{Weekday: day, Number: period.Number, StartsAt: period.StartsAt, EndsAt: period.EndsAt})
		}
	}
	mapping := defaultASCMapping(doc, p)
	report := BuildASCImportReport(doc, mapping, p)
	if !report.CanConfirm || report.Incomplete || report.NeedsTimeAck {
		t.Fatal("valid complete preview rejected", report)
	}
	p.Blocks = []SchoolTimeBlock{{Kind: "activity", Label: "Upacara", Weekday: 1, PeriodNo: 1, StartsAt: "08:00", EndsAt: "08:40"}}
	if BuildASCImportReport(doc, mapping, p).CanConfirm {
		t.Fatal("import bypassed Upacara")
	}
	p.Blocks = nil
	p.Constraints = []ScheduleConstraint{{TeacherID: 1, Severity: "hard", Weekdays: []int{1}, AllDay: true}}
	if BuildASCImportReport(doc, mapping, p).CanConfirm {
		t.Fatal("import bypassed mandatory availability")
	}
	p.Constraints = nil
	mapping.Teachers["T1"] = 2
	if BuildASCImportReport(doc, mapping, p).CanConfirm {
		t.Fatal("wrong/duplicate teacher mapping accepted")
	}
}
