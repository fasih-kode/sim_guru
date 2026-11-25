//go:build integration

package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

// Pengujian hanya membaca fixture RD-02; tidak membuat akun atau nilai.
func TestAssessmentContextCrossTeacherReadOnlyIntegration(t *testing.T) {
	if os.Getenv("SIM_GURU_RUN_RD02_CONTEXT_SECURITY_TEST") != "1" {
		t.Skip("tes otorisasi lintas guru RD-02 hanya berjalan dengan SIM_GURU_RUN_RD02_CONTEXT_SECURITY_TEST=1")
	}

	db := testutil.DatabaseForTarget(t, "../../.env", "sim_guru_rd02_s06_20261123_test", 84604)
	rows, err := db.Query(`SELECT DISTINCT t.user_id FROM app.teachers t JOIN app.users u ON u.id=t.user_id AND u.active JOIN app.user_roles r ON r.user_id=u.id AND r.role_code='guru_mapel' JOIN app.teaching_assignments a ON a.teacher_id=t.id WHERE t.active AND a.effective_from<=current_date AND (a.effective_until IS NULL OR a.effective_until>current_date) ORDER BY t.user_id`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	store := &academic.Store{DB: db}
	type candidate struct {
		id int64
		c  academic.LearningObjectiveContext
	}
	var candidates []candidate
	for _, id := range ids {
		cs, e := store.LearningObjectiveContexts(context.Background(), id)
		if e != nil {
			t.Fatal(e)
		}
		for _, c := range cs {
			candidates = append(candidates, candidate{id, c})
		}
	}
	var a, b candidate
	found := false
	for _, x := range candidates {
		for _, y := range candidates {
			if x.id == y.id || (x.c.SemesterID == y.c.SemesterID && x.c.ClassID == y.c.ClassID && x.c.SubjectID == y.c.SubjectID) {
				continue
			}
			owns := false
			for _, z := range candidates {
				if (z.id == x.id && z.c.SemesterID == y.c.SemesterID && z.c.ClassID == y.c.ClassID && z.c.SubjectID == y.c.SubjectID) || (z.id == y.id && z.c.SemesterID == x.c.SemesterID && z.c.ClassID == x.c.ClassID && z.c.SubjectID == x.c.SubjectID) {
					owns = true
					break
				}
			}
			if !owns {
				a, b = x, y
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("dua guru dengan penugasan terpisah tidak tersedia")
	}
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: &master.Store{DB: db}, Academics: store, CSRFKey: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/guru/tujuan-pembelajaran", "/guru/buku-nilai", "/guru/nilai-asts-asas?tab=asts", "/guru/nilai-asts-asas?tab=asas", "/guru/rekap-nilai-tp", "/guru/rekomendasi-semester"}
	request := func(path string, user int64, c academic.LearningObjectiveContext) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		q := r.URL.Query()
		q.Set("context", fmt.Sprintf("%d:%d:%d", c.SemesterID, c.ClassID, c.SubjectID))
		r.URL.RawQuery = q.Encode()
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &auth.User{ID: user, Active: true, Roles: []string{"guru_mapel"}}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			wa, wb, denied := request(path, a.id, a.c), request(path, b.id, b.c), request(path, a.id, b.c)
			if wa.Code != http.StatusOK || wb.Code != http.StatusOK {
				t.Fatalf("akses sah A=%d B=%d", wa.Code, wb.Code)
			}
			if reverse := request(path, b.id, a.c); reverse.Code != http.StatusForbidden {
				t.Fatalf("URL lama guru A dibuka guru B: %d", reverse.Code)
			}
			blank := httptest.NewRequest("GET", path, nil)
			blank = blank.WithContext(context.WithValue(blank.Context(), principalKey{}, &auth.User{ID: b.id, Active: true, Roles: []string{"guru_mapel"}}))
			wbBlank := httptest.NewRecorder()
			handler.ServeHTTP(wbBlank, blank)
			if wbBlank.Code != http.StatusOK || strings.Contains(wbBlank.Body.String(), `<p class="assessment-context-summary">`) {
				t.Fatal("permintaan guru B tanpa konteks memilih konteks otomatis")
			}
			if denied.Code != http.StatusForbidden {
				t.Fatalf("akses silang mendapat %d", denied.Code)
			}
			if strings.Contains(denied.Body.String(), b.c.SubjectName) || strings.Contains(denied.Body.String(), b.c.ClassName) {
				t.Fatal("respons penolakan memuat identitas konteks asing")
			}
		})
	}
	t.Run("detail rekap", func(t *testing.T) {
		var owner candidate
		var rec academic.AssessmentTPRecap
		for _, c := range candidates {
			var err error
			rec, err = store.AssessmentTPRecap(context.Background(), c.id, c.c.SemesterID, c.c.ClassID, c.c.SubjectID)
			if err != nil {
				t.Fatal(err)
			}
			if len(rec.Students) > 0 {
				owner = c
				break
			}
		}
		if len(rec.Students) == 0 {
			t.Fatal("fixture rekap tidak memiliki siswa")
		}
		pupil := rec.Students[0]
		path := "/guru/rekap-nilai-tp?student_id="
		page := request(path+pupil.ID+"&q=contoh", owner.id, owner.c)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Kembali ke daftar siswa") || strings.Contains(page.Body.String(), `id="recap-students"`) {
			t.Fatalf("detail siswa sendiri tidak berhasil: %d", page.Code)
		}
		if page := request(path+"0", owner.id, owner.c); page.Code != http.StatusUnprocessableEntity {
			t.Fatalf("ID siswa tidak valid: %d", page.Code)
		}
		missing := request(path+"9223372036854775807", owner.id, owner.c)
		if missing.Code != http.StatusNotFound || strings.Contains(missing.Body.String(), "Kembali ke daftar siswa") {
			t.Fatalf("ID siswa di luar roster: %d", missing.Code)
		}
		if page := request(path+pupil.ID, a.id, b.c); page.Code != http.StatusForbidden || strings.Contains(page.Body.String(), "Kembali ke daftar siswa") {
			t.Fatalf("detail pada penugasan asing: %d", page.Code)
		}
		roster := make(map[string]bool, len(rec.Students))
		for _, s := range rec.Students {
			roster[s.ID] = true
		}
		for _, c := range candidates {
			if c.c.ClassID == owner.c.ClassID {
				continue
			}
			foreign, err := store.AssessmentTPRecap(context.Background(), c.id, c.c.SemesterID, c.c.ClassID, c.c.SubjectID)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range foreign.Students {
				if roster[s.ID] {
					continue
				}
				page := request(path+s.ID, owner.id, owner.c)
				if page.Code != missing.Code || page.Body.String() != missing.Body.String() {
					t.Fatalf("respons siswa kelas lain berbeda dari ID yang tidak ditemukan: %d", page.Code)
				}
				return
			}
		}
		t.Fatal("fixture tidak memiliki siswa dengan keanggotaan kelas berbeda")
	})
}
