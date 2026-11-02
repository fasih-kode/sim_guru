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

func TestCopyLearningObjectiveUsesOnlyDefinitionAndChecksBothAssignmentsIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	admin, _ := testutil.User(t, db, "admin")
	curriculum, _ := testutil.User(t, db, "kurikulum")
	teacherUser, _ := testutil.User(t, db, "guru_mapel")
	otherTeacherUser, _ := testutil.User(t, db, "guru_mapel")
	suffix := testutil.Random(t)[:10]
	var yearID, semesterID, otherSemesterID int64
	if err := db.QueryRow(`INSERT INTO app.school_years(name) VALUES($1) RETURNING id`, "Salin TP "+suffix).Scan(&yearID); err != nil {
		t.Fatal("tahun ajaran sintetis", err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,1) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal("semester sintetis", err)
	}
	if err := db.QueryRow(`INSERT INTO app.semesters(school_year_id,number) VALUES($1,2) RETURNING id`, yearID).Scan(&otherSemesterID); err != nil {
		t.Fatal("semester kedua sintetis", err)
	}
	classIDs := map[string]int64{}
	for _, name := range []string{"Sumber", "Tujuan", "MilikLain", "MasaDepan", "Kedaluwarsa", "MapelNonaktif", "BedaMapel", "BedaSemester"} {
		var classID int64
		if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, name+suffix).Scan(&classID); err != nil {
			t.Fatalf("kelas %s: %v", name, err)
		}
		classIDs[name] = classID
	}
	subjects := map[string]int64{}
	for _, name := range []string{"Aktif", "Nonaktif", "Lain"} {
		var subjectID int64
		if err := db.QueryRow(`INSERT INTO app.subjects(code,name) VALUES($1,$2) RETURNING id`, "COPY-"+name+"-"+suffix, "Mapel Salin "+name+" "+suffix).Scan(&subjectID); err != nil {
			t.Fatalf("mata pelajaran %s: %v", name, err)
		}
		subjects[name] = subjectID
	}
	masters := &master.Store{DB: db}
	ownerIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru sumber TP " + suffix, UserID: teacherUser.ID, Active: true}})
	if err != nil {
		t.Fatal("identitas guru sumber", err)
	}
	otherIdentity, err := masters.Save(ctx, admin.ID, master.Teachers, 0, master.Input{Record: master.Record{Name: "Guru pemilik lain TP " + suffix, UserID: otherTeacherUser.ID, Active: true}})
	if err != nil {
		t.Fatal("identitas guru pembanding", err)
	}
	store := &Store{DB: db}
	today := time.Now().In(time.FixedZone("WIB", 7*60*60))
	from := today.AddDate(0, 0, -30)
	for _, className := range []string{"Sumber", "Tujuan", "MasaDepan", "Kedaluwarsa"} {
		start := from
		if className == "MasaDepan" {
			start = today.AddDate(0, 0, 30)
		}
		if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classIDs[className], subjects["Aktif"], ownerIdentity.ID, start); err != nil {
			t.Fatalf("penugasan %s: %v", className, err)
		}
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classIDs["MilikLain"], subjects["Aktif"], otherIdentity.ID, from); err != nil {
		t.Fatal("penugasan guru pembanding", err)
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classIDs["MapelNonaktif"], subjects["Nonaktif"], ownerIdentity.ID, from); err != nil {
		t.Fatal("penugasan mapel sebelum dinonaktifkan", err)
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, semesterID, classIDs["BedaMapel"], subjects["Lain"], ownerIdentity.ID, from); err != nil {
		t.Fatal("penugasan beda mata pelajaran", err)
	}
	if err = store.AssignTeacher(ctx, curriculum.ID, otherSemesterID, classIDs["BedaSemester"], subjects["Aktif"], ownerIdentity.ID, from); err != nil {
		t.Fatal("penugasan beda semester", err)
	}
	var expiredAssignmentID int64
	if err = db.QueryRow(`SELECT id FROM app.teaching_assignments WHERE semester_id=$1 AND class_id=$2`, semesterID, classIDs["Kedaluwarsa"]).Scan(&expiredAssignmentID); err != nil {
		t.Fatal("baca assignment kedaluwarsa", err)
	}
	expiredUntil := today.AddDate(0, 0, -1)
	if err = store.CorrectTeachingAssignment(ctx, curriculum.ID, expiredAssignmentID, semesterID, classIDs["Kedaluwarsa"], subjects["Aktif"], ownerIdentity.ID, from, &expiredUntil); err != nil {
		t.Fatal("akhiri penugasan fixture", err)
	}
	inactiveSubject, err := masters.Get(ctx, curriculum.ID, master.Subjects, subjects["Nonaktif"])
	if err != nil {
		t.Fatal("baca mapel fixture", err)
	}
	inactiveSubject.Active = false
	if _, err = masters.Save(ctx, curriculum.ID, master.Subjects, subjects["Nonaktif"], master.Input{Record: inactiveSubject, Reason: "Fixture uji salin TP mapel nonaktif"}); err != nil {
		t.Fatal("nonaktifkan mapel fixture", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app.learning_objectives WHERE school_year_id=$1`, yearID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignment_history WHERE assignment_id IN (SELECT id FROM app.teaching_assignments WHERE semester_id IN ($1,$2))`, semesterID, otherSemesterID)
		_, _ = db.Exec(`DELETE FROM app.teaching_assignments WHERE semester_id IN ($1,$2)`, semesterID, otherSemesterID)
		for _, teacherID := range []int64{ownerIdentity.ID, otherIdentity.ID} {
			_, _ = db.Exec(`DELETE FROM app.teacher_history WHERE entity_id=$1`, teacherID)
			_, _ = db.Exec(`DELETE FROM app.teachers WHERE id=$1`, teacherID)
		}
		_, _ = db.Exec(`DELETE FROM app.subject_history WHERE entity_id IN ($1,$2)`, subjects["Nonaktif"], subjects["Lain"])
		_, _ = db.Exec(`DELETE FROM app.subject_codes WHERE code IN ($1,$2,$3)`, "COPY-Aktif-"+suffix, "COPY-Nonaktif-"+suffix, "COPY-Lain-"+suffix)
		_, _ = db.Exec(`DELETE FROM app.subjects WHERE id IN ($1,$2,$3)`, subjects["Aktif"], subjects["Nonaktif"], subjects["Lain"])
		_, _ = db.Exec(`DELETE FROM app.semesters WHERE id IN ($1,$2)`, semesterID, otherSemesterID)
		for _, classID := range classIDs {
			_, _ = db.Exec(`DELETE FROM app.classes WHERE id=$1`, classID)
		}
		_, _ = db.Exec(`DELETE FROM app.school_years WHERE id=$1`, yearID)
		for _, userID := range []int64{admin.ID, curriculum.ID, teacherUser.ID, otherTeacherUser.ID} {
			for _, query := range []string{`DELETE FROM app.sessions WHERE user_id=$1`, `DELETE FROM app.access_audit WHERE actor_id=$1 OR target_id=$1`, `DELETE FROM app.user_roles WHERE user_id=$1`, `DELETE FROM app.users WHERE id=$1`} {
				if _, cleanupErr := db.Exec(query, userID); cleanupErr != nil {
					t.Errorf("cleanup akun fixture: %v", cleanupErr)
					return
				}
			}
		}
	})

	if err = store.CreateLearningObjective(ctx, teacherUser.ID, semesterID, classIDs["Sumber"], subjects["Aktif"], "Definisi sumber yang disalin"); err != nil {
		t.Fatal("buat TP sumber", err)
	}
	if err = store.CreateLearningObjective(ctx, otherTeacherUser.ID, semesterID, classIDs["MilikLain"], subjects["Aktif"], "TP milik guru lain"); err != nil {
		t.Fatal("buat TP sumber pembanding", err)
	}
	sourceRows, err := store.LearningObjectives(ctx, teacherUser.ID, semesterID, classIDs["Sumber"], subjects["Aktif"])
	if err != nil || len(sourceRows) != 1 {
		t.Fatalf("baca TP sumber: rows=%v err=%v", sourceRows, err)
	}
	otherRows, err := store.LearningObjectives(ctx, otherTeacherUser.ID, semesterID, classIDs["MilikLain"], subjects["Aktif"])
	if err != nil || len(otherRows) != 1 {
		t.Fatalf("baca TP pembanding: rows=%v err=%v", otherRows, err)
	}
	copiedID, err := store.CopyLearningObjective(ctx, teacherUser.ID, sourceRows[0].ID, semesterID, classIDs["Sumber"], subjects["Aktif"], semesterID, classIDs["Tujuan"], subjects["Aktif"])
	if err != nil || copiedID == sourceRows[0].ID {
		t.Fatalf("penyalinan sah harus menghasilkan TP baru: id=%d err=%v", copiedID, err)
	}
	copiedRows, err := store.LearningObjectives(ctx, teacherUser.ID, semesterID, classIDs["Tujuan"], subjects["Aktif"])
	if err != nil || len(copiedRows) != 1 || copiedRows[0].ID != copiedID || copiedRows[0].Definition != sourceRows[0].Definition || copiedRows[0].CreatedBy != teacherUser.ID || copiedRows[0].CreatedAt == "" {
		t.Fatalf("TP hasil salin harus memiliki konteks, isi, dan metadata baru: rows=%#v err=%v", copiedRows, err)
	}
	var sourceCreatedAt, copyCreatedAt time.Time
	if err = db.QueryRow(`SELECT created_at FROM app.learning_objectives WHERE id=$1`, sourceRows[0].ID).Scan(&sourceCreatedAt); err != nil {
		t.Fatal("baca waktu pembuatan TP sumber", err)
	}
	if err = db.QueryRow(`SELECT created_at FROM app.learning_objectives WHERE id=$1`, copiedID).Scan(&copyCreatedAt); err != nil || !copyCreatedAt.After(sourceCreatedAt) {
		t.Fatalf("TP tujuan harus memiliki waktu pencatatan baru: source=%v copy=%v err=%v", sourceCreatedAt, copyCreatedAt, err)
	}
	if _, err = db.Exec(`UPDATE app.learning_objectives SET definition='Perubahan fixture pada sumber' WHERE id=$1`, sourceRows[0].ID); err != nil {
		t.Fatal("ubah sumber hanya untuk membuktikan independensi fixture", err)
	}
	copiedRows, err = store.LearningObjectives(ctx, teacherUser.ID, semesterID, classIDs["Tujuan"], subjects["Aktif"])
	if err != nil || len(copiedRows) != 1 || copiedRows[0].Definition != "Definisi sumber yang disalin" {
		t.Fatalf("perubahan sumber tidak boleh mengubah TP hasil salin: rows=%#v err=%v", copiedRows, err)
	}

	for _, denied := range []struct {
		name             string
		sourceID         int64
		sourceSemesterID int64
		sourceClassID    int64
		sourceSubjectID  int64
		targetSemesterID int64
		targetClassID    int64
		targetSubjectID  int64
		wantValidation   bool
	}{
		{name: "sumber guru lain", sourceID: otherRows[0].ID, sourceSemesterID: semesterID, sourceClassID: classIDs["MilikLain"], sourceSubjectID: subjects["Aktif"], targetSemesterID: semesterID, targetClassID: classIDs["Tujuan"], targetSubjectID: subjects["Aktif"]},
		{name: "ID sumber dari konteks lain yang juga dimiliki", sourceID: copiedID, sourceSemesterID: semesterID, sourceClassID: classIDs["Sumber"], sourceSubjectID: subjects["Aktif"], targetSemesterID: semesterID, targetClassID: classIDs["Tujuan"], targetSubjectID: subjects["Aktif"]},
		{name: "tujuan guru lain", sourceID: sourceRows[0].ID, sourceSemesterID: semesterID, sourceClassID: classIDs["Sumber"], sourceSubjectID: subjects["Aktif"], targetSemesterID: semesterID, targetClassID: classIDs["MilikLain"], targetSubjectID: subjects["Aktif"]},
		{name: "penugasan masa depan", sourceID: sourceRows[0].ID, sourceSemesterID: semesterID, sourceClassID: classIDs["Sumber"], sourceSubjectID: subjects["Aktif"], targetSemesterID: semesterID, targetClassID: classIDs["MasaDepan"], targetSubjectID: subjects["Aktif"]},
		{name: "penugasan kedaluwarsa", sourceID: sourceRows[0].ID, sourceSemesterID: semesterID, sourceClassID: classIDs["Sumber"], sourceSubjectID: subjects["Aktif"], targetSemesterID: semesterID, targetClassID: classIDs["Kedaluwarsa"], targetSubjectID: subjects["Aktif"]},
		{name: "mata pelajaran nonaktif", sourceID: sourceRows[0].ID, sourceSemesterID: semesterID, sourceClassID: classIDs["Sumber"], sourceSubjectID: subjects["Aktif"], targetSemesterID: semesterID, targetClassID: classIDs["MapelNonaktif"], targetSubjectID: subjects["Nonaktif"]},
		{name: "tujuan beda mata pelajaran", sourceID: sourceRows[0].ID, sourceSemesterID: semesterID, sourceClassID: classIDs["Sumber"], sourceSubjectID: subjects["Aktif"], targetSemesterID: semesterID, targetClassID: classIDs["BedaMapel"], targetSubjectID: subjects["Lain"], wantValidation: true},
		{name: "tujuan beda semester", sourceID: sourceRows[0].ID, sourceSemesterID: semesterID, sourceClassID: classIDs["Sumber"], sourceSubjectID: subjects["Aktif"], targetSemesterID: otherSemesterID, targetClassID: classIDs["BedaSemester"], targetSubjectID: subjects["Aktif"], wantValidation: true},
		{name: "kelas tujuan sama", sourceID: sourceRows[0].ID, sourceSemesterID: semesterID, sourceClassID: classIDs["Sumber"], sourceSubjectID: subjects["Aktif"], targetSemesterID: semesterID, targetClassID: classIDs["Sumber"], targetSubjectID: subjects["Aktif"], wantValidation: true},
	} {
		t.Run(denied.name, func(t *testing.T) {
			var before int
			if err := db.QueryRow(`SELECT count(*) FROM app.learning_objectives WHERE school_year_id=$1`, yearID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			_, copyErr := store.CopyLearningObjective(ctx, teacherUser.ID, denied.sourceID, denied.sourceSemesterID, denied.sourceClassID, denied.sourceSubjectID, denied.targetSemesterID, denied.targetClassID, denied.targetSubjectID)
			if denied.wantValidation {
				var validation ValidationError
				if !errors.As(copyErr, &validation) {
					t.Fatalf("konteks tujuan yang sama harus ditolak sebagai validasi: %v", copyErr)
				}
			} else if !errors.Is(copyErr, ErrForbidden) {
				t.Fatalf("akses sumber/tujuan tidak sah harus ditolak: %v", copyErr)
			}
			var after int
			if err := db.QueryRow(`SELECT count(*) FROM app.learning_objectives WHERE school_year_id=$1`, yearID).Scan(&after); err != nil || after != before {
				t.Fatalf("penolakan tidak boleh menulis data parsial: sebelum=%d sesudah=%d err=%v", before, after, err)
			}
		})
	}
}
