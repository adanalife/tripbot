package onscreens

import (
	"log"
	"path"

	"github.com/adanalife/tripbot/pkg/config"
)

var FlagImage *Onscreen
var FlagImageFile = path.Join(config.RunDir, "flag.png")

// var flagDuration = time.Duration(150 * time.Second)

func InitFlagImage() {
	log.Println("Creating flag image onscreen")
	FlagImage = NewImage(FlagImageFile)
}
