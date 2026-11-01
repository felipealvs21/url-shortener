package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

// env returns a getenv function backed by a map.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// testURL is a fake DATABASE_URL. It contains a recognizable password so tests
// can check that it never leaks into error messages.
const testURL = "postgres://user:s3cr3t@127.0.0.1:5432/db"

// withURL returns a copy of m that has DATABASE_URL set to testURL, unless m
// already sets it. DATABASE_URL is required, so tests about the other
// variables need a valid one to isolate what they check.
func withURL(m map[string]string) map[string]string {
	out := map[string]string{"DATABASE_URL": testURL}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{
			name: "defaults when only DATABASE_URL is set",
			env:  nil,
			want: Config{HTTPAddr: "127.0.0.1:8080", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second},
		},
		{
			name: "all values set",
			env:  map[string]string{"HTTP_ADDR": "0.0.0.0:9000", "LOG_LEVEL": "debug", "SHUTDOWN_TIMEOUT": "1m"},
			want: Config{HTTPAddr: "0.0.0.0:9000", LogLevel: slog.LevelDebug, ShutdownTimeout: time.Minute},
		},
		{
			name: "blank values fall back to defaults",
			env:  map[string]string{"HTTP_ADDR": "   ", "LOG_LEVEL": "", "SHUTDOWN_TIMEOUT": "\t"},
			want: Config{HTTPAddr: "127.0.0.1:8080", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second},
		},
		{
			name: "values are trimmed",
			env:  map[string]string{"HTTP_ADDR": " :8081 ", "LOG_LEVEL": " warn ", "SHUTDOWN_TIMEOUT": " 5s "},
			want: Config{HTTPAddr: ":8081", LogLevel: slog.LevelWarn, ShutdownTimeout: 5 * time.Second},
		},
		{
			name: "DATABASE_URL is trimmed",
			env:  map[string]string{"DATABASE_URL": "  postgres://u:p@h:5432/d\t"},
			want: Config{HTTPAddr: "127.0.0.1:8080", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second, DatabaseURL: "postgres://u:p@h:5432/d"},
		},
		{
			name: "smallest valid port",
			env:  map[string]string{"HTTP_ADDR": "127.0.0.1:1"},
			want: Config{HTTPAddr: "127.0.0.1:1", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second},
		},
		{
			name: "largest valid port",
			env:  map[string]string{"HTTP_ADDR": "127.0.0.1:65535"},
			want: Config{HTTPAddr: "127.0.0.1:65535", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second},
		},
		{
			// The host is not resolved by Load; a bad host fails later, in Listen.
			name: "arbitrary host with valid port is accepted",
			env:  map[string]string{"HTTP_ADDR": "nao.existe.invalid:8080"},
			want: Config{HTTPAddr: "nao.existe.invalid:8080", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second},
		},
		{
			name: "log level is case-insensitive",
			env:  map[string]string{"LOG_LEVEL": "ERROR"},
			want: Config{HTTPAddr: "127.0.0.1:8080", LogLevel: slog.LevelError, ShutdownTimeout: 10 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(env(withURL(tt.env)))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if tt.want.DatabaseURL == "" {
				tt.want.DatabaseURL = testURL
			}
			if got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string // substrings that must appear in the error
	}{
		{name: "address without port", env: map[string]string{"HTTP_ADDR": "semporta"}, want: []string{`HTTP_ADDR="semporta"`}},
		{name: "address with empty port", env: map[string]string{"HTTP_ADDR": "localhost:"}, want: []string{`HTTP_ADDR="localhost:"`}},
		{name: "port too large", env: map[string]string{"HTTP_ADDR": "127.0.0.1:99999"}, want: []string{`HTTP_ADDR="127.0.0.1:99999"`}},
		{name: "port just above range", env: map[string]string{"HTTP_ADDR": "127.0.0.1:65536"}, want: []string{`HTTP_ADDR="127.0.0.1:65536"`}},
		{name: "port zero", env: map[string]string{"HTTP_ADDR": "127.0.0.1:0"}, want: []string{`HTTP_ADDR="127.0.0.1:0"`}},
		{name: "non-numeric port", env: map[string]string{"HTTP_ADDR": "127.0.0.1:abc"}, want: []string{`HTTP_ADDR="127.0.0.1:abc"`}},
		{name: "negative port", env: map[string]string{"HTTP_ADDR": "127.0.0.1:-1"}, want: []string{`HTTP_ADDR="127.0.0.1:-1"`}},
		{name: "port with plus sign", env: map[string]string{"HTTP_ADDR": "127.0.0.1:+8080"}, want: []string{`HTTP_ADDR="127.0.0.1:+8080"`}},
		{name: "unknown log level", env: map[string]string{"LOG_LEVEL": "verbose"}, want: []string{`LOG_LEVEL="verbose"`}},
		{name: "log level with positive offset", env: map[string]string{"LOG_LEVEL": "info+2"}, want: []string{`LOG_LEVEL="info+2"`}},
		{name: "log level with negative offset", env: map[string]string{"LOG_LEVEL": "INFO-4"}, want: []string{`LOG_LEVEL="INFO-4"`}},
		{name: "log level warning is not warn", env: map[string]string{"LOG_LEVEL": "warning"}, want: []string{`LOG_LEVEL="warning"`}},
		{name: "unparsable timeout", env: map[string]string{"SHUTDOWN_TIMEOUT": "abc"}, want: []string{`SHUTDOWN_TIMEOUT="abc"`}},
		{name: "zero timeout", env: map[string]string{"SHUTDOWN_TIMEOUT": "0s"}, want: []string{`SHUTDOWN_TIMEOUT="0s"`}},
		{name: "negative timeout", env: map[string]string{"SHUTDOWN_TIMEOUT": "-5s"}, want: []string{`SHUTDOWN_TIMEOUT="-5s"`}},
		{
			name: "invalid port and invalid level are reported together",
			env:  map[string]string{"HTTP_ADDR": "127.0.0.1:99999", "LOG_LEVEL": "verbose"},
			want: []string{`HTTP_ADDR="127.0.0.1:99999"`, `LOG_LEVEL="verbose"`},
		},
		{
			name: "all problems are reported together",
			env:  map[string]string{"HTTP_ADDR": "semporta", "LOG_LEVEL": "verbose", "SHUTDOWN_TIMEOUT": "abc"},
			want: []string{"HTTP_ADDR", "LOG_LEVEL", "SHUTDOWN_TIMEOUT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(env(withURL(tt.env)))
			if err == nil {
				t.Fatalf("Load() = %+v, want an error", got)
			}
			if got != (Config{}) {
				t.Errorf("Load() config = %+v, want the zero Config on error", got)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestLoadDatabaseURLRequired(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "absent", env: nil},
		{name: "empty", env: map[string]string{"DATABASE_URL": ""}},
		{name: "blank", env: map[string]string{"DATABASE_URL": "  \t "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(env(tt.env))
			if err == nil {
				t.Fatalf("Load() = %+v, want an error", got)
			}
			if got != (Config{}) {
				t.Errorf("Load() config = %+v, want the zero Config on error", got)
			}
			if !strings.Contains(err.Error(), "DATABASE_URL is required") {
				t.Errorf("error %q does not mention that DATABASE_URL is required", err)
			}
		})
	}
}

func TestLoadMissingURLIsReportedWithOtherProblems(t *testing.T) {
	_, err := Load(env(map[string]string{"LOG_LEVEL": "verbose"}))
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	for _, w := range []string{"LOG_LEVEL", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q does not contain %q", err, w)
		}
	}
}

// DATABASE_URL carries the password. Errors about other variables must not
// leak it.
func TestLoadErrorsDoNotLeakDatabaseURL(t *testing.T) {
	_, err := Load(env(map[string]string{
		"DATABASE_URL":     testURL,
		"HTTP_ADDR":        "semporta",
		"LOG_LEVEL":        "verbose",
		"SHUTDOWN_TIMEOUT": "abc",
	}))
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	if strings.Contains(err.Error(), "s3cr3t") || strings.Contains(err.Error(), testURL) {
		t.Errorf("error leaks the DATABASE_URL value: %q", err)
	}
	if strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error mentions DATABASE_URL although it is valid: %q", err)
	}
}
