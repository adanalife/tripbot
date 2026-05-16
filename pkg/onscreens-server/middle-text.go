package onscreensServer

import (
	"log/slog"
)

var MiddleText *Onscreen

func InitMiddleText() {
	slog.Info("creating onscreen", "kind", "middle-text")
	MiddleText = New()
	// this is a permanent onscreen
	MiddleText.DontExpire = true
	// keep the same text from before the bot started
	MiddleText.IsShowing = true
}
