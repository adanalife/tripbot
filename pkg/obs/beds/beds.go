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
	"slices"
	"sort"
	"strings"
	"sync"

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
// image bakes in (carhum) and where cdk8s mounts the obs-music PVC. tripbot
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
	station string   // SomaFM channel id the SomaFM bed plays
	album   string   // album subdir the Album bed plays; "" is the whole share
	tracks  []string // shuffled play order; rebuilt each time the album starts
	idx     int
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

// Album reports the album subdir the Album bed plays, or "" for the whole share
// shuffled together. Like Station, it answers whichever bed is live — it's the
// selection the bed returns to.
func (s *Store) Album() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.album
}

// Albums lists the selectable albums on the share, in display order. Read off
// the filesystem on every call rather than cached at startup: the share is an
// NFS mount Dana drops new music onto, and a list cached at boot would hide a
// new album until the next restart.
func (s *Store) Albums() []string {
	return scanAlbums(s.musicDir)
}

// ValidAlbum reports whether album names a directory on the share holding at
// least one track. Unlike ValidStation, which checks a fixed list, this has to
// hit the filesystem — albums are whatever is on the share.
func (s *Store) ValidAlbum(album string) bool {
	return album != "" && slices.Contains(s.Albums(), album)
}

// ResolveAlbum maps what someone typed onto an album on the share, or "" if
// nothing matches. Exact directory names always win; failing that, a unique
// trailing segment does, so "rose" reaches "synthwave-rose" without anyone
// typing the genre prefix that makes the share sort usefully. Ambiguity resolves
// to nothing rather than to a guess — two albums ending "-midnight" mean the
// shorthand has stopped being a name.
func (s *Store) ResolveAlbum(arg string) string {
	albums := s.Albums()
	if slices.Contains(albums, arg) {
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
// on, so a tripbot restart doesn't report a stale guess. A failure here is not
// fatal — the configured default stands and the next switch corrects it.
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
	slog.InfoContext(ctx, "background audio: detected bed at startup", "bed", bed, "station", station)
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

// Set switches to bed and applies it to OBS. Switching to the album (re)shuffles
// the play order and starts on its first track. The store is only updated once
// OBS accepts the change, so a failed switch leaves the reported bed truthful.
func (s *Store) Set(ctx context.Context, bed Bed) error {
	if !Valid(bed) {
		return fmt.Errorf("unknown background-audio bed %q", bed)
	}

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
	s.mu.Unlock()
	s.record()
	// Counted here rather than at the callers so a console switch and a chat
	// switch land on the same counter — they are the same switch.
	instrumentation.BackgroundAudioSelections.Inc(s.platform, string(bed))
	slog.InfoContext(ctx, "background audio: bed switched", "bed", bed, "track", target)
	return nil
}

// SetAlbum narrows the Album bed to one album on the share and switches to it,
// so picking an album is one action rather than "select album, then narrow".
// Mirrors SetStation, including the rollback: an album we aren't playing must
// not be the one we report. Pass "" to widen back to the whole share.
func (s *Store) SetAlbum(ctx context.Context, album string) error {
	if album != "" && !s.ValidAlbum(album) {
		return fmt.Errorf("unknown album %q", album)
	}
	s.mu.Lock()
	prev := s.album
	s.album = album
	s.mu.Unlock()

	if err := s.Set(ctx, Album); err != nil {
		s.mu.Lock()
		s.album = prev
		s.mu.Unlock()
		return err
	}
	slog.InfoContext(ctx, "background audio: album selected", "album", album)
	return nil
}

// SetStation tunes the SomaFM bed to another channel and switches to it, so
// picking a station is one action rather than "select somafm, then tune". The
// station is recorded before the switch so Set applies it, and rolled back if
// OBS rejects the switch — a station we aren't playing must not be the one we
// report.
func (s *Store) SetStation(ctx context.Context, station string) error {
	if !ValidStation(station) {
		return fmt.Errorf("unknown somafm station %q", station)
	}
	s.mu.Lock()
	prev := s.station
	s.station = station
	s.mu.Unlock()

	if err := s.Set(ctx, SomaFM); err != nil {
		s.mu.Lock()
		s.station = prev
		s.mu.Unlock()
		return err
	}
	slog.InfoContext(ctx, "background audio: station tuned", "station", station)
	return nil
}

// Advance moves to the next album track (wrapping at the end) and plays it.
// A no-op unless the album is the live bed — the watchdog calls this whenever
// OBS reports the media ended, which also happens on the other beds.
func (s *Store) Advance(ctx context.Context) error {
	s.mu.Lock()
	if s.bed != Album || len(s.tracks) == 0 {
		s.mu.Unlock()
		return nil
	}
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

// loadAlbumLocked builds a fresh shuffled play order. playing positions the
// order on a track already on air (pass "" to start at the top), so the next
// advance moves off it instead of restarting it. Caller holds s.mu.
func (s *Store) loadAlbumLocked(playing string) error {
	dir := albumDir(s.musicDir, s.album)
	tracks, err := scanTracks(dir, s.album != "")
	if err != nil {
		return fmt.Errorf("scan album tracks under %s: %w", dir, err)
	}
	if len(tracks) == 0 {
		return fmt.Errorf("no album tracks under %s", dir)
	}
	shuffle(tracks)
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

// albumDir is the directory a selection plays out of: one album's own
// subdirectory, or the whole share when nothing is selected.
func albumDir(musicDir, album string) string {
	if album == "" {
		return musicDir
	}
	return filepath.Join(musicDir, album)
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

// TrackTitle turns an album track path into something worth showing a human:
// the filename without its extension. "" stays "" (the other beds have no
// track), which is how callers tell "no track" from "a track".
func TrackTitle(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
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
