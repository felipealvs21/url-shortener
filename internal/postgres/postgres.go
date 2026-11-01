// Package postgres opens the PostgreSQL connection pool.
//
// It only knows how to connect. Queries and repositories do not belong here:
// they arrive in Stage 2, together with sqlc.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// connectTimeout bounds the startup connection check in Open.
const connectTimeout = 5 * time.Second

// Open creates a connection pool for databaseURL and confirms that the database
// answers. The caller owns the returned pool and must Close it.
//
// databaseURL must be a postgres:// or postgresql:// URL with host, user and
// database name; the keyword/value format is not accepted. Special characters
// in the user and password must be percent-encoded. Fields that are absent
// (password, sslmode...) may still be completed by pgx from the PG* environment
// variables (PGPASSWORD, PGSSLMODE...) and ~/.pgpass, the standard libpq
// behavior, which we do not turn off.
//
// databaseURL is a secret (it carries the password), so no error returned by
// the validation of the URL contains it, nor the text of a third-party parser
// error. Connection errors do wrap pgx's message: it shows user, database and
// host:port, which are not secret and are needed to diagnose a failure.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := parseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	// NewWithConfig is lazy: it validates the config but does not connect.
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Because the pool is lazy, a wrong URL or a stopped database would only
	// show up on the first request. Pinging here makes the process fail fast at
	// startup instead (a project decision).
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		// The pool already holds background resources; close it so a failed
		// Open does not leak them.
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// parseConfig validates databaseURL against our contract and then lets pgx
// parse it. Every error it returns is a fixed message: none contains the URL,
// any part of it, or the text of an error from net/url or pgx.
//
// In particular, the *url.Error returned by url.Parse must never be wrapped or
// printed: it quotes the raw URL, password included.
func parseConfig(databaseURL string) (*pgxpool.Config, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return nil, errors.New("parse DATABASE_URL: not a valid URL (no quotes; percent-encode special characters in the password)")
	}
	// Rejecting every other scheme also rejects the keyword/value format. The
	// raw prefix is checked, not u.Scheme: net/url lowercases the scheme, but
	// pgx only recognizes the lowercase prefix, so POSTGRES:// would be read
	// as keyword/value and fail later with a misleading message.
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return nil, errors.New("parse DATABASE_URL: scheme must be postgres:// or postgresql://")
	}
	if u.Fragment != "" || strings.Contains(databaseURL, "#") {
		return nil, errors.New("parse DATABASE_URL: contains '#'; percent-encode it (%23)")
	}
	var user string
	if u.User != nil {
		user = u.User.Username()
	}
	host := u.Hostname()
	database := strings.TrimPrefix(u.Path, "/")
	if host == "" || user == "" || database == "" {
		return nil, errors.New("parse DATABASE_URL: host, user and database name are required")
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// The text of pgx's error is deliberately dropped. pgx masks the
		// password in it, but only when it understands the URL; we do not
		// depend on that.
		return nil, errors.New("parse DATABASE_URL: invalid port or connection parameter")
	}

	// net/url and pgx split a URL differently when the password has a raw
	// special character (@ # / ...), and the leftover of the password lands in
	// a field that pgx prints in connection errors. The two parsers disagree
	// exactly in that case, so a mismatch means an ambiguous URL.
	cc := cfg.ConnConfig
	if cc.Host != host || cc.User != user || cc.Database != database {
		return nil, errors.New("parse DATABASE_URL: ambiguous URL; percent-encode special characters (@ : / ? # space) in the user and password")
	}
	return cfg, nil
}
