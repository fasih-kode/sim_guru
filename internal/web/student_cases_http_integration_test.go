//go:build integration

package web

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/studentcases"
	"github.com/fasih-kode/sim_guru/internal/testutil"
)

func TestStudentCaseDetailHTTPScopesPrivateNotes(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	store := &academic.Store{DB: db}
	wali, _ := testutil.User(t, db, "wali_kelas")
	bk, _ := testutil.User(t, db, "bk")
	otherBK, _ := testutil.User(t, db, "bk")
	head, _ := testutil.User(t, db, "kepala_sekolah")
	admin, _ := testutil.User(t, db, "admin")
	yearID, _ := testutil.ActiveCalendar(t, db, testutil.Random(t)[:12])
	var classID, teacherID, studentID int64
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "HTTP-UAT-"+testutil.Random(t)[:8]).Scan(&classID); err != nil {
		t.Fatal("create isolated class")
	}
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali HTTP Uji", wali.ID).Scan(&teacherID); err != nil {
		t.Fatal("create homeroom identity")
	}
	if _, err := db.Exec(`INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3)`, classID, teacherID, admin.ID); err != nil {
		t.Fatal("create homeroom assignment")
	}
	if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','P') RETURNING id`, "Siswa HTTP Uji "+testutil.Random(t)[:8]).Scan(&studentID); err != nil {
		t.Fatal("create synthetic student")
	}
	if _, err := db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,CURRENT_DATE-10)`, yearID, studentID, classID, admin.ID); err != nil {
		t.Fatal("create active placement")
	}
	item, err := store.CreateStudentCase(ctx, wali.ID, studentcases.CreateInput{StudentID: studentID, Content: "Ringkasan pembinaan uji"})
	if err != nil {
		t.Fatal("create case for HTTP access test")
	}
	if err = store.ReferStudentCaseToBK(ctx, wali.ID, item.ID, item.Version, studentcases.ReferralInput{TargetUserID: bk.ID, Reason: "Koordinasi lanjutan"}); err != nil {
		t.Fatal("refer to active BK")
	}
	assigned, err := store.GetStudentCase(ctx, bk.ID, item.ID)
	if err != nil {
		t.Fatal("load BK-owned case")
	}
	secret := "RAHASIA-KONSELING-HTTP-UJI"
	if err = store.AddStudentCaseCounselingNote(ctx, bk.ID, item.ID, assigned.Case.Version, studentcases.CounselingInput{Content: secret}); err != nil {
		t.Fatal("add private note")
	}

	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(p, i int) int { return (p-1)*10 + i + 1 }, "weekdayName": func(int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates, options: Options{Academics: store}}
	renderAs := func(user auth.User, want int, shouldShowSecret bool) {
		r := httptest.NewRequest("GET", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10), nil)
		r.SetPathValue("id", strconv.FormatInt(item.ID, 10))
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &user))
		w := httptest.NewRecorder()
		app.studentCaseDetail(w, r)
		body := w.Body.String()
		if w.Code != want || strings.Contains(body, secret) != shouldShowSecret {
			t.Fatalf("HTTP case scope mismatch for role %v: status=%d", user.Roles, w.Code)
		}
	}
	renderAs(auth.User{ID: wali.ID, Active: true, DisplayName: wali.DisplayName, Roles: []string{"wali_kelas"}}, 200, false)
	renderAs(auth.User{ID: head.ID, Active: true, DisplayName: head.DisplayName, Roles: []string{"kepala_sekolah"}}, 200, false)
	renderAs(auth.User{ID: bk.ID, Active: true, DisplayName: bk.DisplayName, Roles: []string{"bk"}}, 200, true)
	renderAs(auth.User{ID: otherBK.ID, Active: true, DisplayName: otherBK.DisplayName, Roles: []string{"bk"}}, 404, false)
	renderAs(auth.User{ID: admin.ID, Active: true, DisplayName: admin.DisplayName, Roles: []string{"admin"}}, 403, false)
}

func TestReferStudentCaseToKesiswaanRedirectsToScopedListIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	store := &academic.Store{DB: db}
	wali, _ := testutil.User(t, db, "wali_kelas")
	bk, _ := testutil.User(t, db, "bk")
	kes, _ := testutil.User(t, db, "kesiswaan")
	admin, _ := testutil.User(t, db, "admin")
	yearID, _ := testutil.ActiveCalendar(t, db, testutil.Random(t)[:12])
	var classID, teacherID, studentID int64
	if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, "CASE-FORWARD-"+testutil.Random(t)[:8]).Scan(&classID); err != nil {
		t.Fatal("create isolated class")
	}
	if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali Forward Uji", wali.ID).Scan(&teacherID); err != nil {
		t.Fatal("create homeroom identity")
	}
	if _, err := db.Exec(`INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3)`, classID, teacherID, admin.ID); err != nil {
		t.Fatal("create homeroom assignment")
	}
	if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','P') RETURNING id`, "Siswa Forward Uji "+testutil.Random(t)[:8]).Scan(&studentID); err != nil {
		t.Fatal("create synthetic student")
	}
	if _, err := db.Exec(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,CURRENT_DATE-10)`, yearID, studentID, classID, admin.ID); err != nil {
		t.Fatal("create active placement")
	}
	item, err := store.CreateStudentCase(ctx, wali.ID, studentcases.CreateInput{StudentID: studentID, Content: "Ringkasan pembinaan awal"})
	if err != nil {
		t.Fatal("create case")
	}
	if err = store.ReferStudentCaseToBK(ctx, wali.ID, item.ID, item.Version, studentcases.ReferralInput{TargetUserID: bk.ID, Reason: "Perlu konseling"}); err != nil {
		t.Fatal("refer case to BK")
	}
	owned, err := store.GetStudentCase(ctx, bk.ID, item.ID)
	if err != nil {
		t.Fatal("read BK-owned case")
	}
	const privateNote = "PRIVATE-CASE-FORWARD-REGRESSION"
	if err = store.AddStudentCaseCounselingNote(ctx, bk.ID, item.ID, owned.Case.Version, studentcases.CounselingInput{Content: privateNote}); err != nil {
		t.Fatal("add private note")
	}
	owned, err = store.GetStudentCase(ctx, bk.ID, item.ID)
	if err != nil {
		t.Fatal("reload BK-owned case")
	}
	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(p, i int) int { return (p-1)*10 + i + 1 }, "weekdayName": func(int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates, options: Options{Academics: store}}
	form := url.Values{"version": {strconv.FormatInt(owned.Case.Version, 10)}, "target_user_id": {strconv.FormatInt(kes.ID, 10)}, "reason": {"Ringkasan tindak lanjut untuk disiplin"}}
	request := func(hx bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10)+"/refer-kesiswaan", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if hx {
			r.Header.Set("HX-Request", "true")
		}
		r.SetPathValue("id", strconv.FormatInt(item.ID, 10))
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &auth.User{ID: bk.ID, Active: true, DisplayName: bk.DisplayName, Roles: []string{"bk"}}))
		w := httptest.NewRecorder()
		app.referStudentCaseToKesiswaan(w, r)
		return w
	}
	badForm := url.Values{"version": {strconv.FormatInt(owned.Case.Version, 10)}, "target_user_id": {"999999999"}, "reason": {"Alasan tetap terlihat"}}
	badRequest := httptest.NewRequest("POST", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10)+"/refer-kesiswaan", strings.NewReader(badForm.Encode()))
	badRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badRequest.Header.Set("HX-Request", "true")
	badRequest.SetPathValue("id", strconv.FormatInt(item.ID, 10))
	badRequest = badRequest.WithContext(context.WithValue(badRequest.Context(), principalKey{}, &auth.User{ID: bk.ID, Active: true, DisplayName: bk.DisplayName, Roles: []string{"bk"}}))
	badResponse := httptest.NewRecorder()
	app.referStudentCaseToKesiswaan(badResponse, badRequest)
	if badResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(badResponse.Body.String(), "Alasan tetap terlihat") || !strings.Contains(badResponse.Body.String(), "role=\"alert\"") {
		t.Fatal("failed referral should show inline form error and preserve the reason")
	}
	unchanged, err := store.GetStudentCase(ctx, bk.ID, item.ID)
	if err != nil || unchanged.Case.Stage != studentcases.StageCounselor || unchanged.Case.ResponsibleID != bk.ID {
		t.Fatal("failed referral changed the case")
	}

	hxResponse := request(true)
	if hxResponse.Code != http.StatusOK || hxResponse.Header().Get("HX-Redirect") != "/kesiswaan/kasus?forwarded=1" {
		t.Fatalf("HTMX referral should redirect to the scoped list: status=%d", hxResponse.Code)
	}
	caseForKes, err := store.GetStudentCase(ctx, kes.ID, item.ID)
	if err != nil || caseForKes.Case.Stage != studentcases.StageDiscipline || caseForKes.Case.ResponsibleID != kes.ID || caseForKes.CanReadCounseling || len(caseForKes.CounselingNotes) != 0 {
		t.Fatal("Kesiswaan should own the forwarded case without private counseling notes")
	}
	var event string
	var actor, fromOwner, toOwner int64
	var fromStage, toStage string
	var reasonPresent bool
	var recordedAt string
	if err = db.QueryRow(`SELECT event_type,actor_id,from_stage,to_stage,from_responsible_user_id,to_responsible_user_id,length(reason)>0,created_at::text FROM app.student_case_history WHERE case_id=$1 AND event_type='referred_to_kesiswaan'`, item.ID).Scan(&event, &actor, &fromStage, &toStage, &fromOwner, &toOwner, &reasonPresent, &recordedAt); err != nil {
		t.Fatal("read referral history")
	}
	if event != "referred_to_kesiswaan" || actor != bk.ID || fromOwner != bk.ID || toOwner != kes.ID || fromStage != "guru_bk" || toStage != "kesiswaan" || !reasonPresent || recordedAt == "" {
		t.Fatal("forwarding history is incomplete")
	}

	bkRequest := httptest.NewRequest("GET", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10), nil)
	bkRequest.SetPathValue("id", strconv.FormatInt(item.ID, 10))
	bkRequest = bkRequest.WithContext(context.WithValue(bkRequest.Context(), principalKey{}, &auth.User{ID: bk.ID, Active: true, DisplayName: bk.DisplayName, Roles: []string{"bk"}}))
	bkResponse := httptest.NewRecorder()
	app.studentCaseDetail(bkResponse, bkRequest)
	if bkResponse.Code != http.StatusNotFound || !strings.Contains(bkResponse.Body.String(), "app-sidebar") || strings.Contains(bkResponse.Body.String(), privateNote) {
		t.Fatal("former BK owner should receive a safe app-shell denial without private content")
	}
	kesRequest := httptest.NewRequest("GET", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10), nil)
	kesRequest.SetPathValue("id", strconv.FormatInt(item.ID, 10))
	kesRequest = kesRequest.WithContext(context.WithValue(kesRequest.Context(), principalKey{}, &auth.User{ID: kes.ID, Active: true, DisplayName: kes.DisplayName, Roles: []string{"kesiswaan"}}))
	kesResponse := httptest.NewRecorder()
	app.studentCaseDetail(kesResponse, kesRequest)
	if kesResponse.Code != http.StatusOK || strings.Contains(kesResponse.Body.String(), privateNote) {
		t.Fatal("Kesiswaan detail leaked private counseling content")
	}

	second, err := store.CreateStudentCase(ctx, wali.ID, studentcases.CreateInput{StudentID: studentID, Content: "Kasus kedua untuk redirect HTTP"})
	if err != nil {
		t.Fatal("create second case")
	}
	if err = store.ReferStudentCaseToBK(ctx, wali.ID, second.ID, second.Version, studentcases.ReferralInput{TargetUserID: bk.ID, Reason: "Perlu konseling lanjutan"}); err != nil {
		t.Fatal("refer second case to BK")
	}
	secondOwned, err := store.GetStudentCase(ctx, bk.ID, second.ID)
	if err != nil {
		t.Fatal("load second BK-owned case")
	}
	secondForm := url.Values{"version": {strconv.FormatInt(secondOwned.Case.Version, 10)}, "target_user_id": {strconv.FormatInt(kes.ID, 10)}, "reason": {"Lanjutkan penanganan disiplin"}}
	secondRequest := httptest.NewRequest("POST", "/kesiswaan/kasus/"+strconv.FormatInt(second.ID, 10)+"/refer-kesiswaan", strings.NewReader(secondForm.Encode()))
	secondRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	secondRequest.SetPathValue("id", strconv.FormatInt(second.ID, 10))
	secondRequest = secondRequest.WithContext(context.WithValue(secondRequest.Context(), principalKey{}, &auth.User{ID: bk.ID, Active: true, DisplayName: bk.DisplayName, Roles: []string{"bk"}}))
	secondResponse := httptest.NewRecorder()
	app.referStudentCaseToKesiswaan(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusSeeOther || secondResponse.Header().Get("Location") != "/kesiswaan/kasus?forwarded=1" {
		t.Fatalf("HTTP referral should redirect away from the now-inaccessible detail: status=%d", secondResponse.Code)
	}
	var secondStage string
	var referralCount int
	if err = db.QueryRow(`SELECT c.stage,(SELECT count(*) FROM app.student_case_history h WHERE h.case_id=c.id AND h.event_type='referred_to_kesiswaan') FROM app.student_cases c WHERE c.id=$1`, second.ID).Scan(&secondStage, &referralCount); err != nil || secondStage != "kesiswaan" || referralCount != 1 {
		t.Fatalf("HTTP referral failed or duplicated history: stage=%s count=%d err=%v", secondStage, referralCount, err)
	}
	listRequest := httptest.NewRequest("GET", "/kesiswaan/kasus?forwarded=1", nil)
	listRequest = listRequest.WithContext(context.WithValue(listRequest.Context(), principalKey{}, &auth.User{ID: bk.ID, Active: true, DisplayName: bk.DisplayName, Roles: []string{"bk"}}))
	listResponse := httptest.NewRecorder()
	app.studentCases(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), "berhasil diteruskan") || strings.Contains(listResponse.Body.String(), privateNote) {
		t.Fatal("BK success list should confirm referral without showing the reassigned case or private note")
	}
}

func TestStudentCaseWaliHandoffHTTPIntegration(t *testing.T) {
	db := testutil.Database(t, "../../.env")
	ctx := context.Background()
	store := &academic.Store{DB: db}
	oldWali, _ := testutil.User(t, db, "wali_kelas")
	newWali, _ := testutil.User(t, db, "wali_kelas")
	admin, _ := testutil.User(t, db, "admin")
	yearID, _ := testutil.ActiveCalendar(t, db, testutil.Random(t)[:12])
	suffix := testutil.Random(t)[:8]
	var classA, classB, oldTeacher, newTeacher, studentID, placementID int64
	for _, target := range []struct {
		name string
		user int64
		cid  *int64
		tid  *int64
	}{{name: "HTTP-HANDOFF-A-" + suffix, user: oldWali.ID, cid: &classA, tid: &oldTeacher}, {name: "HTTP-HANDOFF-B-" + suffix, user: newWali.ID, cid: &classB, tid: &newTeacher}} {
		if err := db.QueryRow(`INSERT INTO app.classes(school_year_id,grade,name) VALUES($1,7,$2) RETURNING id`, yearID, target.name).Scan(target.cid); err != nil {
			t.Fatal("create isolated handoff class")
		}
		if err := db.QueryRow(`INSERT INTO app.teachers(name,user_id,active) VALUES($1,$2,true) RETURNING id`, "Wali HTTP "+suffix, target.user).Scan(target.tid); err != nil {
			t.Fatal("create isolated handoff teacher")
		}
		if _, err := db.Exec(`INSERT INTO app.homeroom_assignments(class_id,teacher_id,assigned_by) VALUES($1,$2,$3)`, *target.cid, *target.tid, admin.ID); err != nil {
			t.Fatal("assign isolated class Wali")
		}
	}
	if err := db.QueryRow(`INSERT INTO app.students(name,birth_date,gender) VALUES($1,'2012-01-01','P') RETURNING id`, "Siswa HTTP Handoff "+suffix).Scan(&studentID); err != nil {
		t.Fatal("create isolated handoff student")
	}
	if err := db.QueryRow(`INSERT INTO app.student_placements(school_year_id,student_id,class_id,actor_id,starts_on) VALUES($1,$2,$3,$4,CURRENT_DATE-10) RETURNING id`, yearID, studentID, classA, admin.ID).Scan(&placementID); err != nil {
		t.Fatal("create isolated handoff placement")
	}
	item, err := store.CreateStudentCase(ctx, oldWali.ID, studentcases.CreateInput{StudentID: studentID, Content: "Ringkasan pembinaan awal HTTP"})
	if err != nil {
		t.Fatal("create Wali-stage case")
	}
	effective := time.Now().In(time.FixedZone("WIB", 7*60*60)).AddDate(0, 0, -1).Format("2006-01-02")
	if _, err = db.Exec(`INSERT INTO app.student_class_moves(placement_id,from_class_id,to_class_id,effective_date,reason,actor_id) VALUES($1,$2,$3,$4,'Uji HTTP perpindahan Wali',$5)`, placementID, classA, classB, effective, admin.ID); err != nil {
		t.Fatal("move isolated student to new Wali")
	}
	readable, err := store.GetStudentCase(ctx, newWali.ID, item.ID)
	if err != nil || !readable.CanTakeOver || readable.CanWrite {
		t.Fatal("new Wali did not receive eligible takeover detail")
	}
	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(p, i int) int { return (p-1)*10 + i + 1 }, "weekdayName": func(int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{templates: templates, options: Options{Academics: store}}
	newWaliUser := auth.User{ID: newWali.ID, Active: true, DisplayName: newWali.DisplayName, Roles: []string{"wali_kelas"}}
	getRequest := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10), nil)
		r.SetPathValue("id", strconv.FormatInt(item.ID, 10))
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &newWaliUser))
		w := httptest.NewRecorder()
		app.studentCaseDetail(w, r)
		return w
	}
	get := getRequest()
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "Ambil Alih Penanganan") {
		t.Fatal("new Wali detail did not render the handoff form in the application layout")
	}
	post := func(reason string, hx bool, version int64) *httptest.ResponseRecorder {
		form := url.Values{"version": {strconv.FormatInt(version, 10)}, "reason": {reason}}
		r := httptest.NewRequest("POST", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10)+"/ambil-alih", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if hx {
			r.Header.Set("HX-Request", "true")
		}
		r.SetPathValue("id", strconv.FormatInt(item.ID, 10))
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &newWaliUser))
		w := httptest.NewRecorder()
		app.takeOverStudentCase(w, r)
		return w
	}
	invalid := post("   ", true, readable.Case.Version)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "role=\"alert\"") || !strings.Contains(invalid.Body.String(), "Ambil Alih Penanganan") {
		t.Fatal("invalid handoff did not return a clear inline error in the detail layout")
	}
	const reason = "Meneruskan tindak lanjut setelah siswa pindah kelas"
	response := post(reason, true, readable.Case.Version)
	if response.Code != http.StatusOK || response.Header().Get("HX-Redirect") != "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10)+"?taken-over=1" {
		t.Fatalf("successful HTMX handoff should redirect to the updated detail: status=%d", response.Code)
	}
	owned, err := store.GetStudentCase(ctx, newWali.ID, item.ID)
	if err != nil || !owned.CanWrite || owned.Case.ResponsibleID != newWali.ID || len(owned.History) != 2 || owned.History[0].Event != studentcases.EventHomeroomHandoff {
		t.Fatal("successful HTTP handoff did not update owner and append exactly one history event")
	}
	if owned.History[0].Reason != reason || owned.History[0].FromResponsible != oldWali.ID || owned.History[0].ToResponsible != newWali.ID || owned.History[0].ActorID != newWali.ID || owned.History[0].CreatedAt.IsZero() {
		t.Fatal("HTTP handoff history omitted actor, time, reason, or owner transition")
	}
	confirmed := httptest.NewRequest("GET", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10)+"?taken-over=1", nil)
	confirmed.SetPathValue("id", strconv.FormatInt(item.ID, 10))
	confirmed = confirmed.WithContext(context.WithValue(confirmed.Context(), principalKey{}, &newWaliUser))
	confirmedResponse := httptest.NewRecorder()
	app.studentCaseDetail(confirmedResponse, confirmed)
	if confirmedResponse.Code != http.StatusOK || !strings.Contains(confirmedResponse.Body.String(), "berhasil mengambil alih") || !strings.Contains(confirmedResponse.Body.String(), reason) {
		t.Fatal("new Wali detail did not show handoff confirmation and audit history")
	}
	oldUser := auth.User{ID: oldWali.ID, Active: true, DisplayName: oldWali.DisplayName, Roles: []string{"wali_kelas"}}
	oldRequest := httptest.NewRequest("GET", "/kesiswaan/kasus/"+strconv.FormatInt(item.ID, 10), nil)
	oldRequest.SetPathValue("id", strconv.FormatInt(item.ID, 10))
	oldRequest = oldRequest.WithContext(context.WithValue(oldRequest.Context(), principalKey{}, &oldUser))
	oldResponse := httptest.NewRecorder()
	app.studentCaseDetail(oldResponse, oldRequest)
	if oldResponse.Code != http.StatusNotFound || strings.Contains(oldResponse.Body.String(), reason) {
		t.Fatal("former Wali could still access detail after handoff")
	}
}
