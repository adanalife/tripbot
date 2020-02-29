package background

import (
	"log"
	"path"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
)

var Timewarp *onscreens.Onscreen

var timewarpDuration = time.Duration(6 * time.Second)
var timewarpFile = path.Join(helpers.ProjectRoot(), "OBS/timewarp.txt")

func InitTimewarp() {
	log.Println("Creating timewarp onscreen")
	Timewarp = onscreens.New(timewarpFile)
}

func ShowTimewarp() {
	Timewarp.ShowFor("Timewarp!", timewarpDuration)
}
