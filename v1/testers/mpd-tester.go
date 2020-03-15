package main

import (
	"fmt"
	"log"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/fhs/gompd/mpd"
)

var mpdServer = "localhost:6600"

func main() {
	// Connect to MPD server
	conn, err := mpd.Dial("tcp", mpdServer)
	if err != nil {
		terrors.Fatal(err, "Error connecting to MPD")
	}
	defer conn.Close()

	// conn.Add("http://somafm.com/groovesalad256.pls")
	// conn.Play(-1)

	output := ""
	status, err := conn.Status()
	if err != nil {
		terrors.Log(err, "Error getting MPD status")
	}
	song, err := conn.CurrentSong()
	if err != nil {
		terrors.Log(err, "Error getting current song from MPD")
	}
	if status["state"] == "play" {
		output = fmt.Sprintf("%s - %s", song["Artist"], song["Title"])
	} else {
		output = fmt.Sprintf("State: %s", status["state"])
	}
	log.Println(output)
}
