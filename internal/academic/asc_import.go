package academic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/auth"
)

type ASCMapping struct {
	Classes, Teachers, Subjects map[string]int64
	Days                        map[string]int
	Periods                     map[string]int
}
type ASCImportPayload struct {
	Document                ASCDocument
	Mapping                 ASCMapping
	Date                    time.Time
	PlanDigest, DraftDigest string
	DraftID                 int64
	DraftCount              int
	Previewed               bool
}
type ASCImport struct {
	Token, FileName, State          string
	Version, SemesterID, RevisionID int64
	Payload                         ASCImportPayload
}
type ASCPreviewRow struct {
	Number                         int
	Source, SourceTime, SchoolTime string
	Entry                          TimetableEntry
	Errors                         []string
}
type ASCImportReport struct {
	Rows                                 []ASCPreviewRow
	Errors, Warnings                     []string
	Incomplete, NeedsTimeAck, CanConfirm bool
}

func ASCPeriodKey(day string, number int) string { return day + "/" + strconv.Itoa(number) }
func (d ASCDocument) PeriodKeys() []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range d.Meetings {
		for n := m.Start; n <= m.End; n++ {
			key := ASCPeriodKey(m.DayID, n)
			if !seen[key] {
				keys = append(keys, key)
				seen[key] = true
			}
		}
	}
	sort.Strings(keys)
	return keys
}
func normalizeASCName(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }
func matchASCItems(source []ASCItem, targets map[int64]string) map[string]int64 {
	out := map[string]int64{}
	for _, item := range source {
		var matches []int64
		for id, name := range targets {
			if normalizeASCName(item.Name) == normalizeASCName(name) {
				matches = append(matches, id)
			}
		}
		if len(matches) == 1 {
			out[item.ID] = matches[0]
		}
	}
	return out
}
func defaultASCMapping(doc ASCDocument, p SchedulePlan) ASCMapping {
	classes, teachers, subjects := map[int64]string{}, map[int64]string{}, map[int64]string{}
	for _, c := range p.Classes {
		classes[c.ID] = c.Name
	}
	for _, r := range p.Rules {
		subjects[r.SubjectID] = r.SubjectName
		if r.TeacherID > 0 {
			teachers[r.TeacherID] = r.TeacherName
		}
	}
	m := ASCMapping{Classes: matchASCItems(doc.Classes, classes), Teachers: matchASCItems(doc.Teachers, teachers), Subjects: matchASCItems(doc.Subjects, subjects), Days: map[string]int{}, Periods: map[string]int{}}
	names := map[string]int{"senin": 1, "monday": 1, "mon": 1, "selasa": 2, "tuesday": 2, "tue": 2, "rabu": 3, "wednesday": 3, "wed": 3, "kamis": 4, "thursday": 4, "thu": 4, "jumat": 5, "friday": 5, "fri": 5, "sabtu": 6, "saturday": 6, "sat": 6, "minggu": 7, "sunday": 7, "sun": 7}
	for _, d := range doc.Days {
		m.Days[d.ID] = names[normalizeASCName(d.Name)]
	}
	for _, meeting := range doc.Meetings {
		for n := meeting.Start; n <= meeting.End; n++ {
			for _, lp := range p.Periods {
				if lp.Weekday == m.Days[meeting.DayID] && lp.Number == n && !periodBlocked(lp, p.Blocks) {
					m.Periods[ASCPeriodKey(meeting.DayID, n)] = n
				}
			}
		}
	}
	return m
}
func ascSourceName(items []ASCItem, id string) string {
	for _, v := range items {
		if v.ID == id {
			return v.Name
		}
	}
	return "Referensi XML tidak dikenal"
}
func BuildASCImportReport(doc ASCDocument, m ASCMapping, p SchedulePlan) ASCImportReport {
	report := ASCImportReport{Errors: append([]string(nil), doc.Issues...)}
	checkMappings := func(label string, items []ASCItem, values map[string]int64) {
		seen := map[int64]string{}
		for _, v := range items {
			id := values[v.ID]
			if id < 1 {
				report.Errors = append(report.Errors, "Pilih padanan "+label+": "+v.Name)
				continue
			}
			if previous, ok := seen[id]; ok {
				report.Errors = append(report.Errors, "Dua referensi "+label+" dipetakan ke data yang sama: "+previous+" dan "+v.Name)
			}
			seen[id] = v.Name
		}
	}
	checkMappings("kelas", doc.Classes, m.Classes)
	checkMappings("guru", doc.Teachers, m.Teachers)
	checkMappings("mapel", doc.Subjects, m.Subjects)
	days := map[int]bool{}
	for _, d := range doc.Days {
		day := m.Days[d.ID]
		if day < 1 || day > 7 || days[day] {
			report.Errors = append(report.Errors, "Pemetaan hari harus valid dan berbeda untuk setiap hari XML.")
		}
		days[day] = true
	}
	rules := map[[2]int64]EffectiveCurriculum{}
	for _, r := range p.Rules {
		rules[[2]int64{r.ClassID, r.SubjectID}] = r
	}
	var entries []TimetableEntry
	for i, source := range doc.Meetings {
		row := ASCPreviewRow{Number: i + 1, Source: ascSourceName(doc.Classes, source.ClassID) + " · " + ascSourceName(doc.Subjects, source.SubjectID) + " · " + ascSourceName(doc.Teachers, source.TeacherID)}
		e := TimetableEntry{ClassID: m.Classes[source.ClassID], SubjectID: m.Subjects[source.SubjectID], TeacherID: m.Teachers[source.TeacherID], Weekday: m.Days[source.DayID]}
		rule, ok := rules[[2]int64{e.ClassID, e.SubjectID}]
		if !ok || rule.AssignmentID == 0 || rule.TeacherID != e.TeacherID {
			row.Errors = append(row.Errors, "Kelas–mapel–guru tidak sesuai pengampu aktif pada tanggal berlaku. Periksa penugasan dan padanan.")
		} else {
			e.AssignmentID = rule.AssignmentID
			e.ClassName, e.SubjectName, e.TeacherName = rule.ClassName, rule.SubjectName, rule.TeacherName
		}
		var sourcePeriods, targetPeriods []LessonPeriod
		for n := source.Start; n <= source.End; n++ {
			target := m.Periods[ASCPeriodKey(source.DayID, n)]
			if n == source.Start {
				e.PeriodStart = target
			}
			e.PeriodEnd = target
			if target < 1 || target != e.PeriodStart+n-source.Start {
				row.Errors = append(row.Errors, "Pemetaan jam dalam satu pertemuan harus lengkap dan berurutan.")
			}
			var actual *LessonPeriod
			for j := range p.Periods {
				lp := &p.Periods[j]
				if lp.Weekday == e.Weekday && lp.Number == target {
					actual = lp
					break
				}
			}
			if actual == nil || periodBlocked(*actual, p.Blocks) {
				row.Errors = append(row.Errors, "Jam tujuan belum diatur atau merupakan istirahat/kegiatan sekolah.")
			}
			for _, sp := range doc.Periods {
				if sp.Number == n {
					sourcePeriods = append(sourcePeriods, LessonPeriod{Weekday: e.Weekday, Number: n, StartsAt: sp.StartsAt, EndsAt: sp.EndsAt})
					if actual != nil && (clockMinute(sp.StartsAt) != clockMinute(actual.StartsAt) || clockMinute(sp.EndsAt) != clockMinute(actual.EndsAt)) {
						report.NeedsTimeAck = true
					}
					break
				}
			}
			if actual != nil {
				targetPeriods = append(targetPeriods, *actual)
			}
		}
		srcEntry := TimetableEntry{Weekday: e.Weekday, PeriodStart: source.Start, PeriodEnd: source.End}
		row.SourceTime = entrySegments(srcEntry, sourcePeriods)
		row.SchoolTime = entrySegments(e, targetPeriods)
		e.TimeSegments = row.SchoolTime
		row.Entry = e
		if len(row.Errors) == 0 {
			entries = append(entries, e)
		}
		report.Rows = append(report.Rows, row)
	}
	if len(report.Rows) == 0 {
		report.Errors = append(report.Errors, "Tidak ada pertemuan yang dapat diimpor.")
	}
	hasRowErrors := false
	for _, row := range report.Rows {
		if len(row.Errors) > 0 {
			hasRowErrors = true
		}
	}
	if len(report.Errors) == 0 && !hasRowErrors {
		if err := ValidatePlannedEntries(p, entries, false); err != nil {
			report.Errors = append(report.Errors, err.Error())
		} else if err = ValidatePlannedEntries(p, entries, true); err != nil {
			report.Incomplete = true
			report.Warnings = append(report.Warnings, "Jadwal belum memenuhi seluruh target sekolah: "+err.Error()+" Draf belum dapat dipublikasikan sampai lengkap.")
		}
	}
	report.CanConfirm = len(report.Errors) == 0 && !hasRowErrors && len(report.Rows) > 0
	if report.NeedsTimeAck {
		report.Warnings = append(report.Warnings, "Waktu XML berbeda dari jam sekolah. Penyimpanan memakai jam tujuan SIM-GURU; definisi jam sekolah tidak diubah.")
	}
	return report
}
func currentASCDraft(ctx context.Context, tx *sql.Tx, semester int64) (id int64, date time.Time, count int, digest string, err error) {
	var snapshot []byte
	err = tx.QueryRowContext(ctx, `SELECT id,effective_from,plan_snapshot FROM app.weekly_timetable_revisions WHERE semester_id=$1 AND status='draft'`, semester).Scan(&id, &date, &snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, time.Time{}, 0, "", nil
	}
	if err != nil {
		return
	}
	entries, e := queryTimetableEntries(ctx, tx, id, 0)
	if e != nil {
		err = e
		return
	}
	count = len(entries)
	periods, e := queryRevisionPeriods(ctx, tx, id)
	if e != nil {
		err = e
		return
	}
	raw, e := json.Marshal(struct {
		ID       int64
		Date     time.Time
		Entries  []TimetableEntry
		Periods  []LessonPeriod
		Snapshot []byte
	}{id, date, entries, periods, snapshot})
	if e != nil {
		err = e
		return
	}
	digest = fmt.Sprintf("%x", sha256.Sum256(raw))
	return
}
func (s *Store) StageASCImport(ctx context.Context, actor, semester int64, date time.Time, name string, doc ASCDocument) (ASCImport, error) {
	var stage ASCImport
	if semester < 1 || date.IsZero() || len(doc.Digest) != 64 || len(doc.Meetings) > 1200 || len(doc.Meetings) == 0 {
		return stage, ValidationError("Pilih semester, tanggal berlaku dan XML jadwal yang valid.")
	}
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return stage, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE app.timetable_xml_imports SET state='expired',payload='{}'::jsonb WHERE owner_id=$1 AND state='pending' AND expires_at<=now()`, actor); err != nil {
		return stage, err
	}
	var pending int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.timetable_xml_imports WHERE owner_id=$1 AND state='pending'`, actor).Scan(&pending); err != nil {
		return stage, err
	}
	if pending >= 5 {
		return stage, ValidationError("Maksimal 5 unggahan aktif; selesaikan atau tunggu masa berlaku 30 menit.")
	}
	p, err := querySchedulePlan(ctx, tx, semester, date)
	if err != nil {
		return stage, err
	}
	id, draftDate, count, digest, err := currentASCDraft(ctx, tx, semester)
	if err != nil {
		return stage, err
	}
	if id > 0 && !draftDate.Equal(date) {
		return stage, ValidationError("Tanggal impor harus sama dengan tanggal draf yang ada.")
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return stage, err
	}
	stage = ASCImport{Token: hex.EncodeToString(secret), FileName: filepath.Base(name), State: "pending", Version: 1, SemesterID: semester, Payload: ASCImportPayload{Document: doc, Mapping: defaultASCMapping(doc, p), Date: date, PlanDigest: p.fingerprint(), DraftDigest: digest, DraftID: id, DraftCount: count}}
	if len(stage.FileName) > 160 {
		stage.FileName = "jadwal.xml"
	}
	data, err := json.Marshal(stage.Payload)
	if err != nil {
		return stage, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app.timetable_xml_imports(token_hash,owner_id,semester_id,file_name,file_hash,payload) VALUES($1,$2,$3,$4,$5,$6)`, auth.TokenHash(stage.Token), actor, semester, stage.FileName, doc.Digest, string(data))
	if err != nil {
		return stage, err
	}
	return stage, tx.Commit()
}
func loadASCImport(ctx context.Context, tx *sql.Tx, actor int64, token string) (ASCImport, error) {
	var stage ASCImport
	var data []byte
	var expired bool
	if len(token) != 64 {
		return stage, ErrNotFound
	}
	err := tx.QueryRowContext(ctx, `SELECT semester_id,file_name,state,version,coalesce(revision_id,0),payload,expires_at<=now() FROM app.timetable_xml_imports WHERE token_hash=$1 AND owner_id=$2`, auth.TokenHash(token), actor).Scan(&stage.SemesterID, &stage.FileName, &stage.State, &stage.Version, &stage.RevisionID, &data, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		return stage, ErrNotFound
	}
	if err != nil {
		return stage, err
	}
	if stage.State != "complete" && (expired || stage.State != "pending") {
		return stage, ValidationError("Unggahan telah kedaluwarsa. Unggah ulang XML.")
	}
	if err = json.Unmarshal(data, &stage.Payload); err != nil {
		return stage, err
	}
	stage.Token = token
	return stage, nil
}
func (s *Store) ASCImportPreview(ctx context.Context, actor int64, token string) (ASCImport, ASCImportReport, error) {
	tx, err := s.reader(ctx, actor, "kurikulum")
	if err != nil {
		return ASCImport{}, ASCImportReport{}, err
	}
	defer tx.Rollback()
	stage, err := loadASCImport(ctx, tx, actor, token)
	if err != nil || stage.State == "complete" {
		return stage, ASCImportReport{}, err
	}
	p, err := querySchedulePlan(ctx, tx, stage.SemesterID, stage.Payload.Date)
	if err != nil {
		return stage, ASCImportReport{}, err
	}
	report := BuildASCImportReport(stage.Payload.Document, stage.Payload.Mapping, p)
	_, _, _, digest, err := currentASCDraft(ctx, tx, stage.SemesterID)
	if err != nil {
		return stage, report, err
	}
	if stage.Payload.PlanDigest != p.fingerprint() || stage.Payload.DraftDigest != digest {
		report.Errors = append(report.Errors, "Pengaturan atau isi draf berubah. Periksa pratinjau ulang sebelum menyimpan.")
		report.CanConfirm = false
	}
	if !stage.Payload.Previewed {
		report.CanConfirm = false
	}
	return stage, report, nil
}
func (s *Store) PreviewASCMapping(ctx context.Context, actor int64, token string, version int64, mapping ASCMapping) (ASCImport, ASCImportReport, error) {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return ASCImport{}, ASCImportReport{}, err
	}
	defer tx.Rollback()
	stage, err := loadASCImport(ctx, tx, actor, token)
	if err != nil || stage.State == "complete" {
		return stage, ASCImportReport{}, err
	}
	if stage.Version != version {
		return stage, ASCImportReport{}, ErrConflict
	}
	p, err := querySchedulePlan(ctx, tx, stage.SemesterID, stage.Payload.Date)
	if err != nil {
		return stage, ASCImportReport{}, err
	}
	id, date, count, digest, err := currentASCDraft(ctx, tx, stage.SemesterID)
	if err != nil {
		return stage, ASCImportReport{}, err
	}
	if id > 0 && !date.Equal(stage.Payload.Date) {
		return stage, ASCImportReport{}, ValidationError("Tanggal draf berubah; unggah ulang dengan tanggal yang sesuai.")
	}
	stage.Payload.Mapping = mapping
	stage.Payload.PlanDigest = p.fingerprint()
	stage.Payload.DraftID = id
	stage.Payload.DraftDigest = digest
	stage.Payload.DraftCount = count
	stage.Payload.Previewed = true
	stage.Version++
	report := BuildASCImportReport(stage.Payload.Document, mapping, p)
	data, err := json.Marshal(stage.Payload)
	if err != nil {
		return stage, report, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE app.timetable_xml_imports SET payload=$1,version=$2 WHERE token_hash=$3`, string(data), stage.Version, auth.TokenHash(token))
	if err != nil {
		return stage, report, err
	}
	return stage, report, tx.Commit()
}
func (s *Store) ConfirmASCImport(ctx context.Context, actor int64, token string, version int64, replace, incomplete, schoolTime bool) (int64, error) {
	tx, err := s.begin(ctx, actor, "kurikulum")
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stage, err := loadASCImport(ctx, tx, actor, token)
	if err != nil {
		return 0, err
	}
	if stage.State == "complete" {
		return stage.RevisionID, nil
	}
	if stage.Version != version {
		return 0, ErrConflict
	}
	if !stage.Payload.Previewed {
		return 0, ValidationError("Periksa padanan dan pratinjau terlebih dahulu.")
	}
	data := stage.Payload
	p, err := querySchedulePlan(ctx, tx, stage.SemesterID, data.Date)
	if err != nil {
		return 0, err
	}
	id, _, count, digest, err := currentASCDraft(ctx, tx, stage.SemesterID)
	if err != nil {
		return 0, err
	}
	if p.fingerprint() != data.PlanDigest || id != data.DraftID || digest != data.DraftDigest {
		return 0, ValidationError("Pengaturan atau isi draf berubah. Periksa pratinjau ulang.")
	}
	report := BuildASCImportReport(data.Document, data.Mapping, p)
	if !report.CanConfirm {
		return 0, ValidationError("Masih ada masalah pemetaan/struktur/jadwal. Impor belum dapat disimpan.")
	}
	if count > 0 && !replace {
		return 0, ValidationError("Centang persetujuan penggantian isi draf yang sudah ada.")
	}
	if report.Incomplete && !incomplete {
		return 0, ValidationError("Centang persetujuan menyimpan draf yang belum lengkap.")
	}
	if report.NeedsTimeAck && !schoolTime {
		return 0, ValidationError("Setujui penggunaan jam sekolah SIM-GURU yang ditampilkan pada pratinjau.")
	}
	if id == 0 {
		rev, e := createTimetableDraftTx(ctx, tx, actor, stage.SemesterID, data.Date)
		if e != nil {
			return 0, e
		}
		id = rev.ID
	} else {
		before, e := queryTimetableEntries(ctx, tx, id, 0)
		if e != nil {
			return 0, e
		}
		if count > 0 {
			raw, e := json.Marshal(before)
			if e != nil {
				return 0, e
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM app.weekly_timetable_entries WHERE revision_id=$1`, id); err != nil {
				return 0, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,before_state,after_state) VALUES($1,$2,'entry_removed',$3,'{"reason":"penggantian draf melalui impor XML disetujui"}'::jsonb)`, id, actor, string(raw)); err != nil {
				return 0, err
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM app.weekly_timetable_periods WHERE revision_id=$1`, id); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_periods(revision_id,weekday,period_no,starts_at,ends_at) SELECT $1,weekday,period_no,starts_at,ends_at FROM app.lesson_periods WHERE semester_id=$2`, id, stage.SemesterID); err != nil {
			return 0, err
		}
	}
	if err = savePlanSnapshot(ctx, tx, id, p); err != nil {
		return 0, err
	}
	for _, row := range report.Rows {
		e := row.Entry
		err = tx.QueryRowContext(ctx, `INSERT INTO app.weekly_timetable_entries(revision_id,weekday,class_id,subject_id,teacher_id,assignment_id,period_start,period_end) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, id, e.Weekday, e.ClassID, e.SubjectID, e.TeacherID, e.AssignmentID, e.PeriodStart, e.PeriodEnd).Scan(&e.ID)
		if err != nil {
			return 0, dbError(err)
		}
		after, err := json.Marshal(struct {
			Entry            TimetableEntry
			XML              ASCMeeting
			Source, FileHash string
		}{e, data.Document.Meetings[row.Number-1], "asc_xml", data.Document.Digest})
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.weekly_timetable_history(revision_id,actor_id,action,after_state) VALUES($1,$2,'entry_added',$3)`, id, actor, string(after)); err != nil {
			return 0, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.timetable_xml_imports SET state='complete',revision_id=$1,completed_at=now(),payload='{}'::jsonb WHERE token_hash=$2`, id, auth.TokenHash(token)); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
