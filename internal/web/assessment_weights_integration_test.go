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

func TestAssessmentWeightsHTTPAuthorizationAndUATFlowIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	curriculum, curriculumPassword := testutil.User(t, db, "kurikulum")
	teacher, teacherPassword := testutil.User(t, db, "guru_mapel")
	var yearID, semesterID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "Bobot web "+testutil.Random(t)[:12]).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM app.semester_assessment_weight_history WHERE semester_id=$1`, `DELETE FROM app.semester_assessment_weights WHERE semester_id=$1`, `DELETE FROM app.semesters WHERE id=$1`} {
			if _, err := db.Exec(q, semesterID); err != nil {
				t.Errorf("cleanup bobot HTTP: %v", err)
				return
			}
		}
		if _, err := db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID); err != nil {
			t.Errorf("cleanup tahun ajaran HTTP: %v", err)
		}
		for _, userID := range []int64{curriculum.ID, teacher.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, err := db.Exec(q, userID); err != nil {
					t.Errorf("cleanup akun HTTP: %v", err)
					return
				}
			}
		}
	})
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	academics := &academic.Store{DB: db}
	csrfKey := make([]byte, 32)
	if _, err = rand.Read(csrfKey); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: academics, CSRFKey: csrfKey})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	csrfRE := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`)
	clientFor := func(user auth.User, password string) (*http.Client, string) {
		t.Helper()
		jar, e := cookiejar.New(nil)
		if e != nil {
			t.Fatal(e)
		}
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		page, e := client.Get(server.URL + "/login")
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(page.Body)
		_ = page.Body.Close()
		match := csrfRE.FindSubmatch(body)
		if len(match) != 2 {
			t.Fatal("token CSRF pada halaman masuk tidak ditemukan")
		}
		token, e := accounts.Login(ctx, user.Username, password, "")
		if e != nil {
			t.Fatal("login fixture gagal", e)
		}
		baseURL, _ := url.Parse(server.URL)
		jar.SetCookies(baseURL, []*http.Cookie{{Name: sessionCookie, Value: token, Path: "/"}})
		return client, string(match[1])
	}
	semesterPath := "/akademik/bobot-penilaian?semester_id=" + strconv.FormatInt(semesterID, 10)
	teacherClient, teacherCSRF := clientFor(teacher, teacherPassword)
	postTeacher, err := teacherClient.PostForm(server.URL+semesterPath, url.Values{
		"gorilla.csrf.Token": {teacherCSRF}, "nh": {"60"}, "asts": {"20"}, "asas": {"20"}, "version": {"0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = postTeacher.Body.Close()
	if postTeacher.StatusCode != http.StatusForbidden {
		t.Fatalf("POST Guru Mapel harus ditolak, status=%d", postTeacher.StatusCode)
	}
	var rows int
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_assessment_weights WHERE semester_id=$1`, semesterID).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("POST guru tidak boleh menyimpan data: rows=%d err=%v", rows, err)
	}
	curriculumClient, curriculumCSRF := clientFor(curriculum, curriculumPassword)
	get, err := curriculumClient.Get(server.URL + semesterPath)
	if err != nil {
		t.Fatal(err)
	}
	getBody, _ := io.ReadAll(get.Body)
	_ = get.Body.Close()
	if get.StatusCode != http.StatusOK || !strings.Contains(string(getBody), "Bobot Penilaian") || !strings.Contains(string(getBody), "Belum ada bobot bawaan") || !strings.Contains(string(getBody), "curriculum-assessment-weights") {
		t.Fatalf("halaman bobot tidak siap untuk Kurikulum: %d", get.StatusCode)
	}
	invalid, err := curriculumClient.PostForm(server.URL+semesterPath, url.Values{
		"gorilla.csrf.Token": {curriculumCSRF}, "nh": {"60"}, "asts": {"30"}, "asas": {"20"}, "version": {"0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidBody, _ := io.ReadAll(invalid.Body)
	_ = invalid.Body.Close()
	if invalid.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(invalidBody), "Total bobot NH, ASTS, dan ASAS harus tepat 100%") {
		t.Fatalf("total 110 harus ditolak dengan pesan jelas, status=%d", invalid.StatusCode)
	}
	if got := strings.Count(string(invalidBody), "Total bobot NH, ASTS, dan ASAS harus tepat 100%"); got != 1 {
		t.Fatalf("pesan validasi harus tampil satu kali, ditemukan %d", got)
	}
	if err = db.QueryRow(`SELECT count(*) FROM app.semester_assessment_weights WHERE semester_id=$1`, semesterID).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("total invalid tidak boleh tersimpan: rows=%d err=%v", rows, err)
	}
	valid, err := curriculumClient.PostForm(server.URL+semesterPath, url.Values{
		"gorilla.csrf.Token": {curriculumCSRF}, "nh": {"60"}, "asts": {"20"}, "asas": {"20"}, "version": {"0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = valid.Body.Close()
	if valid.StatusCode != http.StatusSeeOther || valid.Header.Get("Location") == "" {
		t.Fatalf("bobot 100%% harus tersimpan dan redirect, status=%d", valid.StatusCode)
	}
	var nh, asts, asas int
	if err = db.QueryRow(`SELECT nh,asts,asas FROM app.semester_assessment_weights WHERE semester_id=$1`, semesterID).Scan(&nh, &asts, &asas); err != nil || nh != 60 || asts != 20 || asas != 20 {
		t.Fatalf("data bobot tersimpan salah: %d/%d/%d err=%v", nh, asts, asas, err)
	}
}
