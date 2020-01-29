package audio

import (
	"fmt"
	"log"
	"runtime"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/fhs/gompd/mpd"
)

var mpdConn *mpd.Client
var Enabled = true

const (
	grooveSaladURL = "http://somafm.com/groovesalad256.pls"
	mpdServer      = "localhost:6600"
)

func init() {
	var err error

	// disable audio on OS X
	if runtime.GOOS != "linux" {
		log.Println("Disabling audio since we're not on Linux")
		Enabled = false
		return
	}

	// Connect to MPD server
	mpdConn, err = mpd.Dial("tcp", mpdServer)
	if err != nil {
		//TODO: maybe we dont want this hard dependency?
		terrors.Fatal(err, "Error connecting to MPD")
	}

	startGrooveSalad()
}

func mpdState() string {
	status, err := mpdConn.Status()
	if err != nil {
		terrors.Log(err, "Error getting MPD status")
	}
	return status["state"]
}

func startGrooveSalad() {
	if mpdState() != "play" {
		log.Println("Starting Groove Salad")
		err := mpdConn.Add(grooveSaladURL)
		if err != nil {
			terrors.Log(err, "Error adding to MPD playlist")
		}
		// negative values play the current track
		err = mpdConn.Play(-1)
		if err != nil {
			terrors.Log(err, "Error playing MPD track")
		}
	}
}

func CurrentlyPlaying() {
	if Enabled {
		output := ""
		state := mpdState()
		if state != "play" {
			output = fmt.Sprintf("State: %s", state)
			return
		}
		song, err := mpdConn.CurrentSong()
		if err != nil {
			terrors.Log(err, "Error getting current song from MPD")
		}
		output = fmt.Sprintf("%s - %s", song["Artist"], song["Title"])
		log.Println(output)
	}
}

func Shutdown() {
	if Enabled {
		err := mpdConn.Stop()
		if err != nil {
			terrors.Log(err, "Error stopping MPD")
		}
		err = mpdConn.Close()
		if err != nil {
			terrors.Log(err, "Error while closing MPD connection")
		}
	}

}
