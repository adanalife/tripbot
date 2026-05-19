package onscreensServer

import (
	"log/slog"
	"time"
)

var gpsImage *Onscreen

var gpsDuration = time.Duration(150 * time.Second)

func InitGPSImage() {
	gpsImage = newGPSOnscreen()
}

// newGPSOnscreen constructs the GPS *Onscreen and emits the matching
// "creating onscreen" slog line for parity with the legacy InitX free
// functions.
func newGPSOnscreen() *Onscreen {
	slog.Info("creating onscreen", "kind", "gps")
	return newOnscreen()
}

//TODO: this should probably return an error
func ShowGPSImage() {
	gpsImage.Show("")
}

//TODO: this should probably return an error
func HideGPSImage() {
	gpsImage.Hide()
}
