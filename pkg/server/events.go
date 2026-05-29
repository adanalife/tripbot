package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// sseHeartbeat is how often the SSE handler emits a comment line — keeps
// intermediaries (Traefik, Tailscale) from idling out the long-lived
// connection, and surfaces a dead client via the Flush error between events.
const sseHeartbeat = 20 * time.Second

// eventsHandler streams live-console events to the browser over Server-Sent
// Events. HTMX's sse extension connects here (sse-connect="/admin/events") and
// routes each named event to its sse-swap target.
func eventsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := http.NewResponseController(w)

	// Ordinary handlers run under the server's 15s WriteTimeout; an SSE stream
	// is long-lived, so clear the write deadline for this connection. This
	// reaches the underlying writer through the negroni + otelhttp wrappers
	// because each implements Unwrap() (do NOT type-assert http.Flusher —
	// negroni's writer fails that assertion). Best-effort: if a wrapper doesn't
	// support deadlines the connection still works, it just gets recycled at the
	// 15s WriteTimeout — and the browser's EventSource auto-reconnects.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		slog.WarnContext(ctx, "sse: could not clear write deadline; stream will recycle at WriteTimeout", "err", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// Defeat proxy response buffering so events flush promptly end-to-end.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return
	}

	ch := eventHub.register()
	defer eventHub.unregister(ch)

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return // hub closed (process shutdown)
			}
			// Flatten any stray newline so it can't break SSE framing — each
			// data field must be a single line.
			data := strings.ReplaceAll(ev.Data, "\n", " ")
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Name, data); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
