package onscreensServer

import (
	"log/slog"
	"time"
)

// timewarpDuration controls how long a !timewarp render stays on screen
// before the background expiry sweeper hides it.
var timewarpDuration = time.Duration(2 * time.Second)

// newTimewarp constructs the timewarp *Onscreen.
func newTimewarp() *Onscreen {
	slog.Info("creating onscreen", "kind", "timewarp")
	return newOnscreen()
}
