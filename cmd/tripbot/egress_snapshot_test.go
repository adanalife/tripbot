package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/nats-io/nats.go/jetstream"
)

func TestSnapshotThenPoll(t *testing.T) {
	stamp := func(ago time.Duration) string { return time.Now().Add(-ago).UTC().Format(time.RFC3339Nano) }
	errGateway := errors.New("gateway down")

	tests := []struct {
		name       string
		snapshot   eventbus.EgressState
		readErr    error
		pollLive   bool
		pollErr    error
		wantLive   bool
		wantErr    bool
		wantPolled bool
	}{
		{name: "fresh live snapshot answers without a poll",
			snapshot: eventbus.EgressState{Configured: true, Live: true, EmittedAt: stamp(10 * time.Second)},
			wantLive: true},
		{name: "fresh not-live snapshot answers without a poll",
			snapshot: eventbus.EgressState{Configured: true, Live: false, EmittedAt: stamp(10 * time.Second)},
			pollLive: true, wantLive: false},
		{name: "small forward clock skew still counts as fresh",
			snapshot: eventbus.EgressState{Configured: true, Live: true, EmittedAt: stamp(-5 * time.Second)},
			wantLive: true},
		{name: "stale live snapshot falls back to the poll",
			snapshot: eventbus.EgressState{Configured: true, Live: true, EmittedAt: stamp(egressSnapshotMaxAge + time.Second)},
			pollLive: false, wantLive: false, wantPolled: true},
		{name: "unconfigured snapshot falls back to the poll",
			snapshot: eventbus.EgressState{Configured: false, EmittedAt: stamp(time.Second)},
			pollLive: true, wantLive: true, wantPolled: true},
		{name: "unparseable stamp falls back to the poll",
			snapshot: eventbus.EgressState{Configured: true, Live: true, EmittedAt: "yesterday"},
			pollLive: false, wantLive: false, wantPolled: true},
		{name: "no snapshot falls back to the poll",
			readErr:  jetstream.ErrMsgNotFound,
			pollLive: true, wantLive: true, wantPolled: true},
		{name: "jetstream error falls back to the poll",
			readErr:  errors.New("jetstream unavailable"),
			pollLive: true, wantLive: true, wantPolled: true},
		{name: "stale live snapshot with the gateway down is unknown, not live",
			snapshot: eventbus.EgressState{Configured: true, Live: true, EmittedAt: stamp(5 * time.Minute)},
			pollErr:  errGateway, wantErr: true, wantPolled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			polled := false
			read := func(context.Context) (eventbus.EgressState, error) { return tc.snapshot, tc.readErr }
			poll := func(context.Context) (bool, error) { polled = true; return tc.pollLive, tc.pollErr }

			live, err := snapshotThenPoll(read, poll)(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && live {
				t.Error("an errored check answered live")
			}
			if live != tc.wantLive {
				t.Errorf("live = %v, want %v", live, tc.wantLive)
			}
			if polled != tc.wantPolled {
				t.Errorf("polled = %v, want %v", polled, tc.wantPolled)
			}
		})
	}
}
