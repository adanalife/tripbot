package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/obs/beds"
)

// BedStore is the background-audio surface the console drives: read what's
// playing, switch to another bed, retune the radio bed. Implemented by
// *beds.Store; an interface so the handlers test without an OBS WebSocket.
type BedStore interface {
	Current() (beds.Bed, string)
	Playing() (beds.Bed, string)
	Station() string
	Voicing() string
	PlayingAlbum() string
	RadioTrack(ctx context.Context) (artist, title string, err error)
	Pending() (beds.Switch, bool)
	Set(ctx context.Context, bed beds.Bed) error
	SetStation(ctx context.Context, station string) error
	SetVoicing(ctx context.Context, voicing string) error
}

// audioHandler reports the live background-audio bed and the options the
// console can switch to. Reports ok=false with no bed when no store is wired
// (a tripbot running without an OBS pairing), which the console renders as
// "unavailable" rather than an error.
func (s *Server) audioHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store := s.bedStore()
	options := make([]string, 0, len(beds.All))
	for _, b := range beds.All {
		options = append(options, string(b))
	}
	// The station list travels with the state so the console's picker is built
	// from what this tripbot will actually accept, the same way the bed buttons
	// are — nothing about the radio's lineup is duplicated in the console.
	// Private stations ship too, flagged, so a client can list them for home
	// listening; the picker offers only the unflagged ones.
	// The voicings travel for the same reason the stations do: the renders are
	// baked into the OBS image this tripbot is paired with, so the picker is
	// built from what will actually play rather than a list the console repeats.
	body := map[string]any{"ok": false, "beds": options, "stations": beds.StationList(), "voicings": beds.Voicings}
	if store != nil {
		bed, _ := store.Current()
		playing, track := store.Playing()
		body["ok"] = true
		body["bed"] = string(bed)
		// The picker highlights the selected bed, but the now-playing line has to
		// name what's audible: mid-outage the watchdog has another bed on air and
		// the selection still reads the radio, whose feed would name a track nobody
		// is hearing.
		body["on_fallback"] = playing != bed
		body["track"] = s.track(r.Context(), playing, track)
		body["station"] = store.Station()
		body["voicing"] = store.Voicing()
		// The album fallback spans several albums, so the one the current track
		// sits in ships for the console's now-playing line.
		body["playing_album"] = store.PlayingAlbum()
		// A switch waits a few seconds before it reaches OBS, so the fields above
		// still describe the audio while a click is in flight. Ship the waiting one
		// separately: without it the console shows the old state and reads as a
		// button that did nothing. Absent when nothing is waiting.
		if sw, ok := store.Pending(); ok {
			body["pending"] = map[string]any{
				"bed":     string(sw.Bed),
				"station": sw.Station,
				"voicing": sw.Voicing,
				// Seconds left rather than a timestamp: the console renders a
				// countdown, and clock skew between the two would show in it.
				"in_seconds": max(0, int(time.Until(sw.At).Round(time.Second).Seconds())),
			}
		}
	}
	_ = json.NewEncoder(w).Encode(body)
}

// track is the display line for whatever the live bed is playing: the fallback
// album track's title, or the song the station's metadata reports for the tuned channel. "" when
// nothing knows (the car hum has no track, and a feed that won't answer is not
// worth an error — the console renders no line and re-asks on its next poll).
//
// The radio read is here rather than in the console because the station lives
// here: one cached fetch behind this endpoint serves chat and every open tab,
// where a console-side fetch would need the station shipped to it first and
// would still be a second cache to keep honest.
func (s *Server) track(ctx context.Context, bed beds.Bed, albumTrack string) string {
	if bed != beds.Radio {
		return beds.TrackTitle(albumTrack)
	}
	artist, title, err := s.bedStore().RadioTrack(ctx)
	if err != nil {
		slog.DebugContext(ctx, "background audio: radio now-playing unavailable", "err", err)
		return ""
	}
	if artist == "" || title == "" {
		return artist + title
	}
	return artist + " — " + title
}

// audioSetHandler switches the background-audio bed. The console POSTs
// {"bed": "carhum"} to /api/audio, {"station": "lofi"} to tune the radio bed to
// another channel, or {"voicing": "highway"} to pick which car-hum render the
// drone plays — the latter two select their bed too, since tuning a station you
// can't hear isn't a thing anyone means. A name we don't know (or a private
// station) is a 400; a switch OBS rejects is a 502 — either way the previous
// bed keeps playing and the re-read reports it, so a failed switch is visible
// in the UI rather than silently ignored.
func (s *Server) audioSetHandler(w http.ResponseWriter, r *http.Request) {
	store := s.bedStore()
	if store == nil {
		http.Error(w, "background audio unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Bed     string `json:"bed"`
		Station string `json:"station"`
		Voicing string `json:"voicing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}

	var err error
	switch {
	case body.Station != "":
		if !beds.SelectableStation(body.Station) {
			http.Error(w, "unknown or private radio station", http.StatusBadRequest)
			return
		}
		err = store.SetStation(r.Context(), body.Station)
	case body.Voicing != "":
		if !beds.ValidVoicing(body.Voicing) {
			http.Error(w, "unknown car-hum voicing", http.StatusBadRequest)
			return
		}
		err = store.SetVoicing(r.Context(), body.Voicing)
	case beds.Valid(beds.Bed(body.Bed)):
		err = store.Set(r.Context(), beds.Bed(body.Bed))
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

	current, _ := store.Current()
	playing, track := store.Playing()
	slog.InfoContext(r.Context(), "background audio switched via console",
		"bed", current, "station", store.Station(), "voicing", store.Voicing())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":            true,
		"bed":           string(current),
		"track":         s.track(r.Context(), playing, track),
		"station":       store.Station(),
		"voicing":       store.Voicing(),
		"playing_album": store.PlayingAlbum(),
	})
}
