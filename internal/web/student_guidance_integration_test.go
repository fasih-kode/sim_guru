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

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/studentaffairs"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentGuidanceHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	admin, adminPassword := testutil.User(t, db, "admin")
	kes, kesPassword := testutil.User(t, db, "kesiswaan")
	head, headPassword := testutil.User(t, db, "kepala_sekolah")
	bk, bkPassword := testutil.User(t, db, "bk")
	wali, waliPassword := testutil.User(t, db, "wali_kelas")
	users := []int64{admin.ID, kes.ID, head.ID, bk.ID, wali.ID}
	var yearID, classID, studentID, placementID, teacherID int64
	suffix := testutil.Random(t)[:10]
	yearID, eventDate := testutil.ActiveCalendar(t, db, suffix)
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "G-ARSIP-"+suffix).Scan(&classID); err != nil {
		t.Fatal("create class")
	}
	if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','P') RETURNING id`, "Siswa Arsip "+suffix).Scan(&studentID); err != nil {
		t.Fatal("create student")
	}
	if err := db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5) RETURNING id`, yearID, studentID, classID, admin.ID, eventDate).Scan(&placementID); err != nil {
		t.Fatal("create placement")
	}
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali Arsip "+suffix, wali.ID).Scan(&teacherID); err != nil {
		t.Fatal("create teacher identity")
	}
	if _, err := db.Exec(`INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3)`, classID, teacherID, admin.ID); err != nil {
		t.Fatal("create homeroom assignment")
	}
	store := &academic.Store{DB: db}
	record, err := store.CreateStudentGuidance(context.Background(), kes.ID, studentaffairs.GuidanceInput{StudentID: studentID, Date: eventDate, Incident: "Arsip legacy <script>alert('x')</script>", Action: "Tindakan lama", Status: studentaffairs.GuidanceNeedsFollowUp})
	if err != nil {
		t.Fatal("create archived record fixture")
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.student_guidance_history WHERE record_id=$1`, record.ID)
		_, _ = db.Exec(`DELETE FROM app.student_guidance_records WHERE id=$1`, record.ID)
		_, _ = db.Exec(`DELETE FROM app.homeroom_assignments WHERE class_id=$1`, classID)
		if placementID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, placementID)
		}
		if studentID > 0 {
			_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, studentID)
		}
		if teacherID > 0 {
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID)
		}
		if classID > 0 {
			_, _ = db.Exec(`DELETE FROM app.classes WHERE id=$1`, classID)
		}
		_, _ = db.Exec(`DELETE FROM app.user_roles WHERE user_id=ANY($1::bigint[])`, users)
		_, _ = db.Exec(`DELETE FROM app.users WHERE id=ANY($1::bigint[])`, users)
	})

	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: store, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	bodyOf := func(resp *http.Response) string {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return string(body)
	}
	getResponse := func(client *http.Client, path string, want int) (*http.Response, string) {
		resp, getErr := client.Get(server.URL + path)
		if getErr != nil {
			t.Fatal(getErr)
		}
		body := bodyOf(resp)
		if resp.StatusCode != want {
			t.Fatalf("GET %s status=%d want=%d", path, resp.StatusCode, want)
		}
		return resp, body
	}
	get := func(client *http.Client, path string, want int) string {
		_, body := getResponse(client, path, want)
		return body
	}
	getHX := func(client *http.Client, path string, want int) (string, http.Header) {
		req, reqErr := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if reqErr != nil {
			t.Fatal(reqErr)
		}
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-List-Request", "true")
		resp, getErr := client.Do(req)
		if getErr != nil {
			t.Fatal(getErr)
		}
		body := bodyOf(resp)
		if resp.StatusCode != want {
			t.Fatalf("GET HX %s status=%d want=%d body=%s", path, resp.StatusCode, want, body)
		}
		return body, resp.Header
	}
	post := func(client *http.Client, path string, data url.Values, want int) (*http.Response, string) {
		resp, postErr := client.PostForm(server.URL+path, data)
		if postErr != nil {
			t.Fatal(postErr)
		}
		body := bodyOf(resp)
		if resp.StatusCode != want {
			t.Fatalf("POST %s status=%d want=%d", path, resp.StatusCode, want)
		}
		return resp, body
	}
	tokenPattern := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	csrfToken := func(body string) string {
		match := tokenPattern.FindStringSubmatch(body)
		if len(match) != 2 {
			t.Fatal("csrf field absent")
		}
		return match[1]
	}
	login := func(user auth.User, password string) *http.Client {
		client := newClient()
		token := csrfToken(get(client, "/login", http.StatusOK))
		post(client, "/login", url.Values{"gorilla.csrf.Token": {token}, "username": {user.Username}, "password": {password}}, http.StatusSeeOther)
		return client
	}
	kesiswaan := login(kes, kesPassword)
	headmaster := login(head, headPassword)
	adminClient := login(admin, adminPassword)
	bkClient := login(bk, bkPassword)
	waliClient := login(wali, waliPassword)

	legacyResp, _ := getResponse(kesiswaan, "/kesiswaan/pembinaan", http.StatusSeeOther)
	if legacyResp.Header.Get("Location") != "/kesiswaan/kasus/arsip-pembinaan" {
		t.Fatal("legacy list URL should redirect to the read-only archive")
	}
	archivePath := "/kesiswaan/kasus/arsip-pembinaan"
	archive := get(kesiswaan, archivePath, http.StatusOK)
	if !strings.Contains(archive, "Arsip pembinaan lama") || !strings.Contains(archive, "Arsip legacy") || strings.Contains(archive, "/kesiswaan/pembinaan/new") || strings.Contains(archive, "Tambah catatan") {
		t.Fatal("archive should show old records without a creation path")
	}
	if strings.Contains(archive, "<script>alert('x')</script>") || !strings.Contains(archive, "&lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt;") {
		t.Fatal("archived text was not escaped")
	}
	_, waliArchive := getResponse(waliClient, archivePath, http.StatusOK)
	_, headArchive := getResponse(headmaster, archivePath, http.StatusOK)
	if !strings.Contains(waliArchive, "Arsip legacy") || !strings.Contains(headArchive, "Arsip legacy") {
		t.Fatal("D-042 Wali Kelas and Kepala Sekolah archive read scope changed")
	}
	adminArchive := get(adminClient, archivePath, http.StatusForbidden)
	bkArchive := get(bkClient, archivePath, http.StatusForbidden)
	if !strings.Contains(adminArchive, "app-sidebar") || !strings.Contains(bkArchive, "app-sidebar") {
		t.Fatal("archive access denial should use the standard app shell")
	}
	query := url.Values{"student_id": {"0"}, "q": {"Arsip legacy"}, "status": {"perlu_tindak_lanjut"}}
	hxArchive, hxHeaders := getHX(kesiswaan, archivePath+"?"+query.Encode(), http.StatusOK)
	if !strings.Contains(hxArchive, `data-list-page="student-guidance-archive"`) || !strings.Contains(hxArchive, "Arsip legacy") || hxHeaders.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("archive Instant Search did not return the list fragment")
	}
	legacyDetail, _ := getResponse(kesiswaan, "/kesiswaan/pembinaan/"+strconv.FormatInt(record.ID, 10), http.StatusSeeOther)
	if legacyDetail.Header.Get("Location") != archivePath+"/"+strconv.FormatInt(record.ID, 10) {
		t.Fatal("legacy detail URL should redirect to archived detail")
	}
	archiveDetail := get(kesiswaan, archivePath+"/"+strconv.FormatInt(record.ID, 10), http.StatusOK)
	if !strings.Contains(archiveDetail, "Histori arsip") || strings.Contains(archiveDetail, "/edit") || strings.Contains(archiveDetail, "/status\"") {
		t.Fatal("archived detail should preserve history and omit mutation controls")
	}
	if !strings.Contains(get(kesiswaan, "/kesiswaan/kasus", http.StatusOK), "Arsip pembinaan lama") || strings.Contains(get(bkClient, "/kesiswaan/kasus", http.StatusOK), "Arsip pembinaan lama") {
		t.Fatal("archive entry should appear only for roles authorized by D-042")
	}

	beforeCount, beforeHistory := 0, 0
	if err = db.QueryRow(`SELECT count(*),(SELECT count(*) FROM app.student_guidance_history WHERE record_id=$1) FROM app.student_guidance_records WHERE id=$1`, record.ID).Scan(&beforeCount, &beforeHistory); err != nil {
		t.Fatal(err)
	}
	token := csrfToken(archive)
	_, retiredBody := post(kesiswaan, "/kesiswaan/pembinaan/new", url.Values{"gorilla.csrf.Token": {token}, "student_id": {strconv.FormatInt(studentID, 10)}, "occurred_on": {eventDate}, "incident": {"Tidak boleh membuat arsip baru"}, "action": {"Uji"}, "status": {"perlu_tindak_lanjut"}}, http.StatusGone)
	if !strings.Contains(retiredBody, "app-sidebar") || !strings.Contains(retiredBody, "hanya dapat dibaca") {
		t.Fatal("legacy create route should show a clear app-shell read-only message")
	}
	_, retiredStatusBody := post(kesiswaan, "/kesiswaan/pembinaan/"+strconv.FormatInt(record.ID, 10)+"/status", url.Values{"gorilla.csrf.Token": {token}, "version": {"1"}, "status": {"selesai"}, "reason": {"Uji"}}, http.StatusGone)
	if !strings.Contains(retiredStatusBody, "hanya dapat dibaca") {
		t.Fatal("legacy status mutation should be disabled")
	}
	var afterCount, afterHistory int
	if err = db.QueryRow(`SELECT count(*),(SELECT count(*) FROM app.student_guidance_history WHERE record_id=$1) FROM app.student_guidance_records WHERE id=$1`, record.ID).Scan(&afterCount, &afterHistory); err != nil || beforeCount != 1 || afterCount != beforeCount || afterHistory != beforeHistory {
		t.Fatalf("legacy requests changed archived data: records=%d/%d history=%d/%d err=%v", beforeCount, afterCount, beforeHistory, afterHistory, err)
	}
}
