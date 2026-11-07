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

func TestAssessmentScoreRemedialHTTPAuthorizationCSRFAndHistoryIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	owner, ownerPassword := testutil.User(t, db, "guru_mapel")
	other, otherPassword := testutil.User(t, db, "guru_mapel")
	suffix := testutil.Random(t)[:10]
	var yearID, semesterID, classID, subjectID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "Remedial HTTP "+suffix).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,8,$2) RETURNING id`, yearID, "H"+suffix[:6]).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "RH7-"+suffix, "Mapel remedial HTTP "+suffix).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	masters := &master.Store{DB: db}
	identity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Pemilik remedial " + suffix, UserID: owner.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	academics := &academic.Store{DB: db}
	date := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	if err = academics.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, identity.ID, time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -20)); err != nil {
		t.Fatal(err)
	}
	if err = academics.CreateLearningObjective(ctx, owner.ID, semesterID, classID, subjectID, "TP HTTP remedial "+suffix); err != nil {
		t.Fatal(err)
	}
	var objectiveID int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semesterID, classID, subjectID).Scan(&objectiveID); err != nil {
		t.Fatal(err)
	}
	if err = academics.CreateAssessmentActivity(ctx, owner.ID, semesterID, classID, subjectID, "Sumatif remedial HTTP "+suffix, "sumatif", date, []int64{objectiveID}); err != nil {
		t.Fatal(err)
	}
	if err = academics.CreateAssessmentActivity(ctx, owner.ID, semesterID, classID, subjectID, "Formatif remedial HTTP "+suffix, "formatif", date, []int64{objectiveID}); err != nil {
		t.Fatal(err)
	}
	var activityID, formatifID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND activity_type='sumatif'`, yearID).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1 AND activity_type='formatif'`, yearID).Scan(&formatifID); err != nil {
		t.Fatal(err)
	}
	var studentID, placementID int64
	if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','L') RETURNING id`, "Siswa HTTP remedial "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date-10) RETURNING id`, yearID, studentID, classID, admin.ID, date).Scan(&placementID); err != nil {
		t.Fatal(err)
	}
	if err = academics.SaveAssessmentActivityInitialScores(ctx, owner.ID, activityID, []academic.AssessmentScoreInput{{StudentID: studentID, ObjectiveID: objectiveID, Value: "0"}}); err != nil {
		t.Fatal(err)
	}
	if err = academics.SaveAssessmentActivityInitialScores(ctx, owner.ID, formatifID, []academic.AssessmentScoreInput{{StudentID: studentID, ObjectiveID: objectiveID, Value: "0"}}); err != nil {
		t.Fatal(err)
	}
	var scoreID, formatifScoreID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, activityID).Scan(&scoreID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, formatifID).Scan(&formatifScoreID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var remedialCount int
		if cleanupErr := db.QueryRow(`SELECT count(*) FROM app.assessment_activity_score_remedials WHERE initial_score_id=$1`, scoreID).Scan(&remedialCount); cleanupErr != nil {
			t.Errorf("periksa fixture remedial: %v", cleanupErr)
			return
		}
		if remedialCount > 0 {
			t.Log("fixture dipertahankan karena remedial append-only")
			return
		}
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=ANY($1::bigint[])`, []int64{activityID, formatifID})
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_objectives WHERE activity_id=ANY($1::bigint[])`, []int64{activityID, formatifID})
		_, _ = db.Exec(`DELETE FROM app.assessment_activities WHERE id=ANY($1::bigint[])`, []int64{activityID, formatifID})
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE id=$1`, objectiveID)
		_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, placementID)
		_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, studentID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id=$1)`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, identity.ID)
		_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, identity.ID)
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code=$1`, "RH7-"+suffix)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id=$1`, subjectID)
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.classes WHERE id=$1`, classID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		for _, id := range []int64{admin.ID, curriculum.ID, owner.ID, other.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				_, _ = db.Exec(q, id)
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
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: masters, Academics: academics, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	login := func(user auth.User, password string) *http.Client {
		t.Helper()
		jar, jarErr := cookiejar.New(nil)
		if jarErr != nil {
			t.Fatal(jarErr)
		}
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		token, loginErr := accounts.Login(ctx, user.Username, password, "")
		if loginErr != nil {
			t.Fatal(loginErr)
		}
		base, _ := url.Parse(server.URL)
		jar.SetCookies(base, []*http.Cookie{{Name: sessionCookie, Value: token, Path: "/"}})
		return client
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
	ownerClient, otherClient := login(owner, ownerPassword), login(other, otherPassword)
	path := remedialPath(scoreID)
	page, err := ownerClient.Get(server.URL + path)
	if err != nil || page.StatusCode != http.StatusOK {
		t.Fatalf("GET remedial pemilik gagal: %v %v", page, err)
	}
	body := read(page)
	csrfRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	match := csrfRE.FindStringSubmatch(body)
	keyRE := regexp.MustCompile(`name="submission_key" value="([^"]+)"`)
	keyMatch := keyRE.FindStringSubmatch(body)
	if len(match) != 2 || len(keyMatch) != 2 || !strings.Contains(body, "Nilai awal asli") || !strings.Contains(body, "Nilai efektif saat ini") || !strings.Contains(body, "Riwayat Remedial") {
		t.Fatal("form remedial tidak menampilkan konteks, nilai atau CSRF")
	}
	otherGet, err := otherClient.Get(server.URL + path)
	if err != nil || otherGet.StatusCode != http.StatusForbidden {
		t.Fatalf("guru lain membaca remedial pemilik: %v %v", otherGet, err)
	}
	_ = read(otherGet)
	otherHome, err := otherClient.Get(server.URL + "/")
	if err != nil || otherHome.StatusCode != http.StatusOK {
		t.Fatalf("halaman untuk mengambil CSRF pengguna lain gagal: %v %v", otherHome, err)
	}
	otherMatch := csrfRE.FindStringSubmatch(read(otherHome))
	if len(otherMatch) != 2 {
		t.Fatal("token CSRF pengguna lain tidak ditemukan")
	}
	deniedPost, err := otherClient.PostForm(server.URL+path, url.Values{"gorilla.csrf.Token": {otherMatch[1]}, "value": {"99"}, "remedial_date": {date}})
	if err != nil || deniedPost.StatusCode != http.StatusForbidden {
		t.Fatalf("POST lintas pemilik tidak ditolak: %v %v", deniedPost, err)
	}
	_ = read(deniedPost)
	noCSRF, err := ownerClient.PostForm(server.URL+path, url.Values{"value": {"75"}, "remedial_date": {date}})
	if err != nil || noCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("POST tanpa CSRF diterima: %v %v", noCSRF, err)
	}
	_ = read(noCSRF)
	invalid, err := ownerClient.PostForm(server.URL+path, url.Values{"gorilla.csrf.Token": {match[1]}, "value": {"75.001"}, "remedial_date": {date}})
	if err != nil || invalid.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("nilai remedial tidak valid diterima: %v %v", invalid, err)
	}
	if invalidBody := read(invalid); !strings.Contains(invalidBody, "dua angka desimal") {
		t.Fatal("pesan validasi remedial tidak jelas")
	}
	validForm := url.Values{"gorilla.csrf.Token": {match[1]}, "submission_key": {keyMatch[1]}, "value": {"75,00"}, "remedial_date": {date}}
	valid, err := ownerClient.PostForm(server.URL+path, validForm)
	if err != nil || valid.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST remedial sah gagal: %v %v", valid, err)
	}
	_ = read(valid)
	// Pengiriman ulang byte-for-byte berhasil tanpa membuat baris kedua.
	replay, err := ownerClient.PostForm(server.URL+path, validForm)
	if err != nil || replay.StatusCode != http.StatusSeeOther {
		t.Fatalf("replay POST identik gagal: %v %v", replay, err)
	}
	_ = read(replay)
	// Payload berbeda dengan kunci yang sama ditolak dan tidak mengubah histori.
	conflictForm := url.Values{"gorilla.csrf.Token": {match[1]}, "submission_key": {keyMatch[1]}, "value": {"76"}, "remedial_date": {date}}
	conflict, err := ownerClient.PostForm(server.URL+path, conflictForm)
	if err != nil || conflict.StatusCode != http.StatusConflict {
		t.Fatalf("kunci yang dipakai ulang dengan nilai berbeda tidak konflik: %v %v", conflict, err)
	}
	if !strings.Contains(read(conflict), "Data telah berubah") {
		t.Fatal("pesan konflik kunci pengiriman tidak jelas")
	}
	// Kunci terpisah tetap menyimpan tindakan berbeda dengan payload yang sama.
	separateForm := url.Values{"gorilla.csrf.Token": {match[1]}, "submission_key": {"00000000-0000-4000-8000-000000000001"}, "value": {"75,00"}, "remedial_date": {date}}
	separate, err := ownerClient.PostForm(server.URL+path, separateForm)
	if err != nil || separate.StatusCode != http.StatusSeeOther {
		t.Fatalf("submission terpisah dengan nilai sama ditolak: %v %v", separate, err)
	}
	_ = read(separate)
	after, err := ownerClient.Get(server.URL + path)
	if err != nil || after.StatusCode != http.StatusOK {
		t.Fatalf("GET remedial setelah simpan gagal: %v %v", after, err)
	}
	afterBody := read(after)
	if !strings.Contains(afterBody, "75.00") || !strings.Contains(afterBody, owner.DisplayName) {
		t.Fatal("nilai remedial/aktor tidak tampil setelah redirect")
	}
	owned, err := ownerClient.Get(server.URL + "/guru/nilai-awal-saya")
	if err != nil || owned.StatusCode != http.StatusOK {
		t.Fatalf("daftar Nilai Milik Saya gagal: %v %v", owned, err)
	}
	ownedBody := read(owned)
	if !strings.Contains(ownedBody, "Nilai awal asli") || !strings.Contains(ownedBody, "Nilai awal terkoreksi") || !strings.Contains(ownedBody, "Nilai efektif") || !strings.Contains(ownedBody, "0.00") || !strings.Contains(ownedBody, "75.00") {
		t.Fatal("daftar nilai tidak membedakan nilai asli, terkoreksi, dan efektif")
	}
	correction, err := ownerClient.Get(server.URL + "/guru/nilai-awal/" + strconv.FormatInt(scoreID, 10) + "/koreksi")
	if err != nil || correction.StatusCode != http.StatusOK {
		t.Fatalf("detail koreksi nilai gagal: %v %v", correction, err)
	}
	correctionBody := read(correction)
	if !strings.Contains(correctionBody, "Nilai awal asli (tetap)") || !strings.Contains(correctionBody, "Nilai awal terkoreksi") || !strings.Contains(correctionBody, "Nilai efektif Sumatif") || strings.Count(correctionBody, ">0.00</dd>") < 2 || !strings.Contains(correctionBody, ">75.00</dd>") {
		t.Fatal("detail koreksi menampilkan nilai Sumatif yang saling bertentangan")
	}
	if !strings.Contains(correctionBody, "Belum ada koreksi nilai awal. Nilai awal terkoreksi sama dengan nilai awal asli. Nilai efektif Sumatif dapat berbeda apabila terdapat remedial.") || strings.Contains(correctionBody, "Nilai berlaku sama dengan nilai awal.") {
		t.Fatal("pesan kosong Sumatif mengabaikan remedial atau menyamakan nilai efektif dengan nilai awal")
	}
	formatifCorrection, err := ownerClient.Get(server.URL + "/guru/nilai-awal/" + strconv.FormatInt(formatifScoreID, 10) + "/koreksi")
	if err != nil || formatifCorrection.StatusCode != http.StatusOK {
		t.Fatalf("detail koreksi Formatif gagal: %v %v", formatifCorrection, err)
	}
	formatifBody := read(formatifCorrection)
	if !strings.Contains(formatifBody, "Nilai awal terkoreksi sama dengan nilai awal asli. Nilai berlaku Formatif sama dengan nilai awal terkoreksi.") || strings.Contains(formatifBody, "Nilai efektif Sumatif dapat berbeda apabila terdapat remedial.") || strings.Contains(formatifBody, "Nilai berlaku sama dengan nilai awal.") {
		t.Fatal("pesan kosong Formatif tidak sesuai dengan nilai Formatif")
	}
	var initial, count string
	if err = db.QueryRow(`SELECT sc.score::text,count(r.id)::text FROM app.assessment_activity_initial_scores sc LEFT JOIN app.assessment_activity_score_remedials r ON r.initial_score_id=sc.id WHERE sc.id=$1 GROUP BY sc.id`, scoreID).Scan(&initial, &count); err != nil || initial != "0.00" || count != "2" {
		t.Fatalf("nilai awal berubah atau remedial tidak atomik: initial=%s count=%s err=%v", initial, count, err)
	}
}
