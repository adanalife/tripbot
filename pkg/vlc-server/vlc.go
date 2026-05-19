package vlcServer

import (
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
// VLC_CANVAS_WIDTH, VLC_CANVAS_HEIGHT). Defaults match what was
// previously hardcoded here, so this is a no-op refactor unless an
// env var is explicitly set.
var vlcCmdFlags []string

// mediaOptions returns the per-Media options driven by VLC_OUTPUT.
// libvlc's --sout takes effect only when set on the media object itself —
// passing --sout to libvlc.Init does NOT activate the stream-out chain.
// `sout-keep` preserves the chain across playlist transitions so OBS
// doesn't see EOF on every clip change.
//
//   rtsp   — RTSP listener only (container default).
//   window — no sout; libvlc plays to its native window via --vout.
//   both   — duplicate to a local display target and the RTSP listener.
func mediaOptions() []string {
	const rtspChain = "rtp{sdp=rtsp://:8554/dashcam}"
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
// the container no longer ships an X server) and --avcodec-hw
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
	if err := s.setToLoop(); err != nil {
		return err
	}
	return s.loadMedia()
}

// Shutdown cleans up VLC as best it can. The release order (player.Stop →
// player.Release → libvlc.Release) is order-sensitive — releasing the
// libvlc instance before the player segfaults. Preserve verbatim.
//
//TODO: are there more things to close gracefully?
func (s *Server) Shutdown() {
	if helpers.RunningOnDarwin() {
		slog.Info("not stopping VLC on darwin")
		return
	}
	if s.Player != nil {
		if err := s.Player.Stop(); err != nil {
			slog.Error("error stopping player", "err", err)
		}
		if err := s.Player.Release(); err != nil {
			slog.Error("error releasing player", "err", err)
		}
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
	}

	// get media path
	path, err := cur.Location()
	if err != nil {
		slog.Error("error fetching currently-playing media", "err", err)
	}

	// strip the path off and just return the filename
	return filepath.Base(path)
}

func startVLC() error {
	// build the base flag set from the static list + config-driven tuning
	// values. Defaults in pkg/config/vlc-server reproduce what used to be
	// hardcoded here, so unset env vars yield identical behavior.
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
