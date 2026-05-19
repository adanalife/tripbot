package onscreensServer

import (
	"log/slog"
	"time"
)

var flagImage *Onscreen

func InitFlagImage() {
	flagImage = newFlagOnscreen()
}

// newFlagOnscreen constructs the flag *Onscreen and emits the matching
// "creating onscreen" slog line for parity with the legacy InitX free
// functions.
func newFlagOnscreen() *Onscreen {
	slog.Info("creating onscreen", "kind", "flag")
	return newOnscreen()
}

//TODO: this should probably return an error
//TODO: state-driven flag swap is currently disabled — see
// onscreens-client.ShowFlag (no-op) and the placeholder served by
// vlc-server's /onscreens/asset/flag handler. Bringing it back means
// re-implementing the per-state image picker (was flagSourceFile +
// updateFlagFile, removed in the disk-write cleanup).
func ShowFlag(dur time.Duration) {
	flagImage.ShowFor("", 10*time.Second)
}
