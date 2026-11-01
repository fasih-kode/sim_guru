package web

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestParseTeachingJournalReadFilter(t *testing.T) {
	r := httptest.NewRequest("GET", "/jurnal-saya?semester_id=12&class_id=23&subject_id=34&from=2026-10-01&to=2026-10-31&state=final&outcome=held&page=2", nil)
	f, err := parseJournalFilter(r)
	if err != nil {
		t.Fatal(err)
	}
	if f.SemesterID != 12 || f.ClassID != 23 || f.SubjectID != 34 || f.FromDate != "2026-10-01" || f.ToDate != "2026-10-31" || f.State != "final" || f.Outcome != "held" || f.Page != 2 {
		t.Fatalf("filter mismatch: %#v", f)
	}
	for _, target := range []string{
		"/jurnal-saya?from=2026-10-01&page=2&state=final&to=2026-10-31",
	} {
		if got := journalPageURL(academic.TeachingJournalReadFilter{Page: 1, FromDate: "2026-10-01", ToDate: "2026-10-31", State: "final"}, 2); got != target {
			t.Fatalf("pagination URL = %q, want %q", got, target)
		}
	}
	for _, target := range []string{"?semester_id=0", "?class_id=abc", "?page=abc"} {
		if _, err = parseJournalFilter(httptest.NewRequest("GET", "/jurnal-saya"+target, nil)); err == nil {
			t.Errorf("invalid filter %q accepted", target)
		}
	}
}

func TestTeachingJournalReadTemplatesRenderStoredSnapshot(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(day int) string { return "weekday" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templates}
	user := &auth.User{ID: 5, Active: true, Roles: []string{"guru_mapel"}}
	r := httptest.NewRequest("GET", "/jurnal-saya/17", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, user))
	data := view{Page: "teaching-journal-detail", Title: "Detail Jurnal", TeachingJournal: &teachingJournalView{
		BasePath: "/jurnal-saya",
		Detail:   &academic.TeachingJournalDetail{ID: 17, LogicalLessonID: 8, RevisionID: 9, EntryID: 10, MeetingDate: "2026-10-31", ClassSnapshot: "7A", SubjectSnapshot: "Matematika", TeacherSnapshot: "<script>alert(1)</script>", Lineage: academic.TeachingJournalLineageSnapshot{Decision: "cutover", Kind: "cutover", EffectiveFrom: time.Date(2026, 10, 31, 0, 0, 0, 0, time.FixedZone("WIB", 7*60*60))}},
		Selected: &academic.TeachingJournalVersionRecord{Number: 1, State: "final", Outcome: "held", Event: "finalized", ActorName: "Guru", RecordedAt: time.Date(2026, 10, 31, 8, 0, 0, 0, time.FixedZone("WIB", 7*60*60)), Material: "Aljabar", Periods: []academic.TeachingJournalPeriodSnapshot{{Ordinal: 1, PeriodNo: 2, StartsAt: "07:40:00", EndsAt: "08:20:00"}}},
		History:  academic.TeachingJournalHistory{JournalID: 17, Versions: []academic.TeachingJournalVersionRecord{{Number: 1, State: "final", Event: "finalized", ActorName: "Guru"}}},
	}}
	w := httptest.NewRecorder()
	a.render(w, r, "page", 200, data)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Jam ke-2") || !strings.Contains(w.Body.String(), "Aljabar") {
		t.Fatalf("journal detail template did not render stored data: status=%d", w.Code)
	}
	if strings.Contains(w.Body.String(), "<script>alert(1)</script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("journal snapshot text was not HTML escaped")
	}
}

func TestTeachingJournalOperationalTemplatesExposeSafeActionsAndInstantFilters(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(day int) string { return "weekday" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templates}
	user := &auth.User{ID: 5, Active: true, Roles: []string{"guru_mapel"}}
	r := httptest.NewRequest("GET", "/jurnal-saya/17/edit", nil).WithContext(context.WithValue(context.Background(), principalKey{}, user))
	final := academic.TeachingJournalVersionRecord{Number: 2, State: "final", Outcome: "held", Material: "Materi", Periods: []academic.TeachingJournalPeriodSnapshot{{Ordinal: 1, PeriodNo: 2, StartsAt: "07:40", EndsAt: "08:20"}}}
	detail := academic.TeachingJournalDetail{ID: 17, LogicalLessonID: 8, MeetingDate: "2026-10-31", ClassSnapshot: "7A", SubjectSnapshot: "Matematika", TeacherSnapshot: "Guru", CurrentVersion: final}
	w := httptest.NewRecorder()
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journal-form", Title: "Ubah Jurnal", TeachingJournal: &teachingJournalView{Detail: &detail, Selected: &final, Occurrence: &academic.TeachingJournalOccurrenceOption{ClassSnapshot: "7A", SubjectSnapshot: "Matematika", TeacherSnapshot: "Guru", Date: "2026-10-31", Periods: final.Periods}, Form: &teachingJournalForm{LogicalLessonID: 8, Date: "2026-10-31", ExpectedVersion: 2, RequestKey: "abcd1234-1111-4111-8111-123456789012", State: "final", Outcome: "held"}}})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Simpan Koreksi Final") || !strings.Contains(w.Body.String(), "Alasan koreksi") || strings.Contains(w.Body.String(), "Simpan Draf") {
		t.Fatalf("Final journal editor exposed an invalid transition: status=%d", w.Code)
	}
	list := httptest.NewRecorder()
	a.render(list, httptest.NewRequest("GET", "/jurnal-saya", nil).WithContext(context.WithValue(context.Background(), principalKey{}, user)), "page", http.StatusOK, view{Page: "teaching-journals", Title: "Jurnal Saya", TeachingJournal: &teachingJournalView{Filter: academic.TeachingJournalReadFilter{Page: 1}, Page: academic.TeachingJournalPage{Page: 1}}})
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "data-instant-search") || !strings.Contains(list.Body.String(), "data-instant-filter") || strings.Count(list.Body.String(), `id="list-region"`) != 1 || strings.Contains(list.Body.String(), ">Versi</th>") || strings.Contains(list.Body.String(), "· Versi ") {
		t.Fatalf("journal list does not use the shared instant filter contract: status=%d", list.Code)
	}
}

func TestTeachingJournalListShowsCurrentContentAndContext(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(day int) string { return "weekday" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templates}
	user := &auth.User{ID: 5, Active: true, Roles: []string{"guru_mapel"}}
	items := []academic.TeachingJournalListItem{
		{ID: 17, MeetingDate: "2026-10-31", ClassSnapshot: "7A", SubjectSnapshot: "Matematika", State: "final", Outcome: "held", Material: "Materi versi terbaru", ActivityNotes: "Catatan versi terbaru", Version: 9},
		{ID: 18, MeetingDate: "2026-10-30", ClassSnapshot: "7A", SubjectSnapshot: "Matematika", State: "draft", Outcome: "not_held", ActivityNotes: "Catatan konteks", Version: 4},
		{ID: 19, MeetingDate: "2026-10-29", ClassSnapshot: "7A", SubjectSnapshot: "Matematika", State: "draft", Outcome: "held"},
	}
	r := httptest.NewRequest("GET", "/jurnal-saya", nil).WithContext(context.WithValue(context.Background(), principalKey{}, user))
	w := httptest.NewRecorder()
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journals", Title: "Jurnal Saya", TeachingJournal: &teachingJournalView{
		Filter: academic.TeachingJournalReadFilter{Page: 1}, Page: academic.TeachingJournalPage{Page: 1, Items: items},
	}})
	body := w.Body.String()
	for _, want := range []string{
		"Materi Pembelajaran", "Catatan Kegiatan", "Materi versi terbaru", "Catatan versi terbaru",
		"Konteks, bukan bukti pembelajaran", "Catatan konteks", "—",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("journal list missing %q", want)
		}
	}
	if strings.Contains(body, "Versi 9") || strings.Contains(body, "Versi 4") || strings.Contains(body, ">Versi</th>") {
		t.Fatal("operational journal list exposed version numbers")
	}
}

func TestTeachingJournalListShowsEmptyContentAsDash(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(day int) string { return "weekday" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templates}
	user := &auth.User{ID: 5, Active: true, Roles: []string{"guru_mapel"}}
	item := academic.TeachingJournalListItem{ID: 19, MeetingDate: "2026-10-29", ClassSnapshot: "7A", SubjectSnapshot: "Matematika", State: "draft", Outcome: "held"}
	r := httptest.NewRequest("GET", "/jurnal-saya", nil).WithContext(context.WithValue(context.Background(), principalKey{}, user))
	w := httptest.NewRecorder()
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journals", Title: "Jurnal Saya", TeachingJournal: &teachingJournalView{
		Filter: academic.TeachingJournalReadFilter{Page: 1}, Page: academic.TeachingJournalPage{Page: 1, Items: []academic.TeachingJournalListItem{item}},
	}})
	if strings.Count(w.Body.String(), "—") < 2 {
		t.Fatal("empty journal content was not presented with em dashes in desktop and mobile layouts")
	}
}

func TestTeachingJournalEditFormShowsOutcomeControlsAndPlainLanguageHistory(t *testing.T) {
	if title := journalEditTitle("final"); title != "Koreksi Jurnal Final" {
		t.Fatalf("Final edit title = %q", title)
	}
	if title := journalEditTitle("draft"); title != "Lanjutkan Draf" {
		t.Fatalf("Draft edit title = %q", title)
	}
	templateSet, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(day int) string { return "weekday" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templateSet}
	user := &auth.User{ID: 5, Active: true, Roles: []string{"guru_mapel"}}
	final := academic.TeachingJournalVersionRecord{Number: 2, State: "final", Outcome: "not_held", Material: "Konteks", ActivityNotes: "Catatan", NotHeldReason: "Dibatalkan", ActualStart: "07:30:00", ActualEnd: "08:00:00"}
	detail := academic.TeachingJournalDetail{ID: 17, MeetingDate: "2026-10-31", CurrentVersion: final}
	r := httptest.NewRequest("GET", "/jurnal-saya/17/edit", nil).WithContext(context.WithValue(context.Background(), principalKey{}, user))
	w := httptest.NewRecorder()
	a.render(w, r, "page", http.StatusOK, view{Page: "teaching-journal-form", Title: journalEditTitle(final.State), TeachingJournal: &teachingJournalView{
		Detail: &detail, Selected: &final, CanEdit: true, Occurrence: &academic.TeachingJournalOccurrenceOption{Date: "2026-10-31"}, Form: &teachingJournalForm{Outcome: final.Outcome, Material: final.Material, ActivityNotes: final.ActivityNotes, NotHeldReason: final.NotHeldReason, CorrectionReason: "Koreksi"},
	}})
	body := w.Body.String()
	for _, required := range []string{"Koreksi Jurnal Final", `data-journal-outcome`, `data-journal-not-held-field`, "snapshot jadwal resmi", "Catatan kegiatan", "Alasan koreksi", `name="expected_version"`} {
		if !strings.Contains(body, required) {
			t.Errorf("correction form missing %q", required)
		}
	}
	if strings.Contains(body, `name="actual_start"`) || strings.Contains(body, `name="actual_end"`) {
		t.Fatal("journal correction form still accepts teacher-entered actual times")
	}
	detailEvidence := httptest.NewRecorder()
	a.render(detailEvidence, httptest.NewRequest("GET", "/jurnal-saya/17", nil).WithContext(context.WithValue(context.Background(), principalKey{}, user)), "page", http.StatusOK, view{Page: "teaching-journal-detail", Title: "Detail", TeachingJournal: &teachingJournalView{Detail: &detail, Selected: &final}})
	if !strings.Contains(detailEvidence.Body.String(), "Catatan historis dari versi lama") || !strings.Contains(detailEvidence.Body.String(), "snapshot Jam Pelajaran") {
		t.Fatal("previously stored time evidence is not identified as historical alongside official schedule snapshot")
	}
	detailResponse := httptest.NewRecorder()
	a.render(detailResponse, httptest.NewRequest("GET", "/jurnal-saya/17", nil).WithContext(context.WithValue(context.Background(), principalKey{}, user)), "page", http.StatusOK, view{Page: "teaching-journal-detail", Title: "Detail", TeachingJournal: &teachingJournalView{Detail: &detail, Selected: &final, History: academic.TeachingJournalHistory{JournalID: 17, Versions: []academic.TeachingJournalVersionRecord{{Number: 1, State: "final", Event: "finalized"}}}}})
	if !strings.Contains(detailResponse.Body.String(), "Riwayat Perubahan") {
		t.Fatal("journal history heading is not in plain language")
	}
}

func TestTeachingJournalReadHTTPChecksRoleAndFilterBeforeDatabase(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{
		"rowNumber":   func(page, index int) int { return (page-1)*10 + index + 1 },
		"weekdayName": func(day int) string { return "weekday" },
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &application{templates: templates, options: Options{Academics: &academic.Store{}}}
	denied := httptest.NewRequest("GET", "/jurnal-saya", nil)
	denied = denied.WithContext(context.WithValue(denied.Context(), principalKey{}, &auth.User{ID: 22, Active: true, Roles: []string{"kesiswaan"}}))
	deniedResponse := httptest.NewRecorder()
	a.require("guru_mapel", a.teachingJournalList).ServeHTTP(deniedResponse, denied)
	if deniedResponse.Code != http.StatusForbidden {
		t.Fatalf("role without Guru Mapel reached journal reader: %d", deniedResponse.Code)
	}

	invalid := httptest.NewRequest("GET", "/jurnal-saya?semester_id=0", nil)
	invalid = invalid.WithContext(context.WithValue(invalid.Context(), principalKey{}, &auth.User{ID: 23, Active: true, Roles: []string{"guru_mapel"}}))
	invalidResponse := httptest.NewRecorder()
	a.require("guru_mapel", a.teachingJournalList).ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid read filter did not return a client error before database access: %d", invalidResponse.Code)
	}
}
