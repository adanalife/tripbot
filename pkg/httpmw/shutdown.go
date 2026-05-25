package httpmw

import (
	"log/slog"
	"net/http"
	"os"
	"syscall"
	"time"
)

// ShutdownSignal sends SIGTERM to the current process. Exposed as a package
// variable so tests can swap it for a no-op fake. The default lands the
// signal on the same handler each cmd/* main wires via signal.NotifyContext,
// which runs the graceful shutdown chain (HTTP drain, telemetry flush,
// Sentry flush) and exits 0. k8s's restartPolicy: Always brings the pod
// back — the admin panel's "restart" surface, no kube-API needed.
var ShutdownSignal = func() error {
	return syscall.Kill(os.Getpid(), syscall.SIGTERM)
}

// ShutdownDelay is the gap between responding 202 and firing the signal.
// The response needs to leave the wire before the process starts dying;
// shutting down inline would error the in-flight write before the client
// sees the 202. Exposed so tests don't sleep half a second per case.
var ShutdownDelay = 500 * time.Millisecond

// ShutdownHandler returns the POST /admin/shutdown handler shared across
// tripbot's Go servers (tripbot, vlc-server, onscreens-server). Responds
// 202 + a short body, then schedules SIGTERM after ShutdownDelay.
//
// The handler is intentionally minimal: no body parsing, no auth check
// (tailnet-only by Ingress per the admin-panel discussion in CLAUDE.md +
// vault/decisions). If/when the panel reaches beyond the tailnet, the
// auth gate goes on the route registration, not here.
func ShutdownHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.WarnContext(r.Context(), "admin shutdown requested — SIGTERMing self", "delay", ShutdownDelay)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("shutting down\n"))

		go func() {
			time.Sleep(ShutdownDelay)
			if err := ShutdownSignal(); err != nil {
				slog.ErrorContext(r.Context(), "admin shutdown signal failed", "err", err)
			}
		}()
	}
}
