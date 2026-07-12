package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/adanalife/tripbot/pkg/feature"
)

// SetVersion lets cmd/tripbot inject its build-time version string
// before the HTTP server starts.
func (s *Server) SetVersion(v string) {
	if v != "" {
		s.versionTag = v
	}
}

// SetFlags injects the feature-flag client backing the console's /api/flags
// endpoints. Called from cmd/tripbot's startFeatureFlags, before Start runs
// the HTTP server (so there's no race on the field). Left nil when the
// Postgres client fails to load — /api/flags then reports ok=false.
func (s *Server) SetFlags(fc feature.FlagClient) {
	s.flags = fc
}

// startedAt is when the process began; /version reports it so callers can derive
// uptime. (The bot's chat-connection state is surfaced separately via the
// tripbot_twitch_connected gauge, set directly by cmd/tripbot.)
var startedAt = time.Now()

// versionHandler returns build metadata as JSON. The tag comes from the
// build-time ldflag; sha + built_at are read from the binary's embedded
// VCS info (Go's automatic -buildvcs). started_at is the process start time
// (startedAt) so callers can derive uptime themselves.
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Tag       string `json:"tag"`
		Sha       string `json:"sha"`
		BuiltAt   string `json:"built_at"`
		StartedAt string `json:"started_at"`
	}{Tag: s.versionTag, StartedAt: startedAt.UTC().Format(time.RFC3339)}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				resp.Sha = s.Value
			case "vcs.time":
				resp.BuiltAt = s.Value
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode version response", "err", err)
	}
}

// return a favicon if anyone asks for one
func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "assets/favicon.ico")
}

func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		http.Error(w, "404 not found", http.StatusNotFound)
		slog.InfoContext(r.Context(), "404 GET", "path", r.URL.Path)
		return

	case "POST":
		// someone tried to make a post and we dont know what to do with it
		http.Error(w, "404 not found", http.StatusNotFound)
		slog.InfoContext(r.Context(), "404 POST", "path", r.URL.Path)
		return
	// someone tried a PUT or a DELETE or something
	default:
		fmt.Fprintf(w, "Only GET/POST methods are supported.\n")
	}
}
