package onscreensServer

import (
	"log/slog"
)

var middleText *Onscreen

func InitMiddleText() {
	slog.Info("creating onscreen", "kind", "middle-text")
	middleText = New()
	// this is a permanent onscreen
	middleText.DontExpire = true
	// keep the same text from before the bot started
	middleText.IsShowing = true
}
