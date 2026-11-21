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

// Tes HTTP menggunakan fixture sintetis TEST yang sudah tersedia dan hanya
// membaca database; tidak menulis sesi atau catatan akademik.
func TestHistoricalTPPreviewHTTPReadOnlyIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	var semester, class, subject, actor int64
	err := db.QueryRow(`SELECT a.semester_id,a.class_id,a.subject_id,u.id FROM app.teaching_assignments a
	 JOIN app.teachers t ON t.id=a.teacher_id AND t.active JOIN app.users u ON u.id=t.user_id AND u.active
	 JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' JOIN app.subjects s ON s.id=a.subject_id AND s.active
	 JOIN app.semester_calendars cal ON cal.semester_id=a.semester_id
	 WHERE a.effective_from<=(now() AT TIME ZONE 'Asia/Jakarta')::date AND (a.effective_until IS NULL OR a.effective_until>(now() AT TIME ZONE 'Asia/Jakarta')::date)
	 AND EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=a.semester_id AND f.class_id=a.class_id AND f.subject_id=a.subject_id)
	 AND u.username LIKE 'fixture_cp1009_%'
	 AND (SELECT count(*) FROM app.student_placements p JOIN app.semesters sm ON sm.id=a.semester_id AND p.school_year_id=sm.school_year_id WHERE p.class_id=a.class_id)>=30
	 ORDER BY a.id LIMIT 1`).Scan(&semester, &class, &subject, &actor)
	if err != nil {
		t.Fatalf("fixture sintetis TEST dengan Final diperlukan: %v", err)
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	h, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: &academic.Store{DB: db}, CSRFKey: key})
	if err != nil {
		t.Fatal(err)
	}
	user := auth.User{ID: actor, Active: true, Roles: []string{"guru_mapel"}}
	state := func() string {
		t.Helper()
		var hash string
		err := db.QueryRow(`SELECT md5(jsonb_build_object(
		 'objectives',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM app.learning_objectives o WHERE o.semester_id=$1 AND o.class_id=$2 AND o.subject_id=$3),
		 'activities',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM app.assessment_activities a WHERE a.semester_id=$1 AND a.class_id=$2 AND a.subject_id=$3),
		 'scores',(SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM app.assessment_activity_initial_scores s WHERE s.semester_id=$1 AND s.class_id=$2 AND s.subject_id=$3),
		 'finals',(SELECT jsonb_agg(to_jsonb(f) ORDER BY f.id) FROM app.semester_final_versions f WHERE f.semester_id=$1 AND f.class_id=$2 AND f.subject_id=$3))::text)`, semester, class, subject).Scan(&hash)
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	before := state()
	request := func(method string, values url.Values, cookies []*http.Cookie, u auth.User) *httptest.ResponseRecorder {
		body := ""
		if values != nil {
			body = values.Encode()
		}
		req := httptest.NewRequest(method, "/guru/tujuan-pembelajaran/historis", strings.NewReader(body))
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, &u))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	page := request("GET", nil, nil, user)
	if page.Code != 200 {
		t.Fatalf("GET: %d", page.Code)
	}
	if denied := request("GET", nil, nil, auth.User{ID: actor, Active: true, Roles: []string{"admin"}}); denied.Code != 403 {
		t.Fatalf("role lain: %d", denied.Code)
	}
	m := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
	if len(m) != 2 {
		t.Fatal("CSRF tidak ada")
	}
	values := url.Values{"context": {fmt.Sprintf("%d:%d:%d", semester, class, subject)}, "definition": {"TP historis é"}, "reason": {"Alasan uji"}, "assessment_date": {"2020-01-10"}, "date_source": {"arsip"}, "learning_kind": {"manual"}, "learning_document": {"jurnal"}, "learning_archive": {"arsip"}, "learning_teacher": {"guru"}, "learning_date": {"2020-01-10"}, "assessment_kind": {"manual"}, "assessment_document": {"sumatif"}, "assessment_archive": {"arsip"}, "assessment_teacher": {"guru"}, "result_source": {"dokumen"}}
	if w := request("POST", values, nil, user); w.Code != 403 {
		t.Fatalf("CSRF: %d", w.Code)
	}
	values.Set("gorilla.csrf.Token", m[1])
	w := request("POST", values, page.Result().Cookies(), user)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Perlu Verifikasi Manual") || !strings.Contains(w.Body.String(), "Belum dapat dihitung") {
		t.Fatalf("pratinjau: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Halaman 1 dari") {
		t.Fatal("pagination fixture >=30 siswa tidak tampil")
	}
	for _, label := range []string{"Seluruh siswa teridentifikasi:", "Kandidat anggota kelas:", "Bukan anggota kelas pada tanggal penilaian:", "Belum dapat diverifikasi:", "Klasifikasi ini bukan penetapan kewajiban"} {
		if !strings.Contains(w.Body.String(), label) {
			t.Fatalf("ringkasan %q tidak ditampilkan", label)
		}
	}
	snapshot := regexp.MustCompile(`name="snapshot_token" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
	if len(snapshot) != 2 {
		t.Fatal("fingerprint pagination tidak ada")
	}
	values.Set("snapshot_token", snapshot[1])
	values.Set("action", "confirm")
	values.Set("submission_key", "00000000-0000-4000-8000-000000002055")
	if denied := request("POST", values, nil, user); denied.Code != 403 {
		t.Fatalf("konfirmasi tanpa CSRF: %d", denied.Code)
	}
	if invalid := request("POST", values, page.Result().Cookies(), user); invalid.Code != 422 {
		t.Fatalf("konfirmasi tanpa pemeriksaan bukti: %d", invalid.Code)
	}
	values.Del("action")
	values.Del("submission_key")
	values.Set("page", "2")
	second := request("POST", values, page.Result().Cookies(), user)
	if second.Code != 200 || !strings.Contains(second.Body.String(), "Halaman 2 dari") || !strings.Contains(second.Body.String(), "TP historis é") {
		t.Fatalf("pagination maju: %d", second.Code)
	}
	summaryPattern := regexp.MustCompile(`<p>Seluruh siswa teridentifikasi:.*?</p>`)
	firstSummary, secondSummary := summaryPattern.FindString(w.Body.String()), summaryPattern.FindString(second.Body.String())
	if firstSummary == "" || firstSummary != secondSummary {
		t.Fatal("ringkasan siswa berubah antarhalaman")
	}
	values.Set("page", "1")
	back := request("POST", values, page.Result().Cookies(), user)
	if back.Code != 200 || !strings.Contains(back.Body.String(), "Halaman 1 dari") {
		t.Fatalf("pagination kembali: %d", back.Code)
	}
	var activityID, sourceSemester, sourceClass, sourceSubject, sourceActor int64
	var activityDate string
	if err = db.QueryRow(`SELECT x.id,x.activity_date::text,a.semester_id,a.class_id,a.subject_id,u.id FROM app.teaching_assignments a
	 JOIN app.teachers t ON t.id=a.teacher_id AND t.active JOIN app.users u ON u.id=t.user_id AND u.active
	 JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' JOIN app.subjects s ON s.id=a.subject_id AND s.active
	 JOIN app.semester_calendars cal ON cal.semester_id=a.semester_id
	 JOIN app.assessment_activities x ON x.semester_id=a.semester_id AND x.class_id=a.class_id AND x.subject_id=a.subject_id AND x.teacher_id=t.id AND x.activity_type='sumatif'
	 WHERE a.effective_from<=(now() AT TIME ZONE 'Asia/Jakarta')::date AND (a.effective_until IS NULL OR a.effective_until>(now() AT TIME ZONE 'Asia/Jakarta')::date)
	 AND u.username LIKE 'fixture_cp1009_%' AND EXISTS(SELECT 1 FROM app.semester_final_versions f WHERE f.semester_id=a.semester_id AND f.class_id=a.class_id AND f.subject_id=a.subject_id)
	 ORDER BY a.id,x.id LIMIT 1`).Scan(&activityID, &activityDate, &sourceSemester, &sourceClass, &sourceSubject, &sourceActor); err != nil {
		t.Fatalf("fixture Sumatif aplikasi: %v", err)
	}
	sourceUser := auth.User{ID: sourceActor, Active: true, Roles: []string{"guru_mapel"}}
	sourcePage := request("GET", nil, nil, sourceUser)
	sourceCSRF := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(sourcePage.Body.String())
	if sourcePage.Code != 200 || len(sourceCSRF) != 2 {
		t.Fatal("CSRF sumber tidak tersedia")
	}
	values.Del("page")
	values.Del("snapshot_token")
	values.Set("context", fmt.Sprintf("%d:%d:%d", sourceSemester, sourceClass, sourceSubject))
	values.Set("gorilla.csrf.Token", sourceCSRF[1])
	values.Set("assessment_kind", "app")
	values.Set("assessment_id", fmt.Sprint(activityID))
	values.Set("assessment_date", activityDate)
	values.Set("assessment_teacher", "Nama yang bertentangan")
	badMetadata := request("POST", values, sourcePage.Result().Cookies(), sourceUser)
	if badMetadata.Code != 200 || !strings.Contains(badMetadata.Body.String(), "Sumatif: Referensi tidak memadai") {
		t.Fatalf("guru Sumatif bertentangan: %d %s", badMetadata.Code, badMetadata.Body.String())
	}
	values.Set("assessment_kind", "manual")
	values.Set("assessment_date", "2020-01-10")
	values.Set("assessment_teacher", "guru")
	values.Set("context", fmt.Sprintf("%d:%d:%d", semester, class, subject))
	values.Set("gorilla.csrf.Token", m[1])
	if strings.Contains(w.Header().Get("Location"), "TP historis") || strings.Contains(w.Header().Get("Location"), "Alasan uji") {
		t.Fatal("input sensitif bocor dalam URL")
	}
	values.Set("page", "2")
	values.Set("snapshot_token", "stale")
	w = request("POST", values, page.Result().Cookies(), user)
	if w.Code != 409 {
		t.Fatalf("token basi: %d", w.Code)
	}
	values.Del("page")
	values.Del("snapshot_token")
	values.Set("context", fmt.Sprintf("%d:%d:%d", semester, class+999999, subject))
	w = request("POST", values, page.Result().Cookies(), user)
	if w.Code != 403 {
		t.Fatalf("lintas penugasan: %d", w.Code)
	}
	if after := state(); after != before {
		t.Fatal("HTTP pratinjau mengubah bukti akademik atau Final")
	}
}
