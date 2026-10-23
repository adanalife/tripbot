package main

import (
	"context"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/natsclient"
)

// egressSnapshotMaxAge is how old platform-gateway's pushed egress snapshot may
// be and still answer a live-check. The gateway republishes every 30s, so three
// missed beats means it has stopped publishing — and the retained snapshot
// outlives the gateway, so past this a snapshot that last said "live" would keep
// saying it through an outage the watchdog has to see as "unknown".
const egressSnapshotMaxAge = 90 * time.Second

// snapshotThenPoll answers a live-check from the pushed egress snapshot when a
// fresh one is retained, and asks poll otherwise. The snapshot costs no gateway
// round trip and is at most one publish interval behind; poll is the fallback
// for a missing, unconfigured or stale snapshot, and its error still surfaces,
// so a gateway outage reads as an unknown answer rather than a live one.
func snapshotThenPoll(read func(context.Context) (eventbus.EgressState, error), poll func(context.Context) (bool, error)) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		if s, err := read(ctx); err == nil && s.Configured && snapshotFresh(s.EmittedAt, time.Now()) {
			return s.Live, nil
		}
		return poll(ctx)
	}
}

// snapshotFresh reports whether emittedAt is within egressSnapshotMaxAge of
// now, either side: a small clock skew between the gateway's pod and this one
// must not disable the snapshot, and a stamp far in the future is as
// untrustworthy as one far in the past.
func snapshotFresh(emittedAt string, now time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, emittedAt)
	if err != nil {
		return false
	}
	age := now.Sub(at)
	return age <= egressSnapshotMaxAge && age >= -egressSnapshotMaxAge
}

// egressSnapshotReader reads this environment's retained snapshot for platform
// over the process's NATS connection.
func (t *Tripbot) egressSnapshotReader(platform string) func(context.Context) (eventbus.EgressState, error) {
	return func(ctx context.Context) (eventbus.EgressState, error) {
		return eventbus.LastEgressState(ctx, natsclient.JetStream(), t.cfg.Environment, platform)
	}
}
