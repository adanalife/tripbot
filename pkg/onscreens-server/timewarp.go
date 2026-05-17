package onscreensServer

import (
	"log/slog"
	"time"
)

var timewarp *Onscreen

var timewarpDuration = time.Duration(2 * time.Second)

func InitTimewarp() {
	slog.Info("creating onscreen", "kind", "timewarp")
	timewarp = New()
}

func ShowTimewarp() {
	timewarp.ShowFor("Timewarp!", timewarpDuration)
}
