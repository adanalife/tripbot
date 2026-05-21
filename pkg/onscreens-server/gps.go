package onscreensServer

import (
	"log/slog"
	"time"
)

// gpsDuration is the canonical "how long should the GPS overlay stay
// visible" tunable. Currently unused — the handler uses bare Show() and
// the chatbot side hides explicitly — but kept here so a future
// auto-expiry path doesn't have to re-derive a value.
var gpsDuration = time.Duration(150 * time.Second)

// newGPSOnscreen constructs the GPS *Onscreen.
func newGPSOnscreen() *Onscreen {
	slog.Info("creating onscreen", "kind", "gps")
	return newOnscreen()
}
