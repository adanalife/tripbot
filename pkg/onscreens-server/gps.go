package onscreensServer

import (
	"log"
	"path"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
)

var GPSImage *Onscreen

var gpsDuration = time.Duration(150 * time.Second)
var gpsImageFile = path.Join(helpers.ProjectRoot(), "assets", "GPS.png")

func InitGPSImage() {
	log.Println("Creating GPS image onscreen")
	GPSImage = NewImage(gpsImageFile)
}
