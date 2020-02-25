package vlc

import (
	"math/rand"
	"os"
	"path/filepath"

	theirVlc "github.com/adrg/libvlc-go"
	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

var player *theirVlc.Player
var playlist *theirVlc.ListPlayer
var mediaList *theirVlc.MediaList

func Init() {
	var err error

	//TODO: add more flags (--no-audio?)
	if err = theirVlc.Init("--quiet"); err != nil {
		terrors.Fatal(err, "error initializing VLC")
	}

	// create a new player
	playlist, err = theirVlc.NewListPlayer()
	if err != nil {
		terrors.Fatal(err, "error creating VLC playlist player")
	}

	player, err = playlist.Player()
	if err != nil {
		terrors.Fatal(err, "error fetching VLC player")
	}

	mediaList, err = theirVlc.NewMediaList()
	if err != nil {
		terrors.Fatal(err, "error creating VLC media list")
	}

	err = playlist.SetMediaList(mediaList)
	if err != nil {
		terrors.Fatal(err, "error setting VLC media list")
	}

	// loop forever
	err = playlist.SetPlaybackMode(theirVlc.Loop)
	if err != nil {
		terrors.Fatal(err, "error setting VLC playback mode")
	}

	loadMedia()
	createEventHandler()
}

func Shutdown() {
	player.Stop()
	player.Release()
	theirVlc.Release()
}

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

	spew.Dump(mediaList)
}

func PlayRandom() error {
	count, err := mediaList.Count()
	if err != nil {
		terrors.Log(err, "error counting media in VLC media list")
	}

	random := rand.Intn(count)

	// start playing the media
	return playlist.PlayAtIndex(uint(random))
}
