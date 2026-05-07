package onscreensServer

import (
	"log"
	"time"
)

var Timewarp *Onscreen

var timewarpDuration = time.Duration(2 * time.Second)

func InitTimewarp() {
	log.Println("Creating timewarp onscreen")
	Timewarp = New()
}

func ShowTimewarp() {
	Timewarp.ShowFor("Timewarp!", timewarpDuration)
}
