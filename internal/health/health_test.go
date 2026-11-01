package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLiveness(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	Liveness(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status field = %q, want %q", body.Status, "ok")
	}
}

// fakePinger is a Pinger for tests: it records the context it received and
// returns err.
type fakePinger struct {
	err error
	ctx context.Context
}

func (f *fakePinger) Ping(ctx context.Context) error {
	f.ctx = ctx
	return f.err
}

func decodeStatus(t *testing.T, body *bytes.Buffer) string {
	t.Helper()
	var v struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(body).Decode(&v); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	return v.Status
}

func TestReadinessOK(t *testing.T) {
	fake := &fakePinger{}
	rec := httptest.NewRecorder()

	Readiness(fake, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if got := decodeStatus(t, rec.Body); got != "ok" {
		t.Errorf("status field = %q, want %q", got, "ok")
	}
}

func TestReadinessUnavailable(t *testing.T) {
	const detail = "dial tcp 10.0.0.5:5432: connect: connection refused"
	fake := &fakePinger{err: errors.New(detail)}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	rec := httptest.NewRecorder()

	Readiness(fake, logger).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	body := rec.Body.String()
	if strings.Contains(body, detail) || strings.Contains(body, "refused") {
		t.Errorf("body leaks the error detail: %q", body)
	}
	if got := decodeStatus(t, bytes.NewBufferString(body)); got != "unavailable" {
		t.Errorf("status field = %q, want %q", got, "unavailable")
	}

	// The detail goes to the log, at Warn level, instead.
	var entry struct {
		Level string `json:"level"`
		Msg   string `json:"msg"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("log is not a single JSON entry: %v (%q)", err, logs.String())
	}
	if entry.Level != "WARN" || entry.Msg != "readiness check failed" || entry.Error != detail {
		t.Errorf("log entry = %+v, want WARN %q with the error detail", entry, "readiness check failed")
	}
}

func TestReadinessPingHasDeadline(t *testing.T) {
	fake := &fakePinger{}
	rec := httptest.NewRecorder()

	before := time.Now()
	Readiness(fake, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	after := time.Now()

	deadline, ok := fake.ctx.Deadline()
	if !ok {
		t.Fatal("the context passed to Ping has no deadline")
	}
	// The deadline was set somewhere between before and after, so it must lie
	// within [before+timeout, after+timeout].
	if deadline.Before(before.Add(readinessTimeout)) || deadline.After(after.Add(readinessTimeout)) {
		t.Errorf("deadline = %v, want about %v after the call (between %v and %v)", deadline, readinessTimeout, before.Add(readinessTimeout), after.Add(readinessTimeout))
	}
}

// blockingPinger blocks until its context ends and then returns the context's
// error, recording it. That makes visible WHY the context ended: canceled by the
// caller (context.Canceled) or by the handler's own timeout (DeadlineExceeded).
type blockingPinger struct{ err error }

func (b *blockingPinger) Ping(ctx context.Context) error {
	<-ctx.Done()
	b.err = ctx.Err()
	return b.err
}

// If the client gives up, the ping must stop right away, not run until the
// handler's own timeout. This only holds if the handler derives its context
// from the request's.
func TestReadinessPropagatesRequestContext(t *testing.T) {
	fake := &blockingPinger{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	start := time.Now()
	Readiness(fake, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed >= time.Second {
		t.Errorf("handler took %v after the client gave up, want it to return promptly", elapsed)
	}
	if !errors.Is(fake.err, context.Canceled) {
		t.Errorf("Ping context error = %v, want %v (the request's cancellation)", fake.err, context.Canceled)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
