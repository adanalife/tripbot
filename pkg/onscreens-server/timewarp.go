package onscreensServer

import (
	"log/slog"
	"time"
)

// timewarpDuration controls how long a !timewarp render stays "showing" before
// the background expiry sweeper hides it. It spans the full-screen warp
// animation in the browser source (~3.8s) with a little headroom so the cover
// is still up while the new clip's first frames decode.
var timewarpDuration = time.Duration(4400 * time.Millisecond)

// newTimewarp constructs the timewarp *Onscreen. It checks for expiry more
// often than the default so the onscreen flips back to hidden soon after the
// animation finishes, re-arming the browser source's showing rising-edge for
// the next run.
func newTimewarp() *Onscreen {
	slog.Info("creating onscreen", "kind", "timewarp")
	osc := newOnscreen()
	osc.SleepInterval = time.Second
	return osc
}
