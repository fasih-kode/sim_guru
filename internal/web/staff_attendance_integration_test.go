//go:build integration

package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStaffAttendanceHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	service := &academic.Store{DB: db}
	initial, _, err := service.TeacherAttendanceLocation(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	var historyBefore int64
	if err = db.QueryRow(`SELECT coalesce(max(id),0) FROM app.teacher_attendance_location_history`).Scan(&historyBefore); err != nil {
		t.Fatal(err)
	}
	if err = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: -6.2, Longitude: 106.8, RadiusM: 100, ExpectedVersion: initial.Version, Reason: "Fixture sintetis kehadiran pegawai HTTP"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location_history WHERE actor_id=$1 AND id>$2`, admin.ID, historyBefore)
		if initial.Configured {
			if _, e := db.Exec(`UPDATE app.teacher_attendance_location SET updated_by=$1 WHERE id=1`, initial.UpdatedBy); e != nil {
				t.Errorf("restore location editor: %v", e)
				return
			}
		}
		for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
			if _, e := db.Exec(q, admin.ID); e != nil {
				t.Errorf("cleanup location admin fixture: %v", e)
				return
			}
		}
	})
	if initial.Configured {
		t.Cleanup(func() {
			latest, _, e := service.TeacherAttendanceLocation(ctx, admin.ID)
			if e == nil {
				_ = service.SaveTeacherAttendanceLocation(ctx, admin.ID, academic.TeacherAttendanceLocationInput{Latitude: initial.Latitude, Longitude: initial.Longitude, RadiusM: initial.RadiusM, ExpectedVersion: latest.Version, Reason: "Pulihkan konfigurasi sesudah tes HTTP TU"})
			}
		})
	} else {
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location_history WHERE location_id=1`)
			_, _ = db.Exec(`DELETE FROM app.teacher_attendance_location WHERE id=1`)
		})
	}
	base := t.TempDir()
	repo, public, private := filepath.Join(base, "repo"), filepath.Join(base, "public"), filepath.Join(base, "private")
	for _, dir := range []string{repo, public, private} {
		if err = os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	photos, err := teacherphoto.NewStore(teacherphoto.Config{Directory: filepath.Join(private, "staff-photos"), RepositoryRoot: repo, PublicRoots: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	defer photos.Close()
	service.StaffPhotos = photos
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	csrfKey := make([]byte, 32)
	if _, err = rand.Read(csrfKey); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: service, CSRFKey: csrfKey})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var users, staffIDs, eventIDs []int64
	t.Cleanup(func() {
		for _, staffID := range staffIDs {
			rows, e := db.Query(`SELECT id FROM app.staff_attendance_events WHERE staff_member_id=$1`, staffID)
			if e != nil {
				t.Error(e)
				continue
			}
			var ids []int64
			for rows.Next() {
				var id int64
				if e = rows.Scan(&id); e != nil {
					t.Error(e)
					break
				}
				ids = append(ids, id)
			}
			_ = rows.Close()
			eventIDs = append(eventIDs, ids...)
		}
		tx, e := db.Begin()
		if e != nil {
			t.Error(e)
			return
		}
		defer tx.Rollback()
		seen := map[int64]bool{}
		for _, id := range eventIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			for _, q := range []string{`DELETE FROM app.staff_attendance_history WHERE event_id=$1`, `DELETE FROM app.staff_attendance_photos WHERE event_id=$1`, `DELETE FROM app.staff_attendance_events WHERE id=$1`} {
				if _, e = tx.Exec(q, id); e != nil {
					t.Error(e)
					return
				}
			}
		}
		for _, id := range staffIDs {
			if _, e = tx.Exec(`DELETE FROM app.staff_members WHERE id=$1`, id); e != nil {
				t.Error(e)
				return
			}
		}
		for _, id := range users {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, e = tx.Exec(q, id); e != nil {
					t.Error(e)
					return
				}
			}
		}
		if e = tx.Commit(); e != nil {
			t.Error(e)
		}
		keys, _ := photos.ListKeys()
		for _, key := range keys {
			_ = photos.Delete(key)
		}
	})
	csrfRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	type account struct {
		id, staffID        int64
		username, password string
		client             *http.Client
	}
	makeAccount := func(name string, active, linked bool, roles ...string) account {
		t.Helper()
		u, password := testutil.User(t, db, roles...)
		users = append(users, u.ID)
		a := account{id: u.ID, username: u.Username, password: password}
		if linked {
			if e := db.QueryRow(`INSERT INTO app.staff_members(name,user_id,active) VALUES($1,$2,$3) RETURNING id`, name, u.ID, active).Scan(&a.staffID); e != nil {
				t.Fatal(e)
			}
			staffIDs = append(staffIDs, a.staffID)
		}
		jar, _ := cookiejar.New(nil)
		a.client = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		loginPage, e := a.client.Get(server.URL + "/login")
		if e != nil {
			t.Fatal(e)
		}
		loginBody, _ := io.ReadAll(loginPage.Body)
		_ = loginPage.Body.Close()
		match := csrfRE.FindSubmatch(loginBody)
		if len(match) != 2 {
			t.Fatal("login csrf missing")
		}
		response, e := a.client.PostForm(server.URL+"/login", url.Values{"username": {a.username}, "password": {a.password}, "gorilla.csrf.Token": {string(match[1])}})
		if e != nil {
			t.Fatal(e)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusSeeOther {
			t.Fatalf("login status %d", response.StatusCode)
		}
		return a
	}
	read := func(client *http.Client, path string, want int) string {
		t.Helper()
		resp, e := client.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("GET %s status=%d want=%d", path, resp.StatusCode, want)
		}
		return string(body)
	}
	owner := makeAccount("Pegawai HTTP A", true, true, "pegawai_tu", "guru_mapel")
	other := makeAccount("Pegawai HTTP B", true, true, "pegawai_tu")
	noProfile := makeAccount("", true, false, "pegawai_tu")
	noRole := makeAccount("Pegawai tanpa role", true, true, "guru_mapel")
	inactive := makeAccount("Pegawai nonaktif HTTP", false, true, "pegawai_tu")
	headmaster := makeAccount("Kepala sekolah staf", true, true, "kepala_sekolah")
	headmasterNoIdentity := makeAccount("", true, false, "kepala_sekolah")
	inactiveHeadmaster := makeAccount("Kepala sekolah nonaktif", false, true, "kepala_sekolah")
	var beforeHomeEvents int
	if err = db.QueryRow(`SELECT count(*) FROM app.staff_attendance_events WHERE staff_member_id=$1`, owner.staffID).Scan(&beforeHomeEvents); err != nil {
		t.Fatal(err)
	}
	home := read(owner.client, "/", http.StatusOK)
	if !strings.Contains(home, `href="/pegawai/kehadiran-saya#attendance-check-in"`) || !strings.Contains(home, `href="/pegawai/kehadiran-saya#attendance-check-out"`) || strings.Contains(home, ">Muat ulang") {
		t.Fatal("role Pegawai TU + Guru tanpa identitas guru aktif harus tetap mendapat pintasan TU, tanpa muat ulang")
	}
	var afterHomeEvents int
	if err = db.QueryRow(`SELECT count(*) FROM app.staff_attendance_events WHERE staff_member_id=$1`, owner.staffID).Scan(&afterHomeEvents); err != nil || afterHomeEvents != beforeHomeEvents {
		t.Fatalf("GET Beranda tidak boleh mencatat kehadiran: sebelum=%d sesudah=%d err=%v", beforeHomeEvents, afterHomeEvents, err)
	}
	page := read(owner.client, "/pegawai/kehadiran-saya", http.StatusOK)
	if !strings.Contains(page, "Kehadiran Pegawai") || !strings.Contains(page, "Catat Masuk") || !strings.Contains(page, "Catat Pulang") || !strings.Contains(page, "Ambil lokasi") || !strings.Contains(page, "Buka kamera") {
		t.Fatal("self-service page or role menu is incomplete")
	}
	if strings.Count(page, "Kehadiran Pegawai") != 1 {
		t.Fatal("role combination duplicated the staff attendance menu")
	}
	for _, account := range []account{noProfile, noRole, headmasterNoIdentity} {
		homeBody := read(account.client, "/", http.StatusOK)
		if !strings.Contains(homeBody, `disabled aria-disabled="true"`) || !strings.Contains(homeBody, "identitas Guru atau Pegawai TU aktif") || strings.Contains(homeBody, `href="/kehadiran-saya#`) || strings.Contains(homeBody, `href="/pegawai/kehadiran-saya#`) {
			t.Fatal("akun tanpa identitas presensi aktif harus melihat tombol nonaktif, bukan route yang akan ditolak")
		}
	}
	tokenMatch := csrfRE.FindStringSubmatch(page)
	if len(tokenMatch) != 2 {
		t.Fatal("staff attendance csrf missing")
	}
	csrfToken := tokenMatch[1]
	if body := read(noProfile.client, "/pegawai/kehadiran-saya", http.StatusForbidden); strings.Contains(body, "Pegawai HTTP A") {
		t.Fatal("TU without linked profile read another staff record")
	}
	read(noRole.client, "/pegawai/kehadiran-saya", http.StatusForbidden)
	read(headmasterNoIdentity.client, "/pegawai/kehadiran-saya", http.StatusForbidden)
	read(inactive.client, "/pegawai/kehadiran-saya", http.StatusOK)
	post := func(a account, token, key string, want int) string {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for field, value := range map[string]string{"gorilla.csrf.Token": token, "action": "check_in", "idempotency_key": key, "latitude": "-6.2", "longitude": "106.8", "accuracy_m": "5"} {
			if e := writer.WriteField(field, value); e != nil {
				t.Fatal(e)
			}
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="selfie"; filename="selfie.jpg"`)
		header.Set("Content-Type", "image/jpeg")
		part, e := writer.CreatePart(header)
		if e != nil {
			t.Fatal(e)
		}
		imageBytes := new(bytes.Buffer)
		_ = jpeg.Encode(imageBytes, image.NewRGBA(image.Rect(0, 0, 32, 32)), &jpeg.Options{Quality: 80})
		_, _ = part.Write(imageBytes.Bytes())
		if e = writer.Close(); e != nil {
			t.Fatal(e)
		}
		request, e := http.NewRequest(http.MethodPost, server.URL+"/pegawai/kehadiran-saya", &body)
		if e != nil {
			t.Fatal(e)
		}
		request.Header.Set("Content-Type", writer.FormDataContentType())
		response, e := a.client.Do(request)
		if e != nil {
			t.Fatal(e)
		}
		result, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("POST staff attendance status=%d want=%d body=%s", response.StatusCode, want, result)
		}
		return string(result)
	}
	headHome := read(headmaster.client, "/", http.StatusOK)
	if !strings.Contains(headHome, `href="/pegawai/kehadiran-saya#attendance-check-in"`) || !strings.Contains(headHome, `href="/pegawai/kehadiran-saya#attendance-check-out"`) {
		t.Fatal("Kepala Sekolah dengan identitas pegawai aktif harus mendapat pintasan presensi pegawai")
	}
	headPage := read(headmaster.client, "/pegawai/kehadiran-saya", http.StatusOK)
	headToken := csrfRE.FindStringSubmatch(headPage)
	if len(headToken) != 2 {
		t.Fatal("Kepala Sekolah tidak mendapat token CSRF presensi pegawai")
	}
	post(headmaster, headToken[1], "11223344556677889900aabbccddeeff", http.StatusSeeOther)
	inactiveHeadPage := read(inactiveHeadmaster.client, "/pegawai/kehadiran-saya", http.StatusOK)
	inactiveHeadToken := csrfRE.FindStringSubmatch(inactiveHeadPage)
	if len(inactiveHeadToken) != 2 {
		t.Fatal("halaman riwayat identitas nonaktif seharusnya tetap terbaca")
	}
	post(inactiveHeadmaster, inactiveHeadToken[1], "22334455667788990011aabbccddeeff", http.StatusForbidden)
	post(owner, "", "aabbccddeeff00112233445566778899", http.StatusForbidden)
	post(owner, csrfToken, "aabbccddeeff00112233445566778899", http.StatusSeeOther)
	var eventID, photoID int64
	if err = db.QueryRow(`SELECT id,photo_metadata_id FROM app.staff_attendance_events WHERE staff_member_id=$1 AND action='check_in'`, owner.staffID).Scan(&eventID, &photoID); err != nil {
		t.Fatal(err)
	}
	eventIDs = append(eventIDs, eventID)
	page = read(owner.client, "/pegawai/kehadiran-saya", http.StatusOK)
	if !strings.Contains(page, "Sudah Dicatat") || !strings.Contains(page, "Riwayat Kehadiran Saya") {
		t.Fatal("saved event missing from owner page")
	}
	resp, err := owner.client.Get(server.URL + "/pegawai/kehadiran-saya/foto/" + strconv.FormatInt(photoID, 10))
	if err != nil {
		t.Fatal(err)
	}
	photoBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(photoBody) == 0 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("owner photo endpoint unavailable or cacheable")
	}
	resp, err = other.client.Get(server.URL + "/pegawai/kehadiran-saya/foto/" + strconv.FormatInt(photoID, 10))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-staff photo status=%d", resp.StatusCode)
	}
	post(owner, csrfToken, "aabbccddeeff00112233445566778899", http.StatusSeeOther)
	var eventCount, photoCount, historyCount int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM app.staff_attendance_events WHERE id=$1),(SELECT count(*) FROM app.staff_attendance_photos WHERE event_id=$1),(SELECT count(*) FROM app.staff_attendance_history WHERE event_id=$1)`, eventID).Scan(&eventCount, &photoCount, &historyCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 || photoCount != 1 || historyCount != 1 {
		t.Fatalf("idempotent retry duplicated evidence: events=%d photos=%d history=%d", eventCount, photoCount, historyCount)
	}
	post(owner, csrfToken, "bbccddeeff00112233445566778899aa", http.StatusConflict)
	if _, err = db.Exec(`UPDATE app.staff_members SET active=false WHERE id=$1`, owner.staffID); err != nil {
		t.Fatal(err)
	}
	post(owner, csrfToken, "ccddeeff00112233445566778899aabb", http.StatusForbidden)
	page = read(owner.client, "/pegawai/kehadiran-saya", http.StatusOK)
	if !strings.Contains(page, "Pegawai Nonaktif") || !strings.Contains(page, "Sudah Dicatat") {
		t.Fatal("deactivation removed self history")
	}
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, other.id); err != nil {
		t.Fatal(err)
	}
	read(other.client, "/pegawai/kehadiran-saya", http.StatusSeeOther)
	var total int
	if err = db.QueryRow(`SELECT count(*) FROM app.teacher_attendance_events`).Scan(&total); err != nil {
		t.Fatal(err)
	}
}
