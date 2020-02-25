package vlc

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path"
	"path/filepath"

	theirVlc "github.com/adrg/libvlc-go"
	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/mitchellh/go-ps"
)

var player *theirVlc.ListPlayer
var mediaList *theirVlc.MediaList

const pidFile = "OBS/VLC.pid"

func Init() {
	var err error

	//TODO: add more flags (--no-audio?)
	if err = theirVlc.Init("--quiet"); err != nil {
		terrors.Fatal(err, "error initializing VLC")
	}

	// create a new player
	player, err = theirVlc.NewListPlayer()
	if err != nil {
		terrors.Fatal(err, "error creating VLC player")
	}
	mediaList, err = theirVlc.NewMediaList()
	if err != nil {
		terrors.Fatal(err, "error creating VLC media list")
	}

	err = player.SetMediaList(mediaList)
	if err != nil {
		terrors.Fatal(err, "error setting VLC media list")
	}

	// loop forever
	err = player.SetPlaybackMode(theirVlc.Loop)
	if err != nil {
		terrors.Fatal(err, "error setting VLC playback mode")
	}

	loadMedia()
	writePidFile()
}

func writePidFile() {
	vlcBinary := "vlc"

	processes, err := ps.Processes()
	if err != nil {
		terrors.Log(err, "error getting pid for VLC")
	}

	var vlcProcess ps.Process
	for _, p := range processes {
		if p.Executable() == vlcBinary {
			vlcProcess = p
			// ignore other VLC processes
			break
		}
	}

	if vlcProcess != nil {
		log.Printf("pid for VLC is %d, writing to file", vlcProcess.Pid())
		pidPath := path.Join(helpers.ProjectRoot(), pidFile)
		ioutil.WriteFile(pidPath, []byte(fmt.Sprintf("%d", vlcProcess.Pid())), 0664)
	} else {
		log.Println("no VLC process found")
	}
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
	random := uint(rand.Intn(count))
	// start playing the media
	return player.PlayAtIndex(random)
}
