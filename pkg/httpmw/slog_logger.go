// Package httpmw holds HTTP middleware shared across tripbot's web servers
// (tripbot, vlc-server, onscreens-server).
package httpmw

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/urfave/negroni/v3"
)

// healthPaths are logged at Debug instead of Info so kubelet's liveness
// and readiness probes don't dominate the default log stream. Other
// frequent paths (e.g. /metrics) stay at Info — add them here if they
// become noisy.
var healthPaths = map[string]bool{
	"/health/live":  true,
	"/health/ready": true,
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
	if healthPaths[r.URL.Path] {
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
