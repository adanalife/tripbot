package beds

import (
	"strings"
)

// Station is one SomaFM channel: the id SomaFM keys it by, and the name it
// publishes for humans.
type Station struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DefaultStation is the channel the SomaFM bed plays when nothing has chosen
// one. Matches the `input` URL baked into the obs repo's scene config, so a
// fresh OBS container and a fresh Store agree on what's on air.
const DefaultStation = "gsclassic"

// SomaFM serves every channel at one predictable pair of URLs, keyed by the
// channel id — which is what makes a station just an id rather than a row of
// per-channel URLs.
//
// streamHost is the round-robin hostname (several ICEcast edges behind one
// name) so DNS hands out a healthy edge instead of pinning a dead one. 128k mp3
// is the one bitrate every channel offers, and it's plenty under a video
// stream. The audio watchdog probes this same URL, so what it checks is exactly
// what OBS plays.
const (
	streamHost   = "https://ice.somafm.com"
	streamSuffix = "-128-mp3"
	songsHost    = "https://somafm.com/songs"
)

// StreamURL is the ICEcast URL OBS's ffmpeg_source plays for a station.
func StreamURL(station string) string {
	return streamHost + "/" + station + streamSuffix
}

// SongsURL is the station's now-playing JSON feed — the only source that knows
// what SomaFM is currently playing, since the audio arrives as a bare stream.
func SongsURL(station string) string {
	return songsHost + "/" + station + ".json"
}

// stationFromURL recovers the station id from a stream URL, so the Store can
// read the live station back off OBS at startup rather than guessing. Tolerates
// any host and any bitrate suffix (the scene config's URL is hand-written and
// needn't match StreamURL exactly); an unrecognised URL yields "".
func stationFromURL(url string) string {
	_, last, ok := strings.Cut(url, "://")
	if !ok {
		return ""
	}
	host, path, ok := strings.Cut(last, "/")
	if !ok || !strings.HasSuffix(host, "somafm.com") {
		// Some ids ("live") are ordinary path words, so a non-SomaFM URL could
		// otherwise read as a station.
		return ""
	}
	// Station ids contain no "-", so the first one starts the bitrate suffix.
	id, _, _ := strings.Cut(path, "-")
	if !ValidStation(id) {
		return ""
	}
	return id
}

// ValidStation reports whether id is a SomaFM channel we know about.
func ValidStation(id string) bool {
	for _, s := range Stations {
		if s.ID == id {
			return true
		}
	}
	return false
}

// StationName is the human name for a station id, falling back to the id for
// one we don't know (which only happens if OBS was pointed somewhere by hand).
func StationName(id string) string {
	for _, s := range Stations {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

// Stations is every SomaFM channel, in display order (alphabetical by name —
// the order a dropdown wants; SomaFM publishes no ranking).
//
// ponytail: hardcoded from somafm.com/channels.json. The lineup changes about
// once a year and the ids never do, so a hand-maintained list beats a fetch +
// cache + "what do we offer when SomaFM is down" answer. Regenerate it if that
// stops being true. Every entry is verified to serve StreamURL (2026-07-29).
var Stations = []Station{
	{"beatblender", "Beat Blender"},
	{"brfm", "Black Rock FM"},
	{"bootliquor", "Boot Liquor"},
	{"bossa", "Bossa Beyond"},
	{"chillits", "Chillits Radio"},
	{"cliqhop", "cliqhop idm"},
	{"covers", "Covers"},
	{"deepspaceone", "Deep Space One"},
	{"defcon", "DEF CON Radio"},
	{"digitalis", "Digitalis"},
	{"doomed", "Doomed"},
	{"dronezone", "Drone Zone"},
	{"dz2", "Drone Zone 2"},
	{"dubstep", "Dub Step Beyond"},
	{"fluid", "Fluid"},
	{"folkfwd", "Folk Forward"},
	{"groovesalad", "Groove Salad"},
	{"groovesalad2", "Groove Salad 2"},
	{"gsclassic", "Groove Salad Classic"},
	{"reggae", "Heavyweight Reggae"},
	{"illstreet", "Illinois Street Lounge"},
	{"indiepop", "Indie Pop Rocks!"},
	{"seventies", "Left Coast 70s"},
	{"lush", "Lush"},
	{"metal", "Metal Detector"},
	{"missioncontrol", "Mission Control"},
	{"n5md", "n5MD Radio"},
	{"poptron", "PopTron"},
	{"secretagent", "Secret Agent"},
	{"7soul", "Seven Inch Soul"},
	{"sf1033", "SF 10-33"},
	{"sfinsf", "SF in SF"},
	{"scanner", "SF Police Scanner"},
	{"live", "SomaFM Live"},
	{"specials", "SomaFM Specials"},
	{"sonicuniverse", "Sonic Universe"},
	{"spacestation", "Space Station Soma"},
	{"suburbsofgoa", "Suburbs of Goa"},
	{"synphaera", "Synphaera Radio"},
	{"darkzone", "The Dark Zone"},
	{"insound", "The In-Sound"},
	{"thetrip", "The Trip"},
	{"thistle", "ThistleRadio"},
	{"tikitime", "Tiki Time"},
	{"u80s", "Underground 80s"},
	{"vaporwaves", "Vaporwaves"},
}
