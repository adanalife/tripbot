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
	// SomaFM is the Groove Salad Classic internet-radio stream. Its music is not
	// cleared for our rebroadcast — it trips YouTube's Content ID and the other
	// platforms' audio ID — so it's a Twitch-only default, by tolerance rather
	// than licence.
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
)

// audioExts are the track extensions scanned for under MusicDir. Matches the
// find in the obs repo's script/background-audio.sh.
var audioExts = map[string]bool{".mp3": true, ".flac": true, ".m4a": true, ".ogg": true}

// OBS is the subset of pkg/obs the store drives. Injectable so switching logic
// unit-tests without a real OBS WebSocket.
type OBS interface {
	// SetNetwork flips the source back to its configured stream URL.
	SetNetwork(ctx context.Context, inputName string) error
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
	platform string // stamped on the bed metrics; each platform picks its own bed

	mu     sync.Mutex
	bed    Bed
	tracks []string // shuffled play order; rebuilt each time the album starts
	idx    int
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
	return &Store{obs: o, bed: bed, musicDir: musicDir, platform: platform}
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
	s.mu.Lock()
	s.bed = bed
	var loadErr error
	if bed == Album {
		// OBS booted straight onto the album — its per-platform default, no Set
		// involved — so nothing has built the play order yet. Without one Advance
		// has nowhere to go and the bed plays a single track and falls silent.
		loadErr = s.loadAlbumLocked(file)
	}
	s.mu.Unlock()
	s.record()
	if loadErr != nil {
		slog.WarnContext(ctx, "background audio: album is live but its play order is empty",
			"err", loadErr)
	}
	slog.InfoContext(ctx, "background audio: detected bed at startup", "bed", bed)
}

// bedFromSettings maps an ffmpeg_source's settings back to the bed that would
// have produced them. A local file under the music share is the album; any
// other local file is the car-hum drone (that's the only other one the image
// ships); a network source is SomaFM.
func bedFromSettings(settings map[string]any, musicDir string) Bed {
	if local, _ := settings["is_local_file"].(bool); !local {
		return SomaFM
	}
	file, _ := settings["local_file"].(string)
	if under(file, musicDir) {
		return Album
	}
	return CarHum
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
	s.mu.Unlock()

	if err := s.apply(ctx, bed, target); err != nil {
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

// Advance moves to the next album track (wrapping at the end) and plays it.
// A no-op unless the album is the live bed — the watchdog calls this whenever
// OBS reports the media ended, which also happens on the other beds.
func (s *Store) Advance(ctx context.Context) error {
	s.mu.Lock()
	if s.bed != Album || len(s.tracks) == 0 {
		s.mu.Unlock()
		return nil
	}
	last := s.tracks[s.idx]
	s.idx = (s.idx + 1) % len(s.tracks)
	// Re-shuffle on wrap so a long stream doesn't repeat the same 100-track
	// sequence in the same order every time. The reshuffle can deal the track
	// that just finished back to the front, which is the one ordering a listener
	// would actually notice, so move it out of the way.
	if s.idx == 0 {
		shuffle(s.tracks)
		if len(s.tracks) > 1 && s.tracks[0] == last {
			s.tracks[0], s.tracks[1] = s.tracks[1], s.tracks[0]
		}
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
	tracks, err := scanTracks(s.musicDir)
	if err != nil {
		return fmt.Errorf("scan album tracks under %s: %w", s.musicDir, err)
	}
	if len(tracks) == 0 {
		return fmt.Errorf("no album tracks under %s", s.musicDir)
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
func (s *Store) apply(ctx context.Context, bed Bed, track string) error {
	if bed == SomaFM {
		return s.obs.SetNetwork(ctx, InputName)
	}
	return s.obs.SetLocalFile(ctx, InputName, track, bed != Album)
}

// scanTracks lists the album tracks under dir, sorted so the pre-shuffle order
// is deterministic (the shuffle is what makes playback vary, not readdir order).
//
// Albums are SUBDIRECTORIES of the share; loose files at its root are not
// tracks. The share holds other audio beside the albums — carsounds.m4a, a
// 556MB archive — and a flat scan would shuffle that in as one enormous track.
//
// ponytail: every album subdirectory is one pool, which with a single album is
// that album. Add a picker when a second one shows up and they shouldn't
// interleave.
func scanTracks(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !audioExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		if filepath.Dir(path) == filepath.Clean(dir) {
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
