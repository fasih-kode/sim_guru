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
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentGuidanceHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	admin, adminPassword := testutil.User(t, db, "admin")
	kes, kesPassword := testutil.User(t, db, "kesiswaan")
	head, headPassword := testutil.User(t, db, "kepala_sekolah")
	bk, bkPassword := testutil.User(t, db, "bk")
	users := []int64{admin.ID, kes.ID, head.ID, bk.ID}
	var yearID, classID, studentID, placementID int64
	suffix := testutil.Random(t)[:10]
	yearID, eventDate := testutil.ActiveCalendar(t, db, suffix)
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "G"+suffix).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2013-01-01','P') RETURNING id`, "Siswa pembinaan "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,$5) RETURNING id`, yearID, studentID, classID, admin.ID, eventDate).Scan(&placementID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.student_guidance_history WHERE record_id IN (SELECT id FROM app.student_guidance_records WHERE student_id=$1)`, studentID)
		_, _ = db.Exec(`DELETE FROM app.student_guidance_records WHERE student_id=$1`, studentID)
		if placementID > 0 {
			_, _ = db.Exec(`DELETE FROM app.student_placements WHERE id=$1`, placementID)
		}
		if studentID > 0 {
			_, _ = db.Exec(`DELETE FROM app.students WHERE id=$1`, studentID)
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
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db}, CSRFKey: key})
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
	get := func(client *http.Client, path string, want int) string {
		resp, getErr := client.Get(server.URL + path)
		if getErr != nil {
			t.Fatal(getErr)
		}
		body := bodyOf(resp)
		if resp.StatusCode != want {
			t.Fatalf("GET %s status=%d want=%d", path, resp.StatusCode, want)
		}
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
	post := func(client *http.Client, path string, data url.Values, want int) string {
		resp, postErr := client.PostForm(server.URL+path, data)
		if postErr != nil {
			t.Fatal(postErr)
		}
		body := bodyOf(resp)
		if resp.StatusCode != want {
			t.Fatalf("POST %s status=%d want=%d body=%s", path, resp.StatusCode, want, body)
		}
		return body
	}
	postHX := func(client *http.Client, path string, data url.Values, want int) (string, http.Header) {
		req, reqErr := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(data.Encode()))
		if reqErr != nil {
			t.Fatal(reqErr)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		resp, postErr := client.Do(req)
		if postErr != nil {
			t.Fatal(postErr)
		}
		body := bodyOf(resp)
		if resp.StatusCode != want {
			t.Fatalf("POST HX %s status=%d want=%d body=%s", path, resp.StatusCode, want, body)
		}
		return body, resp.Header
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
	get(newClient(), "/kesiswaan/pembinaan", http.StatusSeeOther)
	get(adminClient, "/kesiswaan/pembinaan", http.StatusForbidden)
	get(bkClient, "/kesiswaan/pembinaan", http.StatusForbidden)
	newForm := get(kesiswaan, "/kesiswaan/pembinaan?new=1", http.StatusOK)
	newToken := csrfToken(newForm)
	post(kesiswaan, "/kesiswaan/pembinaan/new", url.Values{"student_id": {strconv.FormatInt(studentID, 10)}, "occurred_on": {eventDate}, "incident": {"CSRF tidak ada"}, "action": {"Uji"}, "status": {"perlu_tindak_lanjut"}}, http.StatusForbidden)

	studentIDText := strconv.FormatInt(studentID, 10)
	incident := "Uji XSS <script>alert('x')</script> " + suffix
	for i := 0; i < 11; i++ {
		data := url.Values{
			"gorilla.csrf.Token": {newToken}, "student_id": {studentIDText}, "occurred_on": {eventDate},
			"incident": {incident + strconv.Itoa(i)}, "action": {"Tindakan " + strconv.Itoa(i)}, "status": {"perlu_tindak_lanjut"},
		}
		post(kesiswaan, "/kesiswaan/pembinaan/new", data, http.StatusSeeOther)
	}
	var recordID int64
	if err = db.QueryRow(`SELECT id FROM app.student_guidance_records WHERE student_id=$1 ORDER BY id DESC LIMIT 1`, studentID).Scan(&recordID); err != nil {
		t.Fatal(err)
	}
	base := "/kesiswaan/pembinaan/" + strconv.FormatInt(recordID, 10)
	list := get(kesiswaan, "/kesiswaan/pembinaan", http.StatusOK)
	if !strings.Contains(list, "Berikutnya") || !strings.Contains(list, "sm:hidden") || !strings.Contains(list, "hidden sm:block") {
		t.Fatal("pagination atau layout responsif daftar pembinaan tidak tersedia")
	}
	if strings.Contains(list, "<script>alert('x')</script>") || !strings.Contains(list, "&lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt;") {
		t.Fatal("uraian catatan tidak di-escape pada output HTML")
	}
	query := url.Values{"q": {"<script>"}, "status": {"perlu_tindak_lanjut"}}
	filtered := get(kesiswaan, "/kesiswaan/pembinaan?"+query.Encode(), http.StatusOK)
	if !strings.Contains(filtered, "Uji XSS") || strings.Contains(filtered, "Tindakan 0") || !strings.Contains(filtered, "q=%3Cscript%3E") {
		t.Fatal("pencarian server tidak memfilter catatan sesuai kata kunci")
	}
	hxQuery := url.Values{"student_id": {"0"}, "q": {"Uji XSS"}, "status": {"perlu_tindak_lanjut"}}
	hxList, hxHeaders := getHX(kesiswaan, "/kesiswaan/pembinaan?"+hxQuery.Encode(), http.StatusOK)
	if !strings.Contains(hxList, `id="list-region"`) || strings.Contains(strings.ToLower(hxList), "<!doctype html>") || !strings.Contains(hxList, "Uji XSS") || hxHeaders.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("pencarian HTMX dengan student_id sentinel tidak mengembalikan fragmen daftar")
	}
	pageTwo := url.Values{"student_id": {"0"}, "q": {"Uji XSS"}, "status": {"perlu_tindak_lanjut"}, "page": {"2"}}
	secondPage, _ := getHX(kesiswaan, "/kesiswaan/pembinaan?"+pageTwo.Encode(), http.StatusOK)
	if !strings.Contains(secondPage, `aria-current="page">2`) || strings.Contains(secondPage, "page=3") {
		t.Fatal("paginasi HTMX halaman kedua tidak konsisten")
	}
	emptyQuery := url.Values{"student_id": {"0"}, "q": {"kata-kunci-tidak-ada-" + suffix}}
	emptyList, _ := getHX(kesiswaan, "/kesiswaan/pembinaan?"+emptyQuery.Encode(), http.StatusOK)
	if !strings.Contains(emptyList, "Tidak ada catatan yang cocok dengan filter.") {
		t.Fatal("pencarian HTMX kosong tidak menampilkan empty state")
	}
	invalidFilter := url.Values{"student_id": {"0"}, "status": {"status-tidak-valid"}}
	_, _ = getHX(kesiswaan, "/kesiswaan/pembinaan?"+invalidFilter.Encode(), http.StatusUnprocessableEntity)
	headDetail := get(headmaster, base, http.StatusOK)
	if strings.Contains(headDetail, "Ubah kejadian/tindakan") || strings.Contains(headDetail, "/status\"") {
		t.Fatal("Kepala Sekolah mendapat kontrol mutasi")
	}
	get(adminClient, base, http.StatusForbidden)
	get(bkClient, base, http.StatusForbidden)

	detail := get(kesiswaan, base, http.StatusOK)
	statusToken := csrfToken(detail)
	var beforeVersion, beforeHistory int
	if err = db.QueryRow(`SELECT r.version,(SELECT count(*) FROM app.student_guidance_history h WHERE h.record_id=r.id) FROM app.student_guidance_records r WHERE r.id=$1`, recordID).Scan(&beforeVersion, &beforeHistory); err != nil {
		t.Fatal(err)
	}
	statusPath := base + "/status"
	unchanged := url.Values{
		"gorilla.csrf.Token": {statusToken}, "version": {strconv.Itoa(beforeVersion)},
		"status": {"perlu_tindak_lanjut"}, "reason": {"Alasan untuk status yang sama"},
	}
	invalidBody := post(kesiswaan, statusPath, unchanged, http.StatusUnprocessableEntity)
	if !strings.Contains(strings.ToLower(invalidBody), "<!doctype html>") || !strings.Contains(invalidBody, "Pilih status yang berbeda") || !strings.Contains(invalidBody, "Alasan untuk status yang sama") || !strings.Contains(invalidBody, `data-guidance-status-error`) {
		t.Fatal("validasi status sama tidak kembali sebagai halaman detail dengan nilai form")
	}
	if err = db.QueryRow(`SELECT r.version,(SELECT count(*) FROM app.student_guidance_history h WHERE h.record_id=r.id) FROM app.student_guidance_records r WHERE r.id=$1`, recordID).Scan(&beforeVersion, &beforeHistory); err != nil || beforeVersion != 1 || beforeHistory != 1 {
		t.Fatalf("validasi status sama mengubah database: version=%d history=%d err=%v", beforeVersion, beforeHistory, err)
	}
	emptyReason := url.Values{
		"gorilla.csrf.Token": {statusToken}, "version": {"1"},
		"status": {"selesai"}, "reason": {""},
	}
	emptyReasonBody := post(kesiswaan, statusPath, emptyReason, http.StatusUnprocessableEntity)
	if !strings.Contains(emptyReasonBody, "Alasan perubahan status wajib diisi") || !strings.Contains(emptyReasonBody, `name="reason"`) {
		t.Fatal("validasi alasan kosong tidak ditampilkan dekat form")
	}
	hxInvalid, hxInvalidHeaders := postHX(kesiswaan, statusPath, unchanged, http.StatusUnprocessableEntity)
	if !strings.Contains(hxInvalid, `id="student-guidance-detail-region"`) || strings.Contains(strings.ToLower(hxInvalid), "<!doctype html>") || !strings.Contains(hxInvalid, "Alasan untuk status yang sama") || hxInvalidHeaders.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("validasi HTMX tidak mengembalikan fragmen detail dengan form terjaga")
	}
	completed := url.Values{
		"gorilla.csrf.Token": {statusToken}, "version": {"1"},
		"status": {"selesai"}, "reason": {"Tindak lanjut selesai"},
	}
	completedBody, _ := postHX(kesiswaan, statusPath, completed, http.StatusOK)
	if !strings.Contains(completedBody, `id="student-guidance-detail-region"`) || !strings.Contains(completedBody, "Tindak lanjut selesai") || !strings.Contains(completedBody, "Status catatan berhasil diubah") {
		t.Fatal("sukses HTMX tidak mengembalikan status dan histori terbaru")
	}
	staleVersion := url.Values{
		"gorilla.csrf.Token": {statusToken}, "version": {"1"},
		"status": {"perlu_tindak_lanjut"}, "reason": {"Percobaan dengan versi lama"},
	}
	conflictBody := post(kesiswaan, statusPath, staleVersion, http.StatusConflict)
	if !strings.Contains(strings.ToLower(conflictBody), "<!doctype html>") || !strings.Contains(conflictBody, "Catatan pembinaan berubah") || !strings.Contains(conflictBody, "Percobaan dengan versi lama") || !strings.Contains(conflictBody, `name="version" value="2"`) {
		t.Fatal("konflik versi tanpa HTMX tidak mempertahankan detail dan isian form")
	}
	hxConflict, _ := postHX(kesiswaan, statusPath, staleVersion, http.StatusConflict)
	if !strings.Contains(hxConflict, `id="student-guidance-detail-region"`) || strings.Contains(strings.ToLower(hxConflict), "<!doctype html>") || !strings.Contains(hxConflict, "Catatan pembinaan berubah") || !strings.Contains(hxConflict, "Percobaan dengan versi lama") {
		t.Fatal("konflik versi HTMX tidak mengembalikan fragmen inline")
	}
	var afterVersion, afterHistory int
	var afterStatus string
	if err = db.QueryRow(`SELECT r.version,r.status,(SELECT count(*) FROM app.student_guidance_history h WHERE h.record_id=r.id) FROM app.student_guidance_records r WHERE r.id=$1`, recordID).Scan(&afterVersion, &afterStatus, &afterHistory); err != nil || afterVersion != 2 || afterStatus != "selesai" || afterHistory != 2 {
		t.Fatalf("validasi/konflik status memutasi lebih dari satu kali: version=%d status=%s history=%d err=%v", afterVersion, afterStatus, afterHistory, err)
	}
}
