//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAssessmentScoreCorrectionHTTPAuthorizationHistoryAndCSRFIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	owner, ownerPassword := testutil.User(t, db, "guru_mapel")
	newTeacher, newPassword := testutil.User(t, db, "guru_mapel")
	suffix := testutil.Random(t)[:10]
	var yearID, semesterID, classID, subjectID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "Koreksi nilai CP10-06 "+suffix).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "K"+suffix[:6]).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "C06-"+suffix, "Mapel koreksi "+suffix).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	masters := &master.Store{DB: db}
	ownerIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Pemilik nilai " + suffix, UserID: owner.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	newIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Pengampu baru " + suffix, UserID: newTeacher.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	service := &academic.Store{DB: db}
	assignmentStart := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -20)
	if err = service.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, ownerIdentity.ID, assignmentStart); err != nil {
		t.Fatal("penugasan pemilik awal", err)
	}
	var objectiveID int64
	if err = service.CreateLearningObjective(ctx, owner.ID, semesterID, classID, subjectID, "TP koreksi "+suffix); err != nil {
		t.Fatal("buat TP fixture", err)
	}
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semesterID, classID, subjectID).Scan(&objectiveID); err != nil {
		t.Fatal(err)
	}
	activityDate := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	if err = service.CreateAssessmentActivity(ctx, owner.ID, semesterID, classID, subjectID, "Sumatif koreksi "+suffix, "sumatif", activityDate, []int64{objectiveID}); err != nil {
		t.Fatal("buat kegiatan fixture", err)
	}
	var activityID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1`, yearID).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	var studentID, placementID int64
	if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','L') RETURNING id`, "Siswa koreksi "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date-10) RETURNING id`, yearID, studentID, classID, admin.ID, activityDate).Scan(&placementID); err != nil {
		t.Fatal(err)
	}
	if err = service.SaveAssessmentActivityInitialScores(ctx, owner.ID, activityID, []academic.AssessmentScoreInput{{StudentID: studentID, ObjectiveID: objectiveID, Value: "82.50"}}); err != nil {
		t.Fatal("simpan nilai awal", err)
	}
	var scoreID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, activityID).Scan(&scoreID); err != nil {
		t.Fatal(err)
	}
	if err = service.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, newIdentity.ID, time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -1)); err != nil {
		t.Fatal("pengalihan penugasan", err)
	}
	if err = service.CreateAssessmentActivity(ctx, newTeacher.ID, semesterID, classID, subjectID, "Kegiatan baru "+suffix, "formatif", activityDate, []int64{objectiveID}); err != nil {
		t.Fatal("kegiatan pengampu baru", err)
	}
	var newActivityID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND teacher_id=$2`, yearID, newIdentity.ID).Scan(&newActivityID); err != nil {
		t.Fatal(err)
	}
	if err = service.SaveAssessmentActivityInitialScores(ctx, newTeacher.ID, newActivityID, []academic.AssessmentScoreInput{{StudentID: studentID, ObjectiveID: objectiveID, Value: "75"}}); err != nil {
		t.Fatal("nilai awal kegiatan guru baru", err)
	}
	t.Cleanup(func() {
		var immutableCorrections int
		if cleanupErr := db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_corrections c
			JOIN app.assessment_activity_initial_scores sc ON sc.id=c.initial_score_id
			JOIN app.assessment_activities a ON a.id=sc.assessment_activity_id WHERE a.school_year_id=$1`, yearID).Scan(&immutableCorrections); cleanupErr != nil {
			t.Errorf("periksa histori koreksi fixture: %v", cleanupErr)
			return
		}
		if immutableCorrections > 0 {
			t.Log("fixture dipertahankan karena histori koreksi append-only")
			return
		}
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_score_corrections WHERE initial_score_id IN (SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1))`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_initial_scores WHERE assessment_activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_objectives WHERE activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activities WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE school_year_id=$1`, yearID)
		if placementID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, placementID)
		}
		if studentID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE student_id=$1`, studentID)
			_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, studentID)
		}
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id=$1)`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id=$1`, semesterID)
		for _, identityID := range []int64{ownerIdentity.ID, newIdentity.ID} {
			_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, identityID)
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, identityID)
		}
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code=$1`, "C06-"+suffix)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id=$1`, subjectID)
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.classes WHERE id=$1`, classID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		for _, userID := range []int64{admin.ID, curriculum.ID, owner.ID, newTeacher.ID} {
			for _, query := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, cleanupErr := db.Exec(query, userID); cleanupErr != nil {
					t.Errorf("cleanup fixture CP10-06: %v", cleanupErr)
					return
				}
			}
		}
	})

	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: masters, Academics: service, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	csrfPattern := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	login := func(user auth.User, password string) (*http.Client, string) {
		t.Helper()
		jar, jarErr := cookiejar.New(nil)
		if jarErr != nil {
			t.Fatal(jarErr)
		}
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, getErr := client.Get(server.URL + "/login")
		if getErr != nil {
			t.Fatal(getErr)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("login GET gagal: status=%d err=%v", response.StatusCode, readErr)
		}
		match := csrfPattern.FindSubmatch(body)
		if len(match) != 2 {
			t.Fatal("token CSRF tidak ditemukan")
		}
		token, loginErr := accounts.Login(ctx, user.Username, password, "")
		if loginErr != nil {
			t.Fatal("login fixture gagal", loginErr)
		}
		baseURL, _ := url.Parse(server.URL)
		jar.SetCookies(baseURL, []*http.Cookie{{Name: sessionCookie, Value: token, Path: "/"}})
		return client, string(match[1])
	}
	read := func(response *http.Response) string {
		t.Helper()
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		return string(body)
	}
	ownerClient, ownerCSRF := login(owner, ownerPassword)
	newClient, newCSRF := login(newTeacher, newPassword)
	list, err := ownerClient.Get(server.URL + "/guru/nilai-awal-saya?q=" + url.QueryEscape("Siswa koreksi"))
	if err != nil || list.StatusCode != http.StatusOK || !strings.Contains(read(list), "Koreksi") || !strings.Contains(list.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("pemilik historis tidak dapat menemukan nilainya: response=%v err=%v", list, err)
	}
	correctionPath := "/guru/nilai-awal/" + strconv.FormatInt(scoreID, 10) + "/koreksi"
	page, err := ownerClient.Get(server.URL + correctionPath)
	if err != nil || page.StatusCode != http.StatusOK {
		t.Fatalf("GET form koreksi pemilik gagal: status=%v err=%v", page, err)
	}
	pageBody := read(page)
	if !strings.Contains(pageBody, "82.50") || !strings.Contains(pageBody, "Riwayat Perubahan") || !strings.Contains(pageBody, `name="expected_version" value="0"`) {
		t.Fatal("form koreksi tidak menampilkan nilai awal, nilai berlaku, histori, dan expected version")
	}
	newGet, err := newClient.Get(server.URL + correctionPath)
	if err != nil || newGet.StatusCode != http.StatusNotFound {
		t.Fatalf("pengampu baru tidak boleh menemukan form koreksi pemilik lama: status=%v err=%v", newGet, err)
	}
	withoutCSRF, err := ownerClient.PostForm(server.URL+correctionPath, url.Values{"expected_version": {"0"}, "value": {"86"}, "reason": {"Koreksi"}})
	if err != nil || withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("POST tanpa CSRF harus ditolak: response=%v err=%v", withoutCSRF, err)
	}
	denied, err := newClient.PostForm(server.URL+correctionPath, url.Values{"gorilla.csrf.Token": {newCSRF}, "expected_version": {"0"}, "value": {"86"}, "reason": {"Percobaan lintas guru"}})
	if err != nil || denied.StatusCode != http.StatusForbidden {
		t.Fatalf("POST pengampu baru harus ditolak 403: response=%v err=%v", denied, err)
	}
	invalid, err := ownerClient.PostForm(server.URL+correctionPath, url.Values{"gorilla.csrf.Token": {ownerCSRF}, "expected_version": {"0"}, "value": {"90"}, "reason": {" "}})
	if err != nil || invalid.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("alasan kosong harus ditolak: response=%v err=%v", invalid, err)
	}
	invalidBody := read(invalid)
	if !strings.Contains(invalidBody, "Alasan koreksi wajib diisi") || !strings.Contains(invalidBody, `value="90"`) {
		t.Fatal("pesan alasan kosong atau nilai yang dikirim tidak dipertahankan")
	}
	valid, err := ownerClient.PostForm(server.URL+correctionPath, url.Values{"gorilla.csrf.Token": {ownerCSRF}, "expected_version": {"0"}, "value": {"86,00"}, "reason": {"Koreksi hasil UAT"}})
	if err != nil || valid.StatusCode != http.StatusSeeOther {
		t.Fatalf("koreksi pemilik sah gagal: response=%v err=%v", valid, err)
	}
	_ = valid.Body.Close()
	var initial, effective, historyRows string
	if err = db.QueryRow(`SELECT sc.score::text,latest.after_score::text,latest.reason FROM app.assessment_activity_initial_scores sc JOIN LATERAL (SELECT after_score,reason FROM app.assessment_activity_score_corrections WHERE initial_score_id=sc.id ORDER BY version DESC LIMIT 1) latest ON true WHERE sc.id=$1`, scoreID).Scan(&initial, &effective, &historyRows); err != nil || initial != "82.50" || effective != "86.00" || historyRows != "Koreksi hasil UAT" {
		t.Fatalf("nilai awal/effective/history tidak tepat: %s/%s/%s err=%v", initial, effective, historyRows, err)
	}
	stale, err := ownerClient.PostForm(server.URL+correctionPath, url.Values{"gorilla.csrf.Token": {ownerCSRF}, "expected_version": {"0"}, "value": {"90"}, "reason": {"Versi lama"}})
	if err != nil || stale.StatusCode != http.StatusConflict {
		t.Fatalf("expected version basi harus 409: response=%v err=%v", stale, err)
	}
	staleBody := read(stale)
	if !strings.Contains(staleBody, "Data telah berubah") || !strings.Contains(staleBody, `value="90"`) {
		t.Fatal("konflik versi harus jelas dan mempertahankan input koreksi")
	}
	var correctionCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_corrections WHERE initial_score_id=$1`, scoreID).Scan(&correctionCount); err != nil || correctionCount != 1 {
		t.Fatalf("POST tertolak membuat histori parsial: count=%d err=%v", correctionCount, err)
	}
}
