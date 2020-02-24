package main

import (
	"github.com/dmerrick/danalol-stream/pkg/vlc"
)

func main() {
	vlc.Init()
	vlc.LoadMedia()
	vlc.Play()

	defer vlc.Shutdown()
}
