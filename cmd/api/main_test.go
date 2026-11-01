package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/felipealvs21/url-shortener/internal/health"
)

// fakePinger is a health.Pinger that returns err.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestNewRouter(t *testing.T) {
	tests := []struct {
		name       string
		db         health.Pinger
		method     string
		path       string
		wantStatus int
		wantAllow  string // substring expected in the Allow header, if any
	}{
		{name: "GET /healthz", db: fakePinger{}, method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK},
		{name: "POST /healthz", db: fakePinger{}, method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "GET /healthz with database down", db: fakePinger{err: errors.New("down")}, method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK},
		{name: "GET /readyz database ok", db: fakePinger{}, method: http.MethodGet, path: "/readyz", wantStatus: http.StatusOK},
		{name: "GET /readyz database down", db: fakePinger{err: errors.New("down")}, method: http.MethodGet, path: "/readyz", wantStatus: http.StatusServiceUnavailable},
		{name: "POST /readyz", db: fakePinger{}, method: http.MethodPost, path: "/readyz", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "GET unknown route", db: fakePinger{}, method: http.MethodGet, path: "/nao-existe", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newRouter(tt.db, slog.New(slog.DiscardHandler))
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantAllow != "" {
				if got := rec.Header().Get("Allow"); !strings.Contains(got, tt.wantAllow) {
					t.Errorf("Allow = %q, want it to contain %q", got, tt.wantAllow)
				}
			}
		})
	}
}

func TestRunInvalidConfig(t *testing.T) {
	// Hold a port ourselves. If run got past config validation it would try to
	// bind it and fail with a "listen" error instead of a "load config" one, so
	// the assertion below proves the config is checked before any listen.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	env := map[string]string{
		"HTTP_ADDR": ln.Addr().String(),
		"LOG_LEVEL": "verbose", // invalid
	}
	getenv := func(k string) string { return env[k] }

	// The timeout turns a regression (run starting the server and blocking)
	// into a test failure instead of a hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = run(ctx, slog.New(slog.DiscardHandler), new(slog.LevelVar), getenv)
	if err == nil {
		t.Fatal("run() = nil, want a config error")
	}
	// DATABASE_URL is absent from env, so it is reported together with LOG_LEVEL.
	for _, want := range []string{"load config", "LOG_LEVEL", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("run() error = %q, want it to mention %q", err, want)
		}
	}
	if ctx.Err() != nil {
		t.Error("run() blocked until the context timeout: it probably started the server")
	}
}

func TestRunDatabaseDown(t *testing.T) {
	// Hold a port ourselves. If run opened the HTTP port before confirming the
	// database, it would fail with a "listen" error instead of "open database",
	// so the assertion below proves the server does not listen without a database.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	env := map[string]string{
		"HTTP_ADDR": ln.Addr().String(),
		// Port 1 is closed: the connection is refused right away.
		"DATABASE_URL": "postgres://user:s3cr3t@127.0.0.1:1/db?connect_timeout=1",
	}
	getenv := func(k string) string { return env[k] }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = run(ctx, slog.New(slog.DiscardHandler), new(slog.LevelVar), getenv)
	if err == nil {
		t.Fatal("run() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("run() error = %q, want it to mention \"open database\"", err)
	}
	if strings.Contains(err.Error(), "listen") {
		t.Errorf("run() error = %q: it got as far as listening", err)
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("run() error leaks the password: %q", err)
	}
}
