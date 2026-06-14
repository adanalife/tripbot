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
	osc := newOnscreen()
	// this is a permanent onscreen
	osc.DontExpire = true
	// default to showing; RestoreMiddleText overrides content + visibility
	// from the persisted state when there is any
	osc.IsShowing = true
	return osc
}
