package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/events"
)

// Caps on the console-action audit fields — generous for any real action,
// tight enough that a runaway client can't stuff documents into the events
// table.
const (
	maxConsoleActionLen = 128
	maxConsoleTargetLen = 128
	maxConsoleDetailLen = 512
)

// consoleActionHandler accepts the standalone console's report of one
// successful admin mutation and lands it in the permanent events log as a
// console_action event. The report is fire-and-forget from the console's side
// (auditing must never fail or slow the action it describes), so success is a
// bodyless 204. Internal-only, like the rest of the /api surface.
func (s *Server) consoleActionHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
		Target string `json:"target"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(body.Action)
	target := strings.TrimSpace(body.Target)
	detail := strings.TrimSpace(body.Detail)
	switch {
	case action == "" || target == "":
		http.Error(w, "action and target are required", http.StatusBadRequest)
		return
	case len(action) > maxConsoleActionLen || len(target) > maxConsoleTargetLen || len(detail) > maxConsoleDetailLen:
		http.Error(w, "field too long", http.StatusBadRequest)
		return
	}
	switch err := events.ConsoleAction(r.Context(), s.cfg, action, target, detail); {
	case errors.Is(err, terrors.ErrReadOnly):
		// events.record refuses every write on a read-only instance. Dropping
		// the audit row is that policy working, not a client error the console
		// could act on, so it still answers 204.
	case err != nil:
		slog.ErrorContext(r.Context(), "error recording console action", "err", err,
			"action", action, "target", target)
		http.Error(w, "could not record event", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
