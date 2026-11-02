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

func TestLearningObjectiveHTTPAssignmentAuthorizationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacher, teacherPassword := testutil.User(t, db, "guru_mapel")
	otherTeacher, otherPassword := testutil.User(t, db, "guru_mapel")
	random := testutil.Random(t)[:10]
	var yearID, semesterID, otherSemesterID, classID, otherClassID, copyClassID, differentSubjectClassID, differentSemesterClassID, subjectID, otherSubjectID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "TP HTTP "+random).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,2) RETURNING id`, yearID).Scan(&otherSemesterID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,8,'A') RETURNING id`, yearID).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,8,'B') RETURNING id`, yearID).Scan(&otherClassID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,8,'C') RETURNING id`, yearID).Scan(&copyClassID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,8,'D') RETURNING id`, yearID).Scan(&differentSubjectClassID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,8,'E') RETURNING id`, yearID).Scan(&differentSemesterClassID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "TPH-"+random, "Mapel TP HTTP "+random).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "TPH2-"+random, "Mapel TP HTTP lain "+random).Scan(&otherSubjectID); err != nil {
		t.Fatal(err)
	}
	masters := &master.Store{DB: db}
	teacherIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru HTTP TP " + random, UserID: teacher.ID, Active: true}})
	if err != nil {
		t.Fatal("identitas Guru Mapel", err)
	}
	otherIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru HTTP lain " + random, UserID: otherTeacher.ID, Active: true}})
	if err != nil {
		t.Fatal("identitas Guru Mapel pembanding", err)
	}
	service := &academic.Store{DB: db}
	effectiveFrom := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -30)
	if err = service.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, teacherIdentity.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan guru pertama", err)
	}
	if err = service.AssignTeacher(ctx, curriculum.ID, semesterID, otherClassID, subjectID, otherIdentity.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan guru pembanding", err)
	}
	if err = service.AssignTeacher(ctx, curriculum.ID, semesterID, copyClassID, subjectID, teacherIdentity.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan guru untuk kelas tujuan salin", err)
	}
	if err = service.AssignTeacher(ctx, curriculum.ID, semesterID, differentSubjectClassID, otherSubjectID, teacherIdentity.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan guru pada mata pelajaran berbeda", err)
	}
	if err = service.AssignTeacher(ctx, curriculum.ID, otherSemesterID, differentSemesterClassID, subjectID, teacherIdentity.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan guru pada semester berbeda", err)
	}
	var scoreStudentID, scorePlacementID int64
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_initial_scores WHERE school_year_id=$1`, yearID)
		if scorePlacementID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, scorePlacementID)
		}
		if scoreStudentID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE student_id=$1`, scoreStudentID)
			_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, scoreStudentID)
		}
		_, _ = db.Exec(`DELETE FROM app.assessment_activity_objectives WHERE activity_id IN (SELECT id FROM app.assessment_activities WHERE school_year_id=$1)`, yearID)
		_, _ = db.Exec(`DELETE FROM app.assessment_activities WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id IN ($1,$2))`, semesterID, otherSemesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id IN ($1,$2)`, semesterID, otherSemesterID)
		for _, teacherID := range []int64{teacherIdentity.ID, otherIdentity.ID} {
			_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, teacherID)
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID)
		}
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code IN ($1,$2)`, "TPH-"+random, "TPH2-"+random)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id IN ($1,$2)`, subjectID, otherSubjectID)
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id IN ($1,$2)`, semesterID, otherSemesterID)
		_, _ = db.Exec(`DELETE FROM app.classes WHERE id IN ($1,$2,$3,$4,$5)`, classID, otherClassID, copyClassID, differentSubjectClassID, differentSemesterClassID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		for _, userID := range []int64{admin.ID, curriculum.ID, teacher.ID, otherTeacher.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, cleanupErr := db.Exec(q, userID); cleanupErr != nil {
					t.Errorf("cleanup akun: %v", cleanupErr)
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
		page, getErr := client.Get(server.URL + "/login")
		if getErr != nil {
			t.Fatal(getErr)
		}
		loginBody, readErr := io.ReadAll(page.Body)
		_ = page.Body.Close()
		if readErr != nil || page.StatusCode != http.StatusOK {
			t.Fatalf("login GET gagal: status=%d err=%v", page.StatusCode, readErr)
		}
		match := csrfPattern.FindSubmatch(loginBody)
		if len(match) != 2 {
			t.Fatal("token CSRF tidak tersedia")
		}
		token, loginErr := accounts.Login(ctx, user.Username, password, "")
		if loginErr != nil {
			t.Fatal("login fixture gagal", loginErr)
		}
		base, _ := url.Parse(server.URL)
		jar.SetCookies(base, []*http.Cookie{{Name: sessionCookie, Value: token, Path: "/"}})
		return client, string(match[1])
	}
	contextURL := func(class int64) string {
		return "/guru/tujuan-pembelajaran?semester_id=" + strconv.FormatInt(semesterID, 10) + "&class_id=" + strconv.FormatInt(class, 10) + "&subject_id=" + strconv.FormatInt(subjectID, 10)
	}
	readBody := func(response *http.Response) string {
		t.Helper()
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		return string(body)
	}
	teacherClient, teacherCSRF := login(teacher, teacherPassword)
	page, err := teacherClient.Get(server.URL + "/guru/tujuan-pembelajaran")
	if err != nil || page.StatusCode != http.StatusOK || !strings.Contains(readBody(page), "Semester · Kelas · Mata pelajaran") {
		t.Fatalf("Guru Mapel berhak tidak dapat membuka halaman TP: response=%v err=%v", page, err)
	}
	post, err := teacherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran", url.Values{
		"gorilla.csrf.Token": {teacherCSRF}, "semester_id": {strconv.FormatInt(semesterID, 10)},
		"class_id": {strconv.FormatInt(classID, 10)}, "subject_id": {strconv.FormatInt(subjectID, 10)},
		"definition": {"TP HTTP yang sah"},
	})
	if err != nil || post.StatusCode != http.StatusSeeOther {
		t.Fatalf("simpan TP pada penugasan sah gagal: status=%v err=%v", post, err)
	}
	_ = post.Body.Close()
	page, err = teacherClient.Get(server.URL + serverPathForLocation(post.Header.Get("Location")))
	if err != nil || page.StatusCode != http.StatusOK || !strings.Contains(readBody(page), "TP HTTP yang sah") {
		t.Fatalf("TP tidak muncul kembali setelah refresh: response=%v err=%v", page, err)
	}
	page, err = teacherClient.Get(server.URL + contextURL(classID))
	if err != nil || page.StatusCode != http.StatusOK {
		t.Fatalf("halaman TP sumber gagal dibuka: response=%v err=%v", page, err)
	}
	pageBody := readBody(page)
	targetContextValue := strconv.FormatInt(semesterID, 10) + ":" + strconv.FormatInt(copyClassID, 10) + ":" + strconv.FormatInt(subjectID, 10)
	if !strings.Contains(pageBody, "Salin definisi TP") || !strings.Contains(pageBody, "Konteks kelas tujuan") || !strings.Contains(pageBody, "value=\""+targetContextValue+"\"") {
		t.Fatal("form salin atau konteks tujuan resmi tidak dirender")
	}
	activityPage, err := teacherClient.Get(server.URL + "/guru/kegiatan-penilaian?context=" + strconv.FormatInt(semesterID, 10) + ":" + strconv.FormatInt(classID, 10) + ":" + strconv.FormatInt(subjectID, 10))
	if err != nil || activityPage.StatusCode != http.StatusOK {
		t.Fatalf("halaman kegiatan penilaian tidak dapat dibuka: response=%v err=%v", activityPage, err)
	}
	activityBody := readBody(activityPage)
	if !strings.Contains(activityBody, "Nama / bentuk kegiatan") || !strings.Contains(activityBody, "Formatif") || !strings.Contains(activityBody, "Sumatif") || !strings.Contains(activityBody, `data-navigation-tab="assessment-activities"`) {
		t.Fatal("halaman kegiatan atau tab navigasi tidak dirender")
	}
	targetSelectStart := strings.Index(pageBody, `id="learning-objective-copy-target"`)
	if targetSelectStart < 0 {
		t.Fatal("pilihan kelas tujuan salin tidak ditemukan")
	}
	targetSelectEnd := strings.Index(pageBody[targetSelectStart:], "</select>")
	if targetSelectEnd < 0 {
		t.Fatal("pilihan kelas tujuan salin tidak tertutup")
	}
	targetSelect := pageBody[targetSelectStart : targetSelectStart+targetSelectEnd]
	if !strings.Contains(targetSelect, `value="`+targetContextValue+`"`) || strings.Contains(targetSelect, `value="`+strconv.FormatInt(semesterID, 10)+":"+strconv.FormatInt(differentSubjectClassID, 10)+":"+strconv.FormatInt(otherSubjectID, 10)+`"`) || strings.Contains(targetSelect, `value="`+strconv.FormatInt(otherSemesterID, 10)+":"+strconv.FormatInt(differentSemesterClassID, 10)+":"+strconv.FormatInt(subjectID, 10)+`"`) {
		t.Fatal("daftar tujuan harus membatasi kelas pada semester dan mata pelajaran sumber")
	}
	var sourceObjectiveID int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semesterID, classID, subjectID).Scan(&sourceObjectiveID); err != nil {
		t.Fatal("baca ID TP sumber fixture", err)
	}
	activityValues := url.Values{
		"semester_id": {strconv.FormatInt(semesterID, 10)}, "class_id": {strconv.FormatInt(classID, 10)},
		"subject_id": {strconv.FormatInt(subjectID, 10)}, "name": {"Formatif HTTP"}, "activity_type": {"formatif"},
		"activity_date": {time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")}, "objective_ids": {strconv.FormatInt(sourceObjectiveID, 10)},
	}
	activityNoCSRF, err := teacherClient.PostForm(server.URL+"/guru/kegiatan-penilaian", activityValues)
	if err != nil || activityNoCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("POST kegiatan tanpa CSRF harus ditolak: response=%v err=%v", activityNoCSRF, err)
	}
	_ = activityNoCSRF.Body.Close()
	activityValues.Set("gorilla.csrf.Token", teacherCSRF)
	activityPost, err := teacherClient.PostForm(server.URL+"/guru/kegiatan-penilaian", activityValues)
	if err != nil || activityPost.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST kegiatan valid gagal: response=%v err=%v", activityPost, err)
	}
	activityLocation := serverPathForLocation(activityPost.Header.Get("Location"))
	_ = activityPost.Body.Close()
	activityPage, err = teacherClient.Get(server.URL + activityLocation)
	if err != nil || activityPage.StatusCode != http.StatusOK || !strings.Contains(readBody(activityPage), "Formatif HTTP") {
		t.Fatalf("kegiatan tidak tampil setelah redirect/refresh: response=%v err=%v", activityPage, err)
	}
	var activityID int64
	if err = db.QueryRow(`SELECT id FROM app.assessment_activities WHERE school_year_id=$1`, yearID).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	activityDate := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	if err = db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','P') RETURNING id`, "Siswa HTTP nilai "+random).Scan(&scoreStudentID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5::date-10) RETURNING id`, yearID, scoreStudentID, classID, admin.ID, activityDate).Scan(&scorePlacementID); err != nil {
		t.Fatal(err)
	}
	scoreURL := server.URL + "/guru/kegiatan-penilaian/" + strconv.FormatInt(activityID, 10) + "/nilai"
	scorePage, err := teacherClient.Get(scoreURL)
	if err != nil || scorePage.StatusCode != http.StatusOK {
		t.Fatalf("halaman Input Nilai gagal: response=%v err=%v", scorePage, err)
	}
	scoreBody := readBody(scorePage)
	if !strings.Contains(scoreBody, "score_"+strconv.FormatInt(scoreStudentID, 10)+"_"+strconv.FormatInt(sourceObjectiveID, 10)) || !strings.Contains(scoreBody, `data-navigation-tab="assessment-activities"`) {
		t.Fatal("matriks nilai atau tab bersama tidak muncul")
	}
	scoreForm := url.Values{"score_" + strconv.FormatInt(scoreStudentID, 10) + "_" + strconv.FormatInt(sourceObjectiveID, 10): {"82,50"}}
	withoutScoreCSRF, err := teacherClient.PostForm(scoreURL, scoreForm)
	if err != nil || withoutScoreCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("POST nilai tanpa CSRF harus ditolak: %v %v", withoutScoreCSRF, err)
	}
	_ = withoutScoreCSRF.Body.Close()
	scoreForm.Set("gorilla.csrf.Token", teacherCSRF)
	scorePost, err := teacherClient.PostForm(scoreURL, scoreForm)
	if err != nil || scorePost.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST nilai sah gagal: %v %v", scorePost, err)
	}
	scoreLocation := serverPathForLocation(scorePost.Header.Get("Location"))
	_ = scorePost.Body.Close()
	scorePage, err = teacherClient.Get(server.URL + scoreLocation)
	if err != nil || scorePage.StatusCode != http.StatusOK || !strings.Contains(readBody(scorePage), "82.50") {
		t.Fatalf("nilai tidak muncul setelah redirect/refresh: %v %v", scorePage, err)
	}
	scoreForm.Set("score_"+strconv.FormatInt(scoreStudentID, 10)+"_"+strconv.FormatInt(sourceObjectiveID, 10), "82,51")
	overwrite, err := teacherClient.PostForm(scoreURL, scoreForm)
	if err != nil || overwrite.StatusCode != http.StatusConflict {
		t.Fatalf("nilai awal berbeda harus ditolak: %v %v", overwrite, err)
	}
	_ = overwrite.Body.Close()
	var scoreRows int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activity_initial_scores WHERE assessment_activity_id=$1`, activityID).Scan(&scoreRows); err != nil || scoreRows != 1 {
		t.Fatalf("HTTP meninggalkan nilai duplikat/perubahan: %d err=%v", scoreRows, err)
	}
	otherClient, _ := login(otherTeacher, otherPassword)
	deniedSheet, err := otherClient.Get(scoreURL)
	if err != nil || deniedSheet.StatusCode != http.StatusForbidden {
		t.Fatalf("guru lain membaca lembar nilai: %v %v", deniedSheet, err)
	}
	_ = deniedSheet.Body.Close()
	var activityCountBefore int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activities WHERE school_year_id=$1`, yearID).Scan(&activityCountBefore); err != nil || activityCountBefore != 1 {
		t.Fatalf("jumlah kegiatan sebelum uji lintas assignment=%d err=%v", activityCountBefore, err)
	}
	activityValues.Set("class_id", strconv.FormatInt(otherClassID, 10))
	deniedActivity, err := teacherClient.PostForm(server.URL+"/guru/kegiatan-penilaian", activityValues)
	if err != nil || deniedActivity.StatusCode != http.StatusForbidden {
		t.Fatalf("POST kegiatan pada penugasan guru lain harus ditolak: response=%v err=%v", deniedActivity, err)
	}
	_ = deniedActivity.Body.Close()
	var activityCountAfter int
	if err = db.QueryRow(`SELECT count(*) FROM app.assessment_activities WHERE school_year_id=$1`, yearID).Scan(&activityCountAfter); err != nil || activityCountAfter != activityCountBefore {
		t.Fatalf("POST lintas penugasan meninggalkan kegiatan: sebelum=%d sesudah=%d err=%v", activityCountBefore, activityCountAfter, err)
	}
	copyValues := url.Values{
		"source_id":      {strconv.FormatInt(sourceObjectiveID, 10)},
		"source_context": {strconv.FormatInt(semesterID, 10) + ":" + strconv.FormatInt(classID, 10) + ":" + strconv.FormatInt(subjectID, 10)},
		"target_context": {strconv.FormatInt(semesterID, 10) + ":" + strconv.FormatInt(copyClassID, 10) + ":" + strconv.FormatInt(subjectID, 10)},
	}
	withoutCSRF, err := teacherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran/salin", copyValues)
	if err != nil || withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("salin tanpa CSRF harus ditolak: response=%v err=%v", withoutCSRF, err)
	}
	_ = withoutCSRF.Body.Close()
	copyValues.Set("gorilla.csrf.Token", teacherCSRF)
	copied, err := teacherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran/salin", copyValues)
	if err != nil || copied.StatusCode != http.StatusSeeOther {
		t.Fatalf("salin TP ke penugasan sah gagal: response=%v err=%v", copied, err)
	}
	copyLocation := serverPathForLocation(copied.Header.Get("Location"))
	_ = copied.Body.Close()
	targetPage, err := teacherClient.Get(server.URL + copyLocation)
	if err != nil || targetPage.StatusCode != http.StatusOK {
		t.Fatalf("halaman TP tujuan tidak dapat dibuka: response=%v err=%v", targetPage, err)
	}
	targetBody := readBody(targetPage)
	if !strings.Contains(targetBody, "TP HTTP yang sah") || !strings.Contains(targetBody, "Definisi TP berhasil disalin") {
		t.Fatal("TP hasil salin tidak tampil pada tujuan dengan konfirmasi")
	}
	var copiedObjectiveID int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semesterID, copyClassID, subjectID).Scan(&copiedObjectiveID); err != nil {
		t.Fatal("baca TP konteks tujuan untuk uji manipulasi sumber", err)
	}
	var objectiveCount int
	if err = db.QueryRow(`SELECT count(*) FROM app.learning_objectives WHERE school_year_id=$1`, yearID).Scan(&objectiveCount); err != nil {
		t.Fatal("hitung TP sebelum uji konteks manipulasi", err)
	}
	copyValues.Set("source_id", strconv.FormatInt(copiedObjectiveID, 10))
	copyValues.Set("target_context", strconv.FormatInt(semesterID, 10)+":"+strconv.FormatInt(copyClassID, 10)+":"+strconv.FormatInt(subjectID, 10))
	manipulatedSource, err := teacherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran/salin", copyValues)
	if err != nil || manipulatedSource.StatusCode != http.StatusForbidden {
		t.Fatalf("ID sumber dari konteks lain yang juga dimiliki harus ditolak: response=%v err=%v", manipulatedSource, err)
	}
	_ = manipulatedSource.Body.Close()
	var countAfterSourceMismatch int
	if err = db.QueryRow(`SELECT count(*) FROM app.learning_objectives WHERE school_year_id=$1`, yearID).Scan(&countAfterSourceMismatch); err != nil || countAfterSourceMismatch != objectiveCount {
		t.Fatalf("ID sumber yang tidak cocok konteks tidak boleh membuat TP: sebelum=%d sesudah=%d err=%v", objectiveCount, countAfterSourceMismatch, err)
	}
	copyValues.Set("source_id", strconv.FormatInt(sourceObjectiveID, 10))
	for _, target := range []struct {
		name    string
		context string
	}{
		{name: "mata pelajaran berbeda", context: strconv.FormatInt(semesterID, 10) + ":" + strconv.FormatInt(differentSubjectClassID, 10) + ":" + strconv.FormatInt(otherSubjectID, 10)},
		{name: "semester berbeda", context: strconv.FormatInt(otherSemesterID, 10) + ":" + strconv.FormatInt(differentSemesterClassID, 10) + ":" + strconv.FormatInt(subjectID, 10)},
	} {
		copyValues.Set("target_context", target.context)
		invalidTarget, postErr := teacherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran/salin", copyValues)
		if postErr != nil || invalidTarget.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("POST tujuan %s harus ditolak sebagai validasi: response=%v err=%v", target.name, invalidTarget, postErr)
		}
		_ = invalidTarget.Body.Close()
		var countAfter int
		if err = db.QueryRow(`SELECT count(*) FROM app.learning_objectives WHERE school_year_id=$1`, yearID).Scan(&countAfter); err != nil || countAfter != objectiveCount {
			t.Fatalf("POST tujuan %s tidak boleh membuat TP parsial: sebelum=%d sesudah=%d err=%v", target.name, objectiveCount, countAfter, err)
		}
	}
	copyValues.Set("gorilla.csrf.Token", teacherCSRF)
	copyValues.Set("target_context", strconv.FormatInt(semesterID, 10)+":"+strconv.FormatInt(otherClassID, 10)+":"+strconv.FormatInt(subjectID, 10))
	deniedCopy, err := teacherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran/salin", copyValues)
	if err != nil || deniedCopy.StatusCode != http.StatusForbidden {
		t.Fatalf("salin ke penugasan guru lain harus ditolak: response=%v err=%v", deniedCopy, err)
	}
	_ = deniedCopy.Body.Close()
	post, err = teacherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran", url.Values{
		"gorilla.csrf.Token": {teacherCSRF}, "semester_id": {strconv.FormatInt(semesterID, 10)},
		"class_id": {strconv.FormatInt(otherClassID, 10)}, "subject_id": {strconv.FormatInt(subjectID, 10)},
		"definition": {"Tidak sah melalui ID"},
	})
	if err != nil || post.StatusCode != http.StatusForbidden {
		t.Fatalf("POST langsung ke kelas guru lain harus 403: status=%v err=%v", post, err)
	}
	_ = post.Body.Close()

	otherClient, otherCSRF := login(otherTeacher, otherPassword)
	if err = service.CreateLearningObjective(ctx, otherTeacher.ID, semesterID, otherClassID, subjectID, "TP sumber privat guru lain"); err != nil {
		t.Fatal("buat TP untuk menguji sumber lintas guru", err)
	}
	var otherSourceID int64
	if err = db.QueryRow(`SELECT id FROM app.learning_objectives WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semesterID, otherClassID, subjectID).Scan(&otherSourceID); err != nil {
		t.Fatal("baca TP sumber milik guru lain", err)
	}
	copyValues.Set("source_id", strconv.FormatInt(otherSourceID, 10))
	copyValues.Set("target_context", strconv.FormatInt(semesterID, 10)+":"+strconv.FormatInt(copyClassID, 10)+":"+strconv.FormatInt(subjectID, 10))
	copyValues.Set("source_context", strconv.FormatInt(semesterID, 10)+":"+strconv.FormatInt(otherClassID, 10)+":"+strconv.FormatInt(subjectID, 10))
	deniedCopy, err = teacherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran/salin", copyValues)
	if err != nil || deniedCopy.StatusCode != http.StatusForbidden {
		t.Fatalf("salin dari TP guru lain harus ditolak: response=%v err=%v", deniedCopy, err)
	}
	_ = deniedCopy.Body.Close()
	denied, err := otherClient.Get(server.URL + contextURL(classID))
	if err != nil || denied.StatusCode != http.StatusForbidden {
		t.Fatalf("GET dengan konteks guru lain harus 403: status=%v err=%v", denied, err)
	}
	_ = denied.Body.Close()
	denied, err = otherClient.PostForm(server.URL+"/guru/tujuan-pembelajaran", url.Values{
		"gorilla.csrf.Token": {otherCSRF}, "semester_id": {strconv.FormatInt(semesterID, 10)},
		"class_id": {strconv.FormatInt(classID, 10)}, "subject_id": {strconv.FormatInt(subjectID, 10)},
		"definition": {"Tidak sah oleh guru lain"},
	})
	if err != nil || denied.StatusCode != http.StatusForbidden {
		t.Fatalf("POST langsung oleh guru lain harus 403: status=%v err=%v", denied, err)
	}
	_ = denied.Body.Close()
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.learning_objectives WHERE school_year_id=$1`, yearID).Scan(&count); err != nil || count != 3 {
		t.Fatalf("request ditolak tidak boleh membuat TP tambahan: count=%d err=%v", count, err)
	}
}

func serverPathForLocation(location string) string {
	parsed, _ := url.Parse(location)
	return parsed.RequestURI()
}
