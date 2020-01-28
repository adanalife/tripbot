package audio

import (
	"fmt"
	"log"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/fhs/gompd/mpd"
)

var mpdConn *mpd.Client

const (
	grooveSaladURL = "http://somafm.com/groovesalad256.pls"
	mpdServer      = "localhost:6600"
)

func init() {
	var err error
	// Connect to MPD server
	mpdConn, err = mpd.Dial("tcp", mpdServer)
	if err != nil {
		//TODO: maybe we dont want this hard dependency?
		terrors.Fatal(err, "Error connecting to MPD")
	}

	startGrooveSalad()
}

func Shutdown() {
	err := mpdConn.Close()
	if err != nil {
		terrors.Log(err, "Error while closing MPD connection")
	}
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
