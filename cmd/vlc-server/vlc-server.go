package main

import (
	"log"
	"math/rand"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	vlcServer "github.com/adanalife/tripbot/pkg/vlc-server"
)

// createOnscreens starts the various onscreen elements
// (like the chat boxes in the corners)
func createOnscreens() {
	onscreensServer.InitChat()
	onscreensServer.InitLeftRotator()
	onscreensServer.InitRightRotator()
	onscreensServer.InitMiddleText()
	onscreensServer.InitTimewarp()
	onscreensServer.InitFlagImage()
}

func main() {

	// we don't yet support libvlc on darwin
	if helpers.RunningOnDarwin() {
		log.Fatal("This doesn't yet work on darwin")
	}

	// create a brand new random seed
	rand.Seed(time.Now().UnixNano())

	// initialize the onscreen elements
	createOnscreens()

	// start VLC
	vlcServer.InitPlayer()
	// start by playing a random video
	vlcServer.PlayRandom()

	// start the webserver
	vlcServer.Start() // starts the server

	defer vlcServer.Shutdown()
}
