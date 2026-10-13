package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// Playhead is where this instance's stream is: the clip on screen and how far
// into it. *video.Player implements it.
type Playhead interface {
	Playhead(ctx context.Context) (video.Video, time.Duration)
}

// clipTrack is the data seam the playhead endpoint reads the track through,
// overridable in tests so the handler renders without a DB.
var clipTrack = video.Track

// SetPlayhead injects the player behind GET /api/map/playhead. Called from
// cmd/tripbot before Start runs the HTTP server (so there's no race on the
// field). Left nil, the endpoint answers 503.
func (s *Server) SetPlayhead(p Playhead) {
	s.playhead = p
}

// playheadResponse is the clip on screen, the playhead's offset into it, and
// its track as [sec, lat, lng] triples in clip order. Track is empty for a
// clip with no believable one, and the console keeps its clip-level pin.
type playheadResponse struct {
	File    string       `json:"file"`
	OffsetS float64      `json:"offset_s"`
	Track   [][3]float64 `json:"track"`
}

// mapPlayheadHandler serves GET /api/map/playhead for the console's map, which
// walks its "now playing" pin along the track from the offset rather than
// dead-reckoning a straight line off the clip's opening heading.
//
// It answers per instance, since each platform's playout runs its own clip,
// and it reads the playhead off playout rather than off the video.changed
// timestamp: the cron notices a clip switch up to a minute late, which at
// highway speed puts a pin timed from emitted_at a mile and a half behind.
func (s *Server) mapPlayheadHandler(w http.ResponseWriter, r *http.Request) {
	if s.playhead == nil {
		http.Error(w, "playhead unavailable", http.StatusServiceUnavailable)
		return
	}
	vid, at := s.playhead.Playhead(r.Context())
	resp := playheadResponse{File: vid.File(), OffsetS: at.Seconds(), Track: [][3]float64{}}
	for _, p := range clipTrack(r.Context(), vid) {
		resp.Track = append(resp.Track, [3]float64{p.Sec, p.Lat, p.Lng})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(r.Context(), "couldn't write playhead", "err", err)
	}
}
