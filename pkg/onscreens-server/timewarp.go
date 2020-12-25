package onscreensServer

import (
	"log"
	"path/filepath"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
)

var Timewarp *Onscreen

var timewarpDuration = time.Duration(2 * time.Second)
var timewarpFile = filepath.Join(config.RunDir, "timewarp.txt")

func InitTimewarp() {
	log.Println("Creating timewarp onscreen")
	Timewarp = New(timewarpFile)
}

func ShowTimewarp() {
	Timewarp.ShowFor("Timewarp!", timewarpDuration)
}
