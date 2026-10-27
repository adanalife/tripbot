// Package beds owns which background-audio bed the stream is playing and how
// to switch between them.
//
// The OBS scene collection ships ONE "Background Audio" ffmpeg_source (see the
// adanalife/obs repo's config/Tripbot.json.tmpl). Its settings decide the bed:
// a network stream for the radio, a looping local FLAC for the car-hum drone, a
// non-looping local track for the album the radio falls back to. Switching a
// bed is therefore just a SetInputSettings overlay onto that one source — the
// same mechanism the audio watchdog uses to swap the radio out for a local bed.
//
// Which bed is live is process state, not database state: it's a property of
// the OBS container this tripbot is paired with, and it dies with the pod. The
// Store reads it back off OBS at startup so a tripbot restart doesn't lose
// track of what's actually playing.
package beds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
)

// Bed is one background-audio source. Radio and CarHum are selectable; Album
// is the radio's fallback and only ever reported as what is playing.
type Bed string

const (
	// Radio is an Icecast/Shoutcast internet-radio stream, on whichever station
	// from the mounted list is selected (see Stations), and the stream's default
	// bed. Whether a station's music is cleared for our rebroadcast depends on
	// the station: a third-party one trips YouTube's Content ID and the other
	// platforms' audio ID. OBS never boots here — a station's servers can refuse
	// our IP — so it boots on the Album fallback and the audio watchdog brings
	// it onto the radio once the station answers.
	Radio Bed = "radio"
	// CarHum is the synthesized, licence-clean car-interior drone baked into the
	// OBS image. Safe on every platform; the watchdog's fallback bed.
	CarHum Bed = "carhum"
	// Album is the radio's fallback: tracks from FallbackAlbums on the mounted
	// music share, shuffled, one at a time, advancing when OBS reports the
	// current track ended. It plays between OBS booting and the radio coming up,
	// and while the watchdog rides out a radio outage. Not selectable.
	Album Bed = "album"
)

// All is the bed set the console and chat offer, in display order.
var All = []Bed{Radio, CarHum}

// FallbackAlbums is what the album fallback plays: each entry an album's
// directory or a group prefix, matched the way albumsFor matches. A fixed list
// rather than the whole share, so a directory that lands on the share by
// mistake never reaches the stream.
var FallbackAlbums = []string{"streambeats", "fifty-horizons", "wyoming-miles"}

// Valid reports whether b is a bed that can be selected.
func Valid(b Bed) bool {
	for _, x := range All {
		if x == b {
			return true
		}
	}
	return false
}

// Cross-repo contracts with the adanalife/obs repo. InputName must match the
// source "name" in config/Tripbot.json.tmpl; the paths must match what the
// image bakes in (carhum) and where cdk8s mounts the obs-music-local PVC. tripbot
// mounts that same claim at the same path, so a path chosen here resolves
// identically inside the OBS container.
const (
	InputName = "Background Audio"
	// CarHumDir holds one FLAC per voicing, named car-hum-<voicing>.flac by the
	// obs repo's carhum/render-variants.sh.
	CarHumDir = "/opt/tripbot/assets/carhum"
	MusicDir  = "/opt/tripbot/assets/music"

	// MusicIndexFile is where cdk8s mounts the album track index that
	// bin/stage-streambeats emits — a ConfigMap, so a missing one leaves the
	// pod schedulable where a missing PVC would not. Listing albums from it is
	// what keeps tripbot off the music claim entirely: the paths in it still
	// point into OBS's copy of the same claim, and OBS is the only side that
	// ever opens the audio bytes.
	MusicIndexFile = "/opt/tripbot/assets/music-index/index.json"

	// FallbackFile is the audio watchdog's copy of the car-hum drone: the same
	// audio as CarHumFile at a path of its own. The source's settings record
	// only which file is playing, so the two names are what separate "an
	// operator selected the car hum" from "the watchdog fell back to it" — a
	// distinction the process loses on restart and can only read back here.
	FallbackFile = "/opt/tripbot/assets/carhum/car-hum-fallback.flac"
)

// Voicings are the car-hum renders the OBS image ships, in display order. The
// names are a contract with the obs repo's carhum/render-variants.sh, which
// renders one preset per name; a voicing missing there is a source OBS cannot
// open, which is silence.
var Voicings = []string{"idle", "highway", "backroad", "mountain"}

// DefaultVoicing is the drone a store starts on and the one the audio watchdog
// falls back to — FallbackFile is a copy of it.
const DefaultVoicing = "idle"

// ValidVoicing reports whether v is a car-hum voicing the image ships.
func ValidVoicing(v string) bool {
	return slices.Contains(Voicings, v)
}

// CarHumFile is the drone file for a voicing. An unknown one yields the
// default rather than a path that isn't there: this is what OBS is handed, and
// a bed that refuses to play is worse than the wrong flavour of drone.
func CarHumFile(voicing string) string {
	if !ValidVoicing(voicing) {
		voicing = DefaultVoicing
	}
	return filepath.Join(CarHumDir, "car-hum-"+voicing+".flac")
}

// voicingFromFile recovers the voicing from a drone path, so the store can read
// the live one back off OBS at startup — the counterpart of stationFromURL and
// albumFromFile. "" for anything that isn't one of the shipped renders,
// including the watchdog's FallbackFile, which names an outage rather than a
// choice.
func voicingFromFile(file string) string {
	if filepath.Dir(file) != CarHumDir {
		return ""
	}
	v := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(file), "car-hum-"), ".flac")
	if !ValidVoicing(v) {
		return ""
	}
	return v
}

// audioExts are the track extensions scanned for under MusicDir. Matches the
// find in the obs repo's script/background-audio.sh.
var audioExts = map[string]bool{".mp3": true, ".flac": true, ".m4a": true, ".ogg": true, ".opus": true}

// OBS is the subset of pkg/obs the store drives. Injectable so switching logic
// unit-tests without a real OBS WebSocket.
type OBS interface {
	// SetNetwork flips the source to a network stream URL.
	SetNetwork(ctx context.Context, inputName, url string) error
	// SetLocalFile points the source at a local path; loop=false lets the media
	// end (which is how album advance is triggered).
	SetLocalFile(ctx context.Context, inputName, file string, loop bool) error
	// Settings reads the source's current settings.
	Settings(ctx context.Context, inputName string) (map[string]any, error)
}

// Store holds the live bed + fallback album position and applies changes to OBS.
type Store struct {
	obs OBS
	// musicDir is the share root the track paths sit under. It stays the path
	// vocabulary even where the share isn't mounted: albumFromFile and
	// bedFromSettings classify what OBS reports back by this prefix.
	musicDir string
	// indexFile is the track index to list albums from; "" walks musicDir.
	indexFile string
	platform  string      // stamped on the bed metrics; each platform picks its own bed
	np        *nowPlaying // the station's metadata for the tuned channel

	mu      sync.Mutex
	bed     Bed
	station string // radio station id the radio bed plays
	// voicing is which car-hum render the CarHum bed plays. Like station it
	// survives a switch away, so the bed returns to the one chosen.
	voicing   string
	tracks    []string // the fallback album's shuffled play order
	idx       int
	lastStart time.Time // when a track was last pointed at OBS; see Advance
	// fallback is the bed the audio watchdog has playing while the radio is still
	// the selected one, and "" when nothing is standing in. The selected bed
	// can't say so — the watchdog's outage machinery only runs while the store
	// reads the radio — so this is what keeps Advance walking the fallback album
	// instead of letting one track end in the silence the fallback exists to
	// prevent, and what lets Playing name audio the selection doesn't.
	fallback Bed

	// The switch waiting out switchDelay, and the timer that will apply it. gen
	// rises with every request so a timer that fires after being superseded can
	// tell it is stale — Timer.Stop can't promise it won.
	pending *Switch
	timer   *time.Timer
	gen     uint64

	// albums caches the library read, guarded by its own lock: the listings are
	// taken while s.mu is already held (see loadFallbackLocked).
	albums albumCache
}

// albumCache holds the last library read and the index file it came from, so
// the repeated listings — the console's per-platform polls, a lookup per chat
// command — cost one stat instead of a read and unmarshal.
// Keyed on the file's mtime and size rather than a TTL: new music arrives while
// the bot runs, and the album has to appear the moment the index does.
type albumCache struct {
	mu      sync.Mutex
	tracks  map[string][]string
	modTime time.Time
	size    int64
}

// advanceDebounce is how soon after a track starts another advance is taken to
// be a duplicate report of the same ending rather than a real one. A var so the
// tests, which advance back to back on purpose, can switch it off.
var advanceDebounce = 3 * time.Second

// switchDelay is how long a requested switch waits before it reaches OBS. The
// bed is the audio every viewer hears at once, so a mis-click on the console (or
// a fumbled `!audio`) is disruptive in a way no other control is — this is the
// window to correct it in. Long enough to notice and re-click, short enough that
// a deliberate switch doesn't read as a dead button.
//
// A var so tests can shrink it. Zero applies inline, which is also the honest
// behavior for "no delay configured": the switch lands before the call returns
// and OBS's answer is the caller's.
var switchDelay = 5 * time.Second

// Switch is a bed switch that hasn't reached OBS yet. Station and Voicing carry
// the choice within the bed, since a pending tune has to be describable before
// it's live — the store still reports the playing station and voicing, not these.
type Switch struct {
	Bed     Bed
	Station string
	Voicing string
	// At is when the switch lands. A deadline rather than a duration so a value
	// read once doesn't go stale while it's being rendered.
	At time.Time
}

// NewStore returns a store defaulting to bed (used until Detect reads the real
// one off OBS). musicDir is the album root; "" uses MusicDir. platform labels
// the bed metrics — without it the per-platform instances, which each run a
// different bed, would write the same series.
func NewStore(o OBS, bed Bed, musicDir, platform string) *Store {
	if musicDir == "" {
		musicDir = MusicDir
	}
	if !Valid(bed) {
		bed = CarHum
	}
	return &Store{
		obs:      o,
		bed:      bed,
		musicDir: musicDir,
		platform: platform,
		station:  DefaultStation(),
		voicing:  DefaultVoicing,
		np:       newNowPlaying(),
	}
}

// WithIndex lists the albums from a track index file instead of walking the
// share. The index is how tripbot reads the library without mounting it: the
// bot only ever hands OBS a path, so a list of paths is the whole dependency.
// An absent index falls back to the share, which is what keeps a laptop run
// with the real directory mounted working unchanged.
func (s *Store) WithIndex(path string) *Store {
	s.indexFile = path
	return s
}

// Current reports the selected bed; the track is always "", since only the
// album fallback has one and it is never the selection.
// This is the choice, not always the audio: during a radio outage the watchdog
// puts another bed on air without changing the selection, which is what keeps
// the outage machinery running. Callers naming what a viewer hears want Playing.
func (s *Store) Current() (Bed, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bed, ""
}

// Playing reports the bed actually on air and, on the album, its track file —
// the answer to "what is this?". It differs from Current only during a radio
// outage, where the watchdog swaps the album (or the drone) onto the source and
// leaves the radio selected: reading the selection there names a station nobody is
// hearing, and on the album a radio track nobody is hearing either.
func (s *Store) Playing() (Bed, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bed := s.playingLocked()
	return bed, s.trackOnAirLocked(bed)
}

// playingLocked is the bed on air: the fallback while one is standing in, and
// otherwise the selection. Caller holds s.mu.
func (s *Store) playingLocked() Bed {
	if s.fallback != "" {
		return s.fallback
	}
	return s.bed
}

// trackOnAirLocked is the track file for bed, which only the album has. Caller
// holds s.mu.
func (s *Store) trackOnAirLocked(bed Bed) string {
	if bed != Album || s.idx >= len(s.tracks) {
		return ""
	}
	return s.tracks[s.idx]
}

// Station reports the radio station the radio bed plays. Always a station,
// whichever bed is live — it's the one the bed returns to.
func (s *Store) Station() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.station
}

// Voicing reports which car-hum render the CarHum bed plays. Like Station, it
// answers whichever bed is live — it's the one that bed returns to.
func (s *Store) Voicing() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.voicing
}

// PlayingAlbum reports the album the fallback's current track sits in — the
// fallback spans several albums, so the track is the only way to know which one
// is on air. "" when the album isn't playing.
//
// Read off the track path rather than tracked separately: the path is what OBS
// is playing, so it can't drift from the audio the way a remembered value could.
func (s *Store) PlayingAlbum() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	track := s.trackOnAirLocked(s.playingLocked())
	if track == "" {
		return ""
	}
	return albumFromFile(track, s.musicDir)
}

// RadioTrack reports the song playing on the tuned channel, from the station's metadata
// for it. Cached, so chat and the console's poll share one fetch. Only ask while
// the radio bed is live — it answers for the tuned channel either way, and on
// another bed that names a song nobody is hearing.
func (s *Store) RadioTrack(ctx context.Context) (artist, title string, err error) {
	return s.np.current(ctx, s.Station())
}

// Detect reads the source's settings and records which bed OBS actually booted
// on, so neither a tripbot restart nor an OBS restart leaves the store
// reporting a stale guess. The audio watchdog calls it each time OBS comes
// (back) up. A failure here is not fatal — the current bed stands and the next
// call corrects it.
func (s *Store) Detect(ctx context.Context) {
	settings, err := s.obs.Settings(ctx, InputName)
	if err != nil {
		slog.WarnContext(ctx, "background audio: could not read current bed from obs", "err", err)
		// Publish the seed anyway: it's what Current() reports, so the gauge and
		// the store agree even when the read failed.
		s.record()
		return
	}
	bed := bedFromSettings(settings, s.musicDir)
	file, _ := settings["local_file"].(string)
	// The scene config's `input` URL is the station OBS booted on. It survives a
	// local-file detour (the overlay merge never clears it), so it's readable
	// whichever bed is live.
	url, _ := settings["input"].(string)
	local, _ := settings["is_local_file"].(bool)
	s.mu.Lock()
	s.bed = bed
	if station := stationFromURL(url); station != "" {
		s.station = station
	}
	// The drone file names the voicing, the way the `input` URL names the
	// station. "" covers both a non-drone file and the watchdog's fallback copy,
	// neither of which is a choice anyone made.
	if voicing := voicingFromFile(file); voicing != "" {
		s.voicing = voicing
	}
	// A local file other than a chosen drone is a fallback standing in for the
	// radio.
	var loadErr error
	switch {
	case local && under(file, s.musicDir):
		// OBS boots on the album, so this is every fresh boot as well as a
		// restart mid-outage. Nothing has built the play order yet; without one
		// Advance has nowhere to go and the boot track ends in silence.
		s.fallback = Album
		loadErr = s.loadFallbackLocked(file)
	case local && file == FallbackFile:
		s.fallback = CarHum
	default:
		s.fallback = ""
	}
	station, fallback := s.station, s.fallback
	s.mu.Unlock()
	s.record()
	if loadErr != nil {
		slog.WarnContext(ctx, "background audio: fallback album is live but its play order is empty",
			"err", loadErr)
	}
	slog.InfoContext(ctx, "background audio: detected bed", "bed", bed, "station", station,
		"fallback", fallback)
}

// bedFromSettings maps an ffmpeg_source's settings back to the selected bed
// that would have produced them. A local file under the music share is the
// album fallback and the watchdog's fallback file is the drone standing in, so
// both mean the radio is the selected bed and not yet on air; any other local
// file is the car-hum drone (that's the only other one the image ships); a
// network source is the radio.
func bedFromSettings(settings map[string]any, musicDir string) Bed {
	if local, _ := settings["is_local_file"].(bool); !local {
		return Radio
	}
	file, _ := settings["local_file"].(string)
	switch {
	case under(file, musicDir):
		// OBS boots here, and the watchdog swaps here in an outage: either way
		// the radio is what's selected and the watchdog is what brings it back.
		return Radio
	case file == FallbackFile:
		// Only the watchdog writes this path, and only while the radio is the
		// selected bed. Reading it as CarHum would hand an operator choice to
		// the outage, which switches the watchdog off and strands the stream on
		// the drone once the radio comes back.
		return Radio
	}
	return CarHum
}

// albumFromFile recovers the album subdir from a track path, which is how the
// fallback's album on air is named. A track directly at the share root belongs to
// no album and yields "", as does any path outside the share.
func albumFromFile(file, musicDir string) string {
	if file == "" || !under(file, musicDir) {
		return ""
	}
	rel, err := filepath.Rel(musicDir, file)
	if err != nil {
		return ""
	}
	// Only the first segment is the album; deeper nesting (an album with
	// per-disc subdirs) still belongs to the album at the top.
	album, _, nested := strings.Cut(rel, string(filepath.Separator))
	if !nested {
		return "" // loose file at the share root
	}
	return album
}

// under reports whether file sits inside dir, comparing whole path segments so
// a sibling like "/music-old/x.mp3" can't match "/music".
func under(file, dir string) bool {
	rel, err := filepath.Rel(dir, file)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Set switches to bed after switchDelay. An invalid bed is rejected here and
// now; everything else is the scheduled path, so see schedule for what the
// caller can and can't learn from the error.
func (s *Store) Set(ctx context.Context, bed Bed) error {
	if !Valid(bed) {
		return fmt.Errorf("unknown background-audio bed %q", bed)
	}
	s.mu.Lock()
	station, voicing := s.station, s.voicing
	s.mu.Unlock()
	if bed == Radio && !SelectableStation(station) {
		// The remembered station can be one OBS was found playing; a private one
		// is never put on air by a selection, so tune the default instead.
		if station = DefaultStation(); station == "" {
			return errNoStation
		}
	}
	return s.schedule(ctx, Switch{Bed: bed, Station: station, Voicing: voicing})
}

// errNoStation refuses the radio bed when the station list gave it nothing to
// tune, so the stream keeps its current bed rather than going silent.
var errNoStation = fmt.Errorf("radio bed has no stations: %s is missing, empty or invalid", RadioStationsFile)

// setNow switches to bed and applies it to OBS. The store is only updated once
// OBS accepts the change, so a failed switch leaves the reported bed truthful.
func (s *Store) setNow(ctx context.Context, bed Bed) error {
	s.mu.Lock()
	target := s.trackLocked(bed)
	station := s.station
	s.mu.Unlock()

	if err := s.apply(ctx, bed, target, station); err != nil {
		return err
	}

	s.mu.Lock()
	s.bed = bed
	s.lastStart = time.Now()
	// A chosen bed ends any outage fallback: what's audible is now what was
	// asked for, whichever bed the watchdog had put there.
	s.fallback = ""
	s.mu.Unlock()
	s.record()
	// Counted here rather than at the callers so a console switch and a chat
	// switch land on the same counter — they are the same switch.
	instrumentation.BackgroundAudioSelections.Inc(s.platform, string(bed))
	slog.InfoContext(ctx, "background audio: bed switched", "bed", bed, "track", target)
	return nil
}

// SetStation tunes the radio bed to another channel and switches to it, so
// picking a station is one action rather than "select radio, then tune".
func (s *Store) SetStation(ctx context.Context, station string) error {
	if !SelectableStation(station) {
		return fmt.Errorf("unknown or private radio station %q", station)
	}
	s.mu.Lock()
	voicing := s.voicing
	s.mu.Unlock()
	return s.schedule(ctx, Switch{Bed: Radio, Station: station, Voicing: voicing})
}

// SetVoicing picks which car-hum render the drone plays and switches to it, so
// picking a voicing is one action rather than "select carhum, then voice it" —
// the same shape as SetStation.
func (s *Store) SetVoicing(ctx context.Context, voicing string) error {
	if !ValidVoicing(voicing) {
		return fmt.Errorf("unknown car-hum voicing %q", voicing)
	}
	s.mu.Lock()
	station := s.station
	s.mu.Unlock()
	return s.schedule(ctx, Switch{Bed: CarHum, Station: station, Voicing: voicing})
}

// schedule holds sw for switchDelay and then applies it, replacing whatever was
// already waiting. Superseding rather than queueing is the whole point: a
// mis-click corrected inside the window must not land twice, and the correction
// is what the operator meant.
//
// The returned error only covers what is knowable now — an unreachable OBS or an
// unmounted share can't be, since they're discovered when the switch lands.
// Those log a warning and leave the live bed alone, so the store keeps reporting
// what's actually playing and a caller re-reading it sees the switch didn't take.
func (s *Store) schedule(ctx context.Context, sw Switch) error {
	if switchDelay <= 0 {
		return s.applyPending(ctx, sw)
	}

	sw.At = time.Now().Add(switchDelay)
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.gen++
	gen := s.gen
	s.pending = &sw
	// context.WithoutCancel: the switch outlives the request that asked for it —
	// an HTTP handler returning would otherwise cancel the OBS call that lands
	// seconds later.
	applyCtx := context.WithoutCancel(ctx)
	s.timer = time.AfterFunc(switchDelay, func() {
		s.mu.Lock()
		stale := gen != s.gen
		s.mu.Unlock()
		if stale {
			return // superseded between the timer firing and this lock
		}
		if err := s.applyPending(applyCtx, sw); err != nil {
			slog.WarnContext(applyCtx, "background audio: scheduled switch failed",
				"err", err, "bed", sw.Bed, "station", sw.Station)
		}
	})
	s.mu.Unlock()
	slog.InfoContext(ctx, "background audio: switch scheduled", "bed", sw.Bed,
		"station", sw.Station, "delay", switchDelay)
	return nil
}

// applyPending records sw's station and voicing and switches to its bed,
// rolling both back if OBS rejects the switch — a station we aren't playing must
// not be the one we report.
//
// The pending flag outlives the station swap and clears only once the bed is
// actually audible. setNow takes the lock itself, so the switch can't be made
// under one; the flag is what covers that gap, and clearing it early would let
// a caller read the new station off a store still playing the old bed.
func (s *Store) applyPending(ctx context.Context, sw Switch) error {
	s.mu.Lock()
	prevStation, prevVoicing := s.station, s.voicing
	s.station, s.voicing = sw.Station, sw.Voicing
	s.mu.Unlock()

	if err := s.setNow(ctx, sw.Bed); err != nil {
		s.mu.Lock()
		s.station, s.voicing = prevStation, prevVoicing
		s.pending = nil
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
	slog.InfoContext(ctx, "background audio: switch applied", "bed", sw.Bed,
		"station", sw.Station, "voicing", sw.Voicing)
	return nil
}

// Pending reports the switch waiting to land, if there is one. false means what
// the store reports is what OBS is playing.
func (s *Store) Pending() (Switch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		return Switch{}, false
	}
	return *s.pending, true
}

// Advance moves to the next fallback album track (wrapping at the end) and
// plays it. A no-op unless the album is what's audible, since the callers report
// the media ending, which also happens on the other beds.
//
// Two of them report the same ending: the playback-ended subscription, which
// arrives in milliseconds, and the watchdog tick that backs it up. So an
// advance within advanceDebounce of the last track starting is dropped as that
// duplicate. Tracks run minutes, so nothing legitimate lands that close — and
// the same guard covers a swap onto the fallback, which starts a track and
// would otherwise be stepped over by the outgoing source's own ending.
func (s *Store) Advance(ctx context.Context) error {
	s.mu.Lock()
	if s.playingLocked() != Album || time.Since(s.lastStart) < advanceDebounce {
		s.mu.Unlock()
		return nil
	}
	if len(s.tracks) == 0 {
		s.mu.Unlock()
		return s.rescueEmptyAlbum(ctx)
	}
	s.lastStart = time.Now()
	current := s.tracks[s.idx]
	s.idx = (s.idx + 1) % len(s.tracks)
	// Re-shuffle on wrap so a long stream doesn't repeat the same 100-track
	// sequence in the same order every time. The fresh order has to keep the
	// track that just finished off the front, or wrapping plays it twice back
	// to back.
	if s.idx == 0 {
		reshuffleAvoiding(s.tracks, current)
	}
	track := s.tracks[s.idx]
	s.mu.Unlock()

	if err := s.obs.SetLocalFile(ctx, InputName, track, false); err != nil {
		return fmt.Errorf("advance album track: %w", err)
	}
	slog.InfoContext(ctx, "background audio: next track", "track", filepath.Base(track))
	return nil
}

// rescueEmptyAlbum puts the car hum on air when the album fallback is playing
// with no play order to advance through. Detect reaches that state when OBS
// boots on an album track while the track index is missing or holds none of
// FallbackAlbums: the boot track ends, there is nothing to queue, and the
// stream is silent with OBS reporting a healthy source.
//
// The drone is the only bed left — it is baked into the image, so it needs
// neither the share nor the index — and it loops, so nothing has to advance it.
// Recording it as the fallback is what takes the stream off this path —
// playingLocked then reports CarHum, so the next Advance returns early instead
// of rescuing an already-rescued stream. The selection stays the radio, so the
// watchdog still swaps onto it once the station answers.
func (s *Store) rescueEmptyAlbum(ctx context.Context) error {
	if err := s.obs.SetLocalFile(ctx, InputName, FallbackFile, true); err != nil {
		return fmt.Errorf("rescue empty album bed: %w", err)
	}
	s.mu.Lock()
	s.fallback = CarHum
	s.lastStart = time.Now()
	s.mu.Unlock()
	s.record()
	slog.ErrorContext(ctx, "background audio: fallback album has no tracks, falling back to the car hum")
	return nil
}

// SwapToFallback points the source at the bed the audio watchdog rides out a
// radio outage on: the album when the share has FallbackAlbums tracks, the
// car-hum drone when it doesn't. The album is licence-clean (it was made for
// this stream) and is actual music, so it's the better degraded state; the
// drone covers an empty or unmounted share, where it's the only bed left.
//
// The selection is deliberately left alone — the fallback is recorded beside it
// instead. The watchdog runs its outage machinery only while the store reads
// the radio, so selecting the fallback here would switch the watchdog off and
// strand the stream on it for good.
// That is also why the album's play order is (re)built here: nothing else on
// this path builds one, and without it the album plays a single track and falls
// silent — the tracks=0 state record() exists to expose.
func (s *Store) SwapToFallback(ctx context.Context) error {
	s.mu.Lock()
	track := ""
	loadErr := s.loadFallbackLocked("")
	if loadErr == nil {
		track = s.trackLocked(Album)
	}
	s.mu.Unlock()
	if loadErr != nil {
		slog.WarnContext(ctx, "background audio: no album to fall back to, using the car hum",
			"err", loadErr)
	}

	// The album plays unlooped, so OBS reports the media ending and Advance
	// queues the next track. The drone loops: nothing advances it.
	file, loop := track, false
	if file == "" {
		file, loop = FallbackFile, true
	}
	if err := s.obs.SetLocalFile(ctx, InputName, file, loop); err != nil {
		return fmt.Errorf("swap to fallback bed: %w", err)
	}

	s.mu.Lock()
	s.fallback = CarHum
	if track != "" {
		s.fallback = Album
	}
	s.lastStart = time.Now()
	s.mu.Unlock()
	s.record()
	slog.InfoContext(ctx, "background audio: swapped to the fallback bed", "file", file)
	return nil
}

// SwapToRadio points the source back at the tuned station once the edge is
// serving again, ending the fallback — a fallback album stops advancing because
// it has stopped playing. The counterpart of SwapToFallback; the selection is
// untouched here too, since on this path it never stopped being the radio.
func (s *Store) SwapToRadio(ctx context.Context) error {
	if err := s.apply(ctx, Radio, "", s.Station()); err != nil {
		return fmt.Errorf("swap back to radio: %w", err)
	}
	s.mu.Lock()
	s.fallback = ""
	s.mu.Unlock()
	return nil
}

// loadFallbackLocked builds a fresh shuffled play order over FallbackAlbums.
// playing positions the order on a track already on air (pass "" to start at
// the top), so the next advance moves off it instead of restarting it. Caller
// holds s.mu.
func (s *Store) loadFallbackLocked(playing string) error {
	byAlbum := s.albumTracks()
	albums := slices.Sorted(maps.Keys(byAlbum))
	var tracks []string
	for _, sel := range FallbackAlbums {
		for _, album := range albumsFor(albums, sel) {
			tracks = append(tracks, byAlbum[album]...)
		}
	}
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks for the fallback albums %v under %s", FallbackAlbums, s.musicDir)
	}
	shuffle(tracks)
	s.tracks, s.idx = tracks, max(slices.Index(tracks, playing), 0)
	return nil
}

// trackLocked returns the local file a bed should play, or "" for the radio.
// Caller holds s.mu.
func (s *Store) trackLocked(bed Bed) string {
	switch bed {
	case CarHum:
		return CarHumFile(s.voicing)
	case Album:
		if s.idx < len(s.tracks) {
			return s.tracks[s.idx]
		}
	}
	return ""
}

// apply writes the bed onto the OBS source. Only the album plays unlooped.
func (s *Store) apply(ctx context.Context, bed Bed, track, station string) error {
	if bed == Radio {
		url := StreamURL(station)
		if url == "" {
			return errNoStation
		}
		return s.obs.SetNetwork(ctx, InputName, url)
	}
	return s.obs.SetLocalFile(ctx, InputName, track, bed != Album)
}

// albumsFor returns the albums a FallbackAlbums entry covers: the one it names
// exactly, or every album under it when it's a group prefix. Empty when nothing
// matches.
//
// The prefix has to end at a "-" boundary, or "streambeats-lo" would quietly
// select the lofi albums and read as a working selection rather than a typo.
func albumsFor(albums []string, selection string) []string {
	if slices.Contains(albums, selection) {
		return []string{selection}
	}
	var out []string
	for _, a := range albums {
		if strings.HasPrefix(a, selection+"-") {
			out = append(out, a)
		}
	}
	return out
}

// albumTracks is the library: album directory name to its sorted track paths.
// One source for every listing the store does, read from the index when one is
// configured and by walking the share otherwise.
//
// An unreadable library yields no albums rather than an error. That is the
// designed state where the index is mounted optional and absent: an empty album
// list leaves the fallback to the car-hum drone rather than to silence.
func (s *Store) albumTracks() map[string][]string {
	// ponytail: only the index read is cached. With no index configured the
	// library is a directory walk, and no cheap stat says whether a track
	// appeared inside an album — the share root's mtime doesn't move for it. That
	// path is the laptop run with the share mounted; cache it behind a walk of
	// the album mtimes if it ever serves a poll.
	if s.indexFile == "" {
		return scanShare(s.musicDir)
	}

	// Stat outside the lock's decision so a rewritten index is picked up on the
	// first call after it lands. The returned map is never mutated by callers,
	// so one copy is shared rather than cloned per read.
	fi, statErr := os.Stat(s.indexFile)

	s.albums.mu.Lock()
	defer s.albums.mu.Unlock()
	if statErr == nil && s.albums.tracks != nil &&
		fi.ModTime().Equal(s.albums.modTime) && fi.Size() == s.albums.size {
		return s.albums.tracks
	}
	s.albums.tracks = nil

	byAlbum, err := readIndex(s.indexFile)
	if err == nil {
		if statErr == nil {
			s.albums.tracks, s.albums.modTime, s.albums.size = byAlbum, fi.ModTime(), fi.Size()
		}
		return byAlbum
	}
	// A corrupt index is worth a shout; an absent one is the optional mount
	// doing its job. Both fall through to the share, which is empty wherever
	// the index is the reason the claim isn't mounted.
	if !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("background audio: unreadable album index", "path", s.indexFile, "err", err)
	}
	return scanShare(s.musicDir)
}

// readIndex loads the album track index bin/stage-streambeats emits. Paths are
// taken as written: they are the share paths OBS resolves, which is the only
// consumer that has to open them.
func readIndex(path string) (map[string][]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var byAlbum map[string][]string
	if err := json.Unmarshal(raw, &byAlbum); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for album, tracks := range byAlbum {
		if len(tracks) == 0 {
			delete(byAlbum, album) // an album with no tracks isn't selectable
			continue
		}
		sort.Strings(tracks)
	}
	return byAlbum, nil
}

// scanShare walks the mounted share: every immediate subdirectory holding at
// least one track, keyed by its name.
func scanShare(musicDir string) map[string][]string {
	entries, err := os.ReadDir(musicDir)
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if tracks, err := scanTracks(filepath.Join(musicDir, e.Name())); err == nil && len(tracks) > 0 {
			out[e.Name()] = tracks
		}
	}
	return out
}

// scanTracks lists the album tracks under dir, sorted so the pre-shuffle order
// is deterministic (the shuffle is what makes playback vary, not readdir order).
//
// Only ever called on one album's directory. The share root holds other audio
// beside the albums (carsounds.m4a, a 556MB archive), and reaching it a
// directory at a time is what leaves those out.
func scanTracks(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !audioExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// record publishes the live bed and the depth of the album play order. Both
// are read back under the lock so the gauges describe one consistent moment.
//
// The play-order depth is here because an empty one is invisible everywhere
// else. Advance keeps the stream audible by falling back to the drone, but that
// is a rescue, not a repair: the fallback album still cannot play, and tracks=0
// is the only thing that says so.
func (s *Store) record() {
	s.mu.Lock()
	bed, tracks := s.bed, len(s.tracks)
	s.mu.Unlock()
	for _, b := range All {
		instrumentation.OBSBackgroundAudio.SetBed(s.platform, string(b), b == bed)
	}
	instrumentation.OBSBackgroundAudio.SetAlbumTracks(s.platform, tracks)
}

// Track is an album track as its own filename spells it. Bandcamp names files
// "<artist> - <album> - <NN Title>", so the filename carries the credit the
// share's directory name can't and repeats the album the directory already has.
// Splitting them apart is what lets a caller name a track without saying the
// album twice.
//
// Artist and Album are "" for a filename that names only the track; the
// directory is the only album a caller has in that case.
type Track struct {
	Title  string
	Album  string
	Artist string
}

// ParseTrack reads an album track path. "" yields a zero Track (the other beds
// have no track), which is how callers tell "no track" from "a track".
func ParseTrack(path string) Track {
	if path == "" {
		return Track{}
	}
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	// Under three fields the filename isn't the tagged shape, so all of it is the
	// title — "001 Maine - Atlantic Dawn" is one title, not an artist and a track.
	parts := strings.Split(name, " - ")
	if len(parts) < 3 {
		return Track{Title: trimTrackNumber(name)}
	}
	return Track{
		Artist: parts[0],
		Album:  parts[1],
		Title:  trimTrackNumber(strings.Join(parts[2:], " - ")),
	}
}

// TrackTitle is the track's title alone, for surfaces that show the album
// separately (the console ships playing_album beside this).
func TrackTitle(path string) string { return ParseTrack(path).Title }

// trackNumber matches the leading position a tagged filename carries: "21 ",
// "01. ", "3) ".
var trackNumber = regexp.MustCompile(`^\d+[.)-]?\s+`)

// trimTrackNumber drops that position. Anyone asking what a song is isn't asking
// where it sits on a disc they can't see.
func trimTrackNumber(title string) string {
	if trimmed := trackNumber.ReplaceAllString(title, ""); trimmed != "" {
		return trimmed
	}
	return title // a filename that is only a number keeps it
}

func shuffle(tracks []string) {
	rand.Shuffle(len(tracks), func(i, j int) { tracks[i], tracks[j] = tracks[j], tracks[i] })
}

// reshuffleAvoiding shuffles tracks and keeps avoid off the front, so the play
// order wrapping can't put the track that just finished straight back on air.
// A one-track album has nowhere else to go and repeats regardless.
func reshuffleAvoiding(tracks []string, avoid string) {
	shuffle(tracks)
	if len(tracks) > 1 && tracks[0] == avoid {
		j := 1 + rand.IntN(len(tracks)-1)
		tracks[0], tracks[j] = tracks[j], tracks[0]
	}
}
