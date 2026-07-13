package vlcServer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	libvlc "github.com/adrg/libvlc-go/v3"
)

//TODO: figure out if vdpau_avcodec can be better than none
//TODO: there are a ton of potentially-useful avcodec flags

// platform-invariant flags that never need per-host tuning.
// Stays package-level: immutable build-time configuration; cgo init order
// depends on these being package-scoped, not struct fields.
var vlcStaticFlags = []string{
	"--ignore-config", // ignore any config files that might get loaded
	"--fullscreen",    // start fullscreened (only matters when VLC_OUTPUT renders a window; no-op headless)
	"--no-audio",      // none of the videos have audio
	// "--network-caching", "500", // network cache (in ms)
	// "--aspect-ratio", "16:9",
}

// vlcCmdFlags is built lazily in startVLC() from vlcStaticFlags +
// per-host tuning values pulled from config (VLC_FILE_CACHING,
// VLC_CANVAS_WIDTH, VLC_CANVAS_HEIGHT).
var vlcCmdFlags []string

// mediaOptions returns the per-Media options driven by VLC_OUTPUT.
// libvlc's --sout takes effect only when set on the media object itself —
// passing --sout to libvlc.Init does NOT activate the stream-out chain.
//
// The chain is `gather:rtp{...}` + `sout-keep`, and both halves are
// load-bearing for a seamless stream. `sout-keep` alone preserves the sout
// *instance* across playlist transitions but still ends the RTP stream at
// every media change, so each RTSP client (OBS) gets EOF every clip, eats
// its reconnect delay, and rejoins mid-GOP — the visible inter-clip seam.
// `gather` merges the successive Media into one continuous elementary
// stream with monotonic timestamps, so clients never see a boundary at
// all. gather requires the clips to share codec/dimensions/rate, which the
// transcoded corpus guarantees.
//
//	rtsp   — RTSP listener only (container default).
//	window — no sout; libvlc plays to its native window via --vout.
//	both   — duplicate to a local display target and the RTSP listener.
func mediaOptions() []string {
	const rtspChain = "gather:rtp{sdp=rtsp://:8554/dashcam}"
	switch c.Conf.VlcOutput {
	case "window":
		return nil
	case "both":
		return []string{
			":sout=#duplicate{dst=display,dst=" + rtspChain + "}",
			":sout-keep",
		}
	case "rtsp":
		return []string{
			":sout=#" + rtspChain,
			":sout-keep",
		}
	default:
		terrors.Fatal(fmt.Errorf("unrecognized VLC_OUTPUT=%q (want rtsp|window|both)", c.Conf.VlcOutput), "")
		return nil // unreachable
	}
}

// linuxSpecificFlags returns the Linux-only VLC flags, sourced from
// config (VLC_VOUT, VLC_AVCODEC_HW). Defaults: --vout dummy (headless;
// the container has no X server) and --avcodec-hw
// vdpau_avcodec (can be none, vdpau_avcodec, or cuda). On a Linux dev
// host where you want a preview window, set VLC_VOUT=x11 and
// VLC_OUTPUT=window (or both).
func linuxSpecificFlags() []string {
	return []string{
		"--vout", c.Conf.VlcVout,
		"--avcodec-hw", c.Conf.VlcAvcodecHw,
		// "--avcodec-dr", "0",
	}
}

var vlcWindowsSpecificFlags = []string{
	// we do this so the window is always visible, otherwise
	// when you minimize, it hides the video in OBS
	"--video-wallpaper",
}

// these get added if verbose flag is NOT set
var vlcNotVerboseFlags = []string{
	"--quiet", // reduce terminal output
}

// these add a lot more output
var vlcVerboseFlags = []string{
	"-vv", // be very verbose (used for debugging)
}

// initPlayer creates a VLC player and sets up a playlist. Order is
// load-bearing: libvlc.Init must happen before any *libvlc* object can be
// created, and the media list must be attached to the playlist before
// media is added to it. Preserve this sequence verbatim.
func (s *Server) initPlayer() error {
	if err := startVLC(); err != nil {
		return err
	}
	if err := s.createPlayer(); err != nil {
		return err
	}
	if err := s.initSwapGapEvents(); err != nil {
		return err
	}
	if err := s.setToLoop(); err != nil {
		return err
	}
	if err := s.loadMedia(); err != nil {
		return err
	}
	return s.primePlayer()
}

// primePlayer sets the underlying media player's media to the first loaded
// clip so it is non-nil before the first PlayAtIndex.
//
// This works around a libvlc 3.0.x bug: libvlc_media_list_player_play_item_-
// at_index reads the player's *existing* media (libvlc_media_player_get_media)
// before swapping in the requested one, and returns -1 when that prior media
// is nil — i.e. on the very first play against a freshly-created list player —
// even though set_current_playing_item succeeds and playback actually starts.
// Without priming, the first play after boot (the startup resume) reports a
// spurious failure, so ResumeFromLastPlayed falls back to PlayRandom and a
// restart never resumes the clip it left off on. The prime doesn't start
// playback; it only seeds the player so the first real play returns correctly.
func (s *Server) primePlayer() error {
	count, err := s.MediaList.Count()
	if err != nil {
		return fmt.Errorf("counting media to prime player: %w", err)
	}
	if count == 0 {
		return nil
	}
	media, err := s.MediaList.MediaAtIndex(0)
	if err != nil {
		return fmt.Errorf("fetching media to prime player: %w", err)
	}
	if err := s.Player.SetMedia(media); err != nil {
		return fmt.Errorf("priming player media: %w", err)
	}
	return nil
}

// Health returns nil when the server is ready to serve a viewer, or an
// error describing why it isn't. Used by /health/ready (readinessHandler)
// so K8s readiness probes reflect libvlc player state.
//
// Healthy means s.Player is non-nil AND its libvlc MediaState is one of
// the "active" states: Opening, Buffering, Playing, Paused. Stopped,
// Ended, Error, and NothingSpecial are treated as unhealthy — Ended is
// excluded conservatively because while a looping playlist passes
// through it between clips, a sustained Ended generally indicates a
// stalled player (the loop logic isn't advancing), and we'd rather fail
// readiness too eagerly than too lazily.
func (s *Server) Health() error {
	if s.Player == nil {
		return errors.New("player not initialized")
	}
	state, err := s.Player.MediaState()
	if err != nil {
		return fmt.Errorf("reading player state: %w", err)
	}
	switch state {
	case libvlc.MediaOpening, libvlc.MediaBuffering, libvlc.MediaPlaying, libvlc.MediaPaused:
		return nil
	default:
		return fmt.Errorf("player not running (state=%v)", state)
	}
}

// Shutdown drains the HTTP server (bounded by ctx) then cleans up libvlc.
// The libvlc release order (player.Stop → player.Release → libvlc.Release)
// is order-sensitive — releasing the libvlc instance before the player
// segfaults. Preserve verbatim.
//
// New guarantees that on success s.Player is non-nil; on failure New
// releases any partially-allocated libvlc resources itself and returns
// (nil, err). Callers that hold a non-nil *Server therefore never observe
// a nil Player, so this method assumes both fields are valid.
//
// s.http may be nil if Shutdown is called before Start populated it; in
// that case there's nothing to drain and we skip straight to libvlc
// cleanup.
func (s *Server) Shutdown(ctx context.Context) {
	if s.http != nil {
		slog.InfoContext(ctx, "shutting down VLC web server")
		if err := s.http.Shutdown(ctx); err != nil {
			slog.ErrorContext(ctx, "error during VLC web server shutdown", "err", err)
		}
	}
	if helpers.RunningOnDarwin() {
		slog.Info("not stopping VLC on darwin")
		return
	}
	if err := s.Player.Stop(); err != nil {
		slog.Error("error stopping player", "err", err)
	}
	if err := s.Player.Release(); err != nil {
		slog.Error("error releasing player", "err", err)
	}
	if err := libvlc.Release(); err != nil {
		slog.Error("error releasing libvlc", "err", err)
	}
}

// currentlyPlaying finds the currently-playing video path
// (it's pretty hacky right now)
func (s *Server) currentlyPlaying() string {
	cur, err := s.Player.Media()
	if err != nil {
		slog.Error("error fetching currently-playing media", "err", err)
		return ""
	}
	if cur == nil {
		// no media loaded in the player
		return ""
	}

	// get media path
	path, err := cur.Location()
	if err != nil {
		slog.Error("error fetching currently-playing media location", "err", err)
		return ""
	}

	// strip the path off and just return the filename
	return filepath.Base(path)
}

func startVLC() error {
	// build the base flag set from the static list + config-driven tuning
	// values (defaults live in pkg/config/vlc-server)
	canvasW := strconv.Itoa(c.Conf.VlcCanvasWidth)
	canvasH := strconv.Itoa(c.Conf.VlcCanvasHeight)
	vlcCmdFlags = append([]string{}, vlcStaticFlags...)
	vlcCmdFlags = append(vlcCmdFlags,
		"--file-caching", strconv.Itoa(c.Conf.VlcFileCaching),
		"--width", canvasW,
		"--height", canvasH,
		"--canvas-width", canvasW,
		"--canvas-height", canvasH,
	)

	// set command line flags
	if c.Conf.VlcVerbose {
		vlcCmdFlags = append(vlcCmdFlags, vlcVerboseFlags...)
		// we use syslog on linux
		if helpers.RunningOnLinux() {
			// post debug output to syslog
			vlcCmdFlags = append(vlcCmdFlags, "--syslog-debug")
		}
	} else {
		vlcCmdFlags = append(vlcCmdFlags, vlcNotVerboseFlags...)
		if helpers.RunningOnLinux() {
			// log to syslog
			vlcCmdFlags = append(vlcCmdFlags, "--syslog")
		}
	}

	if helpers.RunningOnLinux() {
		vlcCmdFlags = append(vlcCmdFlags, linuxSpecificFlags()...)
	}

	if helpers.RunningOnWindows() {
		vlcCmdFlags = append(vlcCmdFlags, vlcWindowsSpecificFlags...)
	}

	// start up VLC with given command flags
	if err := libvlc.Init(vlcCmdFlags...); err != nil {
		return fmt.Errorf("error initializing VLC: %w", err)
	}
	return nil
}

func (s *Server) createPlayer() error {
	// create a new playlist-player
	playlist, err := libvlc.NewListPlayer()
	if err != nil {
		return fmt.Errorf("error creating VLC playlist player: %w", err)
	}
	s.Playlist = playlist

	// save the player so we can use it later
	player, err := playlist.Player()
	if err != nil {
		return fmt.Errorf("error fetching VLC player: %w", err)
	}
	s.Player = player

	// this will store all of our videos
	mediaList, err := libvlc.NewMediaList()
	if err != nil {
		return fmt.Errorf("error creating VLC media list: %w", err)
	}
	s.MediaList = mediaList

	// plug our medialist into the player
	if err := playlist.SetMediaList(mediaList); err != nil {
		return fmt.Errorf("error setting VLC media list: %w", err)
	}
	return nil
}

func (s *Server) setToLoop() error {
	// set the player to loop forever
	if err := s.Playlist.SetPlaybackMode(libvlc.Loop); err != nil {
		return fmt.Errorf("error setting VLC playback mode: %w", err)
	}
	return nil
}

func (s *Server) loadMedia() error {
	return s.loadLocalMedia()
}

// loadLocalMedia walks the VideoDir and adds all videos to
// the playlist.
func (s *Server) loadLocalMedia() error {
	var filePaths []string
	// add all files from the VideoDir to the medialist
	err := filepath.Walk(c.Conf.VideoDir, func(path string, info os.FileInfo, err error) error {
		// skip the dir itself
		if path == c.Conf.VideoDir {
			return nil
		}
		// skip non-video files
		if filepath.Ext(path) != ".MP4" {
			return nil
		}
		// add full path to list of paths
		filePaths = append(filePaths, path)
		// add the video filename to videoFiles list
		videoFile := filepath.Base(path)
		s.VideoFiles = append(s.VideoFiles, videoFile)
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking VideoDir: %w", err)
	}

	// loop over the files and add their paths to VLC
	for _, file := range filePaths {
		media, err := libvlc.NewMediaFromPath(file)
		if err != nil {
			return fmt.Errorf("error creating media from path: %w", err)
		}
		if err := media.AddOptions(mediaOptions()...); err != nil {
			return fmt.Errorf("error setting media options: %w", err)
		}
		if err := s.MediaList.AddMedia(media); err != nil {
			return fmt.Errorf("error adding files to VLC media list: %w", err)
		}
	}
	return nil
}
