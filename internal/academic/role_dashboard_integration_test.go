//go:build integration

package academic_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/studentcases"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestRoleDashboardsReadOwnOfficialSourcesAndRejectOtherRoles(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	store := academic.Store{DB: db}
	ctx := context.Background()
	teacher, _ := testutil.User(t, db, "guru_mapel", "wali_kelas")
	otherRole, _ := testutil.User(t, db, "kesiswaan")
	noProfile, _ := testutil.User(t, db, "guru_mapel")
	var teacherID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Dashboard fixture "+testutil.Random(t)[:10], teacher.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}

	teacherStats, err := store.TeacherDashboard(ctx, teacher.ID)
	if err != nil {
		t.Fatal(err)
	}
	if teacherStats.Date != time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02") {
		t.Fatalf("tanggal dashboard harus tanggal WIB, got %s", teacherStats.Date)
	}
	if teacherStats.HasCheckIn || teacherStats.HasCheckOut || teacherStats.ActiveClasses != 0 {
		t.Fatalf("dashboard fixture tanpa penugasan/presensi harus kosong, got %+v", teacherStats)
	}
	if _, err = store.TeacherDashboard(ctx, noProfile.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("akun Guru Mapel tanpa identitas guru harus ditolak: %v", err)
	}
	if _, err = store.TeacherDashboard(ctx, otherRole.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("role lain tidak boleh membaca statistik Guru Mapel: %v", err)
	}

	homeroomStats, err := store.HomeroomDashboard(ctx, teacher.ID)
	if err != nil {
		t.Fatal(err)
	}
	if homeroomStats.HasHomeroom || homeroomStats.Classes != 0 || homeroomStats.HasAttendance || homeroomStats.OpenCases != 0 {
		t.Fatalf("akun tanpa penugasan Wali Kelas harus menerima kondisi kosong, got %+v", homeroomStats)
	}
	if _, err = store.HomeroomDashboard(ctx, otherRole.ID); !errors.Is(err, academic.ErrForbidden) {
		t.Fatalf("role Kesiswaan tidak boleh membaca statistik Wali Kelas: %v", err)
	}
}

func TestRoleDashboardsFollowCurrentSemesterAndHomeroomAssignment(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	store := academic.Store{DB: db}
	ctx := context.Background()
	teacher, _ := testutil.User(t, db, "guru_mapel", "wali_kelas")
	newHomeroom, _ := testutil.User(t, db, "wali_kelas")
	var teacherID, newHomeroomTeacherID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Dashboard teacher "+testutil.Random(t)[:10], teacher.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Dashboard wali "+testutil.Random(t)[:10], newHomeroom.ID).Scan(&newHomeroomTeacherID); err != nil {
		t.Fatal(err)
	}

	var oldYear, oldSemester sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT id FROM app.school_years WHERE active`).Scan(&oldYear); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id FROM app.semesters WHERE active`).Scan(&oldSemester); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	var yearID, semesterID, classID, subjectID, assignmentID, revisionID, studentID, placementID, caseID int64
	t.Cleanup(func() {
		if studentID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.student_daily_attendance WHERE student_id=$1`, studentID)
		}
		if caseID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.student_case_history WHERE case_id=$1`, caseID)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.student_case_entries WHERE case_id=$1`, caseID)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.student_case_counseling_notes WHERE case_id=$1`, caseID)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.student_cases WHERE id=$1`, caseID)
		}
		if placementID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.student_placements WHERE id=$1`, placementID)
		}
		if studentID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.students WHERE id=$1`, studentID)
		}
		if semesterID > 0 {
			_, _ = db.ExecContext(context.Background(), `UPDATE app.semesters SET active=false WHERE id=$1`, semesterID)
		}
		if yearID > 0 {
			_, _ = db.ExecContext(context.Background(), `UPDATE app.school_years SET active=false WHERE id=$1`, yearID)
		}
		if classID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.homeroom_assignments WHERE class_id=$1`, classID)
		}
		if revisionID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.weekly_timetable_entries WHERE revision_id=$1`, revisionID)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.weekly_timetable_history WHERE revision_id=$1`, revisionID)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.weekly_timetable_revisions WHERE id=$1`, revisionID)
		}
		if assignmentID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.teaching_assignments WHERE id=$1`, assignmentID)
		}
		if classID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.classes WHERE id=$1`, classID)
		}
		if semesterID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.semester_calendars WHERE semester_id=$1`, semesterID)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.semesters WHERE id=$1`, semesterID)
		}
		if subjectID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.subjects WHERE id=$1`, subjectID)
		}
		if yearID > 0 {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM app.school_years WHERE id=$1`, yearID)
		}
		if oldYear.Valid {
			_, _ = db.ExecContext(context.Background(), `UPDATE app.school_years SET active=true WHERE id=$1`, oldYear.Int64)
		}
		if oldSemester.Valid {
			_, _ = db.ExecContext(context.Background(), `UPDATE app.semesters SET active=true WHERE id=$1`, oldSemester.Int64)
		}
	})
	if _, err := db.ExecContext(ctx, `UPDATE app.semesters SET active=false WHERE active`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE app.school_years SET active=false WHERE active`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.school_years(name,active) VALUES($1,true) RETURNING id`, "Role dashboard "+testutil.Random(t)[:12]).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.semesters(school_year_id,number,active) VALUES($1,1,true) RETURNING id`, yearID).Scan(&semesterID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "A"+testutil.Random(t)[:6]).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.subjects(code,name) VALUES($1,$1) RETURNING id`, "RD"+testutil.Random(t)[:8]).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.teaching_assignments(semester_id,class_id,subject_id,teacher_id,effective_from,assigned_by) VALUES($1,$2,$3,$4,'1900-01-01',$5) RETURNING id`, semesterID, classID, subjectID, teacherID, teacher.ID).Scan(&assignmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO app.semester_calendars(semester_id,starts_on,ends_on,school_days) VALUES($1,'1900-01-01','2999-01-01','[1,2,3,4,5,6,7]'::jsonb)`, semesterID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.weekly_timetable_revisions(semester_id,revision_no,status,effective_from,created_by,published_by,published_at)
		VALUES($1,1,'published','1900-01-01',$2,$2,clock_timestamp()) RETURNING id`, semesterID, teacher.ID).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	var todayWeekday int
	if err := db.QueryRowContext(ctx, `SELECT extract(isodow FROM (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date)::int`).Scan(&todayWeekday); err != nil {
		t.Fatal(err)
	}
	for _, weekday := range []int{todayWeekday, todayWeekday%7 + 1} {
		if _, err := db.ExecContext(ctx, `INSERT INTO app.weekly_timetable_entries(revision_id,weekday,class_id,subject_id,teacher_id,assignment_id,period_start,period_end) VALUES($1,$2,$3,$4,$5,$6,1,1)`, revisionID, weekday, classID, subjectID, teacherID, assignmentID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3)`, classID, teacherID, teacher.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-03-04','P') RETURNING id`, "Dashboard student "+testutil.Random(t)[:8]).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,(now() AT TIME ZONE 'Asia/Jakarta')::date-1) RETURNING id`, yearID, studentID, classID, teacher.ID).Scan(&placementID); err != nil {
		t.Fatal(err)
	}
	createdCase, err := store.CreateStudentCase(ctx, teacher.ID, studentcases.CreateInput{StudentID: studentID, Content: "Ringkasan pembinaan fixture dashboard"})
	if err != nil {
		t.Fatal(err)
	}
	caseID = createdCase.ID

	stats, err := store.TeacherDashboard(ctx, teacher.ID)
	if err != nil || !stats.HasActiveSemester || stats.ActiveClasses != 1 || !stats.HasPublishedTimetable || stats.TodayMeetings != 1 || stats.WeeklyMeetings != 2 {
		t.Fatalf("penugasan dan jadwal terbit harus dihitung menurut tanggal WIB: stats=%+v err=%v", stats, err)
	}
	waliStats, err := store.HomeroomDashboard(ctx, teacher.ID)
	if err != nil || !waliStats.HasHomeroom || waliStats.Classes != 1 || waliStats.HasAttendance || waliStats.Students != 1 || waliStats.OpenCases != 1 {
		t.Fatalf("Wali Kelas aktif harus mendapat ringkasan kelas tanpa membuat data absensi: stats=%+v err=%v", waliStats, err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO app.student_daily_attendance(student_id,placement_id,class_id,semester_id,on_date,student_name,nis,class_name,status,source,actor_id)
		VALUES($1,$2,$3,$4,(now() AT TIME ZONE 'Asia/Jakarta')::date,'Dashboard student','', '7A','hadir','manual',$5)`, studentID, placementID, classID, semesterID, teacher.ID); err != nil {
		t.Fatal(err)
	}
	waliStats, err = store.HomeroomDashboard(ctx, teacher.ID)
	if err != nil || !waliStats.HasAttendance || waliStats.AttendanceRecorded != 1 || waliStats.Present != 1 {
		t.Fatalf("ringkasan Wali Kelas harus mengikuti bukti absensi hari ini: stats=%+v err=%v", waliStats, err)
	}
	if transferred, err := store.HomeroomDashboard(ctx, newHomeroom.ID); err != nil || transferred.HasHomeroom {
		t.Fatalf("Wali Kelas yang belum ditugaskan tidak boleh membaca kelas: stats=%+v err=%v", transferred, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE app.homeroom_assignments SET teacher_id=$1 WHERE class_id=$2`, newHomeroomTeacherID, classID); err != nil {
		t.Fatal(err)
	}
	oldStats, err := store.HomeroomDashboard(ctx, teacher.ID)
	if err != nil || oldStats.HasHomeroom || oldStats.Classes != 0 || oldStats.OpenCases != 0 {
		t.Fatalf("akses dashboard mantan Wali Kelas harus mengikuti penugasan terkini: stats=%+v err=%v", oldStats, err)
	}
	newStats, err := store.HomeroomDashboard(ctx, newHomeroom.ID)
	if err != nil || !newStats.HasHomeroom || newStats.Classes != 1 || newStats.OpenCases != 1 {
		t.Fatalf("dashboard Wali Kelas baru harus mengikuti penugasan terkini: stats=%+v err=%v", newStats, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE app.teaching_assignments SET effective_until='1900-01-02' WHERE id=$1`, assignmentID); err != nil {
		t.Fatal(err)
	}
	stats, err = store.TeacherDashboard(ctx, teacher.ID)
	if err != nil || stats.ActiveClasses != 0 {
		t.Fatalf("dashboard Guru Mapel harus berhenti menghitung penugasan tidak aktif: stats=%+v err=%v", stats, err)
	}
}
