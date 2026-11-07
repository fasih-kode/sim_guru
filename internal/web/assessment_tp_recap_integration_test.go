//go:build integration

package web

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAssessmentTPRecapHTTPContextAuthorizationIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	other, _ := testutil.User(t, db, "guru_mapel")
	suffix := testutil.Random(t)[:9]
	var yearID, semesterID, classID, otherClassID, subjectID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "HTTP Rekap "+suffix).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	for _, target := range []*int64{&classID, &otherClassID} {
		if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, testutil.Random(t)[:7]).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	today := time.Now().In(time.FixedZone("WIB", 7*60*60))
	if _, err := db.Exec(`INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,$2::date,$3::date,'[1,2,3,4,5]'::jsonb)`, semesterID, today.AddDate(0, 0, -20).Format("2006-01-02"), today.AddDate(0, 0, 20).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "RHTTP-"+suffix, "Mapel Rekap HTTP "+suffix).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	masters := &master.Store{DB: db}
	teacherIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru rekap HTTP " + suffix, UserID: teacher.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	otherIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru lain rekap HTTP " + suffix, UserID: other.ID, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	service := &academic.Store{DB: db}
	from := today.AddDate(0, 0, -5)
	if err = service.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, teacherIdentity.ID, from); err != nil {
		t.Fatal("penugasan guru utama", err)
	}
	if err = service.AssignTeacher(ctx, curriculum.ID, semesterID, otherClassID, subjectID, otherIdentity.ID, from); err != nil {
		t.Fatal("penugasan guru pembanding", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id=$1)`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id=$1`, semesterID)
		for _, id := range []int64{teacherIdentity.ID, otherIdentity.ID} {
			_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, id)
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, id)
		}
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code=$1`, "RHTTP-"+suffix)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id=$1`, subjectID)
		_, _ = db.Exec(`DELETE FROM app.semester_calendars WHERE semester_id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.classes WHERE id IN ($1,$2)`, classID, otherClassID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		for _, id := range []int64{admin.ID, curriculum.ID, teacher.ID, other.ID} {
			for _, query := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, e := db.Exec(query, id); e != nil {
					t.Errorf("hapus akun fixture: %v", e)
					return
				}
			}
		}
	})
	accounts, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	csrfKey := make([]byte, 32)
	if _, err = rand.Read(csrfKey); err != nil {
		t.Fatal(err)
	}
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: accounts, Masters: masters, Academics: service, CSRFKey: csrfKey})
	if err != nil {
		t.Fatal(err)
	}
	path := "/guru/rekap-nilai-tp?context=" + strconv.FormatInt(semesterID, 10) + ":" + strconv.FormatInt(classID, 10) + ":" + strconv.FormatInt(subjectID, 10)
	requestFor := func(user auth.User) *http.Request {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		return r.WithContext(context.WithValue(r.Context(), principalKey{}, &user))
	}
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, requestFor(auth.User{ID: teacher.ID, Active: true, Roles: []string{"guru_mapel"}}))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `data-navigation-tab="assessment-tp-recap"`) || !strings.Contains(page.Body.String(), "NH adalah rata-rata tanpa bobot") {
		t.Fatalf("Guru Mapel dengan penugasan tidak melihat rekap: status=%d body=%s", page.Code, page.Body.String())
	}
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, requestFor(auth.User{ID: other.ID, Active: true, Roles: []string{"guru_mapel"}}))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("guru yang hanya diampu kelas lain harus ditolak pada HTTP: status=%d body=%s", denied.Code, denied.Body.String())
	}
	roleDenied := httptest.NewRecorder()
	handler.ServeHTTP(roleDenied, requestFor(auth.User{ID: teacher.ID, Active: true, Roles: []string{"kurikulum"}}))
	if roleDenied.Code != http.StatusForbidden {
		t.Fatalf("role tanpa Guru Mapel harus ditolak: status=%d", roleDenied.Code)
	}
}
