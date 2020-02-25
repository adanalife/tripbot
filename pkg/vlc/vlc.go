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
var mediaToVid = make(map[theirVlc.Media]string)

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
}

func Shutdown() {
	player.Stop()
	player.Release()
	theirVlc.Release()
}

func CurrentlyPlaying() string {
	// count, err := mediaList.Count()
	// if err != nil {
	// 	terrors.Log(err, "error counting media in VLC media list")
	// }

	cur, err := player.Media()
	if err != nil {
		terrors.Log(err, "error fetching currently-playing media")
	}

	return mediaToVid[*cur]

	// for i := 0; i < count; i++ {
	// 	m, err := mediaList.MediaAtIndex(i)
	// 	if err != nil {
	// 		terrors.Log(err, "error fetching currently-playing media")
	// 	}
	// 	if cur == m {
	// 		return allVids[i]
	// 	}
	// }
	// return ""
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

	i := uint(0)
	for _, file := range files {
		// add the media to VLC
		err = mediaList.AddMediaFromPath(file)
		if err != nil {
			terrors.Fatal(err, "error adding files to VLC media list")
		}
		// get that media object back
		m, err := mediaList.MediaAtIndex(i)
		if err != nil {
			terrors.Log(err, "error fetching media at index")
		}
		// store the media in a map
		mediaToVid[*m] = files[i]
		i++
	}

	spew.Dump(mediaToVid)
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
