package onscreensServer

import (
	"log/slog"
)

var middleText *Onscreen

func InitMiddleText() {
	middleText = newMiddleText()
}

// newMiddleText constructs the middle-text *Onscreen. Unlike the other
// onscreens this one is permanent (DontExpire = true) and starts in the
// "showing" state so the OBS browser source keeps rendering whatever
// text was on screen before the bot restarted.
func newMiddleText() *Onscreen {
	slog.Info("creating onscreen", "kind", "middle-text")
	osc := newOnscreen()
	// this is a permanent onscreen
	osc.DontExpire = true
	// keep the same text from before the bot started
	osc.IsShowing = true
	return osc
}
