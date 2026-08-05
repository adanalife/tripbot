package onscreensServer

import (
	"log/slog"
)

// newMiddleText constructs the middle-text *Onscreen. Unlike the other
// onscreens this one is permanent (DontExpire = true) and starts in the
// "showing" state. The actual pre-restart text is restored separately from
// the JetStream last-value cache by Server.RestoreMiddleText (see
// middle-state.go) — this constructor only sets the default before that
// restore runs, so a brand-new server (empty cache) still shows-but-empty.
func newMiddleText() *Onscreen {
	slog.Info("creating onscreen", "kind", "middle-text")
	osc := newOnscreen(defaultSleepInterval)
	// Show is the permanent-and-visible state this overlay wants: it sets
	// dontExpire so the background loop never sweeps it, and starts it showing
	// with empty content. RestoreMiddleText overrides content + visibility from
	// the persisted state when there is any.
	osc.Show("")
	return osc
}
