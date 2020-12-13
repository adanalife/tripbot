package onscreensServer

import (
	"log"
	"path/filepath"

	"github.com/adanalife/tripbot/pkg/config"
)

var middleTextFile = filepath.Join(config.RunDir, "middle-text.txt")

var MiddleText *Onscreen

func InitMiddleText() {
	log.Println("Creating middle text onscreen")
	MiddleText = New(middleTextFile)
	// this is a permanent onscreen
	MiddleText.DontExpire = true
	// keep the same text from before the bot started
	MiddleText.IsShowing = true
}
