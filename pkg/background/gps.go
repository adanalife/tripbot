package background

import (
	"log"
	"path"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
)

var GPSImage *onscreens.Onscreen

var gpsDuration = time.Duration(150 * time.Second)
var gpsImageFile = path.Join(config.RunDir, "GPS.png")

func InitGPSImage() {
	log.Println("Creating GPS image onscreen")
	GPSImage = onscreens.NewImage(gpsImageFile)
}
