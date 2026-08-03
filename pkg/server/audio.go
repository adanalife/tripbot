package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/adanalife/tripbot/pkg/obs/beds"
)

// BedStore is the background-audio surface the console drives: read what's
// playing, switch to another bed, retune the SomaFM bed. Implemented by
// *beds.Store; an interface so the handlers test without an OBS WebSocket.
type BedStore interface {
	Current() (beds.Bed, string)
	Station() string
	Album() string
	Albums() []string
	ValidAlbum(album string) bool
	SomaFMTrack(ctx context.Context) (artist, title string, err error)
	Set(ctx context.Context, bed beds.Bed) error
	SetStation(ctx context.Context, station string) error
	SetAlbum(ctx context.Context, album string) error
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
	// The station list travels with the state so the console's picker is built
	// from what this tripbot will actually accept, the same way the bed buttons
	// are — nothing about SomaFM's lineup is duplicated in the console.
	body := map[string]any{"ok": false, "beds": options, "stations": beds.Stations}
	if s.beds != nil {
		bed, track := s.beds.Current()
		body["ok"] = true
		body["bed"] = string(bed)
		body["track"] = s.track(r.Context(), bed, track)
		body["station"] = s.beds.Station()
		// The album list ships for the same reason the stations do, but it's read
		// off the share per request rather than from a constant: new music appears
		// there without a deploy, so a picker built from a compiled-in list would
		// be wrong the first time Dana drops an album on the NAS.
		body["album"] = s.beds.Album()
		body["albums"] = s.beds.Albums()
	}
	_ = json.NewEncoder(w).Encode(body)
}

// track is the display line for whatever the live bed is playing: the album's
// filename, or the song SomaFM's feed reports for the tuned channel. "" when
// nothing knows (the car hum has no track, and a feed that won't answer is not
// worth an error — the console renders no line and re-asks on its next poll).
//
// The SomaFM read is here rather than in the console because the station lives
// here: one cached fetch behind this endpoint serves chat and every open tab,
// where a console-side fetch would need the station shipped to it first and
// would still be a second cache to keep honest.
func (s *Server) track(ctx context.Context, bed beds.Bed, albumTrack string) string {
	if bed != beds.SomaFM {
		return beds.TrackTitle(albumTrack)
	}
	artist, title, err := s.beds.SomaFMTrack(ctx)
	if err != nil {
		slog.DebugContext(ctx, "background audio: somafm now-playing unavailable", "err", err)
		return ""
	}
	if artist == "" || title == "" {
		return artist + title
	}
	return artist + " — " + title
}

// audioSetHandler switches the background-audio bed. The console POSTs
// {"bed": "album"} to /api/audio, {"station": "dronezone"} to tune the SomaFM
// bed to another channel, or {"album": "streambeats-lofi"} to narrow the album
// bed to one album — the latter two select their bed too, since tuning a station
// or picking an album you can't hear isn't a thing anyone means. A name we don't
// know is a 400; a
// switch OBS rejects (unreachable, or an album with no share mounted) is a 502
// — either way the previous bed keeps playing and the re-read reports it, so a
// failed switch is visible in the UI rather than silently ignored.
func (s *Server) audioSetHandler(w http.ResponseWriter, r *http.Request) {
	if s.beds == nil {
		http.Error(w, "background audio unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Bed     string `json:"bed"`
		Station string `json:"station"`
		Album   string `json:"album"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}

	var err error
	switch {
	case body.Station != "":
		if !beds.ValidStation(body.Station) {
			http.Error(w, "unknown somafm station", http.StatusBadRequest)
			return
		}
		err = s.beds.SetStation(r.Context(), body.Station)
	case body.Album != "":
		// Asked of the store rather than a list this handler re-derives: the store
		// owns the share, so its answer is the one that will load tracks. Checking
		// here anyway is what separates "no such album" (400, the console sent a
		// stale name) from "the share went away" (502).
		if !s.beds.ValidAlbum(body.Album) {
			http.Error(w, "unknown album", http.StatusBadRequest)
			return
		}
		err = s.beds.SetAlbum(r.Context(), body.Album)
	case beds.Valid(beds.Bed(body.Bed)):
		err = s.beds.Set(r.Context(), beds.Bed(body.Bed))
	default:
		http.Error(w, "unknown background-audio bed", http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.WarnContext(r.Context(), "background audio switch failed",
			"bed", body.Bed, "station", body.Station, "err", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	current, track := s.beds.Current()
	slog.InfoContext(r.Context(), "background audio switched via console",
		"bed", current, "station", s.beds.Station(), "album", s.beds.Album())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"bed":     string(current),
		"track":   s.track(r.Context(), current, track),
		"station": s.beds.Station(),
		"album":   s.beds.Album(),
	})
}
