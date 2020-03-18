package background

import (
	"log"
	"path"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
)

var GPSImage *onscreens.Onscreen

var gpsDuration = time.Duration(150 * time.Second)
var gpsImageFile = path.Join(helpers.ProjectRoot(), "assets", "GPS.png")

func InitGPSImage() {
	log.Println("Creating GPS image onscreen")
	GPSImage = onscreens.NewImage(gpsImageFile)
}
