package onscreensServer

import (
	"log/slog"
	"time"
)

var FlagImage *Onscreen

func InitFlagImage() {
	slog.Info("creating onscreen", "kind", "flag")
	FlagImage = New()
}

//TODO: this should probably return an error
//TODO: state-driven flag swap is currently disabled — see
// onscreens-client.ShowFlag (no-op) and the placeholder served by
// vlc-server's /onscreens/asset/flag handler. Bringing it back means
// re-implementing the per-state image picker (was flagSourceFile +
// updateFlagFile, removed in the disk-write cleanup).
func ShowFlag(dur time.Duration) {
	FlagImage.ShowFor("", 10*time.Second)
}
