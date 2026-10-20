package httpmw

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// startedAt marks process start so /version can report uptime. Set at
// package load; close enough to process start for a human-readable "up Xh".
var startedAt = time.Now()

// WriteVersion writes build metadata as JSON. tag is the build-time ldflag
// the binary was given; sha + built_at are read from the binary's embedded
// VCS info (Go's automatic -buildvcs). started_at is the process start time
// so callers can derive uptime themselves.
func WriteVersion(w http.ResponseWriter, r *http.Request, tag string) {
	resp := struct {
		Tag       string `json:"tag"`
		Sha       string `json:"sha"`
		BuiltAt   string `json:"built_at"`
		StartedAt string `json:"started_at"`
	}{Tag: tag, StartedAt: startedAt.UTC().Format(time.RFC3339)}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, set := range info.Settings {
			switch set.Key {
			case "vcs.revision":
				resp.Sha = set.Value
			case "vcs.time":
				resp.BuiltAt = set.Value
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode version response", "err", err)
	}
}
