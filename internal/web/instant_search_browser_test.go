//go:build browserqa

package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/studentaffairs"
)

// TestInstantSearchBrowserHarness hanya menghidangkan view sintetis di loopback.
// Tidak membuka database, membuat akun, atau melewati otorisasi server aplikasi.
func TestInstantSearchBrowserHarness(t *testing.T) {
	path := os.Getenv("SIM_GURU_BROWSER_ADDRESS_FILE")
	if path == "" {
		t.Skip("harness browser hanya ketika diminta eksplisit")
	}
	templates, e := template.New("web").Funcs(template.FuncMap{"rowNumber": func(p, i int) int { return (p-1)*10 + i + 1 }, "weekdayName": func(i int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if e != nil {
		t.Fatal(e)
	}
	app := &application{templates: templates}
	done := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var failCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__qa_done" {
			once.Do(func() { close(done) })
			return
		}
		if strings.HasPrefix(r.URL.Path, "/static/") {
			http.FileServer(http.FS(content)).ServeHTTP(w, r)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &auth.User{ID: 1, Active: true, DisplayName: "Fixture UI tanpa akun DB", Roles: []string{"admin", "kurikulum", "pegawai_tu", "pengelola_kehadiran_guru", "kesiswaan"}}))
		if r.Method == "POST" {
			if r.URL.Path != "/kesiswaan/pembinaan/1/status" {
				http.Error(w, "Fixture tidak menyimpan data", 405)
				return
			}
			_ = r.ParseForm()
			data := &studentGuidanceView{
				Record:    studentaffairs.GuidanceRecord{ID: 1, Version: 1, StudentNameAtEvent: "Siswa Fixture 01", ClassNameAtEvent: "8A", Date: "2026-10-20", Incident: "Kejadian Fixture", Action: "Percakapan dan tindak lanjut", Status: studentaffairs.GuidanceNeedsFollowUp},
				CanManage: true, StatusVersion: 1,
				StatusChange: studentaffairs.GuidanceStatusChange{Status: studentaffairs.GuidanceStatus(r.PostForm.Get("status")), Reason: r.PostForm.Get("reason")},
			}
			status := http.StatusUnprocessableEntity
			data.StatusError = "Pilih status yang berbeda untuk mencatat perubahan."
			if r.PostForm.Get("version") != "1" {
				status = http.StatusConflict
				data.StatusError = "Catatan pembinaan berubah. Muat ulang sebelum menyimpan kembali."
			}
			app.render(w, r, "student-guidance-detail", status, view{Page: "student-guidance-detail", Title: "Detail Catatan Pembinaan", StudentGuidance: data})
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Fixture tidak menyimpan data", 405)
			return
		}
		q := r.URL.Query().Get("q")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		if q == "lama" {
			time.Sleep(2 * time.Second)
		}
		if q == "gagal" && r.URL.Path == "/kesiswaan/pembinaan" {
			mu.Lock()
			failCount++
			fail := failCount == 1
			mu.Unlock()
			if fail {
				http.Error(w, "Gangguan sintetis", 503)
				return
			}
		}
		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		if from == "" {
			from = "2026-10-01"
		}
		if to == "" {
			to = "2026-10-29"
		}
		v := view{Title: "Pengujian otomatis — bukan data sekolah", Page: "teacher-recap"}
		switch {
		case strings.HasPrefix(r.URL.Path, "/admin/users"):
			v.Page = "users"
			v.Filter = auth.Filter{Search: q, Page: page, Role: r.URL.Query().Get("role"), Status: r.URL.Query().Get("status")}
			v.ModalOpen = strings.HasSuffix(r.URL.Path, "/new")
			v.Input.Active = true
			for i := 1; i <= 13; i++ {
				name := fmt.Sprintf("Personel Fixture %02d", i)
				if q == "" || strings.Contains(strings.ToLower(name), strings.ToLower(q)) {
					v.Users = append(v.Users, auth.User{ID: int64(i), Username: fmt.Sprintf("fixture_%02d", i), DisplayName: name, Active: true, Roles: []string{"guru_mapel"}})
				}
			}
			v.RoleChoices = []roleChoice{{Code: "guru_mapel", Name: "Guru Mapel"}}
		case r.URL.Path == "/akademik/tahun-ajaran":
			v.Page = "academic-years"
			v.Academic = &academicView{}
			for i := 1; i <= 20; i++ {
				v.Academic.Years = append(v.Academic.Years, academic.Year{ID: int64(i), Name: fmt.Sprintf("Tahun Fixture %02d", i)})
			}
		case r.URL.Path == "/tu/kehadiran-guru/laporan":
			v.Page = "teacher-reports"
			v.TeacherReports = &teacherReportsView{Base: r.URL.Path, Filter: academic.TeacherAttendanceReportFilter{Search: q, StartsOn: from, EndsOn: to, Kind: r.URL.Query().Get("kind"), Page: 1}}
			for i := 1; i <= 5; i++ {
				kind := "izin"
				if i%2 == 0 {
					kind = "sakit"
				}
				if v.TeacherReports.Filter.Kind != "" && v.TeacherReports.Filter.Kind != kind {
					continue
				}
				v.TeacherReports.Reports = append(v.TeacherReports.Reports, academic.TeacherAttendanceReport{ID: int64(i), TeacherName: fmt.Sprintf("Guru Fixture %02d", i), Kind: kind, StartsOn: from, EndsOn: to, Active: true})
			}
		case r.URL.Path == "/kesiswaan/pembinaan":
			v.Page = "student-guidance"
			guidance := &studentGuidanceView{CanManage: true, Filter: studentaffairs.GuidanceFilter{Search: q, Page: page, Status: studentaffairs.GuidanceStatus(r.URL.Query().Get("status"))}}
			for i := 1; i <= 13; i++ {
				incident := fmt.Sprintf("Pembinaan Fixture %02d", i)
				if i == 1 {
					incident += " " + strings.Repeat("uraianpanjang", 35)
				}
				record := studentaffairs.GuidanceRecord{ID: int64(i), StudentNameAtEvent: fmt.Sprintf("Siswa Fixture %02d", i), ClassNameAtEvent: "8A", Date: "2026-10-20", Incident: incident, Action: "Percakapan dan tindak lanjut", Status: studentaffairs.GuidanceNeedsFollowUp}
				if i%3 == 0 {
					record.Status = studentaffairs.GuidanceCompleted
				}
				if q != "" && q != "gagal" && !strings.Contains(strings.ToLower(record.StudentNameAtEvent+" "+record.Incident+" "+record.Action), strings.ToLower(q)) {
					continue
				}
				if guidance.Filter.Status != "" && guidance.Filter.Status != record.Status {
					continue
				}
				guidance.List.Records = append(guidance.List.Records, record)
			}
			guidance.List.Page = page
			start := (page - 1) * 10
			if start > len(guidance.List.Records) {
				start = len(guidance.List.Records)
			}
			end := start + 10
			if end < len(guidance.List.Records) {
				guidance.List.HasNext = true
			} else {
				end = len(guidance.List.Records)
			}
			guidance.List.Records = guidance.List.Records[start:end]
			guidance.Next = 0
			if guidance.List.HasNext {
				guidance.Next = page + 1
			}
			if page > 1 {
				guidance.Previous = page - 1
			}
			if r.URL.Query().Get("new") == "1" {
				guidance.ModalOpen = true
				guidance.Students = []studentaffairs.GuidanceStudentOption{{ID: 1, Name: "Siswa Fixture 01", ClassName: "8A"}}
				guidance.Input = studentaffairs.GuidanceInput{Date: "2026-10-29", Status: studentaffairs.GuidanceNeedsFollowUp}
			}
			v.StudentGuidance = guidance
		case strings.HasPrefix(r.URL.Path, "/kesiswaan/pembinaan/"):
			v.Page = "student-guidance-detail"
			v.StudentGuidance = &studentGuidanceView{
				Record:    studentaffairs.GuidanceRecord{ID: 1, Version: 1, StudentNameAtEvent: "Siswa Fixture 01", ClassNameAtEvent: "8A", Date: "2026-10-20", Incident: "Kejadian Fixture", Action: "Percakapan dan tindak lanjut", Status: studentaffairs.GuidanceNeedsFollowUp},
				CanManage: true, StatusVersion: 1,
			}
		default:
			data := &teacherRecapView{Base: r.URL.Path, From: from, To: to, Search: q, Managed: true, Page: page, DayPage: 1}
			for i := 1; i <= 13; i++ {
				name := fmt.Sprintf("Guru Fixture %02d", i)
				if q != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(q)) && q != "lama" && q != "baru" && q != "gagal" {
					continue
				}
				if q == "lama" {
					name = "Hasil LAMA"
				}
				if q == "baru" {
					name = "Hasil BARU"
				}
				data.Teachers = append(data.Teachers, academic.TeacherAttendanceRecap{TeacherID: int64(i), TeacherName: name})
			}
			start := (page - 1) * 10
			if start > len(data.Teachers) {
				start = len(data.Teachers)
			}
			end := start + 10
			if end < len(data.Teachers) {
				data.NextURL = fmt.Sprintf("%s?from=%s&to=%s&q=%s&page=%d", r.URL.Path, from, to, q, page+1)
			} else {
				end = len(data.Teachers)
			}
			data.Teachers = data.Teachers[start:end]
			if page > 1 {
				data.PreviousURL = fmt.Sprintf("%s?from=%s&to=%s&q=%s&page=%d", r.URL.Path, from, to, q, page-1)
			}
			v.TeacherRecap = data
		}
		app.render(w, r, "page", 200, v)
	}))
	defer server.Close()
	if e = os.WriteFile(path, []byte(server.URL), 0600); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		t.Fatal("harness browser melewati batas waktu")
	}
}
