package onscreensServer

import "log/slog"

// newFlagOnscreen constructs the flag *Onscreen.
//
// The state-driven flag swap is currently disabled — see
// onscreens-client.ShowFlag (no-op) and the placeholder served by the
// /onscreens/asset/flag handler. Bringing it back means re-implementing
// a per-state image picker.
func newFlagOnscreen() *Onscreen {
	slog.Info("creating onscreen", "kind", "flag")
	return newOnscreen()
}
