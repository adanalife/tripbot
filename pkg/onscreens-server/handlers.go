package onscreensServer

import (
	"fmt"
	"log/slog"
	"net/http"
)

// Onscreens overlay commands (middle / leaderboard / timewarp / gps / flag)
// no longer have HTTP handlers — they arrive over NATS and are dispatched by
// the subscribers in nats.go. The HTTP server here serves only the
// browser-source feeds (state.json / render / asset), health, version,
// metrics, and admin.

// healthHandler is the liveness probe target.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

// catchAllHandler 404s unknown routes for visibility.
func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		http.Error(w, "404 not found", http.StatusNotFound)
		slog.InfoContext(r.Context(), "404 GET", "path", r.URL.Path)
		return
	default:
		fmt.Fprintf(w, "Only GET methods are supported.\n")
	}
}
