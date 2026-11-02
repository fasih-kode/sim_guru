package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFailureLogsGenericCategoryAndReturnsSafeResponse(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	// Tes ini tidak paralel karena slog.Default berlaku untuk seluruh proses.
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	tests := []struct {
		name, category string
		err            error
	}{
		{name: "internal", category: "storage_or_internal", err: errors.New("token-sensitive-marker")},
		{name: "sqlstate", category: "sqlstate_23505", err: &pgconn.PgError{Code: "23505", Message: "sql-sensitive-marker", Detail: "sql-sensitive-marker"}},
		{name: "timeout", category: "timeout", err: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		before := output.Len()
		response := httptest.NewRecorder()
		(&application{}).failure(response, tt.err)

		if response.Code != http.StatusInternalServerError {
			t.Errorf("%s: status=%d want=%d", tt.name, response.Code, http.StatusInternalServerError)
		}
		if response.Body.String() != "Layanan belum dapat memproses permintaan. Coba kembali.\n" {
			t.Errorf("%s: response body is not the expected generic message", tt.name)
		}

		line := output.String()[before:]
		if !strings.Contains(line, `msg="operasi aplikasi gagal"`) {
			t.Errorf("%s: log message is not module-neutral: %q", tt.name, line)
		}
		if !strings.Contains(line, `category=`+tt.category) {
			t.Errorf("%s: log category missing or changed: %q", tt.name, line)
		}
		if strings.Contains(line, "sensitive-marker") || strings.Contains(line, tt.err.Error()) {
			t.Errorf("%s: raw error details leaked to log: %q", tt.name, line)
		}
	}
}

func TestAcademicFailureKeepsClientErrorStatusAndMessage(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{name: "validation", err: academic.ValidationError("Input tidak valid."), status: http.StatusUnprocessableEntity, message: "Input tidak valid."},
		{name: "forbidden", err: academic.ErrForbidden, status: http.StatusForbidden, message: academic.ErrForbidden.Error()},
		{name: "not found", err: academic.ErrNotFound, status: http.StatusNotFound, message: academic.ErrNotFound.Error()},
		{name: "teacher lesson attendance blocks mutation", err: &academic.TeacherLessonIneligibleError{Reason: "Presensi guru belum memenuhi syarat."}, status: http.StatusConflict, message: "Presensi guru belum memenuhi syarat."},
		{name: "conflict", err: academic.ErrConflict, status: http.StatusConflict, message: academic.ErrConflict.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			(&application{}).academicFailure(response, tt.err)
			if response.Code != tt.status {
				t.Fatalf("status=%d want=%d", response.Code, tt.status)
			}
			if response.Body.String() != tt.message+"\n" {
				t.Fatalf("body=%q want=%q", response.Body.String(), tt.message+"\n")
			}
		})
	}
}
