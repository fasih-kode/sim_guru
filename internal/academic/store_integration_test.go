//go:build integration

package academic_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestAcademicCalendarAndClassIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := academic.Store{DB: db}
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := testutil.Random(t)[:12]

	if _, err := s.Semesters(ctx, admin.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Admin tanpa role Kurikulum dapat membaca daftar semester")
	}
	if _, err := s.CreateYear(ctx, admin.ID, "Denied "+suffix); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Admin tanpa role Kurikulum dapat membuat tahun ajaran")
	}
	if _, err := s.Years(ctx, admin.ID); err != nil {
		t.Fatal("Admin tidak dapat membaca pilihan tahun untuk kelas:", err)
	}
	year1, err := s.CreateYear(ctx, curriculum.ID, "Tahun "+suffix+" A")
	if err != nil {
		t.Fatal(err)
	}
	terms, err := s.Semesters(ctx, curriculum.ID)
	if err != nil {
		t.Fatal(err)
	}
	var firstYearTerms []academic.Semester
	for _, term := range terms {
		if term.SchoolYearID == year1.ID {
			firstYearTerms = append(firstYearTerms, term)
		}
	}
	if len(firstYearTerms) != 2 || firstYearTerms[0].Number != 1 || firstYearTerms[1].Number != 2 {
		t.Fatal("tahun ajaran tidak memiliki tepat dua semester berurutan")
	}
	if _, err = s.CreateYear(ctx, curriculum.ID, year1.Name); !errors.Is(err, academic.ErrConflict) {
		t.Fatal("nama tahun ajaran duplikat diterima")
	}
	if err = s.ActivateSemester(ctx, curriculum.ID, firstYearTerms[0].ID); err == nil {
		t.Fatal("semester dapat aktif sebelum tahun ajaran")
	}
	if err = s.ActivateYear(ctx, curriculum.ID, year1.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateSemester(ctx, curriculum.ID, firstYearTerms[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateYear(ctx, admin.ID, year1.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Admin dapat mengaktifkan tahun")
	}
	if err = s.ActivateSemester(ctx, admin.ID, firstYearTerms[1].ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Admin dapat mengaktifkan semester")
	}
	year2, err := s.CreateYear(ctx, curriculum.ID, "Tahun "+suffix+" B")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateYear(ctx, curriculum.ID, year2.ID); err != nil {
		t.Fatal(err)
	}
	var activeYears, activeSemesters int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM app.school_years WHERE active`).Scan(&activeYears); err != nil || activeYears != 1 {
		t.Fatal("jumlah tahun ajaran aktif bukan satu")
	}
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM app.semesters WHERE active`).Scan(&activeSemesters); err != nil || activeSemesters != 0 {
		t.Fatal("semester tahun ajaran lama tidak dinonaktifkan saat tahun berganti")
	}
	terms, err = s.Semesters(ctx, curriculum.ID)
	if err != nil {
		t.Fatal(err)
	}
	var secondYearTerms []academic.Semester
	for _, term := range terms {
		if term.SchoolYearID == year2.ID {
			secondYearTerms = append(secondYearTerms, term)
		}
	}
	if err = s.ActivateSemester(ctx, curriculum.ID, secondYearTerms[1].ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateSemester(ctx, curriculum.ID, firstYearTerms[1].ID); err == nil {
		t.Fatal("semester pada tahun tidak aktif dapat dipilih")
	}
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM app.semesters WHERE active`).Scan(&activeSemesters); err != nil || activeSemesters != 1 {
		t.Fatal("jumlah semester aktif bukan satu")
	}

	class, err := s.SaveClass(ctx, admin.ID, 0, year2.ID, 7, "A", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveClass(ctx, admin.ID, 0, year2.ID, 7, "a", "", 0); !errors.Is(err, academic.ErrConflict) {
		t.Fatal("nama kelas yang sama dengan kapitalisasi berbeda diterima pada tingkat/tahun yang sama")
	}
	if _, err = s.SaveClass(ctx, admin.ID, 0, year2.ID, 8, "A", "", 0); err != nil {
		t.Fatal("nama kelas yang sama ditolak pada tingkat berbeda")
	}
	oldVersion := class.Version
	class, err = s.SaveClass(ctx, admin.ID, class.ID, year2.ID, 7, "B", "Koreksi nama rombongan", class.Version)
	if err != nil || class.ID < 1 || class.Version != oldVersion+1 {
		t.Fatal("koreksi kelas gagal atau mengganti ID permanen")
	}
	if _, err = s.SaveClass(ctx, admin.ID, class.ID, year2.ID, 7, "C", "Stale", oldVersion); !errors.Is(err, academic.ErrConflict) {
		t.Fatal("versi kelas kedaluwarsa diterima")
	}
	if _, err = s.SaveClass(ctx, curriculum.ID, 0, year2.ID, 7, "Z", "", 0); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Kurikulum tanpa role Admin dapat menulis kelas")
	}
	if _, err = s.SaveClass(ctx, curriculum.ID, class.ID, year2.ID, 7, "Z", "Tanpa hak Admin", class.Version); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Kurikulum tanpa role Admin dapat mengoreksi kelas")
	}
	for _, actor := range []int64{admin.ID, curriculum.ID} {
		classes, years, err := s.Classes(ctx, actor)
		if err != nil || len(classes) == 0 || len(years) == 0 {
			t.Fatal("Admin/Kurikulum tidak dapat membaca daftar kelas dan tahun:", err)
		}
		read, err := s.Class(ctx, actor, class.ID)
		if err != nil || read.ID != class.ID || read.Name != "B" {
			t.Fatal("kelas tidak terbaca atau berubah setelah mutasi terlarang:", err)
		}
		if _, err = s.History(ctx, actor, "class", class.ID); err != nil {
			t.Fatal("histori kelas tidak terbaca:", err)
		}
	}
	if _, err = s.History(ctx, admin.ID, "school_year", year2.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("hak histori kelas membuka histori kalender untuk Admin")
	}
	if _, err = s.History(ctx, admin.ID, "semester", secondYearTerms[1].ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("hak histori kelas membuka histori semester untuk Admin")
	}
	history, err := s.History(ctx, curriculum.ID, "class", class.ID)
	if err != nil || len(history) != 2 || history[0].Reason != "Koreksi nama rombongan" {
		t.Fatal("histori kelas tidak tercatat atomik")
	}
}

func TestClassRoleChangesAndLegacyHistoryIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	s := academic.Store{DB: db}
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	multi, _ := testutil.User(t, db, "admin", "kurikulum")
	teacher, _ := testutil.User(t, db, "guru_mapel")
	ctx := context.Background()
	year, err := s.CreateYear(ctx, curriculum.ID, "Akses "+testutil.Random(t)[:12])
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a class made under the previous policy, retaining its original actor.
	var legacyID int64
	if err = db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,'Lama') RETURNING id`, year.ID).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO app.academic_history(entity_type,entity_id,actor_id,after_state,reason) VALUES('class',$1,$2,'{"Name":"Lama"}','Pembuatan sebelum revisi kewenangan')`, legacyID, curriculum.ID); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.Class(ctx, admin.ID, legacyID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.SaveClass(ctx, admin.ID, legacy.ID, year.ID, 7, "Baru", "Koreksi oleh Admin", legacy.Version)
	if err != nil || updated.ID != legacyID {
		t.Fatal("Admin tidak dapat melanjutkan kelas lama dengan ID tetap:", err)
	}
	var originalActor int64
	if err = db.QueryRow(`SELECT actor_id FROM app.academic_history WHERE entity_type='class' AND entity_id=$1 ORDER BY id LIMIT 1`, legacyID).Scan(&originalActor); err != nil || originalActor != curriculum.ID {
		t.Fatal("pelaku histori kelas lama berubah")
	}
	history, err := s.History(ctx, admin.ID, "class", legacyID)
	if err != nil || len(history) != 2 || history[0].Reason != "Koreksi oleh Admin" {
		t.Fatal("histori lama tidak dipertahankan:", err)
	}
	if _, err = s.SaveClass(ctx, multi.ID, 0, year.ID, 8, "Multi", "", 0); err != nil {
		t.Fatal("multi-role tidak dapat mengelola kelas:", err)
	}
	if _, err = s.CreateYear(ctx, multi.ID, "Multi "+testutil.Random(t)[:12]); err != nil {
		t.Fatal("multi-role tidak dapat mengelola kalender:", err)
	}
	for _, actor := range []int64{teacher.ID, 0} {
		if _, _, err = s.Classes(ctx, actor); !errors.Is(err, academic.ErrForbidden) {
			t.Fatal("kelas terbuka bagi pengguna tanpa role")
		}
		if _, err = s.Class(ctx, actor, legacyID); !errors.Is(err, academic.ErrForbidden) {
			t.Fatal("detail kelas terbuka bagi pengguna tanpa role")
		}
		if _, err = s.History(ctx, actor, "class", legacyID); !errors.Is(err, academic.ErrForbidden) {
			t.Fatal("histori kelas terbuka bagi pengguna tanpa role")
		}
		if _, err = s.SaveClass(ctx, actor, 0, year.ID, 7, "Terlarang", "", 0); !errors.Is(err, academic.ErrForbidden) {
			t.Fatal("kelas dapat ditulis pengguna tanpa role")
		}
	}
	if _, err = db.Exec(`DELETE FROM app.user_roles WHERE user_id=$1 AND role_code='admin'`, multi.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveClass(ctx, multi.ID, 0, year.ID, 8, "Dicabut", "", 0); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("pencabutan Admin tidak menutup hak tulis kelas")
	}
	if _, _, err = s.Classes(ctx, multi.ID); err != nil {
		t.Fatal("role Kurikulum yang tersisa kehilangan hak baca:", err)
	}
	if _, err = db.Exec(`UPDATE app.users SET active=false WHERE id=$1`, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveClass(ctx, admin.ID, 0, year.ID, 9, "Nonaktif", "", 0); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Admin nonaktif masih dapat menulis kelas")
	}
	if _, _, err = s.Classes(ctx, admin.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatal("Admin nonaktif masih dapat membaca kelas")
	}
}
