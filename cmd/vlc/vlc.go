package main

import (
	"time"

	"github.com/dmerrick/danalol-stream/pkg/vlc"
)

func main() {
	vlc.Init()
	vlc.LoadMedia()
	vlc.Play()

	time.Sleep(30 * time.Second)
	defer vlc.Shutdown()
}
