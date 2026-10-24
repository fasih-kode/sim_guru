//go:build integration

package academic

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestASCImportLifecycleIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	cur, _ := testutil.User(t, db, "kurikulum")
	other, _ := testutil.User(t, db, "kurikulum")
	admin, _ := testutil.User(t, db, "admin")
	ua, _ := testutil.User(t, db, "guru_mapel")
	ub, _ := testutil.User(t, db, "guru_mapel")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	y, err := s.CreateYear(ctx, cur.ID, "XML "+testutil.Random(t)[:10])
	must(err)
	var sem, ta, tb, math, english int64
	must(db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, y.ID).Scan(&sem))
	cl, err := s.SaveClass(ctx, admin.ID, 0, y.ID, 7, "A", "", 0)
	must(err)
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Teacher A',$1) RETURNING id`, ua.ID).Scan(&ta))
	must(db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES('Teacher B',$1) RETURNING id`, ub.ID).Scan(&tb))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Matematika') RETURNING id`, "XM"+testutil.Random(t)[:8]).Scan(&math))
	must(db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,'Bahasa Inggris') RETURNING id`, "XE"+testutil.Random(t)[:8]).Scan(&english))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	must(s.AssignTeacher(ctx, cur.ID, sem, cl.ID, math, ta, date))
	must(s.AssignTeacher(ctx, cur.ID, sem, cl.ID, english, tb, date))
	doc := parsedASCFixture(t)
	for day := 1; day <= 2; day++ {
		var periods []LessonPeriod
		for _, p := range doc.Periods {
			periods = append(periods, LessonPeriod{Number: p.Number, StartsAt: p.StartsAt, EndsAt: p.EndsAt})
		}
		must(s.SaveLessonDay(ctx, cur.ID, sem, day, periods))
	}
	old, err := s.CreateTimetableDraft(ctx, cur.ID, sem, date)
	must(err)
	must(s.AddTimetableEntry(ctx, cur.ID, old.ID, 1, cl.ID, math, ta, 1, 2))
	must(s.PublishTimetable(ctx, cur.ID, old.ID))
	oldHash := func() string {
		var h string
		must(db.QueryRow(`SELECT md5(jsonb_build_object('revision',(SELECT to_jsonb(r) FROM app.weekly_timetable_revisions r WHERE id=$1),'entries',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM app.weekly_timetable_entries e WHERE revision_id=$1),'periods',(SELECT jsonb_agg(to_jsonb(p) ORDER BY weekday,period_no) FROM app.weekly_timetable_periods p WHERE revision_id=$1))::text)`, old.ID).Scan(&h))
		return h
	}
	original := oldHash()
	for _, rule := range []CurriculumRule{{SemesterID: sem, Grade: 7, SubjectID: math, WeeklyJP: 2, Meetings: []int{2}}, {SemesterID: sem, Grade: 7, SubjectID: english, WeeklyJP: 3, Meetings: []int{3}}} {
		must(s.SaveCurriculumRule(ctx, cur.ID, rule))
	}
	date = date.AddDate(0, 0, 1)
	if _, err = s.StageASCImport(ctx, admin.ID, sem, date, "test.xml", doc); err != ErrForbidden {
		t.Fatal("admin staged timetable import", err)
	}
	stage := func(d ASCDocument) ASCImport {
		t.Helper()
		v, e := s.StageASCImport(ctx, cur.ID, sem, date, "test.xml", d)
		must(e)
		return v
	}
	preview := func(st ASCImport) (ASCImport, ASCImportReport) {
		t.Helper()
		v, r, e := s.PreviewASCMapping(ctx, cur.ID, st.Token, st.Version, st.Payload.Mapping)
		must(e)
		return v, r
	}
	st := stage(doc)
	if _, _, err = s.ASCImportPreview(ctx, other.ID, st.Token); err != ErrNotFound {
		t.Fatal("foreign owner read import", err)
	}
	if _, err = s.ConfirmASCImport(ctx, cur.ID, st.Token, st.Version, false, false, false); err == nil {
		t.Fatal("unreviewed preview confirmed")
	}
	st, report := preview(st)
	if !report.CanConfirm || report.Incomplete || report.NeedsTimeAck {
		t.Fatal("complete preview failed", report)
	}
	if _, err = s.ConfirmASCImport(ctx, cur.ID, st.Token, st.Version-1, false, false, false); err != ErrConflict {
		t.Fatal("stale preview confirmed")
	}
	rev, err := s.ConfirmASCImport(ctx, cur.ID, st.Token, st.Version, false, false, false)
	must(err)
	repeated, err := s.ConfirmASCImport(ctx, cur.ID, st.Token, st.Version, false, false, false)
	must(err)
	if rev != repeated {
		t.Fatal("duplicate confirm created revision")
	}
	draftHash := func() string {
		var h string
		must(db.QueryRow(`SELECT md5(jsonb_agg(to_jsonb(e) ORDER BY id)::text) FROM app.weekly_timetable_entries e WHERE revision_id=$1`, rev).Scan(&h))
		return h
	}
	before := draftHash()
	bad := stage(doc)
	bad.Payload.Mapping.Teachers["T1"] = tb
	bad, report = preview(bad)
	if report.CanConfirm {
		t.Fatal("wrong teacher accepted")
	}
	if _, err = s.ConfirmASCImport(ctx, cur.ID, bad.Token, bad.Version, true, true, true); err == nil || draftHash() != before {
		t.Fatal("failed import changed draft")
	}
	replacement := stage(doc)
	replacement, report = preview(replacement)
	if _, err = s.ConfirmASCImport(ctx, cur.ID, replacement.Token, replacement.Version, false, false, false); err == nil || draftHash() != before {
		t.Fatal("draft overwritten without approval")
	}
	_, err = s.ConfirmASCImport(ctx, cur.ID, replacement.Token, replacement.Version, true, false, false)
	must(err)
	stale := stage(doc)
	stale, _ = preview(stale)
	rules, err := s.CurriculumRules(ctx, cur.ID, sem)
	must(err)
	must(s.SaveCurriculumRule(ctx, cur.ID, rules[0]))
	before = draftHash()
	if _, err = s.ConfirmASCImport(ctx, cur.ID, stale.Token, stale.Version, true, false, false); err == nil || draftHash() != before {
		t.Fatal("changed configuration not detected")
	}
	stale, _ = preview(stale)
	_, err = s.ConfirmASCImport(ctx, cur.ID, stale.Token, stale.Version, true, false, false)
	must(err)
	changedDraft := stage(doc)
	changedDraft, _ = preview(changedDraft)
	_, _, entries, err := s.Timetable(ctx, cur.ID, sem)
	must(err)
	must(s.RemoveTimetableEntry(ctx, cur.ID, rev, entries[0].ID))
	before = draftHash()
	if _, err = s.ConfirmASCImport(ctx, cur.ID, changedDraft.Token, changedDraft.Version, true, false, false); err == nil || draftHash() != before {
		t.Fatal("changed draft not detected")
	}
	changedDraft, _ = preview(changedDraft)
	_, err = s.ConfirmASCImport(ctx, cur.ID, changedDraft.Token, changedDraft.Version, true, false, false)
	must(err)
	partialXML := regexp.MustCompile(`<lesson id="L2"[^>]*/>`).ReplaceAllString(ascFixtureXML(), "")
	partialXML = regexp.MustCompile(`<card lessonid="L2"[^>]*/>`).ReplaceAllString(partialXML, "")
	partialDoc, err := ParseASCXML(strings.NewReader(partialXML))
	must(err)
	partial := stage(partialDoc)
	partial, report = preview(partial)
	if !report.CanConfirm || !report.Incomplete {
		t.Fatal("valid partial schedule not recognized", report)
	}
	if _, err = s.ConfirmASCImport(ctx, cur.ID, partial.Token, partial.Version, true, false, false); err == nil {
		t.Fatal("incomplete draft accepted without acknowledgment")
	}
	_, err = s.ConfirmASCImport(ctx, cur.ID, partial.Token, partial.Version, true, true, false)
	must(err)
	if s.PublishTimetable(ctx, cur.ID, rev) == nil {
		t.Fatal("partial school timetable published")
	}
	cutDoc, err := ParseASCXML(strings.NewReader(strings.Replace(ascFixtureXML(), `<card lessonid="L2" period="3" days="01" weeks="1" terms="1"/>`, "", 1)))
	must(err)
	cut := stage(cutDoc)
	cut, report = preview(cut)
	if report.CanConfirm || len(report.Errors) == 0 {
		t.Fatal("truncated lesson accepted")
	}
	before = draftHash()
	if _, err = s.ConfirmASCImport(ctx, cur.ID, cut.Token, cut.Version, true, true, true); err == nil || draftHash() != before {
		t.Fatal("truncated import changed draft")
	}
	expired := stage(doc)
	_, err = db.Exec(`UPDATE app.timetable_xml_imports SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, auth.TokenHash(expired.Token))
	must(err)
	if _, _, err = s.ASCImportPreview(ctx, cur.ID, expired.Token); err == nil {
		t.Fatal("expired upload accepted")
	}
	timeDoc, err := ParseASCXML(strings.NewReader(strings.Replace(ascFixtureXML(), `starttime="8:00"`, `starttime="7:50"`, 1)))
	must(err)
	timed := stage(timeDoc)
	timed, report = preview(timed)
	if !report.CanConfirm || !report.NeedsTimeAck {
		t.Fatal("different source time ignored", report)
	}
	if _, err = s.ConfirmASCImport(ctx, cur.ID, timed.Token, timed.Version, true, false, false); err == nil {
		t.Fatal("time mismatch not acknowledged")
	}
	_, err = s.ConfirmASCImport(ctx, cur.ID, timed.Token, timed.Version, true, false, true)
	must(err)
	must(s.PublishTimetable(ctx, cur.ID, rev))
	if oldHash() != original {
		t.Fatal("old published version changed")
	}
	var sourceTime string
	must(db.QueryRow(`SELECT starts_at::text FROM app.lesson_periods WHERE semester_id=$1 AND weekday=1 AND period_no=1`, sem).Scan(&sourceTime))
	if sourceTime != "08:00:00" {
		t.Fatal("XML overwrote school clock")
	}
	var payload string
	must(db.QueryRow(`SELECT payload::text FROM app.timetable_xml_imports WHERE token_hash=$1`, auth.TokenHash(timed.Token)).Scan(&payload))
	if payload != "{}" {
		t.Fatal("completed import retained payload")
	}
}
