package onscreensServer

import (
	"log/slog"
	"time"
)

// flagDuration is how long the state flag overlay stays up after a flag.show.
// Server-owned (like gpsDuration) so the publisher doesn't transport it.
var flagDuration = 10 * time.Second

// newFlagOnscreen constructs the flag *Onscreen.
//
// The flag onscreen's Content holds the current state abbreviation (set by
// handleFlagShow); the /onscreens/asset/flag handler resolves it to the
// matching embedded per-state flag image, falling back to a transparent
// placeholder for an unset or unknown state.
func newFlagOnscreen() *Onscreen {
	slog.Info("creating onscreen", "kind", "flag")
	return newOnscreen()
}
