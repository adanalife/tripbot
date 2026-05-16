package onscreensServer

import (
	"log/slog"
	"time"
)

var GPSImage *Onscreen

var gpsDuration = time.Duration(150 * time.Second)

func InitGPSImage() {
	slog.Info("creating onscreen", "kind", "gps")
	GPSImage = New()
}

//TODO: this should probably return an error
func ShowGPSImage() {
	GPSImage.Show("")
}

//TODO: this should probably return an error
func HideGPSImage() {
	GPSImage.Hide()
}
