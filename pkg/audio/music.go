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
	// disable audio on OS X
	if runtime.GOOS != "linux" {
		log.Println("Disabling audio since we're not on Linux")
		Enabled = false
		return
	}

	// connect to the MPD server
	connect()

	//TODO: this shouldn't live in init probably
	startGrooveSalad()
}

func connect() {
	var err error
	// Connect to MPD server
	mpdConn, err = mpd.Dial("tcp", mpdServer)
	if err != nil {
		terrors.Log(err, "Error connecting to MPD")
	}
}

func mpdState() string {
	status, err := mpdConn.Status()
	if err != nil {
		terrors.Log(err, "Error getting MPD status")
		return "error"
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

// RefreshClient is intended to run periodically, because otherwise
// the connection will time out
func RefreshClient() {
	if Enabled {
		state := mpdState()
		log.Println(state)
		if state == "error" {
			connect()
		}
	}
}

func CurrentlyPlaying() string {
	output := ""
	if Enabled {
		state := mpdState()
		if state != "play" {
			output = fmt.Sprintf("Player state: %s", state)
			return output
		}
		song, err := mpdConn.CurrentSong()
		if err != nil {
			terrors.Log(err, "Error getting current song from MPD")
			return ""
		}
		//TODO: there are other attributes in here, use them?
		output = song["Title"]
	}
	return output
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
