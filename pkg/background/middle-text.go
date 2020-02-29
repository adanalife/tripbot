package background

import (
	"log"
	"path"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
)

var middleTextFile = path.Join(helpers.ProjectRoot(), "OBS/middle-text.txt")

var MiddleText *onscreens.Onscreen

func InitMiddleText() {
	log.Println("Creating middle text onscreen")
	MiddleText = onscreens.New(middleTextFile)
	// this is a permanent onscreen
	MiddleText.DontExpire = true
	// keep the same text from before the bot started
	MiddleText.IsShowing = true
}