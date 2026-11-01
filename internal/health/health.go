// Package health exposes the endpoints that report the state of the service.
//
// For now there is only liveness: "is the process up and able to answer HTTP?".
// It deliberately checks nothing else, so an orchestrator never restarts the
// service just because a dependency (e.g. the database) is down. Readiness
// ("can it serve real traffic?") will be added later, when there are
// dependencies to check.
package health

import "net/http"

// Liveness answers 200 with {"status":"ok"}. It does not query anything.
func Liveness(w http.ResponseWriter, r *http.Request) {
	// Headers must be set before WriteHeader: after it, they are already sent.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
