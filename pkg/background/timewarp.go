package background

import (
	"log"
	"path"
	"time"

	"github.com/dmerrick/tripbot/pkg/config"
	"github.com/dmerrick/tripbot/pkg/onscreens"
)

var Timewarp *onscreens.Onscreen

var timewarpDuration = time.Duration(2 * time.Second)
var timewarpFile = path.Join(config.RunDir, "timewarp.txt")

func InitTimewarp() {
	log.Println("Creating timewarp onscreen")
	Timewarp = onscreens.New(timewarpFile)
}

func ShowTimewarp() {
	Timewarp.ShowFor("Timewarp!", timewarpDuration)
}
