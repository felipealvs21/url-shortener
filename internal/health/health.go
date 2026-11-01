// Package health exposes the endpoints that report the state of the service.
//
// There are two, and the difference matters. Liveness asks "is the process up
// and able to answer HTTP?" and never queries a dependency, so an orchestrator
// does not restart the service just because the database is down (a restart
// would not fix that). Readiness asks "can it serve real traffic right now?"
// and does check dependencies, so traffic can be held back until they recover.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// readinessTimeout bounds the dependency check, so a hung database makes
// /readyz answer 503 instead of hanging too.
const readinessTimeout = 2 * time.Second

// Pinger is anything that can confirm it is reachable. *pgxpool.Pool satisfies
// it implicitly. It is defined here, in the consumer, so Readiness can be
// tested with a fake instead of a real database.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Liveness answers 200 with {"status":"ok"}. It does not query anything.
func Liveness(w http.ResponseWriter, r *http.Request) {
	// Headers must be set before WriteHeader: after it, they are already sent.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Readiness returns a handler that pings p. It answers 200 with {"status":"ok"}
// when the ping succeeds and 503 with {"status":"unavailable"} when it fails.
// The error is logged but never sent to the client: it may expose internals.
func Readiness(p Pinger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Derived from the request context, so a client that gives up also
		// cancels the ping.
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := p.Ping(ctx); err != nil {
			logger.Warn("readiness check failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
