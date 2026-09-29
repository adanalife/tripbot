package main

import (
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
)

// The gate is what keeps a once-a-second read from becoming a once-a-second
// publish: a snapshot goes out when it changes, and an unchanged one only as
// the heartbeat.
func TestChangeGate(t *testing.T) {
	var g changeGate
	t0 := time.Now()
	steady := eventbus.AudioBed{Platform: "twitch", Bed: "somafm"}

	if !g.due(steady, t0) {
		t.Fatal("first snapshot held back")
	}
	if g.due(steady, t0.Add(time.Second)) {
		t.Fatal("unchanged snapshot sent again before the heartbeat")
	}
	pending := steady
	pending.Pending = &eventbus.AudioBedSwitch{Bed: "album"}
	if !g.due(pending, t0.Add(2*time.Second)) {
		t.Fatal("a scheduled switch was held back")
	}
	again := steady
	again.Pending = &eventbus.AudioBedSwitch{Bed: "album"}
	if g.due(again, t0.Add(3*time.Second)) {
		t.Fatal("an equal snapshot behind a new pointer counted as a change")
	}
	if !g.due(again, t0.Add(3*time.Second+consoleStateHeartbeat)) {
		t.Fatal("unchanged snapshot never re-sent as the heartbeat")
	}
}
