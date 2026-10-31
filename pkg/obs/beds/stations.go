package beds

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
)

// Station is one internet-radio stream the radio bed can play: an id the
// console and chat select it by, a name for humans, and the Icecast/Shoutcast
// URL OBS's ffmpeg_source plays. The audio watchdog probes the same URL, so
// what it checks is exactly what OBS plays.
//
// A Private station is listed but never selectable as the stream's bed: it is
// for listening at home (the native client reads the list through GET
// /api/audio) and plays music that isn't cleared for broadcast.
type Station struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Private bool   `json:"private,omitempty"`
}

// RadioStationsFile is where cdk8s mounts the station list, a Secret
// materialized from SSM so the stations stay out of the repo. Optional, like
// MusicIndexFile: a missing one leaves the radio bed empty, not the pod
// unschedulable.
const RadioStationsFile = "/opt/tripbot/assets/radio/stations.json"

// Stations is the radio bed's station list in display order, set once at
// startup by LoadStations. Empty means the radio bed has nothing to play.
var Stations []Station

// LoadStations reads the station list from path into Stations. A missing,
// empty or invalid file logs one warning and leaves the radio bed without
// stations; the other beds don't depend on it, so it never fails startup.
func LoadStations(path string) {
	stations, err := readStations(path)
	if err != nil {
		slog.Warn("background audio: radio bed has no stations", "path", path, "err", err)
	}
	Stations = stations
}

func readStations(path string) ([]Station, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var stations []Station
	if err := json.Unmarshal(raw, &stations); err != nil {
		return nil, fmt.Errorf("parse station list: %w", err)
	}
	if len(stations) == 0 {
		return nil, errors.New("station list is empty")
	}
	seen := map[string]bool{}
	for i, s := range stations {
		if s.ID == "" || seen[s.ID] {
			return nil, fmt.Errorf("station %d: missing or duplicate id %q", i, s.ID)
		}
		if u, err := url.Parse(s.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("station %q: url %q is not an http(s) stream", s.ID, s.URL)
		}
		if s.Name == "" {
			stations[i].Name = s.ID
		}
		seen[s.ID] = true
	}
	return stations, nil
}

// DefaultStation is the station the radio bed plays when nothing has chosen
// one: the first selectable one in the list, or "" when there are none.
func DefaultStation() string {
	for _, s := range Stations {
		if !s.Private {
			return s.ID
		}
	}
	return ""
}

func station(id string) (Station, bool) {
	for _, s := range Stations {
		if s.ID == id {
			return s, true
		}
	}
	return Station{}, false
}

// StreamURL is the stream URL OBS plays for a station, or "" for one not in
// the list.
func StreamURL(id string) string {
	s, _ := station(id)
	return s.URL
}

// stationFromURL recovers the station id from the URL OBS is playing, so the
// Store can read the live station back off OBS at startup rather than guessing.
// Only an exact match counts; an unrecognized URL yields "".
func stationFromURL(u string) string {
	for _, s := range Stations {
		if s.URL == u {
			return s.ID
		}
	}
	return ""
}

// SelectableStation reports whether id is a station in the list that the
// stream may play: in the list and not private.
func SelectableStation(id string) bool {
	s, ok := station(id)
	return ok && !s.Private
}

// StationName is the human name for a station id, falling back to the id for
// one we don't know.
func StationName(id string) string {
	if s, ok := station(id); ok {
		return s.Name
	}
	return id
}
