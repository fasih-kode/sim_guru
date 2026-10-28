package web

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
	"github.com/fasih-kode/sim_guru/internal/master"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
)

func TestTeacherAttendanceUploadValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*multipart.Writer)
	}{
		{"missing selfie", func(*multipart.Writer) {}},
		{"duplicate field", func(w *multipart.Writer) {
			w.WriteField("action", "check_out")
			p, _ := w.CreateFormFile("selfie", "x.jpg")
			p.Write([]byte("image"))
		}},
		{"invalid number", func(w *multipart.Writer) {
			p, _ := w.CreateFormFile("selfie", "x.jpg")
			p.Write([]byte("image"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data bytes.Buffer
			writer := multipart.NewWriter(&data)
			for key, value := range map[string]string{"action": "check_in", "idempotency_key": strings.Repeat("a", 32), "latitude": "0", "longitude": "0", "accuracy_m": "5"} {
				if tc.name == "invalid number" && key == "latitude" {
					value = "bad"
				}
				writer.WriteField(key, value)
			}
			tc.change(writer)
			writer.Close()
			req := httptest.NewRequest("POST", "/kehadiran-saya", &data)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			_, err := teacherAttendanceUpload(httptest.NewRecorder(), req)
			if req.MultipartForm != nil {
				defer req.MultipartForm.RemoveAll()
			}
			if err == nil {
				t.Fatal("invalid form accepted")
			}
		})
	}
}

func TestTeacherAttendanceHTTPErrorSafe(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{academic.ErrConflict, 409}, {academic.ErrForbidden, 403}, {academic.ErrTeacherAttendanceAction, 422},
		{teacherphoto.ErrInvalidImage, 422}, {teacherphoto.ErrPhotoTooLarge, 422}, {academic.ErrTeacherAttendanceCommit, 503},
		{&http.MaxBytesError{Limit: teacherSelfieRequestLimit}, 413}, {errors.New("private-path secret-dsn"), 503},
	} {
		status, message := teacherAttendanceHTTPError(tc.err)
		if status != tc.want || strings.Contains(message, "private-path") || strings.Contains(message, "secret-dsn") {
			t.Fatalf("unsafe error status=%d", status)
		}
	}
}

func TestTeacherAttendanceRouteSecurity(t *testing.T) {
	handler, err := New(func(context.Context) error { return nil }, Options{Accounts: &auth.Store{}, Masters: &master.Store{}, Academics: &academic.Store{}, CSRFKey: bytes.Repeat([]byte("a"), 32)})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method string
		size   int64
		want   int
	}{{"GET", 0, 303}, {"POST", 0, 403}, {"POST", teacherSelfieRequestLimit + 1, 413}} {
		request := httptest.NewRequest(tc.method, "/kehadiran-saya", nil)
		request.ContentLength = tc.size
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("status=%d want%d", response.Code, tc.want)
		}
		if response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Header().Get("Content-Security-Policy"), "media-src 'self' blob:") {
			t.Fatal("missing secure camera/no-store headers")
		}
	}
}

func TestTeacherAttendanceTemplate(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(int, int) int { return 1 }, "weekdayName": func(int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, available := range []bool{false, true} {
		var out bytes.Buffer
		data := view{Page: "teacher-attendance", Title: "Kehadiran Saya", User: &auth.User{Roles: []string{"guru_mapel", "wali_kelas"}}, TeacherAttendance: &teacherAttendanceView{Available: available, Day: academic.TeacherAttendanceDay{TeacherName: "Guru <uji>", Date: "2026-10-28"}}}
		if err = templates.ExecuteTemplate(&out, "page", data); err != nil {
			t.Fatal(err)
		}
		text := out.String()
		if strings.Count(text, `href="/kehadiran-saya"`) != 1 || !strings.Contains(text, "Belum Dicatat") || !strings.Contains(text, "Guru &lt;uji&gt;") || !strings.Contains(text, "teacher-attendance.js") {
			t.Fatal("missing safe teacher attendance page")
		}
		if !available && !strings.Contains(text, "Kehadiran mandiri belum tersedia") {
			t.Fatal("missing safe disabled state")
		}
	}
}

func TestTeacherAttendanceSavedMessageAcrossWIBDay(t *testing.T) {
	if got := teacherAttendanceSavedMessage("2026-10-29", "2026-10-28"); !strings.Contains(got, "2026-10-28") || !strings.Contains(got, "hari ini") {
		t.Fatal("retry lintas hari kehilangan tanggal bukti")
	}
	for _, recorded := range []string{"2026-10-29", "", "<script>bad</script>"} {
		if got := teacherAttendanceSavedMessage("2026-10-29", recorded); got != "Kehadiran berhasil dicatat." {
			t.Fatal("pesan tanggal tidak valid")
		}
	}
}
