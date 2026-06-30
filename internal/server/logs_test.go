package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mar-schmidt/Podium/internal/config"
	podiumlog "github.com/mar-schmidt/Podium/internal/logging"
)

func TestHandleLogsReturnsTailForLoopback(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := os.MkdirAll(paths.LogsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(podiumlog.Path(paths.LogsDir), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Options{Paths: paths})
	req := httptest.NewRequest(http.MethodGet, "/api/logs?lines=2", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()

	s.handleLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"two"`) || !strings.Contains(body, `"three"`) || strings.Contains(body, `"one"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestHandleLogsRejectsNonLoopback(t *testing.T) {
	s := New(Options{Paths: config.NewPaths(t.TempDir())})
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()

	s.handleLogs(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestHandleLogsFollowStreamsNDJSON(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := os.MkdirAll(paths.LogsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(podiumlog.Path(paths.LogsDir), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Options{Paths: paths})
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/logs/follow?lines=1", nil).WithContext(ctx)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleLogsFollow(rr, req)
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for !strings.Contains(rr.Body.String(), `"line":"hello"`) {
		select {
		case <-deadline:
			cancel()
			t.Fatalf("timed out waiting for stream body: %s", rr.Body.String())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not stop after context cancellation")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
}
