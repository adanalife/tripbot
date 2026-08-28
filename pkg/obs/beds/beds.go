// Package beds owns which background-audio bed the stream is playing and how
// to switch between them.
//
// The OBS scene collection ships ONE "Background Audio" ffmpeg_source (see the
// adanalife/obs repo's config/Tripbot.json.tmpl). Its settings decide the bed:
// a network stream for SomaFM, a looping local FLAC for the car-hum drone, a
// non-looping local track for the album. Switching a bed is therefore just a
// SetInputSettings overlay onto that one source — the same mechanism the audio
// watchdog has used since 2026-06-23 to swap SomaFM out for the local bed.
//
// Which bed is live is process state, not database state: it's a property of
// the OBS container this tripbot is paired with, and it dies with the pod. The
// Store reads it back off OBS at startup so a tripbot restart doesn't lose
// track of what's actually playing.
package beds

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
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

// Bed is one selectable background-audio source.
type Bed string

const (
	// SomaFM is internet radio, on whichever of SomaFM's channels is selected
	// (see Stations). Its music is not cleared for our rebroadcast — it trips
	// YouTube's Content ID and the other platforms' audio ID — so it's a
	// Twitch-only default, by tolerance rather than licence.
	SomaFM Bed = "somafm"
	// CarHum is the synthesized, licence-clean car-interior drone baked into the
	// OBS image. Safe on every platform; the watchdog's fallback bed.
	CarHum Bed = "carhum"
	// Album plays tracks from the mounted music share, one at a time, advancing
	// when OBS reports the current track ended.
	Album Bed = "album"
)

// All is the bed set the console offers, in display order.
var All = []Bed{SomaFM, CarHum, Album}

// Valid reports whether b is a bed we know how to play.
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
	InputName  = "Background Audio"
	CarHumFile = "/opt/tripbot/assets/carhum/car-hum-idle.flac"
	MusicDir   = "/opt/tripbot/assets/music"

	// FallbackFile is the audio watchdog's copy of the car-hum drone: the same
	// audio as CarHumFile at a path of its own. The source's settings record
	// only which file is playing, so the two names are what separate "an
	// operator selected the car hum" from "the watchdog fell back to it" — a
	// distinction the process loses on restart and can only read back here.
	FallbackFile = "/opt/tripbot/assets/carhum/car-hum-fallback.flac"
)

// audioExts are the track extensions scanned for under MusicDir. Matches the
// find in the obs repo's script/background-audio.sh.
var audioExts = map[string]bool{".mp3": true, ".flac": true, ".m4a": true, ".ogg": true}

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

// Store holds the live bed + album position and applies changes to OBS.
type Store struct {
	obs      OBS
	musicDir string
	platform string      // stamped on the bed metrics; each platform picks its own bed
	np       *nowPlaying // SomaFM's feed for the tuned channel

	mu      sync.Mutex
	bed     Bed
	station string // SomaFM channel id the SomaFM bed plays
	// album is the Album bed's selection: one album's directory, a prefix naming
	// a group of them ("streambeats-lofi"), or "" for the whole share.
	album     string
	shuffle   bool     // play order is shuffled rather than sequential
	tracks    []string // the play order; rebuilt each time a selection starts
	idx       int
	lastStart time.Time // when a track was last pointed at OBS; see Advance
	// fallbackAlbum says the audio watchdog has the album playing while SomaFM
	// is still the selected bed. The bed itself can't say so — the watchdog's
	// outage machinery only runs while the store reads SomaFM — so this is what
	// keeps Advance walking the fallback album instead of letting one track end
	// in the silence the fallback exists to prevent.
	fallbackAlbum bool

	// The switch waiting out switchDelay, and the timer that will apply it. gen
	// rises with every request so a timer that fires after being superseded can
	// tell it is stale — Timer.Stop can't promise it won.
	pending *Switch
	timer   *time.Timer
	gen     uint64
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

// Switch is a bed switch that hasn't reached OBS yet. Station and Album carry
// the choice within the bed, since a pending tune has to be describable before
// it's live — the store still reports the playing station and album, not these.
type Switch struct {
	Bed     Bed
	Station string
	Album   string
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
		station:  DefaultStation,
		shuffle:  true, // a single album on a 24/7 stream shouldn't loop in one order
		np:       newNowPlaying(),
	}
}

// Current reports the live bed and, on the album, the track file playing.
func (s *Store) Current() (Bed, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bed != Album || s.idx >= len(s.tracks) {
		return s.bed, ""
	}
	return s.bed, s.tracks[s.idx]
}

// Station reports the SomaFM channel the SomaFM bed plays. Always a station,
// whichever bed is live — it's the one the bed returns to.
func (s *Store) Station() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.station
}

// Album reports the Album bed's selection: one album, a group prefix, or "" for
// the whole share. Like Station, it answers whichever bed is live — it's the
// selection the bed returns to.
//
// This is what was *chosen*, which on a group is not what you're hearing. For
// that, see PlayingAlbum.
func (s *Store) Album() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.album
}

// PlayingAlbum reports the album the current track actually sits in, which on a
// group selection ("streambeats-lofi", or the whole share) is the only way to
// know what's on air. "" when the album bed isn't playing.
//
// Read off the track path rather than tracked separately: the path is what OBS
// is playing, so it can't drift from the audio the way a remembered value could.
func (s *Store) PlayingAlbum() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bed != Album || s.idx >= len(s.tracks) {
		return ""
	}
	return albumFromFile(s.tracks[s.idx], s.musicDir)
}

// Shuffle reports whether the play order is shuffled rather than sequential.
func (s *Store) Shuffle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shuffle
}

// SetShuffle turns shuffling on or off and rebuilds the play order to match,
// keeping the track that's on air at the front so the audio doesn't jump mid-song.
// A no-op off the album bed beyond recording the preference, which the next
// selection then honors.
//
// Not persisted: it lives with the process, like the bed itself. A pod restart
// returns to shuffled, where the album and station survive because they can be
// read back off OBS and this can't.
func (s *Store) SetShuffle(ctx context.Context, on bool) error {
	s.mu.Lock()
	s.shuffle = on
	playing := ""
	if s.bed == Album && s.idx < len(s.tracks) {
		playing = s.tracks[s.idx]
	}
	var err error
	if s.bed == Album {
		err = s.loadAlbumLocked(playing)
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.record()
	slog.InfoContext(ctx, "background audio: shuffle set", "shuffle", on)
	return nil
}

// Albums lists the selectable albums on the share, in display order. Read off
// the filesystem on every call rather than cached at startup: the share is an
// NFS mount Dana drops new music onto, and a list cached at boot would hide a
// new album until the next restart.
func (s *Store) Albums() []string {
	return scanAlbums(s.musicDir)
}

// Groups lists the prefixes that name more than one album, so the picker can
// offer "all of StreamBeats" and "the lofi ones" beside the individual albums.
// Sorted, shortest-first within a name, which puts the broad group above the
// narrow ones it contains.
//
// Derived from the directory names rather than declared: the naming convention
// IS the grouping, so a new album joins its groups by being named like its
// siblings, with nothing to keep in sync.
func (s *Store) Groups() []string {
	return scanGroups(s.Albums())
}

// ValidAlbum reports whether album names something playable on the share: one
// album's directory, or a prefix naming at least one. Unlike ValidStation, which
// checks a fixed list, this has to hit the filesystem — albums are whatever is
// on the share.
func (s *Store) ValidAlbum(album string) bool {
	return album != "" && len(albumsFor(s.Albums(), album)) > 0
}

// ResolveAlbum maps what someone typed onto a selection, or "" if nothing
// matches. In order: an exact directory name, then a prefix naming a group
// ("streambeats", "streambeats-lofi"), then a unique trailing segment, so "rose"
// reaches "streambeats-synthwave-rose" without anyone typing the prefix that
// makes the share sort usefully.
//
// Ambiguity in the trailing-segment step resolves to nothing rather than to a
// guess: there are two albums called Midnight, so once both are on the share
// "midnight" has stopped naming one of them and only the full name will do.
func (s *Store) ResolveAlbum(arg string) string {
	if arg == "" {
		return ""
	}
	albums := s.Albums()
	if slices.Contains(albums, arg) {
		return arg
	}
	// A prefix naming a group is a selection in its own right.
	if len(albumsFor(albums, arg)) > 0 {
		return arg
	}
	var match string
	for _, a := range albums {
		if strings.HasSuffix(a, "-"+arg) {
			if match != "" {
				return ""
			}
			match = a
		}
	}
	return match
}

// SomaFMTrack reports the song playing on the tuned channel, from SomaFM's feed
// for it. Cached, so chat and the console's poll share one fetch. Only ask while
// the SomaFM bed is live — it answers for the tuned channel either way, and on
// another bed that names a song nobody is hearing.
func (s *Store) SomaFMTrack(ctx context.Context) (artist, title string, err error) {
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
	s.mu.Lock()
	s.bed = bed
	if station := stationFromURL(url); station != "" {
		s.station = station
	}
	// The playing file names the album OBS booted on, the way the `input` URL
	// names the station. Without this a restart mid-album reports the whole share
	// and the next advance wanders out of the album that's on air.
	if album := albumFromFile(file, s.musicDir); album != "" {
		s.album = album
	}
	var loadErr error
	if bed == Album {
		// OBS booted straight onto the album — its per-platform default, no Set
		// involved — so nothing has built the play order yet. Without one Advance
		// has nowhere to go and the bed plays a single track and falls silent.
		loadErr = s.loadAlbumLocked(file)
	}
	station := s.station
	s.mu.Unlock()
	s.record()
	if loadErr != nil {
		slog.WarnContext(ctx, "background audio: album is live but its play order is empty",
			"err", loadErr)
	}
	slog.InfoContext(ctx, "background audio: detected bed", "bed", bed, "station", station)
}

// bedFromSettings maps an ffmpeg_source's settings back to the bed that would
// have produced them. A local file under the music share is the album; the
// watchdog's fallback file means SomaFM is still the selected bed and an outage
// is in progress; any other local file is the car-hum drone (that's the only
// other one the image ships); a network source is SomaFM.
func bedFromSettings(settings map[string]any, musicDir string) Bed {
	if local, _ := settings["is_local_file"].(bool); !local {
		return SomaFM
	}
	file, _ := settings["local_file"].(string)
	switch {
	case under(file, musicDir):
		return Album
	case file == FallbackFile:
		// Only the watchdog writes this path, and only while SomaFM is the
		// selected bed. Reading it as CarHum would hand an operator choice to
		// the outage, which switches the watchdog off and strands the stream on
		// the drone once SomaFM comes back.
		return SomaFM
	}
	return CarHum
}

// albumFromFile recovers the album subdir from a track path, so the Store can
// read the live album back off OBS at startup rather than guessing — the
// counterpart of stationFromURL. A track directly at the share root belongs to
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
	station, album := s.station, s.album
	s.mu.Unlock()
	return s.schedule(ctx, Switch{Bed: bed, Station: station, Album: album})
}

// setNow switches to bed and applies it to OBS. Switching to the album
// (re)shuffles the play order and starts on its first track. The store is only
// updated once OBS accepts the change, so a failed switch leaves the reported
// bed truthful.
func (s *Store) setNow(ctx context.Context, bed Bed) error {
	s.mu.Lock()
	if bed == Album {
		if err := s.loadAlbumLocked(""); err != nil {
			s.mu.Unlock()
			// No share mounted / no files: refuse rather than leaving the stream
			// silent. The caller surfaces this; the current bed keeps playing.
			return err
		}
	}
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
	s.fallbackAlbum = false
	s.mu.Unlock()
	s.record()
	// Counted here rather than at the callers so a console switch and a chat
	// switch land on the same counter — they are the same switch.
	instrumentation.BackgroundAudioSelections.Inc(s.platform, string(bed))
	slog.InfoContext(ctx, "background audio: bed switched", "bed", bed, "track", target)
	return nil
}

// SetAlbum narrows the Album bed to one album on the share — or to a group of
// them, when album is a prefix like "streambeats-lofi" — and switches to it, so
// picking is one action rather than "select album, then narrow". Pass "" to
// widen back to the whole share.
func (s *Store) SetAlbum(ctx context.Context, album string) error {
	if album != "" && !s.ValidAlbum(album) {
		return fmt.Errorf("unknown album %q", album)
	}
	s.mu.Lock()
	station := s.station
	s.mu.Unlock()
	return s.schedule(ctx, Switch{Bed: Album, Station: station, Album: album})
}

// SetStation tunes the SomaFM bed to another channel and switches to it, so
// picking a station is one action rather than "select somafm, then tune".
func (s *Store) SetStation(ctx context.Context, station string) error {
	if !ValidStation(station) {
		return fmt.Errorf("unknown somafm station %q", station)
	}
	s.mu.Lock()
	album := s.album
	s.mu.Unlock()
	return s.schedule(ctx, Switch{Bed: SomaFM, Station: station, Album: album})
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
				"err", err, "bed", sw.Bed, "station", sw.Station, "album", sw.Album)
		}
	})
	s.mu.Unlock()
	slog.InfoContext(ctx, "background audio: switch scheduled", "bed", sw.Bed,
		"station", sw.Station, "album", sw.Album, "delay", switchDelay)
	return nil
}

// applyPending records sw's station and album and switches to its bed, rolling
// both back if OBS rejects the switch — a station or album we aren't playing must
// not be the one we report.
//
// The pending flag outlives the station swap and clears only once the bed is
// actually audible. setNow takes the lock itself, so the switch can't be made
// under one; the flag is what covers that gap, and clearing it early would let
// a caller read the new station off a store still playing the old bed.
func (s *Store) applyPending(ctx context.Context, sw Switch) error {
	s.mu.Lock()
	prevStation, prevAlbum := s.station, s.album
	s.station, s.album = sw.Station, sw.Album
	s.mu.Unlock()

	if err := s.setNow(ctx, sw.Bed); err != nil {
		s.mu.Lock()
		s.station, s.album = prevStation, prevAlbum
		s.pending = nil
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
	slog.InfoContext(ctx, "background audio: switch applied", "bed", sw.Bed,
		"station", sw.Station, "album", sw.Album)
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

// Advance moves to the next album track (wrapping at the end) and plays it.
// A no-op unless the album is what's audible — either as the selected bed or as
// the watchdog's fallback for a SomaFM outage — since the callers report the
// media ending, which also happens on the other beds.
//
// Two of them report the same ending: the playback-ended subscription, which
// arrives in milliseconds, and the watchdog tick that backs it up. So an
// advance within advanceDebounce of the last track starting is dropped as that
// duplicate. Tracks run minutes, so nothing legitimate lands that close — and
// the same guard covers an operator's switch, which starts a track through Set
// and would otherwise be stepped over by the outgoing track's own ending.
func (s *Store) Advance(ctx context.Context) error {
	s.mu.Lock()
	if (s.bed != Album && !s.fallbackAlbum) || len(s.tracks) == 0 || time.Since(s.lastStart) < advanceDebounce {
		s.mu.Unlock()
		return nil
	}
	s.lastStart = time.Now()
	current := s.tracks[s.idx]
	s.idx = (s.idx + 1) % len(s.tracks)
	// Re-shuffle on wrap so a long stream doesn't repeat the same 100-track
	// sequence in the same order every time. The fresh order has to keep the
	// track that just finished off the front, or wrapping plays it twice back
	// to back. Sequential order wraps as-is — repeating it is the point.
	if s.idx == 0 && s.shuffle {
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

// SwapToFallback points the source at the bed the audio watchdog rides out a
// SomaFM outage on: the album when the share has tracks, the car-hum drone when
// it doesn't. The album is licence-clean (it was made for this stream) and is
// actual music, so it's the better degraded state; the drone covers an empty or
// unmounted share, where it's the only bed left.
//
// The selected bed is deliberately left alone. The watchdog runs its outage
// machinery only while the store reads SomaFM, so recording the fallback here
// would switch the watchdog off and strand the stream on the fallback for good.
// That is also why the album's play order is (re)built here: nothing else on
// this path builds one, and without it the album plays a single track and falls
// silent — the album=1 / tracks=0 state record() exists to expose.
func (s *Store) SwapToFallback(ctx context.Context) error {
	s.mu.Lock()
	track := ""
	loadErr := s.loadAlbumLocked("")
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
	s.fallbackAlbum = track != ""
	s.lastStart = time.Now()
	s.mu.Unlock()
	s.record()
	slog.InfoContext(ctx, "background audio: swapped to the fallback bed", "file", file)
	return nil
}

// SwapToSomaFM points the source back at the tuned station once the edge is
// serving again, ending any album fallback — the album stops advancing because
// it has stopped playing. The counterpart of SwapToFallback; the selected bed is
// untouched here too, since on this path it never stopped being SomaFM.
func (s *Store) SwapToSomaFM(ctx context.Context) error {
	if err := s.obs.SetNetwork(ctx, InputName, StreamURL(s.Station())); err != nil {
		return fmt.Errorf("swap back to somafm: %w", err)
	}
	s.mu.Lock()
	s.fallbackAlbum = false
	s.mu.Unlock()
	return nil
}

// loadAlbumLocked builds a fresh shuffled play order. playing positions the
// order on a track already on air (pass "" to start at the top), so the next
// advance moves off it instead of restarting it. Caller holds s.mu.
func (s *Store) loadAlbumLocked(playing string) error {
	var tracks []string
	if s.album == "" {
		// The whole share: one walk, skipping the loose files at its root.
		var err error
		if tracks, err = scanTracks(s.musicDir, false); err != nil {
			return fmt.Errorf("scan album tracks under %s: %w", s.musicDir, err)
		}
	} else {
		// One album, or every album under a group prefix. Walked per album and
		// concatenated in sorted order, so an unshuffled group plays album by
		// album instead of interleaving them.
		matched := albumsFor(scanAlbums(s.musicDir), s.album)
		if len(matched) == 0 {
			return fmt.Errorf("no album on the share named or under %q", s.album)
		}
		for _, album := range matched {
			dir := filepath.Join(s.musicDir, album)
			found, err := scanTracks(dir, true)
			if err != nil {
				return fmt.Errorf("scan album tracks under %s: %w", dir, err)
			}
			tracks = append(tracks, found...)
		}
	}
	if len(tracks) == 0 {
		return fmt.Errorf("no album tracks for selection %q under %s", s.album, s.musicDir)
	}
	if s.shuffle {
		shuffle(tracks)
	}
	s.tracks, s.idx = tracks, max(slices.Index(tracks, playing), 0)
	return nil
}

// trackLocked returns the local file a bed should play, or "" for SomaFM.
// Caller holds s.mu.
func (s *Store) trackLocked(bed Bed) string {
	switch bed {
	case CarHum:
		return CarHumFile
	case Album:
		if s.idx < len(s.tracks) {
			return s.tracks[s.idx]
		}
	}
	return ""
}

// apply writes the bed onto the OBS source. Only the album plays unlooped.
func (s *Store) apply(ctx context.Context, bed Bed, track, station string) error {
	if bed == SomaFM {
		return s.obs.SetNetwork(ctx, InputName, StreamURL(station))
	}
	return s.obs.SetLocalFile(ctx, InputName, track, bed != Album)
}

// albumsFor returns the albums a selection covers: the one it names exactly, or
// every album under it when it's a group prefix. Empty when nothing matches.
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

// scanGroups finds the prefixes shared by more than one album — every "-"
// boundary of every name, kept when at least two albums sit under it. Sorted, so
// "streambeats" precedes "streambeats-lofi": broad group first, then the narrow
// ones inside it.
func scanGroups(albums []string) []string {
	counts := map[string]int{}
	for _, a := range albums {
		for i, c := range a {
			if c == '-' {
				counts[a[:i]]++
			}
		}
	}
	var out []string
	for prefix, n := range counts {
		if n > 1 {
			out = append(out, prefix)
		}
	}
	sort.Strings(out)
	return out
}

// scanAlbums lists the selectable albums: every immediate subdirectory of the
// share holding at least one track. Sorted, so the console picker and chat's
// option list read the same order every time. An unreadable share yields no
// albums rather than an error — the caller's next scanTracks reports why.
func scanAlbums(musicDir string) []string {
	entries, err := os.ReadDir(musicDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if tracks, err := scanTracks(filepath.Join(musicDir, e.Name()), true); err == nil && len(tracks) > 0 {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// scanTracks lists the album tracks under dir, sorted so the pre-shuffle order
// is deterministic (the shuffle is what makes playback vary, not readdir order).
//
// loose says whether audio sitting directly in dir counts. Scanning one album,
// it does — that's where its tracks live. Scanning the whole share, it does not:
// albums are the share's SUBDIRECTORIES, and the root holds other audio beside
// them (carsounds.m4a, a 556MB archive) that a flat scan would shuffle in as one
// enormous track.
func scanTracks(dir string, loose bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !audioExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		if !loose && filepath.Dir(path) == filepath.Clean(dir) {
			return nil // loose file at the share root, not an album track
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
// else: Advance returns silently when there is nothing to advance to, so an
// album bed with no tracks plays its boot track and then falls silent with OBS
// still reporting a healthy source. album=1 with tracks=0 is that state, and
// it is the only warning of it.
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
