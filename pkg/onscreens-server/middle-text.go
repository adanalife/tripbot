package onscreensServer

import (
	"log"
)

var MiddleText *Onscreen

func InitMiddleText() {
	log.Println("Creating middle text onscreen")
	MiddleText = New()
	// this is a permanent onscreen
	MiddleText.DontExpire = true
	// keep the same text from before the bot started
	MiddleText.IsShowing = true
}
