package onscreensServer

import (
	"log/slog"
)

// newGPSOnscreen constructs the GPS *Onscreen.
func newGPSOnscreen() *Onscreen {
	slog.Info("creating onscreen", "kind", "gps")
	return newOnscreen()
}
