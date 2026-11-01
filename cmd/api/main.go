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

	"github.com/felipealvs21/url-shortener/internal/health"
)

// Hardcoded for now; task 1.4 replaces them with external configuration.
const (
	// localhost (not ":8080") so the server is not exposed to the local network.
	addr              = "localhost:8080"
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(context.Background(), logger); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// run starts the server and blocks until it fails or a shutdown signal arrives.
// Keeping the logic out of main lets it return errors instead of calling os.Exit
// in the middle, so deferred calls (like stop) always run.
func run(ctx context.Context, logger *slog.Logger) error {
	// ctx is cancelled when SIGINT (Ctrl+C) or SIGTERM (e.g. docker stop) arrives.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Handler:           newRouter(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		// Send net/http's internal errors through slog too, so all logs are structured.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	// Bind synchronously: if the port is taken we fail right here, and we only
	// log "listening" once it is really true.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
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
func newRouter() *http.ServeMux {
	mux := http.NewServeMux()
	// The "GET " prefix makes the mux answer 405 + Allow for other methods.
	mux.HandleFunc("GET /healthz", health.Liveness)
	return mux
}
