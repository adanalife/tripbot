package main

import (
	"log"
	"math/rand"
	"time"

	"github.com/dmerrick/tripbot/pkg/helpers"
	vlcServer "github.com/dmerrick/tripbot/pkg/vlc-server"
)

func main() {
	// we don't yet support libvlc on darwin
	if helpers.RunningOnDarwin() {
		log.Fatal("This doesn't yet work on darwin")
	}

	// create a brand new random seed
	rand.Seed(time.Now().UnixNano())

	// start VLC
	vlcServer.InitPlayer()
	// start by playing a random video
	vlcServer.PlayRandom()

	// start the webserver
	vlcServer.Start() // starts the server

	defer vlcServer.Shutdown()
}
