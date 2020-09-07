package vlcServer

import (
	"log"
	"os"
	"path/filepath"

	"github.com/adanalife/tripbot/pkg/config"
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
	"--vout", "x11",   // use X11 (and skip vdpau)
	"--no-audio",                // none of the videos have audio
	"--network-caching", "6666", // network cache (in ms)
	"--file-caching", "11111", // file cache (in ms)
	// can be none, vdpau_avcodec, or cuda
	"--avcodec-hw", "cuda",
	"--width", "1920",
	"--height", "1080",
	"--canvas-width", "1920",
	"--canvas-height", "1080",
	// "--aspect-ratio", "16:9",
}

// these get added if verbose flag is NOT set
var vlcNotVerboseFlags = []string{
	"--syslog", // log to syslog
	"--quiet",  // reduce terminal output
}

// these add a lot more output
var vlcVerboseFlags = []string{
	"-vv",            // be very verbose (used for debugging)
	"--syslog-debug", // post debug output to syslog
}

// Init creates a VLC player and sets up a playlist
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

	return path
}

func startVLC() {
	// set command line flags
	if config.VlcVerbose {
		vlcCmdFlags = append(vlcCmdFlags, vlcVerboseFlags...)
	} else {
		vlcCmdFlags = append(vlcCmdFlags, vlcNotVerboseFlags...)
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

// loadMedia walks the VideoDir and adds all videos to
// the playlist.
func loadMedia() {
	var filePaths []string
	// add all files from the VideoDir to the medialist
	err := filepath.Walk(config.VideoDir, func(path string, info os.FileInfo, err error) error {
		// skip the dir itself
		if path == config.VideoDir {
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
