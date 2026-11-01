package postgres

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// The URLs below carry fake passwords. Open must never leak any piece of them.

func TestOpenInvalidURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string   // stable part of the expected message (identifies the rule)
		secrets []string // every piece of the password: none may appear in the error
	}{
		// Rule 1: url.Parse fails.
		{name: "non-numeric port", url: "postgres://user:s3cr3t@host:notaport/db", want: "not a valid URL", secrets: []string{"s3cr3t"}},
		{name: "unclosed IPv6 bracket", url: "postgres://user:s3cr3t@[::1/db", want: "not a valid URL", secrets: []string{"s3cr3t"}},
		{name: "with quotes", url: `"postgres://u:s3cr3t@127.0.0.1:5432/db"`, want: "not a valid URL", secrets: []string{"s3cr3t"}},
		{name: "cut at # by an .env reader", url: "postgres://shortener:Pref1x", want: "not a valid URL", secrets: []string{"Pref1x"}},

		// Rule 2: scheme (this also rejects the keyword/value format).
		{name: "other scheme", url: "mysql://u:s3cr3t@127.0.0.1:5432/db", want: "scheme must be", secrets: []string{"s3cr3t"}},
		{name: "uppercase scheme", url: "POSTGRES://u:s3cr3t@127.0.0.1:5432/db", want: "scheme must be", secrets: []string{"s3cr3t"}},
		{name: "no scheme", url: "u:ZQ9xKw!p@ss#w0rd@h:5432/db", want: "scheme must be", secrets: []string{"ZQ9xKw", "ZQ9xKw!p", "p@ss", "ss#w0rd", "w0rd"}},
		{name: "key/value form", url: "host=127.0.0.1 user=u password=TOPsecret Tail9 dbname=db", want: "scheme must be", secrets: []string{"TOPsecret", "Tail9"}},
		{name: "key/value form with invalid port", url: "host=h port=notaport user=user password=s3cr3t dbname=db", want: "scheme must be", secrets: []string{"s3cr3t"}},
		{name: "key/value form with unterminated quote", url: "host=h user=user password='s3cr3t dbname=db", want: "scheme must be", secrets: []string{"s3cr3t"}},

		// Rule 3: '#'.
		{name: "raw # in the password", url: "postgres://u:ZQ9xKw!p@ss#w0rd@h:5432/db", want: "contains '#'", secrets: []string{"ZQ9xKw", "ZQ9xKw!p", "p@ss", "ss#w0rd", "w0rd"}},

		// Rule 4: required parts.
		{name: "empty URL", url: "postgres://", want: "host, user and database name are required"},
		{name: "no database", url: "postgres://u:s3cr3t@127.0.0.1:5432/", want: "host, user and database name are required", secrets: []string{"s3cr3t"}},
		{name: "no user", url: "postgres://127.0.0.1:5432/db", want: "host, user and database name are required"},

		// Rule 5: pgx rejects it.
		{name: "invalid percent escape in a parameter", url: "postgres://user:s3cr3t@host:5432/db?sslmode=%zz", want: "invalid port or connection parameter", secrets: []string{"s3cr3t"}},
		{name: "unknown sslmode", url: "postgres://user:s3cr3t@host:5432/db?sslmode=banana", want: "invalid port or connection parameter", secrets: []string{"s3cr3t"}},
		{name: "invalid pool option", url: "postgres://user:s3cr3t@host:5432/db?pool_max_conns=abc", want: "invalid port or connection parameter", secrets: []string{"s3cr3t"}},

		// Rule 6: net/url and pgx disagree (raw special character in the password).
		{name: "raw @ in the password", url: "postgres://u:TOPsecret@Tail9@127.0.0.1:5432/db", want: "ambiguous URL", secrets: []string{"TOPsecret", "Tail9", "TOPsecret@Tail9"}},
		{name: "user overridden in the query", url: "postgres://u:s3cr3t@127.0.0.1:5432/db?user=other", want: "ambiguous URL", secrets: []string{"s3cr3t"}},
		{name: "database overridden in the query", url: "postgres://u:s3cr3t@127.0.0.1:5432/db?dbname=other", want: "ambiguous URL", secrets: []string{"s3cr3t"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := Open(context.Background(), tt.url)
			if err == nil {
				pool.Close()
				t.Fatal("Open() error = nil, want an error")
			}
			t.Logf("error: %v", err)
			msg := err.Error()
			if !strings.Contains(msg, "parse DATABASE_URL") || !strings.Contains(msg, tt.want) {
				t.Errorf("error %q does not contain %q", msg, tt.want)
			}
			// These cases are rejected by validation: none may reach the
			// network (and so the ping).
			if strings.Contains(msg, "ping database") {
				t.Errorf("error %q: validation let the URL reach the ping", msg)
			}
			for _, s := range tt.secrets {
				if strings.Contains(msg, s) {
					t.Errorf("error leaks %q of the password: %q", s, msg)
				}
			}
		})
	}
}

func TestOpenUnreachableDatabase(t *testing.T) {
	// Port 1 is closed: the connection is refused right away.
	url := "postgres://user:s3cr3t@127.0.0.1:1/db?connect_timeout=1"

	start := time.Now()
	pool, err := Open(context.Background(), url)
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("Open() error = nil, want an error")
	}
	t.Logf("error after %v: %v", elapsed, err)
	if !strings.Contains(err.Error(), "ping database") {
		t.Errorf("error %q does not contain %q", err, "ping database")
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error leaks the password: %q", err)
	}
	if elapsed >= connectTimeout/2 {
		t.Errorf("Open took %v, want well under connectTimeout (%v)", elapsed, connectTimeout)
	}
}

// A password with special characters works when it is percent-encoded: the URL
// passes validation and only fails at the ping (nothing listens on port 1).
func TestOpenPercentEncodedPassword(t *testing.T) {
	url := "postgres://u:TOPsecret%40Tail9@127.0.0.1:1/db?connect_timeout=1"

	pool, err := Open(context.Background(), url)
	if err == nil {
		pool.Close()
		t.Fatal("Open() error = nil, want an error")
	}
	t.Logf("error: %v", err)
	if !strings.Contains(err.Error(), "ping database") {
		t.Errorf("error %q does not contain %q: the URL should have passed validation", err, "ping database")
	}
	for _, s := range []string{"TOPsecret", "Tail9", "%40"} {
		if strings.Contains(err.Error(), s) {
			t.Errorf("error leaks %q of the password: %q", s, err)
		}
	}
}

func TestOpenCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pool, err := Open(ctx, "postgres://user:s3cr3t@127.0.0.1:1/db")
	if err == nil {
		pool.Close()
		t.Fatal("Open() error = nil, want an error")
	}
	t.Logf("error: %v", err)
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error leaks the password: %q", err)
	}
}

// Open must honor the caller's context while waiting for the database. The
// server below accepts connections and never answers, so the only thing that
// can end the wait early is the context.
func TestOpenHonorsContextWhileWaiting(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu    sync.Mutex
		conns []net.Conn
	)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})

	// No connect_timeout in the URL: only ctx (200ms) may end the wait.
	url := "postgres://user:s3cr3t@" + ln.Addr().String() + "/db"
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	pool, err := Open(ctx, url)
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("Open() error = nil, want an error")
	}
	t.Logf("error after %v: %v", elapsed, err)
	if elapsed >= 2*time.Second {
		t.Errorf("Open took %v with a 200ms context: it ignores the context", elapsed)
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error leaks the password: %q", err)
	}
}
