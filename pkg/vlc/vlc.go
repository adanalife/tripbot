package vlc

import (
	"math/rand"
	"os"
	"path/filepath"

	theirVlc "github.com/adrg/libvlc-go"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

var player *theirVlc.Player
var playlist *theirVlc.ListPlayer
var mediaList *theirVlc.MediaList

//TODO: this map is gonna be huge, with 4000+ videos
// it also might do irresponsible things with pointers?
// (see this commit history for another approach)
var mediaToVid = make(map[theirVlc.Media]string)

func Init() {
	var err error

	// the vids dont have audio anyway, so add --no-audio
	if err = theirVlc.Init("--quiet", "--no-audio"); err != nil {
		terrors.Fatal(err, "error initializing VLC")
	}

	// create a new playlist-player
	playlist, err = theirVlc.NewListPlayer()
	if err != nil {
		terrors.Fatal(err, "error creating VLC playlist player")
	}

	// save the player so we can use it later
	player, err = playlist.Player()
	if err != nil {
		terrors.Fatal(err, "error fetching VLC player")
	}

	// this will store all of our videos
	mediaList, err = theirVlc.NewMediaList()
	if err != nil {
		terrors.Fatal(err, "error creating VLC media list")
	}

	// plug our medialist into the player
	err = playlist.SetMediaList(mediaList)
	if err != nil {
		terrors.Fatal(err, "error setting VLC media list")
	}

	// set the player to loop forever
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

	cur, err := player.Media()
	if err != nil {
		terrors.Log(err, "error fetching currently-playing media")
	}

	return mediaToVid[*cur]
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
