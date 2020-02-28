package main

import (
	"log"
	"runtime"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/vlc"
)

func main() {

	if runtime.GOOS != "darwin" {
		log.Fatal("This doesn't yet work on darwin")
	}

	// start VLC
	vlc.Init()
	vlc.PlayRandom()

	time.Sleep(10 * time.Second)
	defer vlc.Shutdown()
}
