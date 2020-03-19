package background

import (
	"log"
	"path"

	"github.com/dmerrick/tripbot/pkg/config"
	"github.com/dmerrick/tripbot/pkg/onscreens"
)

var middleTextFile = path.Join(config.RunDir, "middle-text.txt")

var MiddleText *onscreens.Onscreen

func InitMiddleText() {
	log.Println("Creating middle text onscreen")
	MiddleText = onscreens.New(middleTextFile)
	// this is a permanent onscreen
	MiddleText.DontExpire = true
	// keep the same text from before the bot started
	MiddleText.IsShowing = true
}
