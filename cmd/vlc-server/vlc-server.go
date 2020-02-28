package main

import (
	"log"
	"runtime"
	"time"

	vlcServer "github.com/dmerrick/danalol-stream/pkg/vlc-server"
)

func main() {

	if runtime.GOOS != "darwin" {
		log.Fatal("This doesn't yet work on darwin")
	}

	// start VLC
	vlcServer.Init()
	vlcServer.PlayRandom()

	vlcServer.Start() // starts the server

	time.Sleep(10 * time.Second)
	defer vlcServer.Shutdown()
}
