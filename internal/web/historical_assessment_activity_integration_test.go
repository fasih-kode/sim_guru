//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
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

func TestHistoricalAssessmentActivityHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	f := testutil.ExamFixtureFor(t, db, "cp22http"+testutil.Random(t)[:8])
	store := &academic.Store{DB: db}
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'2020-01-01','2020-12-31','[1,2,3,4,5]')`, f.Semester)
	exec(`INSERT INTO app.semester_assessment_weights(semester_id,nh,asts,asas,actor_id) VALUES($1,0,40,60,$2)`, f.Semester, f.Teacher)
	var year int64
	if err := db.QueryRow(`SELECT school_year_id FROM app.semesters WHERE id=$1`, f.Semester).Scan(&year); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateLearningObjective(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "TP rutin untuk uji kompatibilitas"); err != nil {
		t.Fatal(err)
	}
	var routineObjectiveID int64
	if err := db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND definition='TP rutin untuk uji kompatibilitas'`, f.Semester).Scan(&routineObjectiveID); err != nil {
		t.Fatal(err)
	}
	finalHash := sha256.Sum256([]byte("cp22-http-final"))
	exec(`INSERT INTO app.semester_final_versions(school_year_id,semester_id,class_id,subject_id,student_id,teacher_id,recorded_by,version,recommendation_score,final_score,snapshot,snapshot_sha256,submission_key,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,1,80,80,'{}',$8,'00000000-0000-4000-8000-000000002255',$8)`, year, f.Semester, f.Class, f.Subject, f.StudentA, f.TeacherID, f.Teacher, finalHash[:])

	// Dua TP dibuat lewat alur provenance resmi agar pilihan HTTP tidak dapat
	// memuat TP rutin yang bukan objek sasaran CP10-22.
	objectiveIDs := make([]int64, 0, 2)
	for i := 1; i <= 2; i++ {
		in := academic.HistoricalTPPreviewInput{
			SemesterID: f.Semester, ClassID: f.Class, SubjectID: f.Subject,
			Definition: fmt.Sprintf("TP historis HTTP %d", i), Reason: "Arsip kegiatan historis untuk regresi HTTP.",
			AssessmentDate: "2020-01-10", DateSource: "register sintetis",
			Learning:   academic.HistoricalTPReference{Kind: "manual", Document: fmt.Sprintf("Jurnal CP22 HTTP %d", i), Archive: "fixture TEST", Teacher: "Guru fixture", Date: "2020-01-09"},
			Assessment: academic.HistoricalTPReference{Kind: "manual", Document: fmt.Sprintf("Sumatif CP22 HTTP %d", i), Archive: "fixture TEST", Teacher: "Guru fixture", ResultSource: "lembar hasil sintetis"},
		}
		preview, err := store.PreviewHistoricalTP(ctx, f.Teacher, in, 1, "")
		if err != nil {
			t.Fatal(err)
		}
		id, err := store.ConfirmHistoricalLearningObjective(ctx, f.Teacher, academic.HistoricalTPConfirmation{
			Preview: in, Verification: academic.HistoricalTPVerification{LearningSection: "Butir 1", AssessmentSection: "Kompetensi TP", Method: "mencocokkan dokumen sintetis", ResultSource: "lembar hasil", Acknowledged: true},
			SnapshotToken: preview.SnapshotToken, SubmissionKey: fmt.Sprintf("00000000-0000-4000-8000-00000000225%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		objectiveIDs = append(objectiveIDs, id)
	}
	for i := 0; i < 9; i++ {
		var studentID int64
		if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','L') RETURNING id`, fmt.Sprintf("Siswa CP22 HTTP %s %02d", testutil.Random(t)[:5], i)).Scan(&studentID); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,'2020-01-01')`, year, studentID, f.Class, f.Teacher)
	}

	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	csrfKey := make([]byte, 32)
	if _, err = rand.Read(csrfKey); err != nil {
		t.Fatal(err)
	}
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: store, CSRFKey: csrfKey})
	if err != nil {
		t.Fatal(err)
	}
	user := auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	request := func(method, path string, values url.Values, cookies []*http.Cookie, htmx bool) *httptest.ResponseRecorder {
		t.Helper()
		body := ""
		if values != nil {
			body = values.Encode()
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &user))
		response := httptest.NewRecorder()
		h.ServeHTTP(response, req)
		return response
	}
	field := func(body, name string) string {
		t.Helper()
		match := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`).FindStringSubmatch(body)
		if len(match) != 2 {
			t.Fatalf("field %s tidak ditemukan", name)
		}
		return match[1]
	}
	path := historicalAssessmentActivityPath
	page := request(http.MethodGet, path, nil, nil, false)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Koreksi struktur historis") || !strings.Contains(page.Body.String(), "Tujuan Pembelajaran") || !strings.Contains(page.Body.String(), `aria-current="page"`) || !strings.Contains(page.Body.String(), "/guru/kegiatan-penilaian") {
		t.Fatalf("GET halaman: %d", page.Code)
	}
	csrfToken := field(page.Body.String(), "gorilla.csrf.Token")
	cookies := page.Result().Cookies()
	contextValues := url.Values{"gorilla.csrf.Token": {csrfToken}, "action": {"context"}, "context": {fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)}}
	noCSRF := request(http.MethodPost, path, url.Values{"action": {"context"}}, nil, false)
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("POST tanpa CSRF: %d", noCSRF.Code)
	}
	if denied := request(http.MethodGet, path, nil, nil, false); denied.Code != http.StatusOK {
		t.Fatalf("GET guru sah: %d", denied.Code)
	}
	other := user
	user = auth.User{ID: f.Other, Active: true, Roles: []string{"admin"}}
	if denied := request(http.MethodGet, path, nil, nil, false); denied.Code != http.StatusForbidden {
		t.Fatalf("role tanpa Guru Mapel: %d", denied.Code)
	}
	user = other
	user = auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	if denied := request(http.MethodPost, path, contextValues, cookies, false); denied.Code != http.StatusForbidden {
		t.Fatalf("konteks penugasan guru lain: %d", denied.Code)
	}
	user = other

	choices := request(http.MethodPost, path, contextValues, cookies, false)
	if choices.Code != http.StatusOK || !strings.Contains(choices.Body.String(), "TP historis HTTP 1") || !strings.Contains(choices.Body.String(), "TP historis HTTP 2") {
		t.Fatalf("pilihan konteks/TP: %d", choices.Code)
	}
	if !strings.Contains(choices.Body.String(), "TP rutin untuk uji kompatibilitas") {
		t.Fatal("pilihan TP CP10-22 tidak memuat TP rutin yang sah")
	}
	var sourceTeacherName string
	if err := db.QueryRow(`SELECT name FROM app.teachers WHERE id=$1`, f.TeacherID).Scan(&sourceTeacherName); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"gorilla.csrf.Token": {csrfToken}, "action": {"preview"}, "context": {fmt.Sprintf("%d:%d:%d", f.Semester, f.Class, f.Subject)},
		"objective_ids": {fmt.Sprint(routineObjectiveID), fmt.Sprint(objectiveIDs[0]), fmt.Sprint(objectiveIDs[1])}, "name": {"Kegiatan CP22 HTTP sintetis"}, "activity_date": {"2020-01-10"},
		"reason": {"Pencatatan kegiatan berdasarkan dokumen fixture sintetis."}, "source_identity": {"source-event-cp22-http-" + testutil.Random(t)[:8]},
		"source_document": {"Dokumen Sumatif sintetis"}, "source_archive": {"arsip fixture TEST"}, "source_teacher": {sourceTeacherName},
		"source_section": {"Bagian kompetensi A dan B yang menunjukkan dua TP dinilai."}, "result_source": {"Lembar hasil fixture"},
		"method": {"Mencocokkan kegiatan dan hasil dengan TP"}, "acknowledged": {"yes"},
	}
	wrongTeacherSource := cloneHistoricalActivityValues(form)
	wrongTeacherSource.Set("source_teacher", "Guru lain yang tidak memiliki penugasan")
	if mismatch := request(http.MethodPost, path, wrongTeacherSource, cookies, false); mismatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("metadata guru sumber yang berbeda harus ditolak: %d", mismatch.Code)
	}
	preview := request(http.MethodPost, path, form, cookies, false)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), "Tinjau dampak sebelum konfirmasi") || !strings.Contains(preview.Body.String(), "Semua kandidat siswa") || !strings.Contains(preview.Body.String(), "12") {
		t.Fatalf("preview multi-TP: %d", preview.Code)
	}
	if strings.Contains(preview.Header().Get("Location"), form.Get("source_identity")) || strings.Contains(preview.Header().Get("Location"), form.Get("source_document")) {
		t.Fatal("data sumber muncul dalam URL")
	}
	token, submission := field(preview.Body.String(), "snapshot_token"), field(preview.Body.String(), "submission_key")
	invalidPreview := cloneHistoricalActivityValues(form)
	invalidPreview.Set("reason", " ")
	if invalid := request(http.MethodPost, path, invalidPreview, cookies, false); invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("validasi alasan kosong: %d", invalid.Code)
	}
	pageTwo := cloneHistoricalActivityValues(form)
	pageTwo.Set("page", "2")
	pageTwo.Set("snapshot_token", token)
	pageTwo.Set("submission_key", submission)
	pageTwo.Set("action", "preview")
	pageTwoResponse := request(http.MethodPost, path, pageTwo, cookies, true)
	if pageTwoResponse.Code != http.StatusOK || !strings.Contains(pageTwoResponse.Body.String(), "Halaman 2 dari 2") || !strings.Contains(pageTwoResponse.Body.String(), `name="source_identity"`) {
		t.Fatalf("pagination HTMX POST: %d", pageTwoResponse.Code)
	}
	if strings.Contains(pageTwoResponse.Header().Get("Location"), "source_identity") {
		t.Fatal("metadata sumber terselip pada URL pagination")
	}
	pageOne := cloneHistoricalActivityValues(pageTwo)
	pageOne.Set("page", "1")
	pageOneResponse := request(http.MethodPost, path, pageOne, cookies, true)
	if pageOneResponse.Code != http.StatusOK || !strings.Contains(pageOneResponse.Body.String(), "Halaman 1 dari 2") || !strings.Contains(pageOneResponse.Body.String(), "Semua kandidat siswa") || !strings.Contains(pageOneResponse.Body.String(), "12") {
		t.Fatalf("pagination HTMX kembali: %d", pageOneResponse.Code)
	}

	confirm := cloneHistoricalActivityValues(form)
	confirm.Set("action", "confirm")
	confirm.Set("snapshot_token", token)
	confirm.Set("submission_key", submission)
	confirm.Set("gorilla.csrf.Token", csrfToken)
	exec(`UPDATE app.semester_assessment_weights SET nh=10,asts=40,asas=50 WHERE semester_id=$1`, f.Semester)
	stale := request(http.MethodPost, path, confirm, cookies, false)
	if stale.Code != http.StatusConflict {
		t.Fatalf("snapshot basi: %d", stale.Code)
	}
	// Form tetap membawa input valid; pratinjau baru memberi fingerprint baru.
	preview2 := request(http.MethodPost, path, form, cookies, false)
	if preview2.Code != http.StatusOK {
		t.Fatalf("pratinjau ulang: %d", preview2.Code)
	}
	confirm.Set("snapshot_token", field(preview2.Body.String(), "snapshot_token"))
	confirm.Set("submission_key", field(preview2.Body.String(), "submission_key"))
	saved := request(http.MethodPost, path, confirm, cookies, false)
	if saved.Code != http.StatusSeeOther || !strings.HasPrefix(saved.Header().Get("Location"), historicalAssessmentActivityPath+"/hasil?id=") {
		t.Fatalf("konfirmasi PRG: %d %s", saved.Code, saved.Header().Get("Location"))
	}
	location := saved.Header().Get("Location")
	if strings.Contains(location, form.Get("source_identity")) || strings.Contains(location, form.Get("reason")) {
		t.Fatal("data bukti/reason masuk URL hasil")
	}
	result := request(http.MethodGet, location, nil, cookies, false)
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "Kegiatan historis berhasil dicatat") || !strings.Contains(result.Body.String(), "nilai siswa yang dibuat") {
		t.Fatalf("halaman hasil: %d", result.Code)
	}
	replay := request(http.MethodPost, path, confirm, cookies, false)
	if replay.Code != http.StatusSeeOther || replay.Header().Get("Location") != location {
		t.Fatalf("replay identik: %d", replay.Code)
	}
	changedPayload := cloneHistoricalActivityValues(confirm)
	changedPayload.Set("reason", "Payload berbeda untuk UUID yang sama")
	if conflict := request(http.MethodPost, path, changedPayload, cookies, false); conflict.Code != http.StatusConflict {
		t.Fatalf("replay berbeda: %d", conflict.Code)
	}
	user = auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	if denied := request(http.MethodGet, location, nil, cookies, false); denied.Code != http.StatusForbidden {
		t.Fatalf("halaman hasil guru lain: %d", denied.Code)
	}
	user = auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	var activityID int64
	if err := db.QueryRow(`SELECT activity_id FROM app.historical_assessment_activity_insertions WHERE source_identity=$1`, strings.ToLower(strings.Join(strings.Fields(form.Get("source_identity")), " "))).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	scorePage := request(http.MethodGet, fmt.Sprintf("/guru/kegiatan-penilaian/%d/nilai", activityID), nil, cookies, false)
	if scorePage.Code != http.StatusOK || !strings.Contains(scorePage.Body.String(), "Pencatatan nilai untuk relasi historis memerlukan proses tersendiri") {
		t.Fatalf("nilai historis terkunci pada UI: %d", scorePage.Code)
	}
	scorePost := cloneHistoricalActivityValues(url.Values{"gorilla.csrf.Token": {csrfToken}, fmt.Sprintf("score_%d_%d", f.StudentA, objectiveIDs[0]): {"80"}})
	locked := request(http.MethodPost, fmt.Sprintf("/guru/kegiatan-penilaian/%d/nilai", activityID), scorePost, cookies, false)
	if locked.Code != http.StatusUnprocessableEntity || !strings.Contains(locked.Body.String(), "proses pencatatan historis tersendiri") {
		t.Fatalf("POST nilai untuk pasangan historis: %d", locked.Code)
	}

	// CP10-23 memakai endpoint khusus; nilai 0 sah, ber-CSRF, dan tidak
	// mengubah Final atau snapshot lama.
	historicalScorePath := historicalAssessmentScoresPath
	scoreLanding := request(http.MethodGet, historicalScorePath, nil, nil, false)
	if scoreLanding.Code != http.StatusOK || !strings.Contains(scoreLanding.Body.String(), "Pencatatan Nilai Sumatif Historis") || !strings.Contains(scoreLanding.Body.String(), "Kegiatan Sumatif historis") {
		t.Fatalf("GET pencatatan nilai historis: %d", scoreLanding.Code)
	}
	if deniedCSRF := request(http.MethodPost, historicalScorePath, url.Values{"action": {"load"}, "activity_id": {fmt.Sprint(activityID)}}, cookies, false); deniedCSRF.Code != http.StatusForbidden {
		t.Fatalf("POST nilai historis tanpa token CSRF: %d", deniedCSRF.Code)
	}
	loadScores := url.Values{"gorilla.csrf.Token": {csrfToken}, "action": {"load"}, "activity_id": {fmt.Sprint(activityID)}}
	loadedScores := request(http.MethodPost, historicalScorePath, loadScores, cookies, false)
	if loadedScores.Code != http.StatusOK || !strings.Contains(loadedScores.Body.String(), "Siswa dan relevansi") || !strings.Contains(loadedScores.Body.String(), "entry_value") || !strings.Contains(loadedScores.Body.String(), "Halaman 1 dari 2") {
		t.Fatalf("muat lembar nilai historis: %d", loadedScores.Code)
	}
	pageTwoValues := url.Values{"gorilla.csrf.Token": {csrfToken}, "action": {"preview"}, "activity_id": {fmt.Sprint(activityID)}, "page_override": {"2"}, "source_fingerprint": {field(loadedScores.Body.String(), "source_fingerprint")}, "submission_key": {field(loadedScores.Body.String(), "submission_key")}}
	pageTwoScores := request(http.MethodPost, historicalScorePath, pageTwoValues, cookies, true)
	if pageTwoScores.Code != http.StatusOK || !strings.Contains(pageTwoScores.Body.String(), "Halaman 2 dari 2") {
		t.Fatalf("pagination nilai historis maju: %d", pageTwoScores.Code)
	}
	pageOneValues := cloneHistoricalActivityValues(pageTwoValues)
	pageOneValues.Set("page_override", "1")
	pageOneScores := request(http.MethodPost, historicalScorePath, pageOneValues, cookies, true)
	if pageOneScores.Code != http.StatusOK || !strings.Contains(pageOneScores.Body.String(), "Halaman 1 dari 2") || !strings.Contains(pageOneScores.Body.String(), "Semua siswa teridentifikasi") {
		t.Fatalf("pagination nilai historis mundur: %d", pageOneScores.Code)
	}
	scoreForm := url.Values{
		"gorilla.csrf.Token": {csrfToken}, "action": {"preview"}, "activity_id": {fmt.Sprint(activityID)},
		"reason": {"Transkripsi dari dokumen nilai sintetis"}, "verification": {"Nama siswa, TP, nilai, dan lokasi sumber dicocokkan"}, "acknowledged": {"yes"},
		"entry_student_id": {fmt.Sprint(f.StudentA)}, "entry_objective_id": {fmt.Sprint(objectiveIDs[0])}, "entry_value": {"0"},
		"entry_source_kind": {"arsip_manual"}, "entry_source_identity": {"Arsip Sumatif CP10-23 HTTP"}, "entry_source_locator": {"halaman 2, baris 4, siswa A, TP 1"},
		"entry_source_result": {"nilai 0 pada kolom TP 1"}, "entry_verification": {"nama dan nomor siswa cocok dengan daftar kelas"},
	}
	invalidScore := cloneHistoricalActivityValues(scoreForm)
	invalidScore.Set("entry_value", "100.01")
	if invalid := request(http.MethodPost, historicalScorePath, invalidScore, cookies, false); invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("nilai di luar rentang tidak ditolak: %d", invalid.Code)
	}
	scoreForm.Set("source_fingerprint", field(loadedScores.Body.String(), "source_fingerprint"))
	scorePreview := request(http.MethodPost, historicalScorePath, scoreForm, cookies, false)
	if scorePreview.Code != http.StatusOK || !strings.Contains(scorePreview.Body.String(), "0") || !strings.Contains(scorePreview.Body.String(), "Konfirmasi pencatatan nilai") || !strings.Contains(scorePreview.Body.String(), "setelah nilai terpilih dicatat: Perlu Ditinjau") {
		t.Fatalf("pratinjau nilai historis: %d %s", scorePreview.Code, scorePreview.Body.String())
	}
	if strings.Contains(scorePreview.Header().Get("Location"), "Arsip Sumatif") {
		t.Fatal("identitas sumber nilai muncul dalam URL")
	}
	scoreConfirm := cloneHistoricalActivityValues(scoreForm)
	scoreConfirm.Set("action", "confirm")
	scoreConfirm.Set("snapshot_token", field(scorePreview.Body.String(), "snapshot_token"))
	scoreConfirm.Set("submission_key", field(scorePreview.Body.String(), "submission_key"))
	exec(`UPDATE app.semester_calendars SET starts_on='2020-01-02' WHERE semester_id=$1`, f.Semester)
	staleScore := request(http.MethodPost, historicalScorePath, scoreConfirm, cookies, false)
	if staleScore.Code != http.StatusConflict {
		t.Fatalf("snapshot nilai historis basi: %d", staleScore.Code)
	}
	var partialScores, partialBatches, partialAudits int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_score_batches WHERE assessment_activity_id=$1),(SELECT count(*) FROM app.historical_assessment_initial_score_audits WHERE assessment_activity_id=$1)`, activityID).Scan(&partialScores, &partialBatches, &partialAudits); err != nil || partialScores != 0 || partialBatches != 0 || partialAudits != 0 {
		t.Fatalf("konflik meninggalkan data parsial: scores=%d batches=%d audits=%d err=%v", partialScores, partialBatches, partialAudits, err)
	}
	refreshScore := cloneHistoricalActivityValues(scoreForm)
	refreshScore.Set("action", "load")
	refreshedScorePreview := request(http.MethodPost, historicalScorePath, refreshScore, cookies, false)
	if refreshedScorePreview.Code != http.StatusOK {
		t.Fatalf("refresh pratinjau setelah snapshot basi: %d", refreshedScorePreview.Code)
	}
	scoreConfirm.Set("snapshot_token", field(refreshedScorePreview.Body.String(), "snapshot_token"))
	scoreSaved := request(http.MethodPost, historicalScorePath, scoreConfirm, cookies, false)
	if scoreSaved.Code != http.StatusSeeOther || !strings.HasPrefix(scoreSaved.Header().Get("Location"), historicalAssessmentScoresPath+"/hasil?id=") {
		t.Fatalf("konfirmasi nilai historis PRG: %d %s", scoreSaved.Code, scoreSaved.Body.String())
	}
	scoreResult := request(http.MethodGet, scoreSaved.Header().Get("Location"), nil, cookies, false)
	if scoreResult.Code != http.StatusOK || !strings.Contains(scoreResult.Body.String(), "Nilai historis berhasil dicatat") || !strings.Contains(scoreResult.Body.String(), "0.00") || !strings.Contains(scoreResult.Body.String(), "Arsip Sumatif CP10-23 HTTP") {
		t.Fatalf("hasil nilai historis: %d %s", scoreResult.Code, scoreResult.Body.String())
	}
	if replay := request(http.MethodPost, historicalScorePath, scoreConfirm, cookies, false); replay.Code != http.StatusSeeOther || replay.Header().Get("Location") != scoreSaved.Header().Get("Location") {
		t.Fatalf("replay nilai historis: %d %s", replay.Code, replay.Header().Get("Location"))
	}
	changedScorePayload := cloneHistoricalActivityValues(scoreConfirm)
	changedScorePayload.Set("entry_value", "1")
	if changed := request(http.MethodPost, historicalScorePath, changedScorePayload, cookies, false); changed.Code != http.StatusConflict {
		t.Fatalf("replay payload berbeda: %d", changed.Code)
	}
	user = auth.User{ID: f.Other, Active: true, Roles: []string{"guru_mapel"}}
	if denied := request(http.MethodGet, scoreSaved.Header().Get("Location"), nil, cookies, false); denied.Code != http.StatusForbidden {
		t.Fatalf("hasil nilai historis guru lain: %d", denied.Code)
	}
	user = auth.User{ID: f.Teacher, Active: true, Roles: []string{"guru_mapel"}}
	if err := store.CreateAssessmentActivity(ctx, f.Teacher, f.Semester, f.Class, f.Subject, "Formatif biasa setelah Final", "formatif", "2020-01-10", []int64{routineObjectiveID}); err != nil {
		t.Fatalf("Formatif rutin sesudah Final tetap sah: %v", err)
	}
	var routineActivityID int64
	if err := db.QueryRow(`SELECT id FROM app.assessment_activities WHERE semester_id=$1 AND name='Formatif biasa setelah Final'`, f.Semester).Scan(&routineActivityID); err != nil {
		t.Fatal(err)
	}
	routineSheet := request(http.MethodGet, fmt.Sprintf("/guru/kegiatan-penilaian/%d/nilai", routineActivityID), nil, cookies, false)
	if routineSheet.Code != http.StatusOK || strings.Contains(routineSheet.Body.String(), "Pencatatan nilai untuk relasi historis memerlukan proses tersendiri") {
		t.Fatalf("lembar nilai rutin: %d", routineSheet.Code)
	}
	routineValues := url.Values{"gorilla.csrf.Token": {csrfToken}, fmt.Sprintf("score_%d_%d", f.StudentA, routineObjectiveID): {"0"}}
	routineSaved := request(http.MethodPost, fmt.Sprintf("/guru/kegiatan-penilaian/%d/nilai", routineActivityID), routineValues, cookies, false)
	if routineSaved.Code != http.StatusSeeOther {
		t.Fatalf("nilai rutin setelah Final: %d %s", routineSaved.Code, routineSaved.Body.String())
	}
	var auditCount, relationCount, scoreCount, finalCount, finalVersion int
	var finalSnapshot, storedFinalHash []byte
	if err := db.QueryRow(`SELECT count(*) FROM app.historical_assessment_activity_insertions WHERE activity_id=$1`, activityID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.assessment_activity_objectives WHERE activity_id=$1`, activityID).Scan(&relationCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, activityID).Scan(&scoreCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, f.Semester, f.Class, f.Subject).Scan(&finalCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT version,snapshot,snapshot_sha256 FROM app.semester_final_versions WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3 AND student_id=$4`, f.Semester, f.Class, f.Subject, f.StudentA).Scan(&finalVersion, &finalSnapshot, &storedFinalHash); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 || relationCount != 3 || scoreCount != 1 || finalCount != 1 || finalVersion != 1 || string(finalSnapshot) != "{}" || string(storedFinalHash) != string(finalHash[:]) {
		t.Fatalf("integritas pasca-HTTP: audit=%d relasi=%d nilai=%d final=%d versi=%d snapshot=%s hash=%x", auditCount, relationCount, scoreCount, finalCount, finalVersion, finalSnapshot, storedFinalHash)
	}
	var historicalScore string
	var historicalAuditCount int
	if err := db.QueryRow(`SELECT s.score::text,(SELECT count(*) FROM app.historical_assessment_initial_score_audits a WHERE a.assessment_activity_id=s.assessment_activity_id AND a.learning_objective_id=s.learning_objective_id AND a.student_id=s.student_id) FROM app.assessment_activity_initial_scores s WHERE s.assessment_activity_id=$1 AND s.learning_objective_id=$2 AND s.student_id=$3`, activityID, objectiveIDs[0], f.StudentA).Scan(&historicalScore, &historicalAuditCount); err != nil || historicalScore != "0.00" || historicalAuditCount != 1 {
		t.Fatalf("nilai historis dan audit: value=%s audits=%d err=%v", historicalScore, historicalAuditCount, err)
	}
	var routineScore string
	if err := db.QueryRow(`SELECT score::text FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1 AND student_id=$2 AND learning_objective_id=$3`, routineActivityID, f.StudentA, routineObjectiveID).Scan(&routineScore); err != nil || routineScore != "0.00" {
		t.Fatalf("nilai rutin 0 harus tersimpan mandiri: score=%s err=%v", routineScore, err)
	}
}

func cloneHistoricalActivityValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, entries := range values {
		out[key] = append([]string(nil), entries...)
	}
	return out
}
