package onscreensServer

import (
	"log"
	"path/filepath"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
)

var Timewarp *Onscreen

var timewarpDuration = time.Duration(2 * time.Second)
var timewarpFile = filepath.Join(c.Conf.RunDir, "timewarp.txt")

func InitTimewarp() {
	log.Println("Creating timewarp onscreen")
	Timewarp = New(timewarpFile)
}

func ShowTimewarp() {
	Timewarp.ShowFor("Timewarp!", timewarpDuration)
}
