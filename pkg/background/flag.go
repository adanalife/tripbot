package background

import (
	"log"
	"path"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
)

var FlagImage *onscreens.Onscreen
var FlagImageFile = path.Join(helpers.ProjectRoot(), "OBS/flag.jpg")

// var flagDuration = time.Duration(150 * time.Second)

func InitFlagImage() {
	log.Println("Creating flag image onscreen")
	FlagImage = onscreens.NewImage(FlagImageFile)
}
