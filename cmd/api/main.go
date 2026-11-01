// Command api runs the URL shortener HTTP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/felipealvs21/url-shortener/internal/config"
	"github.com/felipealvs21/url-shortener/internal/health"
	"github.com/felipealvs21/url-shortener/internal/postgres"
)

// Server timeouts do not change between environments, so they are constants
// rather than configuration.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	// The logger must exist before the configuration is loaded (so that config
	// errors can be logged), but the configuration is what defines the log
	// level. A LevelVar breaks the cycle: the level is adjusted after Load.
	var level slog.LevelVar // starts at Info
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &level}))

	if err := run(context.Background(), logger, &level, os.Getenv); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// run starts the server and blocks until it fails or a shutdown signal arrives.
// Keeping the logic out of main lets it return errors instead of calling os.Exit
// in the middle, so deferred calls (like stop) always run.
func run(ctx context.Context, logger *slog.Logger, level *slog.LevelVar, getenv func(string) string) error {
	// Configuration comes first: an invalid one must fail before any signal
	// handler is installed or any port is opened.
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	level.Set(cfg.LogLevel)
	logger.Info("config loaded",
		"addr", cfg.HTTPAddr,
		"log_level", cfg.LogLevel.String(),
		"shutdown_timeout", cfg.ShutdownTimeout.String(),
	)

	// ctx is cancelled when SIGINT (Ctrl+C) or SIGTERM (e.g. docker stop) arrives.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Opened after NotifyContext so that Ctrl+C while waiting for the database
	// cancels the startup. If the database is down, we fail here, before any
	// port is opened (a project decision: fail fast).
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	// Deferred calls run when run returns, which is after srv.Shutdown has
	// waited for the in-flight requests (they may still be using the database).
	// So the pool is closed last, never under a running request.
	defer pool.Close()
	// Only host and database: never the user, the password or the full URL.
	connConfig := pool.Config().ConnConfig
	logger.Info("database connected", "host", connConfig.Host, "database", connConfig.Database)

	srv := &http.Server{
		Handler:           newRouter(pool, logger),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		// Send net/http's internal errors through slog too, so all logs are structured.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	// Bind synchronously: if the port is taken we fail right here, and we only
	// log "listening" once it is really true.
	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	logger.Info("server listening", "addr", ln.Addr().String())

	// Buffer of 1: if run returns through the ctx.Done() branch nobody reads
	// this channel, and the goroutine must still be able to send and exit.
	serveErr := make(chan error, 1)
	go func() {
		// Serve always returns a non-nil error; ErrServerClosed just means
		// Shutdown was called, which is the normal way to stop.
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	// Restore default signal behavior: a second Ctrl+C now kills the process
	// immediately instead of waiting for the graceful shutdown.
	stop()
	logger.Info("shutting down")

	// A fresh context is needed: ctx is already cancelled, and Shutdown would
	// give up instantly with it.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	// Shutdown closes the listener and waits for in-flight requests to finish.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

// newRouter is separate from run so the route table can be tested without
// starting a server.
func newRouter(db health.Pinger, logger *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	// The "GET " prefix makes the mux answer 405 + Allow for other methods.
	mux.HandleFunc("GET /healthz", health.Liveness)
	mux.HandleFunc("GET /readyz", health.Readiness(db, logger))
	return mux
}
