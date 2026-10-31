//go:build integration

package academic

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestTeachingJournalCutoverLineageAndVersionTransactionsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := &Store{DB: db}
	ctx := context.Background()
	cur, _ := testutil.User(t, db, "kurikulum")
	admin, _ := testutil.User(t, db, "admin")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	otherTeacher, _ := testutil.User(t, db, "guru_mapel")
	var priorYear, priorSemester sql.NullInt64
	if err := db.QueryRow(`SELECT id FROM app.school_years WHERE active`).Scan(&priorYear); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM app.semesters WHERE active`).Scan(&priorSemester); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	date := time.Now().In(time.FixedZone("WIB", 7*60*60)).Truncate(24 * time.Hour)
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	year, err := s.CreateYear(ctx, cur.ID, "Journal "+testutil.Random(t)[:10])
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateYear(ctx, cur.ID, year.ID); err != nil {
		t.Fatal(err)
	}
	var semester int64
	if err = db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND number=1`, year.ID).Scan(&semester); err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateSemester(ctx, cur.ID, semester); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`UPDATE app.school_years SET active=false WHERE id=$1`, year.ID)
		if priorYear.Valid {
			_, _ = db.Exec(`UPDATE app.school_years SET active=true WHERE id=$1`, priorYear.Int64)
		}
		_, _ = db.Exec(`UPDATE app.semesters SET active=false WHERE id=$1`, semester)
		if priorSemester.Valid {
			_, _ = db.Exec(`UPDATE app.semesters SET active=true WHERE id=$1`, priorSemester.Int64)
		}
	})
	if err = s.SaveSemesterCalendar(ctx, cur.ID, SemesterCalendar{SemesterID: semester, StartsOn: date.AddDate(0, 0, -40).Format("2006-01-02"), EndsOn: date.AddDate(0, 0, 40).Format("2006-01-02"), SchoolDays: []int{weekday}}, "Rentang test jurnal"); err != nil {
		t.Fatal(err)
	}
	class, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "J"+testutil.Random(t)[:5], "", 0)
	if err != nil {
		t.Fatal(err)
	}
	classB, err := s.SaveClass(ctx, admin.ID, 0, year.ID, 7, "K"+testutil.Random(t)[:5], "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var studentID int64
	if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','L') RETURNING id`, "Siswa jurnal "+testutil.Random(t)[:8]).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreatePlacement(ctx, admin.ID, studentID, class.ID, date.AddDate(0, 0, -40).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	var teacherID, otherID, subjectID int64
	if err = db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES($1,$2) RETURNING id`, "Guru Jurnal", teacher.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.teachers(name,user_id) VALUES($1,$2) RETURNING id`, "Guru Jurnal Lain", otherTeacher.ID).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "J"+testutil.Random(t)[:10], "Mapel Jurnal").Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	effective := date.AddDate(0, 0, -20)
	if err = s.AssignTeacher(ctx, cur.ID, semester, class.ID, subjectID, teacherID, effective); err != nil {
		t.Fatal(err)
	}
	if err = s.AssignTeacher(ctx, cur.ID, semester, classB.ID, subjectID, otherID, effective); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveLessonDay(ctx, cur.ID, semester, weekday, []LessonPeriod{{Number: 1, StartsAt: "07:00", EndsAt: "07:40"}, {Number: 2, StartsAt: "07:40", EndsAt: "08:20"}}); err != nil {
		t.Fatal(err)
	}
	makeRevision := func(on time.Time) (int64, int64) {
		t.Helper()
		rev, e := s.CreateTimetableDraft(ctx, cur.ID, semester, on)
		if e != nil {
			t.Fatal(e)
		}
		if e = s.AddTimetableEntry(ctx, cur.ID, rev.ID, weekday, class.ID, subjectID, teacherID, 1, 2); e != nil {
			t.Fatal(e)
		}
		if e = s.PublishTimetable(ctx, cur.ID, rev.ID); e != nil {
			t.Fatal(e)
		}
		var entry int64
		if e = db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1`, rev.ID).Scan(&entry); e != nil {
			t.Fatal(e)
		}
		return rev.ID, entry
	}
	r1, e1 := makeRevision(effective)
	r2Date := effective.AddDate(0, 0, 5)
	r2Draft, err := s.CreateTimetableDraft(ctx, cur.ID, semester, r2Date)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, r2Draft.ID, weekday, class.ID, subjectID, teacherID, 1, 2); err != nil {
		t.Fatal(err)
	}
	var cutoverIDs [2]int64
	var cutoverErr [2]error
	var publishErr error
	var wg sync.WaitGroup
	for i := range cutoverIDs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cutoverIDs[i], cutoverErr[i] = s.CutoverTeachingJournalLineage(ctx, cur.ID, "Cutover awal jurnal")
		}(i)
	}
	wg.Add(1)
	go func() { defer wg.Done(); publishErr = s.PublishTimetable(ctx, cur.ID, r2Draft.ID) }()
	wg.Wait()
	if cutoverErr[0] != nil || cutoverErr[1] != nil || publishErr != nil || cutoverIDs[0] != cutoverIDs[1] {
		t.Fatalf("cutover/publish serialization failed: %v %v publish=%v %d %d", cutoverErr[0], cutoverErr[1], publishErr, cutoverIDs[0], cutoverIDs[1])
	}
	var baseRevision, baseEntry, logicalID int64
	if err = db.QueryRow(`SELECT d.to_revision_id,d.target_entry_id,d.logical_lesson_id FROM app.teaching_journal_lineage_decisions d JOIN app.teaching_journal_lineage_transitions tr ON tr.id=d.transition_id WHERE tr.cutover_batch_id=$1 AND d.decision='cutover' ORDER BY d.id LIMIT 1`, cutoverIDs[0]).Scan(&baseRevision, &baseEntry, &logicalID); err != nil {
		t.Fatal("cutover did not create complete mapping", err)
	}
	var publishedEntry int64
	if err = db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1`, r2Draft.ID).Scan(&publishedEntry); err != nil {
		t.Fatal(err)
	}
	if baseRevision == r1 {
		if err = s.ResolveTeachingJournalLineage(ctx, cur.ID, r1, r2Draft.ID, "Revision dipublikasikan bersamaan dengan cutover", []TeachingJournalLineageDecision{{Decision: "continue", SourceEntryID: e1, TargetEntryID: publishedEntry, Reason: "Slot berlanjut setelah serialisasi"}}); err != nil {
			t.Fatal(err)
		}
		baseRevision, baseEntry = r2Draft.ID, publishedEntry
	} else if baseRevision != r2Draft.ID {
		t.Fatalf("cutover selected unexpected revision %d", baseRevision)
	}
	if baseRevision == r2Draft.ID && baseEntry == 0 {
		baseEntry = publishedEntry
	}
	lessonContext, err := s.TeacherLessonAttendanceOnDate(ctx, teacher.ID, date.Format("2006-01-02"), baseEntry)
	if err != nil {
		t.Fatal("cannot prepare attendance regression fixture", err)
	}
	if _, err = s.SaveLessonAttendance(ctx, teacher.ID, LessonAttendanceInput{Date: date.Format("2006-01-02"), EntryID: baseEntry, Fingerprint: lessonContext.Fingerprint, Students: []LessonAttendanceChoice{{StudentID: studentID, Status: "hadir"}}}); err != nil {
		t.Fatal("cannot save attendance regression fixture", err)
	}
	var lessonAttendanceBefore, lessonHistoryBefore, teacherEventsBefore, teacherHistoryBefore string
	if err = db.QueryRow(`SELECT count(*)::text||':'||coalesce(md5(jsonb_agg(to_jsonb(a) ORDER BY a.id)::text),'') FROM app.student_lesson_attendance a WHERE timetable_entry_id=$1 AND meeting_date=$2`, baseEntry, date.Format("2006-01-02")).Scan(&lessonAttendanceBefore); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*)::text||':'||coalesce(md5(jsonb_agg(to_jsonb(h) ORDER BY h.id)::text),'') FROM app.student_lesson_attendance_history h JOIN app.student_lesson_attendance a ON a.id=h.attendance_id WHERE a.timetable_entry_id=$1 AND a.meeting_date=$2`, baseEntry, date.Format("2006-01-02")).Scan(&lessonHistoryBefore); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*)::text||':'||coalesce(md5(jsonb_agg(to_jsonb(e) ORDER BY e.id)::text),'') FROM app.teacher_attendance_events e WHERE e.teacher_id=$1`, teacherID).Scan(&teacherEventsBefore); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*)::text||':'||coalesce(md5(jsonb_agg(to_jsonb(h) ORDER BY h.id)::text),'') FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1`, teacherID).Scan(&teacherHistoryBefore); err != nil {
		t.Fatal(err)
	}
	var mappedCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_lineage_decisions d JOIN app.teaching_journal_lineage_transitions tr ON tr.id=d.transition_id WHERE tr.cutover_batch_id=$1 AND d.decision='cutover'`, cutoverIDs[0]).Scan(&mappedCount); err != nil || mappedCount != 1 {
		t.Fatalf("cutover seeded partial or duplicate revision mapping: %d %v", mappedCount, err)
	}
	if baseRevision == r1 && baseEntry != e1 {
		t.Fatalf("cutover entry mismatch: revision=%d entry=%d expected=%d", baseRevision, baseEntry, e1)
	}
	var cutoverDate string
	if err = db.QueryRow(`SELECT cutover_date::text FROM app.teaching_journal_cutover_batches WHERE id=$1`, cutoverIDs[0]).Scan(&cutoverDate); err != nil {
		t.Fatal(err)
	}
	key := "11111111-1111-4111-8111-111111111111"
	input := TeachingJournalInput{LogicalLessonID: logicalID, OccurrenceDate: date, RequestKey: key, State: "draft", Outcome: "held"}
	var results [2]TeachingJournalResult
	var saveErr [2]error
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], saveErr[i] = s.SaveTeachingJournal(ctx, teacher.ID, input) }(i)
	}
	wg.Wait()
	if saveErr[0] != nil || saveErr[1] != nil || results[0].ID != results[1].ID || results[0].Version != 1 || results[1].Version != 1 {
		t.Fatalf("concurrent journal retry duplicated: %#v %#v %v %v", results[0], results[1], saveErr[0], saveErr[1])
	}
	if !(results[0].Replayed || results[1].Replayed) {
		t.Fatal("one concurrent retry should replay idempotently")
	}
	input.RequestKey = "22222222-2222-4222-8222-222222222222"
	input.ExpectedVersion = 1
	input.Material = "Materi awal"
	second, err := s.SaveTeachingJournal(ctx, teacher.ID, input)
	if err != nil || second.Version != 2 {
		t.Fatalf("draft version save failed: %#v %v", second, err)
	}
	input.RequestKey = "33333333-3333-4333-8333-333333333333"
	input.ExpectedVersion = 2
	input.State = "final"
	final, err := s.SaveTeachingJournal(ctx, teacher.ID, input)
	if err != nil || final.Version != 3 {
		t.Fatalf("finalization failed: %#v %v", final, err)
	}
	input.RequestKey = "44444444-4444-4444-8444-444444444444"
	input.ExpectedVersion = 3
	input.Material = "Materi versi terkini"
	input.ActivityNotes = "Catatan versi terkini"
	input.CorrectionReason = "Koreksi catatan final"
	corrected, err := s.SaveTeachingJournal(ctx, teacher.ID, input)
	if err != nil || corrected.Version != 4 {
		t.Fatalf("final correction failed: %#v %v", corrected, err)
	}
	if _, err = s.SaveTeachingJournal(ctx, teacher.ID, input); err != nil {
		t.Fatal("same idempotency key did not replay", err)
	}
	changedRetry := input
	changedRetry.Material = "Payload berbeda dengan key yang sama"
	if _, err = s.SaveTeachingJournal(ctx, teacher.ID, changedRetry); !errors.Is(err, ErrConflict) {
		t.Fatalf("idempotency key accepted a different payload: %v", err)
	}
	input.RequestKey = "55555555-5555-4555-8555-555555555555"
	input.ExpectedVersion = 3
	if _, err = s.SaveTeachingJournal(ctx, teacher.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
	input.RequestKey = "66666666-6666-4666-8666-666666666666"
	input.ExpectedVersion = 0
	if _, err = s.SaveTeachingJournal(ctx, teacher.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate occurrence accepted: %v", err)
	}
	input.RequestKey = "77777777-7777-4777-8777-777777777777"
	input.ExpectedVersion = 4
	if _, err = s.SaveTeachingJournal(ctx, otherTeacher.ID, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-owner journal mutation accepted: %v", err)
	}
	preCut := date.AddDate(0, 0, -1)
	input.OccurrenceDate = preCut
	input.ExpectedVersion = 0
	input.RequestKey = "88888888-8888-4888-8888-888888888888"
	if _, err = s.SaveTeachingJournal(ctx, teacher.ID, input); err == nil {
		t.Fatal("pre-cutover journal accepted")
	}
	var versionCount, historyCount, periodCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_versions WHERE journal_id=$1`, results[0].ID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_history WHERE journal_id=$1`, results[0].ID).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_version_periods p JOIN app.teaching_journal_versions v ON v.id=p.version_id WHERE v.journal_id=$1`, results[0].ID).Scan(&periodCount); err != nil {
		t.Fatal(err)
	}
	if versionCount != 4 || historyCount != 4 || periodCount != 8 {
		t.Fatalf("journal evidence incomplete: versions=%d history=%d periods=%d", versionCount, historyCount, periodCount)
	}
	readFilter := TeachingJournalReadFilter{Page: 1, SemesterID: semester, ClassID: class.ID, SubjectID: subjectID, FromDate: date.Format("2006-01-02"), ToDate: date.Format("2006-01-02"), State: "final", Outcome: "held"}
	readPage, err := s.TeachingJournals(ctx, teacher.ID, readFilter)
	if err != nil || len(readPage.Items) != 1 || readPage.HasNext || readPage.Items[0].ID != results[0].ID || readPage.Items[0].Version != 4 || readPage.Items[0].Material != "Materi versi terkini" || readPage.Items[0].ActivityNotes != "Catatan versi terkini" {
		t.Fatalf("filtered journal list mismatch: %#v %v", readPage, err)
	}
	readPage, err = s.TeachingJournals(ctx, teacher.ID, TeachingJournalReadFilter{Page: 1, Search: "Mapel Jurnal"})
	if err != nil || len(readPage.Items) != 1 || readPage.Items[0].ID != results[0].ID {
		t.Fatalf("snapshot text search did not stay within the owner scope: %#v %v", readPage, err)
	}
	occurrenceOptions, pendingLineage, err := s.TeachingJournalOccurrences(ctx, teacher.ID, date.AddDate(0, 0, 7).Format("2006-01-02"))
	if err != nil || pendingLineage || len(occurrenceOptions) != 1 || occurrenceOptions[0].LogicalLessonID != logicalID || len(occurrenceOptions[0].Periods) != 2 {
		t.Fatalf("eligible occurrence read did not use effective schedule/lineage: %#v pending=%t err=%v", occurrenceOptions, pendingLineage, err)
	}
	if emptyPage, e := s.TeachingJournals(ctx, teacher.ID, TeachingJournalReadFilter{Page: 2, SemesterID: semester}); e != nil || len(emptyPage.Items) != 0 {
		t.Fatalf("journal pagination offset failed: %#v %v", emptyPage, e)
	}
	for _, invalidFilter := range []TeachingJournalReadFilter{{Page: 1, FromDate: "2026-02-30"}, {Page: 1, FromDate: "2026-10-20", ToDate: "2026-10-01"}, {Page: 1, State: "invalid"}, {Page: 0}} {
		if _, err = s.TeachingJournals(ctx, teacher.ID, invalidFilter); err == nil {
			t.Fatalf("invalid journal filter accepted: %#v", invalidFilter)
		}
	}
	readDetail, err := s.TeachingJournalDetail(ctx, teacher.ID, results[0].ID)
	if err != nil || readDetail.CurrentVersion.Number != 4 || readDetail.CurrentVersion.State != "final" || readDetail.CurrentVersion.Outcome != "held" || len(readDetail.CurrentVersion.Periods) != 2 || readDetail.RevisionID != results[0].RevisionID || readDetail.ClassSnapshot == "" {
		t.Fatalf("journal detail did not return stored snapshot: %#v %v", readDetail, err)
	}
	finalToDraft := TeachingJournalInput{LogicalLessonID: logicalID, OccurrenceDate: date, RequestKey: "12111111-1111-4111-8111-111111111111", State: "draft", Outcome: "held", Material: "Tidak boleh menurunkan status Final", ExpectedVersion: 4, CorrectionReason: "Percobaan transisi tidak sah"}
	if _, err = s.SaveTeachingJournal(ctx, teacher.ID, finalToDraft); err == nil {
		t.Fatal("Final journal was allowed to return to Draft")
	} else {
		var validation ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("Final-to-Draft rejection was not a clear validation error: %v", err)
		}
	}
	readVersion, err := s.TeachingJournalVersionDetail(ctx, teacher.ID, results[0].ID, 1)
	if err != nil || readVersion.Number != 1 || readVersion.Event != "created" || readVersion.Material != "" || readVersion.ActivityNotes != "" || len(readVersion.Periods) != 2 {
		t.Fatalf("historical version read failed: %#v %v", readVersion, err)
	}
	readHistory, err := s.TeachingJournalVersions(ctx, teacher.ID, results[0].ID)
	if err != nil || len(readHistory.Versions) != 4 || readHistory.Versions[0].Number != 1 || readHistory.Versions[3].Number != 4 || readHistory.Versions[0].RecordedAt.After(readHistory.Versions[3].RecordedAt) {
		t.Fatalf("chronological version history failed: %#v %v", readHistory, err)
	}
	if _, err = s.TeachingJournalDetail(ctx, otherTeacher.ID, results[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner detail was not hidden: %v", err)
	}
	if _, err = s.TeachingJournalVersionDetail(ctx, otherTeacher.ID, results[0].ID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner version was not hidden: %v", err)
	}
	if otherPage, e := s.TeachingJournals(ctx, otherTeacher.ID, TeachingJournalReadFilter{Page: 1}); e != nil || len(otherPage.Items) != 0 {
		t.Fatalf("cross-owner list leaked journals: %#v %v", otherPage, e)
	}
	if _, err = s.TeachingJournalVersions(ctx, teacher.ID, 999999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing journal history returned wrong result: %v", err)
	}
	if _, err = s.TeachingJournalDetail(ctx, admin.ID, results[0].ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-Guru Mapel read unexpectedly allowed: %v", err)
	}
	var assignmentID int64
	if err = db.QueryRow(`SELECT assignment_id FROM app.teaching_journals WHERE id=$1`, results[0].ID).Scan(&assignmentID); err != nil {
		t.Fatal(err)
	}
	var otherSemester int64
	if err = db.QueryRow(`SELECT id FROM app.semesters WHERE school_year_id=$1 AND id<>$2 ORDER BY number LIMIT 1`, year.ID, semester).Scan(&otherSemester); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.teaching_journals(logical_lesson_id,meeting_date,semester_id,revision_id,timetable_entry_id,assignment_id,class_id,subject_id,teacher_id,owner_user_id,class_snapshot,subject_snapshot,teacher_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'kelas','mapel','guru')`, logicalID, date.AddDate(0, 0, 1), otherSemester, results[0].RevisionID, results[0].EntryID, assignmentID, class.ID, subjectID, teacherID, teacher.ID); err == nil {
		t.Fatal("database accepted a revision paired with another semester")
	}
	if _, err = db.Exec(`INSERT INTO app.teaching_journals(logical_lesson_id,meeting_date,semester_id,revision_id,timetable_entry_id,assignment_id,class_id,subject_id,teacher_id,owner_user_id,class_snapshot,subject_snapshot,teacher_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'kelas','mapel','guru')`, logicalID, date.AddDate(0, 0, 1), semester, results[0].RevisionID, publishedEntry, assignmentID, class.ID, subjectID, teacherID, teacher.ID); err == nil {
		t.Fatal("database accepted a timetable entry paired with another revision")
	}
	if _, err = db.Exec(`INSERT INTO app.teaching_journals(logical_lesson_id,meeting_date,semester_id,revision_id,timetable_entry_id,assignment_id,class_id,subject_id,teacher_id,owner_user_id,class_snapshot,subject_snapshot,teacher_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'duplikat','duplikat','duplikat')`, logicalID, date, semester, results[0].RevisionID, results[0].EntryID, assignmentID, class.ID, subjectID, teacherID, teacher.ID); err == nil {
		t.Fatal("database accepted duplicate occurrence")
	}
	// Bukti versi tanpa history/snapshot JP harus gagal COMMIT dan meninggalkan nol baris.
	rollbackTx, txErr := db.BeginTx(ctx, nil)
	if txErr != nil {
		t.Fatal(txErr)
	}
	var rollbackJournal int64
	rollbackDate := date.AddDate(0, 0, 1)
	if txErr = rollbackTx.QueryRowContext(ctx, `INSERT INTO app.teaching_journals(logical_lesson_id,meeting_date,semester_id,revision_id,timetable_entry_id,assignment_id,class_id,subject_id,teacher_id,owner_user_id,class_snapshot,subject_snapshot,teacher_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`, logicalID, rollbackDate, semester, results[0].RevisionID, results[0].EntryID, assignmentID, class.ID, subjectID, teacherID, teacher.ID, "kelas", "mapel", "guru").Scan(&rollbackJournal); txErr != nil {
		rollbackTx.Rollback()
		t.Fatal(txErr)
	}
	if _, txErr = rollbackTx.ExecContext(ctx, `INSERT INTO app.teaching_journal_versions(journal_id,version_no,state,outcome,actor_id,request_key,request_hash) VALUES($1,1,'draft','held',$2,'99999999-9999-4999-8999-999999999999',decode(repeat('00',32),'hex'))`, rollbackJournal, teacher.ID); txErr != nil {
		rollbackTx.Rollback()
		t.Fatal(txErr)
	}
	if txErr = rollbackTx.Commit(); txErr == nil {
		t.Fatal("incomplete journal version committed")
	}
	var partialRows int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journals WHERE id=$1`, rollbackJournal).Scan(&partialRows); err != nil || partialRows != 0 {
		t.Fatalf("failed evidence commit left partial journal: %d %v", partialRows, err)
	}
	var beforeSnapshot, afterSnapshot string
	if err = db.QueryRow(`SELECT period_no::text||'|'||starts_at::text||'|'||ends_at::text FROM app.teaching_journal_version_periods WHERE version_id=(SELECT id FROM app.teaching_journal_versions WHERE journal_id=$1 AND version_no=1) ORDER BY ordinal LIMIT 1`, results[0].ID).Scan(&beforeSnapshot); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT period_no::text||'|'||starts_at::text||'|'||ends_at::text FROM app.teaching_journal_version_periods WHERE version_id=(SELECT id FROM app.teaching_journal_versions WHERE journal_id=$1 AND version_no=4) ORDER BY ordinal LIMIT 1`, results[0].ID).Scan(&afterSnapshot); err != nil {
		t.Fatal(err)
	}
	if beforeSnapshot != afterSnapshot {
		t.Fatal("correction changed historical JP snapshot", beforeSnapshot, afterSnapshot)
	}
	if err = s.SaveLessonDay(ctx, cur.ID, semester, weekday, []LessonPeriod{{Number: 1, StartsAt: "08:00", EndsAt: "08:40"}, {Number: 2, StartsAt: "08:40", EndsAt: "09:20"}}); err != nil {
		t.Fatal("cannot change timetable configuration for the next published revision", err)
	}
	// Buat split eksplisit: satu cabang meneruskan logical ID, cabang kedua baru.
	r3Draft, err := s.CreateTimetableDraft(ctx, cur.ID, semester, date.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, r3Draft.ID, weekday, class.ID, subjectID, teacherID, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err = s.AddTimetableEntry(ctx, cur.ID, r3Draft.ID, weekday, classB.ID, subjectID, otherID, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err = s.PublishTimetable(ctx, cur.ID, r3Draft.ID); err != nil {
		t.Fatal(err)
	}
	r2 := r3Draft.ID
	var r2Entry, r2NewEntry int64
	if err = db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND class_id=$2`, r2, class.ID).Scan(&r2Entry); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.weekly_timetable_entries WHERE revision_id=$1 AND class_id=$2`, r2, classB.ID).Scan(&r2NewEntry); err != nil {
		t.Fatal(err)
	}
	beforeTransitions := 0
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_lineage_transitions WHERE to_revision_id=$1`, r2).Scan(&beforeTransitions); err != nil {
		t.Fatal(err)
	}
	if err = s.ResolveTeachingJournalLineage(ctx, cur.ID, r1, r2, "Resolusi lengkap", nil); err == nil {
		t.Fatal("empty lineage decision accepted")
	}
	var afterTransitions int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_lineage_transitions WHERE to_revision_id=$1`, r2).Scan(&afterTransitions); err != nil {
		t.Fatal(err)
	}
	if beforeTransitions != afterTransitions {
		t.Fatal("failed lineage resolution left a partial header")
	}
	if err = s.ResolveTeachingJournalLineage(ctx, cur.ID, baseRevision, r2, "Split slot Kurikulum", []TeachingJournalLineageDecision{{Decision: "continue", SourceEntryID: baseEntry, TargetEntryID: r2Entry, Reason: "Cabang utama berlanjut"}, {Decision: "new", TargetEntryID: r2NewEntry, Reason: "Cabang kedua menjadi slot baru"}}); err != nil {
		t.Fatal(err)
	}
	occurrenceRevision, occurrenceEntry, err := s.TeachingJournalOccurrenceRevision(ctx, teacher.ID, logicalID, date)
	if err != nil || occurrenceRevision != baseRevision || occurrenceEntry != baseEntry || occurrenceRevision != results[0].RevisionID || occurrenceEntry != results[0].EntryID {
		t.Fatalf("historical occurrence did not retain the revision effective on its WIB date: revision=%d entry=%d want=%d/%d error=%v", occurrenceRevision, occurrenceEntry, baseRevision, baseEntry, err)
	}
	versionAfterPublish, err := s.TeachingJournalVersionDetail(ctx, teacher.ID, results[0].ID, 1)
	if err != nil || len(versionAfterPublish.Periods) != 2 || versionAfterPublish.Periods[0].StartsAt != "07:00:00" {
		t.Fatalf("publishing a new timetable revision changed a saved journal snapshot: %#v %v", versionAfterPublish, err)
	}
	var nextRevisionStart string
	if err = db.QueryRow(`SELECT starts_at::text FROM app.weekly_timetable_periods WHERE revision_id=$1 AND weekday=$2 AND period_no=1`, r2, weekday).Scan(&nextRevisionStart); err != nil || nextRevisionStart != "08:00:00" {
		t.Fatalf("new revision did not get its own timetable period snapshot: %q %v", nextRevisionStart, err)
	}
	if err = s.ResolveTeachingJournalLineage(ctx, admin.ID, baseRevision, r2, "Tidak berwenang", []TeachingJournalLineageDecision{{Decision: "continue", SourceEntryID: baseEntry, TargetEntryID: r2Entry, Reason: "Ditolak"}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-Curriculum lineage mutation was not denied: %v", err)
	}
	var newLogicalID int64
	if err = db.QueryRow(`SELECT logical_lesson_id FROM app.teaching_journal_lineage_decisions WHERE to_revision_id=$1 AND target_entry_id=$2`, r2, r2NewEntry).Scan(&newLogicalID); err != nil {
		t.Fatal(err)
	}
	if newLogicalID == logicalID {
		t.Fatal("split cabang memperoleh logical ID yang sama")
	}
	// Merge operasional tidak menyatukan ID: satu sumber berlanjut dan sumber lain dihentikan.
	r4, r4Entry := makeRevision(date.AddDate(0, 0, 2))
	if err = s.ResolveTeachingJournalLineage(ctx, cur.ID, r2, r4, "Konsolidasi slot", []TeachingJournalLineageDecision{{Decision: "continue", SourceEntryID: r2Entry, TargetEntryID: r4Entry, Reason: "Sumber utama diteruskan"}, {Decision: "stop", SourceEntryID: r2NewEntry, Reason: "Slot kedua dihentikan"}}); err != nil {
		t.Fatal(err)
	}
	var stopCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.teaching_journal_lineage_decisions WHERE from_revision_id=$1 AND source_entry_id=$2 AND decision='stop' AND logical_lesson_id=$3`, r2, r2NewEntry, newLogicalID).Scan(&stopCount); err != nil {
		t.Fatal(err)
	}
	if stopCount != 1 {
		t.Fatal("merge tidak mencatat penghentian lineage kedua")
	}
	// Upaya memakai kembali ID yang dihentikan gagal di trigger PostgreSQL.
	r5, r5Entry := makeRevision(date.AddDate(0, 0, 3))
	reuseTx, txErr := db.BeginTx(ctx, nil)
	if txErr != nil {
		t.Fatal(txErr)
	}
	var reuseTransition int64
	if txErr = reuseTx.QueryRowContext(ctx, `INSERT INTO app.teaching_journal_lineage_transitions(semester_id,from_revision_id,to_revision_id,effective_from,transition_kind,actor_id,reason) VALUES($1,$2,$3,$4,'revision',$5,'Uji reuse') RETURNING id`, semester, r4, r5, date.AddDate(0, 0, 3), cur.ID).Scan(&reuseTransition); txErr != nil {
		reuseTx.Rollback()
		t.Fatal(txErr)
	}
	if _, txErr = reuseTx.ExecContext(ctx, `INSERT INTO app.teaching_journal_lineage_decisions(transition_id,semester_id,from_revision_id,to_revision_id,target_entry_id,logical_lesson_id,decision,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,'new',$7,'Reuse terlarang')`, reuseTransition, semester, r4, r5, r5Entry, newLogicalID, cur.ID); txErr == nil {
		reuseTx.Rollback()
		t.Fatal("database menerima logical ID yang sudah dihentikan")
	}
	_ = reuseTx.Rollback()
	// Jurnal tidak menulis ulang snapshot absensi pelajaran maupun presensi guru.
	var lessonAttendanceAfter, lessonHistoryAfter, teacherEventsAfter, teacherHistoryAfter string
	if err = db.QueryRow(`SELECT count(*)::text||':'||coalesce(md5(jsonb_agg(to_jsonb(a) ORDER BY a.id)::text),'') FROM app.student_lesson_attendance a WHERE timetable_entry_id=$1 AND meeting_date=$2`, baseEntry, date.Format("2006-01-02")).Scan(&lessonAttendanceAfter); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*)::text||':'||coalesce(md5(jsonb_agg(to_jsonb(h) ORDER BY h.id)::text),'') FROM app.student_lesson_attendance_history h JOIN app.student_lesson_attendance a ON a.id=h.attendance_id WHERE a.timetable_entry_id=$1 AND a.meeting_date=$2`, baseEntry, date.Format("2006-01-02")).Scan(&lessonHistoryAfter); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*)::text||':'||coalesce(md5(jsonb_agg(to_jsonb(e) ORDER BY e.id)::text),'') FROM app.teacher_attendance_events e WHERE e.teacher_id=$1`, teacherID).Scan(&teacherEventsAfter); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*)::text||':'||coalesce(md5(jsonb_agg(to_jsonb(h) ORDER BY h.id)::text),'') FROM app.teacher_attendance_history h JOIN app.teacher_attendance_events e ON e.id=h.event_id WHERE e.teacher_id=$1`, teacherID).Scan(&teacherHistoryAfter); err != nil {
		t.Fatal(err)
	}
	if lessonAttendanceBefore != lessonAttendanceAfter || lessonHistoryBefore != lessonHistoryAfter || teacherEventsBefore != teacherEventsAfter || teacherHistoryBefore != teacherHistoryAfter {
		t.Fatalf("journal operations changed attendance/presence evidence: lesson=%s/%s lesson_history=%s/%s teacher=%s/%s teacher_history=%s/%s", lessonAttendanceBefore, lessonAttendanceAfter, lessonHistoryBefore, lessonHistoryAfter, teacherEventsBefore, teacherEventsAfter, teacherHistoryBefore, teacherHistoryAfter)
	}
}
