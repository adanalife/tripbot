package onscreensServer

import (
	"log/slog"
	"time"
)

var timewarp *Onscreen

var timewarpDuration = time.Duration(2 * time.Second)

func InitTimewarp() {
	timewarp = newTimewarp()
}

// newTimewarp constructs the timewarp *Onscreen and emits the matching
// "creating onscreen" slog line for parity with the legacy InitX free
// functions.
func newTimewarp() *Onscreen {
	slog.Info("creating onscreen", "kind", "timewarp")
	return newOnscreen()
}

func ShowTimewarp() {
	timewarp.ShowFor("Timewarp!", timewarpDuration)
}
