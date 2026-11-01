package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRouter(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  string // substring expected in the Allow header, if any
	}{
		{name: "GET /healthz", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK},
		{name: "POST /healthz", method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "GET unknown route", method: http.MethodGet, path: "/nao-existe", wantStatus: http.StatusNotFound},
	}

	router := newRouter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	if !strings.Contains(err.Error(), "load config") || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("run() error = %q, want it to mention \"load config\" and LOG_LEVEL", err)
	}
	if ctx.Err() != nil {
		t.Error("run() blocked until the context timeout: it probably started the server")
	}
}
