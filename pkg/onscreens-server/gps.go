package onscreensServer

import (
	"log/slog"
	"time"
)

var gpsImage *Onscreen

var gpsDuration = time.Duration(150 * time.Second)

func InitGPSImage() {
	slog.Info("creating onscreen", "kind", "gps")
	gpsImage = New()
}

//TODO: this should probably return an error
func ShowGPSImage() {
	gpsImage.Show("")
}

//TODO: this should probably return an error
func HideGPSImage() {
	gpsImage.Hide()
}
