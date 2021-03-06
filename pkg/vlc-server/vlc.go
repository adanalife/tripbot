package vlcServer

import (
	"log"
	"os"
	"path/filepath"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	libvlc "github.com/adrg/libvlc-go/v3"
)

var player *libvlc.Player
var playlist *libvlc.ListPlayer
var mediaList *libvlc.MediaList
var videoFiles []string

//TODO: figure out if vdpau_avcodec can be better than none
//TODO: there are a ton of potentially-useful avcodec flags
//TODO: break some of these into ENV vars
var vlcCmdFlags = []string{
	"--ignore-config", // ignore any config files that might get loaded
	"--fullscreen",    // start fullscreened
	"--no-audio",      // none of the videos have audio
	// "--network-caching", "500", // network cache (in ms)
	"--file-caching", "1111", // file cache (in ms)
	"--width", "1920",
	"--height", "1080",
	"--canvas-width", "1920",
	"--canvas-height", "1080",
	// "--aspect-ratio", "16:9",
}

var vlcLinuxSpecificFlags = []string{
	"--vout", "x11", // use X11 (and skip vdpau, improves performance)
	"--avcodec-hw", "vdpau_avcodec", // can be none, vdpau_avcodec, or cuda
	// "--avcodec-dr", "0",

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

// InitPlayer creates a VLC player and sets up a playlist
func InitPlayer() {
	startVLC()
	createPlayer()
	setToLoop()
	loadMedia()
}

// Shutdown cleans up VLC as best it can
//TODO: are there more things to close gracefully?
func Shutdown() {
	if helpers.RunningOnDarwin() {
		log.Println("not stopping VLC cause we're on darwin")
		return
	}
	err := player.Stop()
	if err != nil {
		terrors.Log(err, "error stopping player")
	}
	err = player.Release()
	if err != nil {
		terrors.Log(err, "error releasing player")
	}
	err = libvlc.Release()
	if err != nil {
		terrors.Log(err, "error releasing libvlc")
	}
}

// currentlyPlaying finds the currently-playing video path
// (it's pretty hacky right now)
func currentlyPlaying() string {
	cur, err := player.Media()
	if err != nil {
		terrors.Log(err, "error fetching currently-playing media")
	}

	// get media path
	path, err := cur.Location()
	if err != nil {
		terrors.Log(err, "error fetching currently-playing media")
	}

	// strip the path off and just return the filename
	return filepath.Base(path)
}

func startVLC() {
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
		vlcCmdFlags = append(vlcCmdFlags, vlcLinuxSpecificFlags...)
	}

	if helpers.RunningOnWindows() {
		vlcCmdFlags = append(vlcCmdFlags, vlcWindowsSpecificFlags...)
	}

	// start up VLC with given command flags
	if err := libvlc.Init(vlcCmdFlags...); err != nil {
		terrors.Fatal(err, "error initializing VLC")
	}
}

func createPlayer() {
	var err error

	// create a new playlist-player
	playlist, err = libvlc.NewListPlayer()
	if err != nil {
		terrors.Fatal(err, "error creating VLC playlist player")
	}

	// save the player so we can use it later
	player, err = playlist.Player()
	if err != nil {
		terrors.Fatal(err, "error fetching VLC player")
	}

	// this will store all of our videos
	mediaList, err = libvlc.NewMediaList()
	if err != nil {
		terrors.Fatal(err, "error creating VLC media list")
	}

	// plug our medialist into the player
	err = playlist.SetMediaList(mediaList)
	if err != nil {
		terrors.Fatal(err, "error setting VLC media list")
	}
}

func setToLoop() {
	// set the player to loop forever
	err := playlist.SetPlaybackMode(libvlc.Loop)
	if err != nil {
		terrors.Fatal(err, "error setting VLC playback mode")
	}
}

func loadMedia() {
	loadLocalMedia()
}

// loadLocalMedia walks the VideoDir and adds all videos to
// the playlist.
func loadLocalMedia() {
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
		videoFiles = append(videoFiles, videoFile)
		return nil
	})
	if err != nil {
		terrors.Fatal(err, "error walking VideoDir")
	}

	// loop over the files and add their paths to VLC
	for _, file := range filePaths {
		// add the media to VLC
		err = mediaList.AddMediaFromPath(file)
		if err != nil {
			terrors.Fatal(err, "error adding files to VLC media list")
		}
	}
}
