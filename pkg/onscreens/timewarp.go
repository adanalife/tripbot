package onscreens

import (
	"log"
	"path"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
)

var Timewarp *Onscreen

var timewarpDuration = time.Duration(2 * time.Second)
var timewarpFile = path.Join(config.RunDir, "timewarp.txt")

func InitTimewarp() {
	log.Println("Creating timewarp onscreen")
	Timewarp = New(timewarpFile)
}

func ShowTimewarp() {
	Timewarp.ShowFor("Timewarp!", timewarpDuration)
}
