//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestLearningObjectiveCorrectionHTTPRoutinePreviewHistoryAndCSRFIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "cp1018http"+testutil.Random(t)[:6])
	s := &academic.Store{DB: db}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP HTTP CP10-18"); err != nil {
		t.Fatal(err)
	}
	var objectiveID int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&objectiveID); err != nil {
		t.Fatal(err)
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	path := fmt.Sprintf("/guru/tujuan-pembelajaran/%d/koreksi", objectiveID)
	request := func(method, target string, values url.Values, cookies []*http.Cookie, user auth.User) *httptest.ResponseRecorder {
		body := ""
		if values != nil {
			body = values.Encode()
		}
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	page := request(http.MethodGet, path, nil, nil, owner)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Simpan koreksi rutin") || !strings.Contains(page.Body.String(), "TP ini belum memiliki kegiatan") {
		t.Fatalf("halaman koreksi rutin: %d %s", page.Code, page.Body.String())
	}
	if denied := request(http.MethodGet, path, nil, nil, other); denied.Code != http.StatusForbidden {
		t.Fatalf("guru lain harus ditolak: %d", denied.Code)
	}
	field := func(name string) string {
		match := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`).FindStringSubmatch(page.Body.String())
		if len(match) != 2 {
			t.Fatalf("field %s tidak ada", name)
		}
		return match[1]
	}
	routineForm := url.Values{"expected_version": {field("expected_version")}, "snapshot_token": {field("snapshot_token")}, "submission_key": {field("submission_key")}, "definition": {"TP HTTP CP10-18 diperbaiki"}}
	if noCSRF := request(http.MethodPost, path+"/rutin", routineForm, nil, owner); noCSRF.Code != http.StatusForbidden {
		t.Fatalf("POST tanpa CSRF: %d", noCSRF.Code)
	}
	routineForm.Set("gorilla.csrf.Token", field("gorilla.csrf.Token"))
	if saved := request(http.MethodPost, path+"/rutin", routineForm, page.Result().Cookies(), owner); saved.Code != http.StatusSeeOther {
		t.Fatalf("koreksi rutin harus redirect 303: %d %s", saved.Code, saved.Body.String())
	}
	historyPath := fmt.Sprintf("/guru/tujuan-pembelajaran/%d/riwayat?saved=1&key=%s", objectiveID, url.QueryEscape(routineForm.Get("submission_key")))
	history := request(http.MethodGet, historyPath, nil, nil, owner)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), "Koreksi uraian TP berhasil dicatat") || !strings.Contains(history.Body.String(), "TP HTTP CP10-18 diperbaiki") {
		t.Fatalf("riwayat hasil simpan: %d %s", history.Code, history.Body.String())
	}
	if err = s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Kegiatan untuk mengunci TP", "sumatif", "2020-01-10", []int64{objectiveID}); err != nil {
		t.Fatal(err)
	}
	activityOnlyPage := request(http.MethodGet, path, nil, nil, owner)
	if activityOnlyPage.Code != http.StatusOK || !strings.Contains(activityOnlyPage.Body.String(), "Simpan koreksi rutin") || !strings.Contains(activityOnlyPage.Body.String(), "Kegiatan sudah ditautkan, tetapi belum memiliki nilai") {
		t.Fatalf("kegiatan tanpa nilai tidak boleh mengunci jalur rutin: %d %s", activityOnlyPage.Code, activityOnlyPage.Body.String())
	}
	activityToken := regexp.MustCompile(`name="snapshot_token" value="([^"]+)"`).FindStringSubmatch(activityOnlyPage.Body.String())
	activityCSRF := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(activityOnlyPage.Body.String())
	if len(activityToken) != 2 || len(activityCSRF) != 2 {
		t.Fatal("token koreksi/CSRF kegiatan tanpa nilai tidak ditemukan")
	}
	activityRoutineForm := url.Values{"expected_version": {"1"}, "snapshot_token": {activityToken[1]}, "submission_key": {"00000000-0000-4000-8000-000000001854"}, "definition": {"TP dikoreksi setelah taut kegiatan"}, "gorilla.csrf.Token": {activityCSRF[1]}}
	if corrected := request(http.MethodPost, path+"/rutin", activityRoutineForm, activityOnlyPage.Result().Cookies(), owner); corrected.Code != http.StatusSeeOther {
		t.Fatalf("koreksi rutin saat kegiatan belum bernilai: %d %s", corrected.Code, corrected.Body.String())
	}
	var activityID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activityID, []academic.AssessmentScoreInput{{StudentID: f.StudentA, ObjectiveID: objectiveID, Value: "80"}}); err != nil {
		t.Fatal(err)
	}
	usedPage := request(http.MethodGet, path, nil, nil, owner)
	if usedPage.Code != http.StatusOK || !strings.Contains(usedPage.Body.String(), "Pratinjau dampak") || strings.Contains(usedPage.Body.String(), "Simpan koreksi rutin") {
		t.Fatalf("TP terpakai harus melalui jalur pratinjau: %d %s", usedPage.Code, usedPage.Body.String())
	}
	values := url.Values{"expected_version": {"2"}, "submission_key": {"00000000-0000-4000-8000-000000001851"}, "definition": {"TP HTTP CP10-18 substansi"}, "change_type": {"substansi"}, "reason": {"Penyelarasan substansi"}, "gorilla.csrf.Token": {regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(usedPage.Body.String())[1]}}
	preview := request(http.MethodPost, path+"/pratinjau", values, usedPage.Result().Cookies(), owner)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), "Kegiatan untuk mengunci TP") || !strings.Contains(preview.Body.String(), "Penyelarasan substansi") || !strings.Contains(preview.Body.String(), "Konfirmasi koreksi TP") {
		t.Fatalf("pratinjau dampak HTTP: %d %s", preview.Code, preview.Body.String())
	}
	previewField := func(name string) string {
		match := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`).FindStringSubmatch(preview.Body.String())
		if len(match) != 2 {
			t.Fatalf("field pratinjau %s tidak ada", name)
		}
		return match[1]
	}
	confirm := url.Values{"expected_version": {previewField("expected_version")}, "definition": {previewField("definition")}, "change_type": {previewField("change_type")}, "reason": {previewField("reason")}, "snapshot_token": {previewField("snapshot_token")}, "submission_key": {previewField("submission_key")}, "gorilla.csrf.Token": {regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(preview.Body.String())[1]}}
	confirmed := request(http.MethodPost, fmt.Sprintf("/guru/tujuan-pembelajaran/%d/koreksi/konfirmasi", objectiveID), confirm, usedPage.Result().Cookies(), owner)
	if confirmed.Code != http.StatusSeeOther {
		t.Fatalf("konfirmasi harus tersimpan dan redirect: %d %s", confirmed.Code, confirmed.Body.String())
	}
	finalHistory := request(http.MethodGet, fmt.Sprintf("/guru/tujuan-pembelajaran/%d/riwayat?saved=1&key=%s", objectiveID, url.QueryEscape(confirm.Get("submission_key"))), nil, nil, owner)
	if finalHistory.Code != http.StatusOK || !strings.Contains(finalHistory.Body.String(), "Koreksi uraian TP berhasil dicatat") || !strings.Contains(finalHistory.Body.String(), "TP HTTP CP10-18 substansi") {
		t.Fatalf("konfirmasi tercatat dan tampil: %d %s", finalHistory.Code, finalHistory.Body.String())
	}
	// Perubahan lain sesudah pratinjau harus menghasilkan HTTP 409 tanpa histori parsial.
	stalePage := request(http.MethodGet, path, nil, nil, owner)
	staleValues := url.Values{"expected_version": {"3"}, "submission_key": {"00000000-0000-4000-8000-000000001852"}, "definition": {"Definisi yang menjadi basi"}, "change_type": {"redaksi"}, "reason": {"Alasan pratinjau basi"}, "gorilla.csrf.Token": {regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(stalePage.Body.String())[1]}}
	staleRendered := request(http.MethodPost, path+"/pratinjau", staleValues, stalePage.Result().Cookies(), owner)
	if staleRendered.Code != http.StatusOK {
		t.Fatalf("pratinjau lama: %d %s", staleRendered.Code, staleRendered.Body.String())
	}
	winningPreview, err := s.PreviewLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, 3, "Definisi yang menang lebih dahulu", "redaksi", "Koreksi sah", 1, "00000000-0000-4000-8000-000000001853")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, 3, winningPreview.DefinitionAfter, winningPreview.ChangeType, winningPreview.Reason, winningPreview.SnapshotToken, winningPreview.SubmissionKey); err != nil {
		t.Fatal(err)
	}
	staleField := func(name string) string {
		match := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`).FindStringSubmatch(staleRendered.Body.String())
		if len(match) != 2 {
			t.Fatalf("field basi %s tidak ada", name)
		}
		return match[1]
	}
	staleConfirm := url.Values{"expected_version": {staleField("expected_version")}, "definition": {staleField("definition")}, "change_type": {staleField("change_type")}, "reason": {staleField("reason")}, "snapshot_token": {staleField("snapshot_token")}, "submission_key": {staleField("submission_key")}, "gorilla.csrf.Token": {regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(staleRendered.Body.String())[1]}}
	conflict := request(http.MethodPost, fmt.Sprintf("/guru/tujuan-pembelajaran/%d/koreksi/konfirmasi", objectiveID), staleConfirm, stalePage.Result().Cookies(), owner)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("pratinjau basi harus 409: %d %s", conflict.Code, conflict.Body.String())
	}
	var revisions int
	if err = db.QueryRow(`SELECT count(*) FROM app.learning_objective_definition_revisions WHERE learning_objective_id=$1`, objectiveID).Scan(&revisions); err != nil || revisions != 4 {
		t.Fatalf("konflik tidak boleh menambah histori: count=%d err=%v", revisions, err)
	}
}

func TestLearningObjectiveCorrectionHTTPPreviewPaginationPostsSensitiveFieldsAndRejectsStaleSnapshotIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	f := testutil.ExamFixtureFor(t, db, "cp1018pages"+testutil.Random(t)[:6])
	s := &academic.Store{DB: db}
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]'::jsonb)`, f.Semester); err != nil {
		t.Fatal(err)
	}
	cur, _ := testutil.User(t, db, "kurikulum")
	if err := s.SaveAssessmentWeight(ctx, cur.ID, academic.AssessmentWeights{SemesterID: f.Semester, NH: 60, ASTS: 20, ASAS: 20}, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP untuk pagination dampak CP10-18"); err != nil {
		t.Fatal(err)
	}
	var objectiveID int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&objectiveID); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Sumatif pagination", "sumatif", "2020-01-10", []int64{objectiveID}); err != nil {
		t.Fatal(err)
	}
	var activityID int64
	if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	var yearID int64
	if err := db.QueryRow(`SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	var students []int64
	for i := 0; i < 11; i++ {
		var studentID int64
		name := fmt.Sprintf("CP18 Affected %02d", i)
		if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2010-01-01','L') RETURNING id`, name).Scan(&studentID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01')`, yearID, studentID, f.Class, f.Teacher); err != nil {
			t.Fatal(err)
		}
		students = append(students, studentID)
	}
	var scoreInputs []academic.AssessmentScoreInput
	var examInputs []academic.ExamInitialInput
	for _, studentID := range students {
		scoreInputs = append(scoreInputs, academic.AssessmentScoreInput{StudentID: studentID, ObjectiveID: objectiveID, Value: "80"})
		examInputs = append(examInputs,
			academic.ExamInitialInput{StudentID: studentID, Component: "asts", Date: "2020-01-10", Value: "70"},
			academic.ExamInitialInput{StudentID: studentID, Component: "asas", Date: "2020-01-10", Value: "90"})
	}
	if err := s.SaveAssessmentActivityInitialScores(ctx, f.Teacher, activityID, scoreInputs); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExamInitialScores(ctx, f.Teacher, f.Semester, f.Class, f.Subject, examInputs); err != nil {
		t.Fatal(err)
	}
	for i, studentID := range students {
		preview, err := s.SemesterFinalPreview(ctx, f.Teacher, f.Semester, f.Class, f.Subject, studentID)
		if err != nil || !preview.CanFinalize {
			t.Fatalf("siswa %d harus siap Final: %+v %v", i, preview, err)
		}
		key := fmt.Sprintf("00000000-0000-4000-8000-%012d", 1900+i)
		if err = s.FinalizeSemesterNormal(ctx, f.Teacher, f.Semester, f.Class, f.Subject, studentID, 80, "", preview.SnapshotToken, key); err != nil {
			t.Fatalf("buat Final fixture siswa %d: %v", i, err)
		}
	}

	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	csrfKey := make([]byte, 32)
	if _, err = rand.Read(csrfKey); err != nil {
		t.Fatal(err)
	}
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: s, CSRFKey: csrfKey})
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	other := auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	path := fmt.Sprintf("/guru/tujuan-pembelajaran/%d/koreksi", objectiveID)
	request := func(method, target string, values url.Values, cookies []*http.Cookie, user auth.User) *httptest.ResponseRecorder {
		body := ""
		if values != nil {
			body = values.Encode()
		}
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if denied := request(http.MethodGet, path, nil, nil, other); denied.Code != http.StatusForbidden {
		t.Fatalf("guru tanpa penugasan harus 403: %d", denied.Code)
	}
	edit := request(http.MethodGet, path, nil, nil, owner)
	if edit.Code != http.StatusOK {
		t.Fatalf("halaman koreksi: %d %s", edit.Code, edit.Body.String())
	}
	csrf := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(edit.Body.String())
	if len(csrf) != 2 {
		t.Fatal("token CSRF tidak tersedia")
	}
	definition := "Perubahan UTF-8 🧭 " + strings.Repeat("tujuan pembelajaran é漢 ", 18)
	reason := "Alasan redaksi UTF-8 🧪 " + strings.Repeat("konsistensi istilah kurikulum é ", 12)
	key := "00000000-0000-4000-8000-000000001899"
	initial := url.Values{"expected_version": {"0"}, "definition": {definition}, "change_type": {"substansi"}, "reason": {reason}, "submission_key": {key}, "gorilla.csrf.Token": {csrf[1]}}
	if invalidCSRF := request(http.MethodPost, path+"/pratinjau", initial, nil, owner); invalidCSRF.Code != http.StatusForbidden {
		t.Fatalf("navigasi/pratinjau tanpa cookie CSRF harus 403: %d", invalidCSRF.Code)
	}
	badCSRF := url.Values{}
	for name, values := range initial {
		badCSRF[name] = append([]string(nil), values...)
	}
	badCSRF.Set("gorilla.csrf.Token", "invalid-token")
	if invalidCSRF := request(http.MethodPost, path+"/pratinjau", badCSRF, edit.Result().Cookies(), owner); invalidCSRF.Code != http.StatusForbidden {
		t.Fatalf("CSRF tidak valid harus 403: %d", invalidCSRF.Code)
	}
	missingSnapshot := url.Values{}
	for name, values := range initial {
		missingSnapshot[name] = append([]string(nil), values...)
	}
	missingSnapshot.Set("page", "1")
	if navWithoutSnapshot := request(http.MethodPost, path+"/pratinjau", missingSnapshot, edit.Result().Cookies(), owner); navWithoutSnapshot.Code != http.StatusConflict {
		t.Fatalf("navigasi halaman tanpa token snapshot harus 409: %d", navWithoutSnapshot.Code)
	}
	first := request(http.MethodPost, path+"/pratinjau", initial, edit.Result().Cookies(), owner)
	firstBody := html.UnescapeString(first.Body.String())
	if first.Code != http.StatusOK || !strings.Contains(firstBody, "Halaman 1 dari 2") || !strings.Contains(firstBody, strings.TrimSpace(definition)) {
		t.Fatalf("pratinjau halaman pertama: status=%d page=%v definition=%v", first.Code, strings.Contains(firstBody, "Halaman 1 dari 2"), strings.Contains(firstBody, strings.TrimSpace(definition)))
	}
	if strings.Contains(first.Body.String(), `href="/guru/tujuan-pembelajaran/`+fmt.Sprint(objectiveID)+`/koreksi/pratinjau?`) {
		t.Fatal("tautan pagination GET dengan data koreksi di URL tidak boleh muncul")
	}
	if !strings.Contains(first.Body.String(), `form method="post" action="/guru/tujuan-pembelajaran/`+fmt.Sprint(objectiveID)+`/koreksi/pratinjau"`) {
		t.Fatal("pagination harus berupa form POST ke endpoint pratinjau")
	}
	pageSnapshot := regexp.MustCompile(`name="snapshot_token" value="([^"]+)"`).FindStringSubmatch(first.Body.String())
	if len(pageSnapshot) != 2 {
		t.Fatal("snapshot token halaman pertama tidak ditemukan")
	}
	pageForm := func(page int, token string) url.Values {
		return url.Values{"expected_version": {"0"}, "definition": {definition}, "change_type": {"substansi"}, "reason": {reason}, "snapshot_token": {token}, "submission_key": {key}, "page": {fmt.Sprint(page)}, "gorilla.csrf.Token": {csrf[1]}}
	}
	if denied := request(http.MethodPost, path+"/pratinjau", pageForm(2, pageSnapshot[1]), edit.Result().Cookies(), other); denied.Code != http.StatusForbidden {
		t.Fatalf("guru tanpa penugasan harus ditolak saat navigasi POST: %d", denied.Code)
	}
	second := request(http.MethodPost, path+"/pratinjau", pageForm(2, pageSnapshot[1]), edit.Result().Cookies(), owner)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "Halaman 2 dari 2") || !strings.Contains(second.Body.String(), "CP18 Affected 10") {
		t.Fatalf("navigasi maju dengan POST: %d %s", second.Code, second.Body.String())
	}
	if strings.Contains(second.Header().Get("Location"), definition) || strings.Contains(second.Header().Get("Location"), reason) || strings.Contains(second.Header().Get("Location"), key) {
		t.Fatal("URL respons tidak boleh berisi data koreksi")
	}
	backSnapshot := regexp.MustCompile(`name="snapshot_token" value="([^"]+)"`).FindStringSubmatch(second.Body.String())
	if len(backSnapshot) != 2 {
		t.Fatal("snapshot token halaman kedua tidak ditemukan")
	}
	back := request(http.MethodPost, path+"/pratinjau", pageForm(1, backSnapshot[1]), edit.Result().Cookies(), owner)
	if back.Code != http.StatusOK || !strings.Contains(back.Body.String(), "Halaman 1 dari 2") || !strings.Contains(back.Body.String(), "CP18 Affected 00") {
		t.Fatalf("navigasi mundur dengan POST: %d %s", back.Code, back.Body.String())
	}
	// Nilai berubah setelah pratinjau: navigasi harus menolak snapshot basi,
	// bukan diam-diam menghasilkan token baru untuk bukti yang belum dilihat.
	var scoreID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2 AND learning_objective_id=$3`, activityID, students[0], objectiveID).Scan(&scoreID); err != nil {
		t.Fatal(err)
	}
	if err = s.CorrectAssessmentInitialScore(ctx, f.Teacher, scoreID, 0, "81", "Perubahan sumber untuk uji snapshot basi"); err != nil {
		t.Fatal(err)
	}
	stale := request(http.MethodPost, path+"/pratinjau", pageForm(2, backSnapshot[1]), edit.Result().Cookies(), owner)
	if stale.Code != http.StatusConflict {
		t.Fatalf("snapshot pagination basi harus 409: %d %s", stale.Code, stale.Body.String())
	}
	var revisionCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.learning_objective_definition_revisions WHERE learning_objective_id=$1`, objectiveID).Scan(&revisionCount); err != nil || revisionCount != 0 {
		t.Fatalf("preview/navigation tidak boleh mengubah histori TP: %d %v", revisionCount, err)
	}
}
