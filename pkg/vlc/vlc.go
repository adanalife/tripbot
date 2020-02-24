package vlc

import (
	"log"
	"os"
	"path/filepath"

	theirVlc "github.com/adrg/libvlc-go"
	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
)

//TODO: remove all panics and Fatals

var player *theirVlc.ListPlayer
var mediaList *theirVlc.MediaList

func Init() {
	var err error

	//TODO: add more flags (--no-audio?)
	if err = theirVlc.Init("--quiet"); err != nil {
		log.Fatal(err)
	}

	// create a new player
	player, err = theirVlc.NewListPlayer()
	if err != nil {
		log.Fatal(err)
	}
	mediaList, err = theirVlc.NewMediaList()
	if err != nil {
		log.Fatal(err)
	}

	err = player.SetMediaList(mediaList)
	if err != nil {
		log.Fatal(err)
	}
}

func Shutdown() {
	player.Stop()
	player.Release()
	theirVlc.Release()
}

func LoadMedia() {
	log.Println("loadMedia()")
	var files []string

	err := filepath.Walk(config.VideoDir, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		// add the media to VLC
		err = mediaList.AddMediaFromPath(file)
		if err != nil {
			log.Fatal(err)
		}
	}

	spew.Dump(mediaList)

	//TODO: deal with this
	// defer media.Release()
}

func Play() {
	log.Println("play()")
	// Start playing the media.
	if err := player.Play(); err != nil {
		log.Fatal(err)
	}
}
