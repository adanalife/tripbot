package onscreensServer

import (
	"fmt"
	"log/slog"
	"net/http"
)

// Onscreens overlay commands (middle / leaderboard / timewarp / gps)
// arrive over NATS and are dispatched by the subscribers in nats.go. The
// HTTP server here serves only the browser-source feeds (state.json /
// render / asset), health, version, metrics, and admin.

// healthHandler is the liveness probe target.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

// catchAllHandler 404s unknown routes for visibility, whatever the method.
func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "404 not found", http.StatusNotFound)
	slog.InfoContext(r.Context(), "404 "+r.Method, "path", r.URL.Path)
}
