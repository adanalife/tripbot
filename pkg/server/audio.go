package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/adanalife/tripbot/pkg/obs/beds"
)

// BedStore is the background-audio surface the console drives: read what's
// playing, switch to another bed. Implemented by *beds.Store; an interface so
// the handlers test without an OBS WebSocket.
type BedStore interface {
	Current() (beds.Bed, string)
	Set(ctx context.Context, bed beds.Bed) error
}

// audioHandler reports the live background-audio bed and the options the
// console can switch to. Reports ok=false with no bed when no store is wired
// (a tripbot running without an OBS pairing), which the console renders as
// "unavailable" rather than an error.
func (s *Server) audioHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	options := make([]string, 0, len(beds.All))
	for _, b := range beds.All {
		options = append(options, string(b))
	}
	if s.beds == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "beds": options})
		return
	}
	bed, track := s.beds.Current()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"bed":   string(bed),
		"beds":  options,
		"track": beds.TrackTitle(track),
	})
}

// audioSetHandler switches the background-audio bed. The console POSTs
// {"bed": "album"} to /api/audio. A bed name we don't know is a 400; a switch
// OBS rejects (unreachable, or an album with no share mounted) is a 502 —
// either way the previous bed keeps playing and the re-read reports it, so a
// failed switch is visible in the UI rather than silently ignored.
func (s *Server) audioSetHandler(w http.ResponseWriter, r *http.Request) {
	if s.beds == nil {
		http.Error(w, "background audio unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Bed string `json:"bed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	bed := beds.Bed(body.Bed)
	if !beds.Valid(bed) {
		http.Error(w, "unknown background-audio bed", http.StatusBadRequest)
		return
	}
	if err := s.beds.Set(r.Context(), bed); err != nil {
		slog.WarnContext(r.Context(), "background audio switch failed", "bed", bed, "err", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	slog.InfoContext(r.Context(), "background audio switched via console", "bed", bed)
	current, track := s.beds.Current()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"bed":   string(current),
		"track": beds.TrackTitle(track),
	})
}
