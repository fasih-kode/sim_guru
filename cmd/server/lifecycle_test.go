package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
)

func TestServeStopsWorkerBeforeStorageClose(t *testing.T) {
	for _, shutdown := range []string{"context cancelled", "HTTP server closed"} {
		t.Run(shutdown, func(t *testing.T) {
			photos, err := openTeacherPhotoStorage(photoStorageConfig(t))
			if err != nil {
				t.Fatal(err)
			}
			defer photos.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started, exited, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			purger := photoPurgeFunc(func(ctx context.Context, limit int) (academic.TeacherAttendancePhotoPurgeResult, error) {
				close(started)
				<-ctx.Done()
				// Worker wajib masih boleh mengakses storage selama cleanup transaksi berlangsung.
				if _, err := photos.ListKeys(); err != nil {
					t.Errorf("storage ditutup terlalu dini: %v", err)
				}
				close(exited)
				return academic.TeacherAttendancePhotoPurgeResult{}, ctx.Err()
			})
			server := &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()}
			go func() {
				defer close(done)
				if err := serve(ctx, server, purger, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
					t.Errorf("serve=%v", err)
				}
			}()
			waitSignal(t, started)
			if shutdown == "context cancelled" {
				cancel()
			} else {
				if err := server.Close(); err != nil {
					t.Fatal(err)
				}
			}
			waitSignal(t, done)
			select {
			case <-exited:
			default:
				t.Fatal("serve selesai sebelum worker berhenti")
			}
			if err := photos.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := photos.ListKeys(); !errors.Is(err, teacherphoto.ErrClosed) {
				t.Fatalf("storage belum ditutup: %v", err)
			}
		})
	}
}

func TestServeListenFailureDoesNotStartWorker(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var calls atomic.Int32
	purger := photoPurgeFunc(func(context.Context, int) (academic.TeacherAttendancePhotoPurgeResult, error) {
		calls.Add(1)
		return academic.TeacherAttendancePhotoPurgeResult{}, nil
	})
	err = serve(context.Background(), &http.Server{Addr: listener.Addr().String()}, purger, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || calls.Load() != 0 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestServeWithoutPhotoStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address := make(chan string, 1)
	done := make(chan struct{})
	server := &http.Server{
		Addr: "127.0.0.1:0", Handler: http.NotFoundHandler(),
		BaseContext: func(listener net.Listener) context.Context {
			address <- listener.Addr().String()
			return context.Background()
		},
	}
	go func() {
		defer close(done)
		if err := serve(ctx, server, nil, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
			t.Errorf("serve tanpa storage=%v", err)
		}
	}()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + <-address + "/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("HTTP status=%d", response.StatusCode)
	}
	cancel()
	waitSignal(t, done)
}

func TestServeShutdownTimeoutStillStopsWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address := make(chan string, 1)
	entered, release, workerStarted, workerExited := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(release)
	server := &http.Server{
		Addr: "127.0.0.1:0",
		BaseContext: func(listener net.Listener) context.Context {
			address <- listener.Addr().String()
			return context.Background()
		},
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			close(entered)
			<-release
		}),
	}
	purger := photoPurgeFunc(func(ctx context.Context, _ int) (academic.TeacherAttendancePhotoPurgeResult, error) {
		defer close(workerExited)
		close(workerStarted)
		<-ctx.Done()
		return academic.TeacherAttendancePhotoPurgeResult{}, ctx.Err()
	})
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, purger, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		client := &http.Client{Timeout: 15 * time.Second}
		response, err := client.Get("http://" + <-address + "/")
		if err == nil {
			response.Body.Close()
		}
	}()
	waitSignal(t, entered)
	waitSignal(t, workerStarted)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("shutdown request aktif seharusnya mencapai timeout")
		}
	case <-time.After(13 * time.Second):
		t.Fatal("shutdown server tidak berhenti")
	}
	waitSignal(t, workerExited)
	waitSignal(t, requestDone)
}
