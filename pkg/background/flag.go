package background

import (
	"log"
	"path"

	"github.com/dmerrick/tripbot/pkg/config"
	"github.com/dmerrick/tripbot/pkg/onscreens"
)

var FlagImage *onscreens.Onscreen
var FlagImageFile = path.Join(config.RunDir, "flag.png")

// var flagDuration = time.Duration(150 * time.Second)

func InitFlagImage() {
	log.Println("Creating flag image onscreen")
	FlagImage = onscreens.NewImage(FlagImageFile)
}
