// Package httpmw holds HTTP middleware shared across tripbot's web servers
// (tripbot, vlc-server, onscreens-server).
package httpmw

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/urfave/negroni/v3"
)

// debugPaths are logged at Debug instead of Info because they're polled
// often enough to dominate the default log stream. Kubelet's liveness and
// readiness probes hit /health/{live,ready} every few seconds; OBS's CEF
// browser sources poll /onscreens/state.json at ~14 req/sec idle. Other
// frequent paths (e.g. /metrics) stay at Info — add them here if they
// become noisy.
var debugPaths = map[string]bool{
	"/health/live":          true,
	"/health/ready":         true,
	"/onscreens/state.json": true,
}

// SlogLogger is a negroni-compatible middleware that emits one slog
// record per HTTP request. It replaces negroni.Logger so request logs
// flow through the project's slog handlers (console + OTel + Sentry)
// and structured fields land cleanly in Loki.
type SlogLogger struct{}

// NewSlogLogger constructs a SlogLogger.
func NewSlogLogger() *SlogLogger { return &SlogLogger{} }

// ServeHTTP implements negroni.Handler.
func (l *SlogLogger) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	next(rw, r)

	status := 0
	size := 0
	if res, ok := rw.(negroni.ResponseWriter); ok {
		status = res.Status()
		size = res.Size()
	}

	level := slog.LevelInfo
	if debugPaths[r.URL.Path] {
		level = slog.LevelDebug
	}
	slog.LogAttrs(r.Context(), level, "http request",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Int("status", status),
		slog.Int("bytes", size),
		slog.Duration("duration", time.Since(start)),
		slog.String("remote", r.RemoteAddr),
	)
}
