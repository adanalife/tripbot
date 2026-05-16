package onscreensServer

import (
	"log/slog"
	"time"
)

var Timewarp *Onscreen

var timewarpDuration = time.Duration(2 * time.Second)

func InitTimewarp() {
	slog.Info("creating onscreen", "kind", "timewarp")
	Timewarp = New()
}

func ShowTimewarp() {
	Timewarp.ShowFor("Timewarp!", timewarpDuration)
}
