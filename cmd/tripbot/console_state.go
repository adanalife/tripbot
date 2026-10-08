package main

import (
	"context"
	"reflect"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/server"
)

// consoleStateTick is how often the bed and flag state are re-read for a change.
// Both reads are in-memory, so a second costs nothing and is the latency a
// console sees on a click landing.
const consoleStateTick = time.Second

// consoleStateHeartbeat re-sends an unchanged snapshot, so emitted_at tells a
// subscriber the instance is alive and a publish lost to a NATS reconnect heals.
const consoleStateHeartbeat = 30 * time.Second

// publishConsoleState puts this instance's background-audio bed and feature
// flags on the eventbus (audio.bed.<platform>, flags.snapshot.<platform>) on
// every change, so the console holds them instead of polling GET /api/audio and
// GET /api/flags. It reads the same bed store and flag client those endpoints
// do, so every path that changes either — a console click, `!audio`, the audio
// watchdog, an album advance, a flag edited in the database — is covered
// without each one having to remember to publish.
func (t *Tripbot) publishConsoleState(ctx context.Context) {
	var bedGate, flagGate changeGate
	tick := time.NewTicker(consoleStateTick)
	defer tick.Stop()
	for {
		now := time.Now()
		if t.beds != nil {
			if bed := audioBed(t.cfg.Platform, t.beds); bedGate.due(bed, now) {
				eventbus.EmitAudioBed(ctx, t.cfg.Environment, bed)
			}
		}
		if t.flagClient != nil {
			if flags := server.FeatureFlags(t.flagClient.Snapshot(ctx)); flagGate.due(flags, now) {
				eventbus.EmitFeatureFlags(ctx, t.cfg.Environment, t.cfg.Platform, flags)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// changeGate lets a snapshot through when it differs from the last one sent, or
// when consoleStateHeartbeat has passed since.
type changeGate struct {
	last any
	sent time.Time
}

func (g *changeGate) due(v any, now time.Time) bool {
	if reflect.DeepEqual(v, g.last) && now.Sub(g.sent) < consoleStateHeartbeat {
		return false
	}
	g.last, g.sent = v, now
	return true
}

// audioBed reads the store the way GET /api/audio does, minus the option lists
// and SomaFM's song (a network read, not store state).
func audioBed(platform string, s *beds.Store) eventbus.AudioBed {
	bed, _ := s.Current()
	playing, track := s.Playing()
	out := eventbus.AudioBed{
		Platform:     platform,
		Bed:          string(bed),
		Playing:      string(playing),
		Station:      s.Station(),
		Voicing:      s.Voicing(),
		Album:        s.Album(),
		PlayingAlbum: s.PlayingAlbum(),
		Shuffle:      s.Shuffle(),
	}
	if playing != beds.SomaFM {
		out.Track = beds.TrackTitle(track)
	}
	if sw, ok := s.Pending(); ok {
		out.Pending = &eventbus.AudioBedSwitch{
			Bed:     string(sw.Bed),
			Station: sw.Station,
			Album:   sw.Album,
			Voicing: sw.Voicing,
			At:      sw.At.UTC().Format(time.RFC3339Nano),
		}
	}
	return out
}
