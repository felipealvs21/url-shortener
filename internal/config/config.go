// Package config loads the application settings from environment variables.
//
// It reads only the process environment. It does not read a .env file: in
// development the Makefile loads .env and exports it, so the application sees
// the same variables it would see in any other environment.
//
// Only what changes between environments belongs here. Secrets (today only
// DATABASE_URL, which carries the database password) must never be logged, not
// even on a validation error: Load's error messages quote the received value of
// the non-secret variables, but never the value of a secret one.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr        = "127.0.0.1:8080"
	defaultShutdownTimeout = 10 * time.Second
)

// Config holds the validated application settings.
type Config struct {
	// HTTPAddr is the host:port the HTTP server listens on.
	HTTPAddr string
	// LogLevel is the minimum severity that is logged.
	LogLevel slog.Level
	// ShutdownTimeout is how long in-flight requests get to finish on shutdown.
	ShutdownTimeout time.Duration
	// DatabaseURL is the PostgreSQL connection URL. It is a SECRET (it contains
	// the password): never log it, and never log the whole Config struct.
	DatabaseURL string
}

// Load builds a Config from the variables HTTP_ADDR, LOG_LEVEL,
// SHUTDOWN_TIMEOUT and DATABASE_URL. The first three fall back to defaults when
// unset or blank; DATABASE_URL is required and has no default.
//
// It takes getenv as a parameter instead of calling os.Getenv so tests can
// pass a map and never touch (or depend on) the real process environment.
// Production code passes os.Getenv.
//
// Every variable is validated and all problems are returned together, so a
// misconfigured deployment can be fixed in one attempt. On error, Load returns
// the zero Config.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	cfg := Config{
		HTTPAddr:        defaultHTTPAddr,
		LogLevel:        slog.LevelInfo,
		ShutdownTimeout: defaultShutdownTimeout,
	}

	if v := strings.TrimSpace(getenv("HTTP_ADDR")); v != "" {
		if validAddr(v) {
			cfg.HTTPAddr = v
		} else {
			errs = append(errs, fmt.Errorf(`HTTP_ADDR=%q: want host:port with a numeric port 1-65535, e.g. "127.0.0.1:8080"`, v))
		}
	}

	if v := strings.TrimSpace(getenv("LOG_LEVEL")); v != "" {
		if level, ok := parseLevel(v); ok {
			cfg.LogLevel = level
		} else {
			errs = append(errs, fmt.Errorf("LOG_LEVEL=%q: want one of debug, info, warn, error", v))
		}
	}

	if v := strings.TrimSpace(getenv("SHUTDOWN_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf(`SHUTDOWN_TIMEOUT=%q: want a positive duration, e.g. "10s" or "1m"`, v))
		} else {
			cfg.ShutdownTimeout = d
		}
	}

	// Only the presence of DATABASE_URL is checked here. Parsing it is the job
	// of internal/postgres, which owns the pgx dependency; config stays free of
	// it. The value is never put in an error message because it is a secret.
	if v := strings.TrimSpace(getenv("DATABASE_URL")); v != "" {
		cfg.DatabaseURL = v
	} else {
		errs = append(errs, errors.New("DATABASE_URL is required, e.g. postgres://user:password@host:5432/dbname"))
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validAddr reports whether addr is "host:port" with a numeric port in 1-65535.
//
// The host is deliberately not validated or resolved here: resolving DNS while
// loading configuration would be a side effect, and a DNS failure can be
// transient. An unusable host is still reported, by net.Listen at startup.
func validAddr(addr string) bool {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	// ParseUint (not Atoi) so signs such as "-1" or "+80" are rejected too.
	n, err := strconv.ParseUint(port, 10, 16)
	return err == nil && n >= 1
}

// parseLevel maps exactly debug, info, warn and error (case-insensitive) to a
// slog level. It does not use slog.Level.UnmarshalText: that parser accepts a
// larger grammar than our contract (offsets such as "info+2").
func parseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return 0, false
}
