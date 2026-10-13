package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/gorilla/mux"
)

// FeatureFlags converts a flag client's snapshot to the wire shape, shared by
// GET /api/flags and the flags.snapshot eventbus subject. It exposes the
// targeting allowlists and the target-removal date for read-only display — the
// console can only flip the global default (the FlagToggler write surface), not
// edit allowlists.
func FeatureFlags(snap []feature.Flag) []eventbus.FeatureFlag {
	out := make([]eventbus.FeatureFlag, 0, len(snap))
	for _, f := range snap {
		ff := eventbus.FeatureFlag{
			Key:                 f.Key,
			Description:         f.Description,
			Enabled:             f.Enabled,
			EnabledForUsernames: f.EnabledForUsernames,
			EnabledForRoles:     f.EnabledForRoles,
		}
		if !f.TargetRemovalDate.IsZero() {
			ff.TargetRemovalDate = f.TargetRemovalDate.UTC().Format(time.RFC3339)
		}
		out = append(out, ff)
	}
	return out
}

// flagsHandler serves the current feature-flag snapshot as JSON for the
// standalone console (which holds no DB access of its own). Reports ok=false
// with an empty list when no flag client is wired — the in-memory fallback
// window before the Postgres-backed client loads.
func (s *Server) flagsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.flags == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "flags": []eventbus.FeatureFlag{}})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "flags": FeatureFlags(s.flags.Snapshot(r.Context()))})
}

// flagToggleHandler flips a flag's global-default enabled state. The console
// POSTs {"enabled": bool} to /api/flags/{key}. Requires the wired client to
// implement feature.FlagToggler (the Postgres client does; the in-memory
// fallback doesn't), so a toggle before the DB client loads returns 503. An
// unknown key returns 404 (SetEnabled errors). Internal-only, like the rest of
// the /api surface — the console reaches it over the in-namespace Service.
func (s *Server) flagToggleHandler(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	toggler, ok := s.flags.(feature.FlagToggler)
	if !ok {
		http.Error(w, "flag toggling unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if err := toggler.SetEnabled(r.Context(), key, body.Enabled); err != nil {
		slog.WarnContext(r.Context(), "feature flag toggle failed", "flag", key, "err", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	slog.InfoContext(r.Context(), "feature flag toggled via console",
		"flag", key, "enabled", body.Enabled)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "key": key, "enabled": body.Enabled})
}
