package vlcServer

import (
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"

	libvlc "github.com/adrg/libvlc-go/v3"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

var player *libvlc.Player
var playlist *libvlc.ListPlayer
var mediaList *libvlc.MediaList

// Init creates a VLC player and sets up a playlist
func InitPlayer() {
	var err error

	// the vids dont have audio anyway, so add --no-audio
	//TODO: move these to a const
	//TODO: this should probably include/exclude hardware decoding
	if err = libvlc.Init("--quiet", "--no-audio", "--network-caching", "6666"); err != nil {
		terrors.Fatal(err, "error initializing VLC")
	}

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

	// set the player to loop forever
	err = playlist.SetPlaybackMode(libvlc.Loop)
	if err != nil {
		terrors.Fatal(err, "error setting VLC playback mode")
	}

	loadMedia()
}

// Shutdown cleans up VLC as best it can
//TODO: are there more things to close gracefully?
func Shutdown() {
	if runtime.GOOS == "darwin" {
		log.Println("not stopping VLC cause we're on darwin")
		return
	}
	player.Stop()
	player.Release()
	libvlc.Release()
}

// CurrentlyPlaying finds the currently-playing video path
// (it's pretty hacky right now)
func CurrentlyPlaying() string {
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

// loadMedia walks the VideoDir and adds all videos to
// the playlist.
func loadMedia() {
	var files []string
	// add all files from the VideoDir to the medialist
	err := filepath.Walk(config.VideoDir, func(path string, info os.FileInfo, err error) error {
		// skip the dir itself
		if path == config.VideoDir {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		terrors.Fatal(err, "error walking VideoDir")
	}

	for _, file := range files {
		// add the media to VLC
		err = mediaList.AddMediaFromPath(file)
		if err != nil {
			terrors.Fatal(err, "error adding files to VLC media list")
		}
	}
}

// PlayRandom plays a random file from the playlist
func PlayRandom() error {
	count, err := mediaList.Count()
	if err != nil {
		terrors.Log(err, "error counting media in VLC media list")
	}

	random := rand.Intn(count)

	// start playing the media
	return playlist.PlayAtIndex(uint(random))
}
