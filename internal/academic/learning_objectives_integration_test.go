//go:build integration

package academic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestLearningObjectivesAreScopedToOfficialTeacherAssignmentIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacherUser, _ := testutil.User(t, db, "guru_mapel")
	otherTeacherUser, _ := testutil.User(t, db, "guru_mapel")
	random := testutil.Random(t)[:10]
	var yearID, otherYearID, semesterID, classID, otherClassID, mismatchedClassID int64
	var futureClassID, expiredClassID, inactiveClassID int64
	var subjectID, inactiveSubjectID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "TP Uji "+random).Scan(&yearID); err != nil {
		t.Fatal("tahun sintetis", err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal("semester sintetis", err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,'A') RETURNING id`, yearID).Scan(&classID); err != nil {
		t.Fatal("kelas sintetis", err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,'B') RETURNING id`, yearID).Scan(&otherClassID); err != nil {
		t.Fatal("kelas pembanding", err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,'C') RETURNING id`, yearID).Scan(&futureClassID); err != nil {
		t.Fatal("kelas penugasan masa depan", err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,'D') RETURNING id`, yearID).Scan(&expiredClassID); err != nil {
		t.Fatal("kelas penugasan kedaluwarsa", err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,'E') RETURNING id`, yearID).Scan(&inactiveClassID); err != nil {
		t.Fatal("kelas mapel nonaktif", err)
	}
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "TP mismatch "+random).Scan(&otherYearID); err != nil {
		t.Fatal("tahun pembanding", err)
	}
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,'A') RETURNING id`, otherYearID).Scan(&mismatchedClassID); err != nil {
		t.Fatal("kelas pembanding tahun", err)
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "TP-"+random, "Mapel TP "+random).Scan(&subjectID); err != nil {
		t.Fatal("mapel sintetis", err)
	}
	if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "TP-INACTIVE-"+random, "Mapel TP nonaktif "+random).Scan(&inactiveSubjectID); err != nil {
		t.Fatal("mapel nonaktif sintetis", err)
	}
	masters := &master.Store{DB: db}
	teacher, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru TP " + random, UserID: teacherUser.ID, Active: true}})
	if err != nil {
		t.Fatal("identitas guru sintetis", err)
	}
	otherTeacher, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru pembanding TP " + random, UserID: otherTeacherUser.ID, Active: true}})
	if err != nil {
		t.Fatal("identitas guru pembanding", err)
	}
	store := &Store{DB: db}
	effectiveFrom := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -30)
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classID, subjectID, teacher.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan resmi pertama", err)
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, otherClassID, subjectID, otherTeacher.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan resmi pembanding", err)
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, futureClassID, subjectID, teacher.ID, time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, 30)); err != nil {
		t.Fatal("penugasan masa depan", err)
	}
	todayWIB := time.Now().In(time.FixedZone("WIB", 7*60*60))
	expiredFrom := todayWIB.AddDate(0, 0, -30)
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, expiredClassID, subjectID, teacher.ID, expiredFrom); err != nil {
		t.Fatal("penugasan yang akan kedaluwarsa", err)
	}
	var expiredAssignmentID int64
	if err = db.QueryRow(`SELECT id FROM app.teaching_assignments WHERE semester_id=$1 AND class_id=$2 AND subject_id=$3`, semesterID, expiredClassID, subjectID).Scan(&expiredAssignmentID); err != nil {
		t.Fatal("baca penugasan untuk fixture tanggal", err)
	}
	expiredUntil := todayWIB.AddDate(0, 0, -1)
	if err = store.CorrectTeachingAssignment(ctx, curriculum.ID, expiredAssignmentID, semesterID, expiredClassID, subjectID, teacher.ID, expiredFrom, &expiredUntil); err != nil {
		t.Fatal("tetapkan rentang penugasan fixture yang telah berakhir", err)
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, inactiveClassID, inactiveSubjectID, teacher.ID, effectiveFrom); err != nil {
		t.Fatal("penugasan mapel yang nantinya dinonaktifkan", err)
	}
	inactiveSubject, err := masters.Get(ctx, curriculum.ID, master.Subjects, inactiveSubjectID)
	if err != nil {
		t.Fatal("baca mapel untuk menyiapkan status nonaktif", err)
	}
	inactiveSubject.Active = false
	if _, err = masters.Save(ctx, curriculum.ID, master.Subjects, inactiveSubjectID, master.Input{Record: inactiveSubject, Reason: "Fixture uji regression mapel nonaktif"}); err != nil {
		t.Fatal("nonaktifkan mapel melalui service master", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id=$1)`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id=$1`, semesterID)
		for _, teacherID := range []int64{teacher.ID, otherTeacher.ID} {
			_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, teacherID)
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID)
		}
		_, _ = db.Exec(`DELETE FROM app.subject_history WHERE entity_id=$1`, inactiveSubjectID)
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code IN ($1,$2)`, "TP-"+random, "TP-INACTIVE-"+random)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id IN ($1,$2)`, subjectID, inactiveSubjectID)
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id=$1`, semesterID)
		_, _ = db.Exec(`DELETE FROM app.classes WHERE id IN ($1,$2,$3,$4,$5,$6)`, classID, otherClassID, futureClassID, expiredClassID, inactiveClassID, mismatchedClassID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, otherYearID)
		for _, userID := range []int64{admin.ID, curriculum.ID, teacherUser.ID, otherTeacherUser.ID} {
			for _, q := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, cleanupErr := db.Exec(q, userID); cleanupErr != nil {
					t.Errorf("cleanup akun: %v", cleanupErr)
					return
				}
			}
		}
	})

	contexts, err := store.LearningObjectiveContexts(ctx, teacherUser.ID)
	if err != nil || len(contexts) != 1 || contexts[0].ClassID != classID || contexts[0].SubjectID != subjectID || contexts[0].SemesterID != semesterID {
		t.Fatalf("konteks resmi guru salah: %#v err=%v", contexts, err)
	}
	if err = store.CreateLearningObjective(ctx, teacherUser.ID, semesterID, classID, subjectID, "  Siswa menjelaskan konsep dasar secara runtut.  "); err != nil {
		t.Fatal("TP pada penugasan sah gagal disimpan", err)
	}
	objectives, err := store.LearningObjectives(ctx, teacherUser.ID, semesterID, classID, subjectID)
	if err != nil || len(objectives) != 1 || objectives[0].Definition != "Siswa menjelaskan konsep dasar secara runtut." || objectives[0].CreatedBy != teacherUser.ID || objectives[0].CreatedAt == "" {
		t.Fatalf("TP tidak terbaca dengan konteks/pencatat yang benar: %#v err=%v", objectives, err)
	}
	otherContext, err := store.LearningObjectives(ctx, teacherUser.ID, semesterID, otherClassID, subjectID)
	if !errors.Is(err, ErrForbidden) || len(otherContext) != 0 {
		t.Fatalf("guru tidak boleh membaca kelas/mapel guru lain: %#v err=%v", otherContext, err)
	}
	if err = store.CreateLearningObjective(ctx, teacherUser.ID, semesterID, otherClassID, subjectID, "Tidak sah"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("POST/service harus menolak konteks penugasan guru lain: %v", err)
	}
	if err = store.CreateLearningObjective(ctx, curriculum.ID, semesterID, classID, subjectID, "Tidak sah"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Kurikulum tidak otomatis mendapat akses mutasi TP Guru Mapel: %v", err)
	}
	contexts, err = store.LearningObjectiveContexts(ctx, teacherUser.ID)
	if err != nil || len(contexts) != 1 || contexts[0].ClassID != classID {
		t.Fatalf("daftar konteks harus mengecualikan penugasan masa depan, kedaluwarsa, dan mapel nonaktif: %#v err=%v", contexts, err)
	}
	for _, denied := range []struct {
		name      string
		classID   int64
		subjectID int64
	}{
		{name: "penugasan masa depan", classID: futureClassID, subjectID: subjectID},
		{name: "penugasan kedaluwarsa", classID: expiredClassID, subjectID: subjectID},
		{name: "mata pelajaran nonaktif", classID: inactiveClassID, subjectID: inactiveSubjectID},
	} {
		t.Run(denied.name, func(t *testing.T) {
			objectives, readErr := store.LearningObjectives(ctx, teacherUser.ID, semesterID, denied.classID, denied.subjectID)
			if !errors.Is(readErr, ErrForbidden) || len(objectives) != 0 {
				t.Fatalf("pembacaan harus ditolak tanpa membocorkan TP: %#v err=%v", objectives, readErr)
			}
			if createErr := store.CreateLearningObjective(ctx, teacherUser.ID, semesterID, denied.classID, denied.subjectID, "Tidak boleh dibuat"); !errors.Is(createErr, ErrForbidden) {
				t.Fatalf("pembuatan TP harus ditolak: %v", createErr)
			}
		})
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM app.learning_objectives WHERE school_year_id=$1`, yearID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("percobaan tak sah tidak boleh membuat data: count=%d err=%v", count, err)
	}
	if _, err = db.Exec(`INSERT INTO app.learning_objectives(school_year_id,semester_id,class_id,subject_id,definition,created_by) VALUES($1,$2,$3,$4,'Konteks tidak cocok',$5)`, yearID, semesterID, mismatchedClassID, subjectID, teacherUser.ID); err == nil {
		t.Fatal("FK harus menolak semester dan kelas yang berasal dari tahun berbeda")
	}
}
