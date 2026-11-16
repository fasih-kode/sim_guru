//go:build integration

package web

import (
	"context"
	"crypto/rand"
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
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Simpan koreksi rutin") || !strings.Contains(page.Body.String(), "TP ini belum digunakan") {
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
	usedPage := request(http.MethodGet, path, nil, nil, owner)
	if usedPage.Code != http.StatusOK || !strings.Contains(usedPage.Body.String(), "Pratinjau dampak") || strings.Contains(usedPage.Body.String(), "Simpan koreksi rutin") {
		t.Fatalf("TP terpakai harus melalui jalur pratinjau: %d %s", usedPage.Code, usedPage.Body.String())
	}
	values := url.Values{"expected_version": {"1"}, "submission_key": {"00000000-0000-4000-8000-000000001851"}, "definition": {"TP HTTP CP10-18 substansi"}, "change_type": {"substansi"}, "reason": {"Penyelarasan substansi"}, "gorilla.csrf.Token": {regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(usedPage.Body.String())[1]}}
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
	staleValues := url.Values{"expected_version": {"2"}, "submission_key": {"00000000-0000-4000-8000-000000001852"}, "definition": {"Definisi yang menjadi basi"}, "change_type": {"redaksi"}, "reason": {"Alasan pratinjau basi"}, "gorilla.csrf.Token": {regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(stalePage.Body.String())[1]}}
	staleRendered := request(http.MethodPost, path+"/pratinjau", staleValues, stalePage.Result().Cookies(), owner)
	if staleRendered.Code != http.StatusOK {
		t.Fatalf("pratinjau lama: %d %s", staleRendered.Code, staleRendered.Body.String())
	}
	winningPreview, err := s.PreviewLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, 2, "Definisi yang menang lebih dahulu", "redaksi", "Koreksi sah", 1, "00000000-0000-4000-8000-000000001853")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmLearningObjectiveCorrection(ctx, f.Teacher, objectiveID, 2, winningPreview.DefinitionAfter, winningPreview.ChangeType, winningPreview.Reason, winningPreview.SnapshotToken, winningPreview.SubmissionKey); err != nil {
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
	if err = db.QueryRow(`SELECT count(*) FROM app.learning_objective_definition_revisions WHERE learning_objective_id=$1`, objectiveID).Scan(&revisions); err != nil || revisions != 3 {
		t.Fatalf("konflik tidak boleh menambah histori: count=%d err=%v", revisions, err)
	}
}
